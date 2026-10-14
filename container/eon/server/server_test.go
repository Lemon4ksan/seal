// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/server"
)

func createTestArchive(t *testing.T) (*engine.Reader, [32]byte) {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf, engine.WithProfileFastMount())

	now := time.Now()
	testFiles := map[string][]byte{
		"config.json":  []byte(`{"app": "aeon-cdn", "version": "1.0.0"}`),
		"hello.txt":    []byte("Hello from Aeon Edge CDN!"),
		"data/bin.dat": bytes.Repeat([]byte("0123456789ABCDEF"), 2000), // 32 KB
	}

	for name, content := range testFiles {
		if err := w.AddFileStream(name, int64(len(content)), now, bytes.NewReader(content)); err != nil {
			t.Fatalf("AddFileStream(%s): %v", name, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	data := buf.Bytes()
	r, err := engine.OpenReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}

	h0, err := r.ChunkHash(0)
	if err != nil {
		t.Fatalf("ChunkHash(0): %v", err)
	}

	return r, h0
}

func TestServer_FullAndRangeRequests(t *testing.T) {
	r, h0 := createTestArchive(t)
	srv := server.New(r, server.Config{
		EnableCORS: true,
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := ts.Client()

	// 1. Test /api/v1/info
	resp, err := client.Get(ts.URL + "/api/v1/info")
	if err != nil {
		t.Fatalf("GET /api/v1/info: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		t.Fatalf("decode info JSON: %v", err)
	}
	resp.Body.Close()
	if int(info["total_files"].(float64)) != 3 {
		t.Fatalf("expected 3 files, got %v", info["total_files"])
	}

	// 2. Test full file request
	resp, err = client.Get(ts.URL + "/hello.txt")
	if err != nil {
		t.Fatalf("GET /hello.txt: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "Hello from Aeon Edge CDN!" {
		t.Fatalf("content mismatch: got %q", string(body))
	}

	// 3. Test HTTP Range request (206 Partial Content)
	req, _ := http.NewRequest("GET", ts.URL+"/hello.txt", nil)
	req.Header.Set("Range", "bytes=0-4") // First 5 bytes: "Hello"
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Range request failed: %v", err)
	}
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", resp.StatusCode)
	}
	rangeBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(rangeBody) != "Hello" {
		t.Fatalf("range content mismatch: expected 'Hello', got %q", string(rangeBody))
	}

	// 4. Test Content-Addressed Chunk Serving (/chunks/<hash>)
	hexHash := hex.EncodeToString(h0[:])
	resp, err = client.Get(ts.URL + "/chunks/" + hexHash)
	if err != nil {
		t.Fatalf("GET /chunks/%s: %v", hexHash, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// Verify immutable CDN cache headers
	cacheControl := resp.Header.Get("Cache-Control")
	if !strings.Contains(cacheControl, "immutable") {
		t.Fatalf("expected immutable Cache-Control header, got %q", cacheControl)
	}
	etag := resp.Header.Get("ETag")
	if etag != fmt.Sprintf(`"%s"`, hexHash) {
		t.Fatalf("expected ETag %q, got %q", fmt.Sprintf(`"%s"`, hexHash), etag)
	}
	resp.Body.Close()

	// 5. Test 304 Not Modified when ETag matches
	req, _ = http.NewRequest("GET", ts.URL+"/chunks/"+hexHash, nil)
	req.Header.Set("If-None-Match", fmt.Sprintf(`"%s"`, hexHash))
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("If-None-Match request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("expected 304 Not Modified, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 6. Test Directory listing JSON
	req, _ = http.NewRequest("GET", ts.URL+"/", nil)
	req.Header.Set("Accept", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET / json failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	var dirItems []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&dirItems)
	resp.Body.Close()
	if len(dirItems) == 0 {
		t.Fatalf("expected items in directory listing")
	}
}

func TestServer_AuthToken(t *testing.T) {
	r, _ := createTestArchive(t)
	srv := server.New(r, server.Config{
		AuthToken: "secret123",
	})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	client := ts.Client()

	// Unauthorized
	resp, err := client.Get(ts.URL + "/hello.txt")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Authorized with Header
	req, _ := http.NewRequest("GET", ts.URL+"/hello.txt", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET with auth: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Authorized with query param
	resp, err = client.Get(ts.URL + "/hello.txt?token=secret123")
	if err != nil {
		t.Fatalf("GET with query token: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
