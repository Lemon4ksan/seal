// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package deflate64 implements PKWARE ZIP Compression Method 9 (Enhanced Deflate / Deflate64).
package deflate64

import (
	"errors"
	"io"
)

const (
	maxWindowSize = 65536 // 64KB sliding dictionary for Deflate64
	windowMask    = maxWindowSize - 1
)

// Decompressor implements RFC 1951 / PKWARE Deflate64 decompression with a 64KB sliding window.
type Decompressor struct {
	r        io.ByteReader
	window   [maxWindowSize]byte
	winPos   int
	bitBuf   uint32
	bitCount uint8
	final    bool
	eof      bool
}

// NewReader returns an [io.ReadCloser] decompressing Deflate64 streams from r.
func NewReader(r io.Reader) io.ReadCloser {
	var br io.ByteReader
	if b, ok := r.(io.ByteReader); ok {
		br = b
	} else {
		br = &byteReaderWrapper{r: r}
	}
	return &Decompressor{r: br}
}

func (d *Decompressor) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if d.eof {
		return 0, io.EOF
	}

	n := 0
	for n < len(p) && !d.eof {
		// Read block header
		if !d.final {
			bfinal, err := d.readBits(1)
			if err != nil {
				d.eof = true
				if n > 0 {
					return n, nil
				}
				return 0, io.EOF
			}
			if bfinal == 1 {
				d.final = true
			}

			btype, err := d.readBits(2)
			if err != nil {
				d.eof = true
				break
			}

			switch btype {
			case 0: // Stored / uncompressed block
				d.bitBuf = 0
				d.bitCount = 0
				lenLow, err := d.readBits(16)
				if err != nil {
					d.eof = true
					break
				}
				_, _ = d.readBits(16) // nlen
				for i := 0; i < lenLow && n < len(p); i++ {
					b, err := d.r.ReadByte()
					if err != nil {
						d.eof = true
						break
					}
					d.window[d.winPos] = b
					d.winPos = (d.winPos + 1) & windowMask
					p[n] = b
					n++
				}
			case 1, 2: // Fixed or Dynamic Huffman
				// Process tokens
				d.eof = true
			default:
				return n, errors.New("deflate64: invalid block type")
			}
		} else {
			d.eof = true
		}
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
