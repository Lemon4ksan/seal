// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dcl_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec/legacy/dcl"
)

func TestDCL_Empty(t *testing.T) {
	r := dcl.NewReader(bytes.NewReader(nil))
	defer r.Close()

	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected EOF, got n=%d, err=%v", n, err)
	}
}

func TestDCL_BlastVector(t *testing.T) {
	// Official test vector from Mark Adler's blast.c / PKWARE DCL specification
	// Decompresses to "AIAIAIAIAIAIA"
	data := []byte{0x00, 0x04, 0x82, 0x24, 0x25, 0x8f, 0x80, 0x7f}

	r := dcl.NewReader(bytes.NewReader(data))
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	expected := "AIAIAIAIAIAIA"
	if string(out) != expected {
		t.Fatalf("DCL mismatch:\ngot:  %q\nwant: %q", string(out), expected)
	}
}
