// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package chunk manages solid block-aligned chunks and the extensible chunk pipeline.
package chunk

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/container/eon/codec"
	"github.com/lemon4ksan/seal/container/eon/filter"
	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/foundation/codec/filter/shuffle"
)

// Chunk represents an independently addressable storage chunk in an EON container.
type Chunk struct {
	Index            uint32
	CodecID          uint8
	CodecLevel       uint8
	Flags            uint16
	UncompressedSize uint64
	CompressedSize   uint64
	ItemCount        uint32
	Hash             [32]byte
	BlockIndex       uint64
	Payload          []byte           // Compressed on disk, uncompressed in memory
	Pipeline         *filter.Pipeline // Optional custom filter and codec pipeline
}

// MicroFrameCheckpoint records a 64 KiB entropy decoder restart point.
type MicroFrameCheckpoint struct {
	UncompressedOffset uint64
	CompressedOffset   uint64
}

// BuildHeader serializes the 64-byte binary Chunk Header.
func (c *Chunk) BuildHeader() [spec.ChunkHeaderSize]byte {
	var hdr [spec.ChunkHeaderSize]byte
	copy(hdr[0:4], spec.MagicChunk[:])
	binary.LittleEndian.PutUint32(hdr[4:8], c.Index)
	hdr[8] = c.CodecID
	hdr[9] = c.CodecLevel
	binary.LittleEndian.PutUint16(hdr[10:12], c.Flags)
	binary.LittleEndian.PutUint64(hdr[12:20], c.UncompressedSize)
	binary.LittleEndian.PutUint64(hdr[20:28], c.CompressedSize)
	binary.LittleEndian.PutUint32(hdr[28:32], c.ItemCount)
	copy(hdr[32:64], c.Hash[:])
	return hdr
}

// ParseHeader parses the 64-byte binary Chunk Header.
func ParseHeader(r io.Reader) (*Chunk, error) {
	var hdr [spec.ChunkHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicChunkLE {
		return nil, spec.ErrInvalidChunk
	}

	c := &Chunk{
		Index:            binary.LittleEndian.Uint32(hdr[4:8]),
		CodecID:          hdr[8],
		CodecLevel:       hdr[9],
		Flags:            binary.LittleEndian.Uint16(hdr[10:12]),
		UncompressedSize: binary.LittleEndian.Uint64(hdr[12:20]),
		CompressedSize:   binary.LittleEndian.Uint64(hdr[20:28]),
		ItemCount:        binary.LittleEndian.Uint32(hdr[28:32]),
	}
	copy(c.Hash[:], hdr[32:64])
	return c, nil
}

// NewWithPipeline constructs a new Chunk pre-configured with a custom filter pipeline.
func NewWithPipeline(index uint32, p *filter.Pipeline) *Chunk {
	return &Chunk{
		Index:      index,
		CodecID:    p.CodecID,
		CodecLevel: p.CodecLevel,
		Flags:      spec.ChunkFlagSolid | spec.ChunkFlagPipeline,
		Pipeline:   p,
	}
}

// Compress encodes raw payload using the designated codec (or custom pipeline) and updates chunk sizes and hash.
func (c *Chunk) Compress(raw []byte) error {
	c.UncompressedSize = uint64(len(raw))
	c.Hash = sha256.Sum256(raw)

	if (c.Flags&spec.ChunkFlagPipeline) != 0 && c.Pipeline != nil {
		compressed, err := c.Pipeline.Encode(raw)
		if err != nil {
			return fmt.Errorf("chunk pipeline compress: %w", err)
		}
		c.CompressedSize = uint64(len(compressed))
		c.Payload = compressed
		return nil
	}

	payloadToCompress := raw
	if (c.Flags & spec.ChunkFlagByteShuffle) != 0 {
		var err error
		payloadToCompress, err = shuffle.Encode(raw, nil, 4)
		if err != nil {
			return fmt.Errorf("chunk shuffle encode: %w", err)
		}
	}

	eng, err := codec.Get(c.CodecID)
	if err != nil {
		return fmt.Errorf("chunk compress: %w", err)
	}

	compressed, err := eng.Compress(payloadToCompress, int(c.CodecLevel))
	if err != nil {
		return fmt.Errorf("chunk compress: %w", err)
	}

	c.CompressedSize = uint64(len(compressed))
	c.Payload = compressed
	return nil
}

// Decompress decodes the compressed payload into uncompressed bytes.
func (c *Chunk) Decompress() ([]byte, error) {
	var raw []byte
	var err error

	if (c.Flags & spec.ChunkFlagPipeline) != 0 {
		raw, err = filter.Decode(c.Payload, c.CodecID)
		if err != nil {
			return nil, fmt.Errorf("chunk pipeline decompress: %w", err)
		}
	} else {
		eng, err := codec.Get(c.CodecID)
		if err != nil {
			return nil, fmt.Errorf("chunk decompress: %w", err)
		}

		raw, err = eng.Decompress(c.Payload)
		if err != nil {
			return nil, fmt.Errorf("chunk decompress: %w", err)
		}

		if (c.Flags & spec.ChunkFlagByteShuffle) != 0 {
			raw, err = shuffle.Decode(raw, nil, 4)
			if err != nil {
				return nil, fmt.Errorf("chunk shuffle decode: %w", err)
			}
		}
	}

	if uint64(len(raw)) != c.UncompressedSize {
		return nil, fmt.Errorf("%w: expected %d bytes, got %d", spec.ErrChecksumFailed, c.UncompressedSize, len(raw))
	}
	if c.Hash != [32]byte{} {
		h := sha256.Sum256(raw)
		if h != c.Hash {
			return nil, fmt.Errorf("%w: chunk payload hash mismatch", spec.ErrChecksumFailed)
		}
	}
	return raw, nil
}
