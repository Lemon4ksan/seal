// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ppmd implements PPMd (Variant H / 7z) compression and decompression.
package ppmd

import (
	"errors"
	"io"
)

const (
	kTopValue = 1 << 24
)

// RangeDecoder decodes symbols encoded with the 7z PPMd range coder.
type RangeDecoder struct {
	r      io.Reader
	range_ uint32
	code   uint32
	buf    [4096]byte
	pos    int
	limit  int
}

// Init initializes the PPMd range decoder reading 5 bytes from r.
func (rd *RangeDecoder) Init(r io.Reader) error {
	rd.r = r
	rd.range_ = 0xFFFFFFFF
	rd.code = 0
	rd.pos = 0
	rd.limit = 0

	first, err := rd.readByte()
	if err != nil {
		return err
	}
	if first != 0 {
		return errors.New("ppmd: invalid range coder header")
	}

	for i := 0; i < 4; i++ {
		b, err := rd.readByte()
		if err != nil {
			return err
		}
		rd.code = (rd.code << 8) | uint32(b)
	}
	return nil
}

func (rd *RangeDecoder) readByte() (byte, error) {
	if rd.pos >= rd.limit {
		n, err := rd.r.Read(rd.buf[:])
		if n == 0 {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		rd.pos = 0
		rd.limit = n
	}
	b := rd.buf[rd.pos]
	rd.pos++
	return b, nil
}

// Normalize updates range and code from input bytes.
func (rd *RangeDecoder) Normalize() error {
	for rd.range_ < kTopValue {
		b, err := rd.readByte()
		if err != nil {
			return err
		}
		rd.code = (rd.code << 8) | uint32(b)
		rd.range_ <<= 8
	}
	return nil
}

// GetThreshold calculates threshold frequency for decoding.
func (rd *RangeDecoder) GetThreshold(total uint32) uint32 {
	rd.range_ /= total
	return rd.code / rd.range_
}

// Decode updates range and code with symbol offset and count.
func (rd *RangeDecoder) Decode(start, size uint32) error {
	rd.code -= start * rd.range_
	rd.range_ *= size
	return rd.Normalize()
}

// RangeEncoder implements the 7z PPMd range encoder.
type RangeEncoder struct {
	w         io.Writer
	low       uint64
	range_    uint32
	cacheSize uint64
	cache     byte
	buf       [4096]byte
	pos       int
}

// Init initializes the range encoder writing to w.
func (re *RangeEncoder) Init(w io.Writer) {
	re.w = w
	re.low = 0
	re.range_ = 0xFFFFFFFF
	re.cacheSize = 1
	re.cache = 0
	re.pos = 0
}

func (re *RangeEncoder) writeByte(b byte) error {
	re.buf[re.pos] = b
	re.pos++
	if re.pos >= len(re.buf) {
		_, err := re.w.Write(re.buf[:])
		re.pos = 0
		return err
	}
	return nil
}

func (re *RangeEncoder) shiftLow() error {
	low := uint32(re.low)
	high := uint32(re.low >> 32)
	if low < 0xFF000000 || high != 0 {
		temp := re.cache
		for {
			if err := re.writeByte(temp + byte(high)); err != nil {
				return err
			}
			temp = 0xFF
			re.cacheSize--
			if re.cacheSize == 0 {
				break
			}
		}
		re.cache = byte(low >> 24)
	}
	re.cacheSize++
	re.low = uint64(low << 8)
	return nil
}

// Encode encodes a symbol with specified start offset, size, and total frequency.
func (re *RangeEncoder) Encode(start, size, total uint32) error {
	re.range_ /= total
	re.low += uint64(start) * uint64(re.range_)
	re.range_ *= size
	for re.range_ < kTopValue {
		re.range_ <<= 8
		if err := re.shiftLow(); err != nil {
			return err
		}
	}
	return nil
}

// Flush flushes buffered range coder bytes.
func (re *RangeEncoder) Flush() error {
	for i := 0; i < 5; i++ {
		if err := re.shiftLow(); err != nil {
			return err
		}
	}
	if re.pos > 0 {
		_, err := re.w.Write(re.buf[:re.pos])
		re.pos = 0
		return err
	}
	return nil
}
