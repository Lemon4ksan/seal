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
	// MaxSymbolCount is the total discrete byte alphabet size.
	MaxSymbolCount = 256
	// MaxFreqLimit is the maximum frequency before rescaling.
	MaxFreqLimit = 124
	// MaxTotalLimit is the threshold before model frequency rescaling.
	MaxTotalLimit = 16384
	// MaxContextOrder is the maximum allowed context tree depth.
	MaxContextOrder = 16
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

// FenwickModel maintains dynamic cumulative frequency distributions using a binary indexed tree.
type FenwickModel struct {
	freqs [MaxSymbolCount]uint16
	tree  [MaxSymbolCount + 1]uint32
	total uint32
}

// Init initializes the model with uniform symbol probabilities.
func (m *FenwickModel) Init() {
	m.total = MaxSymbolCount
	for i := 0; i < MaxSymbolCount; i++ {
		m.freqs[i] = 1
	}
	for i := 1; i <= MaxSymbolCount; i++ {
		m.tree[i] = 0
	}
	for i := 0; i < MaxSymbolCount; i++ {
		idx := i + 1
		for idx <= MaxSymbolCount {
			m.tree[idx]++
			idx += idx & (-idx)
		}
	}
}

// Rescale halves frequencies when total threshold is exceeded to prevent overflow.
func (m *FenwickModel) Rescale() {
	for i := 0; i < MaxSymbolCount; i++ {
		m.freqs[i] = (m.freqs[i] + 1) >> 1
	}
	for i := 1; i <= MaxSymbolCount; i++ {
		m.tree[i] = 0
	}
	var newTotal uint32
	for i := 0; i < MaxSymbolCount; i++ {
		val := uint32(m.freqs[i])
		newTotal += val
		idx := i + 1
		for idx <= MaxSymbolCount {
			m.tree[idx] += val
			idx += idx & (-idx)
		}
	}
	m.total = newTotal
}

// GetStart returns the cumulative frequency prefix sum before sym in O(log 256) = 4..8 steps.
func (m *FenwickModel) GetStart(sym byte) uint32 {
	var sum uint32
	idx := int(sym)
	for idx > 0 {
		sum += m.tree[idx]
		idx -= idx & (-idx)
	}
	return sum
}

// Update increments the frequency of sym and updates the Fenwick tree in O(log 256) steps.
func (m *FenwickModel) Update(sym byte) {
	if m.freqs[sym] < MaxFreqLimit {
		m.freqs[sym]++
		m.total++
		idx := int(sym) + 1
		for idx <= MaxSymbolCount {
			m.tree[idx]++
			idx += idx & (-idx)
		}
	}
	if m.total >= MaxTotalLimit {
		m.Rescale()
	}
}

// FindSymbol locates the symbol corresponding to cumulative threshold th in O(8) bit steps.
func (m *FenwickModel) FindSymbol(th uint32) (byte, uint32) {
	var idx int
	var currentSum uint32
	for mask := 128; mask > 0; mask >>= 1 {
		nextIdx := idx + mask
		if nextIdx <= MaxSymbolCount && currentSum+m.tree[nextIdx] <= th {
			idx = nextIdx
			currentSum += m.tree[nextIdx]
		}
	}
	if idx >= MaxSymbolCount {
		idx = MaxSymbolCount - 1
	}
	return byte(idx), currentSum
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

// Compress compresses payload from src into dest using context-predictive PPMd.
func (c *Compressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	data, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}

	var re RangeEncoder
	re.Init(dest)

	const numContexts = 65536
	type FastContext struct {
		model  FenwickModel
		inited bool
	}
	contexts := make([]FastContext, numContexts)

	var prev1, prev2 byte
	for _, b := range data {
		ctxIdx := (uint32(prev2)<<8 | uint32(prev1)) & (numContexts - 1)
		ctx := &contexts[ctxIdx]
		if !ctx.inited {
			ctx.model.Init()
			ctx.inited = true
		}

		start := ctx.model.GetStart(b)
		size := uint32(ctx.model.freqs[b])
		total := ctx.model.total

		if err := re.Encode(start, size, total); err != nil {
			return 0, err
		}

		ctx.model.Update(b)

		prev2 = prev1
		prev1 = b
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
	if uncompressedSize > 0 && uncompressedSize < 1<<31 {
		out.Grow(int(uncompressedSize))
	}

	const numContexts = 65536
	type FastContext struct {
		model  FenwickModel
		inited bool
	}
	contexts := make([]FastContext, numContexts)

	var prev1, prev2 byte
	for totalDecoded := uint64(0); totalDecoded < uncompressedSize; totalDecoded++ {
		ctxIdx := (uint32(prev2)<<8 | uint32(prev1)) & (numContexts - 1)
		ctx := &contexts[ctxIdx]
		if !ctx.inited {
			ctx.model.Init()
			ctx.inited = true
		}

		total := ctx.model.total
		th := rd.GetThreshold(total)

		sym, start := ctx.model.FindSymbol(th)
		size := uint32(ctx.model.freqs[sym])

		if err := rd.Decode(start, size); err != nil {
			return nil, err
		}

		out.WriteByte(sym)
		ctx.model.Update(sym)

		prev2 = prev1
		prev1 = sym
	}

	return io.NopCloser(&out), nil
}
