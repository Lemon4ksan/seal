// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec

import (
	"encoding/binary"
	"io"
)

// AppendULEB128 encodes an unsigned integer into ULEB128 bytes and appends to dst.
func AppendULEB128(dst []byte, val uint64) []byte {
	for {
		b := byte(val & 0x7F)
		val >>= 7
		if val != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if val == 0 {
			break
		}
	}
	return dst
}

// ReadULEB128 reads an unsigned integer encoded in ULEB128 format from r.
func ReadULEB128(r io.ByteReader) (uint64, error) {
	return binary.ReadUvarint(r)
}
