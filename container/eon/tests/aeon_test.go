// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/chunk"
	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func TestAeonRoundtrip(t *testing.T) {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf)

	files := map[string][]byte{
		"greeting.txt":       []byte("Hello, Aeon format!"),
		"config/system.json": []byte(`{"version": 1, "profile": "production"}`),
		"large/payload.dat":  make([]byte, 256*1024),
	}

	for i := range files["large/payload.dat"] {
		files["large/payload.dat"][i] = byte(i % 173)
	}

	for name, data := range files {
		if err := w.AddFile(name, data); err != nil {
			t.Fatalf("AddFile %s failed: %v", name, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	raw := buf.Bytes()
	r, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	// Verify all extents
	extents := r.Extents()
	if len(extents) != len(files) {
		t.Fatalf("expected %d extents, got %d", len(files), len(extents))
	}

	for name, want := range files {
		got, err := r.ReadExtent(name)
		if err != nil {
			t.Fatalf("ReadExtent %s failed: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("content mismatch for %s", name)
		}
	}
}

func TestAeonVFS(t *testing.T) {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf)

	testDoc := []byte("Aeon Virtual Filesystem Integration")
	if err := w.AddFile("docs/readme.txt", testDoc); err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	raw := buf.Bytes()
	vfsLayer, err := engine.OpenFS(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("OpenFS failed: %v", err)
	}

	data, err := fs.ReadFile(vfsLayer, "docs/readme.txt")
	if err != nil {
		t.Fatalf("fs.ReadFile failed: %v", err)
	}
	if string(data) != string(testDoc) {
		t.Fatalf("expected %q, got %q", string(testDoc), string(data))
	}
}

func TestCustomChunkPipeline(t *testing.T) {
	p := chunk.NewPipeline()

	// 1. Dispatch to unhandled namespace -> MUST silently succeed (Rule 6)
	err := p.Dispatch(0x9999, "custom_tensor", []byte("unhandled tensor blob"))
	if err != nil {
		t.Fatalf("expected unhandled namespace to gracefully bypass, got: %v", err)
	}

	// 2. Register custom handler
	handled := false
	handler := &mockHandler{
		ns: 0x8888,
		fn: func(name string, payload []byte) error {
			handled = true
			return nil
		},
	}
	p.Register(handler)

	err = p.Dispatch(0x8888, "my_block", []byte("data"))
	if err != nil {
		t.Fatalf("dispatch error: %v", err)
	}
	if !handled {
		t.Fatalf("expected custom handler to execute")
	}
}

type mockHandler struct {
	ns uint16
	fn func(name string, payload []byte) error
}

func (m *mockHandler) Namespace() uint16                  { return m.ns }
func (m *mockHandler) Description() string                { return "mock" }
func (m *mockHandler) Handle(name string, b []byte) error { return m.fn(name, b) }

func TestMultiNamespaceContainer(t *testing.T) {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf)

	// File in VFS namespace
	if err := w.AddFile("system.conf", []byte("conf_data")); err != nil {
		t.Fatalf("add vfs file: %v", err)
	}

	// Extent in Tensor namespace (0x0002)
	tensorData := []byte{0x01, 0x02, 0x03, 0x04}
	tensorExt := catalog.Extent{
		Namespace: spec.NamespaceTensor,
		Name:      "layer_0.weight",
	}
	if err := w.AddExtent(tensorExt, tensorData); err != nil {
		t.Fatalf("add tensor extent: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close container: %v", err)
	}

	r, err := engine.OpenReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}

	exts := r.Extents()
	if len(exts) != 2 {
		t.Fatalf("expected 2 extents, got %d", len(exts))
	}

	readTensor, err := r.ReadExtent("layer_0.weight")
	if err != nil {
		t.Fatalf("read tensor: %v", err)
	}
	if !bytes.Equal(readTensor, tensorData) {
		t.Fatalf("tensor data mismatch")
	}
}

func TestConveniencePackAndExtract(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.eon")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}

	files := map[string][]byte{
		"a.txt": []byte("File A"),
		"b.txt": []byte("File B"),
	}

	if err := engine.Pack(f, files); err != nil {
		f.Close()
		t.Fatalf("Pack failed: %v", err)
	}
	f.Close()

	r, closer, err := engine.OpenFile(archivePath)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer closer.Close()

	if len(r.Extents()) != 2 {
		t.Fatalf("expected 2 extents, got %d", len(r.Extents()))
	}
}
