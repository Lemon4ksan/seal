// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vfs

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/net/webdav"
)

// WebDAVFS adapts an EON virtual file system into a standard webdav.FileSystem.
type WebDAVFS struct {
	fs *FileSystem
}

// NewWebDAV constructs a WebDAV-compatible filesystem adapter.
func NewWebDAV(fs *FileSystem) *WebDAVFS {
	return &WebDAVFS{fs: fs}
}

func (w *WebDAVFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return os.ErrPermission
}

func (w *WebDAVFS) RemoveAll(ctx context.Context, name string) error {
	return os.ErrPermission
}

func (w *WebDAVFS) Rename(ctx context.Context, oldName, newName string) error {
	return os.ErrPermission
}

func (w *WebDAVFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	clean := strings.TrimPrefix(path.Clean(name), "/")
	if clean == "" || clean == "." {
		return rootDirInfo{}, nil
	}
	return fs.Stat(w.fs, clean)
}

func (w *WebDAVFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, os.ErrPermission
	}

	clean := strings.TrimPrefix(path.Clean(name), "/")
	if clean == "" || clean == "." {
		entries, err := fs.ReadDir(w.fs, ".")
		if err != nil {
			return nil, err
		}
		return &webdavDirFile{name: "/", entries: entries}, nil
	}

	st, err := fs.Stat(w.fs, clean)
	if err != nil {
		return nil, err
	}

	if st.IsDir() {
		entries, err := fs.ReadDir(w.fs, clean)
		if err != nil {
			return nil, err
		}
		return &webdavDirFile{name: clean, st: st, entries: entries}, nil
	}

	f, err := w.fs.Open(clean)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, err
	}

	return &webdavRegularFile{
		name:   clean,
		st:     st,
		reader: bytes.NewReader(data),
	}, nil
}

type rootDirInfo struct{}

func (rootDirInfo) Name() string       { return "/" }
func (rootDirInfo) Size() int64        { return 0 }
func (rootDirInfo) Mode() os.FileMode  { return os.ModeDir | 0o755 }
func (rootDirInfo) ModTime() time.Time { return time.Time{} }
func (rootDirInfo) IsDir() bool        { return true }
func (rootDirInfo) Sys() any           { return nil }

type webdavDirFile struct {
	name    string
	st      os.FileInfo
	entries []fs.DirEntry
	offset  int
}

func (d *webdavDirFile) Close() error                                 { return nil }
func (d *webdavDirFile) Read(p []byte) (int, error)                   { return 0, io.EOF }
func (d *webdavDirFile) Write(p []byte) (int, error)                  { return 0, os.ErrPermission }
func (d *webdavDirFile) Seek(offset int64, whence int) (int64, error) { return 0, nil }

func (d *webdavDirFile) Stat() (os.FileInfo, error) {
	if d.st != nil {
		return d.st, nil
	}
	return rootDirInfo{}, nil
}

func (d *webdavDirFile) Readdir(count int) ([]os.FileInfo, error) {
	if d.offset >= len(d.entries) {
		if count > 0 {
			return nil, io.EOF
		}
		return nil, nil
	}
	limit := len(d.entries)
	if count > 0 && d.offset+count < limit {
		limit = d.offset + count
	}
	var res []os.FileInfo
	for _, e := range d.entries[d.offset:limit] {
		fi, err := e.Info()
		if err == nil {
			res = append(res, fi)
		}
	}
	d.offset = limit
	return res, nil
}

type webdavRegularFile struct {
	name   string
	st     os.FileInfo
	reader *bytes.Reader
}

func (r *webdavRegularFile) Close() error                { return nil }
func (r *webdavRegularFile) Read(p []byte) (int, error)  { return r.reader.Read(p) }
func (r *webdavRegularFile) Write(p []byte) (int, error) { return 0, os.ErrPermission }
func (r *webdavRegularFile) Seek(offset int64, whence int) (int64, error) {
	return r.reader.Seek(offset, whence)
}
func (r *webdavRegularFile) Stat() (os.FileInfo, error)               { return r.st, nil }
func (r *webdavRegularFile) Readdir(count int) ([]os.FileInfo, error) { return nil, os.ErrInvalid }
