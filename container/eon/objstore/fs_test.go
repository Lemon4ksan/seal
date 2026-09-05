// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objstore_test

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/objstore"
)

func TestFSStore_CRUD(t *testing.T) {
	tempDir := t.TempDir()
	store := objstore.NewFSStore(tempDir)
	ctx := context.Background()

	key := "chunks/ab/cd/sample.bin"
	payload := []byte("Hello, zero-allocation object storage layout!")

	// 1. Put
	if err := store.Put(ctx, key, bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 2. Exists & Head
	exists, err := store.Exists(ctx, key)
	if err != nil || !exists {
		t.Fatalf("expected key to exist, err: %v", err)
	}

	info, err := store.Head(ctx, key)
	if err != nil {
		t.Fatalf("Head failed: %v", err)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("expected size %d, got %d", len(payload), info.Size)
	}

	// 3. Get
	rc, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("mismatch: got %q, want %q", data, payload)
	}

	// 4. GetRange
	rangeRC, err := store.GetRange(ctx, key, 7, 15)
	if err != nil {
		t.Fatalf("GetRange failed: %v", err)
	}
	rangeData, err := io.ReadAll(rangeRC)
	_ = rangeRC.Close()
	if err != nil {
		t.Fatalf("ReadAll range failed: %v", err)
	}
	expectedRange := payload[7 : 7+15]
	if !bytes.Equal(rangeData, expectedRange) {
		t.Fatalf("range mismatch: got %q, want %q", rangeData, expectedRange)
	}

	// 5. List
	objs, err := store.List(ctx, "chunks/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(objs) != 1 || objs[0].Key != "chunks/ab/cd/sample.bin" {
		t.Fatalf("unexpected list results: %+v", objs)
	}

	// 6. Delete
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	exists, err = store.Exists(ctx, key)
	if err != nil || exists {
		t.Fatalf("expected key to be deleted, exists=%v, err=%v", exists, err)
	}
}

func TestParseURI(t *testing.T) {
	tempDir := t.TempDir()

	// Local path
	s1, p1, err := objstore.ParseURI(tempDir)
	if err != nil {
		t.Fatalf("ParseURI local failed: %v", err)
	}
	if p1 != "" || s1 == nil {
		t.Fatalf("unexpected result: p1=%q", p1)
	}

	// file:// URI
	s2, p2, err := objstore.ParseURI("file://" + filepath.ToSlash(tempDir))
	if err != nil {
		t.Fatalf("ParseURI file:// failed: %v", err)
	}
	if p2 != "" || s2 == nil {
		t.Fatalf("unexpected result: p2=%q", p2)
	}

	// s3:// URI
	s3, p3, err := objstore.ParseURI("s3://my-bucket/archives/prod")
	if err != nil {
		t.Fatalf("ParseURI s3:// failed: %v", err)
	}
	if p3 != "archives/prod" || s3 == nil {
		t.Fatalf("unexpected result: p3=%q", p3)
	}
}
