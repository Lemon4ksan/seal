// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eon

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func TestEONRoundtrip(t *testing.T) {
	// Prepare test files
	f1, err := file.New("hello.txt", false)
	if err != nil {
		t.Fatalf("file.New hello.txt: %v", err)
	}
	f1Content := []byte("Hello, EON Container Specification v1.0!")
	f1.WithData(f1Content)
	f1.WithModTime(time.Date(2026, 9, 3, 21, 0, 0, 0, time.UTC))

	f2, err := file.New("data/binary.bin", false)
	if err != nil {
		t.Fatalf("file.New data/binary.bin: %v", err)
	}
	f2Content := make([]byte, 128*1024) // 128 KiB
	for i := range f2Content {
		f2Content[i] = byte(i % 251)
	}
	f2.WithData(f2Content)

	dir, err := file.New("data/", true)
	if err != nil {
		t.Fatalf("file.New dir: %v", err)
	}

	files := []*file.File{dir, f1, f2}

	// 1. Serialize archive
	var buf bytes.Buffer
	w := NewWriter(&buf)
	written, err := w.WriteArchive(files, container.Config{})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}

	archiveBytes := buf.Bytes()
	if int64(len(archiveBytes)) != written {
		t.Fatalf("written length mismatch: expected %d, got %d", written, len(archiveBytes))
	}

	// 2. Verify binary mirror symmetry
	if len(archiveBytes) < HeaderSize+TrailerSize {
		t.Fatalf("archive too small: %d bytes", len(archiveBytes))
	}
	// Check Header Magic ('EON!')
	if string(archiveBytes[0:4]) != "EON!" {
		t.Fatalf("header magic mismatch: expected 'EON!', got %q", string(archiveBytes[0:4]))
	}
	// Check Trailer Magic ('!EON')
	trailerStart := len(archiveBytes) - TrailerSize
	trailerMagic := string(archiveBytes[len(archiveBytes)-4:])
	if trailerMagic != "!EON" {
		t.Fatalf("trailer magic mismatch: expected '!EON', got %q", trailerMagic)
	}
	_ = trailerStart

	// 3. Test OpenCatalog (Random Access)
	rAt := bytes.NewReader(archiveBytes)
	reader, err := NewReader(rAt, int64(len(archiveBytes)))
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	entries := reader.Entries()
	if len(entries) != 3 {
		t.Fatalf("entries count mismatch: expected 3, got %d", len(entries))
	}

	entryMap := make(map[string]*file.File)
	for _, entry := range entries {
		entryMap[entry.Name()] = entry
	}

	// Verify hello.txt
	helloEntry, ok := entryMap["hello.txt"]
	if !ok {
		t.Fatalf("missing hello.txt entry")
	}
	if helloEntry.UncompressedSize() != int64(len(f1Content)) {
		t.Fatalf("hello.txt size mismatch: expected %d, got %d", len(f1Content), helloEntry.UncompressedSize())
	}
	rc, err := helloEntry.Open()
	if err != nil {
		t.Fatalf("open hello.txt: %v", err)
	}
	gotF1, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatalf("read hello.txt: %v", err)
	}
	if !bytes.Equal(gotF1, f1Content) {
		t.Fatalf("hello.txt content mismatch")
	}

	// Verify binary.bin
	binEntry, ok := entryMap["data/binary.bin"]
	if !ok {
		t.Fatalf("missing data/binary.bin entry")
	}
	if binEntry.UncompressedSize() != int64(len(f2Content)) {
		t.Fatalf("binary.bin size mismatch: expected %d, got %d", len(f2Content), binEntry.UncompressedSize())
	}
	rc2, err := binEntry.Open()
	if err != nil {
		t.Fatalf("open binary.bin: %v", err)
	}
	gotF2, err := io.ReadAll(rc2)
	rc2.Close()
	if err != nil {
		t.Fatalf("read binary.bin: %v", err)
	}
	if !bytes.Equal(gotF2, f2Content) {
		t.Fatalf("binary.bin content mismatch")
	}
}

func TestEONContainerEngine(t *testing.T) {
	eng, err := container.Get(container.Eon)
	if err != nil {
		t.Fatalf("container.Get(Eon): %v", err)
	}
	if eng.Type() != container.Eon {
		t.Fatalf("expected container.Eon, got %v", eng.Type())
	}

	// Test Empty Archive
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, nil, container.Config{})
	if err != nil {
		t.Fatalf("write empty archive: %v", err)
	}
	if written < HeaderSize+TrailerSize {
		t.Fatalf("empty archive too small: %d", written)
	}

	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	if err != nil {
		t.Fatalf("open empty catalog: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestEONDetect(t *testing.T) {
	if container.DetectExtension("archive.eon") != container.Eon {
		t.Fatalf("expected DetectExtension archive.eon -> Eon")
	}
	if container.DetectExtension("GAME.EON") != container.Eon {
		t.Fatalf("expected DetectExtension GAME.EON -> Eon")
	}

	magic := []byte{'E', 'O', 'N', '!'}
	if container.SniffHeader(magic) != container.Eon {
		t.Fatalf("expected SniffHeader EON! -> Eon")
	}
}

func TestEONFileSaveTo(t *testing.T) {
	tmpDir := t.TempDir()

	f, _ := file.New("nested/sub/hello.txt", false)
	f.WithData([]byte("SaveTo works natively with EON!"))

	var buf bytes.Buffer
	w := NewWriter(&buf)
	_, err := w.WriteArchive([]*file.File{f}, container.Config{})
	if err != nil {
		t.Fatalf("write archive: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}

	entries := r.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	outPath := filepath.Join(tmpDir, "extracted.txt")
	if err := entries[0].SaveTo(outPath); err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}

	readBack, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(readBack) != "SaveTo works natively with EON!" {
		t.Fatalf("extracted content mismatch: %q", string(readBack))
	}
}
