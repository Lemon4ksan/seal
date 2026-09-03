// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rar

import (
	"bytes"
	"errors"
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func init() {
	container.Register(container.Rar, New)
}

// Engine implements [container.Container] for the RAR archive format (.rar).
type Engine struct{}

// New constructs a new RAR [container.Container] engine.
func New() container.Container {
	return &Engine{}
}

// Type returns [container.Rar].
func (e *Engine) Type() container.Type {
	return container.Rar
}

// OpenCatalog parses the RAR file catalog from a random-access source.
func (e *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	password string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < 7 {
		return nil, "", errors.New("rar: invalid source reader")
	}
	rr, err := NewReader(rAt, size, password)
	if err != nil {
		return nil, "", err
	}
	return rr.Entries(), rr.Comment(), nil
}

// ScanStream reads an incoming RAR stream into memory to parse its catalog.
func (e *Engine) ScanStream(
	r io.Reader,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("rar: nil stream reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	return e.OpenCatalog(bytes.NewReader(data), int64(len(data)), password, reg)
}

// WriteArchive is not supported for RAR archives due to proprietary licensing restrictions.
func (e *Engine) WriteArchive(
	_ io.Writer,
	_ []*file.File,
	_ container.Config,
) (int64, error) {
	return 0, ErrCreateNotSupported
}
