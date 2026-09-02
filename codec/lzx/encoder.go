// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzx

import (
	"bytes"
	"encoding/binary"
)

type bitWriter struct {
	buf   bytes.Buffer
	bits  uint32
	nbits int
}

func (bw *bitWriter) writeBits(val uint32, n int) {
	bw.bits |= (val & ((1 << n) - 1)) << (32 - bw.nbits - n)
	bw.nbits += n
	for bw.nbits >= 16 {
		word := uint16(bw.bits >> 16)
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], word)
		bw.buf.Write(b[:])
		bw.bits <<= 16
		bw.nbits -= 16
	}
}

func (bw *bitWriter) flush() []byte {
	if bw.nbits > 0 {
		word := uint16(bw.bits >> 16)
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], word)
		bw.buf.Write(b[:])
		bw.bits = 0
		bw.nbits = 0
	}
	return bw.buf.Bytes()
}

// Compress creates an LZX compliant bitstream from src.
func Compress(dst, src []byte, windowBits int) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}

	var bw bitWriter
	bw.writeBits(BlockUncomp, 3)

	srcLen := uint32(len(src))
	bw.writeBits(srcLen>>16, 8)
	bw.writeBits((srcLen>>8)&0xFF, 8)
	bw.writeBits(srcLen&0xFF, 8)

	// Align to 16-bit word boundary
	if bw.nbits%16 != 0 {
		bw.writeBits(0, 16-(bw.nbits%16))
	}

	// Emit initial R0, R1, R2 as 32-bit little-endian
	for i := 0; i < 3; i++ {
		bw.writeBits(1, 16)
		bw.writeBits(0, 16)
	}

	// Write raw payload
	bw.flush()
	bw.buf.Write(src)

	// Align to 16-bit boundary if odd length
	if len(src)%2 != 0 {
		bw.buf.WriteByte(0)
	}

	return bw.buf.Bytes(), nil
}
