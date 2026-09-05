// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package catalog manages the Chunk Table of Contents (CTOC) and Universal Extent Index (INDX).
package catalog

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/container/eon/spec"
)

// CTOCEntry represents a single physical chunk record in the CTOC.
type CTOCEntry struct {
	BlockIndex       uint64
	CompressedSize   uint32
	UncompressedSize uint32
}

// CTOC represents the complete Chunk Table of Contents.
type CTOC struct {
	TotalChunks     uint32
	HotChunksCount  uint32
	WarmChunksCount uint32
	Entries         []CTOCEntry
}

// Write serializes the CTOC into w.
func (c *CTOC) Write(w io.Writer) (int64, error) {
	var total int64
	var hdr [spec.CTOCHeaderSize]byte
	copy(hdr[0:4], spec.MagicCTOC[:])
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(c.Entries)))
	binary.LittleEndian.PutUint32(hdr[8:12], c.HotChunksCount)
	binary.LittleEndian.PutUint32(hdr[12:16], c.WarmChunksCount)

	if _, err := w.Write(hdr[:]); err != nil {
		return total, err
	}
	total += spec.CTOCHeaderSize

	for _, entry := range c.Entries {
		var b [spec.CTOCEntrySize]byte
		binary.LittleEndian.PutUint64(b[0:8], entry.BlockIndex)
		binary.LittleEndian.PutUint32(b[8:12], entry.CompressedSize)
		binary.LittleEndian.PutUint32(b[12:16], entry.UncompressedSize)
		if _, err := w.Write(b[:]); err != nil {
			return total, err
		}
		total += spec.CTOCEntrySize
	}
	return total, nil
}

// ReadCTOC parses the CTOC table from r.
func ReadCTOC(r io.Reader) (*CTOC, error) {
	var hdr [spec.CTOCHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicCTOCLE {
		return nil, spec.ErrInvalidCTOC
	}

	totalChunks := binary.LittleEndian.Uint32(hdr[4:8])
	hotChunks := binary.LittleEndian.Uint32(hdr[8:12])
	warmChunks := binary.LittleEndian.Uint32(hdr[12:16])

	// Guard against integer overflow or unbounded allocations on corrupt input
	if uint64(totalChunks)*uint64(spec.CTOCEntrySize) > (1 << 30) {
		return nil, fmt.Errorf("%w: chunk count (%d) exceeds limit", spec.ErrInvalidCTOC, totalChunks)
	}

	entries := make([]CTOCEntry, totalChunks)
	entryBuf := make([]byte, int(totalChunks)*spec.CTOCEntrySize)
	if _, err := io.ReadFull(r, entryBuf); err != nil {
		return nil, fmt.Errorf("read ctoc entries: %w", err)
	}

	for i := uint32(0); i < totalChunks; i++ {
		offset := i * spec.CTOCEntrySize
		entries[i] = CTOCEntry{
			BlockIndex:       binary.LittleEndian.Uint64(entryBuf[offset : offset+8]),
			CompressedSize:   binary.LittleEndian.Uint32(entryBuf[offset+8 : offset+12]),
			UncompressedSize: binary.LittleEndian.Uint32(entryBuf[offset+12 : offset+16]),
		}
	}

	return &CTOC{
		TotalChunks:     totalChunks,
		HotChunksCount:  hotChunks,
		WarmChunksCount: warmChunks,
		Entries:         entries,
	}, nil
}
