// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tar

import (
	"bytes"
	"errors"
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

// Engine implements [container.Container] for the POSIX / GNU / USTAR TAR tape archive format.
type Engine struct{}

// New constructs a new TAR [container.Container] engine.
func New() container.Container {
	return &Engine{}
}

// NewEngine constructs a new concrete [*Engine] instance.
func NewEngine() *Engine {
	return &Engine{}
}

// Type returns [container.Tar].
func (t *Engine) Type() container.Type {
	return container.Tar
}

// ReadIndex parses the table of contents and file entries from a reader or random-access source.
func (t *Engine) ReadIndex(
	r io.Reader,
	rAt io.ReaderAt,
	size int64,
	_ string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		if rAt != nil && size > 0 {
			r = io.NewSectionReader(rAt, 0, size)
		} else {
			return nil, "", errors.New("tar: invalid source reader")
		}
	}

	tr := NewReader(r)
	var files []*file.File

	for {
		entry, payloadR, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", err
		}

		data, err := io.ReadAll(payloadR)
		if err != nil {
			return nil, "", err
		}

		f, _ := file.New(entry.Name, entry.IsDir)
		f.SetSizes(int64(len(data)), int64(len(data)), 0)
		f.WithMode(entry.Mode)
		f.WithModTime(entry.ModTime)
		f.SetOpenFunc(func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		})
		files = append(files, f)
	}

	return files, "", nil
}

// WriteArchive serializes the entries into dest and returns the total bytes written.
func (t *Engine) WriteArchive(dest io.Writer, files []*file.File, cfg container.Config) (int64, error) {
	tw := NewWriter(dest)
	var totalWritten int64

	for _, entry := range files {
		tarEntry := &Entry{
			Name:    entry.Name(),
			Size:    entry.UncompressedSize(),
			Mode:    entry.Mode(),
			ModTime: entry.ModTime(),
			IsDir:   entry.IsDir(),
		}

		if err := tw.WriteHeader(tarEntry); err != nil {
			if cfg.OnFileDone != nil {
				cfg.OnFileDone(entry, err)
			}
			return totalWritten, err
		}

		if !entry.IsDir() {
			rc, err := entry.Open()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(entry, err)
				}
				return totalWritten, err
			}

			n, err := tw.WritePayload(rc)
			_ = rc.Close()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(entry, err)
				}
				return totalWritten, err
			}
			totalWritten += n
		}

		if cfg.OnFileDone != nil {
			cfg.OnFileDone(entry, nil)
		}
	}

	if err := tw.Close(); err != nil {
		return totalWritten, err
	}

	return totalWritten, nil
}
