// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"io"
	"iter"
	"slices"

	"github.com/lemon4ksan/seal/file"
)

// All returns a standard Go range-over-func iterator yielding all [*file.File] entries in the archive.
//
// Usage:
//
//	for f := range arc.All() {
//	    fmt.Println(f.Name())
//	}
func (a *Archive) All() iter.Seq[*file.File] {
	return func(yield func(*file.File) bool) {
		a.mu.RLock()
		files := slices.Clone(a.files)
		a.mu.RUnlock()

		for _, f := range files {
			if !yield(f) {
				return
			}
		}
	}
}

// AllWithIndex returns a standard Go range-over-func iterator yielding (index, [*file.File]) pairs.
//
// Usage:
//
//	for i, f := range arc.AllWithIndex() {
//	    fmt.Printf("[%d] %s\n", i, f.Name())
//	}
func (a *Archive) AllWithIndex() iter.Seq2[int, *file.File] {
	return func(yield func(int, *file.File) bool) {
		a.mu.RLock()
		files := slices.Clone(a.files)
		a.mu.RUnlock()

		for i, f := range files {
			if !yield(i, f) {
				return
			}
		}
	}
}

// Iterator provides cursor-based sequential traversal over file entries in an archive.
type Iterator struct {
	files []*file.File
	pos   int
	curr  *file.File
	err   error
}

// NewIterator constructs a stateful cursor iterator over archive files.
func (a *Archive) NewIterator() *Iterator {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return &Iterator{
		files: slices.Clone(a.files),
		pos:   -1,
	}
}

// Iter is an alias for [Archive.NewIterator].
func (a *Archive) Iter() *Iterator {
	return a.NewIterator()
}

// Next advances the cursor to the next file entry.
// Returns false when there are no more entries or if an error occurred.
func (it *Iterator) Next() bool {
	it.pos++
	if it.pos >= len(it.files) {
		it.curr = nil
		return false
	}
	it.curr = it.files[it.pos]
	return true
}

// File returns the current [*file.File] under the cursor.
func (it *Iterator) File() *file.File {
	return it.curr
}

// Open opens an [io.ReadCloser] stream for the current file payload.
func (it *Iterator) Open() (io.ReadCloser, error) {
	if it.curr == nil {
		return nil, io.EOF
	}
	return it.curr.Open()
}

// Bytes reads and returns the uncompressed payload of the current file.
func (it *Iterator) Bytes() ([]byte, error) {
	if it.curr == nil {
		return nil, io.EOF
	}
	return it.curr.Bytes()
}

// Err returns any error encountered during iteration.
func (it *Iterator) Err() error {
	return it.err
}

// Pos returns the current 0-based index of the iterator (-1 before first Next).
func (it *Iterator) Pos() int {
	return it.pos
}

// Len returns the total number of entries in the iterator.
func (it *Iterator) Len() int {
	return len(it.files)
}

// Reset rewinds the cursor back to the beginning before the first entry.
func (it *Iterator) Reset() {
	it.pos = -1
	it.curr = nil
	it.err = nil
}
