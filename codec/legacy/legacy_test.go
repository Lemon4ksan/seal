// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package legacy_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/codec"
	_ "github.com/lemon4ksan/seal/codec/legacy"
)

func TestLegacyCodecs_Registration(t *testing.T) {
	methods := []codec.Method{
		codec.Shrink,
		codec.Reduce1,
		codec.Reduce2,
		codec.Reduce3,
		codec.Reduce4,
		codec.Implode,
		codec.Deflate64,
		codec.PKWAREDCL,
	}

	reg := codec.DefaultRegistry

	for _, m := range methods {
		dec, err := reg.Decompressor(m)
		require.NoError(t, err)
		require.NotNil(t, dec)

		// Verify decompressor creates valid reader on empty input
		rc, err := dec.Decompress(bytes.NewReader(nil))
		if err != nil && !errors.Is(err, io.EOF) {
			require.NoError(t, err)
		}
		if rc != nil {
			_ = rc.Close()
		}
	}
}
