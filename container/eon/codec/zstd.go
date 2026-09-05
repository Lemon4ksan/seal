// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"runtime"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// ZstdCodec implements the RFC 8878 Zstandard compression engine.
type ZstdCodec struct {
	encoderPool sync.Pool
	decoderPool sync.Pool
}

// NewZstdCodec constructs a thread-safe pooled ZSTD codec.
func NewZstdCodec() *ZstdCodec {
	concurrency := runtime.GOMAXPROCS(0)
	return &ZstdCodec{
		encoderPool: sync.Pool{
			New: func() any {
				enc, _ := zstd.NewWriter(nil,
					zstd.WithEncoderLevel(zstd.SpeedDefault),
					zstd.WithEncoderConcurrency(concurrency),
				)
				return enc
			},
		},
		decoderPool: sync.Pool{
			New: func() any {
				dec, _ := zstd.NewReader(nil,
					zstd.WithDecoderConcurrency(concurrency),
				)
				return dec
			},
		},
	}
}

// ID returns [spec.CodecZSTD].
func (z *ZstdCodec) ID() uint8 {
	return spec.CodecZSTD
}

// Name returns "zstd".
func (z *ZstdCodec) Name() string {
	return "zstd"
}

// Compress encodes src with Zstandard.
func (z *ZstdCodec) Compress(src []byte, level int) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	enc := z.encoderPool.Get().(*zstd.Encoder)
	defer z.encoderPool.Put(enc)

	var buf bytes.Buffer
	enc.Reset(&buf)
	if _, err := enc.Write(src); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decompress decodes src with Zstandard.
func (z *ZstdCodec) Decompress(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	dec := z.decoderPool.Get().(*zstd.Decoder)
	defer z.decoderPool.Put(dec)

	return dec.DecodeAll(src, nil)
}
