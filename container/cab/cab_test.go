// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cab_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/cab"
	"github.com/lemon4ksan/seal/file"
)

func TestCAB_Roundtrip_MSZip(t *testing.T) {
	t.Parallel()

	f1, err := file.New("documents/readme.txt", false)
	require.NoError(t, err)
	f1.WithData([]byte("Welcome to Seal CAB engine!\n"))
	f1.WithModTime(time.Date(2026, 9, 3, 14, 30, 0, 0, time.UTC))

	f2, err := file.New("config.json", false)
	require.NoError(t, err)
	f2.WithData([]byte(`{"engine":"seal","format":"cab"}`))

	files := []*file.File{f1, f2}

	eng := cab.New()
	var buf bytes.Buffer
	written, err := eng.WriteArchive(&buf, files, container.Config{})
	require.NoError(t, err)
	assert.Equal(t, int64(buf.Len()), written)

	// Read catalog
	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// Verify entries and data
	for _, entry := range entries {
		r, err := entry.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(r)
		_ = r.Close()
		require.NoError(t, err)

		switch entry.Name() {
		case "documents/readme.txt":
			assert.Equal(t, "Welcome to Seal CAB engine!\n", string(data))
			assert.Equal(t, 2026, entry.ModTime().Year())
		case "config.json":
			assert.Equal(t, `{"engine":"seal","format":"cab"}`, string(data))
		default:
			t.Fatalf("Unexpected entry: %q", entry.Name())
		}
	}
}

func TestCAB_Roundtrip_Store(t *testing.T) {
	t.Parallel()

	f, err := file.New("plain.txt", false)
	require.NoError(t, err)
	f.WithData([]byte("Uncompressed plain data inside Cabinet"))

	eng := cab.New()
	var buf bytes.Buffer
	_, err = eng.WriteArchive(&buf, []*file.File{f}, container.Config{DefaultMethod: codec.Store})
	require.NoError(t, err)

	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	r, err := entries[0].Open()
	require.NoError(t, err)
	data, err := io.ReadAll(r)
	_ = r.Close()
	require.NoError(t, err)

	assert.Equal(t, "Uncompressed plain data inside Cabinet", string(data))
}

func TestCAB_MultiBlock_Spanning(t *testing.T) {
	t.Parallel()

	// Create payload larger than 32KB block (e.g. 100 KB of repetitive text)
	largeData := bytes.Repeat([]byte("Microsoft Cabinet 32KB chunk boundary test. "), 2500) // ~110 KB

	f, err := file.New("large.bin", false)
	require.NoError(t, err)
	f.WithData(largeData)

	eng := cab.New()
	var buf bytes.Buffer
	_, err = eng.WriteArchive(&buf, []*file.File{f}, container.Config{})
	require.NoError(t, err)

	entries, _, err := eng.OpenCatalog(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", nil)
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	r, err := entries[0].Open()
	require.NoError(t, err)
	data, err := io.ReadAll(r)
	_ = r.Close()
	require.NoError(t, err)

	assert.Equal(t, largeData, data)
}

func TestCAB_CorruptedSignature(t *testing.T) {
	t.Parallel()

	invalidBytes := []byte("INVALID_HEADER_DATA_NOT_MSCF_HEADER")
	eng := cab.New()
	_, _, err := eng.OpenCatalog(bytes.NewReader(invalidBytes), int64(len(invalidBytes)), "", nil)
	require.ErrorIs(t, err, cab.ErrInvalidSignature)
}
