// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/file"
)

func TestInspector_SFX_And_TrailingGarbage(t *testing.T) {
	zipBuf := new(bytes.Buffer)
	w := zip.NewWriter(zipBuf, codec.DefaultRegistry)

	f, _ := file.New("doc.txt", false)
	_ = w.WriteEntry(f, bytes.NewReader([]byte("Important document payload")))
	_ = w.Close()

	// Build SFX with 512 bytes stub and 128 bytes trailing garbage
	sfxStub := make([]byte, 512)
	copy(sfxStub, []byte("MZ\x90\x00PE-SFX-STUB"))
	trailingGarbage := []byte("STEGANOGRAPHY_OR_APPENDED_OVERLAY_BYTES_DATA")

	fullData := make([]byte, 0, len(sfxStub)+zipBuf.Len()+len(trailingGarbage))
	fullData = append(fullData, sfxStub...)
	fullData = append(fullData, zipBuf.Bytes()...)
	fullData = append(fullData, trailingGarbage...)

	report, err := zip.Inspect(bytes.NewReader(fullData), int64(len(fullData)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !report.IsSFX {
		t.Errorf("expected IsSFX to be true")
	}
	if report.SFXStubSize != 512 {
		t.Errorf("expected SFXStubSize 512, got %d", report.SFXStubSize)
	}
	if report.TrailingGarbageSize != int64(len(trailingGarbage)) {
		t.Errorf("expected TrailingGarbageSize %d, got %d", len(trailingGarbage), report.TrailingGarbageSize)
	}
	if report.EntriesCount != 1 {
		t.Errorf("expected 1 entry, got %d", report.EntriesCount)
	}
}

func TestInspector_ZipSlip_SecurityHazard(t *testing.T) {
	buf := new(bytes.Buffer)
	content := []byte("root:x:0:0:...")
	name := "../../etc/shadow"

	lfh := zip.LocalFileHeader{
		VersionNeededToExtract: zip.VersionDefault,
		CompressionMethod:      zip.MethodStore,
		CRC32:                  0x12345678,
		CompressedSize:         uint32(len(content)),
		UncompressedSize:       uint32(len(content)),
		FilenameLength:         uint16(len(name)),
		Filename:               name,
	}
	hdrBytes := lfh.AppendBytes(nil)
	buf.Write(hdrBytes)
	buf.Write(content)

	cdOffset := int64(len(hdrBytes) + len(content))
	cd := zip.CentralDirectory{
		VersionMadeBy:          uint16(zip.HostUNIX)<<8 | zip.VersionDefault,
		VersionNeededToExtract: zip.VersionDefault,
		CompressionMethod:      zip.MethodStore,
		CRC32:                  0x12345678,
		CompressedSize:         uint32(len(content)),
		UncompressedSize:       uint32(len(content)),
		FilenameLength:         uint16(len(name)),
		LocalHeaderOffset:      0,
		Filename:               name,
	}
	cdBytes := cd.AppendBytes(nil)
	buf.Write(cdBytes)

	eocd := zip.EncodeEOCD(1, int64(len(cdBytes)), cdOffset, "")
	buf.Write(eocd)

	report, err := zip.Inspect(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	foundZipSlip := false
	for _, issue := range report.Issues {
		if issue.Code == "SEC_ZIP_SLIP" && issue.Severity == zip.SeveritySecurityHazard {
			foundZipSlip = true
			break
		}
	}

	if !foundZipSlip {
		t.Errorf("expected SEC_ZIP_SLIP security hazard to be reported")
	}
}

func TestInspector_VintageHostOS_And_UnassignedExtraTag_And_ReservedBitFlags(t *testing.T) {
	buf := new(bytes.Buffer)
	content := []byte("Commodore Amiga workbench data")
	name := "amiga.dat"

	// Unassigned custom extra field: 0xDEAD with 4 bytes payload
	customExtra := []byte{0xAD, 0xDE, 0x04, 0x00, 0x01, 0x02, 0x03, 0x04}
	// Reserved bit 15 set: 0x8000
	reservedFlags := uint16(0x8000)

	lfh := zip.LocalFileHeader{
		VersionNeededToExtract: zip.VersionDefault,
		GeneralPurposeBitFlag:  reservedFlags,
		CompressionMethod:      zip.MethodStore,
		CRC32:                  0x98765432,
		CompressedSize:         uint32(len(content)),
		UncompressedSize:       uint32(len(content)),
		FilenameLength:         uint16(len(name)),
		ExtraFieldLength:       uint16(len(customExtra)),
		Filename:               name,
		ExtraField:             customExtra,
	}
	hdrBytes := lfh.AppendBytes(nil)
	buf.Write(hdrBytes)
	buf.Write(content)

	cdOffset := int64(len(hdrBytes) + len(content))
	cd := zip.CentralDirectory{
		// HostAmiga (1) in upper byte
		VersionMadeBy:          uint16(zip.HostAmiga)<<8 | zip.VersionDefault,
		VersionNeededToExtract: zip.VersionDefault,
		GeneralPurposeBitFlag:  reservedFlags,
		CompressionMethod:      zip.MethodStore,
		CRC32:                  0x98765432,
		CompressedSize:         uint32(len(content)),
		UncompressedSize:       uint32(len(content)),
		FilenameLength:         uint16(len(name)),
		ExtraFieldLength:       uint16(len(customExtra)),
		LocalHeaderOffset:      0,
		Filename:               name,
		ExtraField:             customExtra,
	}
	cdBytes := cd.AppendBytes(nil)
	buf.Write(cdBytes)

	eocd := zip.EncodeEOCD(1, int64(len(cdBytes)), cdOffset, "")
	buf.Write(eocd)

	report, err := zip.Inspect(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// 1. Verify Vintage OS
	if len(report.VintageSystems) == 0 || report.VintageSystems[0] != "Commodore Amiga" {
		t.Errorf("expected VintageSystems to contain 'Commodore Amiga', got: %v", report.VintageSystems)
	}

	// 2. Verify Unassigned Extra Tag
	if len(report.UnassignedExtraFields) == 0 || report.UnassignedExtraFields[0].Tag != 0xDEAD {
		t.Errorf("expected UnassignedExtraFields to contain 0xDEAD, got: %v", report.UnassignedExtraFields)
	}

	// 3. Verify Reserved Bit Flag warning
	foundReservedFlagWarn := false
	for _, issue := range report.Issues {
		if issue.Code == "WARN_RESERVED_BIT_FLAGS" {
			foundReservedFlagWarn = true
			break
		}
	}
	if !foundReservedFlagWarn {
		t.Errorf("expected WARN_RESERVED_BIT_FLAGS issue to be reported")
	}
}
