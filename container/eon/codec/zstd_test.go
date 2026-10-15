// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/codec"
)

func TestZstdCodecRoundTrip(t *testing.T) {
	c := codec.NewZstdCodec()

	original := []byte("Hello, EON container world! Testing pure-Go Zstandard compression substrate.")
	compressed, err := c.Compress(original, 3)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	decompressed, err := c.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Fatalf("Decompressed data mismatch: got %q, want %q", decompressed, original)
	}
}

func TestZstdCodecEmpty(t *testing.T) {
	c := codec.NewZstdCodec()

	comp, err := c.Compress(nil, 3)
	if err != nil {
		t.Fatalf("Compress(nil) failed: %v", err)
	}
	if comp != nil {
		t.Fatalf("expected nil compressed for empty input, got %v", comp)
	}

	decomp, err := c.Decompress(nil)
	if err != nil {
		t.Fatalf("Decompress(nil) failed: %v", err)
	}
	if decomp != nil {
		t.Fatalf("expected nil decompressed for empty input, got %v", decomp)
	}
}
