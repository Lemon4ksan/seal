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

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

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

func TestFile_SetAndWithSymmetry(t *testing.T) {
	t.Parallel()

	// Fluent chaining with With...
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	f, err := file.New("test/file.txt")
	require.NoError(t, err)

	f.WithSize(1024).
		WithCompressedSize(512).
		WithCRC(0xdeadbeef).
		WithOffset(4096).
		WithFlags(0x08).
		WithLevel(5).
		WithCompression(codec.ZStandard, 5).
		WithMode(0o600).
		WithModTime(now).
		WithComment("fluent comment").
		WithPassword("fluentPass").
		WithImplicit(true)

	assert.Equal(t, int64(1024), f.Size())
	assert.Equal(t, int64(512), f.CompressedSize())
	assert.Equal(t, uint32(0xdeadbeef), f.CRC())
	assert.Equal(t, int64(4096), f.LocalHeaderOffset())
	assert.Equal(t, uint16(0x08), f.Flags())
	assert.Equal(t, 5, f.Level())
	assert.Equal(t, codec.ZStandard, f.Method())
	assert.Equal(t, "fluent comment", f.Comment())
	assert.Equal(t, "fluentPass", f.Password())
	assert.True(t, f.IsImplicit())

	// Imperative with Set...
	f2, err := file.New("test/file2.txt")
	require.NoError(t, err)

	f2.SetSize(2048)
	f2.SetCompressedSize(1024)
	f2.SetCRC(0xcafebabe)
	f2.SetOffset(8192)
	f2.SetFlags(0x02)
	f2.SetLevel(9)
	f2.SetCompression(codec.LZMA2, 9)
	f2.SetMode(0o700)
	f2.SetModTime(now)
	f2.SetComment("setter comment")
	f2.SetPassword("setterPass")
	f2.SetIsImplicit(false)

	assert.Equal(t, int64(2048), f2.Size())
	assert.Equal(t, int64(1024), f2.CompressedSize())
	assert.Equal(t, uint32(0xcafebabe), f2.CRC())
	assert.Equal(t, int64(8192), f2.LocalHeaderOffset())
	assert.Equal(t, uint16(0x02), f2.Flags())
	assert.Equal(t, 9, f2.Level())
	assert.Equal(t, codec.LZMA2, f2.Method())
	assert.Equal(t, "setter comment", f2.Comment())
	assert.Equal(t, "setterPass", f2.Password())
	assert.False(t, f2.IsImplicit())
}
