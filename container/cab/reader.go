// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cab

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/lemon4ksan/seal/codec/compress/lzx"
	"github.com/lemon4ksan/seal/file"
)

// Reader provides random-access catalog reading and extraction for Microsoft Cabinet (.cab) files.
type Reader struct {
	rAt       io.ReaderAt
	size      int64
	header    Header
	folders   []Folder
	files     []FileEntry
	entries   []*file.File
	folderBuf map[int][]byte
	mu        sync.Mutex
}

// NewReader opens a Cabinet archive from an io.ReaderAt and parses its file catalog.
func NewReader(rAt io.ReaderAt, size int64) (*Reader, error) {
	if rAt == nil || size < 32 {
		return nil, ErrInvalidSignature
	}

	r := &Reader{
		rAt:       rAt,
		size:      size,
		folderBuf: make(map[int][]byte),
	}

	if err := r.readHeader(); err != nil {
		return nil, err
	}

	if err := r.buildEntries(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Reader) readHeader() error {
	sr := io.NewSectionReader(r.rAt, 0, r.size)

	var hdr [36]byte
	if _, err := io.ReadFull(sr, hdr[:]); err != nil {
		return ErrInvalidSignature
	}

	sig := binary.LittleEndian.Uint32(hdr[0:4])
	if sig != MagicSignature {
		return ErrInvalidSignature
	}

	h := &r.header
	h.Signature = sig
	h.Reserved1 = binary.LittleEndian.Uint32(hdr[4:8])
	h.CbCabinet = binary.LittleEndian.Uint32(hdr[8:12])
	h.Reserved2 = binary.LittleEndian.Uint32(hdr[12:16])
	h.CoffFiles = binary.LittleEndian.Uint32(hdr[16:20])
	h.Reserved3 = binary.LittleEndian.Uint32(hdr[20:24])
	h.VersionMinor = hdr[24]
	h.VersionMajor = hdr[25]
	h.CFolders = binary.LittleEndian.Uint16(hdr[26:28])
	h.CFiles = binary.LittleEndian.Uint16(hdr[28:30])
	h.Flags = binary.LittleEndian.Uint16(hdr[30:32])
	h.SetID = binary.LittleEndian.Uint16(hdr[32:34])
	h.ICabinet = binary.LittleEndian.Uint16(hdr[34:36])

	// Optional reserve sizes
	if h.Flags&FlagReservePresent != 0 {
		var resHdr [4]byte
		if _, err := io.ReadFull(sr, resHdr[:]); err != nil {
			return ErrCorruptedCabinet
		}
		h.CbCFHeader = binary.LittleEndian.Uint16(resHdr[0:2])
		h.CbCFFolder = resHdr[2]
		h.CbCFData = resHdr[3]

		if h.CbCFHeader > 0 {
			h.AbReserve = make([]byte, h.CbCFHeader)
			if _, err := io.ReadFull(sr, h.AbReserve); err != nil {
				return ErrCorruptedCabinet
			}
		}
	}

	// Optional previous / next cabinet strings
	if h.Flags&FlagPrevCabinet != 0 {
		prevCab, err := readNullTerminatedString(sr)
		if err != nil {
			return err
		}
		prevDisk, err := readNullTerminatedString(sr)
		if err != nil {
			return err
		}
		h.SzCabinetPrev = prevCab
		h.SzDiskPrev = prevDisk
	}

	if h.Flags&FlagNextCabinet != 0 {
		nextCab, err := readNullTerminatedString(sr)
		if err != nil {
			return err
		}
		nextDisk, err := readNullTerminatedString(sr)
		if err != nil {
			return err
		}
		h.SzCabinetNext = nextCab
		h.SzDiskNext = nextDisk
	}

	// Read CFFOLDER entries
	r.folders = make([]Folder, h.CFolders)
	for i := 0; i < int(h.CFolders); i++ {
		var fHdr [8]byte
		if _, err := io.ReadFull(sr, fHdr[:]); err != nil {
			return ErrCorruptedCabinet
		}
		f := &r.folders[i]
		f.CoffCabStart = binary.LittleEndian.Uint32(fHdr[0:4])
		f.CCFData = binary.LittleEndian.Uint16(fHdr[4:6])
		f.TypeCompress = binary.LittleEndian.Uint16(fHdr[6:8])

		if h.CbCFFolder > 0 {
			f.AbReserve = make([]byte, h.CbCFFolder)
			if _, err := io.ReadFull(sr, f.AbReserve); err != nil {
				return ErrCorruptedCabinet
			}
		}
	}

	// Seek to first CFFILE entry
	if _, err := sr.Seek(int64(h.CoffFiles), io.SeekStart); err != nil {
		return ErrCorruptedCabinet
	}

	// Read CFFILE entries
	r.files = make([]FileEntry, h.CFiles)
	for i := 0; i < int(h.CFiles); i++ {
		var feHdr [16]byte
		if _, err := io.ReadFull(sr, feHdr[:]); err != nil {
			return ErrCorruptedCabinet
		}
		fe := &r.files[i]
		fe.CbFile = binary.LittleEndian.Uint32(feHdr[0:4])
		fe.UoffFolderStart = binary.LittleEndian.Uint32(feHdr[4:8])
		fe.IFolder = binary.LittleEndian.Uint16(feHdr[8:10])
		fe.Date = binary.LittleEndian.Uint16(feHdr[10:12])
		fe.Time = binary.LittleEndian.Uint16(feHdr[12:14])
		fe.Attribs = binary.LittleEndian.Uint16(feHdr[14:16])

		name, err := readNullTerminatedString(sr)
		if err != nil {
			return err
		}
		fe.Name = strings.ReplaceAll(name, "\\", "/")
	}

	return nil
}

func (r *Reader) buildEntries() error {
	r.entries = make([]*file.File, 0, len(r.files))

	for i := range r.files {
		fe := &r.files[i]
		cleanName := strings.TrimPrefix(fe.Name, "/")
		isDir := fe.Attribs&AttrDirectory != 0
		if isDir && !strings.HasSuffix(cleanName, "/") {
			cleanName += "/"
		}

		f, err := file.New(cleanName, isDir)
		if err != nil {
			return fmt.Errorf("cab: create file entry %q: %w", cleanName, err)
		}

		f.SetSizes(int64(fe.CbFile), 0, 0)
		modTime := fe.ModTime()
		if !modTime.IsZero() {
			f.WithModTime(modTime)
		}

		if !isDir && fe.CbFile > 0 {
			folderIdx := int(fe.IFolder)
			uoff := int64(fe.UoffFolderStart)
			cb := int64(fe.CbFile)

			f.WithOpenFunc(func() (io.ReadCloser, error) {
				decompressed, err := r.decompressFolder(folderIdx)
				if err != nil {
					return nil, err
				}
				if uoff+cb > int64(len(decompressed)) {
					return nil, ErrCorruptedBlock
				}
				return io.NopCloser(bytes.NewReader(decompressed[uoff : uoff+cb])), nil
			})
		}

		r.entries = append(r.entries, f)
	}

	return nil
}

// decompressFolder extracts the uncompressed bytes for the specified CFFOLDER index, caching results.
func (r *Reader) decompressFolder(folderIdx int) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cached, ok := r.folderBuf[folderIdx]; ok {
		return cached, nil
	}

	if folderIdx < 0 || folderIdx >= len(r.folders) {
		return nil, ErrCorruptedCabinet
	}

	f := &r.folders[folderIdx]
	offset := int64(f.CoffCabStart)
	sr := io.NewSectionReader(r.rAt, offset, r.size-offset)

	method := f.CompressionMethod()
	var out bytes.Buffer

	switch method {
	case CompTypeNone:
		for i := 0; i < int(f.CCFData); i++ {
			block, err := r.readDataBlock(sr)
			if err != nil {
				return nil, err
			}
			out.Write(block.Data)
		}

	case CompTypeMSZip:
		var flateR io.ReadCloser
		var dict []byte

		for i := 0; i < int(f.CCFData); i++ {
			block, err := r.readDataBlock(sr)
			if err != nil {
				return nil, err
			}
			if len(block.Data) < 2 || block.Data[0] != 'C' || block.Data[1] != 'K' {
				return nil, fmt.Errorf("%w: missing MSZIP CK signature", ErrCorruptedBlock)
			}

			payload := block.Data[2:]
			if flateR == nil {
				flateR = flate.NewReader(bytes.NewReader(payload))
			} else {
				if resetter, ok := flateR.(flate.Resetter); ok {
					_ = resetter.Reset(bytes.NewReader(payload), dict)
				} else {
					_ = flateR.Close()
					flateR = flate.NewReaderDict(bytes.NewReader(payload), dict)
				}
			}

			blockUncomp := make([]byte, block.CbUncomp)
			if _, err := io.ReadFull(flateR, blockUncomp); err != nil && !errors.Is(err, io.EOF) {
				_ = flateR.Close()
				return nil, fmt.Errorf("cab: mszip decode block %d: %w", i, err)
			}

			out.Write(blockUncomp)

			// Keep trailing 32KB as dictionary for next block
			if len(blockUncomp) >= 32768 {
				dict = blockUncomp[len(blockUncomp)-32768:]
			} else {
				all := out.Bytes()
				if len(all) > 32768 {
					dict = all[len(all)-32768:]
				} else {
					dict = all
				}
			}
		}
		if flateR != nil {
			_ = flateR.Close()
		}

	case CompTypeLZX:
		windowBits := f.LZXWindowBits()
		dec := lzx.NewDecoder(windowBits)

		for i := 0; i < int(f.CCFData); i++ {
			block, err := r.readDataBlock(sr)
			if err != nil {
				return nil, err
			}
			blockUncomp := make([]byte, block.CbUncomp)
			if _, err := dec.Decompress(blockUncomp, block.Data); err != nil {
				return nil, fmt.Errorf("cab: lzx decode block %d: %w", i, err)
			}
			out.Write(blockUncomp)
		}

	default:
		return nil, fmt.Errorf("%w: 0x%04X", ErrUnsupportedComp, f.TypeCompress)
	}

	result := out.Bytes()
	r.folderBuf[folderIdx] = result
	return result, nil
}

type loadedBlock struct {
	CbData   uint16
	CbUncomp uint16
	Data     []byte
}

func (r *Reader) readDataBlock(sr io.Reader) (*loadedBlock, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(sr, hdr[:]); err != nil {
		return nil, ErrCorruptedBlock
	}

	cbData := binary.LittleEndian.Uint16(hdr[4:6])
	cbUncomp := binary.LittleEndian.Uint16(hdr[6:8])

	if r.header.CbCFData > 0 {
		res := make([]byte, r.header.CbCFData)
		if _, err := io.ReadFull(sr, res); err != nil {
			return nil, ErrCorruptedBlock
		}
	}

	payload := make([]byte, cbData)
	if _, err := io.ReadFull(sr, payload); err != nil {
		return nil, ErrCorruptedBlock
	}

	return &loadedBlock{
		CbData:   cbData,
		CbUncomp: cbUncomp,
		Data:     payload,
	}, nil
}

// Entries returns all parsed file entries in the Cabinet archive.
func (r *Reader) Entries() []*file.File {
	return r.entries
}

// Header returns the cabinet file header metadata.
func (r *Reader) Header() Header {
	return r.header
}

// Folders returns the folder metadata structures.
func (r *Reader) Folders() []Folder {
	return r.folders
}

func readNullTerminatedString(r io.Reader) (string, error) {
	var buf strings.Builder
	var b [1]byte
	for {
		if _, err := r.Read(b[:]); err != nil {
			return "", err
		}
		if b[0] == 0 {
			break
		}
		buf.WriteByte(b[0])
		if buf.Len() > 4096 {
			return "", errors.New("cab: string exceeded maximum length")
		}
	}
	return buf.String(), nil
}
