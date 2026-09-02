// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package zip provides a high-performance, specification-complete ZIP archive engine.
package zip

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/lemon4ksan/foundation/binkit"

	"github.com/lemon4ksan/seal/internal/sys"
)

// LocalFileHeader represents the binary header preceding individual entry data streams.
type LocalFileHeader struct {
	VersionNeededToExtract uint16
	GeneralPurposeBitFlag  uint16
	CompressionMethod      uint16
	LastModFileTime        uint16
	LastModFileDate        uint16
	CRC32                  uint32
	CompressedSize         uint32
	UncompressedSize       uint32
	FilenameLength         uint16
	ExtraFieldLength       uint16
	Filename               string
	ExtraField             []byte
}

// ReadLocalFileHeader parses a binary Local File Header record from src.
func ReadLocalFileHeader(src io.Reader) (LocalFileHeader, error) {
	var buf [26]byte
	if _, err := io.ReadFull(src, buf[:]); err != nil {
		return LocalFileHeader{}, fmt.Errorf("read local file header: %w", err)
	}

	var entry LocalFileHeader
	if err := binkit.UnmarshalLE(buf[:], &entry); err != nil {
		return LocalFileHeader{}, fmt.Errorf("unmarshal local file header: %w", err)
	}

	if entry.FilenameLength > 0 {
		fn := make([]byte, entry.FilenameLength)
		if _, err := io.ReadFull(src, fn); err != nil {
			return LocalFileHeader{}, fmt.Errorf("read local filename: %w", err)
		}
		entry.Filename = string(fn)
	}

	if entry.ExtraFieldLength > 0 {
		entry.ExtraField = make([]byte, entry.ExtraFieldLength)
		if _, err := io.ReadFull(src, entry.ExtraField); err != nil {
			return LocalFileHeader{}, fmt.Errorf("read local extra field: %w", err)
		}
	}

	return entry, nil
}

// AppendBytes serializes the LocalFileHeader record into buf using binkit.Writer.
func (h LocalFileHeader) AppendBytes(buf []byte) []byte {
	needed := 30 + int(h.FilenameLength) + int(h.ExtraFieldLength)
	w := binkit.NewWriter(buf, needed)
	w.U32LE(LocalFileHeaderSignature)
	buf, _ = binkit.MarshalLE(&h, w.Bytes())
	w = binkit.NewWriter(buf)
	w.RawString(h.Filename).Raw(h.ExtraField)
	return w.Bytes()
}

// CentralDirectory represents a single entry in the archive's Central Directory index.
type CentralDirectory struct {
	VersionMadeBy          uint16
	VersionNeededToExtract uint16
	GeneralPurposeBitFlag  uint16
	CompressionMethod      uint16
	LastModFileTime        uint16
	LastModFileDate        uint16
	CRC32                  uint32
	CompressedSize         uint32
	UncompressedSize       uint32
	FilenameLength         uint16
	ExtraFieldLength       uint16
	FileCommentLength      uint16
	DiskNumberStart        uint16
	InternalFileAttributes uint16
	ExternalFileAttributes uint32
	LocalHeaderOffset      uint32
	Filename               string
	ExtraField             []byte
	Comment                string
}

// ReadCentralDirEntry parses a Central Directory Entry header from src.
func ReadCentralDirEntry(src io.Reader) (CentralDirectory, error) {
	var buf [42]byte
	if _, err := io.ReadFull(src, buf[:]); err != nil {
		return CentralDirectory{}, fmt.Errorf("read central directory entry: %w", err)
	}

	var entry CentralDirectory
	if err := binkit.UnmarshalLE(buf[:], &entry); err != nil {
		return CentralDirectory{}, fmt.Errorf("unmarshal central directory: %w", err)
	}

	if entry.FilenameLength > 0 {
		fn := make([]byte, entry.FilenameLength)
		if _, err := io.ReadFull(src, fn); err != nil {
			return CentralDirectory{}, fmt.Errorf("read cd filename: %w", err)
		}
		entry.Filename = string(fn)
	}

	if entry.ExtraFieldLength > 0 {
		entry.ExtraField = make([]byte, entry.ExtraFieldLength)
		if _, err := io.ReadFull(src, entry.ExtraField); err != nil {
			return CentralDirectory{}, fmt.Errorf("read cd extra field: %w", err)
		}
	}

	if entry.FileCommentLength > 0 {
		cm := make([]byte, entry.FileCommentLength)
		if _, err := io.ReadFull(src, cm); err != nil {
			return CentralDirectory{}, fmt.Errorf("read cd comment: %w", err)
		}
		entry.Comment = string(cm)
	}

	return entry, nil
}

// AppendBytes serializes the CentralDirectory record into buf using binkit.Writer.
func (d CentralDirectory) AppendBytes(buf []byte) []byte {
	needed := 46 + int(d.FilenameLength) + int(d.ExtraFieldLength) + int(d.FileCommentLength)
	w := binkit.NewWriter(buf, needed)
	w.U32LE(CentralDirectorySignature)
	buf, _ = binkit.MarshalLE(&d, w.Bytes())
	w = binkit.NewWriter(buf)
	w.RawString(d.Filename).Raw(d.ExtraField).RawString(d.Comment)
	return w.Bytes()
}

// EOCD represents the End of Central Directory record terminating a standard ZIP archive.
type EOCD struct {
	ThisDiskNum                     uint16
	DiskNumWithTheStartOfCentralDir uint16
	TotalNumberOfEntriesOnThisDisk  uint16
	EntriesNum                      uint16
	CentralDirSize                  uint32
	CentralDirOffset                uint32
	CommentLength                   uint16
	Comment                         string
}

// ReadEOCD parses an EOCD record from src.
func ReadEOCD(src io.Reader) (EOCD, error) {
	var buf [18]byte
	if _, err := io.ReadFull(src, buf[:]); err != nil {
		return EOCD{}, fmt.Errorf("read EOCD: %w", err)
	}

	var end EOCD
	if err := binkit.UnmarshalLE(buf[:], &end); err != nil {
		return EOCD{}, fmt.Errorf("unmarshal EOCD: %w", err)
	}

	if end.CommentLength > 0 {
		cm := make([]byte, end.CommentLength)
		if _, err := io.ReadFull(src, cm); err != nil {
			return EOCD{}, fmt.Errorf("read EOCD comment: %w", err)
		}
		end.Comment = string(cm)
	}

	return end, nil
}

// EncodeEOCD serializes the End of Central Directory record.
func EncodeEOCD(entriesNum int, centralDirSize, centralDirOffset int64, comment string) []byte {
	commentLen := min(len(comment), int(MaxUint16))
	eocd := EOCD{
		ThisDiskNum:                     0,
		DiskNumWithTheStartOfCentralDir: 0,
		TotalNumberOfEntriesOnThisDisk:  uint16(min(uint64(entriesNum), MaxUint16)),
		EntriesNum:                      uint16(min(uint64(entriesNum), MaxUint16)),
		CentralDirSize:                  uint32(min(uint64(centralDirSize), MaxUint32)),
		CentralDirOffset:                uint32(min(uint64(centralDirOffset), MaxUint32)),
		CommentLength:                   uint16(commentLen),
	}

	w := binkit.NewWriter(nil, 22+commentLen)
	w.U32LE(EOCDSignature)
	buf, _ := binkit.MarshalLE(&eocd, w.Bytes())
	w = binkit.NewWriter(buf)
	w.RawString(comment[:commentLen])
	return w.Bytes()
}

// Zip64EOCD represents the 64-bit End of Central Directory record.
type Zip64EOCD struct {
	Size                            uint64
	VersionMadeBy                   uint16
	VersionNeededToExtract          uint16
	ThisDiskNum                     uint32
	DiskNumWithTheStartOfCentralDir uint32
	TotalNumberOfEntriesOnThisDisk  uint64
	EntriesNum                      uint64
	CentralDirSize                  uint64
	CentralDirOffset                uint64
}

// ReadZip64EOCD parses a Zip64 End of Central Directory record.
func ReadZip64EOCD(src io.Reader) (Zip64EOCD, error) {
	var buf [52]byte
	if _, err := io.ReadFull(src, buf[:]); err != nil {
		return Zip64EOCD{}, fmt.Errorf("read Zip64 EOCD: %w", err)
	}

	var entry Zip64EOCD
	if err := binkit.UnmarshalLE(buf[:], &entry); err != nil {
		return Zip64EOCD{}, fmt.Errorf("unmarshal Zip64 EOCD: %w", err)
	}
	return entry, nil
}

// EncodeZip64EOCDRecord serializes the Zip64 End of Central Directory record.
func EncodeZip64EOCDRecord(entriesNum int, centralDirSize, centralDirOffset int64) []byte {
	rec := Zip64EOCD{
		Size:                            44,
		VersionMadeBy:                   45,
		VersionNeededToExtract:          45,
		ThisDiskNum:                     0,
		DiskNumWithTheStartOfCentralDir: 0,
		TotalNumberOfEntriesOnThisDisk:  uint64(entriesNum),
		EntriesNum:                      uint64(entriesNum),
		CentralDirSize:                  uint64(centralDirSize),
		CentralDirOffset:                uint64(centralDirOffset),
	}

	w := binkit.NewWriter(nil, 56)
	w.U32LE(Zip64EOCDSignature)
	buf, _ := binkit.MarshalLE(&rec, w.Bytes())
	return buf
}

// Zip64EOCDLocator locates the Zip64 End of Central Directory record.
type Zip64EOCDLocator struct {
	EndOfCentralDirStartDiskNum uint32
	Zip64EndOfCentralDirOffset  uint64
	TotalNumberOfDisks          uint32
}

// ReadZip64EOCDLocator parses the Zip64 EOCD Locator record.
func ReadZip64EOCDLocator(src io.Reader) (Zip64EOCDLocator, error) {
	var buf [16]byte
	if _, err := io.ReadFull(src, buf[:]); err != nil {
		return Zip64EOCDLocator{}, fmt.Errorf("read Zip64 EOCD locator: %w", err)
	}

	var entry Zip64EOCDLocator
	if err := binkit.UnmarshalLE(buf[:], &entry); err != nil {
		return Zip64EOCDLocator{}, fmt.Errorf("unmarshal Zip64 EOCD locator: %w", err)
	}
	return entry, nil
}

// EncodeZip64EOCDLocator serializes the Zip64 EOCD locator.
func EncodeZip64EOCDLocator(eocdOffset int64) []byte {
	loc := Zip64EOCDLocator{
		EndOfCentralDirStartDiskNum: 0,
		Zip64EndOfCentralDirOffset:  uint64(eocdOffset),
		TotalNumberOfDisks:          1,
	}

	w := binkit.NewWriter(nil, 20)
	w.U32LE(Zip64EOCDLocatorSignature)
	buf, _ := binkit.MarshalLE(&loc, w.Bytes())
	return buf
}

// ParseExtraFields decomposes raw extra field bytes into a map keyed by 16-bit header tags.
func ParseExtraFields(extra []byte) map[uint16][]byte {
	m := make(map[uint16][]byte)
	r := binkit.NewReader(extra)
	for r.Remaining() >= 4 {
		tag := r.U16LE()
		size := int(r.U16LE())
		if size > r.Remaining() {
			break
		}
		data := r.Bytes(size)
		m[tag] = extra[r.Pos()-size-4 : r.Pos()]
		_ = data
	}
	return m
}

// EncodeZip64ExtraField encodes variable-sized 64-bit offsets and sizes for Central Directory.
func EncodeZip64ExtraField(buf []byte, uncompSize, compSize, headerOffset int64) []byte {
	w := binkit.NewWriter(buf, 28)
	w.U16LE(Zip64ExtraFieldTag)

	// Reserve 2 bytes for payload size (updated below)
	w.U16LE(0)
	startPayload := w.Len()

	if uint64(uncompSize) > MaxUint32 {
		w.U64LE(uint64(uncompSize))
	}
	if uint64(compSize) > MaxUint32 {
		w.U64LE(uint64(compSize))
	}
	if uint64(headerOffset) > MaxUint32 {
		w.U64LE(uint64(headerOffset))
	}

	payloadSize := uint16(w.Len() - startPayload)
	res := w.Bytes()
	res[2] = byte(payloadSize)
	res[3] = byte(payloadSize >> 8)
	return res
}

// EncodeZip64LocalExtraField encodes fixed-size 64-bit sizes for Local File Headers.
func EncodeZip64LocalExtraField(uncompSize, compSize int64) []byte {
	w := binkit.NewWriter(nil, 20)
	w.U16LE(Zip64ExtraFieldTag).
		U16LE(16). // Size of payload
		U64LE(uint64(uncompSize)).
		U64LE(uint64(compSize))
	return w.Bytes()
}

// EncodeDataDescriptor formats a 16-byte standard or 24-byte ZIP64 Data Descriptor.
func EncodeDataDescriptor(crc uint32, compSize, uncompSize int64) []byte {
	if uint64(compSize) > MaxUint32 || uint64(uncompSize) > MaxUint32 {
		w := binkit.NewWriter(nil, 24)
		w.U32LE(DataDescriptorSignature).
			U32LE(crc).
			U64LE(uint64(compSize)).
			U64LE(uint64(uncompSize))
		return w.Bytes()
	}

	w := binkit.NewWriter(nil, 16)
	w.U32LE(DataDescriptorSignature).
		U32LE(crc).
		U32LE(uint32(compSize)).
		U32LE(uint32(uncompSize))
	return w.Bytes()
}

// DataDescriptor holds CRC-32 and compressed/uncompressed sizes parsed from a Data Descriptor record.
type DataDescriptor struct {
	CRC32            uint32
	CompressedSize   int64
	UncompressedSize int64
}

// ReadDataDescriptor parses a Data Descriptor record from src.
// It automatically detects whether the optional 4-byte signature (0x08074b50) is present or omitted,
// and supports both standard 32-bit and 64-bit Zip64 descriptor layouts.
func ReadDataDescriptor(src io.Reader, isZip64 bool) (DataDescriptor, error) {
	var first4 [4]byte
	if _, err := io.ReadFull(src, first4[:]); err != nil {
		return DataDescriptor{}, fmt.Errorf("read data descriptor header: %w", err)
	}

	val := binary.LittleEndian.Uint32(first4[:])
	var crc uint32

	if val == DataDescriptorSignature {
		// Signature is present; read the actual CRC-32
		var crcBuf [4]byte
		if _, err := io.ReadFull(src, crcBuf[:]); err != nil {
			return DataDescriptor{}, fmt.Errorf("read data descriptor crc: %w", err)
		}
		crc = binary.LittleEndian.Uint32(crcBuf[:])
	} else {
		// Signature was omitted; first 4 bytes ARE the CRC-32
		crc = val
	}

	if isZip64 {
		var sizes [16]byte
		if _, err := io.ReadFull(src, sizes[:]); err != nil {
			return DataDescriptor{}, fmt.Errorf("read zip64 data descriptor sizes: %w", err)
		}
		comp := int64(binary.LittleEndian.Uint64(sizes[0:8]))
		uncomp := int64(binary.LittleEndian.Uint64(sizes[8:16]))
		return DataDescriptor{CRC32: crc, CompressedSize: comp, UncompressedSize: uncomp}, nil
	}

	var sizes [8]byte
	if _, err := io.ReadFull(src, sizes[:]); err != nil {
		return DataDescriptor{}, fmt.Errorf("read data descriptor sizes: %w", err)
	}
	comp := int64(binary.LittleEndian.Uint32(sizes[0:4]))
	uncomp := int64(binary.LittleEndian.Uint32(sizes[4:8]))
	return DataDescriptor{CRC32: crc, CompressedSize: comp, UncompressedSize: uncomp}, nil
}

// EncodeAESExtraField encodes WinZip AES extra field (tag 0x9901).
// strength: 1 for AES-128, 2 for AES-192, 3 for AES-256 (default).
func EncodeAESExtraField(compMethod uint16, strength ...byte) []byte {
	st := byte(0x03) // Default AES-256
	if len(strength) > 0 && strength[0] != 0 {
		st = strength[0]
	}
	w := binkit.NewWriter(nil, 11)
	w.U16LE(AESEncryptionTag).
		U16LE(7).
		U16LE(0x0002). // Vendor Version AE-2
		U8('A').
		U8('E').
		U8(st).
		U16LE(compMethod)
	return w.Bytes()
}

// ParseFileMode derives the [fs.FileMode] from a Central Directory entry.
func ParseFileMode(entry CentralDirectory) fs.FileMode {
	hostSystem := sys.HostSystem(entry.VersionMadeBy >> 8)

	if hostSystem.IsUnix() {
		unixMode := entry.ExternalFileAttributes >> 16
		mode := fs.FileMode(unixMode & 0o777)

		switch unixMode & sys.S_IFMT {
		case sys.S_IFDIR:
			mode |= fs.ModeDir
		case sys.S_IFLNK:
			mode |= fs.ModeSymlink
		case sys.S_IFSOCK:
			mode |= fs.ModeSocket
		case sys.S_IFIFO:
			mode |= fs.ModeNamedPipe
		case sys.S_IFCHR:
			mode |= fs.ModeCharDevice
		case sys.S_IFBLK:
			mode |= fs.ModeDevice
		}
		return mode
	}

	if hostSystem.IsWindows() {
		isDir := strings.HasSuffix(entry.Filename, "/") || (entry.ExternalFileAttributes&0x10 != 0)
		var mode fs.FileMode = 0o644
		if isDir {
			mode = 0o755 | fs.ModeDir
		}
		if entry.ExternalFileAttributes&0x01 != 0 {
			mode &^= 0o222 // Read-only
		}
		return mode
	}

	if strings.HasSuffix(entry.Filename, "/") {
		return 0o755 | fs.ModeDir
	}
	return 0o644
}
