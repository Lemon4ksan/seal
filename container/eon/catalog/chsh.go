// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/container/eon/spec"
)

// ChunkHashTable stores content-addressed SHA-256 hashes for each chunk in the CTOC.
// Used for detached/object store layout (S3 / R2 / GCS) to map chunk index -> CAS object key.
type ChunkHashTable struct {
	Hashes [][32]byte
}

// NewChunkHashTable creates an empty ChunkHashTable.
func NewChunkHashTable() *ChunkHashTable {
	return &ChunkHashTable{}
}

// Add appends a chunk hash to the table.
func (t *ChunkHashTable) Add(h [32]byte) {
	t.Hashes = append(t.Hashes, h)
}

// Get returns the hash for chunkIdx, or empty hash and false if out of range.
func (t *ChunkHashTable) Get(chunkIdx uint32) ([32]byte, bool) {
	if int(chunkIdx) >= len(t.Hashes) {
		return [32]byte{}, false
	}
	return t.Hashes[chunkIdx], true
}

// Write serializes the CHSH table to w.
func (t *ChunkHashTable) Write(w io.Writer) (int64, error) {
	var total int64
	var hdr [8]byte
	copy(hdr[0:4], spec.MagicCHSH[:])
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(t.Hashes)))

	if _, err := w.Write(hdr[:]); err != nil {
		return total, err
	}
	total += 8

	for _, h := range t.Hashes {
		if _, err := w.Write(h[:]); err != nil {
			return total, err
		}
		total += 32
	}
	return total, nil
}

// ReadChunkHashTable parses a CHSH table from r.
func ReadChunkHashTable(r io.Reader) (*ChunkHashTable, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicCHSHLE {
		return nil, fmt.Errorf("invalid CHSH magic")
	}

	count := binary.LittleEndian.Uint32(hdr[4:8])
	hashes := make([][32]byte, count)
	buf := make([]byte, count*32)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read chunk hashes: %w", err)
	}

	for i := uint32(0); i < count; i++ {
		copy(hashes[i][:], buf[i*32:(i+1)*32])
	}

	return &ChunkHashTable{Hashes: hashes}, nil
}
