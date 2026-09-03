// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline_test

import (
	"bytes"
	"io"
	"sync"
	"testing"

	"github.com/lemon4ksan/seal/pipeline"
)

func TestPipeline_Chain_Roundtrip(t *testing.T) {
	t.Parallel()
	original := []byte("Universal Mini-SDK for custom pipelines in Seal 2026!")

	// 1. Define custom XOR transformation stage
	xorKey := byte(0x5A)
	xorFunc := func(b []byte) {
		for i := range b {
			b[i] ^= xorKey
		}
	}
	xorStage := pipeline.NewBlockTransformStage("xor-stage", xorFunc, xorFunc)

	// 2. Define custom prefix stage
	prefix := []byte("SEAL-MAGIC:")
	prefixStage := pipeline.NewStage("prefix-stage",
		func(w io.Writer) (io.WriteCloser, error) {
			if _, err := w.Write(prefix); err != nil {
				return nil, err
			}
			return &nopWriteCloser{w: w}, nil
		},
		func(r io.Reader) (io.ReadCloser, error) {
			magic := make([]byte, len(prefix))
			if _, err := io.ReadFull(r, magic); err != nil {
				return nil, err
			}
			if !bytes.Equal(magic, prefix) {
				t.Fatalf("Prefix mismatch: got %q, want %q", magic, prefix)
			}
			return io.NopCloser(r), nil
		},
	)

	// 3. Assemble Chain
	chain := pipeline.NewChain().Add(prefixStage).Add(xorStage)

	// Test compression/encoding
	var encodedBuf bytes.Buffer
	wc, err := chain.WrapWriter(&encodedBuf)
	if err != nil {
		t.Fatalf("WrapWriter failed: %v", err)
	}
	if _, err := wc.Write(original); err != nil {
		t.Fatalf("Writing to chain: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Closing chain writer: %v", err)
	}

	// Encoded output must not equal original
	if bytes.Equal(encodedBuf.Bytes(), original) {
		t.Fatal("Expected encoded output to be transformed")
	}

	// Test decoding
	rc, err := chain.WrapReader(&encodedBuf)
	if err != nil {
		t.Fatalf("WrapReader failed: %v", err)
	}
	decoded, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Reading from chain: %v", err)
	}
	_ = rc.Close()

	if !bytes.Equal(decoded, original) {
		t.Fatalf("Decoded content mismatch: got %q, want %q", string(decoded), string(original))
	}
}

type mockSplitter struct{}

func (m *mockSplitter) NumOutputs() int { return 3 }
func (m *mockSplitter) Split(src io.Reader, targets []io.Writer) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	for i, b := range data {
		targetIdx := i % 3
		if _, err := targets[targetIdx].Write([]byte{b}); err != nil {
			return err
		}
	}
	return nil
}

type mockMerger struct{}

func (m *mockMerger) NumInputs() int { return 3 }
func (m *mockMerger) Merge(sources []io.Reader, dest io.Writer) error {
	streams := make([][]byte, 3)
	for i, s := range sources {
		b, err := io.ReadAll(s)
		if err != nil {
			return err
		}
		streams[i] = b
	}

	maxLen := 0
	for _, s := range streams {
		if len(s) > maxLen {
			maxLen = len(s)
		}
	}

	for i := 0; i < maxLen; i++ {
		for sIdx := 0; sIdx < 3; sIdx++ {
			if i < len(streams[sIdx]) {
				if _, err := dest.Write([]byte{streams[sIdx][i]}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func TestPipeline_ForkJoinDAG(t *testing.T) {
	t.Parallel()
	original := []byte("Parallel multi-stream fork-join execution engine in Go!")

	dag := pipeline.NewForkJoinDAG(&mockSplitter{}, &mockMerger{})

	var processedCount int
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		dag.WithBranchWorker(i, func(idx int, r io.Reader, w io.Writer) error {
			mu.Lock()
			processedCount++
			mu.Unlock()
			_, err := io.Copy(w, r)
			return err
		})
	}

	var dest bytes.Buffer
	if err := dag.Execute(bytes.NewReader(original), &dest); err != nil {
		t.Fatalf("DAG Execute failed: %v", err)
	}

	if processedCount != 3 {
		t.Fatalf("Expected all 3 branches to be processed, got %d", processedCount)
	}

	if !bytes.Equal(dest.Bytes(), original) {
		t.Fatalf("DAG roundtrip mismatch: got %q, want %q", dest.String(), string(original))
	}
}

type nopWriteCloser struct {
	w io.Writer
}

func (n *nopWriteCloser) Write(p []byte) (int, error) {
	return n.w.Write(p)
}

func (n *nopWriteCloser) Close() error {
	return nil
}
