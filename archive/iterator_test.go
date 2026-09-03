// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/file"
)

func TestArchive_All_Iterator(t *testing.T) {
	t.Parallel()
	arc := archive.New()
	f1, _ := file.New("a.txt")
	f1.WithData([]byte("hello A"))
	f2, _ := file.New("b.txt")
	f2.WithData([]byte("hello B"))
	f3, _ := file.New("c.txt")
	f3.WithData([]byte("hello C"))

	arc.Add(f1, f2, f3)

	var collected []string
	for f := range arc.All() {
		collected = append(collected, f.Name())
	}

	if len(collected) != 3 || collected[0] != "a.txt" || collected[1] != "b.txt" || collected[2] != "c.txt" {
		t.Fatalf("unexpected files from All(): %v", collected)
	}

	// Early break test
	var early []string
	for f := range arc.All() {
		early = append(early, f.Name())
		if len(early) == 2 {
			break
		}
	}
	if len(early) != 2 {
		t.Fatalf("expected 2 elements from early break, got %d", len(early))
	}
}

func TestArchive_AllWithIndex_Iterator(t *testing.T) {
	t.Parallel()
	arc := archive.New()
	f1, _ := file.New("x.txt")
	f2, _ := file.New("y.txt")
	arc.Add(f1, f2)

	indices := make([]int, 0)
	names := make([]string, 0)
	for i, f := range arc.AllWithIndex() {
		indices = append(indices, i)
		names = append(names, f.Name())
	}

	if len(indices) != 2 || indices[0] != 0 || indices[1] != 1 {
		t.Fatalf("unexpected indices: %v", indices)
	}
	if len(names) != 2 || names[0] != "x.txt" || names[1] != "y.txt" {
		t.Fatalf("unexpected names: %v", names)
	}
}

func TestArchive_StatefulIterator(t *testing.T) {
	t.Parallel()
	arc := archive.New()
	f1, _ := file.New("file1.txt")
	f1.WithData([]byte("content-1"))
	f2, _ := file.New("file2.txt")
	f2.WithData([]byte("content-2"))
	arc.Add(f1, f2)

	it := arc.NewIterator()
	if it.Len() != 2 {
		t.Errorf("expected Len 2, got %d", it.Len())
	}
	if it.Pos() != -1 {
		t.Errorf("expected initial Pos -1, got %d", it.Pos())
	}

	// First item
	if !it.Next() {
		t.Fatalf("expected Next() true for item 0")
	}
	if it.Pos() != 0 || it.File().Name() != "file1.txt" {
		t.Errorf("expected item 0 to be file1.txt, got %v (pos %d)", it.File(), it.Pos())
	}
	rc, err := it.Open()
	if err != nil {
		t.Fatalf("it.Open() failed: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(data, []byte("content-1")) {
		t.Errorf("unexpected content: %s", string(data))
	}

	// Second item
	if !it.Next() {
		t.Fatalf("expected Next() true for item 1")
	}
	if it.Pos() != 1 || it.File().Name() != "file2.txt" {
		t.Errorf("expected item 1 to be file2.txt, got %v (pos %d)", it.File(), it.Pos())
	}
	b, err := it.Bytes()
	if err != nil || !bytes.Equal(b, []byte("content-2")) {
		t.Errorf("it.Bytes() mismatch: %s, %v", string(b), err)
	}

	// End of iteration
	if it.Next() {
		t.Errorf("expected Next() false at end")
	}
	if it.File() != nil {
		t.Errorf("expected File() nil at end")
	}

	// Reset
	it.Reset()
	if it.Pos() != -1 {
		t.Errorf("expected Pos -1 after Reset")
	}
	if !it.Next() || it.File().Name() != "file1.txt" {
		t.Errorf("failed to iterate after Reset")
	}
}
