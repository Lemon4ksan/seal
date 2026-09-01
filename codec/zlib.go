// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"compress/zlib"
	"io"
	"sync"

	"github.com/lemon4ksan/seal/internal/pool"
)

// ZlibCompressor implements RFC 1950 ZLIB compression.
type ZlibCompressor struct {
	level   int
	writers sync.Pool
}

// NewZlibCompressor constructs a reusable ZLIB compressor.
func NewZlibCompressor(level int) Compressor {
	if level < zlib.HuffmanOnly || level > zlib.BestCompression {
		level = zlib.DefaultCompression
	}

	c := &ZlibCompressor{level: level}
	c.writers = sync.Pool{
		New: func() any {
			w, _ := zlib.NewWriterLevel(io.Discard, level)
			return w
		},
	}
	return c
}

// Compress encodes stream with ZLIB format.
func (z *ZlibCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	w := z.writers.Get().(*zlib.Writer)
	defer z.writers.Put(w)

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

// ZlibDecompressor implements RFC 1950 ZLIB decompression.
type ZlibDecompressor struct{}

// NewZlibDecompressor creates a new ZLIB decompressor.
func NewZlibDecompressor() Decompressor {
	return &ZlibDecompressor{}
}

// Decompress wraps the stream with [zlib.NewReader].
func (z *ZlibDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return zlib.NewReader(src)
}
