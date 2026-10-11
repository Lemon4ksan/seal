// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xpress_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/codec/compress/xpress"
)

func TestXpress_ShortBuffer(t *testing.T) {
	out := make([]byte, 100)
	in := make([]byte, 10) // Less than HeaderTableBytes (256)
	_, err := xpress.Decompress(out, in)
	require.Error(t, err, "Expected error for short buffer")
}

func TestXpress_EmptyHeader(t *testing.T) {
	out := make([]byte, 100)
	in := make([]byte, 256) // All zeros is invalid Huffman table
	_, err := xpress.Decompress(out, in)
	require.Error(t, err, "Expected error for invalid table")
}
