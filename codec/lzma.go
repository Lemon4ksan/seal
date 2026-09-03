// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"io"

	"github.com/lemon4ksan/foundation/codec/compress/lzma"
)

// LZMACompressor implements LZMA1 compression.
type LZMACompressor struct {
	level int
}

// NewLZMACompressor creates a new LZMA1 compressor for the specified level.
func NewLZMACompressor(level int) Compressor {
	return &LZMACompressor{level: level}
}

// Compress compresses payload from src into dest using LZMA1.
func (c *LZMACompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	comp := lzma.NewCompressor(c.level)
	return comp.Compress(src, dest)
}

// LZMADecompressor implements LZMA1 decompression.
type LZMADecompressor struct{}

// NewLZMADecompressor creates a new LZMA1 decompressor.
func NewLZMADecompressor() Decompressor {
	return &LZMADecompressor{}
}

// Decompress wraps the compressed stream with LZMA1 decompressor.
func (d *LZMADecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	decomp := lzma.NewDecompressor(0, ^uint64(0))
	return decomp.Decompress(src)
}

// LZMA2Compressor implements LZMA2 compression.
type LZMA2Compressor struct {
	level int
}

// NewLZMA2Compressor creates a new LZMA2 compressor.
func NewLZMA2Compressor(level int) Compressor {
	return &LZMA2Compressor{level: level}
}

// Compress compresses payload from src into dest using LZMA2.
func (c *LZMA2Compressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	comp := lzma.NewCompressor2(8 * 1024 * 1024)
	return comp.Compress(src, dest)
}

// LZMA2Decompressor implements LZMA2 decompression.
type LZMA2Decompressor struct{}

// NewLZMA2Decompressor creates a new LZMA2 decompressor.
func NewLZMA2Decompressor() Decompressor {
	return &LZMA2Decompressor{}
}

// Decompress wraps the compressed stream with LZMA2 decompressor.
func (d *LZMA2Decompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	decomp := lzma.NewDecompressor2(8 * 1024 * 1024)
	return decomp.Decompress(src)
}
