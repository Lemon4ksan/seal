// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Splitter splits a single input stream into N separate output streams.
type Splitter interface {
	NumOutputs() int
	Split(src io.Reader, targets []io.Writer) error
}

// Merger combines N input streams back into a single output stream.
type Merger interface {
	NumInputs() int
	Merge(sources []io.Reader, dest io.Writer) error
}

// BranchWorker defines a transformation applied to an individual branch in a DAG.
type BranchWorker func(branchIndex int, src io.Reader, dest io.Writer) error

// ParallelBranchExecutor processes N streams concurrently across goroutines.
type ParallelBranchExecutor struct {
	maxWorkers int
}

// NewParallelBranchExecutor creates an executor. If maxWorkers <= 0, unbounded goroutines are used.
func NewParallelBranchExecutor(maxWorkers int) *ParallelBranchExecutor {
	return &ParallelBranchExecutor{maxWorkers: maxWorkers}
}

// Execute runs worker across all branches in parallel.
func (pbe *ParallelBranchExecutor) Execute(sources []io.Reader, targets []io.Writer, worker BranchWorker) error {
	return pbe.ExecuteContext(context.Background(), sources, targets, worker)
}

// ExecuteContext runs worker across branches honoring context cancellation.
func (pbe *ParallelBranchExecutor) ExecuteContext(
	ctx context.Context,
	sources []io.Reader,
	targets []io.Writer,
	worker BranchWorker,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(sources) != len(targets) {
		return errors.New("pipeline: sources and targets count mismatch")
	}

	n := len(sources)
	if n == 0 {
		return nil
	}

	var wg sync.WaitGroup
	var errOnce sync.Once
	var firstErr error

	var sem chan struct{}
	if pbe.maxWorkers > 0 {
		sem = make(chan struct{}, pbe.maxWorkers)
	}

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if sem != nil {
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					errOnce.Do(func() { firstErr = ctx.Err() })
					return
				}
			}

			if err := ctx.Err(); err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}

			if err := worker(idx, sources[idx], targets[idx]); err != nil {
				errOnce.Do(func() {
					firstErr = fmt.Errorf("pipeline: branch %d failed: %w", idx, err)
				})
			}
		}(i)
	}

	wg.Wait()
	return firstErr
}

// ForkJoinDAG defines a 1-to-N-to-1 or 1-to-N stream processing graph.
type ForkJoinDAG struct {
	splitter Splitter
	workers  []BranchWorker
	merger   Merger
	executor *ParallelBranchExecutor
}

// NewForkJoinDAG creates an empty ForkJoinDAG.
func NewForkJoinDAG(splitter Splitter, merger Merger) *ForkJoinDAG {
	return &ForkJoinDAG{
		splitter: splitter,
		merger:   merger,
		workers:  make([]BranchWorker, splitter.NumOutputs()),
		executor: NewParallelBranchExecutor(0),
	}
}

// WithBranchWorker registers a transformation worker for a specific branch index.
func (dag *ForkJoinDAG) WithBranchWorker(branchIndex int, worker BranchWorker) *ForkJoinDAG {
	if branchIndex >= 0 && branchIndex < len(dag.workers) {
		dag.workers[branchIndex] = worker
	}
	return dag
}

// ProcessFork runs the splitter and processes all branches concurrently, returning the branch buffers.
func (dag *ForkJoinDAG) ProcessFork(src io.Reader) ([]*bytes.Buffer, error) {
	return dag.ProcessForkContext(context.Background(), src)
}

// ProcessForkContext runs the splitter and processes all branches concurrently with cancellation support.
func (dag *ForkJoinDAG) ProcessForkContext(ctx context.Context, src io.Reader) ([]*bytes.Buffer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	numOut := dag.splitter.NumOutputs()
	splitTargets := make([]io.Writer, numOut)
	splitBufs := make([]bytes.Buffer, numOut)
	for i := 0; i < numOut; i++ {
		splitTargets[i] = &splitBufs[i]
	}

	// 1. Split source into N buffers
	if err := dag.splitter.Split(src, splitTargets); err != nil {
		return nil, fmt.Errorf("pipeline: split failed: %w", err)
	}

	// 2. Prepare branch sources and targets
	branchSources := make([]io.Reader, numOut)
	branchTargets := make([]io.Writer, numOut)
	resultBufs := make([]*bytes.Buffer, numOut)

	for i := 0; i < numOut; i++ {
		branchSources[i] = &splitBufs[i]
		buf := &bytes.Buffer{}
		resultBufs[i] = buf
		branchTargets[i] = buf
	}

	// 3. Process branches concurrently
	err := dag.executor.ExecuteContext(
		ctx,
		branchSources,
		branchTargets,
		func(idx int, r io.Reader, w io.Writer) error {
			if worker := dag.workers[idx]; worker != nil {
				return worker(idx, r, w)
			}
			// Default: pass-through copy
			_, copyErr := io.Copy(w, r)
			return copyErr
		},
	)
	if err != nil {
		return nil, err
	}

	return resultBufs, nil
}

// Execute processes the complete Fork -> Parallel Branches -> Merge pipeline.
func (dag *ForkJoinDAG) Execute(src io.Reader, dest io.Writer) error {
	return dag.ExecuteContext(context.Background(), src, dest)
}

// ExecuteContext processes the pipeline with context cancellation support.
func (dag *ForkJoinDAG) ExecuteContext(ctx context.Context, src io.Reader, dest io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if dag.merger == nil {
		return errors.New("pipeline: merger is required for Execute")
	}

	branchBufs, err := dag.ProcessForkContext(ctx, src)
	if err != nil {
		return err
	}

	mergeSources := make([]io.Reader, len(branchBufs))
	for i, b := range branchBufs {
		mergeSources[i] = b
	}

	return dag.merger.Merge(mergeSources, dest)
}
