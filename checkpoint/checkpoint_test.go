// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package checkpoint_test

import (
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/checkpoint"
)

func TestCheckpoint_Roundtrip(t *testing.T) {
	t.Parallel()
	rem := []string{"src/file2.go", "src/file3.go", "assets/logo.png"}
	st := checkpoint.NewState("sess-12345", "/var/data", "7z", 10, 7, rem)
	st.SourceHash = "sha256:abcdef0123456789"

	encoded, err := st.Encode()
	require.NoError(t, err)

	decoded, err := checkpoint.Decode(encoded)
	require.NoError(t, err)

	assert.Equal(t, checkpoint.Magic, decoded.Magic)
	assert.Equal(t, st.SessionID, decoded.SessionID)
	assert.Equal(t, 7, decoded.DoneFiles)
	assert.Equal(t, 10, decoded.TotalFiles)
	assert.Equal(t, rem, decoded.RemainingFiles)
}

func TestCheckpoint_InvalidMagic(t *testing.T) {
	t.Parallel()
	badJSON := []byte(`{"magic":"CORRUPTED","version":1}`)
	_, err := checkpoint.Decode(badJSON)
	require.ErrorIs(t, err, checkpoint.ErrInvalidMagic)
}
