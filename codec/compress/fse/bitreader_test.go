// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !race

package fse_test

import (
	"testing"

	"github.com/lemon4ksan/seal/codec/compress/fse"
)

func TestCompressAllocs(t *testing.T) {
	data := []byte("aaabbbbccccccdddddddd")
	var s fse.Scratch

	// Compress once to initialize the scratch
	_, _ = fse.Compress(data, &s)

	allocs := testing.AllocsPerRun(10, func() {
		_, _ = fse.Compress(data, &s)
	})

	if allocs > 0 {
		t.Errorf("Compress allocated %v times", allocs)
	}
}
