// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ar_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/ar"
	"github.com/lemon4ksan/seal/file"
)

func TestAR_Roundtrip_DebianPackage(t *testing.T) {
	t.Parallel()

	f1, err := file.New("debian-binary", false)
	require.NoError(t, err)
	f1.WithData([]byte("2.0\n"))
	f1.WithModTime(time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC))

	f2, err := file.New("control.tar.gz", false)
	require.NoError(t, err)
	f2.WithData([]byte("GZIP_CONTROL_DATA_MOCK"))

	// Long filename (> 15 characters) testing GNU String Table ("//")
	f3, err := file.New("data_archive_payload.tar.xz", false)
	require.NoError(t, err)
	f3.WithData([]byte("XZ_PAYLOAD_DATA_MOCK_ODD_LENGTH!")) // 33 bytes (odd length testing '\n' padding)

	files := []*file.File{f1, f2, f3}

	eng := ar.New()
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, files, container.Config{})
	require.NoError(t, err)
	assert.Equal(t, int64(buf.Len()), written)

	// Read catalog
	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	foundDebianBin := false
	foundControl := false
	foundData := false

	for _, entry := range entries {
		r, err := entry.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(r)
		_ = r.Close()
		require.NoError(t, err)

		switch entry.Name() {
		case "debian-binary":
			foundDebianBin = true
			assert.Equal(t, "2.0\n", string(data))
			assert.Equal(t, 2026, entry.ModTime().Year())
		case "control.tar.gz":
			foundControl = true
			assert.Equal(t, "GZIP_CONTROL_DATA_MOCK", string(data))
		case "data_archive_payload.tar.xz":
			foundData = true
			assert.Equal(t, "XZ_PAYLOAD_DATA_MOCK_ODD_LENGTH!", string(data))
		}
	}

	assert.True(t, foundDebianBin, "debian-binary not found")
	assert.True(t, foundControl, "control.tar.gz not found")
	assert.True(t, foundData, "data_archive_payload.tar.xz not found")

	// Verify ScanStream
	streamEntries, _, err := eng.ScanStream(bytes.NewReader(buf.Bytes()), "", nil)
	require.NoError(t, err)
	assert.Len(t, streamEntries, 3)
}

func TestAR_CorruptedMagic(t *testing.T) {
	t.Parallel()

	corrupted := []byte("INVALID_NOT_AR_MAGIC_HEADER_DATA_12345678")
	eng := ar.New()
	_, _, err := eng.OpenCatalog(bytes.NewReader(corrupted), int64(len(corrupted)), "", nil)
	require.ErrorIs(t, err, ar.ErrInvalidMagic)
}
