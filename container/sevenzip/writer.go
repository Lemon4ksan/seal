// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/lemon4ksan/seal/codec/lzma"
	"github.com/lemon4ksan/seal/file"
)

// Writer implements 7-Zip archive serialization.
type Writer struct {
	w       io.Writer
	options lzma.Options
}

// NewWriter constructs a new 7z Writer.
func NewWriter(w io.Writer, opts ...lzma.Options) *Writer {
	var options lzma.Options
	if len(opts) > 0 {
		options = opts[0]
	}
	return &Writer{w: w, options: options}
}

// WriteArchive serializes files into a standard 7-Zip archive.
func (zw *Writer) WriteArchive(files []*file.File) (int64, error) {
	var totalWritten int64

	// Emit 7z magic signature and allocate 32-byte StartHeader slot for deferred CRC patch
	var sigAndStart [StartHeaderSize]byte
	copy(sigAndStart[:SignatureSize], Signature[:])
	sigAndStart[6] = 0 // Major 0
	sigAndStart[7] = 4 // Minor 4

	if _, err := zw.w.Write(sigAndStart[:]); err != nil {
		return 0, err
	}
	totalWritten += int64(StartHeaderSize)

	// Sort files by directory status, extension, and path name to maximize solid compression locality
	sortedFiles := make([]*file.File, len(files))
	copy(sortedFiles, files)
	slices.SortStableFunc(sortedFiles, func(a, b *file.File) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		extA := strings.ToLower(filepath.Ext(a.Name()))
		extB := strings.ToLower(filepath.Ext(b.Name()))
		if extA != extB {
			return strings.Compare(extA, extB)
		}
		return strings.Compare(a.Name(), b.Name())
	})

	// Collect file descriptors, calculate CRCs, and concatenate non-empty payloads into solid block
	var fileEntries []FileEntry
	var unpackSizes []uint64

	type preparedFile struct {
		entry     FileEntry
		data      []byte
		err       error
		unpackLen uint64
	}

	prep := make([]preparedFile, len(sortedFiles))
	numWorkers := min(runtime.GOMAXPROCS(0), len(sortedFiles))
	if numWorkers <= 0 {
		numWorkers = 1
	}

	var wg sync.WaitGroup
	fileChan := make(chan int, len(sortedFiles))
	for i := range sortedFiles {
		fileChan <- i
	}
	close(fileChan)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range fileChan {
				f := sortedFiles[i]
				if f.IsDir() {
					var winAttr uint32 = 0x10 // FILE_ATTRIBUTE_DIRECTORY
					posixMode := uint32(f.Mode().Perm())
					if posixMode == 0 {
						posixMode = 0o755
					}
					posixMode |= 0o040000 // S_IFDIR
					attrib := (posixMode << 16) | winAttr

					prep[i].entry = FileEntry{
						Name:          f.Name(),
						IsDir:         true,
						IsEmptyStream: true,
						IsEmptyFile:   true,
						ModTime:       f.ModTime(),
						HasModTime:    true,
						Attrib:        attrib,
						HasAttrib:     true,
					}
					continue
				}

				rc, err := f.Open()
				if err != nil {
					prep[i].err = err
					return
				}
				var data []byte
				sz := f.Size()
				if sz > 0 && sz < 1<<31 {
					data = make([]byte, sz)
					_, err = io.ReadFull(rc, data)
				} else {
					data, err = io.ReadAll(rc)
				}
				_ = rc.Close()
				if err != nil {
					prep[i].err = err
					return
				}

				var winAttr uint32 = 0x20 // FILE_ATTRIBUTE_ARCHIVE
				posixMode := uint32(f.Mode().Perm())
				if posixMode == 0 {
					posixMode = 0o644
				}
				posixMode |= 0o100000 // S_IFREG
				attrib := (posixMode << 16) | winAttr

				entry := FileEntry{
					Name:       f.Name(),
					IsDir:      false,
					Size:       uint64(len(data)),
					CRC:        crc32.ChecksumIEEE(data),
					HasCRC:     true,
					ModTime:    f.ModTime(),
					HasModTime: true,
					Attrib:     attrib,
					HasAttrib:  true,
				}
				prep[i].entry = entry
				prep[i].data = data
				prep[i].unpackLen = entry.Size
			}
		}()
	}

	wg.Wait()

	var totalUnpackSize uint64
	for i := range prep {
		if prep[i].err != nil {
			return totalWritten, prep[i].err
		}
		fileEntries = append(fileEntries, prep[i].entry)
		if !prep[i].entry.IsDir {
			totalUnpackSize += prep[i].unpackLen
			unpackSizes = append(unpackSizes, prep[i].unpackLen)
		}
	}

	rawBytes := make([]byte, totalUnpackSize)
	var copyOff uint64
	for i := range prep {
		if !prep[i].entry.IsDir && len(prep[i].data) > 0 {
			copy(rawBytes[copyOff:], prep[i].data)
			copyOff += uint64(len(prep[i].data))
			prep[i].data = nil // release slice to assist GC
		}
	}
	unpackCRC := crc32.ChecksumIEEE(rawBytes)

	// Stream solid payload through multithreaded LZMA2 compressor
	var compBuf bytes.Buffer
	var comp2 *lzma.Compressor2
	if zw.options.Level != 0 || zw.options.DictSize != 0 || zw.options.ChainLength != 0 {
		comp2 = lzma.NewCompressor2WithOptions(zw.options)
	} else {
		comp2 = lzma.NewCompressor2(8 * 1024 * 1024)
	}
	if _, err := comp2.Compress(bytes.NewReader(rawBytes), &compBuf); err != nil {
		return totalWritten, err
	}

	compBytes := compBuf.Bytes()
	packSize := uint64(len(compBytes))

	if _, err := zw.w.Write(compBytes); err != nil {
		return totalWritten, err
	}
	totalWritten += int64(len(compBytes))

	// Construct 7z binary metadata structures (PackInfo, UnpackInfo, SubStreamsInfo, FilesInfo)
	var headerBuf bytes.Buffer

	// NIDHeader
	headerBuf.WriteByte(NIDHeader)

	// MainStreamsInfo
	headerBuf.WriteByte(NIDMainStreamsInfo)

	// PackInfo
	headerBuf.WriteByte(NIDPackInfo)
	_ = WriteNumber(&headerBuf, 0) // PackPos = 0
	_ = WriteNumber(&headerBuf, 1) // NumPackStreams = 1
	headerBuf.WriteByte(NIDSize)
	_ = WriteNumber(&headerBuf, packSize)
	headerBuf.WriteByte(NIDEnd)

	// UnpackInfo
	headerBuf.WriteByte(NIDUnpackInfo)
	headerBuf.WriteByte(NIDFolder)
	_ = WriteNumber(&headerBuf, 1) // 1 Folder
	headerBuf.WriteByte(0)         // External = 0

	// 1 Coder (LZMA2: 0x21, 1-byte prop 0x18 = 8MB)
	_ = WriteNumber(&headerBuf, 1) // NumCoders = 1
	headerBuf.WriteByte(0x21)      // MainByte: 1-byte ID, Simple coder, has properties
	headerBuf.WriteByte(0x21)      // ID = 0x21 (LZMA2)
	_ = WriteNumber(&headerBuf, 1) // PropSize = 1
	headerBuf.WriteByte(0x18)      // 8MB dict

	// CodersUnpackSize
	headerBuf.WriteByte(NIDCodersUnpackSize)
	_ = WriteNumber(&headerBuf, totalUnpackSize)

	// CRC
	headerBuf.WriteByte(NIDCRC)
	headerBuf.WriteByte(1) // AllDefined = 1
	var crcB [4]byte
	binary.LittleEndian.PutUint32(crcB[:], unpackCRC)
	headerBuf.Write(crcB[:])

	headerBuf.WriteByte(NIDEnd) // End UnpackInfo

	// SubStreamsInfo
	if len(unpackSizes) > 1 {
		headerBuf.WriteByte(NIDSubStreamsInfo)
		headerBuf.WriteByte(NIDNumUnpackStream)
		_ = WriteNumber(&headerBuf, uint64(len(unpackSizes)))

		headerBuf.WriteByte(NIDSize)
		for _, sz := range unpackSizes[:len(unpackSizes)-1] {
			_ = WriteNumber(&headerBuf, sz)
		}

		headerBuf.WriteByte(NIDCRC)
		headerBuf.WriteByte(1) // AllDefined
		for _, fe := range fileEntries {
			if !fe.IsDir {
				binary.LittleEndian.PutUint32(crcB[:], fe.CRC)
				headerBuf.Write(crcB[:])
			}
		}

		headerBuf.WriteByte(NIDEnd)
	}

	headerBuf.WriteByte(NIDEnd) // End MainStreamsInfo

	// FilesInfo
	headerBuf.WriteByte(NIDFilesInfo)
	_ = WriteNumber(&headerBuf, uint64(len(fileEntries)))

	// Check EmptyStreams
	var emptyCount int
	for _, fe := range fileEntries {
		if fe.IsEmptyStream {
			emptyCount++
		}
	}
	if emptyCount > 0 {
		headerBuf.WriteByte(NIDEmptyStream)
		numBytes := (len(fileEntries) + 7) / 8
		_ = WriteNumber(&headerBuf, uint64(numBytes))
		bitmask := make([]byte, numBytes)
		for i, fe := range fileEntries {
			if fe.IsEmptyStream {
				byteIdx := i / 8
				bitIdx := 7 - (i % 8)
				bitmask[byteIdx] |= (1 << bitIdx)
			}
		}
		headerBuf.Write(bitmask)
	}

	// Names (UTF-16LE + 2 zero bytes)
	headerBuf.WriteByte(NIDName)
	var namesBuf bytes.Buffer
	namesBuf.WriteByte(0) // External = 0
	for _, fe := range fileEntries {
		u16 := utf16.Encode([]rune(fe.Name))
		for _, v := range u16 {
			var b [2]byte
			binary.LittleEndian.PutUint16(b[:], v)
			namesBuf.Write(b[:])
		}
		namesBuf.Write([]byte{0, 0}) // Zero terminator
	}
	_ = WriteNumber(&headerBuf, uint64(namesBuf.Len()))
	headerBuf.Write(namesBuf.Bytes())

	// WinAttrib
	headerBuf.WriteByte(NIDWinAttrib)
	var attrBuf bytes.Buffer
	attrBuf.WriteByte(1) // AllDefined = 1
	attrBuf.WriteByte(0) // External = 0
	for _, fe := range fileEntries {
		var b [4]byte
		binary.LittleEndian.PutUint32(b[:], fe.Attrib)
		attrBuf.Write(b[:])
	}
	_ = WriteNumber(&headerBuf, uint64(attrBuf.Len()))
	headerBuf.Write(attrBuf.Bytes())

	headerBuf.WriteByte(NIDEnd) // End FilesInfo
	headerBuf.WriteByte(NIDEnd) // End Header

	headerBytes := headerBuf.Bytes()
	nextHeaderOffset := packSize
	nextHeaderSize := uint64(len(headerBytes))
	nextHeaderCRC := crc32.ChecksumIEEE(headerBytes)

	// Write actual Header
	if _, err := zw.w.Write(headerBytes); err != nil {
		return totalWritten, err
	}
	totalWritten += int64(len(headerBytes))

	// If writer is an io.WriteSeeker, write back the actual StartHeader at offset 0
	if ws, ok := zw.w.(io.WriteSeeker); ok {
		var startHeaderBuf [20]byte
		binary.LittleEndian.PutUint64(startHeaderBuf[0:8], nextHeaderOffset)
		binary.LittleEndian.PutUint64(startHeaderBuf[8:16], nextHeaderSize)
		binary.LittleEndian.PutUint32(startHeaderBuf[16:20], nextHeaderCRC)
		startCRC := crc32.ChecksumIEEE(startHeaderBuf[:])

		var finalStart [StartHeaderSize - SignatureSize]byte
		finalStart[0] = 0 // Major
		finalStart[1] = 4 // Minor
		binary.LittleEndian.PutUint32(finalStart[2:6], startCRC)
		copy(finalStart[6:26], startHeaderBuf[:])

		if _, err := ws.Seek(SignatureSize, io.SeekStart); err == nil {
			_, _ = ws.Write(finalStart[:])
			_, _ = ws.Seek(totalWritten, io.SeekStart)
		}
	}

	return totalWritten, nil
}
