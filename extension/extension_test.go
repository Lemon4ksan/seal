// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package extension_test

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/seal/extension"
)

func TestBundle_Roundtrip(t *testing.T) {
	t.Parallel()

	entries := map[extension.ID][]byte{
		"seal.signature": []byte("ED25519-SIGNATURE-BYTES-12345"),
		"acme.license":   []byte(`{"user":"Alice","tier":"enterprise"}`),
		"test.raw":       []byte{0x00, 0xFF, 0x42, 0x13, 0x37},
	}

	encoded := extension.EncodeBundle(entries)
	if len(encoded) == 0 {
		t.Fatalf("expected non-empty encoded bundle")
	}

	decoded, err := extension.DecodeBundle(encoded)
	if err != nil {
		t.Fatalf("DecodeBundle failed: %v", err)
	}

	if len(decoded) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(decoded))
	}

	for id, original := range entries {
		actual, ok := decoded[id]
		if !ok {
			t.Errorf("missing entry %q in decoded bundle", id)
			continue
		}
		if !bytes.Equal(actual, original) {
			t.Errorf("payload mismatch for %q: got %v, want %v", id, actual, original)
		}
	}
}

func TestBundle_LegacyCheckpointFallback(t *testing.T) {
	t.Parallel()

	legacyPayload := []byte("SEAL_CHECKPOINT_V1\n{\"session_id\":\"legacy-123\"}\n---END_SEAL_CHECKPOINT---")
	decoded, err := extension.DecodeBundle(legacyPayload)
	if err != nil {
		t.Fatalf("DecodeBundle with legacy checkpoint failed: %v", err)
	}

	cp, ok := decoded["seal.checkpoint"]
	if !ok {
		t.Fatalf("expected 'seal.checkpoint' entry for legacy payload")
	}
	if !bytes.Equal(cp, legacyPayload) {
		t.Fatalf("legacy payload mismatch: got %q, want %q", string(cp), string(legacyPayload))
	}
}

func TestBundle_StripBundle(t *testing.T) {
	t.Parallel()

	comment := "Official Release v1.0.0 SEXT\x01\x00\x00\x00"
	stripped := extension.StripBundle(comment)
	if stripped != "Official Release v1.0.0" {
		t.Fatalf("unexpected stripped comment: %q", stripped)
	}

	plain := "Just a normal comment without extension"
	if extension.StripBundle(plain) != plain {
		t.Fatalf("plain comment was unexpectedly modified")
	}
}
