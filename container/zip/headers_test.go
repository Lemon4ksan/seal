// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"testing"

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
	if len(buf) < 30 {
		t.Fatalf("encoded buffer too short: %d", len(buf))
	}

	// Skip signature (4 bytes) before passing to ReadLocalFileHeader
	r := bytes.NewReader(buf[4:])
	parsed, err := zip.ReadLocalFileHeader(r)
	if err != nil {
		t.Fatalf("ReadLocalFileHeader failed: %v", err)
	}

	if parsed.Filename != header.Filename {
		t.Errorf("filename mismatch: got %q, want %q", parsed.Filename, header.Filename)
	}
	if parsed.CRC32 != header.CRC32 {
		t.Errorf("CRC32 mismatch: got %x, want %x", parsed.CRC32, header.CRC32)
	}
	if parsed.CompressedSize != header.CompressedSize {
		t.Errorf("CompressedSize mismatch: got %d, want %d", parsed.CompressedSize, header.CompressedSize)
	}
	if parsed.UncompressedSize != header.UncompressedSize {
		t.Errorf("UncompressedSize mismatch: got %d, want %d", parsed.UncompressedSize, header.UncompressedSize)
	}
}

func TestEOCD_Roundtrip(t *testing.T) {
	encoded := zip.EncodeEOCD(42, 1024, 2048, "test archive comment")
	// Skip 4-byte signature
	r := bytes.NewReader(encoded[4:])
	eocd, err := zip.ReadEOCD(r)
	if err != nil {
		t.Fatalf("ReadEOCD failed: %v", err)
	}

	if eocd.EntriesNum != 42 {
		t.Errorf("EntriesNum mismatch: got %d, want 42", eocd.EntriesNum)
	}
	if eocd.CentralDirSize != 1024 {
		t.Errorf("CentralDirSize mismatch: got %d, want 1024", eocd.CentralDirSize)
	}
	if eocd.CentralDirOffset != 2048 {
		t.Errorf("CentralDirOffset mismatch: got %d, want 2048", eocd.CentralDirOffset)
	}
	if eocd.Comment != "test archive comment" {
		t.Errorf("Comment mismatch: got %q", eocd.Comment)
	}
}

func TestNTFSExtraField_Roundtrip(t *testing.T) {
	meta := sys.Metadata{
		LastWriteTime:  132000000000000000,
		LastAccessTime: 132000000000000000,
		CreationTime:   132000000000000000,
	}
	encoded := zip.EncodeNTFSExtraField(meta)
	parsed := zip.ParseNTFSExtraField(encoded)

	if parsed.LastWriteTime != meta.LastWriteTime {
		t.Errorf("LastWriteTime mismatch: got %d, want %d", parsed.LastWriteTime, meta.LastWriteTime)
	}
	if parsed.CreationTime != meta.CreationTime {
		t.Errorf("CreationTime mismatch: got %d, want %d", parsed.CreationTime, meta.CreationTime)
	}
}
