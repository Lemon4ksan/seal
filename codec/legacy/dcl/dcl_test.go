// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dcl_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/codec/legacy/dcl"
)

func TestDCL_Empty(t *testing.T) {
	r := dcl.NewReader(bytes.NewReader(nil))
	defer r.Close()

	buf := make([]byte, 16)
	_, err := r.Read(buf)
	assert.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF), "expected EOF or ErrUnexpectedEOF")
}

func TestDCL_BlastVector(t *testing.T) {
	// Official test vector from Mark Adler's blast.c / PKWARE DCL specification
	// Decompresses to "AIAIAIAIAIAIA"
	data := []byte{0x00, 0x04, 0x82, 0x24, 0x25, 0x8f, 0x80, 0x7f}

	r := dcl.NewReader(bytes.NewReader(data))
	defer r.Close()

	out, err := io.ReadAll(r)
	require.NoError(t, err)

	expected := "AIAIAIAIAIAIA"
	assert.Equal(t, expected, string(out))
}
