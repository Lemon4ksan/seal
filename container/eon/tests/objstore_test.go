// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/objstore"
)

func TestObjStore_DecomposeStreamAndCompose(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "source.eon")
	storeDir := filepath.Join(tempDir, "remote_bucket")
	cacheDir := filepath.Join(tempDir, "local_cache")
	composedPath := filepath.Join(tempDir, "composed.eon")

	// 1. Create a sample archive with multiple files
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive file: %v", err)
	}

	w := engine.NewWriter(f, engine.WithProfileFastMount())
	filesData := map[string][]byte{
		"config.json": []byte(`{"version": "1.0.0", "name": "eon-cloud-test"}`),
		"data/large_1.bin": bytes.Repeat([]byte("ABCDEF0123456789_CHUNK_ONE_DATA_"), 10000), // ~320 KB
		"data/large_2.bin": bytes.Repeat([]byte("FEDCBA9876543210_CHUNK_TWO_DATA_"), 12000), // ~384 KB
		"readme.txt":       []byte("Aeon S3/R2 detached chunk storage integration."),
	}

	now := time.Now()
	for name, data := range filesData {
		if err := w.AddFileStream(name, int64(len(data)), now, bytes.NewReader(data)); err != nil {
			t.Fatalf("add file %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	f.Close()

	// 2. Decompose archive into FSStore
	store := objstore.NewFSStore(storeDir)
	stats, err := engine.DecomposeArchive(archivePath, store, "")
	if err != nil {
		t.Fatalf("decompose archive: %v", err)
	}

	if stats.UploadedChunks == 0 {
		t.Fatalf("expected uploaded chunks > 0, got %d", stats.UploadedChunks)
	}
	if stats.IndexBytes == 0 {
		t.Fatalf("expected index bytes > 0, got %d", stats.IndexBytes)
	}

	// Verify index.eon exists in store
	exists, err := store.Exists(context.Background(), "index.eon")
	if err != nil || !exists {
		t.Fatalf("index.eon not found in store: %v", err)
	}

	// 3. Open remote archive via OpenStore with lazy streaming
	r, err := engine.OpenStore(store, "", engine.WithDiskCache(cacheDir))
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}

	if len(r.Extents()) != len(filesData) {
		t.Fatalf("expected %d extents, got %d", len(filesData), len(r.Extents()))
	}

	// Read each file lazily and verify content matches
	for name, expected := range filesData {
		got, err := r.ReadExtent(name)
		if err != nil {
			t.Fatalf("ReadExtent(%s): %v", name, err)
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("content mismatch for %s: got len %d, want len %d", name, len(got), len(expected))
		}
	}

	// 4. Test streaming hydration with disk cache
	if err := r.Hydrate(); err != nil {
		t.Fatalf("Hydrate() failed: %v", err)
	}

	// Verify that chunks were cached in local disk cache
	cachedFiles, err := os.ReadDir(cacheDir)
	if err != nil || len(cachedFiles) == 0 {
		t.Fatalf("expected cached files in %s, got err: %v", cacheDir, err)
	}

	// Read again; should hit disk/memory cache seamlessly
	for name, expected := range filesData {
		got, err := r.ReadExtent(name)
		if err != nil {
			t.Fatalf("cached ReadExtent(%s): %v", name, err)
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("cached content mismatch for %s", name)
		}
	}

	// 5. Test ComposeArchive back to monolithic .eon file
	compFile, err := os.Create(composedPath)
	if err != nil {
		t.Fatalf("create composed file: %v", err)
	}
	if err := engine.ComposeArchive(store, "", compFile); err != nil {
		t.Fatalf("ComposeArchive failed: %v", err)
	}
	compFile.Close()

	// Verify composed archive can be opened as normal monolithic container
	cr, closer, err := engine.OpenFile(composedPath)
	if err != nil {
		t.Fatalf("open composed archive: %v", err)
	}
	defer closer.Close()

	if err := cr.Verify(nil); err != nil {
		t.Fatalf("composed archive verification failed: %v", err)
	}

	for name, expected := range filesData {
		got, err := cr.ReadExtent(name)
		if err != nil {
			t.Fatalf("composed ReadExtent(%s): %v", name, err)
		}
		if !bytes.Equal(got, expected) {
			t.Fatalf("composed content mismatch for %s", name)
		}
	}
}

func TestObjStore_CrossVersionDeduplication(t *testing.T) {
	tempDir := t.TempDir()
	storeDir := filepath.Join(tempDir, "shared_bucket")
	store := objstore.NewFSStore(storeDir)

	now := time.Now()

	// Version 1: 3 files
	v1Path := filepath.Join(tempDir, "v1.eon")
	f1, _ := os.Create(v1Path)
	w1 := engine.NewWriter(f1, engine.WithProfileFastMount())
	fileShared1 := bytes.Repeat([]byte("SHARED_PAYLOAD_ALPHA_"), 8000)
	fileShared2 := bytes.Repeat([]byte("SHARED_PAYLOAD_BETA__"), 8000)
	fileV1Only := bytes.Repeat([]byte("V1_SPECIFIC_CONTENT___"), 8000)

	_ = w1.AddFileStream("shared1.bin", int64(len(fileShared1)), now, bytes.NewReader(fileShared1))
	_ = w1.AddFileStream("shared2.bin", int64(len(fileShared2)), now, bytes.NewReader(fileShared2))
	_ = w1.AddFileStream("diff.bin", int64(len(fileV1Only)), now, bytes.NewReader(fileV1Only))
	_ = w1.Close()
	f1.Close()

	// Decompose V1 into repository prefix "repo"
	stats1, err := engine.DecomposeArchive(v1Path, store, "repo")
	if err != nil {
		t.Fatalf("decompose v1: %v", err)
	}
	t.Logf("V1 stats: total=%d, uploaded=%d, reused=%d", stats1.TotalChunks, stats1.UploadedChunks, stats1.ReusedChunks)
	if stats1.UploadedChunks == 0 {
		t.Fatalf("expected V1 to upload chunks")
	}

	// Version 2: 2 shared files identical, 1 modified file
	v2Path := filepath.Join(tempDir, "v2.eon")
	f2, _ := os.Create(v2Path)
	w2 := engine.NewWriter(f2, engine.WithProfileFastMount())
	fileV2Only := bytes.Repeat([]byte("V2_MUTATED_DIFFERENT_"), 8000)

	_ = w2.AddFileStream("shared1.bin", int64(len(fileShared1)), now, bytes.NewReader(fileShared1))
	_ = w2.AddFileStream("shared2.bin", int64(len(fileShared2)), now, bytes.NewReader(fileShared2))
	_ = w2.AddFileStream("diff.bin", int64(len(fileV2Only)), now, bytes.NewReader(fileV2Only))
	_ = w2.Close()
	f2.Close()

	// Decompose V2 into the SAME repository prefix "repo"!
	// Chunks for shared1 and shared2 already exist and will be deduplicated via CAS.
	stats2, err := engine.DecomposeArchive(v2Path, store, "repo")
	if err != nil {
		t.Fatalf("decompose v2: %v", err)
	}
	t.Logf("V2 stats: total=%d, uploaded=%d, reused=%d", stats2.TotalChunks, stats2.UploadedChunks, stats2.ReusedChunks)

	// Since shared1 and shared2 were already uploaded in V1,
	// V2 must detect and reuse them via CAS!
	if stats2.ReusedChunks == 0 {
		t.Errorf("expected V2 to reuse shared chunks, but got 0 reused")
	}

	// Verify V2 can be read from the object store
	r2, err := engine.OpenStore(store, "repo")
	if err != nil {
		t.Fatalf("open v2 store: %v", err)
	}
	data2, err := r2.ReadExtent("diff.bin")
	if err != nil || !bytes.Equal(data2, fileV2Only) {
		t.Fatalf("v2 diff.bin mismatch")
	}

	// Can read shared1
	s1, err := r2.ReadExtent("shared1.bin")
	if err != nil || !bytes.Equal(s1, fileShared1) {
		t.Fatalf("shared1 content mismatch")
	}
}

func TestObjStore_URI_Parsing(t *testing.T) {
	cases := []struct {
		uri        string
		wantPrefix string
		wantS3     bool
	}{
		{"s3://my-bucket/models/llama3", "models/llama3", true},
		{"r2://data-lake/backups/", "backups", true},
		{"file:///tmp/repo", "", false},
	}

	for _, c := range cases {
		store, prefix, err := objstore.ParseURI(c.uri)
		if err != nil {
			t.Fatalf("ParseURI(%q) failed: %v", c.uri, err)
		}
		if prefix != c.wantPrefix {
			t.Errorf("ParseURI(%q) prefix = %q, want %q", c.uri, prefix, c.wantPrefix)
		}
		if c.wantS3 {
			if _, ok := store.(*objstore.S3Store); !ok {
				t.Errorf("ParseURI(%q) expected *S3Store, got %T", c.uri, store)
			}
		}
	}
}
