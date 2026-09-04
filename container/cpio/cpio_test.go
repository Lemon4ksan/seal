// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cpio_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/cpio"
	"github.com/lemon4ksan/seal/file"
)

func TestCPIO_Roundtrip_Newc(t *testing.T) {
	t.Parallel()

	f1, err := file.New("init", false)
	require.NoError(t, err)
	f1.WithData([]byte("#!/bin/sh\necho Booting Linux\nexec /bin/sh\n"))
	f1.WithModTime(time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC))

	f2, err := file.New("etc/hostname", false)
	require.NoError(t, err)
	f2.WithData([]byte("seal-box\n"))

	files := []*file.File{f1, f2}

	eng := cpio.New()
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, files, container.Config{})
	require.NoError(t, err)
	assert.Equal(t, int64(buf.Len()), written)

	// 1. Parse via OpenCatalog
	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	foundInit := false
	foundHost := false

	for _, entry := range entries {
		r, err := entry.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(r)
		_ = r.Close()
		require.NoError(t, err)

		switch entry.Name() {
		case "init":
			foundInit = true
			assert.Equal(t, "#!/bin/sh\necho Booting Linux\nexec /bin/sh\n", string(data))
			assert.Equal(t, 2026, entry.ModTime().Year())
		case "etc/hostname":
			foundHost = true
			assert.Equal(t, "seal-box\n", string(data))
		}
	}

	assert.True(t, foundInit, "init not found")
	assert.True(t, foundHost, "hostname not found")

	// 2. Parse via ScanStream (pipes / non-seekable streams)
	streamEntries, _, err := eng.ScanStream(bytes.NewReader(buf.Bytes()), "", nil)
	require.NoError(t, err)
	assert.Len(t, streamEntries, 2)
}

func TestCPIO_Padding_Alignment(t *testing.T) {
	t.Parallel()

	// Test various payload lengths to ensure 4-byte padding works correctly
	for length := 1; length <= 16; length++ {
		t.Run(fmt.Sprintf("Length_%d", length), func(t *testing.T) {
			t.Parallel()

			payload := bytes.Repeat([]byte("A"), length)
			f, _ := file.New(fmt.Sprintf("file_%d.txt", length), false)
			f.WithData(payload)

			eng := cpio.New()
			var buf bytes.Buffer
			_, err := eng.WriteArchive(&buf, []*file.File{f}, container.Config{})
			require.NoError(t, err)

			entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
			require.NoError(t, err)
			assert.Len(t, entries, 1)

			r, err := entries[0].Open()
			require.NoError(t, err)
			data, err := io.ReadAll(r)
			_ = r.Close()
			require.NoError(t, err)

			assert.Equal(t, payload, data)
		})
	}
}

func TestCPIO_CorruptedMagic(t *testing.T) {
	t.Parallel()

	corrupted := []byte("000000INVALID_CPIO_DATA_NOT_MAGIC")
	eng := cpio.New()
	_, _, err := eng.OpenCatalog(bytes.NewReader(corrupted), int64(len(corrupted)), "", nil)
	require.ErrorIs(t, err, cpio.ErrInvalidMagic)
}
