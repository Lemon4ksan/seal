// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/internal/pool"
)

// Chain defines an ordered sequence of bidirectional transformation stages.
//
// When writing, data flows forward: Stage 0 -> Stage 1 -> ... -> Stage N-1 -> Dest.
// When reading, data flows in reverse: Src -> Stage N-1 -> ... -> Stage 1 -> Stage 0.
type Chain struct {
	stages []Stage
}

// NewChain instantiates a new transformation Chain.
func NewChain(stages ...Stage) *Chain {
	return &Chain{stages: stages}
}

// Add appends one or more Stages to the Chain.
func (c *Chain) Add(stages ...Stage) *Chain {
	c.stages = append(c.stages, stages...)
	return c
}

// Stages returns a slice of all registered stages in execution order.
func (c *Chain) Stages() []Stage {
	return c.stages
}

// WrapWriter wraps dest with all stages in reverse nesting so data flows from Stage 0 to Stage N-1.
// The caller writes raw data to the returned io.WriteCloser.
func (c *Chain) WrapWriter(dest io.Writer) (io.WriteCloser, error) {
	curr := dest
	var closers []io.Closer

	// Outer-most wrapper receives from inner-most stage and writes to dest.
	// Therefore, Stage N-1 wraps dest first, then Stage N-2 wraps Stage N-1, etc.
	for i := len(c.stages) - 1; i >= 0; i-- {
		s := c.stages[i]
		wc, err := s.WrapWriter(curr)
		if err != nil {
			// Close any already opened closers on failure
			_ = closeAll(closers)
			return nil, fmt.Errorf("pipeline: wrap writer stage %q failed: %w", s.Name(), err)
		}
		curr = wc
		closers = append(closers, wc)
	}

	return &compositeWriteCloser{
		Writer:  curr,
		closers: closers,
	}, nil
}

// WrapReader wraps src with all stages in forward nesting so data flows from Stage N-1 to Stage 0.
// The caller reads transformed data from the returned io.ReadCloser.
func (c *Chain) WrapReader(src io.Reader) (io.ReadCloser, error) {
	curr := src
	var closers []io.Closer

	for i := len(c.stages) - 1; i >= 0; i-- {
		s := c.stages[i]
		rc, err := s.WrapReader(curr)
		if err != nil {
			_ = closeAll(closers)
			return nil, fmt.Errorf("pipeline: wrap reader stage %q failed: %w", s.Name(), err)
		}
		curr = rc
		closers = append(closers, rc)
	}

	return &compositeReadCloser{
		Reader:  curr,
		closers: closers,
	}, nil
}

// Execute streams all bytes from src through the chain to dest.
func (c *Chain) Execute(src io.Reader, dest io.Writer) (int64, error) {
	return c.ExecuteContext(context.Background(), src, dest)
}

// ExecuteContext streams data through the chain with support for cancellation.
func (c *Chain) ExecuteContext(ctx context.Context, src io.Reader, dest io.Writer) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	wc, err := c.WrapWriter(dest)
	if err != nil {
		return 0, err
	}

	buf := pool.Get64K()
	defer pool.Put64K(buf)

	var written int64
	for {
		if err := ctx.Err(); err != nil {
			_ = wc.Close()
			return written, err
		}
		nr, rErr := src.Read(*buf)
		if nr > 0 {
			nw, wErr := wc.Write((*buf)[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if wErr != nil {
				_ = wc.Close()
				return written, wErr
			}
		}
		if rErr != nil {
			if errors.Is(rErr, io.EOF) {
				break
			}
			_ = wc.Close()
			return written, rErr
		}
	}

	closeErr := wc.Close()
	return written, closeErr
}

type compositeWriteCloser struct {
	io.Writer
	closers []io.Closer
}

func (c *compositeWriteCloser) Close() error {
	return closeAll(c.closers)
}

type compositeReadCloser struct {
	io.Reader
	closers []io.Closer
}

func (c *compositeReadCloser) Close() error {
	return closeAll(c.closers)
}

func closeAll(closers []io.Closer) error {
	var firstErr error
	for i := len(closers) - 1; i >= 0; i-- {
		if c := closers[i]; c != nil {
			if err := c.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
