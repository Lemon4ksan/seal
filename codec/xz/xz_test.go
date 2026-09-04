// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xz_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/codec/xz"
)

func TestVLI_Roundtrip(t *testing.T) {
	testValues := []uint64{
		0, 1, 127, 128, 255, 16383, 16384, 65535, 1<<21 - 1, 1 << 32, 1<<63 - 1,
	}

	for _, v := range testValues {
		var buf bytes.Buffer
		n, err := xz.WriteVLI(&buf, v)
		require.NoError(t, err)
		assert.Equal(t, xz.VLISize(v), n)

		decoded, err := xz.ReadVLI(bytes.NewReader(buf.Bytes()))
		require.NoError(t, err)
		assert.Equal(t, v, decoded)
	}
}

func TestXZ_Roundtrip_Plain(t *testing.T) {
	data := []byte("The quick brown fox jumps over the lazy dog! XZ compression stream verification test.")

	var compBuf bytes.Buffer
	w, err := xz.NewWriter(&compBuf)
	require.NoError(t, err)

	_, err = w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Decompress
	r, err := xz.NewReader(bytes.NewReader(compBuf.Bytes()))
	require.NoError(t, err)
	defer r.Close()

	decomp, err := io.ReadAll(r)
	require.NoError(t, err)

	assert.Equal(t, data, decomp)
}

func TestXZ_Roundtrip_WithBCJ(t *testing.T) {
	// Sample binary data with x86 CALL instructions (0xE8)
	data := make([]byte, 4096)
	for i := 0; i+4 < len(data); i += 5 {
		data[i] = 0xE8
		data[i+1] = byte(i & 0xFF)
		data[i+2] = byte((i >> 8) & 0xFF)
		data[i+3] = 0x00
		data[i+4] = 0x00
	}

	opts := xz.DefaultOptions()
	opts.UseBCJ = true

	var compBuf bytes.Buffer
	w, err := xz.NewWriterOptions(&compBuf, opts)
	require.NoError(t, err)

	_, err = w.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Decompress
	r, err := xz.NewReader(bytes.NewReader(compBuf.Bytes()))
	require.NoError(t, err)
	defer r.Close()

	decomp, err := io.ReadAll(r)
	require.NoError(t, err)

	assert.Equal(t, data, decomp)
}

func TestXZ_InvalidHeader(t *testing.T) {
	invalid := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	_, err := xz.NewReader(bytes.NewReader(invalid))
	require.Error(t, err, "expected error on invalid magic")
}
