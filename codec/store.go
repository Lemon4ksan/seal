// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"io"

	"github.com/lemon4ksan/seal/internal/pool"
)

type storeCompressor struct{}

// NewStoreCompressor creates a pass-through compressor without compression overhead.
func NewStoreCompressor(_ int) Compressor {
	return &storeCompressor{}
}

// Compress passes bytes directly from src to dest using a pooled 64 KB memory buffer.
func (s *storeCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	bufPtr := pool.Get64K()
	defer pool.Put64K(bufPtr)

	return io.CopyBuffer(dest, src, *bufPtr)
}

type storeDecompressor struct{}

// NewStoreDecompressor creates a pass-through decompressor.
func NewStoreDecompressor() Decompressor {
	return &storeDecompressor{}
}

// Decompress wraps the stream in a non-closing reader.
func (s *storeDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(src), nil
}
