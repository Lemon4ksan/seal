// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package checkpoint_test

import (
	"errors"
	"testing"

	"github.com/lemon4ksan/seal/checkpoint"
)

func TestCheckpoint_Roundtrip(t *testing.T) {
	t.Parallel()
	rem := []string{"src/file2.go", "src/file3.go", "assets/logo.png"}
	st := checkpoint.NewState("sess-12345", "/var/data", "7z", 10, 7, rem)
	st.SourceHash = "sha256:abcdef0123456789"

	encoded, err := st.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := checkpoint.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	if decoded.Magic != checkpoint.Magic {
		t.Fatalf("Magic mismatch: got %q, want %q", decoded.Magic, checkpoint.Magic)
	}
	if decoded.SessionID != st.SessionID {
		t.Fatalf("SessionID mismatch: got %q, want %q", decoded.SessionID, st.SessionID)
	}
	if decoded.DoneFiles != 7 || decoded.TotalFiles != 10 {
		t.Fatalf("File count mismatch: done %d, total %d", decoded.DoneFiles, decoded.TotalFiles)
	}
	if len(decoded.RemainingFiles) != len(rem) {
		t.Fatalf("Remaining count mismatch: got %d, want %d", len(decoded.RemainingFiles), len(rem))
	}
}

func TestCheckpoint_InvalidMagic(t *testing.T) {
	t.Parallel()
	badJSON := []byte(`{"magic":"CORRUPTED","version":1}`)
	_, err := checkpoint.Decode(badJSON)
	if !errors.Is(err, checkpoint.ErrInvalidMagic) {
		t.Fatalf("Expected ErrInvalidMagic, got %v", err)
	}
}
