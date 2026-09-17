// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package iso_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/iso"
	"github.com/lemon4ksan/seal/file"
)

func TestISO_Roundtrip_Joliet(t *testing.T) {
	t.Parallel()

	f1, err := file.New("boot/grub/grub.cfg", false)
	require.NoError(t, err)
	f1.WithData([]byte("set default=0\nmenuentry \"Linux\" {}\n"))
	f1.WithModTime(time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC))

	f2, err := file.New("documents/русский_текст.txt", false)
	require.NoError(t, err)
	f2.WithData([]byte("Привет, мир! ISO 9660 с поддержкой Joliet Unicode."))

	files := []*file.File{f1, f2}

	eng := iso.New()
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, files, container.Config{})
	require.NoError(t, err)
	assert.Equal(t, int64(buf.Len()), written)

	// Must be multiple of 2048
	assert.Equal(t, 0, buf.Len()%iso.SectorSize, "ISO image size must be multiple of 2048")

	// Open and inspect catalog
	entries, volID, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, volID, "Expected non-empty VolumeID")

	foundGrub := false
	foundUnicode := false

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		r, err := entry.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(r)
		_ = r.Close()
		require.NoError(t, err)

		switch entry.Name() {
		case "boot/grub/grub.cfg":
			foundGrub = true
			assert.Equal(t, "set default=0\nmenuentry \"Linux\" {}\n", string(data))
		case "documents/русский_текст.txt":
			foundUnicode = true
			assert.Equal(t, "Привет, мир! ISO 9660 с поддержкой Joliet Unicode.", string(data))
		}
	}

	assert.True(t, foundGrub, "grub.cfg not found in extracted entries")
	assert.True(t, foundUnicode, "Unicode file not found in extracted entries")
}

func TestISO_CorruptedImage(t *testing.T) {
	t.Parallel()

	shortBytes := make([]byte, 1000)
	eng := iso.New()
	_, _, err := eng.OpenCatalog(bytes.NewReader(shortBytes), int64(len(shortBytes)), "", nil)
	require.ErrorIs(t, err, iso.ErrInvalidISO)
}
