// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmpsc_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/codec/legacy/cmpsc"
)

func TestCMPSC_Roundtrip_Text(t *testing.T) {
	input := []byte("IBM z/OS Enterprise Systems Architecture ESA/390 CMPSC Hardware Compression Test Vector 2026.")

	buf := new(bytes.Buffer)
	w := cmpsc.NewWriter(buf)
	_, err := w.Write(input)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	r, err := cmpsc.NewReader(buf)
	require.NoError(t, err)

	decompressed, err := io.ReadAll(r)
	require.NoError(t, err)

	assert.Equal(t, input, decompressed)
}

func TestCMPSC_Levels(t *testing.T) {
	input := bytes.Repeat([]byte("MAINFRAME_VSAM_RECORD_DATA_0123456789_EBCDIC_ASCII_"), 50)

	for _, level := range []int{1, 3, 6} {
		buf := new(bytes.Buffer)
		w := cmpsc.NewWriterLevel(buf, level)
		_, err := w.Write(input)
		require.NoError(t, err)
		require.NoError(t, w.Close())

		r, err := cmpsc.NewReader(buf)
		require.NoError(t, err)

		out, err := io.ReadAll(r)
		require.NoError(t, err)

		assert.Equal(t, input, out)
	}
}

func TestCMPSC_CodecRegistry(t *testing.T) {
	input := []byte("Testing CMPSC integration through codec.DefaultRegistry Method 16.")

	comp, err := codec.DefaultRegistry.Compressor(codec.IBMCmpsc, 6)
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	_, err = comp.Compress(bytes.NewReader(input), buf)
	require.NoError(t, err)

	decomp, err := codec.DefaultRegistry.Decompressor(codec.IBMCmpsc)
	require.NoError(t, err)

	rc, err := decomp.Decompress(buf)
	require.NoError(t, err)
	defer rc.Close()

	out, err := io.ReadAll(rc)
	require.NoError(t, err)

	assert.Equal(t, input, out)
}
