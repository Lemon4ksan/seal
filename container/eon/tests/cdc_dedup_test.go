// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/cdc"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func TestCDC_DeduplicationAndRoundtrip(t *testing.T) {
	// Base data of 3 MiB
	baseData := make([]byte, 3*1024*1024)
	if _, err := rand.Read(baseData); err != nil {
		t.Fatal(err)
	}

	// File 1: original base data
	file1 := make([]byte, len(baseData))
	copy(file1, baseData)

	// File 2: shifted with 200-byte prefix insertion
	file2 := make([]byte, 200+len(baseData))
	copy(file2[0:200], []byte("TEST_PREFIX_CDC_DEDUPLICATION_REUSED_CHUNKS_SHIFT_INSERTION_MARKER_BYTE_STREAM_TEST_ABC_123_456_789_000_111_222"))
	copy(file2[200:], baseData)

	buf := new(bytes.Buffer)
	cdcOpts := cdc.Options{
		MinSize: 32 * 1024,
		AvgSize: 128 * 1024,
		MaxSize: 512 * 1024,
	}

	w := engine.NewWriter(buf, engine.WithCDCOptions(cdcOpts))

	if err := w.AddFile("file1.bin", file1); err != nil {
		t.Fatalf("AddFile file1 failed: %v", err)
	}
	if err := w.AddFile("file2.bin", file2); err != nil {
		t.Fatalf("AddFile file2 failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	reused, saved := w.DeduplicationStats()
	t.Logf("CDC Deduplication Results: %d chunks reused, %d bytes saved", reused, saved)

	if reused == 0 {
		t.Fatalf("expected at least 1 reused chunk, got 0")
	}
	if saved < 1*1024*1024 {
		t.Fatalf("expected at least 1 MiB saved, got %d bytes", saved)
	}

	// Open and verify roundtrip extraction
	r, err := engine.OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	rep := r.Diagnose(false)
	if !rep.OK() {
		t.Fatalf("Diagnose failed: %+v", rep)
	}

	// Read file1
	out1, err := r.ReadExtent("file1.bin")
	if err != nil {
		t.Fatalf("ReadExtent file1 failed: %v", err)
	}
	if !bytes.Equal(file1, out1) {
		t.Fatalf("file1 content mismatch (len %d vs %d)", len(file1), len(out1))
	}

	// Read file2
	out2, err := r.ReadExtent("file2.bin")
	if err != nil {
		t.Fatalf("ReadExtent file2 failed: %v", err)
	}
	if !bytes.Equal(file2, out2) {
		t.Fatalf("file2 content mismatch (len %d vs %d)", len(file2), len(out2))
	}

	// Check if file2 has non-contiguous chunk list
	ext2, ok := r.FindExtent(spec.NamespaceVFS, "file2.bin")
	if !ok {
		t.Fatalf("FindExtent file2.bin not found")
	}
	chunkList := ext2.GetChunkList()
	t.Logf("file2 chunk list: %v (ChunkCount: %d, FlagChunkList: %v)",
		chunkList, ext2.ChunkCount, (ext2.ExtentFlags&spec.ExtentFlagChunkList) != 0)

	if len(chunkList) < 2 {
		t.Fatalf("expected file2 to span multiple chunks, got %d", len(chunkList))
	}
}
