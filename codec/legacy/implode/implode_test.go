// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package implode_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/codec/legacy/implode"
)

func TestImplode_Empty(t *testing.T) {
	r, err := implode.NewReader(bytes.NewReader(nil), true, true)
	require.NoError(t, err)
	defer r.Close()

	buf := make([]byte, 16)
	n, err := r.Read(buf)
	require.ErrorIs(t, err, io.EOF)
	assert.Equal(t, 0, n)
}
