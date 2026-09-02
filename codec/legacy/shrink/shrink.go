// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package shrink implements PKWARE ZIP Compression Method 1 (Shrinking / Dynamic LZW).
package shrink

import (
	"errors"
	"io"
)

const (
	minCodeSize = 9
	maxCodeSize = 13
	maxCode     = 1 << maxCodeSize // 8192
	escapeCode  = 256
	firstFree   = 257
)

// Decompressor implements decompression for PKWARE Shrink (Method 1).
type Decompressor struct {
	r         io.ByteReader
	bitBuf    uint32
	bitCount  uint8
	codeSize  uint8
	freeCode  int
	oldCode   int
	finChar   byte
	parent    [maxCode]int16
	suffix    [maxCode]byte
	stack     [maxCode]byte
	stackPtr  int
	firstCode bool
	eof       bool
}

// NewReader returns an [io.ReadCloser] decompressing PKWARE Shrunk data from r.
func NewReader(r io.Reader) io.ReadCloser {
	var br io.ByteReader
	if b, ok := r.(io.ByteReader); ok {
		br = b
	} else {
		br = &byteReaderWrapper{r: r}
	}

	d := &Decompressor{
		r:         br,
		codeSize:  minCodeSize,
		freeCode:  firstFree,
		firstCode: true,
	}
	for i := range 256 {
		d.parent[i] = -1
		d.suffix[i] = byte(i)
	}
	return d
}

func (d *Decompressor) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if d.eof && d.stackPtr == 0 {
		return 0, io.EOF
	}

	n := 0
	for n < len(p) {
		if d.stackPtr > 0 {
			d.stackPtr--
			p[n] = d.stack[d.stackPtr]
			n++
			continue
		}

		if d.eof {
			break
		}

		code, err := d.readBits(d.codeSize)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				d.eof = true
				if n > 0 {
					return n, nil
				}
				return 0, io.EOF
			}
			return n, err
		}

		if code == escapeCode {
			sub, err := d.readBits(8)
			if err != nil {
				d.eof = true
				break
			}
			switch sub {
			case 1:
				if d.codeSize < maxCodeSize {
					d.codeSize++
				}
			case 2:
				d.partialClear()
			}
			continue
		}

		if d.firstCode {
			d.firstCode = false
			d.oldCode = code
			d.finChar = byte(code)
			p[n] = d.finChar
			n++
			continue
		}

		inCode := code
		if code >= d.freeCode {
			d.stack[d.stackPtr] = d.finChar
			d.stackPtr++
			code = d.oldCode
		}

		for code >= 256 && code < maxCode {
			d.stack[d.stackPtr] = d.suffix[code]
			d.stackPtr++
			code = int(d.parent[code])
		}
		d.finChar = d.suffix[code]
		d.stack[d.stackPtr] = d.finChar
		d.stackPtr++

		if d.freeCode < maxCode {
			d.parent[d.freeCode] = int16(d.oldCode)
			d.suffix[d.freeCode] = d.finChar
			d.freeCode++
		}
		d.oldCode = inCode
	}

	return n, nil
}

func (d *Decompressor) partialClear() {
	hasChildren := make([]bool, maxCode)
	for i := firstFree; i < d.freeCode; i++ {
		p := d.parent[i]
		if p >= 0 && int(p) < maxCode {
			hasChildren[p] = true
		}
	}

	newFree := firstFree
	for i := firstFree; i < d.freeCode; i++ {
		if hasChildren[i] {
			d.parent[newFree] = d.parent[i]
			d.suffix[newFree] = d.suffix[i]
			newFree++
		}
	}
	d.freeCode = newFree
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
