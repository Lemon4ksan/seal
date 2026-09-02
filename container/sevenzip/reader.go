// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/lemon4ksan/seal/file"
)

var (
	// ErrInvalidSignature indicates the input stream does not start with '7', 'z', ...
	ErrInvalidSignature = errors.New("sevenzip: invalid 7z magic signature")
	// ErrCorruptedHeader indicates header CRC mismatch or structural violation.
	ErrCorruptedHeader = errors.New("sevenzip: corrupted archive header")
)

// Reader parses and extracts files from a 7-Zip archive.
type Reader struct {
	rAt      io.ReaderAt
	size     int64
	password string

	folders        []Folder
	subStreamSizes [][]uint64
	files          []FileEntry

	packPos   int64
	packSizes []uint64
	packCRCs  []uint32

	entries []*file.File

	folderCacheMu sync.Mutex
	folderCache   map[int][]byte
}

// NewReader constructs a 7z archive reader.
func NewReader(rAt io.ReaderAt, size int64, password string) (*Reader, error) {
	if size < StartHeaderSize {
		return nil, ErrCorruptedHeader
	}

	zr := &Reader{
		rAt:         rAt,
		size:        size,
		password:    password,
		folderCache: make(map[int][]byte),
	}

	if err := zr.readStartHeader(); err != nil {
		return nil, err
	}

	return zr, nil
}

// Entries returns the slice of canonical file entries in the archive.
func (zr *Reader) Entries() []*file.File {
	return zr.entries
}

func (zr *Reader) readStartHeader() error {
	var sig [SignatureSize]byte
	if _, err := zr.rAt.ReadAt(sig[:], 0); err != nil {
		return err
	}
	if sig != Signature {
		return ErrInvalidSignature
	}

	var startBuf [StartHeaderSize - SignatureSize]byte
	if _, err := zr.rAt.ReadAt(startBuf[:], SignatureSize); err != nil {
		return err
	}

	startCRC := binary.LittleEndian.Uint32(startBuf[2:6])
	calcCRC := crc32.ChecksumIEEE(startBuf[6:26])
	if startCRC != calcCRC {
		return ErrCorruptedHeader
	}

	nextOffset := binary.LittleEndian.Uint64(startBuf[6:14])
	nextSize := binary.LittleEndian.Uint64(startBuf[14:22])
	nextCRC := binary.LittleEndian.Uint32(startBuf[22:26])

	headerOffset := int64(StartHeaderSize) + int64(nextOffset)
	if headerOffset+int64(nextSize) > zr.size {
		return ErrCorruptedHeader
	}

	headerData := make([]byte, nextSize)
	if _, err := zr.rAt.ReadAt(headerData, headerOffset); err != nil {
		return err
	}

	if crc32.ChecksumIEEE(headerData) != nextCRC {
		return ErrCorruptedHeader
	}

	zr.packPos = int64(StartHeaderSize)

	return zr.parseHeader(bytes.NewReader(headerData))
}

func (zr *Reader) parseHeader(r io.Reader) error {
	for {
		var nidBuf [1]byte
		_, err := r.Read(nidBuf[:])
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		nid := nidBuf[0]
		switch nid {
		case NIDEnd:
			return zr.buildEntries()

		case NIDHeader:
			continue

		case NIDEncodedHeader:
			if err := zr.readEncodedHeader(r); err != nil {
				return err
			}
			return zr.buildEntries()

		case NIDMainStreamsInfo:
			if err := zr.readMainStreamsInfo(r); err != nil {
				return err
			}

		case NIDFilesInfo:
			if err := zr.readFilesInfo(r); err != nil {
				return err
			}

		default:
			continue
		}
	}

	return zr.buildEntries()
}

func (zr *Reader) readEncodedHeader(r io.Reader) error {
	if err := zr.readMainStreamsInfo(r); err != nil {
		return err
	}
	if len(zr.folders) == 0 {
		return errors.New("sevenzip: encoded header missing folder")
	}

	data, err := zr.decompressFolder(0)
	if err != nil {
		return err
	}

	zr.packPos = int64(StartHeaderSize)
	zr.folders = nil
	zr.subStreamSizes = nil
	zr.folderCache = make(map[int][]byte)

	innerReader := bytes.NewReader(data)
	return zr.parseHeader(innerReader)
}

func (zr *Reader) readMainStreamsInfo(r io.Reader) error {
	for {
		var nidBuf [1]byte
		if _, err := io.ReadFull(r, nidBuf[:]); err != nil {
			return err
		}
		nid := nidBuf[0]
		if nid == NIDEnd {
			break
		}

		switch nid {
		case NIDPackInfo:
			if err := zr.readPackInfo(r); err != nil {
				return err
			}
		case NIDUnpackInfo:
			if err := zr.readUnpackInfo(r); err != nil {
				return err
			}
		case NIDSubStreamsInfo:
			if err := zr.readSubStreamsInfo(r); err != nil {
				return err
			}
		}
	}
	return nil
}

func (zr *Reader) readPackInfo(r io.Reader) error {
	packPos, err := ReadNumber(r)
	if err != nil {
		return err
	}
	zr.packPos += int64(packPos)

	numPackStreams, err := ReadNumber(r)
	if err != nil {
		return err
	}

	zr.packSizes = make([]uint64, numPackStreams)

	for {
		var nidBuf [1]byte
		if _, err := io.ReadFull(r, nidBuf[:]); err != nil {
			return err
		}
		nid := nidBuf[0]
		if nid == NIDEnd {
			break
		}

		switch nid {
		case NIDSize:
			for i := uint64(0); i < numPackStreams; i++ {
				size, err := ReadNumber(r)
				if err != nil {
					return err
				}
				zr.packSizes[i] = size
			}
		case NIDCRC:
			defined, err := ReadBoolVector(r, int(numPackStreams))
			if err != nil {
				return err
			}
			zr.packCRCs = make([]uint32, numPackStreams)
			for i, def := range defined {
				if def {
					var crcBuf [4]byte
					if _, err := io.ReadFull(r, crcBuf[:]); err != nil {
						return err
					}
					zr.packCRCs[i] = binary.LittleEndian.Uint32(crcBuf[:])
				}
			}
		}
	}
	return nil
}

func (zr *Reader) readUnpackInfo(r io.Reader) error {
	for {
		var nidBuf [1]byte
		if _, err := io.ReadFull(r, nidBuf[:]); err != nil {
			return err
		}
		nid := nidBuf[0]
		if nid == NIDEnd {
			break
		}

		switch nid {
		case NIDFolder:
			numFolders, err := ReadNumber(r)
			if err != nil {
				return err
			}
			var ext [1]byte
			if _, err := io.ReadFull(r, ext[:]); err != nil {
				return err
			}

			zr.folders = make([]Folder, numFolders)
			zr.subStreamSizes = make([][]uint64, numFolders)

			for i := uint64(0); i < numFolders; i++ {
				numCoders, err := ReadNumber(r)
				if err != nil {
					return err
				}
				folder := Folder{
					Coders:           make([]CoderInfo, numCoders),
					NumUnpackStreams: 1,
				}

				totalInStreams := 0
				totalOutStreams := 0

				for j := uint64(0); j < numCoders; j++ {
					var mainByte [1]byte
					if _, err := io.ReadFull(r, mainByte[:]); err != nil {
						return err
					}
					b := mainByte[0]
					idSize := int(b & 0x0F)
					idBytes := make([]byte, idSize)
					if _, err := io.ReadFull(r, idBytes); err != nil {
						return err
					}

					var methodID uint64
					for _, b := range idBytes {
						methodID = (methodID << 8) | uint64(b)
					}

					inStreams := 1
					outStreams := 1
					if (b & 0x10) != 0 {
						inCount, _ := ReadNumber(r)
						outCount, _ := ReadNumber(r)
						inStreams = int(inCount)
						outStreams = int(outCount)
					}

					var props []byte
					if (b & 0x20) != 0 {
						propSize, _ := ReadNumber(r)
						props = make([]byte, propSize)
						if _, err := io.ReadFull(r, props); err != nil {
							return err
						}
					}

					folder.Coders[j] = CoderInfo{
						MethodID:      methodID,
						NumInStreams:  inStreams,
						NumOutStreams: outStreams,
						Properties:    props,
					}

					totalInStreams += inStreams
					totalOutStreams += outStreams
				}

				numBindPairs := totalOutStreams - 1
				folder.BindPairs = make([]BindPair, numBindPairs)
				for j := 0; j < numBindPairs; j++ {
					inIdx, _ := ReadNumber(r)
					outIdx, _ := ReadNumber(r)
					folder.BindPairs[j] = BindPair{InIndex: int(inIdx), OutIndex: int(outIdx)}
				}

				numPackedStreams := totalInStreams - numBindPairs
				if numPackedStreams == 1 {
					folder.PackedIndices = []int{0}
				} else {
					folder.PackedIndices = make([]int, numPackedStreams)
					for j := 0; j < numPackedStreams; j++ {
						idx, _ := ReadNumber(r)
						folder.PackedIndices[j] = int(idx)
					}
				}

				zr.folders[i] = folder
			}

		case NIDCodersUnpackSize:
			for i := range zr.folders {
				totalOut := 0
				for _, c := range zr.folders[i].Coders {
					totalOut += c.NumOutStreams
				}
				zr.folders[i].UnpackSizes = make([]uint64, totalOut)
				for j := 0; j < totalOut; j++ {
					sz, err := ReadNumber(r)
					if err != nil {
						return err
					}
					zr.folders[i].UnpackSizes[j] = sz
				}
				if len(zr.folders[i].UnpackSizes) > 0 {
					zr.subStreamSizes[i] = []uint64{zr.folders[i].UnpackSizes[0]}
				}
			}

		case NIDCRC:
			defined, err := ReadBoolVector(r, len(zr.folders))
			if err != nil {
				return err
			}
			for i, def := range defined {
				if def {
					var crcBuf [4]byte
					if _, err := io.ReadFull(r, crcBuf[:]); err != nil {
						return err
					}
					zr.folders[i].UnpackCRC = binary.LittleEndian.Uint32(crcBuf[:])
					zr.folders[i].HasCRC = true
				}
			}
		}
	}
	return nil
}

func (zr *Reader) readSubStreamsInfo(r io.Reader) error {
	for {
		var nidBuf [1]byte
		if _, err := io.ReadFull(r, nidBuf[:]); err != nil {
			return err
		}
		nid := nidBuf[0]
		if nid == NIDEnd {
			break
		}

		switch nid {
		case NIDNumUnpackStream:
			for i := range zr.folders {
				num, err := ReadNumber(r)
				if err != nil {
					return err
				}
				zr.folders[i].NumUnpackStreams = int(num)
			}
		case NIDSize:
			for i := range zr.folders {
				count := zr.folders[i].NumUnpackStreams
				if count <= 1 {
					if len(zr.folders[i].UnpackSizes) > 0 {
						zr.subStreamSizes[i] = []uint64{zr.folders[i].UnpackSizes[0]}
					}
					continue
				}
				var sum uint64
				zr.subStreamSizes[i] = make([]uint64, count)
				for j := 0; j < count-1; j++ {
					sz, err := ReadNumber(r)
					if err != nil {
						return err
					}
					zr.subStreamSizes[i][j] = sz
					sum += sz
				}
				if len(zr.folders[i].UnpackSizes) > 0 {
					total := zr.folders[i].UnpackSizes[0]
					if total >= sum {
						zr.subStreamSizes[i][count-1] = total - sum
					}
				}
			}
		case NIDCRC:
			numStreams := 0
			for _, folder := range zr.folders {
				if folder.NumUnpackStreams > 0 {
					numStreams += folder.NumUnpackStreams
				} else {
					numStreams++
				}
			}
			defined, err := ReadBoolVector(r, numStreams)
			if err != nil {
				return err
			}
			for _, def := range defined {
				if def {
					var crcBuf [4]byte
					if _, err := io.ReadFull(r, crcBuf[:]); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (zr *Reader) readFilesInfo(r io.Reader) error {
	numFiles, err := ReadNumber(r)
	if err != nil {
		return err
	}

	zr.files = make([]FileEntry, numFiles)

	for {
		var nidBuf [1]byte
		if _, err := io.ReadFull(r, nidBuf[:]); err != nil {
			return err
		}
		nid := nidBuf[0]
		if nid == NIDEnd {
			break
		}

		size, err := ReadNumber(r)
		if err != nil {
			return err
		}

		switch nid {
		case NIDEmptyStream:
			bitmask := make([]byte, size)
			if _, err := io.ReadFull(r, bitmask); err != nil {
				return err
			}
			for i := 0; i < int(numFiles); i++ {
				byteIdx := i / 8
				bitIdx := 7 - (i % 8)
				if byteIdx < len(bitmask) {
					zr.files[i].IsEmptyStream = (bitmask[byteIdx] & (1 << bitIdx)) != 0
				}
			}

		case NIDEmptyFile:
			bitmask := make([]byte, size)
			if _, err := io.ReadFull(r, bitmask); err != nil {
				return err
			}
			emptyStreamIdx := 0
			for i := range zr.files {
				if zr.files[i].IsEmptyStream {
					byteIdx := emptyStreamIdx / 8
					bitIdx := 7 - (emptyStreamIdx % 8)
					if byteIdx < len(bitmask) {
						if (bitmask[byteIdx] & (1 << bitIdx)) != 0 {
							zr.files[i].IsDir = false
						} else {
							zr.files[i].IsDir = true
						}
					}
					emptyStreamIdx++
				}
			}

		case NIDName:
			var ext [1]byte
			if _, err := io.ReadFull(r, ext[:]); err != nil {
				return err
			}
			for i := range zr.files {
				name, err := readUTF16String(r)
				if err != nil {
					return err
				}
				zr.files[i].Name = name
			}

		case NIDMTime:
			defined, _ := ReadBoolVector(r, int(numFiles))
			var ext [1]byte
			_, _ = io.ReadFull(r, ext[:])
			for i, def := range defined {
				if def {
					var timeBuf [8]byte
					_, _ = io.ReadFull(r, timeBuf[:])
					winTime := binary.LittleEndian.Uint64(timeBuf[:])
					zr.files[i].ModTime = windowsTimeToTime(winTime)
					zr.files[i].HasModTime = true
				}
			}

		case NIDWinAttrib:
			defined, _ := ReadBoolVector(r, int(numFiles))
			var ext [1]byte
			_, _ = io.ReadFull(r, ext[:])
			for i, def := range defined {
				if def {
					var attrBuf [4]byte
					_, _ = io.ReadFull(r, attrBuf[:])
					zr.files[i].Attrib = binary.LittleEndian.Uint32(attrBuf[:])
					zr.files[i].HasAttrib = true
					if (zr.files[i].Attrib & 0x10) != 0 {
						zr.files[i].IsDir = true
					}
				}
			}

		default:
			_, _ = io.CopyN(io.Discard, r, int64(size))
		}
	}

	return nil
}

func readUTF16String(r io.Reader) (string, error) {
	var u16 []uint16
	for {
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		val := binary.LittleEndian.Uint16(b[:])
		if val == 0 {
			break
		}
		u16 = append(u16, val)
	}
	return string(utf16.Decode(u16)), nil
}

func windowsTimeToTime(winTime uint64) time.Time {
	const epochDiff = 116444736000000000
	if winTime < epochDiff {
		return time.Unix(0, 0)
	}
	unixNsec := int64(winTime-epochDiff) * 100
	return time.Unix(0, unixNsec)
}

func (zr *Reader) decompressFolder(folderIdx int) ([]byte, error) {
	zr.folderCacheMu.Lock()
	if cached, ok := zr.folderCache[folderIdx]; ok {
		zr.folderCacheMu.Unlock()
		return cached, nil
	}
	zr.folderCacheMu.Unlock()

	if folderIdx >= len(zr.folders) {
		return nil, errors.New("sevenzip: invalid folder index")
	}

	folder := &zr.folders[folderIdx]

	var packedStreams []io.Reader
	currPackPos := zr.packPos

	for _, packSize := range zr.packSizes {
		sr := io.NewSectionReader(zr.rAt, currPackPos, int64(packSize))
		packedStreams = append(packedStreams, sr)
		currPackPos += int64(packSize)
	}

	if len(packedStreams) == 0 {
		packedStreams = []io.Reader{io.NewSectionReader(zr.rAt, zr.packPos, zr.size-zr.packPos)}
	}

	rc, err := DecodeFolderStream(folder, packedStreams, zr.password)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var data []byte
	if len(folder.UnpackSizes) > 0 && folder.UnpackSizes[0] > 0 && folder.UnpackSizes[0] < 1<<31 {
		data = make([]byte, folder.UnpackSizes[0])
		_, err = io.ReadFull(rc, data)
	} else {
		data, err = io.ReadAll(rc)
	}
	if err != nil {
		return nil, err
	}

	zr.folderCacheMu.Lock()
	zr.folderCache[folderIdx] = data
	zr.folderCacheMu.Unlock()

	return data, nil
}

func (zr *Reader) buildEntries() error {
	var entries []*file.File

	folderIdx := 0
	subStreamIdx := 0
	subStreamOffset := int64(0)

	for _, fe := range zr.files {
		f, err := file.New(fe.Name, fe.IsDir)
		if err != nil {
			continue
		}
		f.WithRawName(fe.Name)

		if fe.HasModTime {
			f.WithModTime(fe.ModTime)
		}

		if fe.IsDir || fe.IsEmptyStream {
			f.SetSizes(0, 0, 0)
			openFn := func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(nil)), nil
			}
			f.SetOpenFunc(openFn)
			entries = append(entries, f)
			continue
		}

		curFolder := folderIdx
		curOffset := subStreamOffset

		var fileSize int64
		if curFolder < len(zr.subStreamSizes) && subStreamIdx < len(zr.subStreamSizes[curFolder]) {
			fileSize = int64(zr.subStreamSizes[curFolder][subStreamIdx])
		} else if curFolder < len(zr.folders) && len(zr.folders[curFolder].UnpackSizes) > 0 {
			fileSize = int64(zr.folders[curFolder].UnpackSizes[0])
		}

		f.SetSizes(fileSize, 0, fe.CRC)

		openFn := func() (io.ReadCloser, error) {
			data, err := zr.decompressFolder(curFolder)
			if err != nil {
				return nil, err
			}
			if curOffset >= int64(len(data)) {
				return io.NopCloser(bytes.NewReader(nil)), nil
			}
			limit := int64(len(data)) - curOffset
			if fileSize > 0 && fileSize < limit {
				limit = fileSize
			}
			return io.NopCloser(bytes.NewReader(data[curOffset : curOffset+limit])), nil
		}

		f.SetOpenFunc(openFn)
		entries = append(entries, f)

		subStreamOffset += fileSize
		subStreamIdx++

		if curFolder < len(zr.folders) &&
			(subStreamOffset >= int64(zr.folders[curFolder].UnpackSizes[0]) || (zr.folders[curFolder].NumUnpackStreams > 0 && subStreamIdx >= zr.folders[curFolder].NumUnpackStreams)) {
			folderIdx++
			subStreamIdx = 0
			subStreamOffset = 0
		}
	}

	zr.entries = entries
	return nil
}
