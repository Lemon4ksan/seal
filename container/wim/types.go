// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wim

import (
	"encoding/binary"
	"io"
	"time"
	"unicode/utf16"
)

// WIM Constants
const (
	Magic            = "MSWIM\x00\x00\x00"
	MagicLen         = 8
	HeaderSize       = 208
	Version          = 0x00010D00
	LookupEntrySize  = 50
	DentryFixedSize  = 102
	DefaultChunkSize = 32768
)

// Header Flags
const (
	FlagHeaderCompression    uint32 = 0x00000002
	FlagHeaderReadOnly       uint32 = 0x00000004
	FlagHeaderSpanned        uint32 = 0x00000008
	FlagHeaderResourceOnly   uint32 = 0x00000010
	FlagHeaderRPFix          uint32 = 0x00000080
	FlagHeaderCompressXpress uint32 = 0x00020000
	FlagHeaderCompressLZX    uint32 = 0x00040000
	FlagHeaderCompressLZMS   uint32 = 0x00080000
)

// Resource Flags (stored in the upper byte of SizeAndFlags)
const (
	ResFlagFree       uint8 = 0x01
	ResFlagMetadata   uint8 = 0x02
	ResFlagCompressed uint8 = 0x04
	ResFlagSpanned    uint8 = 0x08
)

// Windows File Attributes
const (
	AttributeReadOnly  uint32 = 0x00000001
	AttributeHidden    uint32 = 0x00000002
	AttributeSystem    uint32 = 0x00000004
	AttributeDirectory uint32 = 0x00000010
	AttributeArchive   uint32 = 0x00000020
	AttributeNormal    uint32 = 0x00000080
)

// ResHdr represents a 24-byte WIM resource descriptor.
type ResHdr struct {
	SizeAndFlags [8]byte // low 7 bytes = compressed size, byte 7 = flags
	Offset       uint64
	OriginalSize uint64
}

// CompressedSize returns the 56-bit resource size on disk.
func (r *ResHdr) CompressedSize() uint64 {
	var sz uint64
	for i := 0; i < 7; i++ {
		sz |= uint64(r.SizeAndFlags[i]) << (i * 8)
	}
	return sz
}

// Flags returns the 8-bit resource flags.
func (r *ResHdr) Flags() uint8 {
	return r.SizeAndFlags[7]
}

// Set populates the resource descriptor fields.
func (r *ResHdr) Set(compSize uint64, flags uint8, offset, origSize uint64) {
	for i := 0; i < 7; i++ {
		r.SizeAndFlags[i] = byte(compSize >> (i * 8))
	}
	r.SizeAndFlags[7] = flags
	r.Offset = offset
	r.OriginalSize = origSize
}

// Write serializes the 24-byte ResHdr into w.
func (r *ResHdr) Write(w io.Writer) error {
	if _, err := w.Write(r.SizeAndFlags[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, r.Offset); err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, r.OriginalSize)
}

// Read deserializes the 24-byte ResHdr from r.
func (r *ResHdr) Read(reader io.Reader) error {
	if _, err := io.ReadFull(reader, r.SizeAndFlags[:]); err != nil {
		return err
	}
	if err := binary.Read(reader, binary.LittleEndian, &r.Offset); err != nil {
		return err
	}
	return binary.Read(reader, binary.LittleEndian, &r.OriginalSize)
}

// Header represents the 208-byte WIM file header.
type Header struct {
	Magic        [8]byte
	HeaderSize   uint32
	Version      uint32
	Flags        uint32
	ChunkSize    uint32
	GUID         [16]byte
	PartNumber   uint16
	TotalParts   uint16
	ImageCount   uint32
	LookupTable  ResHdr
	XMLData      ResHdr
	BootMetadata ResHdr
	BootIndex    uint32
	Integrity    ResHdr
	Reserved     [60]byte
}

// Write serializes the 208-byte Header into w.
func (h *Header) Write(w io.Writer) error {
	if _, err := w.Write(h.Magic[:]); err != nil {
		return err
	}
	fields := []uint32{h.HeaderSize, h.Version, h.Flags, h.ChunkSize}
	for _, f := range fields {
		if err := binary.Write(w, binary.LittleEndian, f); err != nil {
			return err
		}
	}
	if _, err := w.Write(h.GUID[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, h.PartNumber); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, h.TotalParts); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, h.ImageCount); err != nil {
		return err
	}
	if err := h.LookupTable.Write(w); err != nil {
		return err
	}
	if err := h.XMLData.Write(w); err != nil {
		return err
	}
	if err := h.BootMetadata.Write(w); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, h.BootIndex); err != nil {
		return err
	}
	if err := h.Integrity.Write(w); err != nil {
		return err
	}
	_, err := w.Write(h.Reserved[:])
	return err
}

// Read deserializes the 208-byte Header from r.
func (h *Header) Read(r io.Reader) error {
	if _, err := io.ReadFull(r, h.Magic[:]); err != nil {
		return err
	}
	fields := []*uint32{&h.HeaderSize, &h.Version, &h.Flags, &h.ChunkSize}
	for _, f := range fields {
		if err := binary.Read(r, binary.LittleEndian, f); err != nil {
			return err
		}
	}
	if _, err := io.ReadFull(r, h.GUID[:]); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.PartNumber); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.TotalParts); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.ImageCount); err != nil {
		return err
	}
	if err := h.LookupTable.Read(r); err != nil {
		return err
	}
	if err := h.XMLData.Read(r); err != nil {
		return err
	}
	if err := h.BootMetadata.Read(r); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.BootIndex); err != nil {
		return err
	}
	if err := h.Integrity.Read(r); err != nil {
		return err
	}
	_, err := io.ReadFull(r, h.Reserved[:])
	return err
}

// LookupEntry represents a 50-byte resource lookup table entry.
type LookupEntry struct {
	ResHdr     ResHdr
	PartNumber uint16
	RefCount   uint32
	SHA1       [20]byte
}

// Write serializes the 50-byte LookupEntry into w.
func (e *LookupEntry) Write(w io.Writer) error {
	if err := e.ResHdr.Write(w); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, e.PartNumber); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, e.RefCount); err != nil {
		return err
	}
	_, err := w.Write(e.SHA1[:])
	return err
}

// Read deserializes the 50-byte LookupEntry from r.
func (e *LookupEntry) Read(r io.Reader) error {
	if err := e.ResHdr.Read(r); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &e.PartNumber); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &e.RefCount); err != nil {
		return err
	}
	_, err := io.ReadFull(r, e.SHA1[:])
	return err
}

// TimeToFiletime converts a time.Time into Windows FILETIME (100ns units since Jan 1, 1601 UTC).
func TimeToFiletime(t time.Time) uint64 {
	if t.IsZero() {
		return 0
	}
	const epochDiff = 11644473600 // seconds between 1601-01-01 and 1970-01-01
	sec := t.Unix() + epochDiff
	nsec := t.Nanosecond()
	return uint64(sec)*10000000 + uint64(nsec)/100
}

// FiletimeToTime converts Windows FILETIME into time.Time.
func FiletimeToTime(ft uint64) time.Time {
	if ft == 0 {
		return time.Time{}
	}
	const epochDiff = 11644473600
	sec := int64(ft/10000000) - epochDiff
	nsec := int64((ft % 10000000) * 100)
	return time.Unix(sec, nsec).UTC()
}

// EncodeUTF16LE converts a Go string into a Little-Endian UTF-16 byte slice.
func EncodeUTF16LE(s string) []byte {
	runes := utf16.Encode([]rune(s))
	buf := make([]byte, len(runes)*2)
	for i, r := range runes {
		binary.LittleEndian.PutUint16(buf[i*2:], r)
	}
	return buf
}

// DecodeUTF16LE converts a Little-Endian UTF-16 byte slice into a Go string.
func DecodeUTF16LE(b []byte) string {
	u16 := make([]uint16, len(b)/2)
	for i := 0; i < len(u16); i++ {
		u16[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(u16))
}
