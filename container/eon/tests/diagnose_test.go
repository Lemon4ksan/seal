// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/engine"
)

func TestDiagnose_CleanArchive(t *testing.T) {
	buf := new(bytes.Buffer)
	w := engine.NewWriter(buf)
	if err := w.AddFile("hello.txt", []byte("Hello, diagnostics!")); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	r, err := engine.OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	// Quick check
	repQuick := r.Diagnose(true)
	if !repQuick.OK() {
		t.Fatalf("quick diagnose failed on clean archive")
	}
	if repQuick.TotalChunks != 1 || repQuick.ValidChunks != 1 {
		t.Fatalf("unexpected chunk counts: %+v", repQuick)
	}

	// Deep check
	repDeep := r.Diagnose(false)
	if !repDeep.OK() {
		t.Fatalf("deep diagnose failed on clean archive")
	}
	if repDeep.TotalChunks != 1 || repDeep.ValidChunks != 1 || repDeep.TotalExtents != 1 || repDeep.ValidExtents != 1 {
		t.Fatalf("unexpected counts: %+v", repDeep)
	}
}

func TestDiagnose_CorruptedChunkPayload(t *testing.T) {
	buf := new(bytes.Buffer)
	w := engine.NewWriter(buf)
	if err := w.AddFile("data1.txt", []byte("Crucial payload data 1")); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := w.AddFile("data2.txt", []byte("Crucial payload data 2")); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data := buf.Bytes()
	// Chunk 0 starts at offset 4096. Chunk Header is 64 bytes.
	// Compressed payload is at 4096 + 64.
	corruptOffset := 4096 + 64 + 2
	if corruptOffset >= len(data) {
		t.Fatalf("archive size %d too small for corruptOffset %d", len(data), corruptOffset)
	}
	data[corruptOffset] ^= 0xFF

	r, err := engine.OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	// Deep check should catch decompression or hash failure
	rep := r.Diagnose(false)
	if rep.OK() {
		t.Fatalf("expected diagnose to fail on corrupted chunk payload")
	}
	if len(rep.CorruptedChunks) != 1 {
		t.Fatalf("expected 1 corrupted chunk, got %d", len(rep.CorruptedChunks))
	}
	if rep.CorruptedChunks[0].ChunkIndex != 0 {
		t.Fatalf("expected chunk 0 corrupted, got %d", rep.CorruptedChunks[0].ChunkIndex)
	}
	if len(rep.CorruptedFiles) != 2 {
		t.Fatalf("expected 2 corrupted files pointing to chunk 0, got %d", len(rep.CorruptedFiles))
	}
}

func TestDiagnose_CorruptedChunkMagic(t *testing.T) {
	buf := new(bytes.Buffer)
	w := engine.NewWriter(buf)
	if err := w.AddFile("data.txt", []byte("Testing magic mismatch")); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	data := buf.Bytes()
	// Chunk 0 header starts at 4096. Corrupt first byte of magic 'CHNK'
	data[4096] = 0x00

	r, err := engine.OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	// Even quick check should immediately catch chunk magic mismatch
	rep := r.Diagnose(true)
	if rep.OK() {
		t.Fatalf("expected quick diagnose to fail on corrupted chunk magic")
	}
	if len(rep.CorruptedChunks) != 1 {
		t.Fatalf("expected 1 corrupted chunk, got %d", len(rep.CorruptedChunks))
	}
}
