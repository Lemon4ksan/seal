// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func TestContainer_DigitalSignatureAndTamperDetection(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	otherPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	var buf bytes.Buffer
	w := engine.NewWriter(&buf,
		engine.WithCodec(spec.CodecStore),
		engine.WithSigner(priv, "security@lemon4ksan"),
	)

	payload1 := []byte("critical configuration payload")
	if err := w.AddFileStream("etc/config.json", int64(len(payload1)), time.Now(), bytes.NewReader(payload1)); err != nil {
		t.Fatalf("AddFileStream failed: %v", err)
	}

	payload2 := []byte("bin executable data")
	if err := w.AddFileStream("bin/app", int64(len(payload2)), time.Now(), bytes.NewReader(payload2)); err != nil {
		t.Fatalf("AddFileStream failed: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Writer.Close failed: %v", err)
	}

	containerBytes := buf.Bytes()

	// 1. Verify on intact container
	reader, err := engine.OpenReader(bytes.NewReader(containerBytes), int64(len(containerBytes)))
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}

	if !reader.HasSignature() {
		t.Fatal("expected container to have signature")
	}

	dsig := reader.DSIG()
	if dsig.KeyID != "security@lemon4ksan" {
		t.Errorf("expected KeyID security@lemon4ksan, got %q", dsig.KeyID)
	}

	// Verify with embedded key
	if err := reader.VerifySignature(nil); err != nil {
		t.Errorf("VerifySignature with embedded key failed: %v", err)
	}

	// Verify with explicit trusted public key
	if err := reader.VerifySignature(pub); err != nil {
		t.Errorf("VerifySignature with trusted key failed: %v", err)
	}

	// Verify with wrong public key
	if err := reader.VerifySignature(otherPub); err == nil {
		t.Error("expected signature failure with wrong public key, got nil")
	}

	// Verify random-access retrieval
	data, err := reader.ReadExtent("etc/config.json")
	if err != nil || string(data) != string(payload1) {
		t.Fatalf("ReadExtent failed: %v", err)
	}

	// 2. Tamper Test: Mutate one byte in INDX (e.g. modify filename)
	tamperedBytes := make([]byte, len(containerBytes))
	copy(tamperedBytes, containerBytes)

	// Search for "etc/config.json" in tamperedBytes and flip a byte
	idx := bytes.Index(tamperedBytes, []byte("etc/config.json"))
	if idx < 0 {
		t.Fatal("could not locate filename in container")
	}
	tamperedBytes[idx] = 'x' // Change to "xtc/config.json"

	tamperedReader, err := engine.OpenReader(bytes.NewReader(tamperedBytes), int64(len(tamperedBytes)))
	if err != nil {
		// If catalog parsing failed due to length/CRC, that's also acceptable, but if it parsed:
		return
	}

	if err := tamperedReader.VerifySignature(pub); err == nil {
		t.Error("expected signature verification to FAIL on tampered container, but it PASSED!")
	} else if !errors.Is(err, spec.ErrInvalidSignature) && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected ErrInvalidSignature, got: %v", err)
	}
}
