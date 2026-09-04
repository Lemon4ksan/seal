// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package deflate64_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/codec/legacy/deflate64"
)

func TestDeflate64_Empty(t *testing.T) {
	r := deflate64.NewReader(bytes.NewReader(nil))
	defer r.Close()

	buf := make([]byte, 16)
	n, err := r.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, 0, n)
}

func TestDeflate64_StoredBlock(t *testing.T) {
	// Stored block format:
	// 3 bits header: BFINAL=1 (1), BTYPE=00 (00) -> 0x01
	// Align to byte boundary
	// LEN = 5 (0x0005)
	// NLEN = ~5 (0xFFFA)
	// Data: "Hello"
	data := []byte{
		0x01,       // BFINAL=1, BTYPE=00
		0x05, 0x00, // LEN = 5
		0xFA, 0xFF, // NLEN = ~5
		'H', 'e', 'l', 'l', 'o', // Payload
	}

	r := deflate64.NewReader(bytes.NewReader(data))
	defer r.Close()

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "Hello", string(out))
}
