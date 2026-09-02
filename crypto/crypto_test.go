// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"bytes"
	"crypto/sha1"
	"io"
	"testing"
)

func TestPBKDF2(t *testing.T) {
	pass := []byte("password")
	salt := []byte("salt")
	key := PBKDF2(pass, salt, 1, 20, sha1.New)
	if len(key) != 20 {
		t.Fatalf("expected key length 20, got %d", len(key))
	}
}

func TestAESCTR_Roundtrip(t *testing.T) {
	pass := "StrongPassword123!"
	content := []byte("Universal pipeline encrypted data payload test across blocks.")

	var buf bytes.Buffer
	w, err := NewAESCTRWriter(&buf, pass, 32, 16) // AES-256
	if err != nil {
		t.Fatalf("NewAESCTRWriter: %v", err)
	}

	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := NewAESCTRReader(&buf, pass, int64(buf.Len()), 32, 16)
	if err != nil {
		t.Fatalf("NewAESCTRReader: %v", err)
	}

	decrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(decrypted) != string(content) {
		t.Fatalf("mismatch: got %q, want %q", string(decrypted), string(content))
	}
}

func TestAESCBC_Roundtrip(t *testing.T) {
	var key [32]byte
	var iv [16]byte
	copy(key[:], "12345678901234567890123456789012")
	copy(iv[:], "1234567890123456")

	content := []byte("Testing AES-CBC stream writer and reader with arbitrary length chunks.")

	var buf bytes.Buffer
	w, err := NewAESCBCWriter(&buf, key, iv)
	if err != nil {
		t.Fatalf("NewAESCBCWriter: %v", err)
	}

	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := NewAESCBCReader(&buf, key, iv)
	if err != nil {
		t.Fatalf("NewAESCBCReader: %v", err)
	}

	decrypted := make([]byte, len(content))
	if _, err := io.ReadFull(r, decrypted); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}

	if string(decrypted) != string(content) {
		t.Fatalf("mismatch: got %q, want %q", string(decrypted), string(content))
	}
}

func TestZipCrypto_Roundtrip(t *testing.T) {
	pass := "ClassicZipPassword"
	content := []byte("Traditional PKWARE ZipCrypto encrypted payload.")

	var buf bytes.Buffer
	w, err := NewZipCryptoWriter(&buf, pass, 0x5A)
	if err != nil {
		t.Fatalf("NewZipCryptoWriter: %v", err)
	}

	if _, err := w.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	r, err := NewZipCryptoReader(&buf, pass, 0x0008, 0, 0x5A00)
	if err != nil {
		t.Fatalf("NewZipCryptoReader: %v", err)
	}

	decrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(decrypted) != string(content) {
		t.Fatalf("mismatch: got %q, want %q", string(decrypted), string(content))
	}
}
