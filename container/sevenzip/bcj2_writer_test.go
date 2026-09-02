// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal/container/sevenzip"
	"github.com/lemon4ksan/seal/file"
)

func TestSevenZip_BCJ2_Roundtrip_And_CrossValidation(t *testing.T) {
	// Generate mock x86 binary code with CALL (E8) and JMP (E9) instructions
	original := make([]byte, 32*1024)
	for i := range original {
		original[i] = byte(i & 0xFF)
	}

	for i := 100; i < len(original)-64; i += 64 {
		original[i] = 0xE8
		original[i+1] = byte(i)
		original[i+2] = byte(i >> 8)
		original[i+3] = 0x00
		original[i+4] = 0x00

		original[i+32] = 0xE9
		original[i+33] = byte(i + 10)
		original[i+34] = byte((i + 10) >> 8)
		original[i+35] = 0x00
		original[i+36] = 0x00
	}

	f1, err := file.New("program.exe", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.WithData(original)

	// Write 7z archive with FilterBCJ2
	var buf bytes.Buffer
	writer := sevenzip.NewWriterWithOptions(&buf, sevenzip.WriterOptions{
		Filter: sevenzip.FilterBCJ2,
		Method: sevenzip.MethodLZMA2Default,
	})

	written, err := writer.WriteArchive([]*file.File{f1})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}
	if written <= 0 {
		t.Fatalf("Expected positive bytes written, got %d", written)
	}

	// 1. Cross-validate with official 7-Zip CLI if installed
	sevenZipPath := `C:\Program Files\7-Zip\7z.exe`
	if _, err := os.Stat(sevenZipPath); err == nil {
		tempDir := t.TempDir()
		archivePath := filepath.Join(tempDir, "seal_bcj2.7z")
		if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
			t.Fatalf("Saving test archive: %v", err)
		}

		// Run 7z test command
		cmdTest := exec.Command(sevenZipPath, "t", archivePath)
		out, err := cmdTest.CombinedOutput()
		if err != nil {
			t.Fatalf("7-Zip test command failed on Seal BCJ2 archive:\n%s", string(out))
		}

		// Run 7z extract command
		extractDir := filepath.Join(tempDir, "extracted")
		cmdExtract := exec.Command(sevenZipPath, "x", archivePath, "-o"+extractDir, "-y")
		out, err = cmdExtract.CombinedOutput()
		if err != nil {
			t.Fatalf("7-Zip extract command failed on Seal BCJ2 archive:\n%s", string(out))
		}

		content7z, err := os.ReadFile(filepath.Join(extractDir, "program.exe"))
		if err != nil {
			t.Fatalf("Reading 7z extracted file: %v", err)
		}
		if !bytes.Equal(content7z, original) {
			t.Fatalf(
				"7z extracted content mismatch with original: got %d bytes, want %d",
				len(content7z),
				len(original),
			)
		}
	}

	// 2. Verify with Seal Reader
	rAt := bytes.NewReader(buf.Bytes())
	reader, err := sevenzip.NewReader(rAt, int64(buf.Len()), "")
	if err != nil {
		t.Fatalf("Seal NewReader failed on BCJ2 archive: %v", err)
	}

	entries := reader.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 file entry, got %d", len(entries))
	}

	rc, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Seal Open failed on BCJ2 archive: %v", err)
	}
	extracted, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("Reading extracted BCJ2 entry: %v", err)
	}

	if !bytes.Equal(extracted, original) {
		t.Fatalf("Seal BCJ2 extracted content mismatch: got %d bytes, want %d bytes", len(extracted), len(original))
	}
}
