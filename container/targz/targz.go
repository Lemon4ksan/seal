// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package targz provides composite pipelines for reading and writing compressed TAR archive streams.
package targz

import (
	"io"

	"github.com/lemon4ksan/foundation/codec/compress/gzip"
	"github.com/lemon4ksan/seal/container/tar"
)

// Reader reads records from a GZIP-compressed TAR stream.
type Reader struct {
	gzReader  *gzip.Reader
	tarReader *tar.Reader
}

// NewReader constructs a compressed TAR reader.
func NewReader(r io.Reader) (*Reader, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &Reader{
		gzReader:  gz,
		tarReader: tar.NewReader(gz),
	}, nil
}

// Next advances to the next entry in the compressed TAR archive.
func (r *Reader) Next() (*tar.Header, io.Reader, error) {
	return r.tarReader.Next()
}

// Close closes the underlying decompression stream.
func (r *Reader) Close() error {
	return r.gzReader.Close()
}

// Writer constructs a GZIP-compressed TAR stream.
type Writer struct {
	gzWriter  *gzip.Writer
	tarWriter *tar.Writer
}

// NewWriter constructs a compressed TAR writer.
func NewWriter(w io.Writer, level int) (*Writer, error) {
	if level == 0 {
		level = gzip.DefaultCompression
	}
	gz, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		return nil, err
	}
	return &Writer{
		gzWriter:  gz,
		tarWriter: tar.NewWriter(gz),
	}, nil
}

// WriteHeader writes an entry header to the TAR archive.
func (w *Writer) WriteHeader(entry *tar.Header) error {
	return w.tarWriter.WriteHeader(entry)
}

// WritePayload copies payload data into the compressed stream.
func (w *Writer) WritePayload(src io.Reader) (int64, error) {
	return w.tarWriter.WritePayload(src)
}

// Close flushes the TAR stream and GZIP container footer.
func (w *Writer) Close() error {
	if err := w.tarWriter.Close(); err != nil {
		return err
	}
	return w.gzWriter.Close()
}
