// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package legacy_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

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
	}

	reg := codec.DefaultRegistry

	for _, m := range methods {
		dec, err := reg.Decompressor(m)
		if err != nil {
			t.Errorf("failed to get decompressor for method %v: %v", m, err)
			continue
		}
		if dec == nil {
			t.Errorf("nil decompressor returned for method %v", m)
			continue
		}

		// Verify decompressor creates valid reader on empty input
		rc, err := dec.Decompress(bytes.NewReader(nil))
		if err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("dec.Decompress failed for method %v: %v", m, err)
		}
		if rc != nil {
			_ = rc.Close()
		}
	}
}
