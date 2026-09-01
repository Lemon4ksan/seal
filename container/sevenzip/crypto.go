// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"unicode/utf16"
)

// ErrInvalidPassword indicates wrong or missing 7z archive password.
var ErrInvalidPassword = errors.New("sevenzip: invalid or missing password for encrypted archive")

// DeriveKey implements 7-Zip SHA-256 key derivation with multi-round stretching.
func DeriveKey(password string, salt []byte, numCyclesPower byte) [32]byte {
	// Encode password into UTF-16LE
	runes := []rune(password)
	u16 := utf16.Encode(runes)
	passBytes := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(passBytes[i*2:(i+1)*2], v)
	}

	h := sha256.New()
	numCycles := uint64(1) << numCyclesPower

	var counterBuf [8]byte
	for cycle := uint64(0); cycle < numCycles; cycle++ {
		h.Write(salt)
		h.Write(passBytes)
		binary.LittleEndian.PutUint64(counterBuf[:], cycle)
		h.Write(counterBuf[:])
	}

	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key
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
		propPos++
	}
	if ivBit != 0 && propPos < len(props) {
		ivSize = int(props[propPos] & 0x0F)
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

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	mode := cipher.NewCBCDecrypter(block, iv[:])
	return &cbcReader{r: r, mode: mode}, nil
}

type cbcReader struct {
	r    io.Reader
	mode cipher.BlockMode
}

func (cr *cbcReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	if n > 0 {
		// Decrypt in full 16-byte blocks
		aligned := (n / 16) * 16
		if aligned > 0 {
			cr.mode.CryptBlocks(p[:aligned], p[:aligned])
		}
	}
	return n, err
}
