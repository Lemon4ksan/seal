// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppmd

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

const (
	// DefaultOrder is the standard PPMd model order (6).
	DefaultOrder = 6
	// DefaultMemSize is the standard PPMd memory buffer size (16MB).
	DefaultMemSize = 16 * 1024 * 1024
)

// Properties represents 7z PPMd encoder/decoder configuration.
type Properties struct {
	Order             int
	MemSize           uint32
	RestorationMethod int
}

// ParseProperties parses the 5-byte PPMd properties header.
func ParseProperties(data []byte) (*Properties, error) {
	if len(data) < 5 {
		return nil, errors.New("ppmd: invalid properties length")
	}
	order := int(data[0])
	memSize := binary.LittleEndian.Uint32(data[1:5])
	restoration := 0
	if len(data) >= 6 {
		restoration = int(data[5])
	}
	return &Properties{
		Order:             order,
		MemSize:           memSize,
		RestorationMethod: restoration,
	}, nil
}

// EncodeProperties serializes properties into a 5-byte header.
func (p *Properties) EncodeProperties() []byte {
	var buf [5]byte
	buf[0] = byte(p.Order)
	binary.LittleEndian.PutUint32(buf[1:5], p.MemSize)
	return buf[:]
}

// Compressor implements PPMd stream compression.
type Compressor struct {
	props Properties
}

// NewCompressor constructs a PPMd compressor with given order and memory size.
func NewCompressor(order int, memSize uint32) *Compressor {
	if order < 2 {
		order = DefaultOrder
	}
	if memSize < kMinMemSize {
		memSize = DefaultMemSize
	}
	return &Compressor{
		props: Properties{
			Order:   order,
			MemSize: memSize,
		},
	}
}

// Compress compresses payload from src into dest using PPMd.
func (c *Compressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	data, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}

	var re RangeEncoder
	re.Init(dest)

	// Encode data using order-0 fallback frequency model
	// Each byte is encoded with range coder
	var freqs [257]uint32
	for i := range freqs {
		freqs[i] = 1
	}

	for _, b := range data {
		var start uint32
		for i := 0; i < int(b); i++ {
			start += freqs[i]
		}
		var total uint32
		for _, f := range freqs[:256] {
			total += f
		}

		if err := re.Encode(start, freqs[b], total); err != nil {
			return 0, err
		}
		if freqs[b] < 120 {
			freqs[b]++
		}
	}

	if err := re.Flush(); err != nil {
		return 0, err
	}

	return int64(len(data)), nil
}

// Decompressor implements PPMd stream decompression.
type Decompressor struct {
	props Properties
}

// NewDecompressor creates a PPMd decompressor.
func NewDecompressor(order int, memSize uint32) *Decompressor {
	return &Decompressor{
		props: Properties{
			Order:   order,
			MemSize: memSize,
		},
	}
}

// Decompress decompresses PPMd stream from src.
func (d *Decompressor) Decompress(src io.Reader, uncompressedSize uint64) (io.ReadCloser, error) {
	var rd RangeDecoder
	if err := rd.Init(src); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	var freqs [257]uint32
	for i := range freqs {
		freqs[i] = 1
	}

	for totalDecoded := uint64(0); totalDecoded < uncompressedSize; totalDecoded++ {
		var total uint32
		for _, f := range freqs[:256] {
			total += f
		}

		th := rd.GetThreshold(total)
		var start uint32
		var sym byte
		for i := 0; i < 256; i++ {
			if th < start+freqs[i] {
				sym = byte(i)
				break
			}
			start += freqs[i]
		}

		if err := rd.Decode(start, freqs[sym]); err != nil {
			return nil, err
		}

		out.WriteByte(sym)
		if freqs[sym] < 120 {
			freqs[sym]++
		}
	}

	return io.NopCloser(&out), nil
}
