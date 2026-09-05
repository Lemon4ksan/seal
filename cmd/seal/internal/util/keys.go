// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"os"
)

// LoadPrivateKey reads and parses an Ed25519 private key or seed from file.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	b = bytes.TrimSpace(b)
	if len(b) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(b), nil
	}
	if len(b) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(b), nil
	}
	if len(b) == ed25519.PrivateKeySize*2 {
		decoded := make([]byte, hex.DecodedLen(len(b)))
		if _, err := hex.Decode(decoded, b); err == nil {
			return ed25519.PrivateKey(decoded), nil
		}
	}
	if len(b) == ed25519.SeedSize*2 {
		decoded := make([]byte, hex.DecodedLen(len(b)))
		if _, err := hex.Decode(decoded, b); err == nil {
			return ed25519.NewKeyFromSeed(decoded), nil
		}
	}
	return nil, fmt.Errorf("unrecognized key format (%d bytes, expected 32/64 raw or hex)", len(b))
}
