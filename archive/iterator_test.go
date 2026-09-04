// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive_test

import (
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/file"
)

func TestArchive_All_Iterator(t *testing.T) {
	t.Parallel()
	arc := archive.New()
	f1, _ := file.New("a.txt")
	f1.WithData([]byte("hello A"))
	f2, _ := file.New("b.txt")
	f2.WithData([]byte("hello B"))
	f3, _ := file.New("c.txt")
	f3.WithData([]byte("hello C"))

	arc.Add(f1, f2, f3)

	var collected []string
	for f := range arc.All() {
		collected = append(collected, f.Name())
	}

	require.Equal(t, []string{"a.txt", "b.txt", "c.txt"}, collected)

	// Early break test
	var early []string
	for f := range arc.All() {
		early = append(early, f.Name())
		if len(early) == 2 {
			break
		}
	}
	assert.Len(t, early, 2)
}

func TestArchive_AllWithIndex_Iterator(t *testing.T) {
	t.Parallel()
	arc := archive.New()
	f1, _ := file.New("x.txt")
	f2, _ := file.New("y.txt")
	arc.Add(f1, f2)

	indices := make([]int, 0)
	names := make([]string, 0)
	for i, f := range arc.AllWithIndex() {
		indices = append(indices, i)
		names = append(names, f.Name())
	}

	assert.Equal(t, []int{0, 1}, indices)
	assert.Equal(t, []string{"x.txt", "y.txt"}, names)
}

func TestArchive_StatefulIterator(t *testing.T) {
	t.Parallel()
	arc := archive.New()
	f1, _ := file.New("file1.txt")
	f1.WithData([]byte("content-1"))
	f2, _ := file.New("file2.txt")
	f2.WithData([]byte("content-2"))
	arc.Add(f1, f2)

	it := arc.NewIterator()
	assert.Equal(t, 2, it.Len())
	assert.Equal(t, -1, it.Pos())

	// First item
	require.True(t, it.Next(), "expected Next() true for item 0")
	assert.Equal(t, 0, it.Pos())
	assert.Equal(t, "file1.txt", it.File().Name())

	rc, err := it.Open()
	require.NoError(t, err)
	data, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, []byte("content-1"), data)

	// Second item
	require.True(t, it.Next(), "expected Next() true for item 1")
	assert.Equal(t, 1, it.Pos())
	assert.Equal(t, "file2.txt", it.File().Name())

	b, err := it.Bytes()
	require.NoError(t, err)
	assert.Equal(t, []byte("content-2"), b)

	// End of iteration
	assert.False(t, it.Next(), "expected Next() false at end")
	assert.Nil(t, it.File(), "expected File() nil at end")

	// Reset
	it.Reset()
	assert.Equal(t, -1, it.Pos(), "expected Pos -1 after Reset")
	require.True(t, it.Next())
	assert.Equal(t, "file1.txt", it.File().Name())
}
