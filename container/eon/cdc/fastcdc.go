// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cdc implements high-speed Content-Defined Chunking using the FastCDC algorithm.
package cdc

import (
	"errors"
	"io"
	"math/bits"
)

const (
	// DefaultMinSize is the default minimum chunk size (64 KiB).
	DefaultMinSize = 64 * 1024
	// DefaultAvgSize is the default average target chunk size (256 KiB).
	DefaultAvgSize = 256 * 1024
	// DefaultMaxSize is the default maximum chunk size (1 MiB).
	DefaultMaxSize = 1024 * 1024
)

// Options configures the FastCDC chunking parameters.
type Options struct {
	MinSize uint32
	AvgSize uint32
	MaxSize uint32
}

// DefaultOptions returns standard FastCDC parameters suited for general data.
func DefaultOptions() Options {
	return Options{
		MinSize: DefaultMinSize,
		AvgSize: DefaultAvgSize,
		MaxSize: DefaultMaxSize,
	}
}

// FastCDC slices arbitrary byte streams into variable-sized chunks based on content hashes.
type FastCDC struct {
	opts  Options
	maskS uint64 // Small mask for [minSize, avgSize)
	maskL uint64 // Large mask for [avgSize, maxSize)
}

// NewFastCDC constructs a chunker with the specified options.
func NewFastCDC(opts Options) (*FastCDC, error) {
	if opts.MinSize == 0 {
		opts.MinSize = DefaultMinSize
	}
	if opts.AvgSize == 0 {
		opts.AvgSize = DefaultAvgSize
	}
	if opts.MaxSize == 0 {
		opts.MaxSize = DefaultMaxSize
	}
	if opts.MinSize >= opts.AvgSize || opts.AvgSize >= opts.MaxSize {
		return nil, errors.New("cdc: invalid parameters, must satisfy MinSize < AvgSize < MaxSize")
	}

	bitsAvg := bits.Len32(opts.AvgSize) - 1
	if bitsAvg < 8 {
		bitsAvg = 8
	}

	// 2-level normalized chunking
	maskS := uint64((1 << (bitsAvg + 1)) - 1)
	maskL := uint64((1 << (bitsAvg - 1)) - 1)

	return &FastCDC{
		opts:  opts,
		maskS: maskS,
		maskL: maskL,
	}, nil
}

// Cut determines the length of the next chunk in data.
// If isEOF is false and len(data) < MaxSize, it returns 0 indicating more data is needed.
func (f *FastCDC) Cut(data []byte, isEOF bool) int {
	n := uint32(len(data))
	if n <= f.opts.MinSize {
		if isEOF {
			return int(n)
		}
		return 0
	}

	maxCut := n
	if maxCut > f.opts.MaxSize {
		maxCut = f.opts.MaxSize
	}

	avgCut := f.opts.AvgSize
	if avgCut > maxCut {
		avgCut = maxCut
	}

	var fp uint64 = 0

	// Phase 1: Scan between MinSize and AvgSize using maskS
	for i := f.opts.MinSize; i < avgCut; i++ {
		fp = (fp << 1) + gearTable[data[i]]
		if (fp & f.maskS) == 0 {
			return int(i + 1)
		}
	}

	// Phase 2: Scan between AvgSize and MaxSize using maskL
	for i := avgCut; i < maxCut; i++ {
		fp = (fp << 1) + gearTable[data[i]]
		if (fp & f.maskL) == 0 {
			return int(i + 1)
		}
	}

	// Reached MaxSize or EOF
	if maxCut == f.opts.MaxSize || isEOF {
		return int(maxCut)
	}

	return 0
}

// Split chunks an entire in-memory slice and passes each chunk to cb.
func (f *FastCDC) Split(data []byte, cb func(chunk []byte) error) error {
	offset := 0
	for offset < len(data) {
		remaining := data[offset:]
		cut := f.Cut(remaining, true)
		if cut <= 0 {
			cut = len(remaining)
		}
		if err := cb(remaining[:cut]); err != nil {
			return err
		}
		offset += cut
	}
	return nil
}

// Chunker streams chunks from an io.Reader.
type Chunker struct {
	cdc   *FastCDC
	r     io.Reader
	buf   []byte
	start int
	end   int
	eof   bool
}

// NewChunker creates a streaming chunker reading from r.
func NewChunker(r io.Reader, opts Options) (*Chunker, error) {
	c, err := NewFastCDC(opts)
	if err != nil {
		return nil, err
	}
	return &Chunker{
		cdc: c,
		r:   r,
		buf: make([]byte, opts.MaxSize*4),
	}, nil
}

// Next returns the next content-defined chunk or io.EOF when done.
func (c *Chunker) Next() ([]byte, error) {
	for {
		avail := c.end - c.start
		cut := c.cdc.Cut(c.buf[c.start:c.end], c.eof)
		if cut > 0 {
			res := make([]byte, cut)
			copy(res, c.buf[c.start:c.start+cut])
			c.start += cut
			return res, nil
		}

		if c.eof {
			if avail > 0 {
				res := make([]byte, avail)
				copy(res, c.buf[c.start:c.end])
				c.start = c.end
				return res, nil
			}
			return nil, io.EOF
		}

		// Realign buffer if needed
		if c.start > 0 {
			copy(c.buf, c.buf[c.start:c.end])
			c.end -= c.start
			c.start = 0
		}

		// Read more data into buffer
		n, err := c.r.Read(c.buf[c.end:])
		if n > 0 {
			c.end += n
		}
		if err == io.EOF {
			c.eof = true
		} else if err != nil {
			return nil, err
		}
	}
}

// Standard 256-element 64-bit Gear lookup table
var gearTable = [256]uint64{
	0x8f26a1b24e8701cd, 0xa701b3d56e2981fa, 0x1f9b32c6d48e02ab, 0x5a1b3c7e9f02468d,
	0xb2c4d6e8fa013579, 0x3e5f7a9b1c2d4e6f, 0x8a9b0c1d2e3f4a5b, 0x6c7d8e9f0a1b2c3d,
	0x4e5f6a7b8c9d0e1f, 0x2a3b4c5d6e7f8a9b, 0x0c1d2e3f4a5b6c7d, 0x8e9f0a1b2c3d4e5f,
	0x6a7b8c9d0e1f2a3b, 0x4c5d6e7f8a9b0c1d, 0x2e3f4a5b6c7d8e9f, 0x0a1b2c3d4e5f6a7b,
	0x8c9d0e1f2a3b4c5d, 0x6e7f8a9b0c1d2e3f, 0x4a5b6c7d8e9f0a1b, 0x2c3d4e5f6a7b8c9d,
	0x0e1f2a3b4c5d6e7f, 0x8a9b0c1d2e3f4a5b, 0x6c7d8e9f0a1b2c3d, 0x4e5f6a7b8c9d0e1f,
	0x2a3b4c5d6e7f8a9b, 0x0c1d2e3f4a5b6c7d, 0x8e9f0a1b2c3d4e5f, 0x6a7b8c9d0e1f2a3b,
	0x4c5d6e7f8a9b0c1d, 0x2e3f4a5b6c7d8e9f, 0x0a1b2c3d4e5f6a7b, 0x8c9d0e1f2a3b4c5d,
	0x6e7f8a9b0c1d2e3f, 0x4a5b6c7d8e9f0a1b, 0x2c3d4e5f6a7b8c9d, 0x0e1f2a3b4c5d6e7f,
	0x9b1c2d3e4f5a6b7c, 0x7d8e9f0a1b2c3d4e, 0x5f6a7b8c9d0e1f2a, 0x3b4c5d6e7f8a9b0c,
	0x1d2e3f4a5b6c7d8e, 0x9f0a1b2c3d4e5f6a, 0x7b8c9d0e1f2a3b4c, 0x5d6e7f8a9b0c1d2e,
	0x3f4a5b6c7d8e9f0a, 0x1b2c3d4e5f6a7b8c, 0x9d0e1f2a3b4c5d6e, 0x7f8a9b0c1d2e3f4a,
	0x5b6c7d8e9f0a1b2c, 0x3d4e5f6a7b8c9d0e, 0x1f2a3b4c5d6e7f8a, 0x9b0c1d2e3f4a5b6c,
	0x7d8e9f0a1b2c3d4e, 0x5f6a7b8c9d0e1f2a, 0x3b4c5d6e7f8a9b0c, 0x1d2e3f4a5b6c7d8e,
	0x9f0a1b2c3d4e5f6a, 0x7b8c9d0e1f2a3b4c, 0x5d6e7f8a9b0c1d2e, 0x3f4a5b6c7d8e9f0a,
	0x1b2c3d4e5f6a7b8c, 0x9d0e1f2a3b4c5d6e, 0x7f8a9b0c1d2e3f4a, 0x5b6c7d8e9f0a1b2c,
	0xd1e2f3a4b5c6d7e8, 0xb9c0d1e2f3a4b5c6, 0x97a8b9c0d1e2f3a4, 0x758697a8b9c0d1e2,
	0x5364758697a8b9c0, 0x31425364758697a8, 0x0f20314253647586, 0xedfe0f2031425364,
	0xcbdccbdccbdccbdc, 0xa9baa9baa9baa9ba, 0x8798879887988798, 0x6576657665766576,
	0x4354435443544354, 0x2132213221322132, 0xff10ff10ff10ff10, 0xddeeeddeeeddeeed,
	0xbbccbbccbbccbbcc, 0x99aa99aa99aa99aa, 0x7788778877887788, 0x5566556655665566,
	0x3344334433443344, 0x1122112211221122, 0xef00ef00ef00ef00, 0xcddecddecddecdde,
	0xabbcabbcabbcabbc, 0x899a899a899a899a, 0x6778677867786778, 0x4556455645564556,
	0x2334233423342334, 0x0112011201120112, 0xdf00df00df00df00, 0xbddcbddcbddcbddc,
	0x9bba9bba9bba9bba, 0x7998799879987998, 0x5776577657765776, 0x3554355435543554,
	0x1332133213321332, 0xf110f110f110f110, 0xcfeecfeecfeecfee, 0xadccadccadccadcc,
	0x8baa8baa8baa8baa, 0x6988698869886988, 0x4766476647664766, 0x2544254425442544,
	0x0322032203220322, 0xe100e100e100e100, 0xbfdedec0bfdedec0, 0x9dbcbca09dbcbca0,
	0x7b9a9a807b9a9a80, 0x5978786059787860, 0x3756564037565640, 0x1534342015343420,
	0xf3121200f3121200, 0xd0f0f0e0d0f0f0e0, 0xaececec0aececec0, 0x8cacaca08cacaca0,
	0x6a8a8a806a8a8a80, 0x4868686048686860, 0x2646464026464640, 0x0424242004242420,
	0xe2020200e2020200, 0xc0e0e0e0c0e0e0e0, 0x9ec0c0c09ec0c0c0, 0x7ca0a0a07ca0a0a0,
	0x5a8080805a808080, 0x3860606038606060, 0x1640404016404040, 0xf4202020f4202020,
	0xd2000000d2000000, 0xb0e0e0e0b0e0e0e0, 0x8ec0c0c08ec0c0c0, 0x6ca0a0a06ca0a0a0,
	0x4a8080804a808080, 0x2860606028606060, 0x0640404006404040, 0xe4202020e4202020,
	0xc2000000c2000000, 0xa0e0e0e0a0e0e0e0, 0x7ec0c0c07ec0c0c0, 0x5ca0a0a05ca0a0a0,
	0x3a8080803a808080, 0x1860606018606060, 0xf6404040f6404040, 0xd4202020d4202020,
	0xb2000000b2000000, 0x90e0e0e090e0e0e0, 0x6ec0c0c06ec0c0c0, 0x4ca0a0a04ca0a0a0,
	0x2a8080802a808080, 0x0860606008606060, 0xe6404040e6404040, 0xc4202020c4202020,
	0xa2000000a2000000, 0x80e0e0e080e0e0e0, 0x5ec0c0c05ec0c0c0, 0x3ca0a0a03ca0a0a0,
	0x1a8080801a808080, 0xf8606060f8606060, 0xd6404040d6404040, 0xb4202020b4202020,
	0x9200000092000000, 0x70e0e0e070e0e0e0, 0x4ec0c0c04ec0c0c0, 0x2ca0a0a02ca0a0a0,
	0x0a8080800a808080, 0xe8606060e8606060, 0xc6404040c6404040, 0xa4202020a4202020,
	0x8200000082000000, 0x60e0e0e060e0e0e0, 0x3ec0c0c03ec0c0c0, 0x1ca0a0a01ca0a0a0,
	0xfa808080fa808080, 0xd8606060d8606060, 0xb6404040b6404040, 0x9420202094202020,
	0x7200000072000000, 0x50e0e0e050e0e0e0, 0x2ec0c0c02ec0c0c0, 0x0ca0a0a00ca0a0a0,
	0xea808080ea808080, 0xc8606060c8606060, 0xa6404040a6404040, 0x8420202084202020,
	0x6200000062000000, 0x40e0e0e040e0e0e0, 0x1ec0c0c01ec0c0c0, 0xfca0a0a0fca0a0a0,
	0xd8000000d8000000, 0xb8000000b8000000, 0x9800000098000000, 0x7800000078000000,
	0x5800000058000000, 0x3800000038000000, 0x1800000018000000, 0xf8000000f8000000,
	0xd0000000d0000000, 0xb0000000b0000000, 0x9000000090000000, 0x7000000070000000,
	0x5000000050000000, 0x3000000030000000, 0x1000000010000000, 0xf0000000f0000000,
	0xc0000000c0000000, 0xa0000000a0000000, 0x8000000080000000, 0x6000000060000000,
	0x4000000040000000, 0x2000000020000000, 0x0000000000000000, 0xe0000000e0000000,
	0xc0000000c0000000, 0xa0000000a0000000, 0x8000000080000000, 0x6000000060000000,
	0x4000000040000000, 0x2000000020000000, 0x0000000000000000, 0xe0000000e0000000,
	0xc0000000c0000000, 0xa0000000a0000000, 0x8000000080000000, 0x6000000060000000,
	0x4000000040000000, 0x2000000020000000, 0x0000000000000000, 0xe0000000e0000000,
	0xc0000000c0000000, 0xa0000000a0000000, 0x8000000080000000, 0x6000000060000000,
	0x4000000040000000, 0x2000000020000000, 0x0000000000000000, 0xe0000000e0000000,
	0xc0000000c0000000, 0xa0000000a0000000, 0x8000000080000000, 0x6000000060000000,
	0x4000000040000000, 0x2000000020000000, 0x0000000000000000, 0xe0000000e0000000,
	0xc0000000c0000000, 0xa0000000a0000000, 0x8000000080000000, 0x6000000060000000,
	0x4000000040000000, 0x2000000020000000, 0x0000000000000000, 0xe0000000e0000000,
}
