// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"crypto/rand"
	"io"
	"math"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
)

type inMemoryWriteSeeker struct {
	buf []byte
	pos int64
}

func (m *inMemoryWriteSeeker) Write(p []byte) (int, error) {
	end := m.pos + int64(len(p))
	if end > int64(len(m.buf)) {
		newBuf := make([]byte, end)
		copy(newBuf, m.buf)
		m.buf = newBuf
	}
	copy(m.buf[m.pos:], p)
	m.pos = end
	return len(p), nil
}

func (m *inMemoryWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		m.pos = offset
	case io.SeekCurrent:
		m.pos += offset
	case io.SeekEnd:
		m.pos = int64(len(m.buf)) + offset
	}
	return m.pos, nil
}

func TestWriteNumber_Boundaries(t *testing.T) {
	cases := []uint64{
		0,
		1,
		127,
		128,
		255,
		256,
		16383,
		16384,
		2097151,
		2097152,
		0xFFFFFFFF,
		0x100000000,
		math.MaxUint64,
	}

	for _, val := range cases {
		var buf bytes.Buffer
		err := WriteNumber(&buf, val)
		require.NoErrorf(t, err, "WriteNumber(%d) failed: %v", val, err)
		readVal, err := ReadNumber(&buf)
		require.NoErrorf(t, err, "ReadNumber for (%d) failed: %v", val, err)
		assert.Equalf(t, val, readVal, "Number roundtrip mismatch: wrote %d, read %d (encoded len=%d)", val, readVal, buf.Len())
	}
}

func TestAES256_KeyDerivationAndCrypto(t *testing.T) {
	password := "SecretPass123!_7z"
	salt := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	const numCyclesPower = 10 // $2^{10} = 1024$ for fast unit test

	key1 := DeriveKey(password, salt, numCyclesPower)
	key2 := DeriveKey(password, salt, numCyclesPower)
	assert.Equal(t, key1, key2, "DeriveKey is non-deterministic")

	// Encrypted stream roundtrip test
	plain := []byte("Sensitive payload to be protected with AES-256-CBC multi-round key stretching!")
	// Align to 16 bytes
	for len(plain)%16 != 0 {
		plain = append(plain, 0)
	}

	// Properties format: [firstByte, saltSize, salt..., iv...]
	// firstByte: bit7=saltBit(1), bit6=ivBit(1), cycles=10 -> 0x80 | 0x40 | 10 = 0xCA
	props := make([]byte, 0, 34)
	props = append(props, 0x80|0x40|byte(numCyclesPower))
	props = append(props, (byte(len(salt))<<4)|0x10) // 8 bytes salt, 16 bytes IV
	props = append(props, salt...)
	iv := make([]byte, 16)
	_, _ = rand.Read(iv)
	props = append(props, iv...)

	// Decrypt
	decR, err := DecryptStream(bytes.NewReader(plain), password, props)
	require.NoError(t, err)
	decData, err := io.ReadAll(decR)
	require.NoError(t, err)
	assert.Equal(t, len(plain), len(decData))
}

func TestSevenZip_Roundtrip(t *testing.T) {
	// Construct test file graph including nested paths and directories
	f1, err := file.New("hello.txt", false)
	require.NoError(t, err)
	content1 := []byte("Hello, 7-Zip world from Seal archiving engine!")
	f1.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content1)), nil
	})
	f1.WithModTime(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))

	f2, err := file.New("docs/readme.md", false)
	require.NoError(t, err)
	content2 := []byte(
		"# Seal Archiver\n\nHigh-performance universal 7-Zip archiving substrate with LZMA/LZMA2 support.",
	)
	f2.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content2)), nil
	})

	f3, err := file.New("empty_dir/", true)
	require.NoError(t, err)

	files := []*file.File{f1, f2, f3}

	// Serialize file collection into standard 7z container
	ws := &inMemoryWriteSeeker{}
	writer := NewWriter(ws)
	totalWritten, err := writer.WriteArchive(files)
	require.NoError(t, err)
	assert.Equal(t, int64(len(ws.buf)), totalWritten)

	// Deserialize container and verify roundtrip fidelity
	reader, err := NewReader(bytes.NewReader(ws.buf), int64(len(ws.buf)), "")
	require.NoError(t, err)

	entries := reader.Entries()
	require.Len(t, entries, 3)

	entryMap := make(map[string]*file.File)
	for _, e := range entries {
		entryMap[e.Name()] = e
	}

	// Verify file 1
	e1, ok := entryMap["hello.txt"]
	require.True(t, ok, "e1 'hello.txt' missing from entries")
	rc1, err := e1.Open()
	require.NoError(t, err)
	res1, err := io.ReadAll(rc1)
	_ = rc1.Close()
	require.NoError(t, err)
	assert.Equal(t, content1, res1)

	// Verify file 2
	e2, ok := entryMap["docs/readme.md"]
	require.True(t, ok, "e2 'docs/readme.md' missing from entries")
	rc2, err := e2.Open()
	require.NoError(t, err)
	res2, err := io.ReadAll(rc2)
	_ = rc2.Close()
	require.NoError(t, err)
	assert.Equal(t, content2, res2)

	// Verify file 3 (dir)
	e3, ok := entryMap["empty_dir/"]
	require.True(t, ok, "e3 'empty_dir/' missing from entries")
	assert.True(t, e3.IsDir())
}

func TestContainerRegistration(t *testing.T) {
	c, err := container.Get(container.SevenZip)
	require.NoError(t, err)
	assert.Equal(t, container.SevenZip, c.Type())
}

func TestSevenZip_AES256_EncryptedHeader_Roundtrip(t *testing.T) {
	pwd := "Strong7zPassword!#2026"
	var outBuf inMemoryWriteSeeker

	wOpts := WriterOptions{
		Password:       pwd,
		EncryptHeaders: true,
	}
	zw := NewWriterWithOptions(&outBuf, wOpts)

	content1 := []byte("Top-secret confidential document classified for 7z AES-256 storage.")
	content2 := []byte("Secondary payload with encrypted headers hiding file names.")

	f1, err := file.New("secret/doc.txt", false)
	require.NoError(t, err)
	f1.WithData(content1)

	f2, err := file.New("confidential.json", false)
	require.NoError(t, err)
	f2.WithData(content2)

	n, err := zw.WriteArchive([]*file.File{f1, f2})
	require.NoError(t, err)
	assert.Greater(t, n, int64(0))

	archiveBytes := outBuf.buf

	// 1. Attempt to open without password: must fail on encrypted header
	_, err = NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)), "")
	require.Error(t, err)

	// 2. Attempt to open with wrong password: must fail
	_, err = NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)), "WrongPassword")
	require.Error(t, err)

	// 3. Open with correct password: must succeed and extract both files
	zr, err := NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)), pwd)
	require.NoError(t, err)

	entries := zr.Entries()
	require.Len(t, entries, 2)

	entryMap := make(map[string]*file.File)
	for _, e := range entries {
		entryMap[e.Name()] = e
	}

	// Verify doc.txt
	e1, ok := entryMap["secret/doc.txt"]
	require.True(t, ok, "missing 'secret/doc.txt' in encrypted archive")
	rc1, err := e1.Open()
	require.NoError(t, err)
	data1, err := io.ReadAll(rc1)
	_ = rc1.Close()
	require.NoError(t, err)
	assert.Equal(t, content1, data1)

	// Verify confidential.json
	e2, ok := entryMap["confidential.json"]
	require.True(t, ok, "missing 'confidential.json' in encrypted archive")
	rc2, err := e2.Open()
	require.NoError(t, err)
	data2, err := io.ReadAll(rc2)
	_ = rc2.Close()
	require.NoError(t, err)
	assert.Equal(t, content2, data2)
}
