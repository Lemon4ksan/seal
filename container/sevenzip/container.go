// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/codec/compress/lzma"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func init() {
	container.Register(container.SevenZip, New)
}

// Engine implements [container.Container] for the 7-Zip (.7z) archive format.
type Engine struct{}

// New constructs a new 7z [container.Container] engine.
func New() container.Container {
	return &Engine{}
}

// Type returns [container.SevenZip].
func (e *Engine) Type() container.Type {
	return container.SevenZip
}

// OpenCatalog parses the 7-Zip header and file catalog from a random-access source.
func (e *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < 0 {
		return nil, "", errors.New("sevenzip: invalid source reader")
	}
	zr, err := NewReader(rAt, size, password)
	if err != nil {
		return nil, "", err
	}
	return zr.Entries(), "", nil
}

// ScanStream reads an incoming 7z stream into memory to parse its catalog.
func (e *Engine) ScanStream(
	r io.Reader,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("sevenzip: nil stream reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	return e.OpenCatalog(bytes.NewReader(data), int64(len(data)), password, reg)
}

// MemoryWriteSeeker provides an in-memory seekable byte buffer for non-seekable destinations.
type MemoryWriteSeeker struct {
	buf []byte
	pos int64
}

// Write writes bytes into the memory buffer at the current position.
func (m *MemoryWriteSeeker) Write(p []byte) (int, error) {
	end := m.pos + int64(len(p))
	if end > int64(len(m.buf)) {
		newBuf := make([]byte, end)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	copy(m.buf[m.pos:], p)
	m.pos = end
	return len(p), nil
}

// Seek moves the current read/write cursor position.
func (m *MemoryWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		m.pos = offset
	case io.SeekCurrent:
		m.pos += offset
	case io.SeekEnd:
		m.pos = int64(len(m.buf)) + offset
	}
	return m.pos, nil
}

// Bytes returns the underlying accumulated byte slice.
func (m *MemoryWriteSeeker) Bytes() []byte {
	return m.buf
}

// WriteArchive serializes the slice of files into dest and returns total bytes written.
func (e *Engine) WriteArchive(dest io.Writer, files []*file.File, cfg container.Config) (int64, error) {
	lzmaOpts := lzma.DefaultOptions()
	if cfg.DefaultLevel > 0 {
		lzmaOpts = lzma.OptionsForLevel(lzma.Level(cfg.DefaultLevel))
	}

	cpData := cfg.CheckpointData
	if len(cfg.ExtensionData) > 0 {
		cpData = cfg.ExtensionData
	}

	wOpts := WriterOptions{
		LZMA:           lzmaOpts,
		Method:         MethodLZMA2Default,
		Password:       cfg.Password,
		EncryptHeaders: cfg.Password != "",
		CheckpointData: cpData,
		Context:        cfg.Context,
		Resumable:      cfg.Resumable,
		SessionID:      cfg.SessionID,
		SourceDir:      cfg.SourceDir,
		OnFileDone:     cfg.OnFileDone,
	}

	if cfg.DefaultMethod == codec.PPMd {
		wOpts.Method = MethodPPMDDefault
	}

	switch strings.ToLower(cfg.BytecodeFilter) {
	case "bcj", "x86":
		wOpts.Filter = FilterBCJ
	case "bcj2":
		wOpts.Filter = FilterBCJ2
	case "arm":
		wOpts.Filter = FilterARM
	}

	// If dest does not implement io.WriteSeeker, buffer into memory to permit StartHeader fixup
	if _, ok := dest.(io.WriteSeeker); !ok {
		var memSeeker MemoryWriteSeeker
		w := NewWriterWithOptions(&memSeeker, wOpts)
		_, err := w.WriteArchive(files)
		if err != nil {
			return 0, err
		}
		// Write in-memory bytes to dest
		n, err := dest.Write(memSeeker.Bytes())
		if err != nil {
			return int64(n), err
		}
		return int64(len(memSeeker.Bytes())), nil
	}

	w := NewWriterWithOptions(dest, wOpts)
	return w.WriteArchive(files)
}
