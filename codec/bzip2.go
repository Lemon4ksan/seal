// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"compress/bzip2"
	"io"
)

// Bzip2Decompressor implements BZIP2 decompression.
type Bzip2Decompressor struct{}

// NewBzip2Decompressor creates a new BZIP2 decompressor.
func NewBzip2Decompressor() Decompressor {
	return &Bzip2Decompressor{}
}

// Decompress wraps the stream with [bzip2.NewReader].
func (b *Bzip2Decompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(bzip2.NewReader(src)), nil
}
