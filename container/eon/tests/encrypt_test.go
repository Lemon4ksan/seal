// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func TestContainer_EncryptionEndToEnd(t *testing.T) {
	ciphers := []uint8{spec.CipherAES256GCM, spec.CipherChaCha20Poly1305}

	for _, cipherID := range ciphers {
		password := "TopSecretPassword987!"
		filename := "vault/confidential_keys.json"
		secretData := []byte("{\"private_key\": \"super-secret-hex-data-1234567890\"}")

		// 1. Pack encrypted container
		var buf bytes.Buffer
		w := engine.NewWriter(&buf,
			engine.WithPassword(password),
			engine.WithCipher(cipherID),
			engine.WithKDFParams(1024, 1, 1), // fast for unit tests
		)

		if err := w.AddFileStream(filename, int64(len(secretData)), time.Now(), bytes.NewReader(secretData)); err != nil {
			t.Fatalf("cipher %d: AddFileStream failed: %v", cipherID, err)
		}

		if err := w.Close(); err != nil {
			t.Fatalf("cipher %d: Close failed: %v", cipherID, err)
		}

		rawContainer := buf.Bytes()

		// 2. Metadata Privacy Test: Ensure filename does NOT appear anywhere in the file!
		if bytes.Contains(rawContainer, []byte("confidential_keys.json")) {
			t.Fatalf("cipher %d: METADATA LEAKAGE DETECTED! Plaintext filename found in encrypted container!", cipherID)
		}
		if bytes.Contains(rawContainer, []byte("super-secret-hex-data")) {
			t.Fatalf("cipher %d: PAYLOAD LEAKAGE DETECTED! Plaintext secret payload found in encrypted container!", cipherID)
		}

		// 3. Open without password -> ErrPasswordRequired
		_, err := engine.OpenReader(bytes.NewReader(rawContainer), int64(len(rawContainer)))
		if !errors.Is(err, spec.ErrPasswordRequired) {
			t.Fatalf("cipher %d: expected ErrPasswordRequired, got %v", cipherID, err)
		}

		// 4. Open with wrong password -> ErrInvalidPassword
		_, err = engine.OpenReader(bytes.NewReader(rawContainer), int64(len(rawContainer)), engine.WithReaderPassword("wrong-pass"))
		if !errors.Is(err, spec.ErrInvalidPassword) {
			t.Fatalf("cipher %d: expected ErrInvalidPassword, got %v", cipherID, err)
		}

		// 5. Open with correct password
		reader, err := engine.OpenReader(bytes.NewReader(rawContainer), int64(len(rawContainer)), engine.WithReaderPassword(password))
		if err != nil {
			t.Fatalf("cipher %d: OpenReader failed: %v", cipherID, err)
		}

		if !reader.IsEncrypted() {
			t.Fatalf("cipher %d: expected IsEncrypted() == true", cipherID)
		}
		if reader.CipherID() != cipherID {
			t.Fatalf("cipher %d: expected CipherID %d, got %d", cipherID, cipherID, reader.CipherID())
		}

		// Read extent
		decrypted, err := reader.ReadExtent(filename)
		if err != nil {
			t.Fatalf("cipher %d: ReadExtent failed: %v", cipherID, err)
		}
		if !bytes.Equal(decrypted, secretData) {
			t.Fatalf("cipher %d: decrypted payload mismatch: %s != %s", cipherID, decrypted, secretData)
		}

		// 6. Chunk Tamper Detection
		tampered := make([]byte, len(rawContainer))
		copy(tampered, rawContainer)

		// Tamper one byte within chunk payload (offset 4096 is safely within Chunk 0)
		tampered[4096+spec.ChunkHeaderSize] ^= 0xFF

		tReader, err := engine.OpenReader(bytes.NewReader(tampered), int64(len(tampered)), engine.WithReaderPassword(password))
		if err == nil {
			_, err = tReader.ReadExtent(filename)
			if err == nil {
				t.Fatalf("cipher %d: expected tamper authentication failure, but read succeeded!", cipherID)
			}
		}
	}
}

func TestContainer_EncryptedAndSigned(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	password := "ContainerVaultPassword!"
	var buf bytes.Buffer
	w := engine.NewWriter(&buf,
		engine.WithPassword(password),
		engine.WithSigner(priv, "signer@engine.vault"),
		engine.WithKDFParams(1024, 1, 1),
	)

	payload := []byte("signed and encrypted payload content")
	if err := w.AddFileStream("keys/token.txt", int64(len(payload)), time.Now(), bytes.NewReader(payload)); err != nil {
		t.Fatalf("AddFileStream: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw := buf.Bytes()
	reader, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithReaderPassword(password))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}

	if !reader.IsEncrypted() {
		t.Fatal("expected encrypted")
	}
	if !reader.HasSignature() {
		t.Fatal("expected signed")
	}

	if err := reader.VerifySignature(pub); err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}

	out, err := reader.ReadExtent("keys/token.txt")
	if err != nil || string(out) != string(payload) {
		t.Fatalf("ReadExtent failed: %v", err)
	}
}
