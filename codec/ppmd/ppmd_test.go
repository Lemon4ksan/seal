// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppmd

import (
	"bytes"
	"io"
	"testing"
)

func TestPPMd_Roundtrip(t *testing.T) {
	original := []byte("Prediction by Partial Matching PPMd compression engine test in Seal archiver substrate.")

	comp := NewCompressor(DefaultOrder, DefaultMemSize)
	var compressed bytes.Buffer
	n, err := comp.Compress(bytes.NewReader(original), &compressed)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	if n != int64(len(original)) {
		t.Fatalf("Compress size mismatch: got %d, want %d", n, len(original))
	}

	decomp := NewDecompressor(DefaultOrder, DefaultMemSize)
	rc, err := decomp.Decompress(&compressed, uint64(len(original)))
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	defer rc.Close()

	decoded, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(decoded, original) {
		t.Fatalf("PPMd mismatch: got %q, want %q", string(decoded), string(original))
	}
}

func TestPPMd_Properties(t *testing.T) {
	props := Properties{Order: 8, MemSize: 32 * 1024 * 1024, RestorationMethod: 0}
	encoded := props.EncodeProperties()
	parsed, err := ParseProperties(encoded)
	if err != nil {
		t.Fatalf("ParseProperties failed: %v", err)
	}
	if parsed.Order != props.Order || parsed.MemSize != props.MemSize {
		t.Fatalf("Properties mismatch: %+v vs %+v", parsed, props)
	}
}
