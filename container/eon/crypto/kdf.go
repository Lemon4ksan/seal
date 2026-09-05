// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/foundation/cryptokit/kdf"
)

// DeriveKey derives a 256-bit Key Encryption Key (KEK) using the designated slot KDF mechanism.
func DeriveKey(slotType uint8, password string, salt []byte, memKiB, iter uint32, threads uint8) ([]byte, error) {
	passBytes := []byte(password)

	switch slotType {
	case spec.SlotTypeArgon2id:
		return kdf.Argon2id(passBytes, salt, kdf.Argon2idProfile{
			MemoryKiB:   memKiB,
			Iterations:  iter,
			Parallelism: threads,
		}, 32), nil

	case spec.SlotTypePBKDF2SHA256:
		if iter == 0 {
			iter = spec.DefaultPBKDF2Iterations
		}
		return kdf.PBKDF2SHA256(passBytes, salt, int(iter), 32), nil

	default:
		return nil, fmt.Errorf("crypto: unsupported slot type 0x%02X", slotType)
	}
}

// DeriveSubkey expands a master key into a domain-separated subkey using HKDF-SHA256.
func DeriveSubkey(masterKey []byte, info string, length int) []byte {
	return kdf.DeriveSubkey(masterKey, info, length)
}

// DeriveChunkNonce derives a deterministic, collision-free 12-byte AEAD nonce for chunk i.
func DeriveChunkNonce(masterIV []byte, chunkIdx uint32) []byte {
	var info [10]byte
	copy(info[0:6], "chunk-")
	binary.BigEndian.PutUint32(info[6:10], chunkIdx)

	nonce, err := kdf.Expand(sha256.New, masterIV, info[:], 12)
	if err != nil {
		panic(fmt.Sprintf("crypto: derive chunk nonce: %v", err))
	}
	return nonce
}

// DeriveDirectoryNonce derives a deterministic 12-byte AEAD nonce for the Central Directory.
func DeriveDirectoryNonce(masterIV []byte) []byte {
	return kdf.DeriveSubkey(masterIV, "central-directory", 12)
}
