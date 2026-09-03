// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eon

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
)

// CTOCEntry represents a single physical chunk locator record in the CTOC.
type CTOCEntry struct {
	BlockIndex       uint64
	CompressedSize   uint32
	UncompressedSize uint32
}

// ExtentRecord represents an addressable entity in the universal catalog.
type ExtentRecord struct {
	Namespace   uint16
	ChunkIndex  uint32
	ChunkCount  uint16
	ExtentFlags uint16
	Timestamp   int64
	Hash        [32]byte
	Target      string
	Name        string
	ExtraTLVs   []byte
	ChunkOffset uint64
	PayloadSize uint64
}

// Reader provides random-access traversal and extraction for EON container archives.
type Reader struct {
	rAt        io.ReaderAt
	size       int64
	blockShift uint8
	blockSize  int64
	chunks     []CTOCEntry
	extents    []ExtentRecord
	files      []*file.File

	mu         sync.Mutex
	chunkCache map[uint32][]byte
}

// NewReader constructs a new EON container reader from a random-access source.
func NewReader(rAt io.ReaderAt, size int64) (*Reader, error) {
	if rAt == nil || size < HeaderSize+TrailerSize {
		return nil, ErrInvalidTrailer
	}

	// 1. Read fixed 32-byte trailer at [size - 32]
	var trailer [TrailerSize]byte
	if _, err := rAt.ReadAt(trailer[:], size-TrailerSize); err != nil {
		return nil, fmt.Errorf("eon: read trailer: %w", err)
	}
	if binary.LittleEndian.Uint32(trailer[28:32]) != MagicTrailerLE {
		return nil, ErrInvalidTrailer
	}

	centralDirOffset := binary.LittleEndian.Uint64(trailer[0:8])
	archiveStartOffset := binary.LittleEndian.Uint64(trailer[16:24])

	// 2. Read 32-byte Archive Header
	var header [HeaderSize]byte
	if _, err := rAt.ReadAt(header[:], int64(archiveStartOffset)); err != nil {
		return nil, fmt.Errorf("eon: read header: %w", err)
	}
	if binary.LittleEndian.Uint32(header[0:4]) != MagicHeaderLE {
		return nil, ErrInvalidHeader
	}

	blockShift := header[9]
	if blockShift == 0 {
		blockShift = DefaultBlockSizeShift
	}
	blockSize := int64(1 << blockShift)

	// 3. Read Chunk Table of Contents (CTOC)
	var ctocHdr [CTOCHeaderSize]byte
	if _, err := rAt.ReadAt(ctocHdr[:], int64(centralDirOffset)); err != nil {
		return nil, fmt.Errorf("eon: read ctoc header: %w", err)
	}
	if binary.LittleEndian.Uint32(ctocHdr[0:4]) != MagicCTOCLE {
		return nil, ErrInvalidCTOC
	}
	totalChunks := binary.LittleEndian.Uint32(ctocHdr[4:8])

	chunks := make([]CTOCEntry, totalChunks)
	if totalChunks > 0 {
		ctocData := make([]byte, totalChunks*CTOCEntrySize)
		ctocDataOffset := int64(centralDirOffset) + CTOCHeaderSize
		if _, err := rAt.ReadAt(ctocData, ctocDataOffset); err != nil {
			return nil, fmt.Errorf("eon: read ctoc entries: %w", err)
		}
		for i := range totalChunks {
			entry := ctocData[i*CTOCEntrySize : (i+1)*CTOCEntrySize]
			chunks[i] = CTOCEntry{
				BlockIndex:       binary.LittleEndian.Uint64(entry[0:8]),
				CompressedSize:   binary.LittleEndian.Uint32(entry[8:12]),
				UncompressedSize: binary.LittleEndian.Uint32(entry[12:16]),
			}
		}
	}

	// 4. Read Extent Catalog (INDX)
	indxOffset := int64(centralDirOffset) + CTOCHeaderSize + int64(totalChunks)*CTOCEntrySize
	var indxHdr [INDXHeaderSize]byte
	if _, err := rAt.ReadAt(indxHdr[:], indxOffset); err != nil {
		return nil, fmt.Errorf("eon: read indx header: %w", err)
	}
	if binary.LittleEndian.Uint32(indxHdr[0:4]) != MagicINDXLE {
		return nil, ErrInvalidINDX
	}
	totalExtents := binary.LittleEndian.Uint32(indxHdr[4:8])

	catalogRemaining := (size - TrailerSize) - (indxOffset + INDXHeaderSize)
	if catalogRemaining < 0 {
		return nil, ErrCorruptData
	}
	catalogReader := bufio.NewReader(io.NewSectionReader(rAt, indxOffset+INDXHeaderSize, catalogRemaining))

	r := &Reader{
		rAt:        rAt,
		size:       size,
		blockShift: blockShift,
		blockSize:  blockSize,
		chunks:     chunks,
		extents:    make([]ExtentRecord, 0, totalExtents),
		chunkCache: make(map[uint32][]byte),
	}

	for i := range totalExtents {
		var prefix [FixedExtentPrefixSize]byte
		if _, err := io.ReadFull(catalogReader, prefix[:]); err != nil {
			return nil, fmt.Errorf("eon: read extent prefix %d: %w", i, err)
		}

		recLen := binary.LittleEndian.Uint16(prefix[0:2])
		ns := binary.LittleEndian.Uint16(prefix[2:4])
		chunkIdx := binary.LittleEndian.Uint32(prefix[4:8])
		chunkCount := binary.LittleEndian.Uint16(prefix[8:10])
		extFlags := binary.LittleEndian.Uint16(prefix[10:12])
		ts := int64(binary.LittleEndian.Uint64(prefix[12:20]))
		var hash [32]byte
		copy(hash[:], prefix[20:52])
		targetLen := binary.LittleEndian.Uint16(prefix[52:54])
		nameLen := binary.LittleEndian.Uint16(prefix[54:56])
		extraLen := binary.LittleEndian.Uint16(prefix[56:58])

		chunkOffset, err := binary.ReadUvarint(catalogReader)
		if err != nil {
			return nil, fmt.Errorf("eon: read chunk offset varint: %w", err)
		}
		payloadSize, err := binary.ReadUvarint(catalogReader)
		if err != nil {
			return nil, fmt.Errorf("eon: read payload size varint: %w", err)
		}

		targetBytes := make([]byte, targetLen)
		if targetLen > 0 {
			if _, err := io.ReadFull(catalogReader, targetBytes); err != nil {
				return nil, fmt.Errorf("eon: read target string: %w", err)
			}
		}

		nameBytes := make([]byte, nameLen)
		if nameLen > 0 {
			if _, err := io.ReadFull(catalogReader, nameBytes); err != nil {
				return nil, fmt.Errorf("eon: read name string: %w", err)
			}
		}

		extraBytes := make([]byte, extraLen)
		if extraLen > 0 {
			if _, err := io.ReadFull(catalogReader, extraBytes); err != nil {
				return nil, fmt.Errorf("eon: read extra TLVs: %w", err)
			}
		}

		_ = recLen

		rec := ExtentRecord{
			Namespace:   ns,
			ChunkIndex:  chunkIdx,
			ChunkCount:  chunkCount,
			ExtentFlags: extFlags,
			Timestamp:   ts,
			Hash:        hash,
			Target:      string(targetBytes),
			Name:        string(nameBytes),
			ExtraTLVs:   extraBytes,
			ChunkOffset: chunkOffset,
			PayloadSize: payloadSize,
		}
		r.extents = append(r.extents, rec)

		// If this extent is in the POSIX VFS namespace, register it as a file.File
		if ns == NamespaceVFS {
			name := rec.Name
			isDir := strings.HasSuffix(name, "/")
			cleanName := strings.TrimPrefix(name, "/")

			f, err := file.New(cleanName, isDir)
			if err != nil {
				return nil, fmt.Errorf("eon: create file entry %q: %w", cleanName, err)
			}

			f.WithSize(int64(payloadSize))
			f.WithCompressedSize(int64(payloadSize))
			if ts != 0 {
				f.WithModTime(time.Unix(0, ts).UTC())
			}

			// Parse mode from TLVs
			mode := fs.FileMode(0o644)
			if isDir {
				mode = fs.FileMode(0o755) | fs.ModeDir
			}
			for offset := 0; offset+4 <= len(extraBytes); {
				tag := binary.LittleEndian.Uint16(extraBytes[offset : offset+2])
				valLen := int(binary.LittleEndian.Uint16(extraBytes[offset+2 : offset+4]))
				offset += 4
				if offset+valLen > len(extraBytes) {
					break
				}
				val := extraBytes[offset : offset+valLen]
				offset += valLen

				if tag == TagPOSIXPermissions && len(val) >= 4 {
					rawMode := binary.LittleEndian.Uint32(val[0:4])
					mode = fs.FileMode(rawMode & 0o7777)
					if rawMode&0x4000 != 0 {
						mode |= fs.ModeDir
					}
				}
			}
			f.WithMode(mode)

			// Lazy content accessor
			cIdx := chunkIdx
			cOffset := chunkOffset
			pSize := payloadSize
			f.WithOpenFunc(func() (io.ReadCloser, error) {
				if isDir || pSize == 0 {
					return io.NopCloser(bytes.NewReader(nil)), nil
				}
				raw, err := r.decompressChunk(cIdx)
				if err != nil {
					return nil, err
				}
				if cOffset+pSize > uint64(len(raw)) {
					return nil, io.ErrUnexpectedEOF
				}
				slice := raw[cOffset : cOffset+pSize]
				return io.NopCloser(bytes.NewReader(slice)), nil
			})

			r.files = append(r.files, f)
		}
	}

	return r, nil
}

// Entries returns all mapped POSIX VFS files.
func (r *Reader) Entries() []*file.File {
	return r.files
}

// Extents returns all raw universal extents across all namespaces.
func (r *Reader) Extents() []ExtentRecord {
	return r.extents
}

func (r *Reader) decompressChunk(chunkIdx uint32) ([]byte, error) {
	r.mu.Lock()
	if cached, ok := r.chunkCache[chunkIdx]; ok {
		r.mu.Unlock()
		return cached, nil
	}
	r.mu.Unlock()

	if int(chunkIdx) >= len(r.chunks) {
		return nil, errors.New("eon: chunk index out of bounds")
	}
	ctoc := r.chunks[chunkIdx]

	diskOffset := int64(ctoc.BlockIndex) << r.blockShift

	// Read 64-byte Chunk Header
	var chnkHdr [ChunkHeaderSize]byte
	if _, err := r.rAt.ReadAt(chnkHdr[:], diskOffset); err != nil {
		return nil, fmt.Errorf("eon: read chunk header: %w", err)
	}
	if binary.LittleEndian.Uint32(chnkHdr[0:4]) != MagicChunkLE {
		return nil, ErrInvalidChunk
	}

	// Read compressed payload
	compressedBytes := make([]byte, ctoc.CompressedSize)
	if _, err := r.rAt.ReadAt(compressedBytes, diskOffset+ChunkHeaderSize); err != nil {
		return nil, fmt.Errorf("eon: read compressed chunk payload: %w", err)
	}

	// Decompress via ZSTD
	codecID := chnkHdr[8]
	var raw []byte
	var err error

	switch codecID {
	case CodecStore:
		raw = compressedBytes
	case CodecZSTD:
		raw, err = codec.Unzstd(compressedBytes, nil)
		if err != nil {
			return nil, fmt.Errorf("eon: decompress zstd chunk: %w", err)
		}
	default:
		return nil, fmt.Errorf("eon: unsupported chunk codec ID %d", codecID)
	}

	r.mu.Lock()
	r.chunkCache[chunkIdx] = raw
	r.mu.Unlock()

	return raw, nil
}
