// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzx_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/codec/lzx"
)

func TestLZX_Roundtrip(t *testing.T) {
	original := []byte("The quick brown fox jumps over the lazy dog. Windows Imaging Format LZX compression test 2026!")

	comp, err := lzx.Compress(nil, original, lzx.DefaultWindowBits)
	require.NoError(t, err)

	decomp := make([]byte, len(original))
	n, err := lzx.Decompress(decomp, comp)
	require.NoError(t, err)
	assert.Equal(t, len(original), n)
	assert.Equal(t, original, decomp)
}

func TestLZX_BinaryData(t *testing.T) {
	original := make([]byte, 16*1024)
	for i := range original {
		original[i] = byte(i * 31)
	}

	comp, err := lzx.Compress(nil, original, lzx.DefaultWindowBits)
	require.NoError(t, err)

	decomp := make([]byte, len(original))
	n, err := lzx.Decompress(decomp, comp)
	require.NoError(t, err)
	assert.Equal(t, len(original), n)
	assert.Equal(t, original, decomp)
}
