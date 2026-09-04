// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package extension_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/extension"
)

func TestBundle_Roundtrip(t *testing.T) {
	t.Parallel()

	entries := map[extension.ID][]byte{
		"seal.signature": []byte("ED25519-SIGNATURE-BYTES-12345"),
		"acme.license":   []byte(`{"user":"Alice","tier":"enterprise"}`),
		"test.raw":       {0x00, 0xFF, 0x42, 0x13, 0x37},
	}

	encoded := extension.EncodeBundle(entries)
	require.NotEmpty(t, encoded)

	decoded, err := extension.DecodeBundle(encoded)
	require.NoError(t, err)
	require.Len(t, decoded, len(entries))

	for id, original := range entries {
		actual, ok := decoded[id]
		require.Truef(t, ok, "missing entry %q in decoded bundle", id)
		assert.Equal(t, original, actual)
	}
}

func TestBundle_LegacyCheckpointFallback(t *testing.T) {
	t.Parallel()

	legacyPayload := []byte("SEAL_CHECKPOINT_V1\n{\"session_id\":\"legacy-123\"}\n---END_SEAL_CHECKPOINT---")
	decoded, err := extension.DecodeBundle(legacyPayload)
	require.NoError(t, err)

	cp, ok := decoded["seal.checkpoint"]
	require.True(t, ok)
	assert.Equal(t, legacyPayload, cp)
}

func TestBundle_StripBundle(t *testing.T) {
	t.Parallel()

	comment := "Official Release v1.0.0 SEXT\x01\x00\x00\x00"
	stripped := extension.StripBundle(comment)
	assert.Equal(t, "Official Release v1.0.0", stripped)

	plain := "Just a normal comment without extension"
	assert.Equal(t, plain, extension.StripBundle(plain))
}
