// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FSStore implements Store backed by a local directory.
type FSStore struct {
	root string
}

// NewFSStore constructs an FSStore rooted at rootDir.
func NewFSStore(rootDir string) *FSStore {
	return &FSStore{
		root: filepath.Clean(rootDir),
	}
}

func (s *FSStore) pathOf(key string) string {
	cleanKey := filepath.FromSlash(strings.TrimPrefix(key, "/"))
	return filepath.Join(s.root, cleanKey)
}

// Get opens the local file for reading.
func (s *FSStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	p := s.pathOf(key)
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

// GetRange opens a byte-range of the local file for reading.
func (s *FSStore) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	p := s.pathOf(key)
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	sr := io.NewSectionReader(f, offset, length)
	return &rangeReadCloser{
		Reader: sr,
		closer: f,
	}, nil
}

type rangeReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *rangeReadCloser) Close() error {
	return r.closer.Close()
}

// Put writes an object atomically to the local filesystem.
func (s *FSStore) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	targetPath := s.pathOf(key)
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	// Write to temporary file first for atomic replacement
	tmpFile, err := os.CreateTemp(dir, "eon-tmp-*")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmpFile, r); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write payload: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Windows: os.Rename fails if target exists, so remove existing first if any
	_ = os.Remove(targetPath)
	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("atomic rename: %w", err)
	}

	return nil
}

// Head returns metadata for the object.
func (s *FSStore) Head(ctx context.Context, key string) (ObjectInfo, error) {
	p := s.pathOf(key)
	fi, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, err
	}
	return ObjectInfo{
		Key:          key,
		Size:         fi.Size(),
		LastModified: fi.ModTime(),
	}, nil
}

// Delete removes the file.
func (s *FSStore) Delete(ctx context.Context, key string) error {
	p := s.pathOf(key)
	err := os.Remove(p)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// List enumerates all files under prefix.
func (s *FSStore) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	var objects []ObjectInfo
	basePath := s.pathOf(prefix)

	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)

		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		objects = append(objects, ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})

	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	_ = basePath
	return objects, nil
}

// Exists checks whether key exists in the store.
func (s *FSStore) Exists(ctx context.Context, key string) (bool, error) {
	p := s.pathOf(key)
	_, err := os.Stat(p)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// URL returns the file:// URL.
func (s *FSStore) URL(key string) string {
	return "file://" + s.pathOf(key)
}
