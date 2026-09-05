// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package vfs provides an io/fs.FS compatible virtual file system layer over EON containers.
package vfs

import (
	"bytes"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/foundation/vfs"
)

// FSReader exposes the minimal reader interface required by the VFS layer.
type FSReader interface {
	Extents() []catalog.Extent
	ReadExtent(name string) ([]byte, error)
}

type eonEntryProvider struct {
	r FSReader
}

func (p *eonEntryProvider) GetEntries() []vfs.VFSEntry {
	exts := p.r.Extents()
	entries := make([]vfs.VFSEntry, 0, len(exts))
	for _, ext := range exts {
		if ext.Namespace == spec.NamespaceVFS {
			entries = append(entries, &eonEntry{ext: ext, r: p.r})
		}
	}
	return entries
}

func (p *eonEntryProvider) FindEntry(name string) (vfs.VFSEntry, bool) {
	clean := vfs.CleanPath(name)
	for _, ext := range p.r.Extents() {
		if ext.Namespace == spec.NamespaceVFS && vfs.CleanPath(ext.Name) == clean {
			return &eonEntry{ext: ext, r: p.r}, true
		}
	}
	return nil, false
}

type eonEntry struct {
	ext catalog.Extent
	r   FSReader
}

func (e *eonEntry) EntryName() string {
	return e.ext.Name
}

func (e *eonEntry) EntrySize() int64 {
	return int64(e.ext.PayloadSize)
}

func (e *eonEntry) EntryMode() fs.FileMode {
	if strings.HasSuffix(e.ext.Name, "/") {
		return fs.ModeDir | 0o755
	}
	return 0o644
}

func (e *eonEntry) EntryModTime() time.Time {
	if e.ext.Timestamp > 0 {
		return time.Unix(0, e.ext.Timestamp)
	}
	return time.Time{}
}

func (e *eonEntry) EntryIsDir() bool {
	return strings.HasSuffix(e.ext.Name, "/")
}

func (e *eonEntry) OpenStream() (io.ReadCloser, error) {
	data, err := e.r.ReadExtent(e.ext.Name)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// FileSystem wraps an EON Reader using the foundation/vfs engine.
type FileSystem struct {
	*vfs.FS
}

// New constructs a virtual file system view backed by r.
func New(r FSReader) *FileSystem {
	provider := &eonEntryProvider{r: r}
	return &FileSystem{FS: vfs.NewFS(provider)}
}

var (
	_ fs.FS         = (*FileSystem)(nil)
	_ fs.StatFS     = (*FileSystem)(nil)
	_ fs.ReadFileFS = (*FileSystem)(nil)
	_ fs.ReadDirFS  = (*FileSystem)(nil)
)
