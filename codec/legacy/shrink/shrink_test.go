// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package shrink_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec/legacy/shrink"
)

func TestShrink_Empty(t *testing.T) {
	r := shrink.NewReader(bytes.NewReader(nil))
	defer r.Close()

	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if !errors.Is(err, io.EOF) || n != 0 {
		t.Errorf("expected EOF, got n=%d, err=%v", n, err)
	}
}

func TestShrink_LiteralStream(t *testing.T) {
	// 9-bit LSB encoded characters 'A' (0x41) and 'B' (0x42)
	// 'A' = 0x041 (9 bits): 01000001 0
	// 'B' = 0x042 (9 bits): 01000010 0
	// Packed LSB bits:
	// Byte 0: 0x41 (01000001)
	// Byte 1: (0 << 0) | (0x42 << 1) = 0x84 (10000100)
	// Byte 2: 0x00
	data := []byte{0x41, 0x84, 0x00}
	r := shrink.NewReader(bytes.NewReader(data))
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(out) != "AB" {
		t.Errorf("expected 'AB', got %q", string(out))
	}
}
