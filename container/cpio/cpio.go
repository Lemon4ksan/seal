// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cpio implements the Unix CPIO archive format (.cpio), supporting SVR4 newc/crc and POSIX odc.
package cpio

import (
	"errors"
	"io/fs"
	"time"
)

// Magic signatures for CPIO archive formats.
const (
	// MagicNewc defines the SVR4 portable ASCII format without CRC ("070701").
	MagicNewc = "070701"

	// MagicCRC defines the SVR4 portable ASCII format with CRC ("070702").
	MagicCRC = "070702"

	// MagicODC defines the POSIX.1-1988 portable ASCII format ("070707").
	MagicODC = "070707"

	// TrailerName marks the terminating record in a CPIO archive.
	TrailerName = "TRAILER!!!"

	// HeaderSizeNewc is the byte length of the SVR4 newc/crc ASCII header.
	HeaderSizeNewc = 110

	// HeaderSizeODC is the byte length of the POSIX odc ASCII header.
	HeaderSizeODC = 76
)

// POSIX mode bitmasks.
const (
	ModeTypeMask uint32 = 0o170000
	ModeTypeReg  uint32 = 0o100000
	ModeTypeDir  uint32 = 0o040000
	ModeTypeSym  uint32 = 0o120000
)

// Common CPIO errors.
var (
	ErrInvalidMagic    = errors.New("cpio: invalid magic number")
	ErrCorruptedHeader = errors.New("cpio: corrupted or truncated header")
)

// Header represents a parsed CPIO file entry header.
type Header struct {
	Magic      string
	Ino        uint64
	Mode       uint32
	UID        uint64
	GID        uint64
	NLink      uint64
	MTime      time.Time
	FileSize   int64
	DevMajor   uint64
	DevMinor   uint64
	RDevMajor  uint64
	RDevMinor  uint64
	Name       string
	Checksum   uint32
}

// IsDir returns true if the mode represents a directory.
func (h *Header) IsDir() bool {
	return h.Mode&ModeTypeMask == ModeTypeDir
}

// FileMode converts the CPIO POSIX mode to [fs.FileMode].
func (h *Header) FileMode() fs.FileMode {
	m := fs.FileMode(h.Mode & 0o777)
	if h.IsDir() {
		m |= fs.ModeDir
	}
	if h.Mode&ModeTypeMask == ModeTypeSym {
		m |= fs.ModeSymlink
	}
	return m
}

// Pad4 computes the number of zero-padding bytes needed to align n to a 4-byte boundary.
func Pad4(n int64) int64 {
	rem := n % 4
	if rem == 0 {
		return 0
	}
	return 4 - rem
}
