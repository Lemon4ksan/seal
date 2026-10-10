// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eon

import (
	"bufio"
	"bytes"
	"container/list"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/przm"
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

const (
	// DefaultMaxCachedChunks defines the maximum number of decompressed chunks
	// retained in memory (default 4 chunks * 32 MiB = 128 MiB max footprint).
	DefaultMaxCachedChunks = 4
)

type chunkCacheEntry struct {
	chunkIdx uint32
	data     []byte
}

type chunkFlight struct {
	wg  sync.WaitGroup
	raw []byte
	err error
}

// Reader provides random-access traversal and extraction for EON container archives.
type Reader struct {
	rAt        io.ReaderAt
	size       int64
	baseOffset int64
	blockShift uint8
	blockSize  int64
	chunks     []CTOCEntry
	extents    []ExtentRecord
	files      []*file.File

	mu              sync.Mutex
	maxCachedChunks int
	chunkCache      map[uint32]*list.Element
	lruList         *list.List
	inFlight        map[uint32]*chunkFlight
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
	baseOffset := int64(archiveStartOffset)

	// If archiveStartOffset == 0, check if offset 0 has MagicHeaderLE
	if baseOffset == 0 {
		var magicBuf [4]byte
		if _, err := rAt.ReadAt(magicBuf[:], 0); err == nil && binary.LittleEndian.Uint32(magicBuf[:]) != MagicHeaderLE {
			// Auto-detect SFX/SMX stub offset by checking block boundaries
			for off := int64(512); off < size-TrailerSize; off += 512 {
				if _, err := rAt.ReadAt(magicBuf[:], off); err == nil && binary.LittleEndian.Uint32(magicBuf[:]) == MagicHeaderLE {
					baseOffset = off
					break
				}
			}
		}
	}

	// 2. Read 32-byte Archive Header
	var header [HeaderSize]byte
	if _, err := rAt.ReadAt(header[:], baseOffset); err != nil {
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
	ctocOffset := baseOffset + int64(centralDirOffset)
	if _, err := rAt.ReadAt(ctocHdr[:], ctocOffset); err != nil {
		return nil, fmt.Errorf("eon: read ctoc header: %w", err)
	}
	if binary.LittleEndian.Uint32(ctocHdr[0:4]) != MagicCTOCLE {
		return nil, ErrInvalidCTOC
	}
	totalChunks := binary.LittleEndian.Uint32(ctocHdr[4:8])

	chunks := make([]CTOCEntry, totalChunks)
	if totalChunks > 0 {
		ctocData := make([]byte, totalChunks*CTOCEntrySize)
		ctocDataOffset := ctocOffset + CTOCHeaderSize
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
	indxOffset := ctocOffset + CTOCHeaderSize + int64(totalChunks)*CTOCEntrySize
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
		rAt:             rAt,
		size:            size,
		baseOffset:      baseOffset,
		blockShift:      blockShift,
		blockSize:       blockSize,
		chunks:          chunks,
		extents:         make([]ExtentRecord, 0, totalExtents),
		maxCachedChunks: DefaultMaxCachedChunks,
		chunkCache:      make(map[uint32]*list.Element),
		lruList:         list.New(),
		inFlight:        make(map[uint32]*chunkFlight),
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
			cCount := chunkCount
			cOffset := chunkOffset
			pSize := payloadSize
			f.WithOpenFunc(func() (io.ReadCloser, error) {
				if isDir || pSize == 0 {
					return io.NopCloser(bytes.NewReader(nil)), nil
				}
				if cCount <= 1 {
					raw, err := r.decompressChunk(cIdx)
					if err != nil {
						return nil, err
					}
					if cOffset+pSize > uint64(len(raw)) {
						return nil, io.ErrUnexpectedEOF
					}
					slice := raw[cOffset : cOffset+pSize]
					return io.NopCloser(bytes.NewReader(slice)), nil
				}
				return &multiChunkReader{
					r:           r,
					startChunk:  cIdx,
					numChunks:   int(cCount),
					startOffset: cOffset,
					totalSize:   pSize,
				}, nil
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

// Files aliases Entries returning all mapped POSIX VFS files.
func (r *Reader) Files() []*file.File {
	return r.files
}

// Extents returns all raw universal extents across all namespaces.
func (r *Reader) Extents() []ExtentRecord {
	return r.extents
}

// Chunks returns the CTOC chunk records.
func (r *Reader) Chunks() []CTOCEntry {
	return r.chunks
}

// BlockSize returns the logical sector block size in bytes.
func (r *Reader) BlockSize() int64 {
	return r.blockSize
}

// SetMaxCachedChunks adjusts the maximum number of 32 MiB chunks held in LRU memory.
func (r *Reader) SetMaxCachedChunks(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 {
		n = 1
	}
	r.maxCachedChunks = n
	for r.lruList.Len() > r.maxCachedChunks {
		oldest := r.lruList.Back()
		r.lruList.Remove(oldest)
		delete(r.chunkCache, oldest.Value.(*chunkCacheEntry).chunkIdx)
	}
}

// Close releases cached chunk memory.
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.chunkCache)
	r.lruList.Init()
	return nil
}

func (r *Reader) decompressChunk(chunkIdx uint32) ([]byte, error) {
	r.mu.Lock()
	// 1. Check LRU cache
	if elem, ok := r.chunkCache[chunkIdx]; ok {
		r.lruList.MoveToFront(elem)
		data := elem.Value.(*chunkCacheEntry).data
		r.mu.Unlock()
		return data, nil
	}

	// 2. Single flight deduplication: if this chunk is already being decompressed
	// by another worker, wait for its completion instead of duplicating work.
	if flight, ok := r.inFlight[chunkIdx]; ok {
		r.mu.Unlock()
		flight.wg.Wait()
		return flight.raw, flight.err
	}

	flight := &chunkFlight{}
	flight.wg.Add(1)
	r.inFlight[chunkIdx] = flight
	r.mu.Unlock()

	// 3. Decompress chunk outside lock
	raw, err := r.loadChunk(chunkIdx)

	// 4. Update cache and broadcast to waiters under lock
	r.mu.Lock()
	flight.raw = raw
	flight.err = err
	delete(r.inFlight, chunkIdx)

	if err == nil {
		if elem, ok := r.chunkCache[chunkIdx]; ok {
			r.lruList.MoveToFront(elem)
			elem.Value.(*chunkCacheEntry).data = raw
		} else {
			for r.lruList.Len() >= r.maxCachedChunks && r.lruList.Len() > 0 {
				oldest := r.lruList.Back()
				r.lruList.Remove(oldest)
				delete(r.chunkCache, oldest.Value.(*chunkCacheEntry).chunkIdx)
			}
			elem := r.lruList.PushFront(&chunkCacheEntry{
				chunkIdx: chunkIdx,
				data:     raw,
			})
			r.chunkCache[chunkIdx] = elem
		}
	}
	flight.wg.Done()
	r.mu.Unlock()

	return raw, err
}

func (r *Reader) loadChunk(chunkIdx uint32) ([]byte, error) {
	if int(chunkIdx) >= len(r.chunks) {
		return nil, errors.New("eon: chunk index out of bounds")
	}
	ctoc := r.chunks[chunkIdx]

	diskOffset := r.baseOffset + (int64(ctoc.BlockIndex) << r.blockShift)

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

	// Decompress via Codec
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
	case CodecPRZM:
		raw, err = przm.Decompress(compressedBytes)
		if err != nil {
			return nil, fmt.Errorf("eon: decompress przm chunk: %w", err)
		}
	default:
		return nil, fmt.Errorf("eon: unsupported chunk codec ID %d", codecID)
	}

	return raw, nil
}

type multiChunkReader struct {
	r           *Reader
	startChunk  uint32
	numChunks   int
	startOffset uint64
	totalSize   uint64
	curChunkRel int
	curSlice    []byte
	bytesRead   uint64
}

func (m *multiChunkReader) Read(p []byte) (int, error) {
	if m.bytesRead >= m.totalSize {
		return 0, io.EOF
	}
	for len(m.curSlice) == 0 {
		if m.curChunkRel >= m.numChunks {
			return 0, io.EOF
		}
		raw, err := m.r.decompressChunk(m.startChunk + uint32(m.curChunkRel))
		if err != nil {
			return 0, err
		}
		if m.curChunkRel == 0 {
			if m.startOffset > uint64(len(raw)) {
				return 0, io.ErrUnexpectedEOF
			}
			m.curSlice = raw[m.startOffset:]
		} else {
			m.curSlice = raw
		}
		rem := m.totalSize - m.bytesRead
		if uint64(len(m.curSlice)) > rem {
			m.curSlice = m.curSlice[:rem]
		}
		m.curChunkRel++
	}

	n := copy(p, m.curSlice)
	m.curSlice = m.curSlice[n:]
	m.bytesRead += uint64(n)
	return n, nil
}

func (m *multiChunkReader) Close() error {
	return nil
}
