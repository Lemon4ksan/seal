// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vfs_test

import (
	"io/fs"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/seal/container/eon/vfs"
)

type mockReader struct {
	extents []catalog.Extent
	data    map[string][]byte
}

func (m *mockReader) Extents() []catalog.Extent {
	return m.extents
}

func (m *mockReader) ReadExtent(name string) ([]byte, error) {
	d, ok := m.data[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return d, nil
}

func TestEON_VFS(t *testing.T) {
	mock := &mockReader{
		extents: []catalog.Extent{
			{Name: "docs/readme.txt", PayloadSize: 12, Namespace: spec.NamespaceVFS},
			{Name: "docs/nested/guide.md", PayloadSize: 20, Namespace: spec.NamespaceVFS},
			{Name: "src/main.go", PayloadSize: 50, Namespace: spec.NamespaceVFS},
		},
		data: map[string][]byte{
			"docs/readme.txt":      []byte("Hello World!"),
			"docs/nested/guide.md": []byte("# User Guide Content"),
			"src/main.go":          []byte("package main\n\nfunc main() {}\n"),
		},
	}

	filesystem := vfs.New(mock)

	// ReadFileFS test
	content, err := filesystem.ReadFile("docs/readme.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "Hello World!" {
		t.Errorf("unexpected content: %q", content)
	}

	// ReadDirFS test
	entries, err := filesystem.ReadDir("docs")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries in docs, got %d", len(entries))
	}

	// Implicit directory Stat test
	info, err := filesystem.Stat("docs/nested")
	if err != nil {
		t.Fatalf("Stat on implicit dir failed: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected docs/nested to be a directory")
	}
}
