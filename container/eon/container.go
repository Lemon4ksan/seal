// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eon

import (
	"bytes"
	"errors"
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func init() {
	container.Register(container.Eon, New)
}

// Engine implements [container.Container] for the EON container format (.eon).
type Engine struct{}

// New constructs a new EON container engine.
func New() container.Container {
	return &Engine{}
}

// Type returns [container.Eon].
func (e *Engine) Type() container.Type {
	return container.Eon
}

// OpenCatalog parses the EON catalog from a random-access source.
func (e *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	_ string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < HeaderSize+TrailerSize {
		return nil, "", ErrInvalidTrailer
	}
	r, err := NewReader(rAt, size)
	if err != nil {
		return nil, "", err
	}
	return r.Entries(), "", nil
}

// ScanStream sequentially scans an incoming stream into memory to parse its catalog.
func (e *Engine) ScanStream(
	r io.Reader,
	_ string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("eon: nil stream reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	readerAt := bytes.NewReader(data)
	return e.OpenCatalog(readerAt, int64(len(data)), "", nil)
}

// WriteArchive serializes files into an EON container archive stream.
func (e *Engine) WriteArchive(dest io.Writer, files []*file.File, cfg container.Config) (int64, error) {
	w := NewWriter(dest)
	return w.WriteArchive(files, cfg)
}
