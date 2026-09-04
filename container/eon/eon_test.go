// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eon

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

func TestEONRoundtrip(t *testing.T) {
	// Prepare test files
	f1, err := file.New("hello.txt", false)
	require.NoError(t, err)
	f1Content := []byte("Hello, EON Container Specification v1.0!")
	f1.WithData(f1Content)
	f1.WithModTime(time.Date(2026, 9, 3, 21, 0, 0, 0, time.UTC))

	f2, err := file.New("data/binary.bin", false)
	require.NoError(t, err)
	f2Content := make([]byte, 128*1024) // 128 KiB
	for i := range f2Content {
		f2Content[i] = byte(i % 251)
	}
	f2.WithData(f2Content)

	dir, err := file.New("data/", true)
	require.NoError(t, err)

	files := []*file.File{dir, f1, f2}

	// 1. Serialize archive
	var buf bytes.Buffer
	w := NewWriter(&buf)
	written, err := w.WriteArchive(files, container.Config{})
	require.NoError(t, err)

	archiveBytes := buf.Bytes()
	assert.Equal(t, written, int64(len(archiveBytes)))

	// 2. Verify binary mirror symmetry
	require.GreaterOrEqual(t, len(archiveBytes), HeaderSize+TrailerSize)
	// Check Header Magic ('EON!')
	assert.Equal(t, "EON!", string(archiveBytes[0:4]))
	// Check Trailer Magic ('!EON')
	assert.Equal(t, "!EON", string(archiveBytes[len(archiveBytes)-4:]))

	// 3. Test OpenCatalog (Random Access)
	rAt := bytes.NewReader(archiveBytes)
	reader, err := NewReader(rAt, int64(len(archiveBytes)))
	require.NoError(t, err)

	entries := reader.Entries()
	assert.Len(t, entries, 3)

	entryMap := make(map[string]*file.File)
	for _, entry := range entries {
		entryMap[entry.Name()] = entry
	}

	// Verify hello.txt
	helloEntry, ok := entryMap["hello.txt"]
	require.True(t, ok, "missing hello.txt entry")
	assert.Equal(t, int64(len(f1Content)), helloEntry.UncompressedSize())
	rc, err := helloEntry.Open()
	require.NoError(t, err)
	gotF1, err := io.ReadAll(rc)
	rc.Close()
	require.NoError(t, err)
	assert.Equal(t, f1Content, gotF1)

	// Verify binary.bin
	binEntry, ok := entryMap["data/binary.bin"]
	require.True(t, ok, "missing data/binary.bin entry")
	assert.Equal(t, int64(len(f2Content)), binEntry.UncompressedSize())
	rc2, err := binEntry.Open()
	require.NoError(t, err)
	gotF2, err := io.ReadAll(rc2)
	rc2.Close()
	require.NoError(t, err)
	assert.Equal(t, f2Content, gotF2)
}

func TestEONContainerEngine(t *testing.T) {
	eng, err := container.Get(container.Eon)
	require.NoError(t, err)
	assert.Equal(t, container.Eon, eng.Type())

	// Test Empty Archive
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, nil, container.Config{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, written, int64(HeaderSize+TrailerSize))

	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestEONDetect(t *testing.T) {
	assert.Equal(t, container.Eon, container.DetectExtension("archive.eon"))
	assert.Equal(t, container.Eon, container.DetectExtension("GAME.EON"))

	magic := []byte{'E', 'O', 'N', '!'}
	assert.Equal(t, container.Eon, container.SniffHeader(magic))
}

func TestEONFileSaveTo(t *testing.T) {
	tmpDir := t.TempDir()

	f, _ := file.New("nested/sub/hello.txt", false)
	f.WithData([]byte("SaveTo works natively with EON!"))

	var buf bytes.Buffer
	w := NewWriter(&buf)
	_, err := w.WriteArchive([]*file.File{f}, container.Config{})
	require.NoError(t, err)

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)

	entries := r.Entries()
	require.Len(t, entries, 1)

	outPath := filepath.Join(tmpDir, "extracted.txt")
	err = entries[0].SaveTo(outPath)
	require.NoError(t, err)

	readBack, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "SaveTo works natively with EON!", string(readBack))
}
