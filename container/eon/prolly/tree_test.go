// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prolly_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/prolly"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func generateTestExtents(count int) []catalog.Extent {
	extents := make([]catalog.Extent, count)
	for i := 0; i < count; i++ {
		extents[i] = catalog.Extent{
			Namespace:   spec.NamespaceVFS,
			Name:        fmt.Sprintf("dir_%03d/file_%04d.txt", i/50, i),
			ChunkIndex:  uint32(i / 10),
			ChunkCount:  1,
			ChunkOffset: uint64((i % 10) * 1024),
			PayloadSize: 1024,
			Timestamp:   1700000000 + int64(i),
			Hash:        [32]byte{byte(i), byte(i >> 8)},
		}
	}
	idx := &catalog.Index{Extents: extents}
	idx.Sort()
	return idx.Extents
}

func TestProllyTree_Roundtrip(t *testing.T) {
	extents := generateTestExtents(500)
	store := prolly.NewMemoryNodeStore()

	tree, err := prolly.BuildTree(extents, store)
	if err != nil {
		t.Fatalf("BuildTree failed: %v", err)
	}

	if tree.ItemCount != 500 {
		t.Fatalf("expected 500 items, got %d", tree.ItemCount)
	}

	// Verify Lookup for every item
	for _, expected := range extents {
		actual, found, err := tree.Lookup(expected.Namespace, expected.Name)
		if err != nil {
			t.Fatalf("Lookup error for %s: %v", expected.Name, err)
		}
		if !found {
			t.Fatalf("item %s not found in tree", expected.Name)
		}
		if actual.PayloadSize != expected.PayloadSize || actual.ChunkIndex != expected.ChunkIndex {
			t.Fatalf("item %s data mismatch: %+v vs %+v", expected.Name, actual, expected)
		}
	}

	// Non-existent item
	_, found, err := tree.Lookup(spec.NamespaceVFS, "non_existent.txt")
	if err != nil {
		t.Fatalf("Lookup error: %v", err)
	}
	if found {
		t.Fatalf("expected non_existent.txt not to be found")
	}

	// Collect all
	collected, err := tree.CollectExtents()
	if err != nil {
		t.Fatalf("CollectExtents failed: %v", err)
	}
	if len(collected) != len(extents) {
		t.Fatalf("collected count mismatch: %d vs %d", len(collected), len(extents))
	}
}

func TestProllyTree_Diff(t *testing.T) {
	extentsA := generateTestExtents(500)
	store := prolly.NewMemoryNodeStore()

	treeA, err := prolly.BuildTree(extentsA, store)
	if err != nil {
		t.Fatalf("BuildTree A failed: %v", err)
	}

	// Create extents B with:
	// - 1 modified item (file_0010.txt)
	// - 2 added items (dir_999/new_a.txt, dir_999/new_b.txt)
	// - 1 deleted item (file_0020.txt)
	var extentsB []catalog.Extent
	for _, ext := range extentsA {
		if ext.Name == "dir_000/file_0020.txt" {
			continue // Deleted
		}
		if ext.Name == "dir_000/file_0010.txt" {
			extCopy := ext
			extCopy.PayloadSize = 2048 // Modified
			extCopy.Hash[0] ^= 0xFF
			extentsB = append(extentsB, extCopy)
			continue
		}
		extentsB = append(extentsB, ext)
	}

	extentsB = append(extentsB, catalog.Extent{
		Namespace:   spec.NamespaceVFS,
		Name:        "dir_999/new_a.txt",
		PayloadSize: 500,
	})
	extentsB = append(extentsB, catalog.Extent{
		Namespace:   spec.NamespaceVFS,
		Name:        "dir_999/new_b.txt",
		PayloadSize: 600,
	})

	idxB := &catalog.Index{Extents: extentsB}
	idxB.Sort()

	treeB, err := prolly.BuildTree(idxB.Extents, store)
	if err != nil {
		t.Fatalf("BuildTree B failed: %v", err)
	}

	diff, err := prolly.Diff(treeA, treeB)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	if len(diff.Added) != 2 {
		t.Fatalf("expected 2 added, got %d: %+v", len(diff.Added), diff.Added)
	}
	if len(diff.Modified) != 1 {
		t.Fatalf("expected 1 modified, got %d: %+v", len(diff.Modified), diff.Modified)
	}
	if len(diff.Deleted) != 1 {
		t.Fatalf("expected 1 deleted, got %d: %+v", len(diff.Deleted), diff.Deleted)
	}

	if diff.Modified[0].Old.Name != "dir_000/file_0010.txt" {
		t.Fatalf("expected modified file_0010.txt, got %s", diff.Modified[0].Old.Name)
	}
	if diff.Deleted[0].Name != "dir_000/file_0020.txt" {
		t.Fatalf("expected deleted file_0020.txt, got %s", diff.Deleted[0].Name)
	}
}

func TestSnapshotTable_Roundtrip(t *testing.T) {
	st := prolly.NewSnapshotTable()
	st.Add(prolly.Snapshot{
		ID:         "snap-001",
		ParentID:   "",
		Timestamp:  time.Now().UnixNano(),
		Message:    "Initial import",
		RootHash:   [32]byte{1, 2, 3},
		FileCount:  100,
		TotalBytes: 50000,
	})
	st.Add(prolly.Snapshot{
		ID:         "snap-002",
		ParentID:   "snap-001",
		Timestamp:  time.Now().UnixNano(),
		Message:    "Update docs",
		RootHash:   [32]byte{4, 5, 6},
		FileCount:  102,
		TotalBytes: 52000,
	})

	var buf bytes.Buffer
	if _, err := st.Write(&buf); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	parsed, err := prolly.ReadSnapshotTable(&buf)
	if err != nil {
		t.Fatalf("ReadSnapshotTable failed: %v", err)
	}

	if len(parsed.Snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(parsed.Snapshots))
	}

	latest := parsed.Latest()
	if latest.ID != "snap-002" || latest.Message != "Update docs" {
		t.Fatalf("unexpected latest: %+v", latest)
	}

	s1, ok := parsed.Find("snap-001")
	if !ok || s1.Message != "Initial import" {
		t.Fatalf("find snap-001 failed: %+v", s1)
	}
}
