// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/endian"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/vfs"
)

// Severity indicates the critical level of a diagnostic issue.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
	SeveritySecurityHazard
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarning:
		return "WARN"
	case SeverityError:
		return "ERROR"
	case SeveritySecurityHazard:
		return "SECURITY"
	default:
		return "UNKNOWN"
	}
}

// DiagnosticIssue represents a structural, historical, or security finding in an archive.
type DiagnosticIssue struct {
	Code           string   `json:"code"`
	Severity       Severity `json:"severity"`
	Message        string   `json:"message"`
	Offset         int64    `json:"offset"`
	FileName       string   `json:"file_name,omitempty"`
	AppnoteSection string   `json:"appnote_section,omitempty"`
}

// ExtraTagDiagnostic details metadata for recognized or undocumented Extra Field tags.
type ExtraTagDiagnostic struct {
	Tag      uint16 `json:"tag"`
	TagHex   string `json:"tag_hex"`
	Name     string `json:"name"`
	Category string `json:"category"`
	IsKnown  bool   `json:"is_known"`
}

// EntryReport contains per-file structural metadata and diagnostics.
type EntryReport struct {
	Name              string               `json:"name"`
	LocalHeaderOffset int64                `json:"local_header_offset"`
	Method            codec.Method         `json:"method"`
	MethodName        string               `json:"method_name"`
	CompressedSize    int64                `json:"compressed_size"`
	UncompressedSize  int64                `json:"uncompressed_size"`
	RatioPercent      float64              `json:"ratio_percent"`
	CRC32             uint32               `json:"crc32"`
	VersionMadeBy     uint16               `json:"version_made_by"`
	HostOS            string               `json:"host_os"`
	VersionNeeded     uint16               `json:"version_needed"`
	BitFlags          uint16               `json:"bit_flags"`
	ExtraFields       []ExtraTagDiagnostic `json:"extra_fields"`
	IsEncrypted       bool                 `json:"is_encrypted"`
	EncryptionType    string               `json:"encryption_type"`
	Issues            []DiagnosticIssue    `json:"issues,omitempty"`
}

// InspectionReport contains comprehensive architectural, security, and metadata analysis of an archive.
type InspectionReport struct {
	FileSize              int64                `json:"file_size"`
	IsSFX                 bool                 `json:"is_sfx"`
	SFXStubSize           int64                `json:"sfx_stub_size"`
	IsZip64               bool                 `json:"is_zip64"`
	HasEOCD               bool                 `json:"has_eocd"`
	EntriesCount          int                  `json:"entries_count"`
	TotalUncompressedSize int64                `json:"total_uncompressed_size"`
	TotalCompressedSize   int64                `json:"total_compressed_size"`
	GlobalRatioPercent    float64              `json:"global_ratio_percent"`
	Comment               string               `json:"comment,omitempty"`
	VintageSystems        []string             `json:"vintage_systems,omitempty"`
	ExoticExtraFields     []ExtraTagDiagnostic `json:"exotic_extra_fields,omitempty"`
	UnassignedExtraFields []ExtraTagDiagnostic `json:"unassigned_extra_fields,omitempty"`
	TrailingGarbageSize   int64                `json:"trailing_garbage_size"`
	Issues                []DiagnosticIssue    `json:"issues"`
	Entries               []EntryReport        `json:"entries"`
}

// HostOSName returns human-readable name of the creator operating system (APPNOTE 4.4.2.2).
func HostOSName(hostByte byte) (string, bool) {
	switch hostByte {
	case HostFAT:
		return "MS-DOS / FAT", false
	case HostAmiga:
		return "Commodore Amiga", true
	case HostOpenVMS:
		return "OpenVMS", true
	case HostUNIX:
		return "UNIX / Linux", false
	case HostVMCMS:
		return "IBM VM/CMS", true
	case HostAtariST:
		return "Atari ST", true
	case HostOS2HPFS:
		return "OS/2 HPFS", true
	case HostMacintosh:
		return "Classic Macintosh", true
	case HostZSystem:
		return "Z-System", true
	case HostCPM:
		return "CP/M", true
	case HostNTFS:
		return "Windows NTFS", false
	case HostMVS:
		return "IBM MVS / zOS", true
	case HostAcornRisc:
		return "Acorn RISC OS", true
	case HostBeOS:
		return "BeOS / Haiku", true
	case HostOSX:
		return "macOS (Darwin)", false
	default:
		return fmt.Sprintf("Undefined / Custom (0x%02X)", hostByte), true
	}
}

// ExtraTagInfo resolves human-readable metadata, standard category, and known status for any Extra Field tag.
func ExtraTagInfo(tag uint16) ExtraTagDiagnostic {
	hexStr := fmt.Sprintf("0x%04X", tag)
	switch tag {
	// PKWARE Defined (APPNOTE 4.5.2)
	case TagZip64:
		return ExtraTagDiagnostic{tag, hexStr, "Zip64 Extended Information", "PKWARE Standard", true}
	case TagAVInfo:
		return ExtraTagDiagnostic{tag, hexStr, "PKWARE AV Info", "PKWARE Standard", true}
	case TagLanguageEncoding:
		return ExtraTagDiagnostic{tag, hexStr, "Extended Language Encoding (PFS)", "PKWARE Standard", true}
	case TagOS2:
		return ExtraTagDiagnostic{tag, hexStr, "OS/2 Extended Attributes", "PKWARE Standard", true}
	case TagNTFS:
		return ExtraTagDiagnostic{tag, hexStr, "NTFS Timestamps / Attributes", "PKWARE Standard", true}
	case TagOpenVMS:
		return ExtraTagDiagnostic{tag, hexStr, "OpenVMS File Attributes", "PKWARE Standard", true}
	case TagUNIX:
		return ExtraTagDiagnostic{tag, hexStr, "UNIX File Attributes", "PKWARE Standard", true}
	case TagFork:
		return ExtraTagDiagnostic{tag, hexStr, "Macintosh Resource Fork Descriptor", "PKWARE Standard", true}
	case TagPatch:
		return ExtraTagDiagnostic{tag, hexStr, "Patch Descriptor", "PKWARE Standard", true}
	case TagPKCS7Store:
		return ExtraTagDiagnostic{tag, hexStr, "PKCS#7 Certificate Store", "PKWARE Security", true}
	case TagFileCert:
		return ExtraTagDiagnostic{tag, hexStr, "X.509 File Signature Certificate", "PKWARE Security", true}
	case TagCDCert:
		return ExtraTagDiagnostic{tag, hexStr, "X.509 Central Directory Certificate", "PKWARE Security", true}
	case TagStrongEncryptHeader:
		return ExtraTagDiagnostic{tag, hexStr, "Strong Encryption Header", "PKWARE Security", true}
	case TagRecordManagement:
		return ExtraTagDiagnostic{tag, hexStr, "Record Management Controls", "PKWARE Standard", true}
	case TagPKCS7RecipientList:
		return ExtraTagDiagnostic{tag, hexStr, "PKCS#7 Recipient List", "PKWARE Security", true}
	case TagTimestampRecord:
		return ExtraTagDiagnostic{tag, hexStr, "Timestamp Record", "PKWARE Standard", true}
	case TagSmartcryptKeyProvider:
		return ExtraTagDiagnostic{tag, hexStr, "Smartcrypt Key Provider", "PKWARE Security", true}
	case TagIBMZ390, TagIBMZ390Compressed:
		return ExtraTagDiagnostic{tag, hexStr, "IBM S/390 AS/400 Attributes", "IBM Mainframe", true}
	case TagPOSZIP:
		return ExtraTagDiagnostic{tag, hexStr, "POSZIP 4690", "IBM POS", true}

	// Info-ZIP & Modern Third Party Standards (APPNOTE 4.6.1)
	case TagExtendedTimestamp:
		return ExtraTagDiagnostic{tag, hexStr, "Info-ZIP Extended Timestamp ('UT')", "Info-ZIP Standard", true}
	case TagInfoZipUnicodePath:
		return ExtraTagDiagnostic{tag, hexStr, "Info-ZIP Unicode Path ('up')", "Info-ZIP Standard", true}
	case TagInfoZipUnicodeComment:
		return ExtraTagDiagnostic{tag, hexStr, "Info-ZIP Unicode Comment ('uc')", "Info-ZIP Standard", true}
	case TagInfoZipUIDGID:
		return ExtraTagDiagnostic{tag, hexStr, "Info-ZIP UID/GID ('ux')", "Info-ZIP Standard", true}
	case TagInfoZipUNIXNew, TagInfoZipUNIXOld:
		return ExtraTagDiagnostic{tag, hexStr, "Info-ZIP UNIX Attributes", "Info-ZIP Standard", true}
	case TagWinZipAES:
		return ExtraTagDiagnostic{tag, hexStr, "WinZip AE-x AES Encryption", "WinZip Standard", true}

	// Exotic & Third Party Ecosystem Tags
	case TagAndroidAlignment:
		return ExtraTagDiagnostic{tag, hexStr, "Android APK Page Alignment", "Google Android", true}
	case TagJavaJAR:
		return ExtraTagDiagnostic{tag, hexStr, "Java JAR Manifest Header", "Oracle Java", true}
	case TagOpenPackagingGrowth:
		return ExtraTagDiagnostic{tag, hexStr, "Open Packaging Growth Hint", "Microsoft OpenXML", true}
	case TagDataStreamAlignment:
		return ExtraTagDiagnostic{tag, hexStr, "Data Stream Alignment Padding", "Apache Commons", true}
	case TagPixarUSD:
		return ExtraTagDiagnostic{tag, hexStr, "Pixar Universal Scene Description", "Pixar USD", true}
	case TagWindowsNTACL:
		return ExtraTagDiagnostic{tag, hexStr, "Windows NT Security Descriptor (ACL)", "Microsoft Windows", true}
	case TagOS2ACL:
		return ExtraTagDiagnostic{tag, hexStr, "OS/2 Access Control List", "IBM OS/2", true}
	case TagMacintosh, TagZipItMacLong, TagZipItMacShortFile, TagZipItMacShortDir, TagInfoZipMac, TagMacSmartzip:
		return ExtraTagDiagnostic{tag, hexStr, "Vintage Macintosh Extra Field", "Classic Mac", true}
	case TagAcornSparkFS:
		return ExtraTagDiagnostic{tag, hexStr, "Acorn SparkFS Extra Field", "Acorn RISC", true}
	case TagBeOS:
		return ExtraTagDiagnostic{tag, hexStr, "BeOS File Attributes", "BeOS / Haiku", true}
	case TagKoreanCodePage:
		return ExtraTagDiagnostic{tag, hexStr, "Korean Code Page Info", "CodePage Spec", true}

	default:
		return ExtraTagDiagnostic{
			tag,
			hexStr,
			"Unassigned / Undocumented Proprietary Field",
			"Unknown / Vendor Custom",
			false,
		}
	}
}

// Inspect performs a comprehensive structural, cryptographic, and security analysis over a ZIP stream.
func Inspect(rAt io.ReaderAt, size int64) (*InspectionReport, error) {
	if rAt == nil || size <= 0 {
		return nil, errors.New("zip: invalid reader or zero size")
	}

	report := &InspectionReport{
		FileSize:              size,
		Issues:                make([]DiagnosticIssue, 0, 8),
		Entries:               make([]EntryReport, 0, 16),
		ExoticExtraFields:     make([]ExtraTagDiagnostic, 0, 4),
		UnassignedExtraFields: make([]ExtraTagDiagnostic, 0, 4),
	}

	// 1. Locate End of Central Directory (EOCD)
	end, endOffset, err := findEOCD(rAt, size)
	if err != nil {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:           "ERR_NO_EOCD",
			Severity:       SeverityError,
			Message:        "End of Central Directory (EOCD) signature not found; archive may be damaged or truncated",
			Offset:         size,
			AppnoteSection: "4.4.14",
		})
		return report, nil //nolint:nilerr // Returning partial diagnostic report on missing EOCD
	}

	report.HasEOCD = true
	report.Comment = end.Comment

	// 2. Check for Trailing Data after EOCD comment
	expectedEndOffset := endOffset + 22 + int64(end.CommentLength)
	if expectedEndOffset < size {
		garbage := size - expectedEndOffset
		report.TrailingGarbageSize = garbage
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "WARN_TRAILING_GARBAGE",
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"Found %d unreferenced trailing bytes after EOCD (possible overlay, steganography, or file tampering)",
				garbage,
			),
			Offset:         expectedEndOffset,
			AppnoteSection: "4.4.14",
		})
	}

	// 3. Check for Zip64 EOCD Locator
	cdEntriesCount := uint64(end.EntriesNum)
	cdSize := int64(end.CentralDirSize)
	cdOffset := int64(end.CentralDirOffset)

	if endOffset >= 20 {
		var locBuf [20]byte
		if _, err := rAt.ReadAt(locBuf[:], endOffset-20); err == nil {
			if endian.Load32(locBuf[:], 0) == Zip64EOCDLocatorSignature {
				report.IsZip64 = true
				z64Offset := int64(endian.Load64(locBuf[:], 8))
				z64Rec := io.NewSectionReader(rAt, z64Offset, 56)
				if z64EOCD, err := ReadZip64EOCD(z64Rec); err == nil {
					cdEntriesCount = z64EOCD.EntriesNum
					cdSize = int64(z64EOCD.CentralDirSize)
					cdOffset = int64(z64EOCD.CentralDirOffset)
				}
			}
		}
	}

	// 4. SFX Stub Detection
	var sfxOffset int64
	if actualCD := endOffset - cdSize; actualCD > cdOffset {
		sfxOffset = actualCD - cdOffset
		report.IsSFX = true
		report.SFXStubSize = sfxOffset
		cdOffset = actualCD

		var stubHdr [4]byte
		_, _ = rAt.ReadAt(stubHdr[:], 0)
		stubType := "Executable Stub"
		if stubHdr[0] == 'M' && stubHdr[1] == 'Z' {
			stubType = "Windows PE32 / DOS Executable"
		} else if stubHdr[0] == 0x7F && stubHdr[1] == 'E' && stubHdr[2] == 'L' && stubHdr[3] == 'F' {
			stubType = "Linux / Unix ELF Executable"
		}

		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "INFO_SFX_STUB",
			Severity: SeverityInfo,
			Message: fmt.Sprintf(
				"Self-Extracting Archive (SFX) detected: %d bytes %s prepended",
				sfxOffset,
				stubType,
			),
			Offset:         0,
			AppnoteSection: "4.1.0",
		})
	}

	if cdOffset < 0 || cdOffset+cdSize > size {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "ERR_INVALID_CD_OFFSET",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"Central Directory offset %d is outside file bounds (file size: %d)",
				cdOffset,
				size,
			),
			Offset:         cdOffset,
			AppnoteSection: "4.4.3",
		})
		return report, nil
	}

	// 5. Parse Central Directory Entries
	cdReader := io.NewSectionReader(rAt, cdOffset, cdSize)
	report.EntriesCount = int(cdEntriesCount)

	vintageOSMap := make(map[string]bool)
	exoticTagMap := make(map[uint16]ExtraTagDiagnostic)
	unassignedTagMap := make(map[uint16]ExtraTagDiagnostic)

	for i := uint64(0); i < cdEntriesCount; i++ {
		var sig [4]byte
		if _, err := io.ReadFull(cdReader, sig[:]); err != nil {
			report.Issues = append(report.Issues, DiagnosticIssue{
				Code:           "ERR_TRUNCATED_CD",
				Severity:       SeverityError,
				Message:        fmt.Sprintf("Central Directory truncated at entry %d: %v", i, err),
				Offset:         cdOffset,
				AppnoteSection: "4.4.3",
			})
			break
		}
		if endian.Load32(sig[:], 0) != CentralDirectorySignature {
			report.Issues = append(report.Issues, DiagnosticIssue{
				Code:     "ERR_CORRUPT_CD_SIG",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"Corrupt Central Directory signature 0x%08X at entry %d",
					endian.Load32(sig[:], 0),
					i,
				),
				Offset:         cdOffset,
				AppnoteSection: "4.4.3",
			})
			break
		}

		cd, err := ReadCentralDirEntry(cdReader)
		if err != nil {
			report.Issues = append(report.Issues, DiagnosticIssue{
				Code:           "ERR_CD_PARSE",
				Severity:       SeverityError,
				Message:        fmt.Sprintf("Failed parsing Central Directory entry %d: %v", i, err),
				Offset:         cdOffset,
				AppnoteSection: "4.4.3",
			})
			break
		}

		entryRep := analyzeEntry(cd, sfxOffset, rAt, size)
		report.Entries = append(report.Entries, entryRep)
		report.TotalUncompressedSize += entryRep.UncompressedSize
		report.TotalCompressedSize += entryRep.CompressedSize

		// Collect Vintage Systems
		hostByte := byte(cd.VersionMadeBy >> 8)
		if osName, isVintage := HostOSName(hostByte); isVintage {
			vintageOSMap[osName] = true
		}

		// Collect Extra Field Insights
		for _, info := range entryRep.ExtraFields {
			if !info.IsKnown {
				unassignedTagMap[info.Tag] = info
			} else if info.Category != "PKWARE Standard" && info.Category != "Info-ZIP Standard" && info.Category != "WinZip Standard" {
				exoticTagMap[info.Tag] = info
			}
		}

		// Bubble up entry issues to report
		report.Issues = append(report.Issues, entryRep.Issues...)
	}

	for osName := range vintageOSMap {
		report.VintageSystems = append(report.VintageSystems, osName)
	}
	for _, info := range exoticTagMap {
		report.ExoticExtraFields = append(report.ExoticExtraFields, info)
	}
	for _, info := range unassignedTagMap {
		report.UnassignedExtraFields = append(report.UnassignedExtraFields, info)
	}

	if report.TotalUncompressedSize > 0 {
		report.GlobalRatioPercent = (float64(report.TotalCompressedSize) / float64(report.TotalUncompressedSize)) * 100.0
	}

	return report, nil
}

func analyzeEntry(cd CentralDirectory, sfxOffset int64, rAt io.ReaderAt, fileSize int64) EntryReport {
	name := cd.Filename
	cleanName := vfs.CleanPath(name)
	method := codec.Method(cd.CompressionMethod)
	compSize := int64(cd.CompressedSize)
	uncompSize := int64(cd.UncompressedSize)
	localOffset := int64(cd.LocalHeaderOffset) + sfxOffset
	hostByte := byte(cd.VersionMadeBy >> 8)
	hostOS, isVintageHost := HostOSName(hostByte)

	var ratio float64
	if uncompSize > 0 {
		ratio = (float64(compSize) / float64(uncompSize)) * 100.0
	}

	var tagsInfo []ExtraTagDiagnostic
	for tag := range ExtraFields(cd.ExtraField) {
		tagsInfo = append(tagsInfo, ExtraTagInfo(tag))
	}

	isEncrypted := (cd.GeneralPurposeBitFlag & FlagEncrypted) != 0
	encType := "None"
	if isEncrypted {
		encType = "ZipCrypto (Traditional)"
		if (cd.GeneralPurposeBitFlag & FlagStrongEncrypt) != 0 {
			encType = "PKWARE Strong Encryption"
		}
	}
	for _, info := range tagsInfo {
		if info.Tag == TagWinZipAES {
			encType = "WinZip AES"
			isEncrypted = true
		}
	}

	rep := EntryReport{
		Name:              cleanName,
		LocalHeaderOffset: localOffset,
		Method:            method,
		MethodName:        method.String(),
		CompressedSize:    compSize,
		UncompressedSize:  uncompSize,
		RatioPercent:      ratio,
		CRC32:             cd.CRC32,
		VersionMadeBy:     cd.VersionMadeBy,
		HostOS:            hostOS,
		VersionNeeded:     cd.VersionNeededToExtract,
		BitFlags:          cd.GeneralPurposeBitFlag,
		ExtraFields:       tagsInfo,
		IsEncrypted:       isEncrypted,
		EncryptionType:    encType,
		Issues:            make([]DiagnosticIssue, 0, 2),
	}

	// 1. Check for Zip Slip Security Hazard
	if strings.Contains(name, "../") || strings.Contains(name, "..\\") || strings.HasPrefix(name, "/") ||
		strings.HasPrefix(name, "\\") {
		rep.Issues = append(rep.Issues, DiagnosticIssue{
			Code:           "SEC_ZIP_SLIP",
			Severity:       SeveritySecurityHazard,
			Message:        fmt.Sprintf("Directory Traversal (Zip Slip) detected in path %q", name),
			Offset:         localOffset,
			FileName:       name,
			AppnoteSection: "4.4.17",
		})
	}

	// 2. Check for Zip Bomb / Extreme Compression Ratio
	if uncompSize > 10*1024*1024 && compSize > 0 && (uncompSize/compSize) > 500 {
		rep.Issues = append(rep.Issues, DiagnosticIssue{
			Code:     "SEC_ZIP_BOMB",
			Severity: SeveritySecurityHazard,
			Message: fmt.Sprintf(
				"Suspicious high compression ratio (%d:1) on %s (Uncompressed: %d MB)",
				uncompSize/compSize,
				name,
				uncompSize/(1024*1024),
			),
			Offset:         localOffset,
			FileName:       name,
			AppnoteSection: "4.4.4",
		})
	}

	// 3. Check for Reserved / Undefined Bit Flags (APPNOTE 4.4.4)
	reservedFlags := cd.GeneralPurposeBitFlag & 0xF480 // Bits 7, 8, 9, 10, 12, 14, 15
	if reservedFlags != 0 {
		rep.Issues = append(rep.Issues, DiagnosticIssue{
			Code:           "WARN_RESERVED_BIT_FLAGS",
			Severity:       SeverityWarning,
			Message:        fmt.Sprintf("Reserved general purpose bit flags set: 0x%04X", reservedFlags),
			Offset:         localOffset,
			FileName:       name,
			AppnoteSection: "4.4.4",
		})
	}

	// 4. Check for Unassigned / Undocumented Extra Fields
	for _, info := range tagsInfo {
		if !info.IsKnown {
			rep.Issues = append(rep.Issues, DiagnosticIssue{
				Code:           "INFO_UNASSIGNED_EXTRA_TAG",
				Severity:       SeverityInfo,
				Message:        fmt.Sprintf("Undocumented/proprietary Extra Field tag %s (%s)", info.TagHex, info.Name),
				Offset:         localOffset,
				FileName:       name,
				AppnoteSection: "4.5.0",
			})
		}
	}

	// 5. Check for Vintage Creator Host System
	if isVintageHost {
		rep.Issues = append(rep.Issues, DiagnosticIssue{
			Code:           "INFO_VINTAGE_HOST_OS",
			Severity:       SeverityInfo,
			Message:        "Archive entry was authored on vintage system " + hostOS,
			Offset:         localOffset,
			FileName:       name,
			AppnoteSection: "4.4.2.2",
		})
	}

	// 6. Inspect Corresponding Local File Header for Inconsistencies
	if localOffset >= 0 && localOffset+30 <= fileSize {
		sr := io.NewSectionReader(rAt, localOffset, fileSize-localOffset)
		var sig [4]byte
		if _, err := io.ReadFull(sr, sig[:]); err == nil && endian.Load32(sig[:], 0) == LocalFileHeaderSignature {
			if lfh, err := ReadLocalFileHeader(sr); err == nil {
				if lfh.Filename != cd.Filename && lfh.Filename != "" {
					rep.Issues = append(rep.Issues, DiagnosticIssue{
						Code:     "WARN_NAME_MISMATCH",
						Severity: SeverityWarning,
						Message: fmt.Sprintf(
							"Local Header filename %q differs from Central Directory filename %q",
							lfh.Filename,
							cd.Filename,
						),
						Offset:         localOffset,
						FileName:       name,
						AppnoteSection: "4.4.1",
					})
				}
				if (lfh.GeneralPurposeBitFlag&FlagDataDescriptor) == 0 && lfh.CRC32 != 0 && lfh.CRC32 != cd.CRC32 {
					rep.Issues = append(rep.Issues, DiagnosticIssue{
						Code:     "ERR_CRC_MISMATCH",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"Local Header CRC 0x%08X differs from Central Directory CRC 0x%08X",
							lfh.CRC32,
							cd.CRC32,
						),
						Offset:         localOffset,
						FileName:       name,
						AppnoteSection: "4.4.1",
					})
				}
			}
		}
	}

	return rep
}
