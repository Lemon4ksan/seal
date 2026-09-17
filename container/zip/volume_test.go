// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/file"
)

type memVolume struct {
	buf bytes.Buffer
}

func (m *memVolume) Write(p []byte) (int, error) {
	return m.buf.Write(p)
}

func (m *memVolume) Close() error {
	return nil
}

func TestMultiVolumeReader_BoundaryCrossing(t *testing.T) {
	seg1 := []byte("0123456789") // 10 bytes
	seg2 := []byte("ABCDEFGHIJ") // 10 bytes
	seg3 := []byte("klmnopqrst") // 10 bytes

	volumes := []zip.Volume{
		{DiskIndex: 0, Reader: bytes.NewReader(seg1), Size: int64(len(seg1))},
		{DiskIndex: 1, Reader: bytes.NewReader(seg2), Size: int64(len(seg2))},
		{DiskIndex: 2, Reader: bytes.NewReader(seg3), Size: int64(len(seg3))},
	}

	mvr, err := zip.NewMultiVolumeReader(volumes)
	require.NoError(t, err)

	assert.Equal(t, int64(30), mvr.Size())
	assert.Equal(t, 3, mvr.TotalVolumes())

	// 1. Read within single volume
	buf := make([]byte, 5)
	n, err := mvr.ReadAt(buf, 2)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "23456", string(buf))

	// 2. Read spanning boundary between seg1 and seg2 (offset 8 to 13: "89ABC")
	bufCross := make([]byte, 5)
	n, err = mvr.ReadAt(bufCross, 8)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "89ABC", string(bufCross))

	// 3. Read spanning all 3 volumes (offset 5 to 25)
	bufAll := make([]byte, 20)
	n, err = mvr.ReadAt(bufAll, 5)
	expected := "56789ABCDEFGHIJklmno"
	require.NoError(t, err)
	assert.Equal(t, 20, n)
	assert.Equal(t, expected, string(bufAll))
}

func TestMultiVolume_ZIP_Roundtrip(t *testing.T) {
	// 1. Prepare multiple files with payload
	f1, _ := file.New("doc1.txt")
	f1.WithData([]byte("Large content piece #1 for multi-volume zip test"))
	f2, _ := file.New("doc2.txt")
	f2.WithData([]byte("Large content piece #2 with more bytes to trigger splitting"))
	f3, _ := file.New("folder/doc3.txt")
	f3.WithData([]byte("Deeply nested content piece #3 in multi-volume archive"))

	files := []*file.File{f1, f2, f3}

	// 2. Create MultiVolumeWriter with a small 64-byte volume limit to force multiple parts
	var volumeBuffers []*memVolume
	opener := func(diskIndex int, isLast bool) (io.WriteCloser, error) {
		mv := &memVolume{}
		volumeBuffers = append(volumeBuffers, mv)
		return mv, nil
	}

	mvw := zip.NewMultiVolumeWriter(opener, 64)

	eng := zip.NewEngine()
	_, err := eng.WriteArchive(mvw, files, container.Config{})
	require.NoError(t, err)
	mvw.Close()

	require.GreaterOrEqual(t, len(volumeBuffers), 2)

	// 3. Assemble MultiVolumeReader
	var readVolumes []zip.Volume
	for i, vb := range volumeBuffers {
		data := vb.buf.Bytes()
		readVolumes = append(readVolumes, zip.Volume{
			DiskIndex: i,
			Reader:    bytes.NewReader(data),
			Size:      int64(len(data)),
		})
	}

	mvr, err := zip.NewMultiVolumeReader(readVolumes)
	require.NoError(t, err)

	// 4. Read Catalog from virtual multi-volume reader
	loadedFiles, _, err := eng.OpenCatalog(mvr, mvr.Size(), "", nil)
	require.NoError(t, err)
	require.Len(t, loadedFiles, 3)

	// 5. Verify uncompressed contents of each file across volumes
	for _, lf := range loadedFiles {
		rc, err := lf.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)

		switch lf.Name() {
		case "doc1.txt":
			assert.Equal(t, "Large content piece #1 for multi-volume zip test", string(data))
		case "doc2.txt":
			assert.Equal(t, "Large content piece #2 with more bytes to trigger splitting", string(data))
		case "folder/doc3.txt":
			assert.Equal(t, "Deeply nested content piece #3 in multi-volume archive", string(data))
		default:
			t.Fatalf("unexpected file in archive: %s", lf.Name())
		}
	}
}
