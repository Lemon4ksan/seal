// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/lemon4ksan/przm"
)

// CompressPRZM compresses src using the PRZM (PrismLZ) structured dictionary compression format.
func CompressPRZM(src, dst []byte, level ...int) ([]byte, error) {
	lvl := 3
	if len(level) > 0 {
		lvl = level[0]
	}

	opts := []any{przm.WithLevel(lvl)}
	if os.Getenv("PRZM_GPU") == "1" {
		opts = append(opts, przm.WithGPUAcceleration())
	}

	compressed, err := przm.CompressWithOptions(src, opts...)
	if err != nil {
		return nil, err
	}
	if cap(dst) >= len(compressed) {
		dst = dst[:len(compressed)]
		copy(dst, compressed)
		return dst, nil
	}
	return compressed, nil
}

// Unprzm decompresses a PRZM payload from src into dst.
func Unprzm(src, dst []byte) ([]byte, error) {
	if len(dst) > 0 {
		return przm.DecompressTo(src, dst)
	}
	decompressed, err := przm.Decode(src)
	if err != nil {
		return nil, err
	}
	if cap(dst) >= len(decompressed) {
		dst = dst[:len(decompressed)]
		copy(dst, decompressed)
		return dst, nil
	}
	return decompressed, nil
}

// PRZMCompressor implements streaming PRZM compression.
type PRZMCompressor struct {
	level int
}

// NewPRZMCompressor creates a new PRZM compressor.
func NewPRZMCompressor(level int) Compressor {
	if level <= 0 {
		level = 3
	}
	return &PRZMCompressor{level: level}
}

// Compress reads from src, compresses via PRZM, and writes length-prefixed blocks into dest.
func (c *PRZMCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	raw, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}
	opts := []any{przm.WithLevel(c.level)}
	if os.Getenv("PRZM_GPU") == "1" {
		opts = append(opts, przm.WithGPUAcceleration())
	}
	compressed, err := przm.CompressWithOptions(raw, opts...)
	if err != nil {
		return 0, err
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(compressed)))
	if _, err := dest.Write(lenBuf[:]); err != nil {
		return 0, err
	}
	if _, err := dest.Write(compressed); err != nil {
		return 0, err
	}
	return int64(len(raw)), nil
}

// CompressPRZMBrotli compresses src using PRZM with the injected 122KB RFC 7932 Brotli Web dictionary.
func CompressPRZMBrotli(src, dst []byte, level ...int) ([]byte, error) {
	RegisterBrotliWebDictionary()
	lvl := 3
	if len(level) > 0 {
		lvl = level[0]
	}
	rawDict := GetBrotliWebDictionaryData()
	opts := []any{
		przm.WithLevel(lvl),
		przm.WithDict(rawDict),
	}
	if os.Getenv("PRZM_GPU") == "1" {
		opts = append(opts, przm.WithGPUAcceleration())
	}
	compressed, err := przm.CompressWithOptions(src, opts...)
	if err != nil {
		return nil, err
	}
	if cap(dst) >= len(compressed) {
		dst = dst[:len(compressed)]
		copy(dst, compressed)
		return dst, nil
	}
	return compressed, nil
}

// PRZMBrotliCompressor implements streaming PRZM compression with injected Brotli dictionary.
type PRZMBrotliCompressor struct {
	level int
}

// NewPRZMBrotliCompressor creates a new PRZM compressor using the 122KB Brotli Web dictionary.
func NewPRZMBrotliCompressor(level int) Compressor {
	if level <= 0 {
		level = 3
	}
	RegisterBrotliWebDictionary()
	return &PRZMBrotliCompressor{level: level}
}

// Compress reads from src, compresses via PRZM with Brotli dictionary, and writes length-prefixed blocks into dest.
func (c *PRZMBrotliCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	raw, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}
	rawDict := GetBrotliWebDictionaryData()
	opts := []any{
		przm.WithLevel(c.level),
		przm.WithDict(rawDict),
	}
	if os.Getenv("PRZM_GPU") == "1" {
		opts = append(opts, przm.WithGPUAcceleration())
	}
	compressed, err := przm.CompressWithOptions(raw, opts...)
	if err != nil {
		return 0, err
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(compressed)))
	if _, err := dest.Write(lenBuf[:]); err != nil {
		return 0, err
	}
	if _, err := dest.Write(compressed); err != nil {
		return 0, err
	}
	return int64(len(raw)), nil
}

// PRZMDecompressor implements PRZM decompression.
type PRZMDecompressor struct{}

// NewPRZMDecompressor creates a new PRZM decompressor.
func NewPRZMDecompressor() Decompressor {
	return &PRZMDecompressor{}
}

// Decompress wraps the stream, reading length-prefixed or bare PRZM frames.
func (d *PRZMDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	payload, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	if len(payload) < 4 {
		return nil, fmt.Errorf("przm: stream too short (%d bytes)", len(payload))
	}
	// Check if bare PRZM frame (starts with 'PRZM' magic)
	if binary.LittleEndian.Uint32(payload[0:4]) == przm.MagicPRZM {
		decompressed, err := przm.Decode(payload)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(decompressed)), nil
	}
	// Length-prefixed block
	frameLen := int(binary.LittleEndian.Uint32(payload[0:4]))
	if len(payload) >= 4+frameLen {
		decompressed, err := przm.Decode(payload[4 : 4+frameLen])
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(decompressed)), nil
	}
	return nil, errors.New("przm: invalid stream header")
}
