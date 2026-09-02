// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"errors"

	"github.com/lemon4ksan/seal/codec"
)

// PKWARE APPNOTE.TXT - .ZIP File Format Specification Version 6.3.10
// Revised: Nov 01, 2022

// APPNOTE Sentinel Errors enforcing normative specification rules.
var (
	// ErrEmptyArchive is returned when attempting to write an empty ZIP archive (APPNOTE 4.1.2).
	ErrEmptyArchive = errors.New("zip: should contain at least one file")
	// ErrInvalidDataDescriptor is returned when Data Descriptor bit configuration violates APPNOTE 4.4.4.
	ErrInvalidDataDescriptor = errors.New("zip: invalid data descriptor configuration")
	// ErrInconsistentHeader is returned when Local File Header and Central Directory metadata conflict (APPNOTE 4.4.3).
	ErrInconsistentHeader = errors.New("zip: local file header and central directory mismatch")
	// ErrFileNameTooLong is returned when a file name exceeds 65535 bytes (APPNOTE 4.4.1.2).
	ErrFileNameTooLong = errors.New("zip: file name length exceeds max uint16 limit")
	// ErrExtraFieldTooLong is returned when an extra field exceeds 65535 bytes (APPNOTE 4.4.1.3).
	ErrExtraFieldTooLong = errors.New("zip: extra field length exceeds max uint16 limit")
	// ErrCommentTooLong is returned when an archive/file comment exceeds 65535 bytes (APPNOTE 4.4.14).
	ErrCommentTooLong = errors.New("zip: comment length exceeds max uint16 limit")
)

// PKWARE ZIP 32-bit Header and Record Signatures (APPNOTE 4.2 & 4.3).
const (
	LocalFileHeaderSignature  uint32 = 0x04034b50 // PK\x03\x04 - Local File Header
	CentralDirectorySignature uint32 = 0x02014b50 // PK\x01\x02 - Central Directory File Header
	EOCDSignature             uint32 = 0x06054b50 // PK\x05\x06 - End of Central Directory (EOCD)
	Zip64EOCDSignature        uint32 = 0x06064b50 // PK\x06\x06 - Zip64 End of Central Directory
	Zip64EOCDLocatorSignature uint32 = 0x07064b50 // PK\x06\x07 - Zip64 End of Central Directory Locator
	DataDescriptorSignature   uint32 = 0x08074b50 // PK\x07\x08 - Data Descriptor Record
	ArchiveExtraDataSignature uint32 = 0x08064b50 // PK\x06\x08 - Archive Extra Data Record
	DigitalHeaderSignature    uint32 = 0x05054b50 // PK\x05\x05 - Digital Signature Record
	SpanningSignature         uint32 = 0x08074b50 // PK\x07\x08 - Spanning Header Marker
	SpanningTempSignature     uint32 = 0x30304b50 // PK00 - Temporary Spanning Marker (single segment)
)

// Host System Compatibility (Version Made By - upper byte) (APPNOTE 4.4.2.2).
const (
	HostFAT          byte = 0  // MS-DOS and OS/2 (FAT / VFAT / FAT32)
	HostAmiga        byte = 1  // Amiga
	HostOpenVMS      byte = 2  // OpenVMS
	HostUNIX         byte = 3  // UNIX
	HostVMCMS        byte = 4  // VM/CMS
	HostAtariST      byte = 5  // Atari ST
	HostOS2HPFS      byte = 6  // OS/2 H.P.F.S.
	HostMacintosh    byte = 7  // Macintosh
	HostZSystem      byte = 8  // Z-System
	HostCPM          byte = 9  // CP/M
	HostNTFS         byte = 10 // Windows NTFS
	HostMVS          byte = 11 // MVS (OS/390 - z/OS)
	HostVSE          byte = 12 // VSE
	HostAcornRisc    byte = 13 // Acorn Risc
	HostVFAT         byte = 14 // VFAT
	HostAlternateMVS byte = 15 // Alternate MVS
	HostBeOS         byte = 16 // BeOS
	HostTandem       byte = 17 // Tandem
	HostOS400        byte = 18 // OS/400
	HostOSX          byte = 19 // OS X (Darwin / macOS)
)

// Minimum Feature Specification Versions (Version Needed to Extract) (APPNOTE 4.4.3.2).
const (
	VersionDefault           uint16 = 10 // 1.0 - Default value
	VersionVolumeLabel       uint16 = 11 // 1.1 - File is a volume label
	VersionFolder            uint16 = 20 // 2.0 - File is a folder (directory)
	VersionDeflate           uint16 = 20 // 2.0 - File is compressed using Deflate compression
	VersionZipCrypto         uint16 = 20 // 2.0 - File is encrypted using traditional PKWARE encryption
	VersionDeflate64         uint16 = 21 // 2.1 - File is compressed using Deflate64(tm)
	VersionDCLImplode        uint16 = 25 // 2.5 - File is compressed using PKWARE DCL Implode
	VersionPatchData         uint16 = 27 // 2.7 - File is a patch data set
	VersionZip64             uint16 = 45 // 4.5 - File uses ZIP64 format extensions
	VersionBZIP2             uint16 = 46 // 4.6 - File is compressed using BZIP2 compression
	VersionDES               uint16 = 50 // 5.0 - File is encrypted using DES/3DES/original RC2/RC4
	VersionAES               uint16 = 51 // 5.1 - File is encrypted using AES / corrected RC2
	VersionRC2_64            uint16 = 52 // 5.2 - File is encrypted using corrected RC2-64
	VersionNonOAEP           uint16 = 61 // 6.1 - Certificate encryption using non-OAEP key wrapping
	VersionCentralDirEncrypt uint16 = 62 // 6.2 - Central directory encryption
	VersionLZMA              uint16 = 63 // 6.3 - File is compressed using LZMA
	VersionPPMd              uint16 = 63 // 6.3 - File is compressed using PPMd
	VersionBlowfish          uint16 = 63 // 6.3 - File is encrypted using Blowfish
	VersionTwofish           uint16 = 63 // 6.3 - File is encrypted using Twofish
)

// ComputeVersionNeeded returns the minimum specification version required to extract the entry (APPNOTE 4.4.1.1 & 4.4.3.2).
//
// Minimum Feature Versions:
//   - 1.0 (10): Default value / Store / Shrink / Reduce
//   - 2.0 (20): Deflate / Directory / ZipCrypto encryption
//   - 2.1 (21): Deflate64
//   - 2.5 (25): PKWARE DCL Implode
//   - 4.5 (45): Zip64 format extensions (size or offset > 2^32-1)
//   - 4.6 (46): BZIP2
//   - 5.0 (50): PKWARE Strong Encryption
//   - 5.1 (51): AES encryption / WinZip AES
//   - 6.3 (63): LZMA / PPMd / Zstandard / XZ / Brotli
func ComputeVersionNeeded(method codec.Method, isDir, isEncrypted, isZip64 bool) uint16 {
	if isZip64 {
		return VersionZip64 // 4.5
	}
	switch method {
	case codec.LZMA, codec.LZMA2, codec.PPMd, codec.ZStandard, codec.XZ, codec.Brotli:
		return VersionLZMA // 6.3
	case codec.BZIP2:
		return VersionBZIP2 // 4.6
	case codec.PKWAREDCL:
		return VersionDCLImplode // 2.5
	case codec.Deflate64:
		return VersionDeflate64 // 2.1
	case codec.Deflate, codec.IBMCmpsc:
		return VersionDeflate // 2.0
	default:
		if isDir || isEncrypted {
			return VersionFolder // 2.0
		}
		return VersionDefault // 1.0
	}
}

// General Purpose Bit Flags (APPNOTE 4.4.4).
const (
	FlagEncrypted            uint16 = 1 << 0  // Bit 0: Encrypted file
	FlagImplode8KDict        uint16 = 1 << 1  // Bit 1: (Method 6 Implode) 8K sliding dictionary
	FlagDeflateMax           uint16 = 1 << 1  // Bit 1: (Method 8/9 Deflate) Maximum compression
	FlagLZMA_EOS             uint16 = 1 << 1  // Bit 1: (Method 14 LZMA) End-of-stream marker present
	FlagImplode3Trees        uint16 = 1 << 2  // Bit 2: (Method 6 Implode) 3 Shannon-Fano trees
	FlagDeflateFast          uint16 = 1 << 2  // Bit 2: (Method 8/9 Deflate) Fast compression
	FlagDeflateSuperFast     uint16 = 3 << 1  // Bits 1+2: (Method 8/9 Deflate) Super Fast compression
	FlagDataDescriptor       uint16 = 1 << 3  // Bit 3: CRC-32 and sizes in Data Descriptor
	FlagEnhancedDeflate      uint16 = 1 << 4  // Bit 4: Reserved for enhanced deflating
	FlagPatchedData          uint16 = 1 << 5  // Bit 5: Compressed patched data (PKZIP >= 2.70)
	FlagStrongEncrypt        uint16 = 1 << 6  // Bit 6: Strong encryption
	FlagLanguageEncodingUTF8 uint16 = 1 << 11 // Bit 11: Language encoding flag (EFS) - UTF-8
	FlagEnhancedCompression  uint16 = 1 << 12 // Bit 12: Reserved by PKWARE for enhanced compression
	FlagMaskedLocalHeader    uint16 = 1 << 13 // Bit 13: Masked local header when Central Directory is encrypted
	FlagAlternateStreams     uint16 = 1 << 14 // Bit 14: Reserved by PKWARE for alternate streams
)

// Compression Methods (APPNOTE 4.4.5).
const (
	MethodStore      uint16 = 0  // 0: Stored (no compression)
	MethodShrunk     uint16 = 1  // 1: Shrunk (Dynamic LZW)
	MethodReduced1   uint16 = 2  // 2: Reduced with compression factor 1
	MethodReduced2   uint16 = 3  // 3: Reduced with compression factor 2
	MethodReduced3   uint16 = 4  // 4: Reduced with compression factor 3
	MethodReduced4   uint16 = 5  // 5: Reduced with compression factor 4
	MethodImploded   uint16 = 6  // 6: Imploded (Sliding dictionary + Shannon-Fano)
	MethodTokenizing uint16 = 7  // 7: Tokenizing (Reserved)
	MethodDeflate    uint16 = 8  // 8: Deflated
	MethodDeflate64  uint16 = 9  // 9: Enhanced Deflating using Deflate64(tm)
	MethodPKWAREDCL  uint16 = 10 // 10: PKWARE Data Compression Library Imploding
	MethodBZIP2      uint16 = 12 // 12: BZIP2
	MethodLZMA       uint16 = 14 // 14: LZMA
	MethodIBMCmpsc   uint16 = 16 // 16: IBM z/OS CMPSC Compression
	MethodIBMTERSE   uint16 = 18 // 18: IBM TERSE (new)
	MethodIBMLZ77    uint16 = 19 // 19: IBM LZ77 z Architecture
	MethodZstandard  uint16 = 93 // 93: Zstandard (zstd) Compression
	MethodMP3        uint16 = 94 // 94: MP3 Compression
	MethodXZ         uint16 = 95 // 95: XZ Compression
	MethodJPEG       uint16 = 96 // 96: JPEG variant
	MethodWavPack    uint16 = 97 // 97: WavPack compressed data
	MethodPPMd       uint16 = 98 // 98: PPMd version I, Rev 1
	MethodWinZipAES  uint16 = 99 // 99: WinZip AE-x encryption marker
)

// Extra Field Header IDs (APPNOTE 4.5.2 PKWARE Defined & 4.6.1 Third Party Mappings).
const (
	// PKWARE Defined Mappings (4.5.2)
	TagZip64                 uint16 = 0x0001 // Zip64 extended information extra field
	Zip64ExtraFieldTag       uint16 = 0x0001 // Backward compatible alias
	TagAVInfo                uint16 = 0x0007 // AV Info
	TagLanguageEncoding      uint16 = 0x0008 // Reserved for extended language encoding data (PFS)
	TagOS2                   uint16 = 0x0009 // OS/2 Extended Attributes
	TagNTFS                  uint16 = 0x000a // NTFS Attributes (Mtime, Atime, Ctime)
	NTFSFieldTag             uint16 = 0x000a // Backward compatible alias
	TagOpenVMS               uint16 = 0x000c // OpenVMS Attributes
	TagUNIX                  uint16 = 0x000d // UNIX Attributes (UID/GID, timestamps)
	TagFork                  uint16 = 0x000e // File stream and fork descriptors
	TagPatch                 uint16 = 0x000f // Patch Descriptor
	TagPKCS7Store            uint16 = 0x0014 // PKCS#7 Store for X.509 Certificates
	TagFileCert              uint16 = 0x0015 // X.509 Certificate ID and Signature for individual file
	TagCDCert                uint16 = 0x0016 // X.509 Certificate ID for Central Directory
	TagStrongEncryptHeader   uint16 = 0x0017 // Strong Encryption Header
	TagRecordManagement      uint16 = 0x0018 // Record Management Controls
	TagPKCS7RecipientList    uint16 = 0x0019 // PKCS#7 Encryption Recipient Certificate List
	TagTimestampRecord       uint16 = 0x0020 // Timestamp record
	TagPolicyDecryptionKey   uint16 = 0x0021 // Policy Decryption Key Record
	TagSmartcryptKeyProvider uint16 = 0x0022 // Smartcrypt Key Provider Record
	TagSmartcryptPolicyKey   uint16 = 0x0023 // Smartcrypt Policy Key Data Record
	TagIBMZ390               uint16 = 0x0065 // IBM S/390 (Z390), AS/400 (I400) attributes - uncompressed
	TagIBMZ390Compressed     uint16 = 0x0066 // IBM S/390 (Z390), AS/400 (I400) attributes - compressed
	TagPOSZIP                uint16 = 0x4690 // POSZIP 4690

	// Third Party Mappings (4.6.1)
	TagMacintosh             uint16 = 0x07c8 // Macintosh
	TagPixarUSD              uint16 = 0x1986 // Pixar USD header ID
	TagZipItMacLong          uint16 = 0x2605 // ZipIt Macintosh (long)
	TagZipItMacShortFile     uint16 = 0x2705 // ZipIt Macintosh (short, for files)
	TagZipItMacShortDir      uint16 = 0x2805 // ZipIt Macintosh (short, for directories)
	TagInfoZipMac            uint16 = 0x334d // Info-ZIP Macintosh
	TagTandem                uint16 = 0x4154 // Tandem
	TagAcornSparkFS          uint16 = 0x4341 // Acorn / SparkFS
	TagWindowsNTACL          uint16 = 0x4453 // Windows NT security descriptor (binary ACL)
	TagVMCMS                 uint16 = 0x4704 // VM/CMS
	TagMVS                   uint16 = 0x470f // MVS
	TagTHEOSOld              uint16 = 0x4854 // THEOS (old)
	TagFWKCSMD5              uint16 = 0x4b46 // FWKCS MD5 Signature
	TagOS2ACL                uint16 = 0x4c41 // OS/2 access control list (text ACL)
	TagInfoZipOpenVMS        uint16 = 0x4d49 // Info-ZIP OpenVMS
	TagMacSmartzip           uint16 = 0x4d63 // Macintosh Smartzip
	TagXceedLocation         uint16 = 0x4f4c // Xceed original location extra field
	TagAOSVSACL              uint16 = 0x5356 // AOS/VS (ACL)
	TagExtendedTimestamp     uint16 = 0x5455 // Info-ZIP extended timestamp ('UT')
	TagXceedUnicode          uint16 = 0x554e // Xceed unicode extra field
	TagInfoZipUNIXOld        uint16 = 0x5855 // Info-ZIP UNIX (original, OS/2, NT)
	TagInfoZipUnicodeComment uint16 = 0x6375 // Info-ZIP Unicode Comment Extra Field ('uc')
	TagBeOS                  uint16 = 0x6542 // BeOS / BeBox
	TagTHEOS                 uint16 = 0x6854 // THEOS
	TagInfoZipUnicodePath    uint16 = 0x7075 // Info-ZIP Unicode Path Extra Field ('up')
	TagAtheOS                uint16 = 0x7441 // AtheOS / Syllable
	TagASiUNIX               uint16 = 0x756e // ASi UNIX
	TagInfoZipUNIXNew        uint16 = 0x7855 // Info-ZIP UNIX (new)
	TagInfoZipUIDGID         uint16 = 0x7875 // Info-ZIP UNIX (newer UID/GID 'ux')
	TagDataStreamAlignment   uint16 = 0xa11e // Data Stream Alignment (Apache Commons-Compress)
	TagOpenPackagingGrowth   uint16 = 0xa220 // Microsoft Open Packaging Growth Hint
	TagJavaJAR               uint16 = 0xcafe // Java JAR file Extra Field Header ID
	TagAndroidAlignment      uint16 = 0xd935 // Android ZIP Alignment Extra Field
	TagKoreanCodePage        uint16 = 0xe57a // Korean ZIP code page info
	TagSMSQDOS               uint16 = 0xfd4a // SMS/QDOS
	TagWinZipAES             uint16 = 0x9901 // WinZip AE-x encryption structure
	AESEncryptionTag         uint16 = 0x9901 // Backward compatible alias
)

// Strong Encryption Algorithm Identifiers (APPNOTE 7.2.3.2 & 7.2.4.3).
const (
	AlgDES      uint16 = 0x6601 // DES
	AlgRC2Old   uint16 = 0x6602 // RC2 (version needed to extract < 5.2)
	Alg3DES168  uint16 = 0x6603 // 3DES 168-bit
	Alg3DES112  uint16 = 0x6609 // 3DES 112-bit
	AlgAES128   uint16 = 0x660e // AES 128-bit
	AlgAES192   uint16 = 0x660f // AES 192-bit
	AlgAES256   uint16 = 0x6610 // AES 256-bit
	AlgRC2New   uint16 = 0x6702 // RC2 (version needed to extract >= 5.2)
	AlgBlowfish uint16 = 0x6720 // Blowfish
	AlgTwofish  uint16 = 0x6721 // Twofish
	AlgRC4      uint16 = 0x6801 // RC4
	AlgUnknown  uint16 = 0xffff // Unknown algorithm
)

// Hash Algorithm Identifiers for Central Directory Encryption / Signing (APPNOTE 7.3.4).
const (
	HashNone      uint16 = 0x0000 // None
	HashCRC32     uint16 = 0x0001 // CRC32
	HashMD5       uint16 = 0x8003 // MD5
	HashSHA1      uint16 = 0x8004 // SHA-1
	HashRIPEMD160 uint16 = 0x8007 // RIPEMD-160
	HashSHA256    uint16 = 0x800c // SHA-256
	HashSHA384    uint16 = 0x800d // SHA-384
	HashSHA512    uint16 = 0x800e // SHA-512
)

// Standard integer limits before ZIP64 extensions become mandatory.
const (
	MaxUint16 uint64 = 1<<16 - 1
	MaxUint32 uint64 = 1<<32 - 1
)
