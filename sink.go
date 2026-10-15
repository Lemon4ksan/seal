// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lemon4ksan/foundation/pathkit"
)

type fileSink struct {
	file *os.File
	path string
}

func (s *fileSink) Path() string {
	if s.path != "" {
		return s.path
	}
	if s.file != nil {
		return s.file.Name()
	}
	return ""
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

// AsSink wraps any compatible destination into a [Sink]:
//   - [Sink]: returned as-is
//   - string: filesystem path (wrapped via [ToFilePath])
//   - [pathkit.Path]: path (wrapped via [ToPath])
//   - [*os.File]: open file (wrapped via [ToFile])
//   - [*bytes.Buffer]: buffer (wrapped via [ToBuffer])
//   - [io.Writer]: writer (wrapped via [ToWriter])
func AsSink(v any) (Sink, error) {
	switch val := v.(type) {
	case Sink:
		return val, nil
	case string:
		return ToFilePath(val), nil
	case pathkit.Path:
		return ToPath(val), nil
	case *os.File:
		return ToFile(val), nil
	case *bytes.Buffer:
		return ToBuffer(val), nil
	case io.Writer:
		return ToWriter(val), nil
	default:
		return nil, fmt.Errorf("seal: unsupported sink type %T", v)
	}
}

// UseSink acquires an output writer from dest (which can be a [Sink], string path, [*os.File], [*bytes.Buffer], or [io.Writer]),
// executes fn, and ensures proper stream flushing and closing.
func UseSink(dest any, fn func(w io.Writer) error) (err error) {
	sink, err := AsSink(dest)
	if err != nil {
		return err
	}
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
