// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package legacy bundles PKWARE legacy compression algorithms (Shrink, Reduce 1-4, Implode, Deflate64).
package legacy

import (
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/codec/legacy/dcl"
	"github.com/lemon4ksan/seal/codec/legacy/deflate64"
	"github.com/lemon4ksan/seal/codec/legacy/implode"
	"github.com/lemon4ksan/seal/codec/legacy/reduce"
	"github.com/lemon4ksan/seal/codec/legacy/shrink"
)

func init() {
	RegisterAll(codec.DefaultRegistry)
}

// RegisterAll registers all legacy decompressors into the target registry.
func RegisterAll(reg *codec.Registry) {
	if reg == nil {
		return
	}

	// Method 1: Shrink
	reg.RegisterCompressor(codec.Shrink, func(_ int) codec.Compressor {
		return shrink.NewCompressor()
	})
	reg.RegisterDecompressor(codec.Shrink, func() codec.Decompressor {
		return &shrinkDecompressor{}
	})

	// Methods 2-5: Reduce 1 to 4
	reg.RegisterDecompressor(codec.Reduce1, func() codec.Decompressor {
		return &reduceDecompressor{factor: 1}
	})
	reg.RegisterDecompressor(codec.Reduce2, func() codec.Decompressor {
		return &reduceDecompressor{factor: 2}
	})
	reg.RegisterDecompressor(codec.Reduce3, func() codec.Decompressor {
		return &reduceDecompressor{factor: 3}
	})
	reg.RegisterDecompressor(codec.Reduce4, func() codec.Decompressor {
		return &reduceDecompressor{factor: 4}
	})

	// Method 6: Implode
	reg.RegisterDecompressor(codec.Implode, func() codec.Decompressor {
		return &implodeDecompressor{}
	})

	// Method 9: Deflate64
	reg.RegisterDecompressor(codec.Deflate64, func() codec.Decompressor {
		return &deflate64Decompressor{}
	})

	// Method 10: PKWARE DCL Implode
	reg.RegisterDecompressor(codec.PKWAREDCL, func() codec.Decompressor {
		return &dclDecompressor{}
	})
}

type shrinkDecompressor struct{}

func (s *shrinkDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return shrink.NewReader(src), nil
}

type reduceDecompressor struct {
	factor int
}

func (r *reduceDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return reduce.NewReader(src, r.factor)
}

type implodeDecompressor struct{}

func (i *implodeDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return implode.NewReader(src, true, true)
}

type deflate64Decompressor struct{}

func (d *deflate64Decompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return deflate64.NewReader(src), nil
}

type dclDecompressor struct{}

func (d *dclDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return dcl.NewReader(src), nil
}
