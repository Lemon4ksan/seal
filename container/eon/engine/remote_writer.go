// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/chunk"
	"github.com/lemon4ksan/seal/container/eon/objstore"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// DecomposeStats tracks uploaded and reused chunks and bytes during archive decomposition.
type DecomposeStats struct {
	TotalChunks    int
	UploadedChunks int
	ReusedChunks   int
	UploadedBytes  uint64
	ReusedBytes    uint64
	IndexBytes     int64
}

// DecomposeArchive decomposes a local monolithic .eon archive into an object store,
// uploading content-addressed chunks to prefix/chunks/<hash> and writing detached index.eon.
func DecomposeArchive(archivePath string, store objstore.Store, prefix string) (*DecomposeStats, error) {
	r, closer, err := OpenFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open source archive: %w", err)
	}
	defer closer.Close()

	ctx := context.Background()
	stats := &DecomposeStats{
		TotalChunks: len(r.ctoc.Entries),
	}

	chsh := catalog.NewChunkHashTable()

	// 1. Process each chunk
	for i, entry := range r.ctoc.Entries {
		chunkOffset := int64(entry.BlockIndex) << r.blockShift
		unpaddedLen := spec.ChunkHeaderSize + int64(entry.CompressedSize)

		rawChunk := make([]byte, unpaddedLen)
		if _, err := r.rAt.ReadAt(rawChunk, chunkOffset); err != nil {
			return nil, fmt.Errorf("read chunk %d: %w", i, err)
		}

		c, err := chunk.ParseHeader(bytes.NewReader(rawChunk[:spec.ChunkHeaderSize]))
		if err != nil {
			return nil, fmt.Errorf("parse chunk header %d: %w", i, err)
		}

		chsh.Add(c.Hash)
		chunkKey := spec.ChunkObjectKey(prefix, c.Hash)

		exists, err := store.Exists(ctx, chunkKey)
		if err == nil && exists {
			stats.ReusedChunks++
			stats.ReusedBytes += uint64(len(rawChunk))
			continue
		}

		if err := store.Put(ctx, chunkKey, bytes.NewReader(rawChunk), int64(len(rawChunk))); err != nil {
			return nil, fmt.Errorf("upload chunk %d (%s): %w", i, chunkKey, err)
		}

		stats.UploadedChunks++
		stats.UploadedBytes += uint64(len(rawChunk))
	}

	// 2. Build index.eon
	var indexBuf bytes.Buffer

	// 32-byte Archive Header
	var hdr [spec.HeaderSize]byte
	copy(hdr[0:4], spec.MagicHeader[:])
	hdr[4] = spec.VersionMajor
	hdr[5] = spec.VersionMinor
	hdr[6] = r.profile
	hdr[7] = 0
	hdr[8] = 0
	hdr[9] = r.blockShift
	flags := r.headerFlags | spec.FlagCentralDir | spec.FlagDetachedChunks
	binary.LittleEndian.PutUint16(hdr[10:12], flags)
	indexBuf.Write(hdr[:])

	centralDirOffset := int64(indexBuf.Len())

	// Write CTOC
	if _, err := r.ctoc.Write(&indexBuf); err != nil {
		return nil, fmt.Errorf("write ctoc: %w", err)
	}

	// Write INDX
	if _, err := r.indx.Write(&indexBuf); err != nil {
		return nil, fmt.Errorf("write indx: %w", err)
	}

	// Write TREE & SNPS if present
	if r.snapshots != nil && r.treeStore != nil {
		if _, err := r.snapshots.Write(&indexBuf); err != nil {
			return nil, fmt.Errorf("write snapshots: %w", err)
		}
		if _, err := writeTreeNodes(&indexBuf, r.treeStore); err != nil {
			return nil, fmt.Errorf("write tree nodes: %w", err)
		}
	}

	// Write CHSH
	if _, err := chsh.Write(&indexBuf); err != nil {
		return nil, fmt.Errorf("write chsh: %w", err)
	}

	// Write DSIG if present
	if r.dsig != nil {
		if _, err := r.dsig.Write(&indexBuf); err != nil {
			return nil, fmt.Errorf("write dsig: %w", err)
		}
	}

	// 32-byte Trailer
	var trailer [spec.TrailerSize]byte
	binary.LittleEndian.PutUint64(trailer[0:8], uint64(centralDirOffset))
	binary.LittleEndian.PutUint64(trailer[8:16], 0)  // RecoveryOffset (0 = absent)
	binary.LittleEndian.PutUint64(trailer[16:24], 0) // ArchiveStartOffset (0 = no stub)
	binary.LittleEndian.PutUint32(trailer[24:28], uint32(flags))
	copy(trailer[28:32], spec.MagicTrailer[:])
	indexBuf.Write(trailer[:])

	// Upload index.eon
	indexKey := "index.eon"
	if prefix != "" {
		indexKey = strings.TrimSuffix(prefix, "/") + "/index.eon"
	}

	indexBytes := indexBuf.Bytes()
	if err := store.Put(ctx, indexKey, bytes.NewReader(indexBytes), int64(len(indexBytes))); err != nil {
		return nil, fmt.Errorf("upload index.eon: %w", err)
	}
	stats.IndexBytes = int64(len(indexBytes))

	return stats, nil
}

// ComposeArchive reconstructs a monolithic .eon archive from remote detached index.eon and chunks.
func ComposeArchive(store objstore.Store, prefix string, w io.Writer) error {
	r, err := OpenStore(store, prefix)
	if err != nil {
		return fmt.Errorf("open remote store: %w", err)
	}

	blockSize := int64(1 << r.blockShift)
	var totalWritten int64

	// Write 32-byte header with FlagDetachedChunks cleared
	var hdr [spec.HeaderSize]byte
	copy(hdr[0:4], spec.MagicHeader[:])
	hdr[4] = spec.VersionMajor
	hdr[5] = spec.VersionMinor
	hdr[6] = r.profile
	hdr[7] = 0
	hdr[8] = 0
	hdr[9] = r.blockShift
	flags := (r.headerFlags &^ spec.FlagDetachedChunks) | spec.FlagCentralDir
	binary.LittleEndian.PutUint16(hdr[10:12], flags)
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	totalWritten += spec.HeaderSize

	// Pad header to logical block boundary so Chunk 0 begins block-aligned
	rem := totalWritten % blockSize
	if rem != 0 {
		pad := blockSize - rem
		padZeros := make([]byte, pad)
		if _, err := w.Write(padZeros); err != nil {
			return err
		}
		totalWritten += pad
	}

	ctx := context.Background()
	newCTOCEntries := make([]catalog.CTOCEntry, len(r.ctoc.Entries))

	// Fetch each chunk and write aligned
	for i, entry := range r.ctoc.Entries {
		var chunkKey string
		if r.chunkHashes != nil {
			if h, ok := r.chunkHashes.Get(uint32(i)); ok {
				chunkKey = spec.ChunkObjectKey(prefix, h)
			}
		}
		if chunkKey == "" {
			chunkKey = fmt.Sprintf("%schunks/%d.chnk", prefix, i)
		}

		rc, err := store.Get(ctx, chunkKey)
		if err != nil {
			return fmt.Errorf("get chunk %d (%s): %w", i, chunkKey, err)
		}

		rawChunk, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return fmt.Errorf("read chunk %d: %w", i, err)
		}

		currentBlock := uint64(totalWritten >> r.blockShift)
		newCTOCEntries[i] = catalog.CTOCEntry{
			BlockIndex:       currentBlock,
			CompressedSize:   entry.CompressedSize,
			UncompressedSize: entry.UncompressedSize,
		}

		// Write chunk bytes + padding
		rem := int64(len(rawChunk)) % blockSize
		pad := int64(0)
		if rem != 0 {
			pad = blockSize - rem
		}

		if _, err := w.Write(rawChunk); err != nil {
			return err
		}
		totalWritten += int64(len(rawChunk))

		if pad > 0 {
			padZeros := make([]byte, pad)
			if _, err := w.Write(padZeros); err != nil {
				return err
			}
			totalWritten += pad
		}
	}

	centralDirOffset := totalWritten

	// Write CTOC
	newCTOC := &catalog.CTOC{
		TotalChunks:     uint32(len(newCTOCEntries)),
		HotChunksCount:  r.ctoc.HotChunksCount,
		WarmChunksCount: r.ctoc.WarmChunksCount,
		Entries:         newCTOCEntries,
	}
	if _, err := newCTOC.Write(w); err != nil {
		return fmt.Errorf("write ctoc: %w", err)
	}

	// Write INDX
	if _, err := r.indx.Write(w); err != nil {
		return fmt.Errorf("write indx: %w", err)
	}

	// Write TREE & SNPS if present
	if r.snapshots != nil && r.treeStore != nil {
		if _, err := r.snapshots.Write(w); err != nil {
			return fmt.Errorf("write snapshots: %w", err)
		}

		if _, err := writeTreeNodes(w, r.treeStore); err != nil {
			return fmt.Errorf("write tree nodes: %w", err)
		}
	}

	// Write DSIG if present
	if r.dsig != nil {
		if _, err = r.dsig.Write(w); err != nil {
			return fmt.Errorf("write dsig: %w", err)
		}
	}

	// Write Trailer (32 bytes)
	var trailer [spec.TrailerSize]byte
	binary.LittleEndian.PutUint64(trailer[0:8], uint64(centralDirOffset))
	binary.LittleEndian.PutUint64(trailer[8:16], 0)
	binary.LittleEndian.PutUint64(trailer[16:24], 0)
	binary.LittleEndian.PutUint32(trailer[24:28], uint32(flags))
	copy(trailer[28:32], spec.MagicTrailer[:])

	if _, err := w.Write(trailer[:]); err != nil {
		return err
	}

	return nil
}
