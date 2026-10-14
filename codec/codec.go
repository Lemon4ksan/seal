// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package codec defines compression and decompression engines for archive payloads.
package codec

import (
	"compress/flate"
	"io"
)

// Method represents a compression algorithm identifier.
type Method uint16

// Standard compression methods across archive specifications.
const (
	Store      Method = 0   // No compression (raw copy)
	Shrink     Method = 1   // Shrink algorithm
	Reduce1    Method = 2   // Reduce algorithm (factor 1)
	Reduce2    Method = 3   // Reduce algorithm (factor 2)
	Reduce3    Method = 4   // Reduce algorithm (factor 3)
	Reduce4    Method = 5   // Reduce algorithm (factor 4)
	Implode    Method = 6   // Implode algorithm
	Deflate    Method = 8   // DEFLATE (RFC 1951)
	Deflate64  Method = 9   // Enhanced Deflate (Deflate64)
	PKWAREDCL  Method = 10  // PKWARE Data Compression Library Implode (DCL)
	BZIP2      Method = 12  // BZIP2 (Burrows-Wheeler)
	LZMA       Method = 14  // LZMA
	LZMA2      Method = 15  // LZMA2
	IBMCmpsc   Method = 16  // IBM z/OS CMPSC
	ZStandard  Method = 93  // Zstandard (RFC 8878)
	XZ         Method = 95  // XZ stream container (LZMA2 + BCJ)
	Gzip       Method = 96  // GZIP container compression
	Zlib       Method = 97  // ZLIB container compression
	PPMd       Method = 98  // PPMd (Prediction by Partial Matching)
	Brotli     Method = 99  // Brotli (RFC 7932)
	Snappy     Method = 100 // Snappy framing
	PRZM       Method = 101 // PRZM (PrismLZ structured dictionary compression)
	PRZMBrotli Method = 102 // PRZM + RFC 7932 Brotli Web Static Dictionary
)

// String returns the human-readable identifier of the compression method.
func (m Method) String() string {
	switch m {
	case Store:
		return "Store"
	case Deflate:
		return "Deflate"
	case Deflate64:
		return "Deflate64"
	case PKWAREDCL:
		return "PKWARE-DCL"
	case BZIP2:
		return "BZIP2"
	case LZMA:
		return "LZMA"
	case LZMA2:
		return "LZMA2"
	case IBMCmpsc:
		return "IBM-CMPSC"
	case ZStandard:
		return "Zstandard"
	case XZ:
		return "XZ"
	case PPMd:
		return "PPMd"
	case Gzip:
		return "Gzip"
	case Zlib:
		return "Zlib"
	case Brotli:
		return "Brotli"
	case Snappy:
		return "Snappy"
	case PRZM:
		return "PRZM"
	case PRZMBrotli:
		return "PRZM-Brotli"
	default:
		return "Unknown"
	}
}

// Compression level constants for DEFLATE and general codecs.
const (
	DefaultCompression = flate.DefaultCompression // -1
	NoCompression      = flate.NoCompression      // 0
	BestSpeed          = flate.BestSpeed          // 1
	BestCompression    = flate.BestCompression    // 9
	HuffmanOnly        = flate.HuffmanOnly        // -2
)

// Compressor compresses an uncompressed byte stream from src into dest.
// It returns the total uncompressed bytes read from src, and any error encountered.
type Compressor interface {
	Compress(src io.Reader, dest io.Writer) (uncompressedBytes int64, err error)
}

// Decompressor wraps a compressed byte stream and returns an [io.ReadCloser] yielding uncompressed bytes.
type Decompressor interface {
	Decompress(src io.Reader) (io.ReadCloser, error)
}

// CompressorFactory defines a constructor function for instantiating a [Compressor] configured for a compression level.
type CompressorFactory func(level int) Compressor

// DecompressorFactory defines a constructor function for instantiating a [Decompressor].
type DecompressorFactory func() Decompressor
