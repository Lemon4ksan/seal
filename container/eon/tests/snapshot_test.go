// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/engine"
)

func TestSnapshot_EndToEndAndDiff(t *testing.T) {
	// Snapshot 1: 20 files
	filesV1 := make(map[string][]byte)
	for i := range 20 {
		filesV1[fmt.Sprintf("docs/page_%02d.txt", i)] = []byte(fmt.Sprintf("Page content v1 for %d", i))
	}

	bufV1 := new(bytes.Buffer)
	w1 := engine.NewWriter(bufV1, engine.WithProllyTree(true), engine.WithSnapshot("Initial documentation", ""))
	for name, data := range filesV1 {
		if err := w1.AddFile(name, data); err != nil {
			t.Fatalf("AddFile %s failed: %v", name, err)
		}
	}
	if err := w1.Close(); err != nil {
		t.Fatalf("Close w1 failed: %v", err)
	}

	r1, err := engine.OpenReader(bytes.NewReader(bufV1.Bytes()), int64(bufV1.Len()))
	if err != nil {
		t.Fatalf("OpenReader v1 failed: %v", err)
	}

	if !r1.HasProllyTree() {
		t.Fatalf("expected r1 to have Prolly Tree")
	}

	snaps1 := r1.Snapshots()
	if len(snaps1) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps1))
	}
	snap1 := snaps1[0]
	t.Logf("Snapshot 1: ID=%s Message=%q Root=%x Files=%d", snap1.ID, snap1.Message, snap1.RootHash[:4], snap1.FileCount)

	// Snapshot 2: Modify page_05.txt, add page_new.txt, delete page_10.txt
	filesV2 := make(map[string][]byte)
	for k, v := range filesV1 {
		if k == "docs/page_10.txt" {
			continue // Deleted
		}
		if k == "docs/page_05.txt" {
			filesV2[k] = []byte("UPDATED PAGE CONTENT V2") // Modified
			continue
		}
		filesV2[k] = v
	}
	filesV2["docs/page_new.txt"] = []byte("BRAND NEW PAGE CONTENT") // Added

	bufV2 := new(bytes.Buffer)
	w2 := engine.NewWriter(bufV2, engine.WithSnapshotsFrom(r1), engine.WithSnapshot("Update page 5 and add page new", snap1.ID))
	for name, data := range filesV2 {
		if err := w2.AddFile(name, data); err != nil {
			t.Fatalf("AddFile %s failed: %v", name, err)
		}
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("Close w2 failed: %v", err)
	}

	r2, err := engine.OpenReader(bytes.NewReader(bufV2.Bytes()), int64(bufV2.Len()))
	if err != nil {
		t.Fatalf("OpenReader v2 failed: %v", err)
	}
	snap2 := r2.LatestSnapshot()
	t.Logf("Snapshot 2: ID=%s Message=%q Root=%x Files=%d", snap2.ID, snap2.Message, snap2.RootHash[:4], snap2.FileCount)

	// Test in-container diff between Snapshot 1 and Snapshot 2
	diff, err := r2.Diff(snap1.ID, snap2.ID)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	// Also test DiffAcross between the two separate readers
	diffCross, err := engine.DiffAcross(r1, snap1.ID, r2, snap2.ID)
	if err != nil {
		t.Fatalf("DiffAcross failed: %v", err)
	}
	if len(diffCross.Added) != 1 || len(diffCross.Modified) != 1 || len(diffCross.Deleted) != 1 {
		t.Fatalf("unexpected DiffAcross result: %+v", diffCross)
	}

	t.Logf("Diff result: Added=%d, Modified=%d, Deleted=%d", len(diff.Added), len(diff.Modified), len(diff.Deleted))

	if len(diff.Added) != 1 || diff.Added[0].Name != "docs/page_new.txt" {
		t.Fatalf("expected 1 added docs/page_new.txt, got %+v", diff.Added)
	}
	if len(diff.Modified) != 1 || diff.Modified[0].Old.Name != "docs/page_05.txt" {
		t.Fatalf("expected 1 modified docs/page_05.txt, got %+v", diff.Modified)
	}
	if len(diff.Deleted) != 1 || diff.Deleted[0].Name != "docs/page_10.txt" {
		t.Fatalf("expected 1 deleted docs/page_10.txt, got %+v", diff.Deleted)
	}

	// Verify reading from snapshot view
	snapView1, err := r1.OpenSnapshot(snap1.ID)
	if err != nil {
		t.Fatalf("OpenSnapshot 1 failed: %v", err)
	}
	dataV1, err := snapView1.ReadExtent("docs/page_05.txt")
	if err != nil || string(dataV1) != "Page content v1 for 5" {
		t.Fatalf("unexpected content in snapView1: %s", string(dataV1))
	}

	dataV2, err := r2.ReadExtent("docs/page_05.txt")
	if err != nil || string(dataV2) != "UPDATED PAGE CONTENT V2" {
		t.Fatalf("unexpected content in r2: %s", string(dataV2))
	}
}
