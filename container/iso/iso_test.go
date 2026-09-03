// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iso_test

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/iso"
	"github.com/lemon4ksan/seal/file"
)

func TestISO_Roundtrip_Joliet(t *testing.T) {
	t.Parallel()

	f1, err := file.New("boot/grub/grub.cfg", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.WithData([]byte("set default=0\nmenuentry \"Linux\" {}\n"))
	f1.WithModTime(time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC))

	f2, err := file.New("documents/русский_текст.txt", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f2.WithData([]byte("Привет, мир! ISO 9660 с поддержкой Joliet Unicode."))

	files := []*file.File{f1, f2}

	eng := iso.New()
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, files, container.Config{})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}
	if written != int64(buf.Len()) {
		t.Fatalf("Size mismatch: written=%d, buf.Len=%d", written, buf.Len())
	}

	// Must be multiple of 2048
	if buf.Len()%iso.SectorSize != 0 {
		t.Fatalf("ISO image size must be multiple of 2048: %d", buf.Len())
	}

	// Open and inspect catalog
	entries, volID, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	if err != nil {
		t.Fatalf("OpenCatalog failed: %v", err)
	}

	if volID == "" {
		t.Fatalf("Expected non-empty VolumeID")
	}

	foundGrub := false
	foundUnicode := false

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		r, err := entry.Open()
		if err != nil {
			t.Fatalf("Open failed for %q: %v", entry.Name(), err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatalf("ReadAll failed for %q: %v", entry.Name(), err)
		}

		switch entry.Name() {
		case "boot/grub/grub.cfg":
			foundGrub = true
			if string(data) != "set default=0\nmenuentry \"Linux\" {}\n" {
				t.Fatalf("Data mismatch for grub.cfg: %q", string(data))
			}
		case "documents/русский_текст.txt":
			foundUnicode = true
			if string(data) != "Привет, мир! ISO 9660 с поддержкой Joliet Unicode." {
				t.Fatalf("Data mismatch for unicode file: %q", string(data))
			}
		}
	}

	if !foundGrub {
		t.Fatalf("grub.cfg not found in extracted entries")
	}
	if !foundUnicode {
		t.Fatalf("Unicode file not found in extracted entries")
	}
}

func TestISO_CorruptedImage(t *testing.T) {
	t.Parallel()

	shortBytes := make([]byte, 1000)
	eng := iso.New()
	_, _, err := eng.OpenCatalog(bytes.NewReader(shortBytes), int64(len(shortBytes)), "", nil)
	if !errors.Is(err, iso.ErrInvalidISO) {
		t.Fatalf("Expected ErrInvalidISO on truncated data, got: %v", err)
	}
}
