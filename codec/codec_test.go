// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/codec"
)

func TestDeflate_Roundtrip(t *testing.T) {
	comp := codec.NewDeflateCompressor(codec.DefaultCompression)
	decomp := codec.NewDeflateDecompressor()

	original := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 50)
	var compressed bytes.Buffer

	uncompBytes, err := comp.Compress(strings.NewReader(original), &compressed)
	require.NoError(t, err)
	assert.Equal(t, int64(len(original)), uncompBytes)

	rc, err := decomp.Decompress(&compressed)
	require.NoError(t, err)
	defer rc.Close()

	decompressed, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, original, string(decompressed))
}

func TestGzip_Roundtrip(t *testing.T) {
	comp := codec.NewGzipCompressor(codec.DefaultCompression)
	decomp := codec.NewGzipDecompressor()

	original := "Hello GZIP Compression Engine"
	var compressed bytes.Buffer

	_, err := comp.Compress(strings.NewReader(original), &compressed)
	require.NoError(t, err)

	rc, err := decomp.Decompress(&compressed)
	require.NoError(t, err)
	defer rc.Close()

	decompressed, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, original, string(decompressed))
}
