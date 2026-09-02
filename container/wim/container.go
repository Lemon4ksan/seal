// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wim

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func init() {
	container.Register(container.Wim, New)
}

// Engine implements [container.Container] for the Windows Imaging Format (.wim).
type Engine struct{}

// New constructs a new WIM [container.Container] engine.
func New() container.Container {
	return &Engine{}
}

// Type returns [container.Wim].
func (e *Engine) Type() container.Type {
	return container.Wim
}

// OpenCatalog parses the WIM header and file catalog from a random-access source.
func (e *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < HeaderSize {
		return nil, "", errors.New("wim: invalid source reader")
	}
	wr, err := NewReader(rAt, size)
	if err != nil {
		return nil, "", err
	}
	return wr.Entries, wr.XMLData, nil
}

// ScanStream sequentially buffers an incoming stream to parse its WIM catalog.
func (e *Engine) ScanStream(
	r io.Reader,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("wim: nil stream reader")
	}

	// Buffer stream to a temporary file for random access
	tmpFile, err := os.CreateTemp("", "seal-wim-stream-*.wim")
	if err != nil {
		return nil, "", err
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	size, err := io.Copy(tmpFile, r)
	if err != nil {
		return nil, "", err
	}

	return e.OpenCatalog(tmpFile, size, password, reg)
}

// WriteArchive serializes files into dest and returns the total bytes written.
func (e *Engine) WriteArchive(
	dest io.Writer,
	files []*file.File,
	cfg container.Config,
) (int64, error) {
	if dest == nil {
		return 0, errors.New("wim: nil destination writer")
	}

	// If destination is seekable, write directly
	if ws, ok := dest.(io.WriteSeeker); ok {
		writer := NewWriter(ws)
		return writer.WriteArchive(files)
	}

	// Otherwise, buffer in memory
	var buf bytes.Buffer
	writer := NewWriter(&buf)
	n, err := writer.WriteArchive(files)
	if err != nil {
		return 0, err
	}

	written, err := dest.Write(buf.Bytes())
	if err != nil {
		return int64(written), err
	}
	return n, nil
}
