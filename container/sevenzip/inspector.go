// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"path/filepath"
	"strings"
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

// DiagnosticIssue represents a structural anomaly, security alert, or integrity warning.
type DiagnosticIssue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Offset   int64    `json:"offset"`
	FileName string   `json:"file_name,omitempty"`
}

// CoderDiagnostic details a single compression or filtering step in a 7z folder coder graph.
type CoderDiagnostic struct {
	MethodID   uint64 `json:"method_id"`
	MethodName string `json:"method_name"`
	Properties string `json:"properties,omitempty"`
}

// FolderDiagnostic details a solid stream folder and its coder chain.
type FolderDiagnostic struct {
	Index      int               `json:"index"`
	Coders     []CoderDiagnostic `json:"coders"`
	UnpackSize uint64            `json:"unpack_size"`
	HasCRC     bool              `json:"has_crc"`
	CRC        uint32            `json:"crc"`
	PackedSize uint64            `json:"packed_size"`
}

// Report captures the comprehensive diagnostic state of a 7-Zip archive.
type Report struct {
	FileSize              int64              `json:"file_size"`
	SignatureValid        bool               `json:"signature_valid"`
	SignatureOffset       int64              `json:"signature_offset"`
	IsSFX                 bool               `json:"is_sfx"`
	SFXStubSize           int64              `json:"sfx_stub_size"`
	MajorVersion          byte               `json:"major_version"`
	MinorVersion          byte               `json:"minor_version"`
	StartHeaderCRCValid   bool               `json:"start_header_crc_valid"`
	NextHeaderOffset      uint64             `json:"next_header_offset"`
	NextHeaderSize        uint64             `json:"next_header_size"`
	NextHeaderCRCValid    bool               `json:"next_header_crc_valid"`
	IsEncodedHeader       bool               `json:"is_encoded_header"`
	IsEncryptedHeader     bool               `json:"is_encrypted_header"`
	HasEncryptedStreams   bool               `json:"has_encrypted_streams"`
	FoldersCount          int                `json:"folders_count"`
	Folders               []FolderDiagnostic `json:"folders"`
	EntriesCount          int                `json:"entries_count"`
	TotalUncompressedSize int64              `json:"total_uncompressed_size"`
	TotalCompressedSize   int64              `json:"total_compressed_size"`
	GlobalRatioPercent    float64            `json:"global_ratio_percent"`
	TrailingGarbageSize   int64              `json:"trailing_garbage_size"`
	Issues                []DiagnosticIssue  `json:"issues"`
}

func methodToString(id uint64) string {
	switch id {
	case MethodCopy:
		return "Copy"
	case MethodDelta:
		return "Delta"
	case MethodLZMA:
		return "LZMA"
	case MethodLZMA2:
		return "LZMA2"
	case MethodAES:
		return "AES-256"
	case MethodBCJ:
		return "BCJ (x86)"
	case MethodBCJ2:
		return "BCJ2 (4-Stream)"
	case MethodARM:
		return "ARM"
	case MethodPPMD:
		return "PPMd"
	case MethodBZip2:
		return "BZip2"
	case MethodDeflate:
		return "Deflate"
	default:
		return fmt.Sprintf("Unknown (0x%X)", id)
	}
}

// Inspect performs deep byte diagnostics, format verification, and security audits on a 7z archive.
func Inspect(rAt io.ReaderAt, size int64, password string) (*Report, error) {
	if rAt == nil || size < StartHeaderSize {
		return nil, errors.New("sevenzip: archive too small or nil reader")
	}

	report := &Report{
		FileSize: size,
	}

	// 1. Locate 7z Signature and detect SFX
	sigOffset, err := findSignature(rAt, size)
	if err != nil {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "ERR_SIGNATURE_NOT_FOUND",
			Severity: SeverityError,
			Message:  "Valid 7-Zip signature (7z 0xBC 0xAF 0x27 0x1C) not found",
			Offset:   0,
		})
		return report, err
	}

	report.SignatureValid = true
	report.SignatureOffset = sigOffset
	if sigOffset > 0 {
		report.IsSFX = true
		report.SFXStubSize = sigOffset
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "INFO_SFX_STUB_DETECTED",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("Self-Extracting (SFX) executable stub detected (%d bytes prepended)", sigOffset),
			Offset:   0,
		})
	}

	// 2. Validate StartHeader
	var startBuf [StartHeaderSize - SignatureSize]byte
	if _, err := rAt.ReadAt(startBuf[:], sigOffset+SignatureSize); err != nil {
		return report, err
	}

	report.MajorVersion = startBuf[0]
	report.MinorVersion = startBuf[1]

	startCRC := binary.LittleEndian.Uint32(startBuf[2:6])
	calcCRC := crc32.ChecksumIEEE(startBuf[6:26])
	report.StartHeaderCRCValid = (startCRC == calcCRC)
	if !report.StartHeaderCRCValid {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "ERR_START_HEADER_CRC_CORRUPTED",
			Severity: SeverityError,
			Message:  fmt.Sprintf("StartHeader CRC32 mismatch: expected 0x%08X, calculated 0x%08X", startCRC, calcCRC),
			Offset:   sigOffset + 8,
		})
	}

	nextOffset := binary.LittleEndian.Uint64(startBuf[6:14])
	nextSize := binary.LittleEndian.Uint64(startBuf[14:22])
	nextCRC := binary.LittleEndian.Uint32(startBuf[22:26])

	report.NextHeaderOffset = nextOffset
	report.NextHeaderSize = nextSize

	headerAbsoluteOffset := sigOffset + int64(StartHeaderSize) + int64(nextOffset)
	if headerAbsoluteOffset+int64(nextSize) > size {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "ERR_HEADER_OUT_OF_BOUNDS",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"Header offset %d + size %d exceeds total archive size %d",
				headerAbsoluteOffset,
				nextSize,
				size,
			),
			Offset: headerAbsoluteOffset,
		})
		return report, nil
	}

	// 3. Read NextHeader and check CRC
	headerData := make([]byte, nextSize)
	if _, err := rAt.ReadAt(headerData, headerAbsoluteOffset); err != nil {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "ERR_NEXT_HEADER_READ_FAILED",
			Severity: SeverityError,
			Message:  fmt.Sprintf("Failed reading next header: %v", err),
			Offset:   headerAbsoluteOffset,
		})
		return report, nil
	}

	calcNextCRC := crc32.ChecksumIEEE(headerData)
	report.NextHeaderCRCValid = (calcNextCRC == nextCRC)
	if !report.NextHeaderCRCValid {
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "ERR_NEXT_HEADER_CRC_CORRUPTED",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"NextHeader CRC32 mismatch: expected 0x%08X, calculated 0x%08X",
				nextCRC,
				calcNextCRC,
			),
			Offset: headerAbsoluteOffset,
		})
	}

	// 4. Trailing garbage check
	endOfArchive := headerAbsoluteOffset + int64(nextSize)
	if size > endOfArchive {
		report.TrailingGarbageSize = size - endOfArchive
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "WARN_TRAILING_GARBAGE",
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"Detected %d unreferenced trailing bytes after end of header (possible steganography or payload appended)",
				report.TrailingGarbageSize,
			),
			Offset: endOfArchive,
		})
	}

	// 5. Inspect Header structure (Encoded vs Plain)
	if len(headerData) > 0 && headerData[0] == NIDEncodedHeader {
		report.IsEncodedHeader = true
		report.Issues = append(report.Issues, DiagnosticIssue{
			Code:     "INFO_ENCODED_HEADER",
			Severity: SeverityInfo,
			Message:  "Archive catalog metadata is compressed inside an EncodedHeader stream",
			Offset:   headerAbsoluteOffset,
		})

		// Check if EncodedHeader uses AES
		if bytes.Contains(headerData, []byte{0x06, 0xF1, 0x07, 0x01}) {
			report.IsEncryptedHeader = true
			report.Issues = append(report.Issues, DiagnosticIssue{
				Code:     "SEC_ENCRYPTED_HEADER",
				Severity: SeverityInfo,
				Message:  "Archive catalog and file names are protected with AES-256 encryption",
				Offset:   headerAbsoluteOffset,
			})
		}
	}

	// 6. Detailed Entry and Coder Inspection via Reader
	readerAt := rAt
	if sigOffset > 0 {
		readerAt = io.NewSectionReader(rAt, sigOffset, size-sigOffset)
	}

	zr, err := NewReader(readerAt, size-sigOffset, password)
	if err != nil {
		if errors.Is(err, ErrInvalidPassword) {
			report.Issues = append(report.Issues, DiagnosticIssue{
				Code:     "INFO_PASSWORD_REQUIRED",
				Severity: SeverityInfo,
				Message:  "Archive requires password for full inspection of catalog entries",
				Offset:   headerAbsoluteOffset,
			})
		} else {
			report.Issues = append(report.Issues, DiagnosticIssue{
				Code:     "WARN_CATALOG_UNREADABLE",
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Could not parse file entries: %v", err),
				Offset:   headerAbsoluteOffset,
			})
		}
		return report, nil
	}

	report.FoldersCount = len(zr.folders)
	for i, f := range zr.folders {
		fd := FolderDiagnostic{
			Index:      i,
			HasCRC:     f.HasCRC,
			CRC:        f.UnpackCRC,
			UnpackSize: 0,
		}
		if len(f.UnpackSizes) > 0 {
			fd.UnpackSize = f.UnpackSizes[0]
		}
		if i < len(zr.packSizes) {
			fd.PackedSize = zr.packSizes[i]
		}

		for _, c := range f.Coders {
			cd := CoderDiagnostic{
				MethodID:   c.MethodID,
				MethodName: methodToString(c.MethodID),
			}
			if len(c.Properties) > 0 {
				cd.Properties = fmt.Sprintf("%d bytes props", len(c.Properties))
			}
			if c.MethodID == MethodAES {
				report.HasEncryptedStreams = true
			}
			fd.Coders = append(fd.Coders, cd)
		}
		report.Folders = append(report.Folders, fd)
	}

	// 7. Inspect Entries & Security Scanning
	entries := zr.Entries()
	report.EntriesCount = len(entries)

	for _, e := range entries {
		report.TotalUncompressedSize += e.UncompressedSize()
		report.TotalCompressedSize += e.CompressedSize()

		// Security Check: Zip Slip (Directory Traversal)
		cleanPath := filepath.Clean(filepath.ToSlash(e.Name()))
		if strings.HasPrefix(cleanPath, "../") || cleanPath == ".." || strings.Contains(e.Name(), "../") ||
			strings.Contains(e.Name(), "..\\") {
			report.Issues = append(report.Issues, DiagnosticIssue{
				Code:     "SEC_ZIP_SLIP_HAZARD",
				Severity: SeveritySecurityHazard,
				Message:  fmt.Sprintf("Dangerous directory traversal sequence detected in entry %q", e.Name()),
				FileName: e.Name(),
			})
		}

		// Security Check: High-ratio Zip Bomb
		if e.UncompressedSize() > 100*1024*1024 && e.CompressedSize() > 0 {
			ratio := float64(e.UncompressedSize()) / float64(e.CompressedSize())
			if ratio > 100.0 {
				report.Issues = append(report.Issues, DiagnosticIssue{
					Code:     "SEC_HIGH_COMPRESSION_BOMB",
					Severity: SeveritySecurityHazard,
					Message: fmt.Sprintf(
						"Suspiciously high compression ratio (%.1f:1) on entry %q (Uncompressed: %d bytes)",
						ratio,
						e.Name(),
						e.UncompressedSize(),
					),
					FileName: e.Name(),
				})
			}
		}
	}

	if report.TotalUncompressedSize > 0 {
		report.GlobalRatioPercent = float64(report.TotalCompressedSize) / float64(report.TotalUncompressedSize) * 100.0
	}

	return report, nil
}

func findSignature(rAt io.ReaderAt, size int64) (int64, error) {
	// Check offset 0 first
	var sig [SignatureSize]byte
	if _, err := rAt.ReadAt(sig[:], 0); err == nil && sig == Signature {
		return 0, nil
	}

	// Scan first 1MB for SFX offset shift
	scanLimit := min(size, 1024*1024)
	buf := make([]byte, scanLimit)
	n, err := rAt.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return -1, err
	}

	idx := bytes.Index(buf[:n], Signature[:])
	if idx >= 0 {
		return int64(idx), nil
	}

	return -1, ErrInvalidSignature
}
