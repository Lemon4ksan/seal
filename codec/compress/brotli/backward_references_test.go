// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !race

package brotli_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec/compress/brotli"
)

func TestAllocationsAmortized(t *testing.T) {
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	w.Write([]byte("test data for allocations"))
	w.Close()
	data := buf.Bytes()

	br := bytes.NewReader(data)
	r := brotli.NewReader(br)

	copyBuf := make([]byte, 32*1024)
	allocs := testing.AllocsPerRun(100, func() {
		br.Reset(data)
		r.Reset(br)
		io.CopyBuffer(io.Discard, r, copyBuf)
	})

	if allocs > 3 {
		t.Errorf("expected max 3 allocs per run with Reset, got %v", allocs)
	}
}
