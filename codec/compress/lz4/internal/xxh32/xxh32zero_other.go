// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (!arm && !arm64) || noasm || (arm64 && !gc)

package xxh32

// ChecksumZero returns the 32-bit hash of input.
func ChecksumZero(input []byte) uint32 { return checksumZeroGo(input) }

func update(v *[4]uint32, buf *[16]byte, input []byte) {
	updateGo(v, buf, input)
}
