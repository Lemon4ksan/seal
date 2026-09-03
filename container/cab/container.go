// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cab

import (
	"bytes"
	"errors"
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func init() {
	container.Register(container.Cab, New)
}

// Engine implements [container.Container] for the Microsoft Cabinet format (.cab).
type Engine struct{}

// New constructs a new Cabinet container engine.
func New() container.Container {
	return &Engine{}
}

// Type returns [container.Cab].
func (e *Engine) Type() container.Type {
	return container.Cab
}

// OpenCatalog parses the Cabinet file catalog from a random-access source.
func (e *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	_ string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < 32 {
		return nil, "", ErrInvalidSignature
	}

	cr, err := NewReader(rAt, size)
	if err != nil {
		return nil, "", err
	}

	return cr.Entries(), "", nil
}

// ScanStream reads an incoming stream into memory to parse its catalog.
func (e *Engine) ScanStream(
	r io.Reader,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("cab: nil stream reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	return e.OpenCatalog(bytes.NewReader(data), int64(len(data)), password, reg)
}

// WriteArchive serializes the slice of files into a Microsoft Cabinet file.
func (e *Engine) WriteArchive(dest io.Writer, files []*file.File, cfg container.Config) (int64, error) {
	comp := CompTypeMSZip
	if cfg.DefaultMethod == codec.Store {
		comp = CompTypeNone
	}

	w := NewWriter(dest, comp)
	return w.WriteArchive(files, cfg)
}
