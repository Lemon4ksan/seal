// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iso

import (
	"bytes"
	"errors"
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func init() {
	container.Register(container.Iso, New)
}

// Engine implements [container.Container] for the ISO 9660 / Joliet optical disc image format (.iso, .img).
type Engine struct{}

// New constructs a new ISO 9660 container engine.
func New() container.Container {
	return &Engine{}
}

// Type returns [container.Iso].
func (e *Engine) Type() container.Type {
	return container.Iso
}

// OpenCatalog parses the ISO file catalog from a random-access source.
func (e *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	_ string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < int64(SystemAreaSize+SectorSize) {
		return nil, "", ErrInvalidISO
	}

	ir, err := NewReader(rAt, size)
	if err != nil {
		return nil, "", err
	}

	return ir.Entries(), ir.VolumeID(), nil
}

// ScanStream reads an incoming stream into memory to parse its catalog.
func (e *Engine) ScanStream(
	r io.Reader,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("iso: nil stream reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	return e.OpenCatalog(bytes.NewReader(data), int64(len(data)), password, reg)
}

// WriteArchive serializes the slice of files into a compliant ISO 9660 disc image.
func (e *Engine) WriteArchive(dest io.Writer, files []*file.File, cfg container.Config) (int64, error) {
	w := NewWriter(dest)
	return w.WriteArchive(files, cfg)
}
