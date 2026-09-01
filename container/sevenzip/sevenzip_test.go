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
		if err := WriteNumber(&buf, val); err != nil {
			t.Fatalf("WriteNumber(%d) failed: %v", val, err)
		}
		readVal, err := ReadNumber(&buf)
		if err != nil {
			t.Fatalf("ReadNumber for (%d) failed: %v", val, err)
		}
		if readVal != val {
			t.Fatalf("Number roundtrip mismatch: wrote %d, read %d (encoded len=%d)", val, readVal, buf.Len())
		}
	}
}

func TestAES256_KeyDerivationAndCrypto(t *testing.T) {
	password := "SecretPass123!_7z"
	salt := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	const numCyclesPower = 10 // $2^{10} = 1024$ for fast unit test

	key1 := DeriveKey(password, salt, numCyclesPower)
	key2 := DeriveKey(password, salt, numCyclesPower)
	if key1 != key2 {
		t.Fatalf("DeriveKey is non-deterministic")
	}

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
	if err != nil {
		t.Fatalf("DecryptStream failed: %v", err)
	}
	decData, err := io.ReadAll(decR)
	if err != nil {
		t.Fatalf("ReadAll decrypt failed: %v", err)
	}
	if len(decData) != len(plain) {
		t.Fatalf("Decrypted length mismatch: got %d, want %d", len(decData), len(plain))
	}
}

func TestSevenZip_Roundtrip(t *testing.T) {
	// Construct test file graph including nested paths and directories
	f1, err := file.New("hello.txt", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	content1 := []byte("Hello, 7-Zip world from Seal archiving engine!")
	f1.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content1)), nil
	})
	f1.WithModTime(time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))

	f2, err := file.New("docs/readme.md", false)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}
	content2 := []byte(
		"# Seal Archiver\n\nHigh-performance universal 7-Zip archiving substrate with LZMA/LZMA2 support.",
	)
	f2.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content2)), nil
	})

	f3, err := file.New("empty_dir/", true)
	if err != nil {
		t.Fatalf("file.New failed: %v", err)
	}

	files := []*file.File{f1, f2, f3}

	// Serialize file collection into standard 7z container
	ws := &inMemoryWriteSeeker{}
	writer := NewWriter(ws)
	totalWritten, err := writer.WriteArchive(files)
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}
	if totalWritten != int64(len(ws.buf)) {
		t.Fatalf("Total written mismatch: got %d, want %d", totalWritten, len(ws.buf))
	}

	// Deserialize container and verify roundtrip fidelity
	reader, err := NewReader(bytes.NewReader(ws.buf), int64(len(ws.buf)), "")
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	entries := reader.Entries()
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries, got %d", len(entries))
	}

	entryMap := make(map[string]*file.File)
	for _, e := range entries {
		entryMap[e.Name()] = e
	}

	// Verify file 1
	e1, ok := entryMap["hello.txt"]
	if !ok {
		t.Fatalf("e1 'hello.txt' missing from entries")
	}
	rc1, err := e1.Open()
	if err != nil {
		t.Fatalf("e1 Open failed: %v", err)
	}
	res1, err := io.ReadAll(rc1)
	_ = rc1.Close()
	if err != nil {
		t.Fatalf("e1 ReadAll failed: %v", err)
	}
	if !bytes.Equal(res1, content1) {
		t.Errorf("e1 content mismatch: got %q, want %q", string(res1), string(content1))
	}

	// Verify file 2
	e2, ok := entryMap["docs/readme.md"]
	if !ok {
		t.Fatalf("e2 'docs/readme.md' missing from entries")
	}
	rc2, err := e2.Open()
	if err != nil {
		t.Fatalf("e2 Open failed: %v", err)
	}
	res2, err := io.ReadAll(rc2)
	_ = rc2.Close()
	if err != nil {
		t.Fatalf("e2 ReadAll failed: %v", err)
	}
	if !bytes.Equal(res2, content2) {
		t.Errorf("e2 content mismatch: got %q, want %q", string(res2), string(content2))
	}

	// Verify file 3 (dir)
	e3, ok := entryMap["empty_dir/"]
	if !ok {
		t.Fatalf("e3 'empty_dir/' missing from entries")
	}
	if !e3.IsDir() {
		t.Errorf("e3 expected directory, got regular file")
	}
}

func TestContainerRegistration(t *testing.T) {
	c, err := container.Get(container.SevenZip)
	if err != nil {
		t.Fatalf("container.Get(SevenZip) failed: %v", err)
	}
	if c.Type() != container.SevenZip {
		t.Fatalf("expected container type %v, got %v", container.SevenZip, c.Type())
	}
}
