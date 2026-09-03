// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ar_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/ar"
	"github.com/lemon4ksan/seal/file"
)

func TestAR_Roundtrip_DebianPackage(t *testing.T) {
	t.Parallel()

	f1, err := file.New("debian-binary", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.WithData([]byte("2.0\n"))
	f1.WithModTime(time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC))

	f2, err := file.New("control.tar.gz", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f2.WithData([]byte("GZIP_CONTROL_DATA_MOCK"))

	// Long filename (> 15 characters) testing GNU String Table ("//")
	f3, err := file.New("data_archive_payload.tar.xz", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f3.WithData([]byte("XZ_PAYLOAD_DATA_MOCK_ODD_LENGTH!")) // 33 bytes (odd length testing '\n' padding)

	files := []*file.File{f1, f2, f3}

	eng := ar.New()
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, files, container.Config{})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}
	if written != int64(buf.Len()) {
		t.Fatalf("Size mismatch: written=%d, buf.Len=%d", written, buf.Len())
	}

	// Read catalog
	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	if err != nil {
		t.Fatalf("OpenCatalog failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	foundDebianBin := false
	foundControl := false
	foundData := false

	for _, entry := range entries {
		r, err := entry.Open()
		if err != nil {
			t.Fatalf("entry.Open failed for %q: %v", entry.Name(), err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatalf("ReadAll failed for %q: %v", entry.Name(), err)
		}

		switch entry.Name() {
		case "debian-binary":
			foundDebianBin = true
			if string(data) != "2.0\n" {
				t.Fatalf("Data mismatch for debian-binary: %q", string(data))
			}
			if entry.ModTime().Year() != 2026 {
				t.Fatalf("ModTime mismatch: got %v", entry.ModTime())
			}
		case "control.tar.gz":
			foundControl = true
			if string(data) != "GZIP_CONTROL_DATA_MOCK" {
				t.Fatalf("Data mismatch for control.tar.gz: %q", string(data))
			}
		case "data_archive_payload.tar.xz":
			foundData = true
			if string(data) != "XZ_PAYLOAD_DATA_MOCK_ODD_LENGTH!" {
				t.Fatalf("Data mismatch for long filename: %q", string(data))
			}
		}
	}

	if !foundDebianBin || !foundControl || !foundData {
		t.Fatalf("Missing entries: debian-binary=%v, control=%v, data=%v", foundDebianBin, foundControl, foundData)
	}

	// Verify ScanStream
	streamEntries, _, err := eng.ScanStream(bytes.NewReader(buf.Bytes()), "", nil)
	if err != nil {
		t.Fatalf("ScanStream failed: %v", err)
	}
	if len(streamEntries) != 3 {
		t.Fatalf("ScanStream expected 3 entries, got %d", len(streamEntries))
	}
}

func TestAR_CorruptedMagic(t *testing.T) {
	t.Parallel()

	corrupted := []byte("INVALID_NOT_AR_MAGIC_HEADER_DATA_12345678")
	eng := ar.New()
	_, _, err := eng.OpenCatalog(bytes.NewReader(corrupted), int64(len(corrupted)), "", nil)
	if !errors.Is(err, ar.ErrInvalidMagic) {
		t.Fatalf("Expected ErrInvalidMagic, got: %v", err)
	}
}
