// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/seal/file"
)

func TestSevenZip_Inspect_CleanArchive(t *testing.T) {
	var buf inMemoryWriteSeeker
	zw := NewWriter(&buf)

	f1, _ := file.New("hello.txt", false)
	f1.WithData([]byte("Hello world from 7z inspector test!"))
	f2, _ := file.New("dir/sub.txt", false)
	f2.WithData([]byte("Secondary sub-entry"))

	_, err := zw.WriteArchive([]*file.File{f1, f2})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}

	data := buf.buf
	report, err := Inspect(bytes.NewReader(data), int64(len(data)), "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !report.SignatureValid {
		t.Errorf("expected signature to be valid")
	}
	if !report.StartHeaderCRCValid {
		t.Errorf("expected start header CRC to be valid")
	}
	if !report.NextHeaderCRCValid {
		t.Errorf("expected next header CRC to be valid")
	}
	if report.EntriesCount != 2 {
		t.Errorf("expected 2 entries, got %d", report.EntriesCount)
	}
	if report.IsSFX {
		t.Errorf("expected IsSFX to be false")
	}
	if len(report.Folders) == 0 {
		t.Errorf("expected at least 1 folder")
	}
	if len(report.Folders[0].Coders) == 0 || report.Folders[0].Coders[0].MethodName != "LZMA2" {
		t.Errorf("expected folder coder to be LZMA2")
	}
}

func TestSevenZip_Inspect_SFX_And_TrailingGarbage(t *testing.T) {
	var buf inMemoryWriteSeeker
	zw := NewWriter(&buf)

	f1, _ := file.New("test.txt", false)
	f1.WithData([]byte("SFX and trailing garbage test content"))

	_, err := zw.WriteArchive([]*file.File{f1})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}

	rawArchive := buf.buf

	// Prepend 512 bytes SFX stub and append 256 bytes trailing garbage
	sfxStub := bytes.Repeat([]byte{0x90}, 512)
	trailingGarbage := bytes.Repeat([]byte{0xDE, 0xAD}, 128)

	var modified []byte
	modified = append(modified, sfxStub...)
	modified = append(modified, rawArchive...)
	modified = append(modified, trailingGarbage...)

	report, err := Inspect(bytes.NewReader(modified), int64(len(modified)), "")
	if err != nil {
		t.Fatalf("Inspect on modified archive failed: %v", err)
	}

	if !report.IsSFX {
		t.Errorf("expected IsSFX to be true")
	}
	if report.SFXStubSize != 512 {
		t.Errorf("expected SFXStubSize to be 512, got %d", report.SFXStubSize)
	}
	if report.TrailingGarbageSize != 256 {
		t.Errorf("expected TrailingGarbageSize to be 256, got %d", report.TrailingGarbageSize)
	}

	hasSFXIssue := false
	hasGarbageIssue := false
	for _, issue := range report.Issues {
		if issue.Code == "INFO_SFX_STUB_DETECTED" {
			hasSFXIssue = true
		}
		if issue.Code == "WARN_TRAILING_GARBAGE" {
			hasGarbageIssue = true
		}
	}

	if !hasSFXIssue {
		t.Errorf("missing INFO_SFX_STUB_DETECTED issue")
	}
	if !hasGarbageIssue {
		t.Errorf("missing WARN_TRAILING_GARBAGE issue")
	}
}

func TestSevenZip_Inspect_ZipSlip_SecurityHazard(t *testing.T) {
	var buf inMemoryWriteSeeker
	zw := NewWriter(&buf)

	malicious, _ := file.New("shadow", false)
	malicious.WithRawName("../../etc/shadow")
	malicious.WithData([]byte("root:$6$malicious_hash"))

	_, err := zw.WriteArchive([]*file.File{malicious})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}

	data := buf.buf
	report, err := Inspect(bytes.NewReader(data), int64(len(data)), "")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	hasZipSlip := false
	for _, issue := range report.Issues {
		if issue.Code == "SEC_ZIP_SLIP_HAZARD" && issue.Severity == SeveritySecurityHazard {
			hasZipSlip = true
			break
		}
	}

	if !hasZipSlip {
		t.Errorf("expected SEC_ZIP_SLIP_HAZARD issue to be detected")
	}
}

func TestSevenZip_Inspect_EncryptedArchive(t *testing.T) {
	pwd := "InspectPass!2026"
	var buf inMemoryWriteSeeker
	zw := NewWriterWithOptions(&buf, WriterOptions{
		Password:       pwd,
		EncryptHeaders: true,
	})

	f1, _ := file.New("secure.dat", false)
	f1.WithData([]byte("Confidential data protected by 7z AES-256"))

	_, err := zw.WriteArchive([]*file.File{f1})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}

	data := buf.buf

	// Inspect with correct password
	report, err := Inspect(bytes.NewReader(data), int64(len(data)), pwd)
	if err != nil {
		t.Fatalf("Inspect with password failed: %v", err)
	}

	if !report.IsEncodedHeader {
		t.Errorf("expected IsEncodedHeader to be true")
	}
	if !report.IsEncryptedHeader {
		t.Errorf("expected IsEncryptedHeader to be true")
	}
	if !report.HasEncryptedStreams {
		t.Errorf("expected HasEncryptedStreams to be true")
	}
	if report.EntriesCount != 1 {
		t.Errorf("expected 1 entry, got %d", report.EntriesCount)
	}
}
