// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/internal/sys"
)

func TestLocalFileHeader_Roundtrip(t *testing.T) {
	header := zip.LocalFileHeader{
		VersionNeededToExtract: 20,
		GeneralPurposeBitFlag:  0x800, // UTF-8
		CompressionMethod:      8,
		LastModFileTime:        1234,
		LastModFileDate:        5678,
		CRC32:                  0xDEADBEEF,
		CompressedSize:         1024,
		UncompressedSize:       2048,
		FilenameLength:         8,
		ExtraFieldLength:       4,
		Filename:               "test.txt",
		ExtraField:             []byte{1, 2, 3, 4},
	}

	buf := header.AppendBytes(nil)
	require.GreaterOrEqual(t, len(buf), 30)

	// Skip signature (4 bytes) before passing to ReadLocalFileHeader
	r := bytes.NewReader(buf[4:])
	parsed, err := zip.ReadLocalFileHeader(r)
	require.NoError(t, err)

	assert.Equal(t, header.Filename, parsed.Filename)
	assert.Equal(t, header.CRC32, parsed.CRC32)
	assert.Equal(t, header.CompressedSize, parsed.CompressedSize)
	assert.Equal(t, header.UncompressedSize, parsed.UncompressedSize)
}

func TestEOCD_Roundtrip(t *testing.T) {
	encoded := zip.EncodeEOCD(42, 1024, 2048, "test archive comment")
	// Skip 4-byte signature
	r := bytes.NewReader(encoded[4:])
	eocd, err := zip.ReadEOCD(r)
	require.NoError(t, err)

	assert.Equal(t, uint16(42), eocd.EntriesNum)
	assert.Equal(t, uint32(1024), eocd.CentralDirSize)
	assert.Equal(t, uint32(2048), eocd.CentralDirOffset)
	assert.Equal(t, "test archive comment", eocd.Comment)
}

func TestNTFSExtraField_Roundtrip(t *testing.T) {
	meta := sys.Metadata{
		LastWriteTime:  132000000000000000,
		LastAccessTime: 132000000000000000,
		CreationTime:   132000000000000000,
	}
	encoded := zip.EncodeNTFSExtraField(meta)
	parsed := zip.ParseNTFSExtraField(encoded)

	assert.Equal(t, meta.LastWriteTime, parsed.LastWriteTime)
	assert.Equal(t, meta.CreationTime, parsed.CreationTime)
}
