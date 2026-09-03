// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal_test

import (
	"bytes"
	"io"
	"testing"
	"testing/fstest"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
)

func TestArchive_BasicOperations(t *testing.T) {
	t.Parallel()
	archive := seal.NewArchive()

	// Add string, bytes, and fs
	archive.AddString("Hello World", "greeting.txt")
	archive.AddBytes([]byte(`{"status":"ok"}`), "config.json")

	if !archive.Exists("greeting.txt") {
		t.Error("greeting.txt should exist")
	}
	if !archive.Exists("config.json") {
		t.Error("config.json should exist")
	}

	// Read through virtual FS
	data, err := archive.ReadFile("greeting.txt")
	if err != nil {
		t.Fatalf("archive.ReadFile failed: %v", err)
	}
	if string(data) != "Hello World" {
		t.Errorf("content mismatch: got %q", string(data))
	}

	// Rename
	if err := archive.Rename("greeting.txt", "docs/greeting.txt"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	if archive.Exists("greeting.txt") {
		t.Error("old name greeting.txt should not exist")
	}
	if !archive.Exists("docs/greeting.txt") {
		t.Error("new name docs/greeting.txt should exist")
	}

	// Tree
	treeStr := archive.Tree()
	if treeStr == "" {
		t.Error("expected non-empty tree string")
	}

	// Serialization roundtrip
	buf := new(bytes.Buffer)
	written, err := archive.WriteTo(t.Context(), buf)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	if written <= 0 {
		t.Errorf("expected positive written count, got %d", written)
	}

	// Read back into new archive
	raw := buf.Bytes()
	freshArchive := seal.NewArchive()
	loaded, err := freshArchive.Load(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("freshArchive.Load failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded entries, got %d", len(loaded))
	}
}

func TestArchive_AddFS(t *testing.T) {
	t.Parallel()
	memFS := fstest.MapFS{
		"a.txt":     {Data: []byte("file A")},
		"sub/b.txt": {Data: []byte("file B")},
	}

	archive := seal.NewArchive()
	added, err := archive.AddFS(memFS)
	if err != nil {
		t.Fatalf("AddFS failed: %v", err)
	}
	if len(added) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(added))
	}

	buf := new(bytes.Buffer)
	if _, err := archive.WriteTo(t.Context(), buf); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	raw := buf.Bytes()
	fresh := seal.NewArchive()
	if _, err := fresh.Load(bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	fileA, ok := fresh.File("a.txt")
	if !ok {
		t.Fatal("file a.txt not found")
	}
	rc, err := fileA.Open()
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(content) != "file A" {
		t.Errorf("content mismatch: got %q, want %q", string(content), "file A")
	}
}

func TestArchive_SevenZip_BasicOperations(t *testing.T) {
	t.Parallel()
	arc := seal.NewArchive(seal.WithContainer(container.SevenZip))

	arc.AddString("7z Content test string", "data.txt")
	arc.AddBytes([]byte("Binary Payload 123456"), "sub/binary.bin")

	if arc.Container() != container.SevenZip {
		t.Fatalf("expected container %v, got %v", container.SevenZip, arc.Container())
	}

	buf := new(bytes.Buffer)
	written, err := arc.WriteTo(t.Context(), buf)
	if err != nil {
		t.Fatalf("7z WriteTo failed: %v", err)
	}
	if written <= 0 {
		t.Fatalf("expected positive written, got %d", written)
	}

	raw := buf.Bytes()
	fresh := seal.New7z()
	loaded, err := fresh.Load(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("7z Load failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded entries, got %d", len(loaded))
	}

	f1, ok := fresh.File("data.txt")
	if !ok {
		t.Fatal("data.txt not found in 7z")
	}
	s, err := f1.String()
	if err != nil {
		t.Fatalf("f1.String failed: %v", err)
	}
	if s != "7z Content test string" {
		t.Errorf("f1 content mismatch: got %q", s)
	}

	f2, ok := fresh.File("sub/binary.bin")
	if !ok {
		t.Fatal("sub/binary.bin not found in 7z")
	}
	b, err := f2.Bytes()
	if err != nil {
		t.Fatalf("f2.Bytes failed: %v", err)
	}
	if string(b) != "Binary Payload 123456" {
		t.Errorf("f2 content mismatch: got %q", string(b))
	}
}
