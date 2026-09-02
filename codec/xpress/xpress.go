// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package xpress implements the Microsoft XPRESS Huffman (LZ77+Huffman) compression algorithm.
package xpress

import (
	"encoding/binary"
	"errors"
)

const (
	// NumSymbols is the number of Huffman symbols (256 literals + 256 matches).
	NumSymbols = 512
	// MaxCodeLen is the maximum Huffman code length in bits.
	MaxCodeLen = 15
	// TableBits is the bit-width of the fast lookup decoding table.
	TableBits = 15
	// TableSize is the number of entries in the decoding table.
	TableSize = 1 << TableBits
	// HeaderTableBytes is the size of the 512 packed 4-bit code lengths (256 bytes).
	HeaderTableBytes = 256
)

var (
	// ErrBadData indicates corrupted or invalid XPRESS data.
	ErrBadData = errors.New("xpress: corrupted or invalid data")
	// ErrBufferTooSmall indicates destination buffer is insufficient.
	ErrBufferTooSmall = errors.New("xpress: output buffer too small")
)

type dentry struct {
	sym uint16
	len uint8
}

// buildDTable constructs a 15-bit canonical Huffman decoding table.
func buildDTable(cl []uint8, tbl []dentry) bool {
	var cnt [MaxCodeLen + 1]uint32
	for _, l := range cl {
		if l > 0 && l <= MaxCodeLen {
			cnt[l]++
		}
	}

	var next [MaxCodeLen + 1]uint32
	var code uint32
	for l := 1; l <= MaxCodeLen; l++ {
		code = (code + cnt[l-1]) << 1
		next[l] = code
	}

	for i := range tbl {
		tbl[i] = dentry{}
	}

	for s := 0; s < NumSymbols; s++ {
		L := int(cl[s])
		if L == 0 {
			continue
		}
		if L > MaxCodeLen {
			return false
		}
		c := next[L]
		next[L]++

		shift := TableBits - L
		base := c << shift
		span := uint32(1) << shift
		if base+span > TableSize {
			return false
		}

		entry := dentry{sym: uint16(s), len: uint8(L)}
		for j := uint32(0); j < span; j++ {
			tbl[base+j] = entry
		}
	}
	return true
}

// Decompress decompresses an XPRESS Huffman chunk from in into out.
// Returns the number of bytes written to out.
func Decompress(out, in []byte) (int, error) {
	if len(in) < HeaderTableBytes {
		return 0, ErrBadData
	}

	var cl [NumSymbols]uint8
	for i := 0; i < HeaderTableBytes; i++ {
		cl[2*i] = in[i] & 0x0F
		cl[2*i+1] = (in[i] >> 4) & 0x0F
	}

	var tbl [TableSize]dentry
	if !buildDTable(cl[:], tbl[:]) {
		return 0, ErrBadData
	}

	p := HeaderTableBytes
	inLen := len(in)
	outLen := len(out)

	var bits uint32
	var nbits int

	// Refill helper: ensures at least n bits in buffer (MSB-first)
	refill := func(n int) {
		if nbits < n && p+1 < inLen {
			word := uint32(binary.LittleEndian.Uint16(in[p:]))
			bits |= word << (16 - nbits)
			p += 2
			nbits += 16
		}
	}

	opos := 0
	for opos < outLen {
		refill(MaxCodeLen)

		idx := bits >> (32 - TableBits)
		sym := tbl[idx].sym
		slen := int(tbl[idx].len)
		if slen == 0 {
			return opos, ErrBadData
		}

		bits <<= slen
		nbits -= slen

		if sym < 256 {
			out[opos] = byte(sym)
			opos++
		} else {
			mi := int(sym - 256)
			lh := mi & 0x0F
			ol := mi >> 4

			refill(16)

			var moff uint32
			if ol == 0 {
				moff = 1
			} else {
				moff = (1 << ol) | (bits >> (32 - ol))
				bits <<= ol
				nbits -= ol
			}

			var mlen uint32
			if lh < 15 {
				mlen = uint32(lh + 3)
			} else {
				if p >= inLen {
					return opos, ErrBadData
				}
				xb := uint32(in[p])
				p++
				if xb < 255 {
					mlen = 15 + 3 + xb
				} else {
					if p+1 >= inLen {
						return opos, ErrBadData
					}
					mlen = uint32(binary.LittleEndian.Uint16(in[p:]))
					p += 2
					if mlen == 0 {
						if p+3 >= inLen {
							return opos, ErrBadData
						}
						mlen = binary.LittleEndian.Uint32(in[p:])
						p += 4
					}
					mlen += 3
				}
			}

			if moff > uint32(opos) {
				return opos, ErrBadData
			}

			remain := uint32(outLen - opos)
			copyLen := mlen
			if copyLen > remain {
				copyLen = remain
			}

			// Copy match bytes
			for j := uint32(0); j < copyLen; j++ {
				out[opos] = out[opos-int(moff)]
				opos++
			}
		}
	}

	return opos, nil
}
