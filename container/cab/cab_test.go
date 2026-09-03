// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cab_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/cab"
	"github.com/lemon4ksan/seal/file"
)

func TestCAB_Roundtrip_MSZip(t *testing.T) {
	t.Parallel()

	f1, err := file.New("documents/readme.txt", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.WithData([]byte("Welcome to Seal CAB engine!\n"))
	f1.WithModTime(time.Date(2026, 9, 3, 14, 30, 0, 0, time.UTC))

	f2, err := file.New("config.json", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f2.WithData([]byte(`{"engine":"seal","format":"cab"}`))

	files := []*file.File{f1, f2}

	eng := cab.New()
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
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	// Verify entries and data
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
		case "documents/readme.txt":
			if string(data) != "Welcome to Seal CAB engine!\n" {
				t.Fatalf("Data mismatch for %q: got %q", entry.Name(), string(data))
			}
			if entry.ModTime().Year() != 2026 {
				t.Fatalf("ModTime year mismatch: got %v", entry.ModTime())
			}
		case "config.json":
			if string(data) != `{"engine":"seal","format":"cab"}` {
				t.Fatalf("Data mismatch for %q: got %q", entry.Name(), string(data))
			}
		default:
			t.Fatalf("Unexpected entry: %q", entry.Name())
		}
	}
}

func TestCAB_Roundtrip_Store(t *testing.T) {
	t.Parallel()

	f, err := file.New("plain.txt", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f.WithData([]byte("Uncompressed plain data inside Cabinet"))

	eng := cab.New()
	var buf bytes.Buffer
	_, err = eng.WriteArchive(&buf, []*file.File{f}, container.Config{DefaultMethod: codec.Store})
	if err != nil {
		t.Fatalf("WriteArchive Store failed: %v", err)
	}

	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	if err != nil {
		t.Fatalf("OpenCatalog failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	r, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	data, _ := io.ReadAll(r)
	_ = r.Close()

	if string(data) != "Uncompressed plain data inside Cabinet" {
		t.Fatalf("Data mismatch: got %q", string(data))
	}
}

func TestCAB_MultiBlock_Spanning(t *testing.T) {
	t.Parallel()

	// Create payload larger than 32KB block (e.g. 100 KB of repetitive text)
	largeData := bytes.Repeat([]byte("Microsoft Cabinet 32KB chunk boundary test. "), 2500) // ~110 KB

	f, err := file.New("large.bin", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f.WithData(largeData)

	eng := cab.New()
	var buf bytes.Buffer
	_, err = eng.WriteArchive(&buf, []*file.File{f}, container.Config{})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}

	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	if err != nil {
		t.Fatalf("OpenCatalog failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	r, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	data, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(data, largeData) {
		t.Fatalf("Multi-block decompressed data mismatch (size got %d, want %d)", len(data), len(largeData))
	}
}

func TestCAB_CorruptedSignature(t *testing.T) {
	t.Parallel()

	invalidBytes := []byte("INVALID_HEADER_DATA_NOT_MSCF_HEADER")
	eng := cab.New()
	_, _, err := eng.OpenCatalog(bytes.NewReader(invalidBytes), int64(len(invalidBytes)), "", nil)
	if !errors.Is(err, cab.ErrInvalidSignature) {
		t.Fatalf("Expected ErrInvalidSignature, got: %v", err)
	}
}
