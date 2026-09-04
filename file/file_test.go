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

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
)

func TestFile_CreationAndProperties(t *testing.T) {
	f, err := file.New("docs/report.txt", false)
	require.NoError(t, err)

	assert.Equal(t, "docs/report.txt", f.Name())
	assert.False(t, f.IsDir())

	f.SetSizes(100, 50, 0x12345678)
	assert.Equal(t, int64(100), f.Size())
	assert.Equal(t, int64(100), f.UncompressedSize())
	assert.Equal(t, int64(50), f.CompressedSize())
	assert.Equal(t, uint32(0x12345678), f.CRC())

	f.WithCompression(codec.ZStandard, 3)
	assert.Equal(t, codec.ZStandard, f.Method())
	assert.Equal(t, 3, f.Level())

	f.WithPassword("pass123")
	assert.Equal(t, "pass123", f.Password())
	assert.Equal(t, file.AES256, f.Encryption())

	data := []byte("Universal File Content")
	f.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})

	content, err := f.String()
	require.NoError(t, err)
	assert.Equal(t, "Universal File Content", content)
}

func TestFile_Concurrency(t *testing.T) {
	f, err := file.New("concurrency_test.bin", false)
	require.NoError(t, err)

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
