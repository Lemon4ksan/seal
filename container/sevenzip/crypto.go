// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"crypto/rand"
	"errors"
	"io"

	"github.com/lemon4ksan/seal/crypto"
)

// ErrInvalidPassword indicates wrong or missing 7z archive password.
var ErrInvalidPassword = errors.New("sevenzip: invalid or missing password for encrypted archive")

// DeriveKey implements 7-Zip SHA-256 key derivation with multi-round stretching.
func DeriveKey(password string, salt []byte, numCyclesPower byte) [32]byte {
	return crypto.Derive7zKey(password, salt, numCyclesPower)
}

// DecryptStream wraps an encrypted 7z stream with AES-256-CBC decryption.
func DecryptStream(r io.Reader, password string, props []byte) (io.Reader, error) {
	if len(props) == 0 {
		return nil, errors.New("sevenzip: missing AES properties")
	}

	firstByte := props[0]
	numCyclesPower := firstByte & 0x3F
	if numCyclesPower > 24 {
		numCyclesPower = 24 // Security cap
	}

	saltSize := 0
	ivSize := 0

	saltBit := (firstByte >> 7) & 1
	ivBit := (firstByte >> 6) & 1

	propPos := 1
	if saltBit != 0 && propPos < len(props) {
		saltSize = int(props[propPos] >> 4)
		if saltSize == 0 {
			saltSize = 16
		}
	}
	if ivBit != 0 && propPos < len(props) {
		ivSize = int(props[propPos] & 0x0F)
		if ivSize == 0 {
			ivSize = 16
		}
	}
	if (saltBit != 0 || ivBit != 0) && propPos < len(props) {
		propPos++
	}

	var salt []byte
	if saltSize > 0 && propPos+saltSize <= len(props) {
		salt = props[propPos : propPos+saltSize]
		propPos += saltSize
	}

	var iv [16]byte
	if ivSize > 0 && propPos+ivSize <= len(props) {
		copy(iv[:], props[propPos:propPos+ivSize])
	}

	key := DeriveKey(password, salt, numCyclesPower)
	return crypto.NewAESCBCReader(r, key, iv)
}

// EncryptStream wraps an output stream with 7-Zip AES-256-CBC encryption.
// Returns the encrypting writer and the encoded 7z AES coder properties.
func EncryptStream(w io.Writer, password string, numCyclesPower byte) (io.WriteCloser, []byte, error) {
	if numCyclesPower == 0 {
		numCyclesPower = 19 // Standard 7-Zip 2^19 cycles
	}
	if numCyclesPower > 24 {
		numCyclesPower = 24
	}

	var salt [16]byte
	var iv [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return nil, nil, err
	}
	if _, err := rand.Read(iv[:]); err != nil {
		return nil, nil, err
	}

	key := DeriveKey(password, salt[:], numCyclesPower)

	props := make([]byte, 2+len(salt)+len(iv))
	props[0] = (1 << 7) | (1 << 6) | (numCyclesPower & 0x3F)
	props[1] = byte(((len(salt) & 0x0F) << 4) | (len(iv) & 0x0F))
	copy(props[2:], salt[:])
	copy(props[2+len(salt):], iv[:])

	cw, err := crypto.NewAESCBCWriter(w, key, iv)
	if err != nil {
		return nil, nil, err
	}

	return cw, props, nil
}
