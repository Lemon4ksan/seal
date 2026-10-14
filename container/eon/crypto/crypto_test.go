// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/crypto"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

func TestCrypto_KeySlotsAndAEAD(t *testing.T) {
	ciphers := []uint8{spec.CipherAES256GCM, spec.CipherChaCha20Poly1305}

	for _, cipherID := range ciphers {
		password := "SuperSecretPassword123!"
		// Fast parameters for testing
		encr, vd, err := crypto.CreateENCR(password, cipherID, 1024, 1, 1)
		if err != nil {
			t.Fatalf("CreateENCR (cipher %d) failed: %v", cipherID, err)
		}

		// Serialization roundtrip
		var buf bytes.Buffer
		written, err := encr.Write(&buf)
		if err != nil {
			t.Fatalf("ENCR.Write failed: %v", err)
		}

		if written != int64(spec.ENCRHeaderSize+spec.KeySlotSize) {
			t.Errorf("unexpected written size: %d", written)
		}

		parsed, err := crypto.ReadENCR(&buf)
		if err != nil {
			t.Fatalf("ReadENCR failed: %v", err)
		}

		// Unlock with correct password
		unlockedVD, err := parsed.Unlock(password)
		if err != nil {
			t.Fatalf("Unlock with valid password failed: %v", err)
		}
		if !bytes.Equal(unlockedVD.DEK[:], vd.DEK[:]) {
			t.Fatalf("unlocked DEK does not match original DEK")
		}
		if unlockedVD.VaultName != "default" {
			t.Fatalf("unexpected vault name: %s", unlockedVD.VaultName)
		}

		// Unlock with wrong password
		_, err = parsed.Unlock("wrong-password")
		if !errors.Is(err, spec.ErrInvalidPassword) {
			t.Fatalf("expected ErrInvalidPassword, got %v", err)
		}

		// Test chunk payload sealing with derived sub-keys
		chunkKey := crypto.DeriveSubkey(vd.DEK[:], "eon-chunk-payload-key", 32)
		aead, err := crypto.NewAEAD(cipherID, chunkKey)
		if err != nil {
			t.Fatalf("NewAEAD failed: %v", err)
		}

		nonce := crypto.DeriveChunkNonce(encr.MasterIV[:], 42)
		aad := []byte("chunk-aad-binding")
		plaintext := []byte("hello encrypted chunk world")

		ciphertext := aead.Seal(nil, nonce, plaintext, aad)

		// Decrypt
		decrypted, err := aead.Open(nil, nonce, ciphertext, aad)
		if err != nil {
			t.Fatalf("chunk decrypt failed: %v", err)
		}
		if string(decrypted) != string(plaintext) {
			t.Fatalf("chunk plaintext mismatch: %s != %s", decrypted, plaintext)
		}

		// Tamper AAD
		_, err = aead.Open(nil, nonce, ciphertext, []byte("tampered-aad"))
		if err == nil {
			t.Fatal("expected decryption failure with tampered AAD, got nil")
		}
	}
}

func TestCrypto_MultiVaultSlots(t *testing.T) {
	encr, vd1, err := crypto.CreateENCR("AlicePass2026!", spec.CipherAES256GCM, 1024, 1, 1)
	if err != nil {
		t.Fatalf("create encr: %v", err)
	}
	vd1.VaultName = "alice_vault"

	var dekBob [32]byte
	copy(dekBob[:], []byte("01234567890123456789012345678901"))
	vd2 := &crypto.VaultDescriptor{
		VaultID:    1,
		VaultFlags: spec.VaultFlagHidden,
		VaultName:  "bob_vault",
		DirOffset:  0x10000,
		DirSize:    512,
		DEK:        dekBob,
	}

	_, err = encr.AddVaultSlot("BobPass2026!", vd2, 1024, 1, 1)
	if err != nil {
		t.Fatalf("add bob slot: %v", err)
	}

	if len(encr.Slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(encr.Slots))
	}

	// Serialize & deserialize
	var buf bytes.Buffer
	if _, err := encr.Write(&buf); err != nil {
		t.Fatalf("write encr: %v", err)
	}

	readENCR, err := crypto.ReadENCR(&buf)
	if err != nil {
		t.Fatalf("read encr: %v", err)
	}

	// Unlock with Alice's password
	aliceRes, err := readENCR.Unlock("AlicePass2026!")
	if err != nil {
		t.Fatalf("unlock alice: %v", err)
	}
	if aliceRes.VaultID != 0 {
		t.Errorf("expected vault ID 0, got %d", aliceRes.VaultID)
	}

	// Unlock with Bob's password
	bobRes, err := readENCR.Unlock("BobPass2026!")
	if err != nil {
		t.Fatalf("unlock bob: %v", err)
	}
	if bobRes.VaultName != "bob_vault" || bobRes.DirOffset != 0x10000 {
		t.Errorf("unexpected bob descriptor: %+v", bobRes)
	}

	// UnlockAll with both passwords
	bothRes, err := readENCR.UnlockAll("AlicePass2026!", "BobPass2026!")
	if err != nil {
		t.Fatalf("unlock all: %v", err)
	}
	if len(bothRes) != 2 {
		t.Fatalf("expected 2 unlocked vaults, got %d", len(bothRes))
	}
}

func TestCrypto_KMSEnvelope(t *testing.T) {
	ctx := context.Background()
	kms := crypto.NewMockKMSClient()
	keyID := "mock://aeon-master-key"

	// 1. Create KMS ENCR
	encr, vd, err := crypto.CreateKMSENCR(ctx, kms, keyID, spec.CipherAES256GCM)
	if err != nil {
		t.Fatalf("CreateKMSENCR failed: %v", err)
	}

	if len(encr.Slots) != 1 {
		t.Fatalf("expected 1 slot, got %d", len(encr.Slots))
	}
	if encr.Slots[0].SlotType != spec.SlotTypeKMS {
		t.Fatalf("expected SlotTypeKMS, got %d", encr.Slots[0].SlotType)
	}
	if encr.KMSKeyID != keyID {
		t.Fatalf("key ID mismatch: %s != %s", encr.KMSKeyID, keyID)
	}

	// 2. Serialize & Deserialize
	var buf bytes.Buffer
	if _, err := encr.Write(&buf); err != nil {
		t.Fatalf("write encr: %v", err)
	}

	readENCR, err := crypto.ReadENCR(&buf)
	if err != nil {
		t.Fatalf("read encr: %v", err)
	}

	// 3. Unlock with valid KMS
	unlockedVD, err := readENCR.UnlockWithKMS(ctx, kms)
	if err != nil {
		t.Fatalf("UnlockWithKMS failed: %v", err)
	}
	if !bytes.Equal(unlockedVD.DEK[:], vd.DEK[:]) {
		t.Fatalf("unlocked DEK mismatch")
	}

	// 4. Unlock with wrong KMS
	wrongKMS := crypto.NewMockKMSClient()
	_, err = readENCR.UnlockWithKMS(ctx, wrongKMS)
	if err == nil {
		t.Fatal("expected failure with wrong KMS client, got success")
	}
}

func TestCrypto_DualSlotPasswordAndKMS(t *testing.T) {
	ctx := context.Background()
	kms := crypto.NewMockKMSClient()
	keyID := "vault://transit/keys/aeon-backup"
	password := "DualSlotSecret123!"

	// 1. Create with password
	encr, vd, err := crypto.CreateENCR(password, spec.CipherAES256GCM, 1024, 1, 1)
	if err != nil {
		t.Fatalf("CreateENCR failed: %v", err)
	}

	// 2. Add KMS slot to the same ENCR record for the same vault
	_, err = encr.AddKMSSlot(ctx, kms, keyID, vd)
	if err != nil {
		t.Fatalf("AddKMSSlot failed: %v", err)
	}

	if len(encr.Slots) != 2 {
		t.Fatalf("expected 2 slots, got %d", len(encr.Slots))
	}

	// 3. Serialize & Deserialize
	var buf bytes.Buffer
	if _, err := encr.Write(&buf); err != nil {
		t.Fatalf("write encr: %v", err)
	}

	readENCR, err := crypto.ReadENCR(&buf)
	if err != nil {
		t.Fatalf("read encr: %v", err)
	}

	// 4. Unlock independently via password
	vdFromPass, err := readENCR.Unlock(password)
	if err != nil {
		t.Fatalf("unlock via password failed: %v", err)
	}
	if !bytes.Equal(vdFromPass.DEK[:], vd.DEK[:]) {
		t.Fatalf("password DEK mismatch")
	}

	// 5. Unlock independently via KMS
	vdFromKMS, err := readENCR.UnlockWithKMS(ctx, kms)
	if err != nil {
		t.Fatalf("unlock via KMS failed: %v", err)
	}
	if !bytes.Equal(vdFromKMS.DEK[:], vd.DEK[:]) {
		t.Fatalf("KMS DEK mismatch")
	}
}
