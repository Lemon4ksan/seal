// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dcl implements PKWARE Data Compression Library (DCL) Implode decompression (Method 10 / blast).
package dcl

import (
	"errors"
	"io"
	"sync"
)

const (
	maxBits = 13
	maxWin  = 4096
)

var (
	// Sentinel errors
	ErrWrongLiteralFlag = errors.New("dcl: literal flag not zero or one")
	ErrWrongDictionary  = errors.New("dcl: dictionary size not in 4..6")
	ErrDistanceTooBig   = errors.New("dcl: distance too far back")
	ErrInvalidCode      = errors.New("dcl: invalid Huffman code")
)

type huffmanTable struct {
	count  [maxBits + 1]int16
	symbol []int16
}

var (
	initTablesOnce sync.Once
	litTable       huffmanTable
	lenTable       huffmanTable
	distTable      huffmanTable
)

var litlen = []byte{
	11, 124, 8, 7, 28, 7, 188, 13, 76, 4, 10, 8, 12, 10, 12, 10, 8, 23, 8,
	9, 7, 6, 7, 8, 7, 6, 55, 8, 23, 24, 12, 11, 7, 9, 11, 12, 6, 7, 22, 5,
	7, 24, 6, 11, 9, 6, 7, 22, 7, 11, 38, 7, 9, 8, 25, 11, 8, 11, 9, 12,
	8, 12, 5, 38, 5, 38, 5, 11, 7, 5, 6, 21, 6, 10, 53, 8, 7, 24, 10, 27,
	44, 253, 253, 253, 252, 252, 252, 13, 12, 45, 12, 45, 12, 61, 12, 45,
	44, 173,
}

var (
	lenlen  = []byte{2, 35, 36, 53, 38, 23}
	distlen = []byte{2, 20, 53, 230, 247, 151, 248}
)

var (
	lenBase  = [16]int{3, 2, 4, 5, 6, 7, 8, 9, 10, 12, 16, 24, 40, 72, 136, 264}
	lenExtra = [16]uint8{0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}
)

func construct(h *huffmanTable, rep []byte, maxSyms int) {
	var lengths [256]int16
	symbol := 0
	for _, b := range rep {
		left := int(b>>4) + 1
		codeLen := int16(b & 15)
		for range left {
			lengths[symbol] = codeLen
			symbol++
		}
	}
	n := symbol

	for i := 0; i <= maxBits; i++ {
		h.count[i] = 0
	}
	for i := range n {
		h.count[lengths[i]]++
	}

	h.symbol = make([]int16, maxSyms)
	var offs [maxBits + 1]int16
	offs[1] = 0
	for i := 1; i < maxBits; i++ {
		offs[i+1] = offs[i] + h.count[i]
	}

	for i := range n {
		if lengths[i] != 0 {
			h.symbol[offs[lengths[i]]] = int16(i)
			offs[lengths[i]]++
		}
	}
}

func initTables() {
	construct(&litTable, litlen, 256)
	construct(&lenTable, lenlen, 16)
	construct(&distTable, distlen, 64)
}

// Decompressor implements decompression for PKWARE DCL Imploded streams.
type Decompressor struct {
	r        io.ByteReader
	bitBuf   uint32
	bitCount uint8
	lit      int
	dict     int
	window   [maxWin]byte
	next     int
	first    bool
	initDone bool
	eof      bool
	holdBuf  []byte
}

// NewReader constructs an [io.ReadCloser] decompressing PKWARE DCL Implode streams.
func NewReader(r io.Reader) io.ReadCloser {
	initTablesOnce.Do(initTables)

	var br io.ByteReader
	if b, ok := r.(io.ByteReader); ok {
		br = b
	} else {
		br = &byteReaderWrapper{r: r}
	}

	return &Decompressor{
		r:     br,
		first: true,
	}
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

func (d *Decompressor) decode(h *huffmanTable) (int, error) {
	code := 0
	first := 0
	index := 0
	lenBit := 1

	for {
		for d.bitCount > 0 {
			code |= int((d.bitBuf & 1) ^ 1)
			d.bitBuf >>= 1
			d.bitCount--

			count := int(h.count[lenBit])
			if code < first+count {
				return int(h.symbol[index+(code-first)]), nil
			}
			index += count
			first += count
			first <<= 1
			code <<= 1
			lenBit++
		}

		if lenBit > maxBits {
			return 0, ErrInvalidCode
		}

		b, err := d.r.ReadByte()
		if err != nil {
			return 0, err
		}
		d.bitBuf = uint32(b)
		d.bitCount = 8
	}
}

func (d *Decompressor) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if len(d.holdBuf) > 0 {
		n := copy(p, d.holdBuf)
		d.holdBuf = d.holdBuf[n:]
		return n, nil
	}

	if d.eof {
		return 0, io.EOF
	}

	if !d.initDone {
		lit, err := d.readBits(8)
		if err != nil {
			return 0, err
		}
		if lit > 1 {
			return 0, ErrWrongLiteralFlag
		}
		dict, err := d.readBits(8)
		if err != nil {
			return 0, err
		}
		if dict < 4 || dict > 6 {
			return 0, ErrWrongDictionary
		}
		d.lit = lit
		d.dict = dict
		d.initDone = true
	}

	var outBuf []byte

	for len(outBuf) < len(p) && !d.eof {
		bit, err := d.readBits(1)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				d.eof = true
				break
			}
			return 0, err
		}

		if bit != 0 {
			// Length / Distance pair
			symbol, err := d.decode(&lenTable)
			if err != nil {
				return 0, err
			}
			extraBits := lenExtra[symbol]
			extraVal, err := d.readBits(extraBits)
			if err != nil {
				return 0, err
			}
			length := lenBase[symbol] + extraVal
			if length == 519 {
				d.eof = true
				break
			}

			distBits := uint8(d.dict)
			if length == 2 {
				distBits = 2
			}
			distSym, err := d.decode(&distTable)
			if err != nil {
				return 0, err
			}
			distExtra, err := d.readBits(distBits)
			if err != nil {
				return 0, err
			}
			dist := (distSym << distBits) + distExtra + 1

			if d.first && dist > d.next {
				return 0, ErrDistanceTooBig
			}

			for length > 0 {
				copyLen := maxWin
				if d.next < dist {
					copyLen = dist
				}
				copyLen -= d.next
				if copyLen > length {
					copyLen = length
				}

				srcPos := (d.next - dist + maxWin) % maxWin
				for range copyLen {
					val := d.window[srcPos]
					srcPos = (srcPos + 1) % maxWin
					d.window[d.next] = val
					d.next++
					if d.next == maxWin {
						d.next = 0
						d.first = false
					}
					outBuf = append(outBuf, val)
				}
				length -= copyLen
			}
		} else {
			// Literal byte
			var val byte
			if d.lit != 0 {
				sym, err := d.decode(&litTable)
				if err != nil {
					return 0, err
				}
				val = byte(sym)
			} else {
				b, err := d.readBits(8)
				if err != nil {
					return 0, err
				}
				val = byte(b)
			}
			d.window[d.next] = val
			d.next++
			if d.next == maxWin {
				d.next = 0
				d.first = false
			}
			outBuf = append(outBuf, val)
		}
	}

	if len(outBuf) == 0 && d.eof {
		return 0, io.EOF
	}

	n := copy(p, outBuf)
	if n < len(outBuf) {
		d.holdBuf = outBuf[n:]
	}
	return n, nil
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
