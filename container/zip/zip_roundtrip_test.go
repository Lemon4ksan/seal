// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
)

func TestZip_Roundtrip_Plain(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)

	file1, err := file.New("hello.txt", false)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	file1.WithModTime(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))

	content1 := []byte("Hello, Seal ZIP engine!")
	if err := w.WriteEntry(file1, bytes.NewReader(content1)); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	entries := r.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name() != "hello.txt" {
		t.Fatalf("expected hello.txt, got %s", entries[0].Name())
	}

	rc, err := entries[0].Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	readBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(readBytes) != string(content1) {
		t.Fatalf("content mismatch: got %q, want %q", string(readBytes), string(content1))
	}
}

func TestZip_Roundtrip_AES256(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)
	w.SetPassword("SecretPassword123")

	f, err := file.New("secure.txt", false)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	f.WithEncryption(file.AES256, "SecretPassword123")

	content := []byte("Highly confidential data protected with AES-256 standard.")
	if err := w.WriteEntry(f, bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify authentication and decryption succeeds when matching passphrase is provided
	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "SecretPassword123", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	entry, ok := r.Entry("secure.txt")
	if !ok {
		t.Fatalf("entry secure.txt not found")
	}
	if entry.Encryption() != file.AES256 {
		t.Fatalf("expected AES256 encryption, got %v", entry.Encryption())
	}

	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	decrypted, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(decrypted) != string(content) {
		t.Fatalf("decrypted mismatch: got %q, want %q", string(decrypted), string(content))
	}

	// Verify authentication failure when incorrect passphrase is provided
	rBad, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "WrongPassword", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader bad: %v", err)
	}

	entryBad, ok := rBad.Entry("secure.txt")
	if !ok {
		t.Fatalf("entry secure.txt not found in bad reader")
	}

	rcBad, err := entryBad.Open()
	if err == nil {
		_, err = io.ReadAll(rcBad)
		_ = rcBad.Close()
	}
	if err == nil {
		t.Fatalf("expected error decrypting with wrong password, got nil")
	}
}

func TestZip_Roundtrip_AES128(t *testing.T) {
	buf := new(bytes.Buffer)
	w := NewWriter(buf, codec.DefaultRegistry)
	w.SetPassword("SecretAES128Password")

	f, err := file.New("secure128.txt", false)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	f.WithEncryption(file.AES128, "SecretAES128Password")

	content := []byte("Highly confidential data protected with WinZip AES-128 standard.")
	if err := w.WriteEntry(f, bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()), "SecretAES128Password", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	entry, ok := r.Entry("secure128.txt")
	if !ok {
		t.Fatalf("entry secure128.txt not found")
	}
	if entry.Encryption() != file.AES128 {
		t.Fatalf("expected AES128 encryption, got %v", entry.Encryption())
	}

	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	decrypted, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(decrypted) != string(content) {
		t.Fatalf("decrypted mismatch: got %q, want %q", string(decrypted), string(content))
	}
}

func TestZip_SFX_SelfExtractingArchive(t *testing.T) {
	// Create a standard ZIP payload
	zipBuf := new(bytes.Buffer)
	w := NewWriter(zipBuf, codec.DefaultRegistry)

	f, err := file.New("setup.cfg", false)
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	content := []byte("[SFX Config]\nAppVersion=2026.1\nAutoRun=true\n")
	if err := w.WriteEntry(f, bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Prepend a 1024-byte mock PE32 executable stub (e.g. SFX unpacker binary)
	sfxStub := make([]byte, 1024)
	stubHeader := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xFF\xFF\x00\x00PE-SFX")
	copy(sfxStub, stubHeader)

	sfxExe := make([]byte, 0, len(sfxStub)+zipBuf.Len())
	sfxExe = append(sfxExe, sfxStub...)
	sfxExe = append(sfxExe, zipBuf.Bytes()...)

	// Open SFX .exe as a ZIP archive
	r, err := NewReader(bytes.NewReader(sfxExe), int64(len(sfxExe)), "", codec.DefaultRegistry)
	if err != nil {
		t.Fatalf("NewReader on SFX failed: %v", err)
	}

	entry, ok := r.Entry("setup.cfg")
	if !ok {
		t.Fatalf("entry setup.cfg not found in SFX archive")
	}

	rc, err := entry.Open()
	if err != nil {
		t.Fatalf("entry.Open failed on SFX: %v", err)
	}
	defer rc.Close()

	readBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll failed on SFX: %v", err)
	}

	if !bytes.Equal(readBytes, content) {
		t.Fatalf("SFX content mismatch: got %q, want %q", string(readBytes), string(content))
	}
}
