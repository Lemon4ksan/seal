// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/codec/lzma"
	"github.com/lemon4ksan/seal/file"
)

func TestSevenZip_BCJ_Filter_Roundtrip(t *testing.T) {
	// Simulated x86 binary code containing multiple CALL / JMP instructions (0xE8, 0xE9)
	var x86Code bytes.Buffer
	for i := 0; i < 2000; i++ {
		// x86 CALL offset instruction
		x86Code.Write([]byte{0x55, 0x89, 0xE5, 0xE8, 0x12, 0x34, 0x00, 0x00, 0x5D, 0xC3})
		x86Code.Write([]byte{0x90, 0x90, 0xE9, 0x56, 0x78, 0x00, 0x00, 0x90, 0x90, 0x90})
	}
	rawBinary := x86Code.Bytes()

	f1, err := file.New("program.exe", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.SetSizes(int64(len(rawBinary)), 0, 0)
	f1.WithModTime(time.Now())
	f1.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(rawBinary)), nil
	})

	var ws inMemoryWriteSeeker
	writer := NewWriterWithOptions(&ws, WriterOptions{
		LZMA:   lzma.OptionsForLevel(lzma.LevelFastest),
		Filter: FilterBCJ,
		Method: MethodLZMA2Default,
	})

	n, err := writer.WriteArchive([]*file.File{f1})
	if err != nil {
		t.Fatalf("WriteArchive with BCJ failed: %v", err)
	}
	if n <= 0 {
		t.Fatalf("WriteArchive returned invalid bytes count: %d", n)
	}

	// Decompress and verify
	reader, err := NewReader(bytes.NewReader(ws.buf), int64(len(ws.buf)), "")
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	entries := reader.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	rc, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Entry.Open failed: %v", err)
	}
	defer rc.Close()

	decompressed, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(decompressed, rawBinary) {
		t.Fatalf("Decompressed binary does not match original! Got len %d, want %d", len(decompressed), len(rawBinary))
	}
}

func TestSevenZip_PPMd_Method_Roundtrip(t *testing.T) {
	sampleText := []byte("The quick brown fox jumps over the lazy dog. " +
		"PPMd (Prediction by Partial Matching) is an entropy compression algorithm " +
		"that yields world-class compression ratios on source code, logs, and structured text documents. " +
		"Seal implements native pure-Go PPMd model encoding and decoding!")

	f1, err := file.New("document.txt", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.SetSizes(int64(len(sampleText)), 0, 0)
	f1.WithModTime(time.Now())
	f1.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(sampleText)), nil
	})

	var ws inMemoryWriteSeeker
	writer := NewWriterWithOptions(&ws, WriterOptions{
		Method:    MethodPPMDDefault,
		PPMdOrder: 6,
		PPMdMem:   16 * 1024 * 1024,
	})

	n, err := writer.WriteArchive([]*file.File{f1})
	if err != nil {
		t.Fatalf("WriteArchive with PPMd failed: %v", err)
	}
	if n <= 0 {
		t.Fatalf("WriteArchive returned invalid bytes count: %d", n)
	}

	// Decompress and verify
	reader, err := NewReader(bytes.NewReader(ws.buf), int64(len(ws.buf)), "")
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	entries := reader.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	rc, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Entry.Open failed: %v", err)
	}
	defer rc.Close()

	decompressed, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(decompressed, sampleText) {
		t.Fatalf("Decompressed text does not match original! Got %q, want %q", string(decompressed), string(sampleText))
	}
}
