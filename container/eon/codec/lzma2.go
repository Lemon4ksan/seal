// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"io"
	"sync"

	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/foundation/codec/compress/lzma"
)

var compressorPools sync.Map

func getCompressor(lvl lzma.Level) *lzma.Compressor2 {
	val, ok := compressorPools.Load(lvl)
	if !ok {
		p := &sync.Pool{
			New: func() any {
				opts := lzma.OptionsForLevel(lvl)
				return lzma.NewCompressor2WithOptions(opts)
			},
		}
		val, _ = compressorPools.LoadOrStore(lvl, p)
	}
	return val.(*sync.Pool).Get().(*lzma.Compressor2)
}

func putCompressor(lvl lzma.Level, c *lzma.Compressor2) {
	if val, ok := compressorPools.Load(lvl); ok {
		val.(*sync.Pool).Put(c)
	}
}

// LZMA2Codec implements LZMA2 chunk compression using the native parallel engine.
type LZMA2Codec struct{}

// ID returns [spec.CodecLZMA2].
func (l *LZMA2Codec) ID() uint8 {
	return spec.CodecLZMA2
}

// Name returns "lzma2".
func (l *LZMA2Codec) Name() string {
	return "lzma2"
}

// Compress encodes src using parallel LZMA2.
func (l *LZMA2Codec) Compress(src []byte, level int) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	lvl := lzma.LevelNormal
	if level > 0 {
		lvl = lzma.Level(level)
	}

	compressor := getCompressor(lvl)
	defer putCompressor(lvl, compressor)

	var buf bytes.Buffer
	if _, err := compressor.Compress(bytes.NewReader(src), &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decompress decodes an LZMA2 stream into uncompressed bytes.
func (l *LZMA2Codec) Decompress(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	decompressor := lzma.NewDecompressor2(32 * 1024 * 1024)
	rc, err := decompressor.Decompress(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	return io.ReadAll(rc)
}
