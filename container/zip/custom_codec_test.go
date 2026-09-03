// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/file"
)

// xorMockCodec implements codec.Compressor and codec.Decompressor for custom ZIP tests.
type xorMockCompressor struct {
	key byte
}

func (x *xorMockCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	data, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}
	encoded := make([]byte, len(data))
	for i, b := range data {
		encoded[i] = b ^ x.key
	}
	n, err := dest.Write(encoded)
	return int64(n), err
}

type xorMockDecompressor struct {
	key byte
}

func (x *xorMockDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	data, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	decoded := make([]byte, len(data))
	for i, b := range data {
		decoded[i] = b ^ x.key
	}
	return io.NopCloser(bytes.NewReader(decoded)), nil
}

func TestZip_CustomCodec_Registration(t *testing.T) {
	const CustomMethod codec.Method = 100
	const xorKey byte = 0x42

	// Register the custom codec into ZIP
	zip.RegisterCodec(
		CustomMethod,
		func(_ int) codec.Compressor {
			return &xorMockCompressor{key: xorKey}
		},
		func() codec.Decompressor {
			return &xorMockDecompressor{key: xorKey}
		},
	)

	// Create an archive with an entry using CustomMethod
	original := []byte("Custom compression algorithm inside a standard ZIP container!")
	f1, err := file.New("payload.bin", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.WithData(original)
	f1.SetMethod(CustomMethod)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf, codec.DefaultRegistry)

	if err := zw.WriteEntry(f1, bytes.NewReader(original)); err != nil {
		t.Fatalf("WriteEntry with custom codec failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Closing zip writer: %v", err)
	}

	// Verify that the file payload on disk is actually encoded/compressed
	if bytes.Contains(zipBuf.Bytes(), original) {
		t.Fatal("Expected raw ZIP payload to be transformed/compressed")
	}

	// Read back and extract using Seal ZIP reader
	rAt := bytes.NewReader(zipBuf.Bytes())
	zr, err := zip.NewReader(rAt, int64(zipBuf.Len()), "", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader failed on custom codec ZIP: %v", err)
	}

	entries := zr.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	rc, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Opening custom codec entry failed: %v", err)
	}
	extracted, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("Reading extracted payload: %v", err)
	}

	if !bytes.Equal(extracted, original) {
		t.Fatalf("Extracted content mismatch: got %q, want %q", string(extracted), string(original))
	}
}
