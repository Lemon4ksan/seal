// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signature_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/extension/signature"
)

func TestSignature_SignAndVerify(t *testing.T) {
	t.Parallel()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	signerName := "Alice <alice@example.com>"
	signer, err := signature.NewSigner(privKey, signerName)
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	contentHash := []byte("sha256-mock-hash-1234567890abcdef")
	signer.WithContentHash(contentHash)

	info := extension.ArchiveInfo{
		Format:     container.SevenZip,
		TotalFiles: 42,
	}

	payload, err := signer.OnWrite(t.Context(), info)
	if err != nil {
		t.Fatalf("OnWrite failed: %v", err)
	}

	// Verify decoding
	st, err := signature.DecodeState(payload)
	if err != nil {
		t.Fatalf("DecodeState failed: %v", err)
	}
	if st.Signer != signerName {
		t.Fatalf("Signer mismatch: got %q, want %q", st.Signer, signerName)
	}

	// 1. Successful verification with matching public key
	if err := st.Verify(pubKey, info); err != nil {
		t.Fatalf("Verify with pubKey failed: %v", err)
	}

	// 2. Successful verification using embedded public key
	if err := st.Verify(nil, info); err != nil {
		t.Fatalf("Verify with nil expected pubKey failed: %v", err)
	}

	// 3. Failure on tampered content
	tamperedInfo := extension.ArchiveInfo{
		Format:     container.SevenZip,
		TotalFiles: 43, // Changed!
	}
	if err := st.Verify(pubKey, tamperedInfo); !errors.Is(err, signature.ErrSignatureFailed) {
		t.Fatalf("Expected ErrSignatureFailed on tampered file count, got %v", err)
	}

	// 4. Failure on wrong public key
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if err := st.Verify(otherPub, info); err == nil {
		t.Fatalf("Expected error on wrong public key, got nil")
	}
}
