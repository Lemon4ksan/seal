// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	archivezip "archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/file"
)

func TestZip_StealthCheckpoint_Roundtrip(t *testing.T) {
	t.Parallel()
	checkpointPayload := []byte(
		`{"magic":"SEAL_CHECKPOINT_V1","session_id":"zip-sess-777","done_files":1,"total_files":2}`,
	)

	f1, err := file.New("notes.txt", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	f1.WithData([]byte("Hello Resumable ZIP World!"))

	eng := zip.New()

	var buf bytes.Buffer
	cfg := container.Config{
		CheckpointData: checkpointPayload,
	}

	n, err := eng.WriteArchive(&buf, []*file.File{f1}, cfg)
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}
	if n <= 0 {
		t.Fatalf("Expected positive bytes written, got %d", n)
	}

	archiveBytes := buf.Bytes()

	// 1. Verify standard Go archive/zip reader considers it 100% valid
	stdZipReader, err := archivezip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		t.Fatalf("Standard archive/zip failed to open archive: %v", err)
	}
	if len(stdZipReader.File) != 1 {
		t.Fatalf("Expected 1 file in standard reader, got %d", len(stdZipReader.File))
	}
	stdFile, err := stdZipReader.File[0].Open()
	if err != nil {
		t.Fatalf("Standard open failed: %v", err)
	}
	stdContent, _ := io.ReadAll(stdFile)
	_ = stdFile.Close()
	if string(stdContent) != "Hello Resumable ZIP World!" {
		t.Fatalf("Standard content mismatch: %s", string(stdContent))
	}

	// 2. Read using Seal's zip.Reader
	rAt := bytes.NewReader(archiveBytes)
	zr, err := zip.NewReader(rAt, int64(len(archiveBytes)), "", nil)
	if err != nil {
		t.Fatalf("zip.NewReader failed: %v", err)
	}

	cpData, ok := zr.Checkpoint()
	if !ok {
		t.Fatalf("Expected checkpoint to be detected, got false")
	}
	if !bytes.Equal(cpData, checkpointPayload) {
		t.Fatalf("Checkpoint mismatch:\ngot  %s\nwant %s", string(cpData), string(checkpointPayload))
	}
}
