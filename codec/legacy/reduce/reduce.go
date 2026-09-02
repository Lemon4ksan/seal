// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package reduce implements PKWARE ZIP Compression Methods 2, 3, 4, 5 (Reducing).
package reduce

import (
	"errors"
	"io"
)

// Decompressor implements decompression for PKWARE Reduced streams (Methods 2-5).
type Decompressor struct {
	r         io.ByteReader
	factor    uint8 // 1, 2, 3, or 4
	bits      uint8 // 6, 5, 4, or 3
	followers [256][]byte
	lastByte  byte
	bitBuf    uint32
	bitCount  uint8
	rleCount  int
	rleChar   byte
	eof       bool
}

// NewReader returns an [io.ReadCloser] decompressing PKWARE Reduced data for the given factor (1 to 4).
func NewReader(r io.Reader, factor int) (io.ReadCloser, error) {
	if factor < 1 || factor > 4 {
		return nil, errors.New("reduce: factor must be between 1 and 4")
	}

	var br io.ByteReader
	if b, ok := r.(io.ByteReader); ok {
		br = b
	} else {
		br = &byteReaderWrapper{r: r}
	}

	d := &Decompressor{
		r:      br,
		factor: uint8(factor),
		bits: uint8(
			7 - factor,
		), // Factor 1: 6 bits (64), Factor 2: 5 bits (32), Factor 3: 4 bits (16), Factor 4: 3 bits (8)
	}

	// Read follower table headers
	numFollowers := 1 << d.bits
	for i := range 256 {
		d.followers[i] = make([]byte, numFollowers)
	}

	for i := 255; i >= 0; i-- {
		n, err := d.readBits(6)
		if err != nil {
			return nil, err
		}
		for j := range n {
			b, err := d.readBits(8)
			if err != nil {
				return nil, err
			}
			if j < numFollowers {
				d.followers[i][j] = byte(b)
			}
		}
	}

	return d, nil
}

func (d *Decompressor) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if d.eof && d.rleCount == 0 {
		return 0, io.EOF
	}

	n := 0
	for n < len(p) {
		if d.rleCount > 0 {
			p[n] = d.rleChar
			d.rleCount--
			n++
			continue
		}

		if d.eof {
			break
		}

		flag, err := d.readBits(1)
		if err != nil {
			d.eof = true
			if n > 0 {
				return n, nil
			}
			return 0, io.EOF
		}

		var out byte
		if flag == 0 {
			// Literal byte
			val, err := d.readBits(8)
			if err != nil {
				d.eof = true
				break
			}
			out = byte(val)
		} else {
			// Follower index
			idx, err := d.readBits(d.bits)
			if err != nil {
				d.eof = true
				break
			}
			out = d.followers[d.lastByte][idx]
		}

		d.lastByte = out
		p[n] = out
		n++
	}

	return n, nil
}

func (d *Decompressor) readBits(n uint8) (int, error) {
	for d.bitCount < n {
		b, err := d.r.ReadByte()
		if err != nil {
			return 0, err
		}
		d.bitBuf |= uint32(b) << d.bitCount
		d.bitCount += 8
	}
	val := int(d.bitBuf & ((1 << n) - 1))
	d.bitBuf >>= n
	d.bitCount -= n
	return val, nil
}

func (d *Decompressor) Close() error {
	return nil
}

type byteReaderWrapper struct {
	r   io.Reader
	buf [1]byte
}

func (b *byteReaderWrapper) ReadByte() (byte, error) {
	_, err := io.ReadFull(b.r, b.buf[:])
	return b.buf[0], err
}
