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

func TestPRZMBrotli_Roundtrip(t *testing.T) {
	comp := codec.NewPRZMBrotliCompressor(3)
	decomp := codec.NewPRZMDecompressor()

	original := "<html><head><title>Test PRZM Brotli Dict</title></head><body class=\"min-h-full flex flex-col\"><div><span class=\"font-medium\">Hello World</span></div></body></html>"
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

func TestComparePRZMBrotliDict_Effect(t *testing.T) {
	sampleHTML := `<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"><title>Dashboard</title><link rel="stylesheet" href="/style.css"></head><body class="min-h-full flex flex-col"><div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8"><div class="flex items-center justify-between h-16"><nav class="hidden md:flex space-x-4"><a href="/dashboard" class="text-slate-600 hover:text-indigo-600 px-3 py-2 text-sm font-medium">Dashboard</a><a href="/projects" class="text-slate-600 hover:text-indigo-600 px-3 py-2 text-sm font-medium">Projects</a><a href="/settings" class="text-slate-600 hover:text-indigo-600 px-3 py-2 text-sm font-medium">Settings</a></nav><div class="flex items-center space-x-4"><button type="button" class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700">New Item</button></div></div></div><div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8"><div class="bg-white p-6 rounded-lg shadow-sm border border-slate-200"><h3 class="text-lg font-semibold text-slate-900">Total Users</h3><p class="text-3xl font-bold text-indigo-600 mt-2">12,482</p></div><div class="bg-white p-6 rounded-lg shadow-sm border border-slate-200"><h3 class="text-lg font-semibold text-slate-900">Server Status</h3><p class="text-3xl font-bold text-emerald-600 mt-2">Operational</p></div><div class="bg-white p-6 rounded-lg shadow-sm border border-slate-200"><h3 class="text-lg font-semibold text-slate-900">Error Rate</h3><p class="text-3xl font-bold text-rose-600 mt-2">0.001%</p></div></div><script src="/app.js" defer></script></body></html>`
	raw := []byte(strings.Repeat(sampleHTML, 10))

	// 1. PRZM Cold (Zero Dict)
	coldComp, err := codec.CompressPRZM(raw, nil, 5)
	require.NoError(t, err)

	// 2. PRZM + Brotli Dict
	brotliDictComp, err := codec.CompressPRZMBrotli(raw, nil, 5)
	require.NoError(t, err)

	// Verify decompression
	dec, err := codec.Unprzm(brotliDictComp, nil)
	require.NoError(t, err)
	assert.Equal(t, raw, dec)

	t.Logf("=== HTML Payload (Raw: %d bytes) ===", len(raw))
	t.Logf("PRZM Cold (Zero Dict): %d bytes (%.2fx)", len(coldComp), float64(len(raw))/float64(len(coldComp)))
	t.Logf("PRZM + Brotli Dict:   %d bytes (%.2fx) -> Saved: %d bytes (-%.1f%%)",
		len(brotliDictComp),
		float64(len(raw))/float64(len(brotliDictComp)),
		len(coldComp)-len(brotliDictComp),
		float64(len(coldComp)-len(brotliDictComp))/float64(len(coldComp))*100)

	// Test 2: Structured JSON
	sampleJSON := `{"id":84192,"name":"alex_developer","email":"alex@engineering.corp","roles":["admin","billing_manager"],"status":"success","data":{"session_id":"sess_98f4a1c0d2","region":"us-east-1","timestamp":"2026-10-11T01:15:30.124Z","metrics":{"cpu":14.2,"mem_rss_mb":248.5,"iops":12840},"in_stock":true,"tags":["cloud","saas","premium"],"organization":"Acme Systems International LLC","tier":"enterprise_plus","mfa_enabled":true,"last_login_ip":"192.168.1.105"}}`
	rawJSON := []byte(strings.Repeat(sampleJSON, 10))

	coldJSON, err := codec.CompressPRZM(rawJSON, nil, 5)
	require.NoError(t, err)

	dictJSON, err := codec.CompressPRZMBrotli(rawJSON, nil, 5)
	require.NoError(t, err)

	decJSON, err := codec.Unprzm(dictJSON, nil)
	require.NoError(t, err)
	assert.Equal(t, rawJSON, decJSON)

	t.Logf("=== JSON Payload (Raw: %d bytes) ===", len(rawJSON))
	t.Logf("PRZM Cold (Zero Dict): %d bytes (%.2fx)", len(coldJSON), float64(len(rawJSON))/float64(len(coldJSON)))
	t.Logf("PRZM + Brotli Dict:   %d bytes (%.2fx) -> Saved: %d bytes (-%.1f%%)",
		len(dictJSON),
		float64(len(rawJSON))/float64(len(dictJSON)),
		len(coldJSON)-len(dictJSON),
		float64(len(coldJSON)-len(dictJSON))/float64(len(coldJSON))*100)
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
