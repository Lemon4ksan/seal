// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"bytes"
	"io"
	"testing"

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
	if err != nil {
		t.Fatalf("NewMultiVolumeReader failed: %v", err)
	}

	if mvr.Size() != 30 {
		t.Errorf("expected size 30, got %d", mvr.Size())
	}
	if mvr.TotalVolumes() != 3 {
		t.Errorf("expected 3 volumes, got %d", mvr.TotalVolumes())
	}

	// 1. Read within single volume
	buf := make([]byte, 5)
	n, err := mvr.ReadAt(buf, 2)
	if err != nil || n != 5 || string(buf) != "23456" {
		t.Errorf("read single volume failed: n=%d, err=%v, data=%q", n, err, string(buf))
	}

	// 2. Read spanning boundary between seg1 and seg2 (offset 8 to 13: "89ABC")
	bufCross := make([]byte, 5)
	n, err = mvr.ReadAt(bufCross, 8)
	if err != nil || n != 5 || string(bufCross) != "89ABC" {
		t.Errorf("read boundary cross failed: n=%d, err=%v, data=%q", n, err, string(bufCross))
	}

	// 3. Read spanning all 3 volumes (offset 5 to 25)
	bufAll := make([]byte, 20)
	n, err = mvr.ReadAt(bufAll, 5)
	expected := "56789ABCDEFGHIJklmno"
	if err != nil || n != 20 || string(bufAll) != expected {
		t.Errorf("read all 3 volumes failed: n=%d, err=%v, data=%q, want=%q", n, err, string(bufAll), expected)
	}
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
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}
	mvw.Close()

	if len(volumeBuffers) < 2 {
		t.Fatalf("expected multiple volume segments, got %d", len(volumeBuffers))
	}

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
	if err != nil {
		t.Fatalf("NewMultiVolumeReader failed: %v", err)
	}

	// 4. Read Catalog from virtual multi-volume reader
	loadedFiles, _, err := eng.OpenCatalog(mvr, mvr.Size(), "", nil)
	if err != nil {
		t.Fatalf("OpenCatalog on multi-volume reader failed: %v", err)
	}

	if len(loadedFiles) != 3 {
		t.Fatalf("expected 3 loaded files, got %d", len(loadedFiles))
	}

	// 5. Verify uncompressed contents of each file across volumes
	for _, lf := range loadedFiles {
		rc, err := lf.Open()
		if err != nil {
			t.Fatalf("open %s failed: %v", lf.Name(), err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s failed: %v", lf.Name(), err)
		}

		switch lf.Name() {
		case "doc1.txt":
			if string(data) != "Large content piece #1 for multi-volume zip test" {
				t.Errorf("doc1.txt content mismatch: %q", string(data))
			}
		case "doc2.txt":
			if string(data) != "Large content piece #2 with more bytes to trigger splitting" {
				t.Errorf("doc2.txt content mismatch: %q", string(data))
			}
		case "folder/doc3.txt":
			if string(data) != "Deeply nested content piece #3 in multi-volume archive" {
				t.Errorf("folder/doc3.txt content mismatch: %q", string(data))
			}
		default:
			t.Errorf("unexpected file in archive: %s", lf.Name())
		}
	}
}
