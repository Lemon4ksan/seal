// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive_test

import (
	"bytes"
	"io"
	"testing"
	"testing/fstest"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/option"
)

func TestArchive_BasicOperations(t *testing.T) {
	t.Parallel()
	a := archive.New(archive.WithContainer(zip.New()))

	a.AddBytes([]byte("Hello world"), "test.txt")
	a.AddString("JSON content", "config.json")
	a.AddLazy("lazy.txt", func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("Lazy evaluated"))), nil
	})

	if !a.Exists("test.txt") {
		t.Errorf("expected test.txt to exist")
	}
	if !a.Exists("config.json") {
		t.Errorf("expected config.json to exist")
	}

	files := a.Files()
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	if err := a.Rename("test.txt", "renamed.txt"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	if a.Exists("test.txt") {
		t.Errorf("test.txt should not exist after rename")
	}
	if !a.Exists("renamed.txt") {
		t.Errorf("renamed.txt should exist after rename")
	}

	if !a.Remove("renamed.txt") {
		t.Errorf("expected Remove to return true")
	}
	if len(a.Files()) != 2 {
		t.Errorf("expected 2 files after delete, got %d", len(a.Files()))
	}
}

func TestArchive_AddFS(t *testing.T) {
	t.Parallel()
	mockFS := fstest.MapFS{
		"file1.txt":      &fstest.MapFile{Data: []byte("file1")},
		"subdir/doc.txt": &fstest.MapFile{Data: []byte("doc")},
	}

	a := archive.New(archive.WithContainer(zip.New()), option.WithCompression(codec.Store, 0))
	added, err := a.AddFS(mockFS)
	if err != nil {
		t.Fatalf("AddFS failed: %v", err)
	}

	if len(added) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(added))
	}

	buf := new(bytes.Buffer)
	written, err := a.WriteTo(t.Context(), buf)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	if written <= 0 {
		t.Errorf("expected written bytes > 0, got %d", written)
	}
}
