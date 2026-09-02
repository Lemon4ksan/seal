// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reduce_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec/legacy/reduce"
)

func TestReduce_InvalidFactor(t *testing.T) {
	_, err0 := reduce.NewReader(bytes.NewReader(nil), 0)
	if err0 == nil {
		t.Errorf("expected error for factor 0, got nil")
	}

	_, err5 := reduce.NewReader(bytes.NewReader(nil), 5)
	if err5 == nil {
		t.Errorf("expected error for factor 5, got nil")
	}
}

func TestReduce_EmptyHeaderEOF(t *testing.T) {
	_, err := reduce.NewReader(bytes.NewReader(nil), 1)
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected EOF reading empty header, got %v", err)
	}
}
