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

func Test7z_StealthCheckpoint_Roundtrip(t *testing.T) {
	t.Parallel()
	checkpointPayload := []byte(`{"magic":"SEAL_CHECKPOINT_V1","session_id":"sess-999","done_files":1,"total_files":2}`)

	f1, err := file.New("hello.txt", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.WithData([]byte("Hello Resumable 7-Zip World!"))

	var buf bytes.Buffer
	w := sevenzip.NewWriterWithOptions(&buf, sevenzip.WriterOptions{
		Method:         sevenzip.MethodLZMA2Default,
		CheckpointData: checkpointPayload,
	})

	n, err := w.WriteArchive([]*file.File{f1})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}
	if n <= 0 {
		t.Fatalf("Expected positive bytes written, got %d", n)
	}

	archiveBytes := buf.Bytes()

	// 1. Read using Seal's sevenzip.Reader
	rAt := bytes.NewReader(archiveBytes)
	zr, err := sevenzip.NewReader(rAt, int64(len(archiveBytes)), "")
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	cpData, ok := zr.Checkpoint()
	if !ok {
		t.Fatalf("Expected checkpoint to be detected, got false")
	}
	if !bytes.Equal(cpData, checkpointPayload) {
		t.Fatalf("Checkpoint payload mismatch:\ngot  %s\nwant %s", string(cpData), string(checkpointPayload))
	}

	// 2. Verify file extraction
	entries := zr.Entries()
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}
	rc, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = rc.Close() }()

	extracted, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(extracted) != "Hello Resumable 7-Zip World!" {
		t.Fatalf("Extracted mismatch: %q", string(extracted))
	}

	// 3. Cross-validate with official 7z.exe if installed
	sevenZipBin, err := exec.LookPath("7z")
	if err == nil {
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, "test_checkpoint.7z")
		if err := os.WriteFile(archivePath, archiveBytes, 0o644); err != nil {
			t.Fatalf("Failed to write temp archive: %v", err)
		}

		cmd := exec.CommandContext(t.Context(), sevenZipBin, "t", archivePath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Official 7z.exe failed to test archive:\nError: %v\nOutput: %s", err, string(output))
		}
		t.Logf("Official 7z.exe verified stealth checkpoint archive successfully:\n%s", string(output))
	}
}
