// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"compress/flate"
	"hash/crc32"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/container/zip"
)

func TestStreamReader_WithDataDescriptor_WithSignature(t *testing.T) {
	// Build a valid raw streaming ZIP entry with Bit 3 Data Descriptor
	var buf bytes.Buffer

	data := []byte("Hello Stream World with Data Descriptor!")
	crc := crc32.ChecksumIEEE(data)

	// 1. Deflate data
	var deflated bytes.Buffer
	zw, _ := flate.NewWriter(&deflated, flate.DefaultCompression)
	zw.Write(data)
	zw.Close()
	compData := deflated.Bytes()

	// 2. Local File Header with CRC=0, Sizes=0, Bit 3 set
	lfh := zip.LocalFileHeader{
		VersionNeededToExtract: 20,
		GeneralPurposeBitFlag:  zip.FlagDataDescriptor | zip.FlagLanguageEncodingUTF8,
		CompressionMethod:      zip.MethodDeflate,
		FilenameLength:         uint16(len("stream.txt")),
		Filename:               "stream.txt",
	}
	buf.Write(lfh.AppendBytes(nil))

	// 3. Payload
	buf.Write(compData)

	// 4. Data Descriptor with 0x08074b50 signature
	buf.Write(zip.EncodeDataDescriptor(crc, int64(len(compData)), int64(len(data))))

	// Central Directory to close stream
	cd := zip.CentralDirectory{
		FilenameLength:   uint16(len("stream.txt")),
		Filename:         "stream.txt",
		CRC32:            crc,
		CompressedSize:   uint32(len(compData)),
		UncompressedSize: uint32(len(data)),
	}
	buf.Write(cd.AppendBytes(nil))
	buf.Write(zip.EncodeEOCD(1, int64(46+len("stream.txt")), int64(buf.Len()), ""))

	// Parse with StreamReader
	sr := zip.NewStreamReader(&buf, "", nil)

	count := 0
	for fe, r := range sr.All() {
		count++
		if fe.Name() != "stream.txt" {
			t.Errorf("filename mismatch: got %q, want stream.txt", fe.Name())
		}
		readData, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("read payload failed: %v", err)
		}
		if !bytes.Equal(readData, data) {
			t.Fatalf("content mismatch: got %q, want %q", string(readData), string(data))
		}
		// Check that Data Descriptor updated entry sizes and CRC
		if fe.CRC() != crc {
			t.Errorf("CRC mismatch after reading: got %x, want %x", fe.CRC(), crc)
		}
		if fe.UncompressedSize() != int64(len(data)) {
			t.Errorf("UncompressedSize mismatch: got %d, want %d", fe.UncompressedSize(), len(data))
		}
	}

	if count != 1 {
		t.Fatalf("expected 1 streamed file, got %d", count)
	}
}

func TestStreamReader_WithDataDescriptor_NoSignature(t *testing.T) {
	var buf bytes.Buffer

	data := []byte("Hello from Info-ZIP / Java style descriptor without PK0708 signature!")
	crc := crc32.ChecksumIEEE(data)

	var deflated bytes.Buffer
	zw, _ := flate.NewWriter(&deflated, flate.DefaultCompression)
	zw.Write(data)
	zw.Close()
	compData := deflated.Bytes()

	// Local File Header with Bit 3 set
	lfh := zip.LocalFileHeader{
		VersionNeededToExtract: 20,
		GeneralPurposeBitFlag:  zip.FlagDataDescriptor,
		CompressionMethod:      zip.MethodDeflate,
		FilenameLength:         uint16(len("nosig.txt")),
		Filename:               "nosig.txt",
	}
	buf.Write(lfh.AppendBytes(nil))
	buf.Write(compData)

	// 12-byte raw descriptor (no signature)
	// [CRC (4B)][Comp (4B)][Uncomp (4B)]
	rawDD := make([]byte, 12)
	rawDD[0] = byte(crc)
	rawDD[1] = byte(crc >> 8)
	rawDD[2] = byte(crc >> 16)
	rawDD[3] = byte(crc >> 24)
	compSize32 := uint32(len(compData))
	rawDD[4] = byte(compSize32)
	rawDD[5] = byte(compSize32 >> 8)
	rawDD[6] = byte(compSize32 >> 16)
	rawDD[7] = byte(compSize32 >> 24)
	uncompSize32 := uint32(len(data))
	rawDD[8] = byte(uncompSize32)
	rawDD[9] = byte(uncompSize32 >> 8)
	rawDD[10] = byte(uncompSize32 >> 16)
	rawDD[11] = byte(uncompSize32 >> 24)
	buf.Write(rawDD)

	// Central Directory
	cd := zip.CentralDirectory{
		FilenameLength: uint16(len("nosig.txt")),
		Filename:       "nosig.txt",
	}
	buf.Write(cd.AppendBytes(nil))

	sr := zip.NewStreamReader(&buf, "", nil)
	fe, r, err := sr.Next()
	if err != nil {
		t.Fatalf("sr.Next() failed: %v", err)
	}
	if fe.Name() != "nosig.txt" {
		t.Errorf("name mismatch: got %q, want nosig.txt", fe.Name())
	}
	readData, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("io.ReadAll failed: %v", err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatalf("data mismatch: got %q, want %q", string(readData), string(data))
	}
	if fe.CRC() != crc {
		t.Errorf("CRC mismatch: got %x, want %x", fe.CRC(), crc)
	}
}
