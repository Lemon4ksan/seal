// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package xz implements the .xz file format specification (RFC / XZ 1.1.0).
package xz

import (
	"errors"
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/codec/compress/lzma"
)

func init() {
	codec.DefaultRegistry.RegisterCompressor(codec.XZ, func(_ int) codec.Compressor {
		return &xzCompressor{}
	})
	codec.DefaultRegistry.RegisterDecompressor(codec.XZ, func() codec.Decompressor {
		return &xzDecompressor{}
	})
	codec.DefaultRegistry.RegisterCompressor(codec.LZMA2, func(level int) codec.Compressor {
		comp := lzma.NewCompressor2(8 * 1024 * 1024)
		comp.Options = lzma.OptionsForLevel(lzma.Level(level))
		return comp
	})
	codec.DefaultRegistry.RegisterDecompressor(codec.LZMA2, func() codec.Decompressor {
		return lzma.NewDecompressor2(8 * 1024 * 1024)
	})
}

type xzCompressor struct{}

func (x *xzCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	w, err := NewWriter(dest)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(w, src)
	if err != nil {
		return n, err
	}
	return n, w.Close()
}

type xzDecompressor struct{}

func (x *xzDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	return NewReader(src)
}

var (
	// HeaderMagic is the 6-byte magic bytes at the beginning of an XZ stream.
	HeaderMagic = [6]byte{0xFD, '7', 'z', 'X', 'Z', 0x00}
	// FooterMagic is the 2-byte magic bytes at the end of an XZ stream.
	FooterMagic = [2]byte{'Y', 'Z'}
)

// CheckType defines the integrity check algorithm used by an XZ stream block.
type CheckType byte

const (
	CheckNone   CheckType = 0x00
	CheckCRC32  CheckType = 0x01
	CheckCRC64  CheckType = 0x04
	CheckSHA256 CheckType = 0x0A
)

// CheckSize returns the byte size of the check digest.
func (c CheckType) CheckSize() int {
	switch c {
	case CheckNone:
		return 0
	case CheckCRC32:
		return 4
	case CheckCRC64:
		return 8
	case CheckSHA256:
		return 32
	default:
		return 0
	}
}

// Filter IDs supported in the XZ block header.
const (
	FilterDelta    uint64 = 0x03
	FilterX86      uint64 = 0x04
	FilterPowerPC  uint64 = 0x05
	FilterIA64     uint64 = 0x06
	FilterARM      uint64 = 0x07
	FilterARMThumb uint64 = 0x08
	FilterSPARC    uint64 = 0x09
	FilterLZMA2    uint64 = 0x21
)

// Sentinel XZ errors.
var (
	ErrInvalidHeaderMagic  = errors.New("xz: invalid stream header magic")
	ErrInvalidFooterMagic  = errors.New("xz: invalid stream footer magic")
	ErrHeaderCRCMismatch   = errors.New("xz: stream header CRC32 mismatch")
	ErrFooterCRCMismatch   = errors.New("xz: stream footer CRC32 mismatch")
	ErrBlockCRCMismatch    = errors.New("xz: block header CRC32 mismatch")
	ErrDataCheckMismatch   = errors.New("xz: block payload data check mismatch")
	ErrIndexCRCMismatch    = errors.New("xz: index CRC32 mismatch")
	ErrUnsupportedCheck    = errors.New("xz: unsupported check type")
	ErrUnsupportedFilter   = errors.New("xz: unsupported filter in block")
	ErrStreamFlagsMismatch = errors.New("xz: stream header and footer flags mismatch")
)

// ReadVLI decodes a variable-length integer (VLI) from r.
func ReadVLI(r io.ByteReader) (uint64, error) {
	var num uint64
	for i := 0; i < 9; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		num |= uint64(b&0x7F) << (i * 7)
		if (b & 0x80) == 0 {
			if b == 0 && i > 0 {
				return 0, errors.New("xz: non-minimal variable-length integer")
			}
			return num, nil
		}
	}
	return 0, errors.New("xz: variable-length integer exceeds maximum size")
}

// WriteVLI encodes a variable-length integer (VLI) into w.
func WriteVLI(w io.Writer, val uint64) (int, error) {
	var buf [9]byte
	pos := 0
	for val >= 0x80 {
		buf[pos] = byte(val | 0x80)
		pos++
		val >>= 7
	}
	buf[pos] = byte(val)
	pos++
	return w.Write(buf[:pos])
}

// VLISize returns the number of bytes required to encode val as a VLI.
func VLISize(val uint64) int {
	size := 1
	for val >= 0x80 {
		size++
		val >>= 7
	}
	return size
}

type byteReaderWrapper struct {
	r   io.Reader
	buf [1]byte
}

func (b *byteReaderWrapper) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

func (b *byteReaderWrapper) ReadByte() (byte, error) {
	_, err := io.ReadFull(b.r, b.buf[:])
	return b.buf[0], err
}
