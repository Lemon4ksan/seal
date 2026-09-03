// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cab implements the Microsoft Cabinet (.cab) archive container format.
package cab

import (
	"errors"
	"time"
)

// Cabinet format signatures and limits.
const (
	// MagicSignature defines the 4-byte MSCF signature ("MSCF").
	MagicSignature uint32 = 0x4643534D

	// MaxBlockUncompressedSize is the maximum uncompressed size of a CFDATA block (32 KB).
	MaxBlockUncompressedSize = 32768

	// MaxBlockCompressedSize is the maximum compressed size of a CFDATA block.
	MaxBlockCompressedSize = 65535
)

// Header flags.
const (
	FlagPrevCabinet    uint16 = 0x0001
	FlagNextCabinet    uint16 = 0x0002
	FlagReservePresent uint16 = 0x0004
)

// Compression type bitmasks and algorithms.
const (
	CompTypeMask uint16 = 0x000F

	CompTypeNone    uint16 = 0x0000
	CompTypeMSZip   uint16 = 0x0001
	CompTypeQuantum uint16 = 0x0002
	CompTypeLZX     uint16 = 0x0003
)

// File attributes (MS-DOS).
const (
	AttrReadOnly  uint16 = 0x0001
	AttrHidden    uint16 = 0x0002
	AttrSystem    uint16 = 0x0004
	AttrDirectory uint16 = 0x0010
	AttrArchive   uint16 = 0x0020
	AttrNameIsUTF uint16 = 0x0080
)

// Common CAB errors.
var (
	ErrInvalidSignature   = errors.New("cab: invalid MSCF cabinet signature")
	ErrUnsupportedVersion = errors.New("cab: unsupported cabinet version")
	ErrUnsupportedComp    = errors.New("cab: unsupported cabinet compression method")
	ErrCorruptedCabinet   = errors.New("cab: corrupted cabinet header or structure")
	ErrCorruptedBlock     = errors.New("cab: corrupted or truncated data block")
)

// Header mirrors the on-disk CFHEADER structure.
type Header struct {
	Signature    uint32
	Reserved1    uint32
	CbCabinet    uint32
	Reserved2    uint32
	CoffFiles    uint32
	Reserved3    uint32
	VersionMinor uint8
	VersionMajor uint8
	CFolders     uint16
	CFiles       uint16
	Flags        uint16
	SetID        uint16
	ICabinet     uint16

	// Present if FlagReservePresent is set
	CbCFHeader uint16
	CbCFFolder uint8
	CbCFData   uint8
	AbReserve  []byte

	// Present if FlagPrevCabinet is set
	SzCabinetPrev string
	SzDiskPrev    string

	// Present if FlagNextCabinet is set
	SzCabinetNext string
	SzDiskNext    string
}

// Folder mirrors the on-disk CFFOLDER structure.
type Folder struct {
	CoffCabStart uint32
	CCFData      uint16
	TypeCompress uint16
	AbReserve    []byte
}

// CompressionMethod returns the base compression type (NONE, MSZIP, QUANTUM, LZX).
func (f *Folder) CompressionMethod() uint16 {
	return f.TypeCompress & CompTypeMask
}

// LZXWindowBits returns the window size in bits (15..21) if the folder uses LZX compression.
func (f *Folder) LZXWindowBits() int {
	bits := int((f.TypeCompress >> 8) & 0x1F)
	if bits < 15 || bits > 21 {
		return 15
	}
	return bits
}

// FileEntry mirrors the on-disk CFFILE structure.
type FileEntry struct {
	CbFile          uint32
	UoffFolderStart uint32
	IFolder         uint16
	Date            uint16
	Time            uint16
	Attribs         uint16
	Name            string
}

// ModTime converts MS-DOS date and time words to [time.Time].
func (e *FileEntry) ModTime() time.Time {
	year := int((e.Date >> 9) & 0x7F) + 1980
	month := time.Month((e.Date >> 5) & 0x0F)
	day := int(e.Date & 0x1F)

	hour := int((e.Time >> 11) & 0x1F)
	minute := int((e.Time >> 5) & 0x3F)
	second := int((e.Time & 0x1F) * 2)

	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}
	}

	return time.Date(year, month, day, hour, minute, second, 0, time.UTC)
}

// TimeToDos converts a [time.Time] into MS-DOS date and time words.
func TimeToDos(t time.Time) (uint16, uint16) {
	if t.IsZero() {
		t = time.Now()
	}
	t = t.UTC()

	year := t.Year()
	if year < 1980 {
		year = 1980
	} else if year > 2107 {
		year = 2107
	}

	date := uint16((year-1980)<<9) | uint16(int(t.Month())<<5) | uint16(t.Day())
	dosTime := uint16(t.Hour()<<11) | uint16(t.Minute()<<5) | uint16(t.Second()/2)

	return date, dosTime
}

// DataBlock mirrors the on-disk CFDATA structure.
type DataBlock struct {
	Csum     uint32
	CbData   uint16
	CbUncomp uint16
}
