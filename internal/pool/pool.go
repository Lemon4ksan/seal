// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pool provides tiered, hardware-aligned memory buffer pools and CRC-32 calculation pipelines.
package pool

import (
	"hash/crc32"
	"io"
	"sync"
)

const (
	// BufferSize32K represents a 32 KB buffer chunk size.
	BufferSize32K = 32 * 1024
	// BufferSize64K represents a 64 KB buffer chunk size (optimal for L2 cacheline throughput).
	BufferSize64K = 64 * 1024
	// BufferSize128K represents a 128 KB buffer chunk size.
	BufferSize128K = 128 * 1024
	// BufferSize256K represents a 256 KB buffer chunk size (optimal for high-throughput multi-core streaming).
	BufferSize256K = 256 * 1024
)

var (
	pool32K = sync.Pool{
		New: func() any {
			b := make([]byte, BufferSize32K)
			return &b
		},
	}
	pool64K = sync.Pool{
		New: func() any {
			b := make([]byte, BufferSize64K)
			return &b
		},
	}
	pool128K = sync.Pool{
		New: func() any {
			b := make([]byte, BufferSize128K)
			return &b
		},
	}
	pool256K = sync.Pool{
		New: func() any {
			b := make([]byte, BufferSize256K)
			return &b
		},
	}
)

// Get32K borrows a 32 KB slice from the memory pool.
func Get32K() *[]byte {
	return pool32K.Get().(*[]byte)
}

// Put32K returns a 32 KB slice to the memory pool.
func Put32K(b *[]byte) {
	if b != nil && cap(*b) >= BufferSize32K {
		*b = (*b)[:BufferSize32K]
		pool32K.Put(b)
	}
}

// Get64K borrows a 64 KB slice from the memory pool.
func Get64K() *[]byte {
	return pool64K.Get().(*[]byte)
}

// Put64K returns a 64 KB slice to the memory pool.
func Put64K(b *[]byte) {
	if b != nil && cap(*b) >= BufferSize64K {
		*b = (*b)[:BufferSize64K]
		pool64K.Put(b)
	}
}

// Get128K borrows a 128 KB slice from the memory pool.
func Get128K() *[]byte {
	return pool128K.Get().(*[]byte)
}

// Put128K returns a 128 KB slice to the memory pool.
func Put128K(b *[]byte) {
	if b != nil && cap(*b) >= BufferSize128K {
		*b = (*b)[:BufferSize128K]
		pool128K.Put(b)
	}
}

// Get256K borrows a 256 KB slice from the memory pool.
func Get256K() *[]byte {
	return pool256K.Get().(*[]byte)
}

// Put256K returns a 256 KB slice to the memory pool.
func Put256K(b *[]byte) {
	if b != nil && cap(*b) >= BufferSize256K {
		*b = (*b)[:BufferSize256K]
		pool256K.Put(b)
	}
}

// CRCReader wraps an [io.Reader] to compute the IEEE CRC-32 checksum and byte length on the fly.
type CRCReader struct {
	reader io.Reader
	crc    uint32
	size   int64
	table  *crc32.Table
}

// Read reads from the underlying stream and updates the CRC-32 checksum.
func (c *CRCReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	if n > 0 {
		c.crc = crc32.Update(c.crc, c.table, p[:n])
		c.size += int64(n)
	}
	return n, err
}

// CRC returns the accumulated CRC-32 checksum.
func (c *CRCReader) CRC() uint32 {
	return c.crc
}

// Size returns the total uncompressed bytes read so far.
func (c *CRCReader) Size() int64 {
	return c.size
}

// Reset re-initializes the reader for reuse.
func (c *CRCReader) Reset(r io.Reader) {
	c.reader = r
	c.crc = 0
	c.size = 0
	if c.table == nil {
		c.table = crc32.IEEETable
	}
}

var crcReaderPool = sync.Pool{
	New: func() any {
		return &CRCReader{table: crc32.IEEETable}
	},
}

// GetCRCReader borrows a re-initialized [CRCReader] wrapping the provided stream.
func GetCRCReader(r io.Reader) *CRCReader {
	cr := crcReaderPool.Get().(*CRCReader)
	cr.Reset(r)
	return cr
}

// PutCRCReader returns a [CRCReader] to the sync.Pool.
func PutCRCReader(cr *CRCReader) {
	if cr != nil {
		cr.reader = nil
		crcReaderPool.Put(cr)
	}
}
