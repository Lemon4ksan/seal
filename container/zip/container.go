// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"errors"
	"io"

	"github.com/lemon4ksan/seal/checkpoint"
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

// OpenCatalog parses the ZIP Central Directory from a random-access source (e.g. *os.File or *bytes.Reader).
func (z *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < 0 {
		return nil, "", errors.New("zip: invalid random-access reader or size")
	}

	zr, err := NewReader(rAt, size, password, reg)
	if err != nil {
		return nil, "", err
	}

	return zr.Entries(), zr.Comment(), nil
}

// ScanStream sequentially parses ZIP Local File Headers from a stream (e.g. stdin, HTTP body).
func (z *Engine) ScanStream(
	r io.Reader,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("zip: nil stream reader")
	}

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
		f.WithData(data).
			WithCompressedSize(fe.CompressedSize()).
			WithCRC(fe.CRC()).
			WithMethod(fe.Method()).
			WithModTime(fe.ModTime())
		files = append(files, f)
	}
	return files, "", nil
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
	if len(cfg.ExtensionData) > 0 {
		w.SetCheckpointData(cfg.ExtensionData)
	} else if len(cfg.CheckpointData) > 0 {
		w.SetCheckpointData(cfg.CheckpointData)
	}

	var totalWritten int64

	for i, entry := range files {
		if cfg.Context != nil && cfg.Context.Err() != nil {
			if cfg.Resumable {
				remNames := make([]string, len(files)-i)
				for j, rem := range files[i:] {
					remNames[j] = rem.Name()
				}
				st := checkpoint.NewState(cfg.SessionID, cfg.SourceDir, string(z.Type()), len(files), i, remNames)
				if cpBytes, err := st.Encode(); err == nil {
					w.SetCheckpointData(cpBytes)
				}
				_ = w.Close()
				return totalWritten, container.ErrArchiveFrozen
			}
			return totalWritten, cfg.Context.Err()
		}

		if entry.Method() == 0 && !entry.IsDir() {
			entry.SetMethod(cfg.DefaultMethod)
		}

		// Check if raw fast-copy is possible
		if entry.CompressedSize() > 0 && entry.CRC() != 0 {
			if rawSec, err := entry.OpenRaw(); err == nil {
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
			continue
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
