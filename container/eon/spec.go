// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eon

import (
	"encoding/binary"
	"errors"
	"io"
)

// Magic constants and binary signatures.
var (
	MagicHeader   = [4]byte{'E', 'O', 'N', '!'} // 0x214E4F45 in Little-Endian uint32
	MagicTrailer  = [4]byte{'!', 'E', 'O', 'N'} // 0x4E4F4521 in Little-Endian uint32
	MagicChunk    = [4]byte{'C', 'H', 'N', 'K'} // 0x4B4E4843 in Little-Endian uint32
	MagicCTOC     = [4]byte{'C', 'T', 'O', 'C'} // 0x434F5443 in Little-Endian uint32
	MagicINDX     = [4]byte{'I', 'N', 'D', 'X'} // 0x58444E49 in Little-Endian uint32
	MagicRecovery = [4]byte{'R', 'E', 'C', 'V'} // 0x56434552 in Little-Endian uint32
)

// Magic integer constants in little-endian representation.
const (
	MagicHeaderLE   uint32 = 0x214E4F45 // 'EON!'
	MagicTrailerLE  uint32 = 0x4E4F4521 // '!EON'
	MagicChunkLE    uint32 = 0x4B4E4843 // 'CHNK'
	MagicCTOCLE     uint32 = 0x434F5443 // 'CTOC'
	MagicINDXLE     uint32 = 0x58444E49 // 'INDX'
	MagicRecoveryLE uint32 = 0x56434552 // 'RECV'
)

// Standard dimensions and fixed structure sizes in bytes.
const (
	HeaderSize             = 32
	TrailerSize            = 32
	ChunkHeaderSize        = 64
	CTOCHeaderSize         = 16
	CTOCEntrySize          = 16
	INDXHeaderSize         = 8
	ExtentRecordPrefixSize = 56

	DefaultBlockSizeShift  = 12               // 4096 bytes (4 KiB)
	DefaultChunkTargetSize = 32 * 1024 * 1024 // 32 MiB uncompressed
	MicroFrameSize         = 64 * 1024        // 64 KiB
)

// Format version.
const (
	VersionMajor = 1
	VersionMinor = 0
)

// Substrate profiles.
const (
	ProfileVFS    uint8 = 0x00
	ProfileBlock  uint8 = 0x01
	ProfileTensor uint8 = 0x02
	ProfileStream uint8 = 0x03
	ProfileCustom uint8 = 0xFF
)

// Global archive flags.
const (
	FlagCentralDir    uint16 = 0x0001
	FlagRecovery      uint16 = 0x0002
	FlagDigitalSig    uint16 = 0x0004
	FlagEncrypted     uint16 = 0x0008
	FlagStreamPipe    uint16 = 0x0010
	FlagAlignedBlocks uint16 = 0x0020
	FlagTieredIngest  uint16 = 0x0040
	FlagSelfMounting  uint16 = 0x0080
)

// Chunk flags.
const (
	ChunkFlagSolid       uint16 = 0x0001
	ChunkFlagEncrypted   uint16 = 0x0002
	ChunkFlagMicroFrames uint16 = 0x0004
)

// Codec identifiers.
const (
	CodecStore  uint8 = 0x00
	CodecZSTD   uint8 = 0x01
	CodecOpenZL uint8 = 0x02
	CodecLZMA2  uint8 = 0x03
	CodecBrotli uint8 = 0x04
	CodecLZ4    uint8 = 0x05
	CodecPRZM   uint8 = 0x06
)

// Namespace registry allocations.
const (
	NamespaceVFS    uint16 = 0x0000
	NamespaceBlock  uint16 = 0x0001
	NamespaceTensor uint16 = 0x0002
	NamespaceKV     uint16 = 0x0003
)

// Standard TLV tags.
const (
	TagPOSIXPermissions uint16 = 0x5058 // 'PX'
	TagDigitalSignature uint16 = 0x5349 // 'SI'
	TagProvenance       uint16 = 0x5056 // 'PV'
	TagEntrypoint       uint16 = 0x4550 // 'EP'
	TagEnvironment      uint16 = 0x4556 // 'EV'
	TagComment          uint16 = 0x434D // 'CM'
	TagWindowsInfo      uint16 = 0x4E54 // 'NT'
	TagExtendedAttrs    uint16 = 0x5841 // 'XA'
	TagIngestPriority   uint16 = 0x5052 // 'PR'
	TagTensorInfo       uint16 = 0x5453 // 'TS'
)

// Sentinel errors.
var (
	ErrInvalidHeader  = errors.New("eon: invalid archive header signature")
	ErrInvalidTrailer = errors.New("eon: invalid archive trailer signature")
	ErrInvalidChunk   = errors.New("eon: invalid chunk header signature")
	ErrInvalidCTOC    = errors.New("eon: invalid chunk table of contents signature")
	ErrInvalidINDX    = errors.New("eon: invalid extent catalog signature")
	ErrCorruptData    = errors.New("eon: checksum or hash mismatch")
)

// AppendULEB128 encodes an unsigned integer into ULEB128 bytes.
func AppendULEB128(dst []byte, val uint64) []byte {
	for {
		b := byte(val & 0x7F)
		val >>= 7
		if val != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if val == 0 {
			break
		}
	}
	return dst
}

// ReadULEB128 reads an unsigned integer encoded in ULEB128 format from r.
func ReadULEB128(r io.ByteReader) (uint64, error) {
	return binary.ReadUvarint(r)
}
