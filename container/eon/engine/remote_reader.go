// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/seal/container/eon/objstore"
)

// OpenStore opens a remote EON container from an object store (S3, R2, FS).
// It retrieves the lightweight index.eon file and prepares a Reader that lazily
// streams content-addressed chunks from the store on demand.
func OpenStore(store objstore.Store, prefix string, opts ...ReaderOption) (*Reader, error) {
	ctx := context.Background()
	indexKey := "index.eon"
	if prefix != "" {
		indexKey = strings.TrimSuffix(prefix, "/") + "/index.eon"
	}

	rc, err := store.Get(ctx, indexKey)
	if err != nil {
		return nil, fmt.Errorf("fetch remote index (%s): %w", indexKey, err)
	}
	indexData, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return nil, fmt.Errorf("read remote index: %w", err)
	}

	allOpts := append([]ReaderOption{
		WithChunkStore(store, prefix),
	}, opts...)

	rAt := bytes.NewReader(indexData)
	return OpenReader(rAt, int64(len(indexData)), allOpts...)
}
