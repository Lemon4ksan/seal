// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !noasm
// +build !noasm

package xxh32

// ChecksumZero returns the 32-bit hash of input.
//
//go:noescape
func ChecksumZero(input []byte) uint32

//go:noescape
func update(v *[4]uint32, buf *[16]byte, input []byte)
