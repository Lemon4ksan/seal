// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"compress/gzip"
	"io"
	"sync"

	"github.com/lemon4ksan/seal/internal/pool"
)

// GzipCompressor implements RFC 1952 GZIP container compression.
type GzipCompressor struct {
	level   int
	writers sync.Pool
}

// NewGzipCompressor creates a reusable GZIP compressor.
func NewGzipCompressor(level int) Compressor {
	if level < gzip.HuffmanOnly || level > gzip.BestCompression {
		level = gzip.DefaultCompression
	}

	c := &GzipCompressor{level: level}
	c.writers = sync.Pool{
		New: func() any {
			w, _ := gzip.NewWriterLevel(io.Discard, level)
			return w
		},
	}
	return c
}

// Compress encodes stream into GZIP container format.
func (g *GzipCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	w := g.writers.Get().(*gzip.Writer)
	defer g.writers.Put(w)

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

// GzipDecompressor implements RFC 1952 GZIP decompression.
type GzipDecompressor struct{}

// NewGzipDecompressor creates a new GZIP decompressor.
func NewGzipDecompressor() Decompressor {
	return &GzipDecompressor{}
}

// Decompress wraps the stream with [gzip.NewReader].
func (g *GzipDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(src)
}
