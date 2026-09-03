// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rar_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/rar"
)

func createTestRAR4Archive(filename string, content []byte) []byte {
	var buf bytes.Buffer

	// 1. Signature (7 bytes: "Rar!\x1a\x07\x00")
	buf.Write([]byte{0x52, 0x61, 0x72, 0x21, 0x1a, 0x07, 0x00})

	// 2. Main Archive Header (blockArc = 0x73, size = 13 bytes)
	var arcHdr [13]byte
	arcHdr[2] = 0x73                                   // blockArc
	binary.LittleEndian.PutUint16(arcHdr[3:5], 0x0000) // flags
	binary.LittleEndian.PutUint16(arcHdr[5:7], 13)     // size = 13
	crcArc := uint16(crc32.ChecksumIEEE(arcHdr[2:13]))
	binary.LittleEndian.PutUint16(arcHdr[0:2], crcArc)
	buf.Write(arcHdr[:])

	// 3. File Header (blockFile = 0x74)
	nameBytes := []byte(filename)
	packSize := uint32(len(content))
	unpackSize := uint32(len(content))
	fileCRC := crc32.ChecksumIEEE(content)
	nameSize := uint16(len(nameBytes))

	headSize := uint16(7 + 4 + 4 + 1 + 4 + 4 + 1 + 1 + 2 + 4 + len(nameBytes))
	fileHdr := make([]byte, headSize)
	fileHdr[2] = 0x74                                   // blockFile
	binary.LittleEndian.PutUint16(fileHdr[3:5], 0x8000) // blockHasData
	binary.LittleEndian.PutUint16(fileHdr[5:7], headSize)

	binary.LittleEndian.PutUint32(fileHdr[7:11], packSize)
	binary.LittleEndian.PutUint32(fileHdr[11:15], unpackSize)
	fileHdr[15] = 0 // HostOS MS-DOS
	binary.LittleEndian.PutUint32(fileHdr[16:20], fileCRC)
	binary.LittleEndian.PutUint32(fileHdr[20:24], 0x21002100) // DOS time
	fileHdr[24] = 29                                          // UnpVer 2.9
	fileHdr[25] = 0x30                                        // Method: 0x30 (Store / no compression)
	binary.LittleEndian.PutUint16(fileHdr[26:28], nameSize)
	binary.LittleEndian.PutUint32(fileHdr[28:32], 0x20) // FileAttr (Archive)
	copy(fileHdr[32:], nameBytes)

	crcFile := uint16(crc32.ChecksumIEEE(fileHdr[2:]))
	binary.LittleEndian.PutUint16(fileHdr[0:2], crcFile)

	buf.Write(fileHdr)
	buf.Write(content)

	// 4. End of Archive block (blockEnd = 0x7b, size = 7 bytes)
	var endHdr [7]byte
	endHdr[2] = 0x7b                                   // blockEnd
	binary.LittleEndian.PutUint16(endHdr[3:5], 0x0000) // flags
	binary.LittleEndian.PutUint16(endHdr[5:7], 7)      // size = 7
	crcEnd := uint16(crc32.ChecksumIEEE(endHdr[2:]))
	binary.LittleEndian.PutUint16(endHdr[0:2], crcEnd)
	buf.Write(endHdr[:])

	return buf.Bytes()
}

func TestRAR_SniffHeader(t *testing.T) {
	// Test RAR4 magic
	rar4Magic := []byte{0x52, 0x61, 0x72, 0x21, 0x1a, 0x07, 0x00}
	if tpe := container.SniffHeader(rar4Magic); tpe != container.Rar {
		t.Fatalf("SniffHeader for RAR4: got %v, want %v", tpe, container.Rar)
	}

	// Test RAR5 magic
	rar5Magic := []byte{0x52, 0x61, 0x72, 0x21, 0x1a, 0x07, 0x01, 0x00}
	if tpe := container.SniffHeader(rar5Magic); tpe != container.Rar {
		t.Fatalf("SniffHeader for RAR5: got %v, want %v", tpe, container.Rar)
	}
}

func TestRAR_WriteArchive_Rejection(t *testing.T) {
	eng := rar.New()
	if eng.Type() != container.Rar {
		t.Fatalf("Expected type %v, got %v", container.Rar, eng.Type())
	}

	// Attempting to write a RAR archive must return ErrCreateNotSupported
	_, err := eng.WriteArchive(&bytes.Buffer{}, nil, container.Config{})
	if !errors.Is(err, rar.ErrCreateNotSupported) {
		t.Fatalf("Expected ErrCreateNotSupported, got: %v", err)
	}
}

func TestRAR_ReadCatalog_And_Extraction(t *testing.T) {
	expectedData := []byte("Hello, this is a test file extracted from a pure Go UnRAR container in Seal!")
	filename := "documents/readme.txt"

	rarBytes := createTestRAR4Archive(filename, expectedData)

	// Test OpenCatalog on Engine
	eng, err := container.Get(container.Rar)
	if err != nil {
		t.Fatalf("container.Get(container.Rar) failed: %v", err)
	}

	rAt := bytes.NewReader(rarBytes)
	entries, _, err := eng.OpenCatalog(rAt, int64(len(rarBytes)), "", nil)
	if err != nil {
		t.Fatalf("OpenCatalog failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.Name() != filename {
		t.Fatalf("Entry name mismatch: got %q, want %q", entry.Name(), filename)
	}
	if entry.UncompressedSize() != int64(len(expectedData)) {
		t.Fatalf("Size mismatch: got %d, want %d", entry.UncompressedSize(), len(expectedData))
	}

	// Test Open() and data read
	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("entry.Open() failed: %v", err)
	}
	defer func() { _ = rc.Close() }()

	extracted, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Reading entry stream: %v", err)
	}

	if !bytes.Equal(extracted, expectedData) {
		t.Fatalf("Extracted content mismatch: got %q, want %q", string(extracted), string(expectedData))
	}

	// Test ScanStream
	streamEntries, _, err := eng.ScanStream(bytes.NewReader(rarBytes), "", nil)
	if err != nil {
		t.Fatalf("ScanStream failed: %v", err)
	}
	if len(streamEntries) != 1 {
		t.Fatalf("ScanStream expected 1 entry, got %d", len(streamEntries))
	}
	if streamEntries[0].Name() != filename {
		t.Fatalf("ScanStream name mismatch: got %q, want %q", streamEntries[0].Name(), filename)
	}
}
