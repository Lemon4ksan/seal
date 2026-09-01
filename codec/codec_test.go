// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/seal/codec"
)

func TestDeflate_Roundtrip(t *testing.T) {
	comp := codec.NewDeflateCompressor(codec.DefaultCompression)
	decomp := codec.NewDeflateDecompressor()

	original := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 50)
	var compressed bytes.Buffer

	uncompBytes, err := comp.Compress(strings.NewReader(original), &compressed)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if uncompBytes != int64(len(original)) {
		t.Errorf("uncompressed bytes mismatch: got %d, want %d", uncompBytes, len(original))
	}

	rc, err := decomp.Decompress(&compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	defer rc.Close()

	decompressed, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(decompressed) != original {
		t.Errorf("content mismatch")
	}
}

func TestGzip_Roundtrip(t *testing.T) {
	comp := codec.NewGzipCompressor(codec.DefaultCompression)
	decomp := codec.NewGzipDecompressor()

	original := "Hello GZIP Compression Engine"
	var compressed bytes.Buffer

	_, err := comp.Compress(strings.NewReader(original), &compressed)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	rc, err := decomp.Decompress(&compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	defer rc.Close()

	decompressed, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(decompressed) != original {
		t.Errorf("content mismatch: got %q, want %q", string(decompressed), original)
	}
}
