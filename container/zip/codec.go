// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"github.com/lemon4ksan/seal/codec"
)

// RegisterCodec installs a custom compression and decompression implementation for ZIP archives.
// This allows plugging in any standard or custom algorithm (e.g. XPRESS, LZX, Zstd, Brotli, Snappy)
// with an arbitrary 16-bit compression Method ID.
func RegisterCodec(method codec.Method, comp codec.CompressorFactory, decomp codec.DecompressorFactory) {
	if comp != nil {
		codec.DefaultRegistry.RegisterCompressor(method, comp)
	}
	if decomp != nil {
		codec.DefaultRegistry.RegisterDecompressor(method, decomp)
	}
}
