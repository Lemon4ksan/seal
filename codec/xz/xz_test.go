// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xz_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec/xz"
)

func TestVLI_Roundtrip(t *testing.T) {
	testValues := []uint64{
		0, 1, 127, 128, 255, 16383, 16384, 65535, 1<<21 - 1, 1 << 32, 1<<63 - 1,
	}

	for _, v := range testValues {
		var buf bytes.Buffer
		n, err := xz.WriteVLI(&buf, v)
		if err != nil {
			t.Fatalf("WriteVLI(%d) failed: %v", v, err)
		}
		if n != xz.VLISize(v) {
			t.Errorf("VLISize mismatch for %d: got %d, want %d", v, n, xz.VLISize(v))
		}

		decoded, err := xz.ReadVLI(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("ReadVLI(%d) failed: %v", v, err)
		}
		if decoded != v {
			t.Errorf("VLI mismatch: got %d, want %d", decoded, v)
		}
	}
}

func TestXZ_Roundtrip_Plain(t *testing.T) {
	data := []byte("The quick brown fox jumps over the lazy dog! XZ compression stream verification test.")

	var compBuf bytes.Buffer
	w, err := xz.NewWriter(&compBuf)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	if _, err := w.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Decompress
	r, err := xz.NewReader(bytes.NewReader(compBuf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	defer r.Close()

	decomp, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(decomp, data) {
		t.Fatalf("data mismatch:\ngot:  %q\nwant: %q", string(decomp), string(data))
	}
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
	if err != nil {
		t.Fatalf("NewWriterOptions failed: %v", err)
	}

	if _, err := w.Write(data); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Decompress
	r, err := xz.NewReader(bytes.NewReader(compBuf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	defer r.Close()

	decomp, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(decomp, data) {
		t.Fatalf("BCJ data mismatch")
	}
}

func TestXZ_InvalidHeader(t *testing.T) {
	invalid := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	_, err := xz.NewReader(bytes.NewReader(invalid))
	if err == nil {
		t.Errorf("expected error on invalid magic, got nil")
	}
}
