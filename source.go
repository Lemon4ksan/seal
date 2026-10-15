// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/lemon4ksan/foundation/pathkit"
)

type fileSource struct {
	file *os.File
	path string
}

func (s *fileSource) Path() string {
	if s.path != "" {
		return s.path
	}
	if s.file != nil {
		return s.file.Name()
	}
	return ""
}

func (s *fileSource) Open() (io.Reader, int64, error) {
	if s.file != nil {
		stat, err := s.file.Stat()
		if err != nil {
			return nil, 0, err
		}
		return s.file, stat.Size(), nil
	}

	f, err := os.Open(s.path)
	if err != nil {
		return nil, 0, err
	}
	s.file = f
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, stat.Size(), nil
}

func (s *fileSource) Close() error {
	if s.file != nil && s.path != "" {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

// FromFile creates a [Source] from an open [*os.File]. The caller retains ownership of closing the file.
func FromFile(f *os.File) Source {
	return &fileSource{file: f}
}

// FromPath creates a polymorphic [Source] from a [pathkit.Path].
// Automatically dispatches between local filesystem files and remote network URLs.
func FromPath(p pathkit.Path, client ...*http.Client) Source {
	if p.IsURL() {
		var c *http.Client
		if len(client) > 0 {
			c = client[0]
		}
		return FromURL(p.String(), c)
	}
	return &fileSource{path: p.FilePath()}
}

// FromFilePath creates a [Source] from a local filesystem path or file:// URI string.
func FromFilePath(filePath string) Source {
	return FromPath(pathkit.New(filePath))
}

type bytesSource struct {
	data []byte
}

func (s *bytesSource) Open() (io.Reader, int64, error) {
	return bytes.NewReader(s.data), int64(len(s.data)), nil
}

func (s *bytesSource) Close() error { return nil }

// FromBytes creates a [Source] backed by an in-memory byte slice.
func FromBytes(b []byte) Source {
	return &bytesSource{data: b}
}

type readerSource struct {
	reader io.Reader
	size   int64
}

func (s *readerSource) Open() (io.Reader, int64, error) {
	return s.reader, s.size, nil
}

func (s *readerSource) Close() error {
	if c, ok := s.reader.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// FromReader creates a [Source] from an [io.Reader] with an explicit size (or -1 if unknown).
func FromReader(r io.Reader, size int64) Source {
	return &readerSource{reader: r, size: size}
}

type httpSource struct {
	url    string
	client *http.Client
	resp   *http.Response
}

func (s *httpSource) Open() (io.Reader, int64, error) {
	req, err := http.NewRequest(http.MethodGet, s.url, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, 0, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	s.resp = resp
	return resp.Body, resp.ContentLength, nil
}

func (s *httpSource) Close() error {
	if s.resp != nil && s.resp.Body != nil {
		return s.resp.Body.Close()
	}
	return nil
}

// FromURL creates a [Source] streaming directly from an HTTP/HTTPS endpoint.
func FromURL(url string, client *http.Client) Source {
	if client == nil {
		client = http.DefaultClient
	}
	return &httpSource{url: url, client: client}
}

// AsSource wraps any compatible source into a [Source]:
//   - [Source]: returned as-is
//   - string: filesystem path or URL (wrapped via [FromFilePath] or [FromURL])
//   - [pathkit.Path]: path (wrapped via [FromPath])
//   - [*os.File]: open file (wrapped via [FromFile])
//   - []byte: byte slice (wrapped via [FromBytes])
//   - [io.Reader]: stream (wrapped via [FromReader] with size -1)
func AsSource(v any) (Source, error) {
	switch val := v.(type) {
	case Source:
		return val, nil
	case string:
		return FromFilePath(val), nil
	case pathkit.Path:
		return FromPath(val), nil
	case *os.File:
		return FromFile(val), nil
	case []byte:
		return FromBytes(val), nil
	case io.Reader:
		return FromReader(val, -1), nil
	default:
		return nil, fmt.Errorf("seal: unsupported source type %T", v)
	}
}

// UseSource opens the source (which can be a [Source], string path, [*os.File], []byte, or [io.Reader]),
// dispatches random-access or stream readers, and guarantees cleanup.
func UseSource(src any, fn func(r io.Reader, rAt io.ReaderAt, size int64) error) (err error) {
	s, err := AsSource(src)
	if err != nil {
		return err
	}
	r, size, err := s.Open()
	if err != nil {
		return err
	}
	defer func() {
		closeErr := s.Close()
		if err == nil {
			err = closeErr
		}
	}()

	var rAt io.ReaderAt
	if ra, ok := r.(io.ReaderAt); ok {
		rAt = ra
	}

	return fn(r, rAt, size)
}
