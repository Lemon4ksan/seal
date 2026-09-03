// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package iso implements the ISO 9660 and Joliet optical disc image (.iso, .img) container format.
package iso

import (
	"encoding/binary"
	"errors"
	"strings"
	"time"
	"unicode/utf16"
)

// ISO 9660 structural constants.
const (
	// SectorSize is the logical sector size of an ISO 9660 volume (2048 bytes).
	SectorSize = 2048

	// SystemAreaSectors defines the count of reserved system sectors (16 sectors = 32768 bytes).
	SystemAreaSectors = 16

	// SystemAreaSize defines the byte offset of the volume descriptor area (32768 bytes).
	SystemAreaSize = SystemAreaSectors * SectorSize

	// StandardIdentifier defines the 5-byte ISO 9660 standard signature ("CD001").
	StandardIdentifier = "CD001"
)

// Volume descriptor types.
const (
	VDTypeBootRecord                  uint8 = 0x00
	VDTypePrimaryVolumeDescriptor     uint8 = 0x01
	VDTypeSupplementaryVolumeDescriptor uint8 = 0x02
	VDTypeVolumePartitionDescriptor   uint8 = 0x03
	VDTypeVolumeDescriptorSetTerminator uint8 = 0xFF
)

// File flags in directory records.
const (
	FileFlagHidden    uint8 = 0x01
	FileFlagDirectory uint8 = 0x02
	FileFlagAssociated uint8 = 0x04
)

// Common ISO errors.
var (
	ErrInvalidISO       = errors.New("iso: not a valid ISO 9660 disc image")
	ErrCorruptedVolume  = errors.New("iso: corrupted volume descriptor table")
	ErrCorruptedDirectory = errors.New("iso: corrupted directory record")
)

// DirectoryRecord represents a parsed ISO 9660 / Joliet directory record.
type DirectoryRecord struct {
	Length        uint8
	ExtAttrLength uint8
	ExtentLBA     uint32
	DataLength    uint32
	DateTime      [7]byte
	Flags         uint8
	Name          string
}

// IsDir returns true if the record denotes a subdirectory.
func (dr *DirectoryRecord) IsDir() bool {
	return dr.Flags&FileFlagDirectory != 0
}

// ModTime converts the 7-byte ISO directory date/time to [time.Time].
func (dr *DirectoryRecord) ModTime() time.Time {
	year := int(dr.DateTime[0]) + 1900
	month := time.Month(dr.DateTime[1])
	day := int(dr.DateTime[2])
	hour := int(dr.DateTime[3])
	minute := int(dr.DateTime[4])
	second := int(dr.DateTime[5])

	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}
	}

	// 15-minute offset from GMT
	offsetMin := int(int8(dr.DateTime[6])) * 15
	loc := time.FixedZone("ISO", offsetMin*60)
	return time.Date(year, month, day, hour, minute, second, 0, loc).UTC()
}

// ParseDirectoryRecord parses a directory record from a byte slice.
func ParseDirectoryRecord(b []byte, isJoliet bool) (*DirectoryRecord, int, error) {
	if len(b) == 0 {
		return nil, 0, nil
	}
	recLen := int(b[0])
	if recLen == 0 {
		return nil, 0, nil // Zero-byte padding at end of sector
	}
	if recLen < 33 || len(b) < recLen {
		return nil, 0, ErrCorruptedDirectory
	}

	dr := &DirectoryRecord{
		Length:        b[0],
		ExtAttrLength: b[1],
		ExtentLBA:     binary.LittleEndian.Uint32(b[2:6]),
		DataLength:    binary.LittleEndian.Uint32(b[10:14]),
		Flags:         b[25],
	}
	copy(dr.DateTime[:], b[18:25])

	nameLen := int(b[32])
	if 33+nameLen > recLen {
		return nil, 0, ErrCorruptedDirectory
	}

	rawName := b[33 : 33+nameLen]
	if nameLen == 1 && rawName[0] == 0 {
		dr.Name = "."
	} else if nameLen == 1 && rawName[0] == 1 {
		dr.Name = ".."
	} else if isJoliet {
		dr.Name = DecodeUTF16BE(rawName)
	} else {
		dr.Name = string(rawName)
	}

	// Strip ISO 9660 version suffix (e.g. ";1")
	if idx := strings.Index(dr.Name, ";"); idx != -1 {
		dr.Name = dr.Name[:idx]
	}

	return dr, recLen, nil
}

// DecodeUTF16BE decodes big-endian UTF-16 bytes into a Go string.
func DecodeUTF16BE(b []byte) string {
	if len(b) < 2 {
		return string(b)
	}
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = binary.BigEndian.Uint16(b[i*2 : i*2+2])
	}
	return string(utf16.Decode(u16))
}

// EncodeUTF16BE encodes a Go string into big-endian UTF-16 bytes.
func EncodeUTF16BE(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	b := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.BigEndian.PutUint16(b[i*2:i*2+2], v)
	}
	return b
}

// PutBothEndianUint32 writes a 32-bit integer in both Little-Endian and Big-Endian order (8 bytes total).
func PutBothEndianUint32(b []byte, v uint32) {
	binary.LittleEndian.PutUint32(b[0:4], v)
	binary.BigEndian.PutUint32(b[4:8], v)
}

// PutBothEndianUint16 writes a 16-bit integer in both Little-Endian and Big-Endian order (4 bytes total).
func PutBothEndianUint16(b []byte, v uint16) {
	binary.LittleEndian.PutUint16(b[0:2], v)
	binary.BigEndian.PutUint16(b[2:4], v)
}

// FormatDateTime formats a [time.Time] into standard 7-byte ISO directory date/time format.
func FormatDateTime(t time.Time) [7]byte {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	t = t.UTC()

	year := t.Year()
	if year < 1900 {
		year = 1900
	} else if year > 2155 {
		year = 2155
	}

	return [7]byte{
		byte(year - 1900),
		byte(t.Month()),
		byte(t.Day()),
		byte(t.Hour()),
		byte(t.Minute()),
		byte(t.Second()),
		0, // GMT offset
	}
}
