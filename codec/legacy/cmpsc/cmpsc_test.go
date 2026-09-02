// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmpsc_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/codec/legacy/cmpsc"
)

func TestCMPSC_Roundtrip_Text(t *testing.T) {
	input := []byte("IBM z/OS Enterprise Systems Architecture ESA/390 CMPSC Hardware Compression Test Vector 2026.")

	buf := new(bytes.Buffer)
	w := cmpsc.NewWriter(buf)
	if _, err := w.Write(input); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	r, err := cmpsc.NewReader(buf)
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	decompressed, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(decompressed, input) {
		t.Fatalf("mismatch: got %q, want %q", string(decompressed), string(input))
	}
}

func TestCMPSC_Levels(t *testing.T) {
	input := bytes.Repeat([]byte("MAINFRAME_VSAM_RECORD_DATA_0123456789_EBCDIC_ASCII_"), 50)

	for _, level := range []int{1, 3, 6} {
		buf := new(bytes.Buffer)
		w := cmpsc.NewWriterLevel(buf, level)
		if _, err := w.Write(input); err != nil {
			t.Fatalf("level %d write failed: %v", level, err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("level %d close failed: %v", level, err)
		}

		r, err := cmpsc.NewReader(buf)
		if err != nil {
			t.Fatalf("level %d NewReader failed: %v", level, err)
		}

		out, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("level %d ReadAll failed: %v", level, err)
		}

		if !bytes.Equal(out, input) {
			t.Fatalf("level %d mismatch", level)
		}
	}
}

func TestCMPSC_CodecRegistry(t *testing.T) {
	input := []byte("Testing CMPSC integration through codec.DefaultRegistry Method 16.")

	comp, err := codec.DefaultRegistry.Compressor(codec.IBMCmpsc, 6)
	if err != nil {
		t.Fatalf("Compressor lookup failed: %v", err)
	}

	buf := new(bytes.Buffer)
	_, err = comp.Compress(bytes.NewReader(input), buf)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	decomp, err := codec.DefaultRegistry.Decompressor(codec.IBMCmpsc)
	if err != nil {
		t.Fatalf("Decompressor lookup failed: %v", err)
	}

	rc, err := decomp.Decompress(buf)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	defer rc.Close()

	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(out, input) {
		t.Fatalf("registry mismatch: got %q, want %q", string(out), string(input))
	}
}
