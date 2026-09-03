// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"errors"
	"io"
)

// Stage represents a bidirectional stream processing step in a pipeline.
// A Stage can transform data during writing (compression, pre-filtering, encryption)
// and during reading (decompression, post-filtering, decryption).
type Stage interface {
	Name() string
	WrapWriter(w io.Writer) (io.WriteCloser, error)
	WrapReader(r io.Reader) (io.ReadCloser, error)
}

type functionalStage struct {
	name       string
	wrapWriter func(w io.Writer) (io.WriteCloser, error)
	wrapReader func(r io.Reader) (io.ReadCloser, error)
}

func (s *functionalStage) Name() string {
	return s.name
}

func (s *functionalStage) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	if s.wrapWriter == nil {
		return &nopWriteCloser{w: w}, nil
	}
	return s.wrapWriter(w)
}

func (s *functionalStage) WrapReader(r io.Reader) (io.ReadCloser, error) {
	if s.wrapReader == nil {
		return io.NopCloser(r), nil
	}
	return s.wrapReader(r)
}

// NewStage constructs a bidirectional Stage from writer and reader wrapping functions.
func NewStage(
	name string,
	wrapW func(w io.Writer) (io.WriteCloser, error),
	wrapR func(r io.Reader) (io.ReadCloser, error),
) Stage {
	return &functionalStage{
		name:       name,
		wrapWriter: wrapW,
		wrapReader: wrapR,
	}
}

// NewFilterStage creates a Stage from simpler non-erroring writer/reader wrappers.
func NewFilterStage(name string, wrapW func(w io.Writer) io.WriteCloser, wrapR func(r io.Reader) io.ReadCloser) Stage {
	return &functionalStage{
		name: name,
		wrapWriter: func(w io.Writer) (io.WriteCloser, error) {
			if wrapW == nil {
				return &nopWriteCloser{w: w}, nil
			}
			return wrapW(w), nil
		},
		wrapReader: func(r io.Reader) (io.ReadCloser, error) {
			if wrapR == nil {
				return io.NopCloser(r), nil
			}
			return wrapR(r), nil
		},
	}
}

type nopWriteCloser struct {
	w io.Writer
}

func (n *nopWriteCloser) Write(p []byte) (int, error) {
	return n.w.Write(p)
}

func (n *nopWriteCloser) Close() error {
	if c, ok := n.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// TransformFunc applies an in-memory block transformation (e.g. XOR, bit rotation, byte delta).
type TransformFunc func(data []byte)

// NewBlockTransformStage creates a streaming Stage from a forward and inverse byte-slice transformation.
func NewBlockTransformStage(name string, forward, reverse TransformFunc) Stage {
	return &functionalStage{
		name: name,
		wrapWriter: func(w io.Writer) (io.WriteCloser, error) {
			return &transformWriter{w: w, fn: forward}, nil
		},
		wrapReader: func(r io.Reader) (io.ReadCloser, error) {
			return &transformReader{r: r, fn: reverse}, nil
		},
	}
}

type transformWriter struct {
	w  io.Writer
	fn TransformFunc
}

func (tw *transformWriter) Write(p []byte) (int, error) {
	buf := make([]byte, len(p))
	copy(buf, p)
	if tw.fn != nil {
		tw.fn(buf)
	}
	return tw.w.Write(buf)
}

func (tw *transformWriter) Close() error {
	if c, ok := tw.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

type transformReader struct {
	r  io.Reader
	fn TransformFunc
}

func (tr *transformReader) Read(p []byte) (int, error) {
	n, err := tr.r.Read(p)
	if n > 0 && tr.fn != nil {
		tr.fn(p[:n])
	}
	return n, err
}

func (tr *transformReader) Close() error {
	if c, ok := tr.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// ErrStageFailed reports an error occurring during pipeline execution.
var ErrStageFailed = errors.New("pipeline: stage execution failed")
