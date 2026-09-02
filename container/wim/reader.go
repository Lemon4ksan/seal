// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wim

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/lemon4ksan/seal/codec/lzx"
	"github.com/lemon4ksan/seal/codec/xpress"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/internal/sys"
)

var (
	// ErrInvalidMagic indicates the file is not a valid WIM archive.
	ErrInvalidMagic = errors.New("wim: invalid magic signature")
	// ErrNoMetadata indicates no metadata resource was found for the image.
	ErrNoMetadata = errors.New("wim: metadata resource not found")
)

// Reader provides random-access inspection and extraction of WIM archives.
type Reader struct {
	rAt         io.ReaderAt
	size        int64
	Header      Header
	LookupTable map[[20]byte]*LookupEntry
	Entries     []*file.File
	XMLData     string
}

// NewReader parses a WIM catalog from rAt.
func NewReader(rAt io.ReaderAt, size int64) (*Reader, error) {
	if rAt == nil || size < HeaderSize {
		return nil, errors.New("wim: invalid reader or size too small")
	}

	var header Header
	hdrBuf := make([]byte, HeaderSize)
	if _, err := rAt.ReadAt(hdrBuf, 0); err != nil {
		return nil, fmt.Errorf("wim: reading header: %w", err)
	}
	if err := header.Read(bytes.NewReader(hdrBuf)); err != nil {
		return nil, fmt.Errorf("wim: decoding header: %w", err)
	}

	if string(header.Magic[:]) != Magic {
		return nil, ErrInvalidMagic
	}

	r := &Reader{
		rAt:         rAt,
		size:        size,
		Header:      header,
		LookupTable: make(map[[20]byte]*LookupEntry),
	}

	// 1. Read and parse Lookup Table
	if err := r.readLookupTable(); err != nil {
		return nil, fmt.Errorf("wim: reading lookup table: %w", err)
	}

	// 2. Read XML metadata if present
	r.readXML()

	// 3. Read Metadata resource and construct file tree
	if err := r.readMetadata(); err != nil {
		return nil, fmt.Errorf("wim: reading image metadata: %w", err)
	}

	return r, nil
}

func (r *Reader) readLookupTable() error {
	tblSize := r.Header.LookupTable.OriginalSize
	if tblSize == 0 {
		return nil
	}

	data, err := r.readResource(&r.Header.LookupTable, r.Header.ChunkSize)
	if err != nil {
		return err
	}

	count := len(data) / LookupEntrySize
	reader := bytes.NewReader(data)

	for i := 0; i < count; i++ {
		entry := new(LookupEntry)
		if err := entry.Read(reader); err != nil {
			return err
		}
		r.LookupTable[entry.SHA1] = entry
	}

	return nil
}

func (r *Reader) readXML() {
	if r.Header.XMLData.OriginalSize == 0 {
		return
	}
	data, err := r.readResource(&r.Header.XMLData, r.Header.ChunkSize)
	if err != nil || len(data) < 2 {
		return
	}

	// Check for UTF-16LE BOM
	if data[0] == 0xFF && data[1] == 0xFE {
		r.XMLData = DecodeUTF16LE(data[2:])
	} else {
		r.XMLData = string(data)
	}
}

func (r *Reader) readMetadata() error {
	// Find metadata entry for image index 1
	var metaEntry *LookupEntry
	for _, entry := range r.LookupTable {
		if entry.ResHdr.Flags()&ResFlagMetadata != 0 {
			metaEntry = entry
			break
		}
	}

	if metaEntry == nil {
		return ErrNoMetadata
	}

	data, err := r.readResource(&metaEntry.ResHdr, r.Header.ChunkSize)
	if err != nil {
		return err
	}

	if len(data) < 8 {
		return errors.New("wim: metadata resource too small")
	}

	secTotal := binary.LittleEndian.Uint32(data[0:4])
	if int(secTotal) > len(data) {
		secTotal = 8
	}

	rootOffset := uint64(secTotal)
	return r.parseDentryTree(data, rootOffset, "")
}

func (r *Reader) parseDentryTree(data []byte, dirOffset uint64, parentPath string) error {
	currOffset := dirOffset

	for currOffset+8 <= uint64(len(data)) {
		length := binary.LittleEndian.Uint64(data[currOffset : currOffset+8])
		if length == 0 {
			// 8-byte zero entry marks the end of this directory's children
			break
		}
		if currOffset+length > uint64(len(data)) {
			break
		}

		entryBuf := data[currOffset : currOffset+length]
		if len(entryBuf) < DentryFixedSize {
			break
		}

		attributes := binary.LittleEndian.Uint32(entryBuf[8:12])
		subdirOffset := binary.LittleEndian.Uint64(entryBuf[16:24])
		creationTime := binary.LittleEndian.Uint64(entryBuf[40:48])
		lastAccessTime := binary.LittleEndian.Uint64(entryBuf[48:56])
		lastWriteTime := binary.LittleEndian.Uint64(entryBuf[56:64])

		var sha1 [20]byte
		copy(sha1[:], entryBuf[64:84])

		fileNameLen := binary.LittleEndian.Uint16(entryBuf[100:102])
		var fileName string
		if fileNameLen > 0 && int(DentryFixedSize+fileNameLen) <= len(entryBuf) {
			fileName = DecodeUTF16LE(entryBuf[DentryFixedSize : DentryFixedSize+fileNameLen])
		}

		isDir := (attributes & AttributeDirectory) != 0
		fullPath := fileName
		if parentPath != "" && fileName != "" {
			fullPath = path.Join(parentPath, fileName)
		} else if parentPath != "" && fileName == "" {
			fullPath = parentPath
		}

		if fullPath != "" {
			f, err := file.New(fullPath, isDir)
			if err != nil {
				continue
			}
			f.WithModTime(FiletimeToTime(lastWriteTime))
			f.WithMetadata(sys.Metadata{
				LastAccessTime: lastAccessTime,
				LastWriteTime:  lastWriteTime,
				CreationTime:   creationTime,
			})

			if isDir {
				f.WithMode(0o755 | 0o040000)
			} else {
				f.WithMode(0o644 | 0o100000)
				lookup := r.LookupTable[sha1]
				if lookup != nil {
					fileSize := int64(lookup.ResHdr.OriginalSize)
					f.WithSize(fileSize)
					entryCopy := *lookup
					f.WithOpenFunc(func() (io.ReadCloser, error) {
						data, err := r.readResource(&entryCopy.ResHdr, r.Header.ChunkSize)
						if err != nil {
							return nil, err
						}
						return io.NopCloser(bytes.NewReader(data)), nil
					})
				} else {
					f.WithSize(0)
					f.WithOpenFunc(func() (io.ReadCloser, error) {
						return io.NopCloser(bytes.NewReader(nil)), nil
					})
				}
			}

			r.Entries = append(r.Entries, f)
		}

		// Recurse into children if this is a non-empty directory
		if isDir && subdirOffset > 0 && subdirOffset < uint64(len(data)) {
			_ = r.parseDentryTree(data, subdirOffset, fullPath)
		}

		currOffset += length
	}

	return nil
}

func (r *Reader) readResource(reshdr *ResHdr, chunkSize uint32) ([]byte, error) {
	origSize := reshdr.OriginalSize
	compSize := reshdr.CompressedSize()
	offset := int64(reshdr.Offset)
	flags := reshdr.Flags()

	if (flags&ResFlagCompressed == 0) || compSize == origSize {
		buf := make([]byte, origSize)
		_, err := r.rAt.ReadAt(buf, offset)
		return buf, err
	}

	if chunkSize == 0 {
		chunkSize = DefaultChunkSize
	}

	numChunks := (origSize + uint64(chunkSize) - 1) / uint64(chunkSize)
	if numChunks == 0 {
		return nil, nil
	}

	use64Bit := origSize > 0xFFFFFFFF
	entrySize := 4
	if use64Bit {
		entrySize = 8
	}
	tableSize := int(numChunks-1) * entrySize

	chunkOffsets := make([]uint64, numChunks+1)
	chunkOffsets[0] = 0

	if tableSize > 0 {
		tableBuf := make([]byte, tableSize)
		if _, err := r.rAt.ReadAt(tableBuf, offset); err != nil {
			return nil, err
		}
		for i := uint64(0); i < numChunks-1; i++ {
			if use64Bit {
				chunkOffsets[i+1] = binary.LittleEndian.Uint64(tableBuf[i*8:])
			} else {
				chunkOffsets[i+1] = uint64(binary.LittleEndian.Uint32(tableBuf[i*4:]))
			}
		}
	}
	chunkOffsets[numChunks] = compSize - uint64(tableSize)

	dataOffset := offset + int64(tableSize)
	out := make([]byte, origSize)
	outOffset := uint64(0)

	for i := uint64(0); i < numChunks; i++ {
		cOff := chunkOffsets[i]
		cEnd := chunkOffsets[i+1]
		cLen := cEnd - cOff

		remaining := origSize - outOffset
		thisChunk := uint64(chunkSize)
		if remaining < thisChunk {
			thisChunk = remaining
		}

		cBuf := make([]byte, cLen)
		if _, err := r.rAt.ReadAt(cBuf, dataOffset+int64(cOff)); err != nil {
			return nil, err
		}

		if cLen == thisChunk {
			copy(out[outOffset:], cBuf)
		} else {
			decompTarget := out[outOffset : outOffset+thisChunk]
			if r.Header.Flags&FlagHeaderCompressLZX != 0 {
				_, err := lzx.Decompress(decompTarget, cBuf)
				if err != nil {
					return nil, fmt.Errorf("wim: decompressing LZX chunk %d: %w", i, err)
				}
			} else {
				_, err := xpress.Decompress(decompTarget, cBuf)
				if err != nil {
					return nil, fmt.Errorf("wim: decompressing XPRESS chunk %d: %w", i, err)
				}
			}
		}
		outOffset += thisChunk
	}

	return out, nil
}
