// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"io"

	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/seal/codec/compress/lz4"
)

// LZ4Codec implements ultra-fast LZ4 compression for low seek latency and FastMount profiles.
type LZ4Codec struct{}

// ID returns [spec.CodecLZ4].
func (l *LZ4Codec) ID() uint8 {
	return spec.CodecLZ4
}

// Name returns "lz4".
func (l *LZ4Codec) Name() string {
	return "lz4"
}

// Compress encodes src with LZ4 framed compression.
func (l *LZ4Codec) Compress(src []byte, level int) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	w := lz4.NewWriter(&buf)
	if level > 0 {
		var cl lz4.CompressionLevel
		switch {
		case level <= 1:
			cl = lz4.Level1
		case level == 2:
			cl = lz4.Level2
		case level == 3:
			cl = lz4.Level3
		case level == 4:
			cl = lz4.Level4
		case level == 5:
			cl = lz4.Level5
		case level == 6:
			cl = lz4.Level6
		case level == 7:
			cl = lz4.Level7
		case level == 8:
			cl = lz4.Level8
		default:
			cl = lz4.Level9
		}
		_ = w.Apply(lz4.CompressionLevelOption(cl))
	}
	if _, err := w.Write(src); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decompress decodes src with LZ4 framed decompression.
func (l *LZ4Codec) Decompress(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	r := lz4.NewReader(bytes.NewReader(src))
	return io.ReadAll(r)
}
