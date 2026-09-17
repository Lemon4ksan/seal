// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"hash/crc32"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/file"
)

func TestDoctor_SalvageMissingCentralDirectory(t *testing.T) {
	// 1. Create a standard ZIP archive with 2 files
	zipBuf := new(bytes.Buffer)
	w := zip.NewWriter(zipBuf, codec.DefaultRegistry)

	f1, _ := file.New("file1.txt", false)
	content1 := []byte("Content of the first salvaged file.")
	_ = w.WriteEntry(f1, bytes.NewReader(content1))

	f2, _ := file.New("file2.log", false)
	content2 := []byte("Content of the second salvaged file log stream.")
	_ = w.WriteEntry(f2, bytes.NewReader(content2))

	_ = w.Close()

	// 2. Locate Central Directory offset and deliberately truncate the archive there
	// (Simulates a file where the Central Directory and EOCD were wiped / truncated)
	r, err := zip.NewReader(bytes.NewReader(zipBuf.Bytes()), int64(zipBuf.Len()), "", codec.DefaultRegistry)
	require.NoError(t, err)

	// Truncate at Central Directory start
	firstEntryOffset := r.Entries()[0].LocalHeaderOffset()
	lastEntry := r.Entries()[1]
	truncatedLen := lastEntry.LocalHeaderOffset() + 30 + int64(len(lastEntry.Name())) + lastEntry.CompressedSize()

	brokenData := zipBuf.Bytes()[:truncatedLen]

	// 3. Normal reader MUST fail on broken data
	_, err = zip.NewReader(bytes.NewReader(brokenData), int64(len(brokenData)), "", codec.DefaultRegistry)
	require.Error(t, err)

	// 4. Run Archive Doctor to reconstruct the Central Directory via linear scan
	repairedBuf := new(bytes.Buffer)
	summary, err := zip.Repair(bytes.NewReader(brokenData), int64(len(brokenData)), repairedBuf, zip.RepairOptions{
		RebuildCentralDirectory: true,
		SanitizePaths:           true,
	})
	require.NoError(t, err)

	assert.True(t, summary.CentralDirectoryRebuilt)
	assert.Equal(t, 2, summary.FilesRecovered)

	// 5. Open the repaired archive with standard Reader and verify file contents
	repairedReader, err := zip.NewReader(
		bytes.NewReader(repairedBuf.Bytes()),
		int64(repairedBuf.Len()),
		"",
		codec.DefaultRegistry,
	)
	require.NoError(t, err)

	e1, ok1 := repairedReader.Entry("file1.txt")
	require.True(t, ok1)
	rc1, err := e1.Open()
	require.NoError(t, err)
	c1, err := io.ReadAll(rc1)
	_ = rc1.Close()
	require.NoError(t, err)
	assert.Equal(t, content1, c1)

	e2, ok2 := repairedReader.Entry("file2.log")
	require.True(t, ok2)
	rc2, err := e2.Open()
	require.NoError(t, err)
	c2, err := io.ReadAll(rc2)
	_ = rc2.Close()
	require.NoError(t, err)
	assert.Equal(t, content2, c2)
	_ = firstEntryOffset
}

func TestDoctor_SanitizeZipSlipPaths(t *testing.T) {
	buf := new(bytes.Buffer)
	content := []byte("#!/bin/sh\necho exploit")
	name := "../../malicious/path.sh"

	realCRC := crc32.ChecksumIEEE(content)
	lfh := zip.LocalFileHeader{
		VersionNeededToExtract: zip.VersionDefault,
		CompressionMethod:      zip.MethodStore,
		CRC32:                  realCRC,
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
		CRC32:                  realCRC,
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

	repairedBuf := new(bytes.Buffer)
	summary, err := zip.Repair(bytes.NewReader(buf.Bytes()), int64(buf.Len()), repairedBuf, zip.RepairOptions{
		SanitizePaths: true,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, summary.PathsSanitized)

	repairedReader, err := zip.NewReader(
		bytes.NewReader(repairedBuf.Bytes()),
		int64(repairedBuf.Len()),
		"",
		codec.DefaultRegistry,
	)
	require.NoError(t, err)

	_, foundMalicious := repairedReader.Entry("../../malicious/path.sh")
	assert.False(t, foundMalicious)

	_, foundSanitized := repairedReader.Entry("malicious/path.sh")
	assert.True(t, foundSanitized)
}
