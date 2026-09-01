// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"errors"
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

// Engine implements [container.Container] for the PKWARE ZIP archive format.
type Engine struct{}

// New constructs a new ZIP [container.Container] engine.
func New() container.Container {
	return &Engine{}
}

// NewEngine constructs a new concrete [*Engine] instance.
func NewEngine() *Engine {
	return &Engine{}
}

// Type returns [container.Zip].
func (z *Engine) Type() container.Type {
	return container.Zip
}

// ReadIndex parses the table of contents and file entries from a random-access or streaming reader.
func (z *Engine) ReadIndex(
	r io.Reader,
	rAt io.ReaderAt,
	size int64,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if rAt != nil && size > 0 {
		zr, err := NewReader(rAt, size, password, reg)
		if err != nil {
			return nil, "", err
		}
		return zr.Entries(), zr.Comment(), nil
	}

	if r != nil {
		sr := NewStreamReader(r, password, reg)
		var files []*file.File
		for {
			fe, payloadR, err := sr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, "", err
			}

			// Buffer content into memory for streaming entry
			data, err := io.ReadAll(payloadR)
			if err != nil {
				return nil, "", err
			}

			f, _ := file.New(fe.Name(), fe.IsDir())
			f.SetSizes(int64(len(data)), fe.CompressedSize(), fe.CRC())
			f.SetMethod(fe.Method())
			f.WithModTime(fe.ModTime())
			f.SetOpenFunc(func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(data)), nil
			})
			files = append(files, f)
		}
		return files, "", nil
	}

	return nil, "", errors.New("zip: invalid source reader")
}

// WriteArchive serializes the entries into dest and returns the total bytes written.
func (z *Engine) WriteArchive(dest io.Writer, files []*file.File, cfg container.Config) (int64, error) {
	w := NewWriter(dest, cfg.Registry)
	if cfg.Comment != "" {
		w.SetComment(cfg.Comment)
	}
	if cfg.Password != "" {
		w.SetPassword(cfg.Password)
	}

	var totalWritten int64

	for _, entry := range files {
		if entry.Method() == 0 && !entry.IsDir() {
			entry.SetMethod(cfg.DefaultMethod)
		}

		// Check if raw fast-copy is possible
		if entry.CompressedSize() > 0 && entry.CRC() != 0 {
			rawSec, err := entry.OpenRaw()
			if err == nil {
				if err := w.WriteEntryRaw(entry, rawSec); err == nil {
					totalWritten += entry.CompressedSize()
					if cfg.OnFileDone != nil {
						cfg.OnFileDone(entry, nil)
					}
					continue
				}
			}
		}

		rc, err := entry.Open()
		if err != nil {
			if cfg.OnFileDone != nil {
				cfg.OnFileDone(entry, err)
			}
			return totalWritten, err
		}

		err = w.WriteEntry(entry, rc)
		_ = rc.Close()

		if err != nil {
			if cfg.OnFileDone != nil {
				cfg.OnFileDone(entry, err)
			}
			return totalWritten, err
		}

		totalWritten += entry.CompressedSize()
		if cfg.OnFileDone != nil {
			cfg.OnFileDone(entry, nil)
		}
	}

	if err := w.Close(); err != nil {
		return totalWritten, err
	}

	return totalWritten, nil
}
