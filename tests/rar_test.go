// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"testing"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/rar"
)

func createTestRARArchive(filename string, content []byte) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x52, 0x61, 0x72, 0x21, 0x1a, 0x07, 0x00})

	var arcHdr [13]byte
	arcHdr[2] = 0x73
	binary.LittleEndian.PutUint16(arcHdr[3:5], 0x0000)
	binary.LittleEndian.PutUint16(arcHdr[5:7], 13)
	crcArc := uint16(crc32.ChecksumIEEE(arcHdr[2:13]))
	binary.LittleEndian.PutUint16(arcHdr[0:2], crcArc)
	buf.Write(arcHdr[:])

	nameBytes := []byte(filename)
	packSize := uint32(len(content))
	unpackSize := uint32(len(content))
	fileCRC := crc32.ChecksumIEEE(content)
	nameSize := uint16(len(nameBytes))

	headSize := uint16(7 + 4 + 4 + 1 + 4 + 4 + 1 + 1 + 2 + 4 + len(nameBytes))
	fileHdr := make([]byte, headSize)
	fileHdr[2] = 0x74
	binary.LittleEndian.PutUint16(fileHdr[3:5], 0x8000)
	binary.LittleEndian.PutUint16(fileHdr[5:7], headSize)
	binary.LittleEndian.PutUint32(fileHdr[7:11], packSize)
	binary.LittleEndian.PutUint32(fileHdr[11:15], unpackSize)
	fileHdr[15] = 0
	binary.LittleEndian.PutUint32(fileHdr[16:20], fileCRC)
	binary.LittleEndian.PutUint32(fileHdr[20:24], 0x21002100)
	fileHdr[24] = 29
	fileHdr[25] = 0x30
	binary.LittleEndian.PutUint16(fileHdr[26:28], nameSize)
	binary.LittleEndian.PutUint32(fileHdr[28:32], 0x20)
	copy(fileHdr[32:], nameBytes)
	crcFile := uint16(crc32.ChecksumIEEE(fileHdr[2:]))
	binary.LittleEndian.PutUint16(fileHdr[0:2], crcFile)
	buf.Write(fileHdr)
	buf.Write(content)

	var endHdr [7]byte
	endHdr[2] = 0x7b
	binary.LittleEndian.PutUint16(endHdr[3:5], 0x0000)
	binary.LittleEndian.PutUint16(endHdr[5:7], 7)
	crcEnd := uint16(crc32.ChecksumIEEE(endHdr[2:]))
	binary.LittleEndian.PutUint16(endHdr[0:2], crcEnd)
	buf.Write(endHdr[:])

	return buf.Bytes()
}

func TestRAR_DefaultEngine_Integration(t *testing.T) {
	t.Parallel()
	eng, err := seal.DefaultContainer(container.Rar)
	if err != nil {
		t.Fatalf("DefaultEngine(container.Rar) failed: %v", err)
	}
	if eng.Type() != container.Rar {
		t.Fatalf("Expected type %v, got %v", container.Rar, eng.Type())
	}

	payload := []byte("Testing seamless UnRAR extraction through seal.Archive facade!")
	archiveBytes := createTestRARArchive("notes/todo.txt", payload)

	arc := seal.NewArchive()
	rAt := bytes.NewReader(archiveBytes)
	files, err := arc.Load(rAt, int64(len(archiveBytes)), eng)
	if err != nil {
		t.Fatalf("arc.Load with RAR engine failed: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(files))
	}

	rc, err := files[0].Open()
	if err != nil {
		t.Fatalf("files[0].Open() failed: %v", err)
	}
	defer func() { _ = rc.Close() }()

	extracted, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Reading extracted data: %v", err)
	}
	if !bytes.Equal(extracted, payload) {
		t.Fatalf("Extracted mismatch: got %q, want %q", string(extracted), string(payload))
	}

	// Verify write rejection
	var outBuf bytes.Buffer
	_, err = eng.WriteArchive(&outBuf, files, container.Config{})
	if !errors.Is(err, rar.ErrCreateNotSupported) {
		t.Fatalf("Expected ErrCreateNotSupported, got %v", err)
	}
}
