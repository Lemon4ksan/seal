// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzx

import (
	"encoding/binary"
)

type huffEntry struct {
	sym uint16
	len uint8
}

type huffTable struct {
	table [4096]huffEntry
}

func (ht *huffTable) build(lengths []byte) bool {
	var count [17]uint32
	for _, l := range lengths {
		if l > 0 && l <= 16 {
			count[l]++
		}
	}

	var nextCode [17]uint32
	var code uint32
	for l := 1; l <= 16; l++ {
		code = (code + count[l-1]) << 1
		nextCode[l] = code
	}

	for i := range ht.table {
		ht.table[i] = huffEntry{}
	}

	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		if l > 16 {
			return false
		}
		c := nextCode[l]
		nextCode[l]++

		if l <= 12 {
			shift := 12 - l
			base := c << shift
			span := uint32(1) << shift
			entry := huffEntry{sym: uint16(sym), len: l}
			for j := uint32(0); j < span; j++ {
				if base+j < 4096 {
					ht.table[base+j] = entry
				}
			}
		} else {
			// Prefix entry in main table points to extended lookup
			prefix := c >> (l - 12)
			if prefix < 4096 {
				ht.table[prefix] = huffEntry{sym: uint16(sym), len: l}
			}
		}
	}
	return true
}

type bitReader struct {
	src   []byte
	pos   int
	bits  uint32
	nbits int
}

func newBitReader(src []byte) *bitReader {
	br := &bitReader{src: src}
	br.refill(16)
	return br
}

func (br *bitReader) refill(n int) {
	for br.nbits < n && br.pos+1 < len(br.src) {
		word := uint32(binary.LittleEndian.Uint16(br.src[br.pos:]))
		br.bits |= word << (16 - br.nbits)
		br.pos += 2
		br.nbits += 16
	}
}

func (br *bitReader) readBits(n int) uint32 {
	if n == 0 {
		return 0
	}
	br.refill(n)
	val := br.bits >> (32 - n)
	br.bits <<= n
	br.nbits -= n
	return val
}

func (br *bitReader) decodeSymbol(ht *huffTable) (uint16, error) {
	br.refill(16)
	peek := br.bits >> 20 // 12-bit peek
	entry := ht.table[peek]
	if entry.len == 0 {
		return 0, ErrCorrupt
	}
	br.bits <<= entry.len
	br.nbits -= int(entry.len)
	return entry.sym, nil
}

// Decoder decompresses LZX blocks.
type Decoder struct {
	windowBits int
	r0, r1, r2 uint32

	mainLens  []byte
	lenLens   []byte
	alignLens [NumAligned]byte

	mainTable  huffTable
	lenTable   huffTable
	alignTable huffTable
}

// NewDecoder creates an initialized LZX Decoder.
func NewDecoder(windowBits int) *Decoder {
	if windowBits < MinWindowBits || windowBits > MaxWindowBits {
		windowBits = DefaultWindowBits
	}
	mainSize := GetMainTreeSize(windowBits)
	d := &Decoder{
		windowBits: windowBits,
		r0:         1,
		r1:         1,
		r2:         1,
		mainLens:   make([]byte, mainSize),
		lenLens:    make([]byte, NumLengthCodes),
	}
	return d
}

// Reset clears the decoder state and history.
func (d *Decoder) Reset() {
	d.r0 = 1
	d.r1 = 1
	d.r2 = 1
	for i := range d.mainLens {
		d.mainLens[i] = 0
	}
	for i := range d.lenLens {
		d.lenLens[i] = 0
	}
	for i := range d.alignLens {
		d.alignLens[i] = 0
	}
}

// Decompress decodes an LZX bitstream from src into dst.
func (d *Decoder) Decompress(dst, src []byte) (int, error) {
	if len(src) < 2 {
		return 0, ErrCorrupt
	}

	br := newBitReader(src)
	dstLen := len(dst)
	outPos := 0

	for outPos < dstLen {
		blockType := br.readBits(3)
		if blockType < 1 || blockType > 3 {
			return outPos, ErrCorrupt
		}

		bSizeHi := br.readBits(8)
		bSizeMid := br.readBits(8)
		bSizeLo := br.readBits(8)
		blockSize := int((bSizeHi << 16) | (bSizeMid << 8) | bSizeLo)

		if blockSize <= 0 || outPos+blockSize > dstLen {
			blockSize = dstLen - outPos
		}

		switch blockType {
		case BlockAligned:
			for i := 0; i < NumAligned; i++ {
				d.alignLens[i] = byte(br.readBits(3))
			}
			if !d.alignTable.build(d.alignLens[:]) {
				return outPos, ErrCorrupt
			}
			fallthrough

		case BlockVerbatim:
			if err := d.readTreeLengths(br, d.mainLens[:NumChars]); err != nil {
				return outPos, err
			}
			if err := d.readTreeLengths(br, d.mainLens[NumChars:]); err != nil {
				return outPos, err
			}
			if !d.mainTable.build(d.mainLens) {
				return outPos, ErrCorrupt
			}

			if err := d.readTreeLengths(br, d.lenLens); err != nil {
				return outPos, err
			}
			if !d.lenTable.build(d.lenLens) {
				return outPos, ErrCorrupt
			}

			blockEnd := outPos + blockSize
			for outPos < blockEnd {
				sym, err := br.decodeSymbol(&d.mainTable)
				if err != nil {
					return outPos, err
				}

				if sym < NumChars {
					dst[outPos] = byte(sym)
					outPos++
					continue
				}

				header := sym - NumChars
				lenHeader := header & 7
				slot := header >> 3

				var matchLen int
				if lenHeader < 7 {
					matchLen = int(lenHeader) + MinMatchLength
				} else {
					extra, err := br.decodeSymbol(&d.lenTable)
					if err != nil {
						return outPos, err
					}
					matchLen = 7 + MinMatchLength + int(extra)
				}

				var matchOffset uint32
				switch slot {
				case 0:
					matchOffset = d.r0
				case 1:
					matchOffset = d.r1
					d.r1 = d.r0
					d.r0 = matchOffset
				case 2:
					matchOffset = d.r2
					d.r2 = d.r1
					d.r1 = d.r0
					d.r0 = matchOffset
				case 3:
					matchOffset = 1
				default:
					extra := int(extraBits[slot])
					if blockType == BlockAligned && extra >= 3 {
						verbatim := br.readBits(extra - 3)
						alignSym, err := br.decodeSymbol(&d.alignTable)
						if err != nil {
							return outPos, err
						}
						matchOffset = basePosition[slot] + (verbatim << 3) + uint32(alignSym) - 2
					} else {
						verbatim := br.readBits(extra)
						matchOffset = basePosition[slot] + verbatim - 2
					}
					d.r2 = d.r1
					d.r1 = d.r0
					d.r0 = matchOffset
				}

				if matchOffset == 0 || matchOffset > uint32(outPos) {
					return outPos, ErrCorrupt
				}

				remain := blockEnd - outPos
				if matchLen > remain {
					matchLen = remain
				}

				for j := 0; j < matchLen; j++ {
					dst[outPos] = dst[outPos-int(matchOffset)]
					outPos++
				}
			}

		case BlockUncomp:
			// Align bit reader to 16-bit word
			if br.nbits%16 != 0 {
				br.readBits(br.nbits % 16)
			}
			d.r0 = br.readBits(16) | (br.readBits(16) << 16)
			d.r1 = br.readBits(16) | (br.readBits(16) << 16)
			d.r2 = br.readBits(16) | (br.readBits(16) << 16)

			n := copy(dst[outPos:outPos+blockSize], br.src[br.pos:])
			outPos += n
			br.pos += n
		}
	}

	return outPos, nil
}

func (d *Decoder) readTreeLengths(br *bitReader, lengths []byte) error {
	var preLens [NumPreTree]byte
	for i := 0; i < NumPreTree; i++ {
		preLens[i] = byte(br.readBits(4))
	}

	var preTable huffTable
	if !preTable.build(preLens[:]) {
		return ErrCorrupt
	}

	i := 0
	for i < len(lengths) {
		sym, err := br.decodeSymbol(&preTable)
		if err != nil {
			return err
		}

		switch sym {
		case 17:
			zeros := int(br.readBits(4)) + 4
			for j := 0; j < zeros && i < len(lengths); j++ {
				lengths[i] = 0
				i++
			}
		case 18:
			zeros := int(br.readBits(5)) + 20
			for j := 0; j < zeros && i < len(lengths); j++ {
				lengths[i] = 0
				i++
			}
		case 19:
			count := int(br.readBits(1)) + 4
			sameSym, err := br.decodeSymbol(&preTable)
			if err != nil {
				return err
			}
			diff := int(lengths[i]) - int(sameSym)
			if diff < 0 {
				diff += 17
			}
			newLen := byte(diff)
			for j := 0; j < count && i < len(lengths); j++ {
				lengths[i] = newLen
				i++
			}
		default:
			diff := int(lengths[i]) - int(sym)
			if diff < 0 {
				diff += 17
			}
			lengths[i] = byte(diff)
			i++
		}
	}

	return nil
}

// Decompress decodes an LZX bitstream from src into dst using default window size (32 KB).
func Decompress(dst, src []byte) (int, error) {
	dec := NewDecoder(DefaultWindowBits)
	return dec.Decompress(dst, src)
}
