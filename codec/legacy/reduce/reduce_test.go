// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reduce_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/codec/legacy/reduce"
)

func TestReduce_InvalidFactor(t *testing.T) {
	_, err0 := reduce.NewReader(bytes.NewReader(nil), 0)
	require.Error(t, err0, "expected error for factor 0")

	_, err5 := reduce.NewReader(bytes.NewReader(nil), 5)
	require.Error(t, err5, "expected error for factor 5")
}

func TestReduce_EmptyHeaderEOF(t *testing.T) {
	_, err := reduce.NewReader(bytes.NewReader(nil), 1)
	assert.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF), "expected EOF reading empty header")
}
