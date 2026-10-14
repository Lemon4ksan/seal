// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vfs_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/webdav"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/seal/container/eon/vfs"
)

func TestWebDAV_MountRead(t *testing.T) {
	mock := &mockReader{
		extents: []catalog.Extent{
			{Name: "docs/readme.txt", PayloadSize: 12, Namespace: spec.NamespaceVFS},
			{Name: "data.bin", PayloadSize: 4, Namespace: spec.NamespaceVFS},
		},
		data: map[string][]byte{
			"docs/readme.txt": []byte("Hello World!"),
			"data.bin":        {0xDE, 0xAD, 0xBE, 0xEF},
		},
	}

	filesystem := vfs.New(mock)
	webdavFS := vfs.NewWebDAV(filesystem)
	handler := &webdav.Handler{
		FileSystem: webdavFS,
		LockSystem: webdav.NewMemLS(),
	}

	server := httptest.NewServer(handler)
	defer server.Close()

	// 1. GET /docs/readme.txt
	resp, err := http.Get(server.URL + "/docs/readme.txt")
	if err != nil {
		t.Fatalf("GET /docs/readme.txt failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body failed: %v", err)
	}
	if string(body) != "Hello World!" {
		t.Errorf("got %q, expected 'Hello World!'", string(body))
	}

	// 2. GET /data.bin
	resp2, err := http.Get(server.URL + "/data.bin")
	if err != nil {
		t.Fatalf("GET /data.bin failed: %v", err)
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	if !bytes.Equal(body2, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("binary payload mismatch: %x", body2)
	}

	// 3. GET nonexistent file -> 404
	resp3, err := http.Get(server.URL + "/missing.txt")
	if err != nil {
		t.Fatalf("GET /missing.txt failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for missing file, got %d", resp3.StatusCode)
	}
}
