// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpio_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/cpio"
	"github.com/lemon4ksan/seal/file"
)

func TestCPIO_Roundtrip_Newc(t *testing.T) {
	t.Parallel()

	f1, err := file.New("init", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.WithData([]byte("#!/bin/sh\necho Booting Linux\nexec /bin/sh\n"))
	f1.WithModTime(time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC))

	f2, err := file.New("etc/hostname", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f2.WithData([]byte("seal-box\n"))

	files := []*file.File{f1, f2}

	eng := cpio.New()
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, files, container.Config{})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}
	if written != int64(buf.Len()) {
		t.Fatalf("Size mismatch: written=%d, buf.Len=%d", written, buf.Len())
	}

	// 1. Parse via OpenCatalog
	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	if err != nil {
		t.Fatalf("OpenCatalog failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	foundInit := false
	foundHost := false

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
		case "init":
			foundInit = true
			if string(data) != "#!/bin/sh\necho Booting Linux\nexec /bin/sh\n" {
				t.Fatalf("Data mismatch for init: %q", string(data))
			}
			if entry.ModTime().Year() != 2026 {
				t.Fatalf("ModTime mismatch: got %v", entry.ModTime())
			}
		case "etc/hostname":
			foundHost = true
			if string(data) != "seal-box\n" {
				t.Fatalf("Data mismatch for etc/hostname: %q", string(data))
			}
		}
	}

	if !foundInit || !foundHost {
		t.Fatalf("Missing entries: init=%v, hostname=%v", foundInit, foundHost)
	}

	// 2. Parse via ScanStream (pipes / non-seekable streams)
	streamEntries, _, err := eng.ScanStream(bytes.NewReader(buf.Bytes()), "", nil)
	if err != nil {
		t.Fatalf("ScanStream failed: %v", err)
	}
	if len(streamEntries) != 2 {
		t.Fatalf("ScanStream expected 2 entries, got %d", len(streamEntries))
	}
}

func TestCPIO_Padding_Alignment(t *testing.T) {
	t.Parallel()

	// Test various payload lengths to ensure 4-byte padding works correctly
	for length := 1; length <= 16; length++ {
		t.Run(fmt.Sprintf("Length_%d", length), func(t *testing.T) {
			t.Parallel()

			payload := bytes.Repeat([]byte("A"), length)
			f, _ := file.New(fmt.Sprintf("file_%d.txt", length), false)
			f.WithData(payload)

			eng := cpio.New()
			var buf bytes.Buffer
			_, err := eng.WriteArchive(&buf, []*file.File{f}, container.Config{})
			if err != nil {
				t.Fatalf("WriteArchive failed for length %d: %v", length, err)
			}

			entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
			if err != nil {
				t.Fatalf("OpenCatalog failed for length %d: %v", length, err)
			}
			if len(entries) != 1 {
				t.Fatalf("Expected 1 entry, got %d", len(entries))
			}

			r, _ := entries[0].Open()
			data, _ := io.ReadAll(r)
			_ = r.Close()

			if !bytes.Equal(data, payload) {
				t.Fatalf("Payload mismatch for length %d", length)
			}
		})
	}
}

func TestCPIO_CorruptedMagic(t *testing.T) {
	t.Parallel()

	corrupted := []byte("000000INVALID_CPIO_DATA_NOT_MAGIC")
	eng := cpio.New()
	_, _, err := eng.OpenCatalog(bytes.NewReader(corrupted), int64(len(corrupted)), "", nil)
	if !errors.Is(err, cpio.ErrInvalidMagic) {
		t.Fatalf("Expected ErrInvalidMagic, got: %v", err)
	}
}
