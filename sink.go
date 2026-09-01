// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"bytes"
	"io"
	"os"
	"path/filepath"

	"github.com/lemon4ksan/foundation/pathkit"
)

type fileSink struct {
	file *os.File
	path string
}

func (s *fileSink) Create() (io.Writer, error) {
	if s.file != nil {
		return s.file, nil
	}

	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	s.file = f
	return f, nil
}

func (s *fileSink) Close() error {
	if s.file != nil && s.path != "" {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

// ToFile creates a [Sink] from an existing open [*os.File]. The caller retains closing responsibility.
func ToFile(f *os.File) Sink {
	return &fileSink{file: f}
}

// ToPath creates a [Sink] writing to a [pathkit.Path] on the local filesystem.
func ToPath(p pathkit.Path) Sink {
	return &fileSink{path: p.FilePath()}
}

// ToFilePath creates a [Sink] targeting a filesystem path string.
func ToFilePath(filePath string) Sink {
	return ToPath(pathkit.New(filePath))
}

type writerSink struct {
	writer io.Writer
}

func (s *writerSink) Create() (io.Writer, error) {
	return s.writer, nil
}

func (s *writerSink) Close() error {
	if c, ok := s.writer.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// ToWriter creates a [Sink] writing into an [io.Writer].
func ToWriter(w io.Writer) Sink {
	return &writerSink{writer: w}
}

// ToBuffer creates a [Sink] collecting payload into a [bytes.Buffer].
func ToBuffer(buf *bytes.Buffer) Sink {
	return &writerSink{writer: buf}
}

// UseSink acquires an output writer from sink, executes fn, and ensures proper stream flushing and closing.
func UseSink(sink Sink, fn func(w io.Writer) error) (err error) {
	w, err := sink.Create()
	if err != nil {
		return err
	}
	defer func() {
		closeErr := sink.Close()
		if err == nil {
			err = closeErr
		}
	}()

	return fn(w)
}
