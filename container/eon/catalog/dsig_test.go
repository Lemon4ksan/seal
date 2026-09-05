// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func TestDSIG_Roundtrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	dataToSign := []byte("simulated-ctoc-and-indx-digest")
	sig := ed25519.Sign(priv, dataToSign)

	original := &catalog.DSIG{
		AlgorithmID: spec.SigAlgoEd25519,
		KeyID:       "signer@engine.foundation",
		PublicKey:   pub,
		Signature:   sig,
	}

	var buf bytes.Buffer
	written, err := original.Write(&buf)
	if err != nil {
		t.Fatalf("DSIG.Write failed: %v", err)
	}

	if written != int64(buf.Len()) {
		t.Errorf("written mismatch: %d != %d", written, buf.Len())
	}

	parsed, err := catalog.ReadDSIG(&buf)
	if err != nil {
		t.Fatalf("ReadDSIG failed: %v", err)
	}

	if parsed.AlgorithmID != original.AlgorithmID {
		t.Errorf("algo mismatch: %d != %d", parsed.AlgorithmID, original.AlgorithmID)
	}
	if parsed.KeyID != original.KeyID {
		t.Errorf("keyID mismatch: %q != %q", parsed.KeyID, original.KeyID)
	}
	if !bytes.Equal(parsed.PublicKey, original.PublicKey) {
		t.Errorf("publicKey mismatch")
	}
	if !bytes.Equal(parsed.Signature, original.Signature) {
		t.Errorf("signature mismatch")
	}

	// Verify signature
	if !ed25519.Verify(parsed.PublicKey, dataToSign, parsed.Signature) {
		t.Errorf("ed25519 verification failed")
	}
}
