// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file_test

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
)

func TestFile_CreationAndProperties(t *testing.T) {
	f, err := file.New("docs/report.txt", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}

	if f.Name() != "docs/report.txt" {
		t.Errorf("Name mismatch: got %s, want docs/report.txt", f.Name())
	}
	if f.IsDir() {
		t.Errorf("expected file, got directory")
	}

	f.SetSizes(100, 50, 0x12345678)
	if f.Size() != 100 || f.UncompressedSize() != 100 {
		t.Errorf("uncompressed size mismatch: %d", f.Size())
	}
	if f.CompressedSize() != 50 {
		t.Errorf("compressed size mismatch: %d", f.CompressedSize())
	}
	if f.CRC() != 0x12345678 {
		t.Errorf("crc mismatch: 0x%08x", f.CRC())
	}

	f.WithCompression(codec.ZStandard, 3)
	if f.Method() != codec.ZStandard || f.Level() != 3 {
		t.Errorf("compression mismatch: method=%v, level=%d", f.Method(), f.Level())
	}

	f.WithPassword("pass123")
	if f.Password() != "pass123" || f.Encryption() != file.AES256 {
		t.Errorf("password/encryption mismatch")
	}

	data := []byte("Universal File Content")
	f.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})

	content, err := f.String()
	if err != nil {
		t.Fatalf("String() failed: %v", err)
	}
	if content != "Universal File Content" {
		t.Errorf("content mismatch: got %q", content)
	}
}

func TestFile_Concurrency(t *testing.T) {
	f, err := file.New("concurrency_test.bin", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}

	f.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader([]byte("test"))), nil
	})

	var wg sync.WaitGroup
	workers := 16
	iterations := 1000

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Concurrent readers and writers
				if j%2 == 0 {
					f.SetSizes(int64(j), int64(j/2), uint32(j))
					f.WithModTime(time.Now())
				} else {
					_ = f.Name()
					_ = f.Size()
					_ = f.CompressedSize()
					_ = f.CRC()
					_ = f.ModTime()
					rc, _ := f.Open()
					if rc != nil {
						_ = rc.Close()
					}
				}
			}
		}(i)
	}

	wg.Wait()
}
