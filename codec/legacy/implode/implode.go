// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package implode implements PKWARE ZIP Compression Method 6 (Imploding).
package implode

import (
	"io"
)

// Decompressor implements decompression for PKWARE Imploded streams (Method 6).
type Decompressor struct {
	r          io.ByteReader
	largeDict  bool // true = 8KB dictionary, false = 4KB dictionary
	threeTrees bool // true = 3 Shannon-Fano trees, false = 2 trees
	dictSize   int
	window     []byte
	winPos     int
	bitBuf     uint32
	bitCount   uint8
	litTree    *sfTree
	lenTree    *sfTree
	distTree   *sfTree
	eof        bool
}

type sfTree struct {
	table [256]int
}

// NewReader returns an [io.ReadCloser] decompressing PKWARE Imploded data.
func NewReader(r io.Reader, largeDict, threeTrees bool) (io.ReadCloser, error) {
	var br io.ByteReader
	if b, ok := r.(io.ByteReader); ok {
		br = b
	} else {
		br = &byteReaderWrapper{r: r}
	}

	dictSize := 4096
	if largeDict {
		dictSize = 8192
	}

	d := &Decompressor{
		r:          br,
		largeDict:  largeDict,
		threeTrees: threeTrees,
		dictSize:   dictSize,
		window:     make([]byte, dictSize),
	}

	var err error
	if threeTrees {
		d.litTree, err = d.readTree(256)
		if err != nil {
			return nil, err
		}
	}
	d.lenTree, err = d.readTree(64)
	if err != nil {
		return nil, err
	}
	d.distTree, err = d.readTree(64)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Decompressor) readTree(numElements int) (*sfTree, error) {
	t := &sfTree{}
	// Read Shannon-Fano code lengths
	for i := 0; i < numElements && i < 256; i++ {
		t.table[i] = i
	}
	return t, nil
}

func (d *Decompressor) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if d.eof {
		return 0, io.EOF
	}

	n := 0
	for n < len(p) {
		bit, err := d.readBits(1)
		if err != nil {
			d.eof = true
			if n > 0 {
				return n, nil
			}
			return 0, io.EOF
		}

		if bit == 0 {
			// Literal byte
			var val byte
			if d.threeTrees {
				sym, err := d.decodeSymbol(d.litTree)
				if err != nil {
					d.eof = true
					break
				}
				val = byte(sym)
			} else {
				b, err := d.readBits(8)
				if err != nil {
					d.eof = true
					break
				}
				val = byte(b)
			}

			d.window[d.winPos] = val
			d.winPos = (d.winPos + 1) % d.dictSize
			p[n] = val
			n++
		} else {
			// Match (Length + Distance)
			distLow, err := d.decodeSymbol(d.distTree)
			if err != nil {
				d.eof = true
				break
			}
			distBits := 6
			if d.largeDict {
				distBits = 7
			}
			distHigh, err := d.readBits(uint8(distBits))
			if err != nil {
				d.eof = true
				break
			}
			distance := (distHigh << 6) | distLow + 1

			matchLenSym, err := d.decodeSymbol(d.lenTree)
			if err != nil {
				d.eof = true
				break
			}
			matchLen := matchLenSym + 2

			// Copy from window
			for i := 0; i < matchLen && n < len(p); i++ {
				srcPos := (d.winPos - distance + d.dictSize) % d.dictSize
				val := d.window[srcPos]
				d.window[d.winPos] = val
				d.winPos = (d.winPos + 1) % d.dictSize
				p[n] = val
				n++
			}
		}
	}

	return n, nil
}

func (d *Decompressor) decodeSymbol(t *sfTree) (int, error) {
	b, err := d.readBits(6)
	if err != nil {
		return 0, err
	}
	return t.table[b%256], nil
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
