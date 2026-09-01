// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"compress/flate"
	"io"
	"sync"

	"github.com/lemon4ksan/seal/internal/pool"
)

// DeflateCompressor implements RFC 1951 DEFLATE compression with pooled writers and sized memory buffers.
type DeflateCompressor struct {
	level   int
	writers sync.Pool
}

// NewDeflateCompressor constructs a reusable DEFLATE compressor instance for the specified level.
func NewDeflateCompressor(level int) Compressor {
	if level < flate.HuffmanOnly || level > flate.BestCompression {
		level = flate.DefaultCompression
	}

	c := &DeflateCompressor{level: level}
	c.writers = sync.Pool{
		New: func() any {
			w, _ := flate.NewWriter(io.Discard, level)
			return w
		},
	}
	return c
}

// Compress compresses payload from src into dest.
func (d *DeflateCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	w := d.writers.Get().(*flate.Writer)
	defer d.writers.Put(w)

	w.Reset(dest)

	bufPtr := pool.Get64K()
	defer pool.Put64K(bufPtr)

	n, err := io.CopyBuffer(w, src, *bufPtr)
	if err != nil {
		return n, err
	}

	if err := w.Close(); err != nil {
		return n, err
	}

	return n, nil
}

// DeflateDecompressor implements RFC 1951 DEFLATE decompression.
type DeflateDecompressor struct{}

// NewDeflateDecompressor creates a new DEFLATE decompressor.
func NewDeflateDecompressor() Decompressor {
	return &DeflateDecompressor{}
}

// Decompress wraps the compressed stream with [flate.NewReader].
func (d *DeflateDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return flate.NewReader(src), nil
}
