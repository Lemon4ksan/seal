// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/cipher"
	"fmt"

	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/foundation/cryptokit/aead"
)

// NewAEAD instantiates the standard AEAD cipher for the given algorithm ID and 256-bit key.
func NewAEAD(cipherID uint8, key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: invalid key length %d, expected 32 bytes", len(key))
	}

	switch cipherID {
	case spec.CipherAES256GCM:
		return aead.New(aead.AES256GCM, key)

	case spec.CipherChaCha20Poly1305:
		return aead.New(aead.ChaCha20Poly1305, key)

	default:
		return nil, fmt.Errorf("%w: 0x%02X", spec.ErrUnsupportedCipher, cipherID)
	}
}
