// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"bytes"
	"crypto/sha1"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
)

func TestPBKDF2(t *testing.T) {
	pass := []byte("password")
	salt := []byte("salt")
	key := PBKDF2(pass, salt, 1, 20, sha1.New)
	assert.Len(t, key, 20)
}

func TestAESCTR_Roundtrip(t *testing.T) {
	pass := "StrongPassword123!"
	content := []byte("Universal pipeline encrypted data payload test across blocks.")

	var buf bytes.Buffer
	w, err := NewAESCTRWriter(&buf, pass, 32, 16) // AES-256
	require.NoError(t, err)

	_, err = w.Write(content)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	r, err := NewAESCTRReader(&buf, pass, int64(buf.Len()), 32, 16)
	require.NoError(t, err)

	decrypted, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, string(content), string(decrypted))
}

func TestAESCBC_Roundtrip(t *testing.T) {
	var key [32]byte
	var iv [16]byte
	copy(key[:], "12345678901234567890123456789012")
	copy(iv[:], "1234567890123456")

	content := []byte("Testing AES-CBC stream writer and reader with arbitrary length chunks.")

	var buf bytes.Buffer
	w, err := NewAESCBCWriter(&buf, key, iv)
	require.NoError(t, err)

	_, err = w.Write(content)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	r, err := NewAESCBCReader(&buf, key, iv)
	require.NoError(t, err)

	decrypted := make([]byte, len(content))
	_, err = io.ReadFull(r, decrypted)
	require.NoError(t, err)
	assert.Equal(t, string(content), string(decrypted))
}

func TestZipCrypto_Roundtrip(t *testing.T) {
	pass := "ClassicZipPassword"
	content := []byte("Traditional PKWARE ZipCrypto encrypted payload.")

	var buf bytes.Buffer
	w, err := NewZipCryptoWriter(&buf, pass, 0x5A)
	require.NoError(t, err)

	_, err = w.Write(content)
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	r, err := NewZipCryptoReader(&buf, pass, 0x0008, 0, 0x5A00)
	require.NoError(t, err)

	decrypted, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, string(content), string(decrypted))
}
