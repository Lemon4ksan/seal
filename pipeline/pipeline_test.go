// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"bytes"
	"hash/crc32"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/crypto"
)

func TestWritePipeline_FullChain(t *testing.T) {
	rawContent := []byte(
		"The quick brown fox jumps over the lazy dog. A quick brown fox jumps over the lazy dog again and again!",
	)
	expectedCRC := crc32.ChecksumIEEE(rawContent)

	var outputBuf bytes.Buffer

	// Build composite pipeline: Hasher -> Deflate -> AES-CTR (WinZip) -> Output
	pipe := NewWritePipeline(&outputBuf).
		WithHasher(crc32.NewIEEE()).
		WithCipher(func(w io.Writer) (io.WriteCloser, error) {
			return crypto.NewAESCTRWriter(w, "PipelineSecret123", 32, 16)
		}).
		WithCompressor(func(src io.Reader, dst io.Writer) (int64, error) {
			comp, err := codec.DefaultRegistry.Compressor(codec.Deflate, 6)
			if err != nil {
				return 0, err
			}
			return comp.Compress(src, dst)
		})

	res, err := pipe.Execute(bytes.NewReader(rawContent))
	if err != nil {
		t.Fatalf("pipeline.Execute failed: %v", err)
	}

	if res.UncompressedBytes != int64(len(rawContent)) {
		t.Fatalf("UncompressedBytes mismatch: got %d, want %d", res.UncompressedBytes, len(rawContent))
	}
	if res.Checksum != expectedCRC {
		t.Fatalf("Checksum mismatch: got 0x%08x, want 0x%08x", res.Checksum, expectedCRC)
	}
	if res.CompressedBytes != int64(outputBuf.Len()) {
		t.Fatalf("CompressedBytes mismatch: got %d, want %d", res.CompressedBytes, outputBuf.Len())
	}

	// Verify decryption and decompression unwrapping
	aesReader, err := crypto.NewAESCTRReader(&outputBuf, "PipelineSecret123", int64(outputBuf.Len()), 32, 16)
	if err != nil {
		t.Fatalf("NewAESCTRReader failed: %v", err)
	}
	decomp, err := codec.DefaultRegistry.Decompressor(codec.Deflate)
	if err != nil {
		t.Fatalf("Decompressor failed: %v", err)
	}

	decompReader, err := decomp.Decompress(aesReader)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	defer decompReader.Close()

	var decompBuf bytes.Buffer
	if _, err := io.Copy(&decompBuf, decompReader); err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}

	if decompBuf.String() != string(rawContent) {
		t.Fatalf("recovered content mismatch: got %q, want %q", decompBuf.String(), string(rawContent))
	}
}
