// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/crypto"
	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/seal/crypto/shamir"
)

func TestSecurity_ShamirEndToEnd(t *testing.T) {
	password := "SecretMasterPassword2026!"
	filename := "confidential/payload.bin"
	secretData := []byte("Aeon Zero-Knowledge Shamir Threshold Encryption Verified!")

	// 1. Pack encrypted container
	var buf bytes.Buffer
	w := engine.NewWriter(&buf,
		engine.WithPassword(password),
		engine.WithKDFParams(1024, 1, 1),
	)

	if err := w.AddFileStream(filename, int64(len(secretData)), time.Now(), bytes.NewReader(secretData)); err != nil {
		t.Fatalf("AddFileStream failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	raw := buf.Bytes()

	// 2. Open with password and obtain DEK
	reader, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithReaderPassword(password))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	dek := reader.DEK()
	if len(dek) != 32 {
		t.Fatalf("expected 32-byte DEK, got %d bytes", len(dek))
	}

	// 3. Split DEK into 3-of-5 Shamir shares
	threshold := 3
	total := 5
	rawShares, err := shamir.Split(dek, threshold, total)
	if err != nil {
		t.Fatalf("shamir.Split failed: %v", err)
	}

	formattedShares := make([]string, total)
	for i, s := range rawShares {
		formattedShares[i] = shamir.FormatShare(s, threshold, total)
	}

	// 4. Unlock container with 3 of 5 shares (indices 0, 2, 4) WITHOUT password
	subsetShares := []string{formattedShares[0], formattedShares[2], formattedShares[4]}
	shamirReader, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithShamirShares(subsetShares...))
	if err != nil {
		t.Fatalf("OpenReader with Shamir shares failed: %v", err)
	}

	extracted, err := shamirReader.ReadExtent(filename)
	if err != nil {
		t.Fatalf("ReadExtent failed: %v", err)
	}
	if !bytes.Equal(extracted, secretData) {
		t.Fatalf("payload mismatch: %s != %s", extracted, secretData)
	}

	// 5. Attempt unlocking with insufficient shares (2 of 5, threshold is 3) -> must fail
	insufficientShares := []string{formattedShares[1], formattedShares[3]}
	_, err = engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithShamirShares(insufficientShares...))
	if err == nil {
		t.Fatal("expected failure with insufficient shares (< threshold), but succeeded")
	}

	// 6. Attempt unlocking with tampered share -> must fail
	tamperedShare := formattedShares[0][:len(formattedShares[0])-4] + "ffff"
	_, err = engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithShamirShares(tamperedShare, formattedShares[1], formattedShares[2]))
	if err == nil {
		t.Fatal("expected failure with tampered share CRC, but succeeded")
	}
}

func TestSecurity_KMSEndToEnd(t *testing.T) {
	kms := crypto.NewMockKMSClient()
	keyID := "mock://aeon-cloud-dek"
	filename := "enterprise/db_backup.sql"
	secretData := []byte("CREATE TABLE users (id INT PRIMARY KEY, email TEXT); INSERT INTO users VALUES (1, 'alice@engine.dev');")

	// 1. Pack container protected via KMS envelope
	var buf bytes.Buffer
	w := engine.NewWriter(&buf,
		engine.WithKMSKey(keyID, kms),
	)

	if err := w.AddFileStream(filename, int64(len(secretData)), time.Now(), bytes.NewReader(secretData)); err != nil {
		t.Fatalf("AddFileStream: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw := buf.Bytes()

	// 2. Open without KMS and without password -> ErrPasswordRequired
	_, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)))
	if !errors.Is(err, spec.ErrPasswordRequired) {
		t.Fatalf("expected ErrPasswordRequired, got: %v", err)
	}

	// 3. Open with wrong KMS client -> ErrInvalidPassword
	wrongKMS := crypto.NewMockKMSClient()
	_, err = engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithKMS(wrongKMS))
	if err == nil {
		t.Fatal("expected failure with wrong KMS, got nil")
	}

	// 4. Open with valid KMS client
	reader, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithKMS(kms))
	if err != nil {
		t.Fatalf("OpenReader with KMS failed: %v", err)
	}

	if !reader.IsEncrypted() {
		t.Fatal("expected IsEncrypted() == true")
	}

	extracted, err := reader.ReadExtent(filename)
	if err != nil {
		t.Fatalf("ReadExtent failed: %v", err)
	}
	if !bytes.Equal(extracted, secretData) {
		t.Fatalf("extracted data mismatch: %s != %s", extracted, secretData)
	}
}

func TestSecurity_DualSlotPasswordAndKMS(t *testing.T) {
	kms := crypto.NewMockKMSClient()
	keyID := "vault://transit/keys/enterprise-backup"
	password := "MasterAdminPassword888!"
	filename := "keys/id_ed25519"
	secretData := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAA=\n-----END OPENSSH PRIVATE KEY-----")

	// 1. Pack container with BOTH passphrase AND KMS envelope
	var buf bytes.Buffer
	w := engine.NewWriter(&buf,
		engine.WithPassword(password),
		engine.WithKMSKey(keyID, kms),
		engine.WithKDFParams(1024, 1, 1),
	)

	if err := w.AddFileStream(filename, int64(len(secretData)), time.Now(), bytes.NewReader(secretData)); err != nil {
		t.Fatalf("AddFileStream: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw := buf.Bytes()

	// 2. Unlock via Passphrase ONLY
	passReader, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithReaderPassword(password))
	if err != nil {
		t.Fatalf("OpenReader via password failed: %v", err)
	}
	passData, err := passReader.ReadExtent(filename)
	if err != nil || !bytes.Equal(passData, secretData) {
		t.Fatalf("passReader ReadExtent failed or data mismatch")
	}

	// 3. Unlock via KMS ONLY
	kmsReader, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithKMS(kms))
	if err != nil {
		t.Fatalf("OpenReader via KMS failed: %v", err)
	}
	kmsData, err := kmsReader.ReadExtent(filename)
	if err != nil || !bytes.Equal(kmsData, secretData) {
		t.Fatalf("kmsReader ReadExtent failed or data mismatch")
	}

	// 4. Both DEKs must be identical
	if !bytes.Equal(passReader.DEK(), kmsReader.DEK()) {
		t.Fatal("DEK from password and KMS slots differ!")
	}

	// 5. Split DEK into 2-of-3 Shamir shares and unlock via Shamir ONLY
	dek := passReader.DEK()
	shares, err := shamir.Split(dek, 2, 3)
	if err != nil {
		t.Fatalf("shamir.Split failed: %v", err)
	}
	s1 := shamir.FormatShare(shares[0], 2, 3)
	s2 := shamir.FormatShare(shares[2], 2, 3)

	shamirReader, err := engine.OpenReader(bytes.NewReader(raw), int64(len(raw)), engine.WithShamirShares(s1, s2))
	if err != nil {
		t.Fatalf("OpenReader via Shamir failed: %v", err)
	}
	shamirData, err := shamirReader.ReadExtent(filename)
	if err != nil || !bytes.Equal(shamirData, secretData) {
		t.Fatalf("shamirReader ReadExtent failed or data mismatch")
	}
}
