// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"hash"
	"io"

	"github.com/lemon4ksan/seal/internal/pool"
)

// Result reports execution statistics returned by the pipeline.
type Result struct {
	UncompressedBytes int64
	CompressedBytes   int64
	Checksum          uint32
	HashSum           []byte
}

// WritePipeline chains checksumming, pre-filtering, compression, and encryption into a single execution stream.
type WritePipeline struct {
	dest       io.Writer
	hasher     hash.Hash
	filters    []func(w io.Writer) io.WriteCloser
	compressor func(src io.Reader, dst io.Writer) (int64, error)
	cipher     func(w io.Writer) (io.WriteCloser, error)
}

// NewWritePipeline constructs a write pipeline writing to dest.
func NewWritePipeline(dest io.Writer) *WritePipeline {
	return &WritePipeline{dest: dest}
}

// WithHasher attaches a checksum calculator on the raw uncompressed input stream.
func (p *WritePipeline) WithHasher(h hash.Hash) *WritePipeline {
	p.hasher = h
	return p
}

// WithFilter attaches a pre-processing stream filter (e.g. BCJ x86 / Delta).
func (p *WritePipeline) WithFilter(f func(w io.Writer) io.WriteCloser) *WritePipeline {
	p.filters = append(p.filters, f)
	return p
}

// WithCompressor attaches a stream compressor function.
func (p *WritePipeline) WithCompressor(comp func(src io.Reader, dst io.Writer) (int64, error)) *WritePipeline {
	p.compressor = comp
	return p
}

// WithCipher attaches a stream encryption wrapper.
func (p *WritePipeline) WithCipher(cipher func(w io.Writer) (io.WriteCloser, error)) *WritePipeline {
	p.cipher = cipher
	return p
}

// Execute streams data from src through all chained pipeline stages to dest.
func (p *WritePipeline) Execute(src io.Reader) (Result, error) {
	var res Result

	// 1. Wrap uncompressed source stream with hasher
	inReader := src
	if p.hasher != nil {
		p.hasher.Reset()
		inReader = io.TeeReader(src, p.hasher)
	}

	// 2. Wrap output stream with byte counter
	byteCounter := &countWriter{w: p.dest}
	var outWriter io.Writer = byteCounter

	// 3. Wrap output with cipher if configured
	var closers []io.Closer
	if p.cipher != nil {
		encW, err := p.cipher(outWriter)
		if err != nil {
			return res, err
		}
		outWriter = encW
		closers = append(closers, encW)
	}

	// 4. Wrap with filters if configured
	for i := len(p.filters) - 1; i >= 0; i-- {
		fltW := p.filters[i](outWriter)
		outWriter = fltW
		closers = append(closers, fltW)
	}

	// 5. Execute compression or direct stream copy
	if p.compressor != nil {
		uncompBytes, err := p.compressor(inReader, outWriter)
		if err != nil {
			_ = p.closeAll(closers)
			return res, err
		}
		res.UncompressedBytes = uncompBytes
	} else {
		buf := pool.Get64K()
		defer pool.Put64K(buf)
		written, err := io.CopyBuffer(outWriter, inReader, *buf)
		if err != nil {
			_ = p.closeAll(closers)
			return res, err
		}
		res.UncompressedBytes = written
	}

	// 6. Flush and close wrapped stages in reverse order
	if err := p.closeAll(closers); err != nil {
		return res, err
	}

	res.CompressedBytes = byteCounter.count
	if p.hasher != nil {
		res.HashSum = p.hasher.Sum(nil)
		if h32, ok := p.hasher.(hash.Hash32); ok {
			res.Checksum = h32.Sum32()
		}
	}

	return res, nil
}

func (p *WritePipeline) closeAll(closers []io.Closer) error {
	for i := len(closers) - 1; i >= 0; i-- {
		if err := closers[i].Close(); err != nil {
			return err
		}
	}
	return nil
}

type countWriter struct {
	w     io.Writer
	count int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.count += int64(n)
	}
	return n, err
}
