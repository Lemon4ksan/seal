// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

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
	require.NoError(t, err)

	data := buf.buf
	report, err := Inspect(bytes.NewReader(data), int64(len(data)), "")
	require.NoError(t, err)

	assert.True(t, report.SignatureValid)
	assert.True(t, report.StartHeaderCRCValid)
	assert.True(t, report.NextHeaderCRCValid)
	assert.Equal(t, 2, report.EntriesCount)
	assert.False(t, report.IsSFX)
	require.NotEmpty(t, report.Folders)
	require.NotEmpty(t, report.Folders[0].Coders)
	assert.Equal(t, "LZMA2", report.Folders[0].Coders[0].MethodName)
}

func TestSevenZip_Inspect_SFX_And_TrailingGarbage(t *testing.T) {
	var buf inMemoryWriteSeeker
	zw := NewWriter(&buf)

	f1, _ := file.New("test.txt", false)
	f1.WithData([]byte("SFX and trailing garbage test content"))

	_, err := zw.WriteArchive([]*file.File{f1})
	require.NoError(t, err)

	rawArchive := buf.buf

	// Prepend 512 bytes SFX stub and append 256 bytes trailing garbage
	sfxStub := bytes.Repeat([]byte{0x90}, 512)
	trailingGarbage := bytes.Repeat([]byte{0xDE, 0xAD}, 128)

	var modified []byte
	modified = append(modified, sfxStub...)
	modified = append(modified, rawArchive...)
	modified = append(modified, trailingGarbage...)

	report, err := Inspect(bytes.NewReader(modified), int64(len(modified)), "")
	require.NoError(t, err)

	assert.True(t, report.IsSFX)
	assert.Equal(t, int64(512), report.SFXStubSize)
	assert.Equal(t, int64(256), report.TrailingGarbageSize)

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

	assert.True(t, hasSFXIssue, "missing INFO_SFX_STUB_DETECTED issue")
	assert.True(t, hasGarbageIssue, "missing WARN_TRAILING_GARBAGE issue")
}

func TestSevenZip_Inspect_ZipSlip_SecurityHazard(t *testing.T) {
	var buf inMemoryWriteSeeker
	zw := NewWriter(&buf)

	malicious, _ := file.New("shadow", false)
	malicious.WithRawName("../../etc/shadow")
	malicious.WithData([]byte("root:$6$malicious_hash"))

	_, err := zw.WriteArchive([]*file.File{malicious})
	require.NoError(t, err)

	data := buf.buf
	report, err := Inspect(bytes.NewReader(data), int64(len(data)), "")
	require.NoError(t, err)

	hasZipSlip := false
	for _, issue := range report.Issues {
		if issue.Code == "SEC_ZIP_SLIP_HAZARD" && issue.Severity == SeveritySecurityHazard {
			hasZipSlip = true
			break
		}
	}

	assert.True(t, hasZipSlip, "expected SEC_ZIP_SLIP_HAZARD issue to be detected")
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
	require.NoError(t, err)

	data := buf.buf

	// Inspect with correct password
	report, err := Inspect(bytes.NewReader(data), int64(len(data)), pwd)
	require.NoError(t, err)

	assert.True(t, report.IsEncodedHeader)
	assert.True(t, report.IsEncryptedHeader)
	assert.True(t, report.HasEncryptedStreams)
	assert.Equal(t, 1, report.EntriesCount)
}
