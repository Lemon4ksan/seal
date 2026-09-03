// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpio

import (
	"errors"
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func init() {
	container.Register(container.Cpio, New)
}

// Engine implements [container.Container] for the Unix CPIO archive format (.cpio).
type Engine struct{}

// New constructs a new CPIO container engine.
func New() container.Container {
	return &Engine{}
}

// Type returns [container.Cpio].
func (e *Engine) Type() container.Type {
	return container.Cpio
}

// OpenCatalog parses the CPIO file catalog from a random-access source.
func (e *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	_ string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < HeaderSizeNewc {
		return nil, "", ErrInvalidMagic
	}
	sr := io.NewSectionReader(rAt, 0, size)
	entries, err := ReadCatalog(sr)
	if err != nil {
		return nil, "", err
	}
	return entries, "", nil
}

// ScanStream reads an incoming stream sequentially to extract CPIO entries.
func (e *Engine) ScanStream(
	r io.Reader,
	_ string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("cpio: nil stream reader")
	}
	entries, err := ReadCatalog(r)
	if err != nil {
		return nil, "", err
	}
	return entries, "", nil
}

// WriteArchive serializes the slice of files into an SVR4 newc CPIO archive.
func (e *Engine) WriteArchive(dest io.Writer, files []*file.File, cfg container.Config) (int64, error) {
	w := NewWriter(dest)
	return w.WriteArchive(files, cfg)
}
