// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal_test

import (
	"bytes"
	"io"
	"testing"
	"testing/fstest"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
)

func TestArchive_BasicOperations(t *testing.T) {
	t.Parallel()
	archive := seal.NewArchive()

	// Add string, bytes, and fs
	archive.AddString("Hello World", "greeting.txt")
	archive.AddBytes([]byte(`{"status":"ok"}`), "config.json")

	assert.True(t, archive.Exists("greeting.txt"), "greeting.txt should exist")
	assert.True(t, archive.Exists("config.json"), "config.json should exist")

	// Read through virtual FS
	data, err := archive.ReadFile("greeting.txt")
	require.NoError(t, err)
	assert.Equal(t, "Hello World", string(data))

	// Rename
	err = archive.Rename("greeting.txt", "docs/greeting.txt")
	require.NoError(t, err)
	assert.False(t, archive.Exists("greeting.txt"), "old name greeting.txt should not exist")
	assert.True(t, archive.Exists("docs/greeting.txt"), "new name docs/greeting.txt should exist")

	// Tree
	treeStr := archive.Tree()
	assert.NotEmpty(t, treeStr, "expected non-empty tree string")

	// Serialization roundtrip
	buf := new(bytes.Buffer)
	written, err := archive.WriteTo(t.Context(), buf)
	require.NoError(t, err)
	assert.Greater(t, written, int64(0), "expected positive written count")

	// Read back into new archive
	raw := buf.Bytes()
	freshArchive := seal.NewArchive()
	loaded, err := freshArchive.Load(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	assert.Len(t, loaded, 2)
}

func TestArchive_AddFS(t *testing.T) {
	t.Parallel()
	memFS := fstest.MapFS{
		"a.txt":     {Data: []byte("file A")},
		"sub/b.txt": {Data: []byte("file B")},
	}

	archive := seal.NewArchive()
	added, err := archive.AddFS(memFS)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(added), 2)

	buf := new(bytes.Buffer)
	_, err = archive.WriteTo(t.Context(), buf)
	require.NoError(t, err)

	raw := buf.Bytes()
	fresh := seal.NewArchive()
	_, err = fresh.Load(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)

	fileA, ok := fresh.File("a.txt")
	require.True(t, ok, "file a.txt not found")
	rc, err := fileA.Open()
	require.NoError(t, err)
	defer rc.Close()

	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "file A", string(content))
}

func TestArchive_SevenZip_BasicOperations(t *testing.T) {
	t.Parallel()
	arc := seal.NewArchive(seal.WithContainer(container.SevenZip))

	arc.AddString("7z Content test string", "data.txt")
	arc.AddBytes([]byte("Binary Payload 123456"), "sub/binary.bin")

	assert.Equal(t, container.SevenZip, arc.Container())

	buf := new(bytes.Buffer)
	written, err := arc.WriteTo(t.Context(), buf)
	require.NoError(t, err)
	assert.Greater(t, written, int64(0))

	raw := buf.Bytes()
	fresh := seal.New7z()
	loaded, err := fresh.Load(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	assert.Len(t, loaded, 2)

	f1, ok := fresh.File("data.txt")
	require.True(t, ok, "data.txt not found in 7z")
	s, err := f1.String()
	require.NoError(t, err)
	assert.Equal(t, "7z Content test string", s)

	f2, ok := fresh.File("sub/binary.bin")
	require.True(t, ok, "sub/binary.bin not found in 7z")
	b, err := f2.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "Binary Payload 123456", string(b))
}

func TestArchive_EONRoundtrip(t *testing.T) {
	t.Parallel()
	arc := seal.NewEon()

	arc.AddString("Hello from EON container!", "hello.txt")
	arc.AddBytes([]byte("EON Universal Extent Payload 42"), "nested/test.bin")

	buf := new(bytes.Buffer)
	written, err := arc.WriteTo(t.Context(), buf)
	require.NoError(t, err)
	assert.Greater(t, written, int64(0))

	raw := buf.Bytes()
	fresh := seal.NewEon()
	loaded, err := fresh.Load(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	assert.Len(t, loaded, 2)

	f1, ok := fresh.File("hello.txt")
	require.True(t, ok, "hello.txt not found in loaded EON")
	s, err := f1.String()
	require.NoError(t, err)
	assert.Equal(t, "Hello from EON container!", s)

	f2, ok := fresh.File("nested/test.bin")
	require.True(t, ok, "nested/test.bin not found in loaded EON")
	b, err := f2.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "EON Universal Extent Payload 42", string(b))
}
