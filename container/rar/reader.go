// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/lemon4ksan/seal/container/rar/internal/rardecode"

	"github.com/lemon4ksan/seal/file"
)

// Reader provides random-access catalog reading and extraction for RAR archives.
type Reader struct {
	rAt      io.ReaderAt
	size     int64
	password string
	entries  []*file.File
	comment  string
	mu       sync.Mutex
}

// NewReader opens a RAR archive from an io.ReaderAt and parses its file catalog.
func NewReader(rAt io.ReaderAt, size int64, password string) (*Reader, error) {
	if rAt == nil || size < 7 {
		return nil, errors.New("rar: invalid source reader")
	}

	sr := io.NewSectionReader(rAt, 0, size)
	var opts []rardecode.Option
	if password != "" {
		opts = append(opts, rardecode.Password(password))
	}

	rr, err := rardecode.NewReader(sr, opts...)
	if err != nil {
		return nil, fmt.Errorf("rar: initialize reader: %w", err)
	}

	var entries []*file.File

	for {
		hdr, err := rr.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("rar: parse header: %w", err)
		}

		targetName := hdr.Name
		isDir := hdr.IsDir

		f, err := file.New(targetName, isDir)
		if err != nil {
			return nil, fmt.Errorf("rar: create file record %q: %w", targetName, err)
		}

		f.SetSizes(hdr.UnPackedSize, hdr.PackedSize, 0)
		if !hdr.ModificationTime.IsZero() {
			f.WithModTime(hdr.ModificationTime)
		}
		f.WithMode(hdr.Mode())

		if hdr.Encrypted {
			f.WithEncryption(file.AES256, password)
		}

		if isDir {
			f.SetOpenFunc(func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(nil)), nil
			})
		} else {
			f.SetOpenFunc(func() (io.ReadCloser, error) {
				fileSR := io.NewSectionReader(rAt, 0, size)
				fileRR, err := rardecode.NewReader(fileSR, opts...)
				if err != nil {
					return nil, err
				}
				for {
					currHdr, err := fileRR.Next()
					if err != nil {
						return nil, err
					}
					if currHdr.Name == targetName {
						return io.NopCloser(fileRR), nil
					}
				}
			})
		}

		entries = append(entries, f)
	}

	return &Reader{
		rAt:      rAt,
		size:     size,
		password: password,
		entries:  entries,
	}, nil
}

// Entries returns all parsed file entries in the archive.
func (r *Reader) Entries() []*file.File {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.entries
}

// Comment returns the global archive comment if present.
func (r *Reader) Comment() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.comment
}
