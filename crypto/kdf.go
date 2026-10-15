// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"unicode/utf16"
)

// PBKDF2 implements the Password-Based Key Derivation Function 2 (RFC 2898 / PKCS #5 v2.0).
func PBKDF2(password, salt []byte, iter, keyLen int, h func() hash.Hash) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var (
		buf  [4]byte
		dk   = make([]byte, 0, numBlocks*hashLen)
		u    = make([]byte, hashLen)
		uXor = make([]byte, hashLen)
	)

	for block := 1; block <= numBlocks; block++ {
		// U_1 = PRF(password, salt || INT_32_BE(block))
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf[:], uint32(block))
		prf.Write(buf[:])
		copy(u, prf.Sum(nil))
		copy(uXor, u)

		// U_2 ... U_c = PRF(password, U_{i-1})
		for i := 2; i <= iter; i++ {
			prf.Reset()
			prf.Write(u)
			copy(u, prf.Sum(nil))
			for k := range hashLen {
				uXor[k] ^= u[k]
			}
		}
		dk = append(dk, uXor...)
	}

	return dk[:keyLen]
}

// Derive7zKey implements the 7-Zip SHA-256 key stretching algorithm.
func Derive7zKey(password string, salt []byte, numCyclesPower byte) [32]byte {
	runes := []rune(password)
	u16 := utf16.Encode(runes)
	passBytes := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(passBytes[i*2:(i+1)*2], v)
	}

	h := sha256.New()
	if numCyclesPower > 24 {
		numCyclesPower = 24
	}
	numCycles := uint64(1) << numCyclesPower

	var counterBuf [8]byte
	for cycle := range numCycles {
		h.Write(salt)
		h.Write(passBytes)
		binary.LittleEndian.PutUint64(counterBuf[:], cycle)
		h.Write(counterBuf[:])
	}

	var key [32]byte
	copy(key[:], h.Sum(nil))
	return key
}
