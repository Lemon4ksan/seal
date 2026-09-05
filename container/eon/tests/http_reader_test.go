// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/engine"
)

func TestHTTPRange_RemoteStreaming(t *testing.T) {
	// 1. Pack in-memory EON container
	buf := new(bytes.Buffer)
	w := engine.NewWriter(buf)

	file1 := []byte("Large simulated asset data stream #1")
	file2 := []byte("Config and metadata payload #2")

	if err := w.AddFile("assets/model.bin", file1); err != nil {
		t.Fatalf("add file 1: %v", err)
	}
	if err := w.AddFile("config.json", file2); err != nil {
		t.Fatalf("add file 2: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	containerBytes := buf.Bytes()
	rangeRequestsCount := 0

	// 2. Start HTTP server supporting Range requests via http.ServeContent
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Range") != "" {
			rangeRequestsCount++
		}
		http.ServeContent(rw, req, "test.eon", time.Now(), bytes.NewReader(containerBytes))
	}))
	defer server.Close()

	// 3. Open remotely via OpenURL
	r, closer, err := engine.OpenURL(server.URL + "/test.eon")
	if err != nil {
		t.Fatalf("OpenURL failed: %v", err)
	}
	defer closer.Close()

	// 4. Verify catalog
	extents := r.Extents()
	if len(extents) != 2 {
		t.Fatalf("expected 2 extents, got %d", len(extents))
	}

	// 5. Read individual file #1
	data1, err := r.ReadExtent("assets/model.bin")
	if err != nil {
		t.Fatalf("read extent 1: %v", err)
	}
	if !bytes.Equal(data1, file1) {
		t.Errorf("data mismatch for model.bin")
	}

	// 6. Read individual file #2
	data2, err := r.ReadExtent("config.json")
	if err != nil {
		t.Fatalf("read extent 2: %v", err)
	}
	if !bytes.Equal(data2, file2) {
		t.Errorf("data mismatch for config.json")
	}

	// 7. Verify that Range requests were actually used
	if rangeRequestsCount == 0 {
		t.Errorf("expected multiple HTTP Range requests, got 0")
	}
	t.Logf("Successfully streamed remote EON container via %d Range requests!", rangeRequestsCount)
}

func TestHTTPRange_EncryptedRemoteContainer(t *testing.T) {
	buf := new(bytes.Buffer)
	password := "SecretCloudKey2026!"
	w := engine.NewWriter(buf, engine.WithPassword(password))

	payload := []byte("Top secret remote AI model weights")
	if err := w.AddFile("weights.safetensors", payload); err != nil {
		t.Fatalf("add file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	containerBytes := buf.Bytes()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		http.ServeContent(rw, req, "weights.eon", time.Now(), bytes.NewReader(containerBytes))
	}))
	defer server.Close()

	// Open with password
	r, closer, err := engine.OpenURL(server.URL+"/weights.eon", engine.WithReaderPassword(password))
	if err != nil {
		t.Fatalf("OpenURL encrypted failed: %v", err)
	}
	defer closer.Close()

	res, err := r.ReadExtent("weights.safetensors")
	if err != nil {
		t.Fatalf("read encrypted extent: %v", err)
	}
	if !bytes.Equal(res, payload) {
		t.Errorf("decrypted payload mismatch")
	}
}
