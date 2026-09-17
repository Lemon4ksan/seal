// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package signature_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/extension/signature"
)

func TestSignature_SignAndVerify(t *testing.T) {
	t.Parallel()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signerName := "Alice <alice@example.com>"
	signer, err := signature.NewSigner(privKey, signerName)
	require.NoError(t, err)

	contentHash := []byte("sha256-mock-hash-1234567890abcdef")
	signer.WithContentHash(contentHash)

	info := extension.ArchiveInfo{
		Format:     container.SevenZip,
		TotalFiles: 42,
	}

	payload, err := signer.OnWrite(t.Context(), info)
	require.NoError(t, err)

	// Verify decoding
	st, err := signature.DecodeState(payload)
	require.NoError(t, err)
	assert.Equal(t, signerName, st.Signer)

	// 1. Successful verification with matching public key
	err = st.Verify(pubKey, info)
	require.NoError(t, err)

	// 2. Successful verification using embedded public key
	err = st.Verify(nil, info)
	require.NoError(t, err)

	// 3. Failure on tampered content
	tamperedInfo := extension.ArchiveInfo{
		Format:     container.SevenZip,
		TotalFiles: 43, // Changed!
	}
	err = st.Verify(pubKey, tamperedInfo)
	require.ErrorIs(t, err, signature.ErrSignatureFailed)

	// 4. Failure on wrong public key
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	err = st.Verify(otherPub, info)
	require.Error(t, err)
}
