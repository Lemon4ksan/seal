// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sevenzip implements the 7-Zip (.7z) archive container specification.
package sevenzip

import (
	"encoding/binary"
	"io"
	"time"
)

const (
	// SignatureSize defines the length of the 7-Zip magic signature (6 bytes).
	SignatureSize = 6
	// StartHeaderSize defines the size of the 7-Zip start header following the signature (32 bytes).
	StartHeaderSize = 32
)

// Signature is the canonical 7-Zip magic signature bytes: '7', 'z', 0xBC, 0xAF, 0x27, 0x1C.
var Signature = [SignatureSize]byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}

// Standard 7-Zip Header Property IDs (NID).
const (
	NIDEnd                   = 0x00
	NIDHeader                = 0x01
	NIDArchiveProperties     = 0x02
	NIDAdditionalStreamsInfo = 0x03
	NIDMainStreamsInfo       = 0x04
	NIDFilesInfo             = 0x05
	NIDPackInfo              = 0x06
	NIDUnpackInfo            = 0x07
	NIDSubStreamsInfo        = 0x08
	NIDSize                  = 0x09
	NIDCRC                   = 0x0A
	NIDFolder                = 0x0B
	NIDCodersUnpackSize      = 0x0C
	NIDNumUnpackStream       = 0x0D
	NIDEmptyStream           = 0x0E
	NIDEmptyFile             = 0x0F
	NIDAnti                  = 0x10
	NIDName                  = 0x11
	NIDCTime                 = 0x12
	NIDATime                 = 0x13
	NIDMTime                 = 0x14
	NIDWinAttrib             = 0x15
	NIDComment               = 0x16
	NIDEncodedHeader         = 0x17
	NIDStartPos              = 0x18
	NIDDummy                 = 0x19
)

// Standard 7-Zip Coder Method IDs.
const (
	MethodCopy    = 0x00
	MethodDelta   = 0x03
	MethodLZMA    = 0x030101
	MethodLZMA2   = 0x21
	MethodAES     = 0x06F10701
	MethodBCJ     = 0x03030103
	MethodBCJ2    = 0x0303011B
	MethodARM     = 0x03030501
	MethodPPMD    = 0x030401
	MethodBZip2   = 0x040202
	MethodDeflate = 0x040108
)

// StartHeader represents the physical header located at offset 0 of a .7z file.
type StartHeader struct {
	NextHeaderOffset uint64
	NextHeaderSize   uint64
	NextHeaderCRC    uint32
}

// CoderInfo describes a single coder in a Folder transformation graph.
type CoderInfo struct {
	MethodID      uint64
	NumInStreams  int
	NumOutStreams int
	Properties    []byte
}

// BindPair connects an output stream of one coder to an input stream of another coder in a Folder.
type BindPair struct {
	InIndex  int
	OutIndex int
}

// Folder represents a solid block of compressed data produced by a chain of coders.
type Folder struct {
	Coders           []CoderInfo
	BindPairs        []BindPair
	PackedIndices    []int
	UnpackSizes      []uint64
	UnpackCRC        uint32
	HasCRC           bool
	NumUnpackStreams int
}

// FileEntry represents a file entry in the 7z metadata table.
type FileEntry struct {
	Name           string
	IsDir          bool
	IsEmptyStream  bool
	IsEmptyFile    bool
	IsAnti         bool
	Size           uint64
	CRC            uint32
	HasCRC         bool
	ModTime        time.Time
	HasModTime     bool
	Attrib         uint32
	HasAttrib      bool
	FolderIndex    int
	SubStreamIndex int
}

// ReadNumber reads a 7z variable-length 64-bit integer from r.
func ReadNumber(r io.Reader) (uint64, error) {
	var b [1]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	first := b[0]
	if (first & 0x80) == 0 {
		return uint64(first), nil
	}

	mask := byte(0x80)
	numExtra := 0
	for (first & mask) != 0 {
		numExtra++
		mask >>= 1
	}

	if numExtra == 8 {
		var full [8]byte
		if _, err := io.ReadFull(r, full[:]); err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint64(full[:]), nil
	}

	var extra [8]byte
	if _, err := io.ReadFull(r, extra[:numExtra]); err != nil {
		return 0, err
	}

	high := uint64(first & (mask - 1))
	var val uint64
	for i := 0; i < numExtra; i++ {
		val |= uint64(extra[i]) << (8 * i)
	}
	return (high << (8 * numExtra)) | val, nil
}

// WriteNumber encodes a 64-bit integer as a 7z variable-length byte sequence to w.
func WriteNumber(w io.Writer, val uint64) error {
	if val < 0x80 {
		_, err := w.Write([]byte{byte(val)})
		return err
	}

	var mask byte = 0x80
	for extra := 1; extra < 8; extra++ {
		limit := uint64(1) << (8*extra + (7 - extra))
		if val < limit {
			first := mask | byte(val>>(8*extra))
			buf := make([]byte, 1+extra)
			buf[0] = first
			for j := 0; j < extra; j++ {
				buf[1+j] = byte(val >> (8 * j))
			}
			_, err := w.Write(buf)
			return err
		}
		mask = (mask >> 1) | 0x80
	}

	var buf [9]byte
	buf[0] = 0xFF
	binary.LittleEndian.PutUint64(buf[1:9], val)
	_, err := w.Write(buf[:])
	return err
}

// ReadBoolVector reads a bit vector of length count from r.
func ReadBoolVector(r io.Reader, count int) ([]bool, error) {
	var allDefined [1]byte
	if _, err := io.ReadFull(r, allDefined[:]); err != nil {
		return nil, err
	}

	result := make([]bool, count)
	if allDefined[0] != 0 {
		for i := range result {
			result[i] = true
		}
		return result, nil
	}

	var b byte
	var mask byte = 0
	for i := 0; i < count; i++ {
		if mask == 0 {
			var buf [1]byte
			if _, err := io.ReadFull(r, buf[:]); err != nil {
				return nil, err
			}
			b = buf[0]
			mask = 0x80
		}
		result[i] = (b & mask) != 0
		mask >>= 1
	}
	return result, nil
}
