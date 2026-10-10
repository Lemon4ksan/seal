// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"io"

	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/seal/codec/compress/brotli"
)

// BrotliCodec implements RFC 7932 Brotli compression for web and asset payloads.
type BrotliCodec struct{}

// ID returns [spec.CodecBrotli].
func (b *BrotliCodec) ID() uint8 {
	return spec.CodecBrotli
}

// Name returns "brotli".
func (b *BrotliCodec) Name() string {
	return "brotli"
}

// Compress encodes src with Brotli.
func (b *BrotliCodec) Compress(src []byte, level int) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	lvl := brotli.DefaultCompression
	if level > 0 {
		lvl = level
	}
	var buf bytes.Buffer
	w := brotli.NewWriterLevel(&buf, lvl)
	if _, err := w.Write(src); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decompress decodes src with Brotli.
func (b *BrotliCodec) Decompress(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	r := brotli.NewReader(bytes.NewReader(src))
	return io.ReadAll(r)
}
