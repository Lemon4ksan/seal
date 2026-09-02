// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package implode_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/codec/legacy/implode"
)

func TestImplode_Empty(t *testing.T) {
	r, err := implode.NewReader(bytes.NewReader(nil), true, true)
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}
	defer r.Close()

	buf := make([]byte, 16)
	n, err := r.Read(buf)
	if !errors.Is(err, io.EOF) || n != 0 {
		t.Errorf("expected EOF, got n=%d, err=%v", n, err)
	}
}
