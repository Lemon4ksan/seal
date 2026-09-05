// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package spec defines binary constants, data structures, and serialization primitives
// for the EON Container Format v1.0.0.
package spec

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Binary signatures.
var (
	MagicHeader   = [4]byte{'E', 'O', 'N', '!'} // 0x214E4F45 LE uint32
	MagicTrailer  = [4]byte{'!', 'E', 'O', 'N'} // 0x4E4F4521 LE uint32
	MagicChunk    = [4]byte{'C', 'H', 'N', 'K'} // 0x4B4E4843 LE uint32
	MagicCTOC     = [4]byte{'C', 'T', 'O', 'C'} // 0x434F5443 LE uint32
	MagicINDX     = [4]byte{'I', 'N', 'D', 'X'} // 0x58444E49 LE uint32
	MagicRecovery = [4]byte{'R', 'E', 'C', 'V'} // 0x56434552 LE uint32
	MagicDSIG     = [4]byte{'D', 'S', 'I', 'G'} // 0x47495344 LE uint32
	MagicENCR     = [4]byte{'E', 'N', 'C', 'R'} // 0x52434E45 LE uint32
	MagicTREE     = [4]byte{'T', 'R', 'E', 'E'} // 0x45455254 LE uint32
	MagicSNPS     = [4]byte{'S', 'N', 'P', 'S'} // 0x53504E53 LE uint32
	MagicCHSH     = [4]byte{'C', 'H', 'S', 'H'} // 0x48534843 LE uint32
	MagicPRZM     = [4]byte{'P', 'R', 'Z', 'M'} // 0x4D5A5250 LE uint32
)

// Little-endian integer representations of signatures.
const (
	MagicHeaderLE   uint32 = 0x214E4F45 // 'EON!'
	MagicTrailerLE  uint32 = 0x4E4F4521 // '!EON'
	MagicChunkLE    uint32 = 0x4B4E4843 // 'CHNK'
	MagicCTOCLE     uint32 = 0x434F5443 // 'CTOC'
	MagicINDXLE     uint32 = 0x58444E49 // 'INDX'
	MagicRecoveryLE uint32 = 0x56434552 // 'RECV'
	MagicDSIGLE     uint32 = 0x47495344 // 'DSIG'
	MagicENCRLE     uint32 = 0x52434E45 // 'ENCR'
	MagicTREELE     uint32 = 0x45455254 // 'TREE'
	MagicSNPSLE     uint32 = 0x53504E53 // 'SNPS'
	MagicCHSHLE     uint32 = 0x48534843 // 'CHSH'
	MagicPRZMLE     uint32 = 0x4D5A5250 // 'PRZM'
)

// Structural byte lengths.
const (
	HeaderSize             = 32
	TrailerSize            = 32
	ChunkHeaderSize        = 64
	CTOCHeaderSize         = 16
	CTOCEntrySize          = 16
	INDXHeaderSize         = 8
	FixedExtentPrefixSize  = 58
	DSIGHeaderSize         = 14
	ENCRHeaderSize         = 24
	KeySlotSize            = 144
	MaxKeySlots            = 16

	DefaultBlockSizeShift  = 12 // 4096 bytes (4 KiB)
	DefaultChunkTargetSize = 32 * 1024 * 1024 // 32 MiB uncompressed
	MicroFrameSize         = 64 * 1024        // 64 KiB
)

// Symmetric cipher identifiers.
const (
	CipherAES256GCM        uint8 = 0x01
	CipherChaCha20Poly1305 uint8 = 0x02
)

// Key slot types.
const (
	SlotTypeArgon2id     uint8 = 0x01
	SlotTypePBKDF2SHA256 uint8 = 0x02
	SlotTypeX25519       uint8 = 0x03
	SlotTypeKMS          uint8 = 0x04
	SlotTypeVault        uint8 = 0x05
)

// Vault descriptor flags.
const (
	VaultFlagHidden       uint8 = 0x01
	VaultFlagSharedChunks uint8 = 0x02
)

// Vault descriptor byte lengths.
const (
	VaultPlaintextSize = 80 // 32B DEK + 8B DirOffset + 8B DirSize + 1B VaultID + 1B VaultFlags + 22B VaultName + 8B Reserved
	VaultWrappedSize   = 96 // 80B plaintext + 16B AEAD tag
)

// Default KDF tuning parameters.
const (
	DefaultArgon2Memory      uint32 = 64 * 1024 // 64 MiB
	DefaultArgon2Iterations  uint32 = 3
	DefaultArgon2Parallelism uint8  = 4
	DefaultPBKDF2Iterations  uint32 = 600000
)

// Signature algorithm identifiers.
const (
	SigAlgoEd25519 uint16 = 0x0001
	SigAlgoECDSA   uint16 = 0x0002
	SigAlgoRSA     uint16 = 0x0003
	SigAlgoMLDSA65 uint16 = 0x0004
)

// Specification version.
const (
	VersionMajor uint8 = 0
	VersionMinor uint8 = 1
)

// Substrate profiles.
const (
	ProfileVFS        uint8 = 0x00
	ProfileBlock      uint8 = 0x01
	ProfileTensor     uint8 = 0x02
	ProfileStream     uint8 = 0x03
	ProfileFastMount  uint8 = 0x10
	ProfileArchiveMax uint8 = 0x20
	ProfileCustom     uint8 = 0xFF
)

// Global archive flags.
const (
	FlagCentralDir     uint16 = 0x0001
	FlagRecovery       uint16 = 0x0002
	FlagDigitalSig     uint16 = 0x0004
	FlagEncrypted      uint16 = 0x0008
	FlagStreamPipe     uint16 = 0x0010
	FlagAlignedBlocks  uint16 = 0x0020
	FlagTieredIngest   uint16 = 0x0040
	FlagSelfMounting   uint16 = 0x0080
	FlagCDC            uint16 = 0x0100
	FlagProllyTree     uint16 = 0x0200
	FlagDetachedChunks uint16 = 0x0400 // Chunks stored as separate objects (S3/R2/GCS layout)
)

// ChunkObjectKey formats the content-addressed object storage key for a chunk hash.
func ChunkObjectKey(prefix string, hash [32]byte) string {
	hexHash := hex.EncodeToString(hash[:])
	key := fmt.Sprintf("chunks/%s/%s/%s", hexHash[:2], hexHash[2:4], hexHash)
	if prefix == "" {
		return key
	}
	return strings.TrimSuffix(prefix, "/") + "/" + key
}

// Extent flags.
const (
	ExtentFlagChunkList uint16 = 0x0002 // Non-contiguous discrete chunk list stored in TagChunkList TLV
)

// Chunk flags.
const (
	ChunkFlagSolid       uint16 = 0x0001
	ChunkFlagEncrypted   uint16 = 0x0002
	ChunkFlagMicroFrames uint16 = 0x0004
	ChunkFlagByteShuffle uint16 = 0x0008 // Byte-shuffle filter applied before compression
	ChunkFlagPipeline    uint16 = 0x0080 // Chunk payload begins with a PipelineDescriptor (filter chain)
)

// Pipeline magic constants.
var (
	MagicPipeline = [4]byte{'P', 'I', 'P', 'E'} // 0x45504950 LE uint32
)

const (
	MagicPipelineLE uint32 = 0x45504950 // 'PIPE'
)

// Standard chunk filter identifiers.
const (
	FilterIDShuffle  uint8 = 0x01
	FilterIDBCJX86   uint8 = 0x02
	FilterIDBCJARM64 uint8 = 0x03
	FilterIDDelta    uint8 = 0x04
	FilterIDCustom   uint8 = 0x80 // Filters >= 0x80 are reserved for third-party custom filters
)

// Compression codec identifiers.
const (
	CodecStore  uint8 = 0x00
	CodecZSTD   uint8 = 0x01
	CodecOpenZL uint8 = 0x02
	CodecLZMA2  uint8 = 0x03
	CodecBrotli uint8 = 0x04
	CodecLZ4    uint8 = 0x05
	CodecPRZM   uint8 = 0x06
)

// Universal namespace registry allocations.
const (
	NamespaceVFS    uint16 = 0x0000
	NamespaceBlock  uint16 = 0x0001
	NamespaceTensor uint16 = 0x0002
	NamespaceKV     uint16 = 0x0003
	NamespaceCustom uint16 = 0x8000
)

// Standard TLV tags.
const (
	TagPOSIXPermissions uint16 = 0x5058 // 'PX'
	TagDigitalSignature  uint16 = 0x5349 // 'SI'
	TagProvenance        uint16 = 0x5056 // 'PV'
	TagEntrypoint        uint16 = 0x4550 // 'EP'
	TagEnvironment       uint16 = 0x4556 // 'EV'
	TagComment           uint16 = 0x434D // 'CM'
	TagWindowsInfo       uint16 = 0x4E54 // 'NT'
	TagExtendedAttrs     uint16 = 0x5841 // 'XA'
	TagIngestPriority    uint16 = 0x5052 // 'PR'
	TagTensorInfo        uint16 = 0x5453 // 'TS'
	TagChunkList         uint16 = 0x434C // 'CL'
	TagChunkRuns         uint16 = 0x4352 // 'CR'
	TagSnapshotID        uint16 = 0x5353 // 'SS'
	TagParentSnapshot    uint16 = 0x5053 // 'PS'
)
