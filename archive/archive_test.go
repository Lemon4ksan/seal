// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive_test

import (
	"bytes"
	"io"
	"testing"
	"testing/fstest"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

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

	assert.True(t, a.Exists("test.txt"), "expected test.txt to exist")
	assert.True(t, a.Exists("config.json"), "expected config.json to exist")

	files := a.Files()
	assert.Len(t, files, 3)

	err := a.Rename("test.txt", "renamed.txt")
	require.NoError(t, err)
	assert.False(t, a.Exists("test.txt"), "test.txt should not exist after rename")
	assert.True(t, a.Exists("renamed.txt"), "renamed.txt should exist after rename")

	assert.True(t, a.Remove("renamed.txt"), "expected Remove to return true")
	assert.Len(t, a.Files(), 2)
}

func TestArchive_AddFS(t *testing.T) {
	t.Parallel()
	mockFS := fstest.MapFS{
		"file1.txt":      &fstest.MapFile{Data: []byte("file1")},
		"subdir/doc.txt": &fstest.MapFile{Data: []byte("doc")},
	}

	a := archive.New(archive.WithContainer(zip.New()), option.WithCompression(codec.Store, 0))
	added, err := a.AddFS(mockFS)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(added), 2)

	buf := new(bytes.Buffer)
	written, err := a.WriteTo(t.Context(), buf)
	require.NoError(t, err)
	assert.Greater(t, written, int64(0), "expected written bytes > 0")
}
