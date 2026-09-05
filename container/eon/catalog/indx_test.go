// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func TestIndex_CanonicalSortAndBinarySearch(t *testing.T) {
	extents := []catalog.Extent{
		{Namespace: spec.NamespaceVFS, Name: "z_file.txt"},
		{Namespace: spec.NamespaceKV, Name: "key_b"},
		{Namespace: spec.NamespaceVFS, Name: "a_file.txt"},
		{Namespace: spec.NamespaceKV, Name: "key_a"},
		{Namespace: spec.NamespaceBlock, Name: "sectors:0-100"},
		{Namespace: spec.NamespaceVFS, Name: "m_file.txt"},
	}

	idx := &catalog.Index{Extents: extents}
	idx.Sort()

	// Verify order:
	// 1. VFS (0x0000): a_file.txt, m_file.txt, z_file.txt
	// 2. Block (0x0001): sectors:0-100
	// 3. KV (0x0003): key_a, key_b
	expected := []struct {
		ns   uint16
		name string
	}{
		{spec.NamespaceVFS, "a_file.txt"},
		{spec.NamespaceVFS, "m_file.txt"},
		{spec.NamespaceVFS, "z_file.txt"},
		{spec.NamespaceBlock, "sectors:0-100"},
		{spec.NamespaceKV, "key_a"},
		{spec.NamespaceKV, "key_b"},
	}

	if len(idx.Extents) != len(expected) {
		t.Fatalf("length mismatch: %d != %d", len(idx.Extents), len(expected))
	}

	for i, exp := range expected {
		if idx.Extents[i].Namespace != exp.ns || idx.Extents[i].Name != exp.name {
			t.Errorf("item %d: expected (%d, %s), got (%d, %s)", i, exp.ns, exp.name, idx.Extents[i].Namespace, idx.Extents[i].Name)
		}
	}

	// Test BinarySearch
	for _, exp := range expected {
		found, ok := idx.BinarySearch(exp.ns, exp.name)
		if !ok {
			t.Errorf("BinarySearch failed to find (%d, %s)", exp.ns, exp.name)
		}
		if found.Name != exp.name {
			t.Errorf("BinarySearch name mismatch: %s != %s", found.Name, exp.name)
		}
	}

	// Test non-existent
	if _, ok := idx.BinarySearch(spec.NamespaceVFS, "not_found.txt"); ok {
		t.Errorf("BinarySearch found non-existent item")
	}
}

func TestIndex_LargeChunkListTagChunkRuns(t *testing.T) {
	// Generate 25,000 chunks composed of 50 contiguous runs
	var originalChunks []uint32
	base := uint32(100)
	for run := 0; run < 50; run++ {
		runLen := uint32(500)
		for c := uint32(0); c < runLen; c++ {
			originalChunks = append(originalChunks, base+c)
		}
		base += runLen + 10 // break contiguity
	}
	if len(originalChunks) != 25000 {
		t.Fatalf("expected 25000 chunks, got %d", len(originalChunks))
	}

	ext := catalog.Extent{
		Namespace:   spec.NamespaceVFS,
		Name:        "large_binary.bin",
		PayloadSize: uint64(len(originalChunks)) * 65536,
	}
	ext.SetChunkList(originalChunks)

	// Verify ExtraTLVs is compact (< 1000 bytes, not 100,000 bytes!)
	if len(ext.ExtraTLVs) > 1000 {
		t.Fatalf("expected ExtraTLVs to be compact, got %d bytes", len(ext.ExtraTLVs))
	}

	// Verify GetChunkList reconstructs exact chunks
	reconstructed := ext.GetChunkList()
	if len(reconstructed) != len(originalChunks) {
		t.Fatalf("reconstructed length mismatch: %d != %d", len(reconstructed), len(originalChunks))
	}
	for i := range originalChunks {
		if reconstructed[i] != originalChunks[i] {
			t.Fatalf("mismatch at chunk %d: expected %d, got %d", i, originalChunks[i], reconstructed[i])
		}
	}

	// Verify INDX serialization roundtrip
	idx := &catalog.Index{Extents: []catalog.Extent{ext}}
	var buf bytes.Buffer
	written, err := idx.Write(&buf)
	if err != nil {
		t.Fatalf("Index.Write failed: %v", err)
	}
	if written <= 0 {
		t.Fatalf("written bytes <= 0: %d", written)
	}

	readIdx, readBytes, err := catalog.ReadIndex(&buf)
	if err != nil {
		t.Fatalf("catalog.ReadIndex failed: %v", err)
	}
	if readBytes != written {
		t.Fatalf("readBytes mismatch: %d != %d", readBytes, written)
	}
	if len(readIdx.Extents) != 1 {
		t.Fatalf("read extents count mismatch: %d", len(readIdx.Extents))
	}

	roundtripChunks := readIdx.Extents[0].GetChunkList()
	if len(roundtripChunks) != len(originalChunks) {
		t.Fatalf("roundtrip chunks count mismatch: %d != %d", len(roundtripChunks), len(originalChunks))
	}
	for i := range originalChunks {
		if roundtripChunks[i] != originalChunks[i] {
			t.Fatalf("roundtrip mismatch at chunk %d: expected %d, got %d", i, originalChunks[i], roundtripChunks[i])
		}
	}
}
