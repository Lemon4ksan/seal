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

func TestStore_Roundtrip(t *testing.T) {
	comp := codec.NewStoreCompressor(codec.NoCompression)
	decomp := codec.NewStoreDecompressor()

	original := "Store Raw Payload Uncompressed"
	var compressed bytes.Buffer

	n, err := comp.Compress(strings.NewReader(original), &compressed)
	require.NoError(t, err)
	assert.Equal(t, int64(len(original)), n)

	rc, err := decomp.Decompress(&compressed)
	require.NoError(t, err)
	defer rc.Close()

	decompressed, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, original, string(decompressed))
}

func TestZlib_Roundtrip(t *testing.T) {
	comp := codec.NewZlibCompressor(codec.DefaultCompression)
	decomp := codec.NewZlibDecompressor()

	original := strings.Repeat("ZLIB Stream Compression Layer Test ", 20)
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

func TestLZMA2_Roundtrip(t *testing.T) {
	comp := codec.NewLZMA2Compressor(codec.DefaultCompression)
	decomp := codec.NewLZMA2Decompressor()

	original := strings.Repeat("LZMA2 Predictive Dictionary Stream Test ", 25)
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

func TestPRZM_Roundtrip(t *testing.T) {
	comp := codec.NewPRZMCompressor(1)
	decomp := codec.NewPRZMDecompressor()

	original := strings.Repeat("PRZM Geometric Compression Engine Payload ", 25)
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

func TestRegistry_Contracts(t *testing.T) {
	reg := codec.DefaultRegistry

	// Verify standard compressor and decompressor resolutions
	comp, err := reg.Compressor(codec.Deflate, 6)
	require.NoError(t, err)
	require.NotNil(t, comp)

	decomp, err := reg.Decompressor(codec.Deflate)
	require.NoError(t, err)
	require.NotNil(t, decomp)

	// Unsupported method error handling
	_, err = reg.Compressor(codec.Method(9999), 1)
	require.Error(t, err)

	_, err = reg.Decompressor(codec.Method(9999))
	require.Error(t, err)

	// Clone isolation
	cloned := reg.Clone()
	require.NotNil(t, cloned)

	cloneComp, err := cloned.Compressor(codec.Deflate, 6)
	require.NoError(t, err)
	require.NotNil(t, cloneComp)
}

func TestMethod_String(t *testing.T) {
	assert.Equal(t, "Store", codec.Store.String())
	assert.Equal(t, "Deflate", codec.Deflate.String())
	assert.Equal(t, "Gzip", codec.Gzip.String())
	assert.Equal(t, "Zlib", codec.Zlib.String())
	assert.Equal(t, "BZIP2", codec.BZIP2.String())
	assert.Equal(t, "LZMA", codec.LZMA.String())
	assert.Equal(t, "PRZM", codec.PRZM.String())
	assert.Equal(t, "Unknown", codec.Method(65535).String())
}
