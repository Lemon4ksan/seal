// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/seal/container/eon/spec"
)

func slotAAD(slotIndex uint8) []byte {
	return []byte(fmt.Sprintf("eon-key-slot-v2-%d", slotIndex))
}

func deriveSlotNonce(masterIV [16]byte, slotIndex uint8) []byte {
	nonce := make([]byte, 12)
	copy(nonce, masterIV[0:12])
	nonce[11] ^= slotIndex
	return nonce
}

// VaultDescriptor describes an isolated virtual vault encapsulated within a key slot.
type VaultDescriptor struct {
	VaultID    uint8
	VaultFlags uint8
	VaultName  string
	DirOffset  uint64
	DirSize    uint64
	DEK        [32]byte
}

// Serialize encodes the VaultDescriptor into an 80-byte buffer.
func (vd *VaultDescriptor) Serialize() [spec.VaultPlaintextSize]byte {
	var buf [spec.VaultPlaintextSize]byte
	copy(buf[0:32], vd.DEK[:])
	binary.LittleEndian.PutUint64(buf[32:40], vd.DirOffset)
	binary.LittleEndian.PutUint64(buf[40:48], vd.DirSize)
	buf[48] = vd.VaultID
	buf[49] = vd.VaultFlags
	nameBytes := []byte(vd.VaultName)
	if len(nameBytes) > 22 {
		nameBytes = nameBytes[:22]
	}
	copy(buf[50:72], nameBytes)
	// buf[72:80] reserved zeros
	return buf
}

// DeserializeVaultDescriptor parses an 80-byte buffer into a VaultDescriptor.
func DeserializeVaultDescriptor(buf []byte) (*VaultDescriptor, error) {
	if len(buf) < spec.VaultPlaintextSize {
		return nil, spec.ErrTruncatedData
	}
	vd := &VaultDescriptor{
		DirOffset:  binary.LittleEndian.Uint64(buf[32:40]),
		DirSize:    binary.LittleEndian.Uint64(buf[40:48]),
		VaultID:    buf[48],
		VaultFlags: buf[49],
		VaultName:  strings.TrimRight(string(buf[50:72]), "\x00"),
	}
	copy(vd.DEK[:], buf[0:32])
	return vd, nil
}

// KeySlot records authentication and key wrapping parameters for a single credential and vault.
type KeySlot struct {
	SlotIndex      uint8
	SlotType       uint8
	SlotFlags      uint16
	Salt           [32]byte
	KDFMemoryKiB   uint32
	KDFIterations  uint32
	KDFParallelism uint8
	WrappedPayload [spec.VaultWrappedSize]byte // 80-byte VaultDescriptor + 16-byte AEAD tag
}

// Wrap seals the VaultDescriptor into WrappedPayload using password.
func (s *KeySlot) Wrap(password string, cipherID uint8, masterIV [16]byte, vd *VaultDescriptor) error {
	kek, err := DeriveKey(s.SlotType, password, s.Salt[:], s.KDFMemoryKiB, s.KDFIterations, s.KDFParallelism)
	if err != nil {
		return fmt.Errorf("derive kek: %w", err)
	}
	return s.WrapWithKEK(kek, cipherID, masterIV, vd)
}

// WrapWithKEK seals the VaultDescriptor directly using a 32-byte KEK.
func (s *KeySlot) WrapWithKEK(kek []byte, cipherID uint8, masterIV [16]byte, vd *VaultDescriptor) error {
	aead, err := NewAEAD(cipherID, kek)
	if err != nil {
		return fmt.Errorf("create aead: %w", err)
	}

	nonce := deriveSlotNonce(masterIV, s.SlotIndex)
	pt := vd.Serialize()
	aad := slotAAD(s.SlotIndex)

	sealed := aead.Seal(nil, nonce, pt[:], aad)
	if len(sealed) != spec.VaultWrappedSize {
		return fmt.Errorf("wrapped payload size mismatch: %d != %d", len(sealed), spec.VaultWrappedSize)
	}

	copy(s.WrappedPayload[:], sealed)
	return nil
}

// Unlock attempts to decrypt and unwrap the VaultDescriptor using password.
func (s *KeySlot) Unlock(password string, cipherID uint8, masterIV [16]byte) (*VaultDescriptor, error) {
	kek, err := DeriveKey(s.SlotType, password, s.Salt[:], s.KDFMemoryKiB, s.KDFIterations, s.KDFParallelism)
	if err != nil {
		return nil, err
	}
	return s.UnlockWithKEK(kek, cipherID, masterIV)
}

// UnlockWithKEK attempts to decrypt and unwrap the VaultDescriptor directly using a 32-byte KEK.
func (s *KeySlot) UnlockWithKEK(kek []byte, cipherID uint8, masterIV [16]byte) (*VaultDescriptor, error) {
	aead, err := NewAEAD(cipherID, kek)
	if err != nil {
		return nil, err
	}

	nonce := deriveSlotNonce(masterIV, s.SlotIndex)
	aad := slotAAD(s.SlotIndex)

	pt, err := aead.Open(nil, nonce, s.WrappedPayload[:], aad)
	if err != nil {
		return nil, spec.ErrInvalidPassword
	}

	return DeserializeVaultDescriptor(pt)
}

const MagicKMSE uint32 = 0x45534D4B // 'KMSE' LE

// ENCR manages multi-key slots and container encryption envelope parameters.
type ENCR struct {
	CipherID      uint8
	MasterIV      [16]byte
	Slots         []KeySlot
	KMSKeyID      string
	KMSCiphertext []byte
}

// Write serializes the ENCR header and all KeySlots to w.
func (e *ENCR) Write(w io.Writer) (int64, error) {
	var hdr [spec.ENCRHeaderSize]byte
	copy(hdr[0:4], spec.MagicENCR[:])
	hdr[4] = e.CipherID
	hdr[5] = uint8(len(e.Slots))
	binary.LittleEndian.PutUint16(hdr[6:8], 0) // Reserved
	copy(hdr[8:24], e.MasterIV[:])

	if _, err := w.Write(hdr[:]); err != nil {
		return 0, err
	}
	total := int64(spec.ENCRHeaderSize)

	for _, slot := range e.Slots {
		var buf [spec.KeySlotSize]byte
		buf[0] = slot.SlotIndex
		buf[1] = slot.SlotType
		binary.LittleEndian.PutUint16(buf[2:4], slot.SlotFlags)
		copy(buf[4:36], slot.Salt[:])
		binary.LittleEndian.PutUint32(buf[36:40], slot.KDFMemoryKiB)
		binary.LittleEndian.PutUint32(buf[40:44], slot.KDFIterations)
		buf[44] = slot.KDFParallelism
		// buf[45:48] = 0 (reserved pad)
		copy(buf[48:144], slot.WrappedPayload[:])

		if _, err := w.Write(buf[:]); err != nil {
			return total, err
		}
		total += spec.KeySlotSize
	}

	if len(e.KMSCiphertext) > 0 {
		var kmsHdr [10]byte
		binary.LittleEndian.PutUint32(kmsHdr[0:4], MagicKMSE)
		binary.LittleEndian.PutUint16(kmsHdr[4:6], uint16(len(e.KMSKeyID)))
		binary.LittleEndian.PutUint32(kmsHdr[6:10], uint32(len(e.KMSCiphertext)))
		if _, err := w.Write(kmsHdr[:]); err != nil {
			return total, err
		}
		total += 10

		if len(e.KMSKeyID) > 0 {
			if _, err := w.Write([]byte(e.KMSKeyID)); err != nil {
				return total, err
			}
			total += int64(len(e.KMSKeyID))
		}

		if _, err := w.Write(e.KMSCiphertext); err != nil {
			return total, err
		}
		total += int64(len(e.KMSCiphertext))
	}

	return total, nil
}

// ReadENCR parses the ENCR header and all contained KeySlots from r.
func ReadENCR(r io.Reader) (*ENCR, error) {
	var hdr [spec.ENCRHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}

	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicENCRLE {
		return nil, spec.ErrInvalidENCR
	}

	cipherID := hdr[4]
	slotCount := int(hdr[5])
	if slotCount > spec.MaxKeySlots {
		return nil, fmt.Errorf("%w: key slot count %d exceeds maximum (%d)", spec.ErrInvalidENCR, slotCount, spec.MaxKeySlots)
	}
	var masterIV [16]byte
	copy(masterIV[:], hdr[8:24])

	slots := make([]KeySlot, slotCount)
	for i := 0; i < slotCount; i++ {
		var buf [spec.KeySlotSize]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, fmt.Errorf("read key slot %d: %w", i, err)
		}

		slot := KeySlot{
			SlotIndex:      buf[0],
			SlotType:       buf[1],
			SlotFlags:      binary.LittleEndian.Uint16(buf[2:4]),
			KDFMemoryKiB:   binary.LittleEndian.Uint32(buf[36:40]),
			KDFIterations:  binary.LittleEndian.Uint32(buf[40:44]),
			KDFParallelism: buf[44],
		}
		copy(slot.Salt[:], buf[4:36])
		copy(slot.WrappedPayload[:], buf[48:144])
		slots[i] = slot
	}

	var kmsKeyID string
	var kmsCiphertext []byte

	hasKMSSlot := false
	for _, s := range slots {
		if s.SlotType == spec.SlotTypeKMS || s.SlotType == spec.SlotTypeVault {
			hasKMSSlot = true
			break
		}
	}

	if hasKMSSlot {
		var kmsHdr [10]byte
		if _, err := io.ReadFull(r, kmsHdr[:]); err == nil && binary.LittleEndian.Uint32(kmsHdr[0:4]) == MagicKMSE {
			keyIDLen := int(binary.LittleEndian.Uint16(kmsHdr[4:6]))
			ctLen := int(binary.LittleEndian.Uint32(kmsHdr[6:10]))
			if keyIDLen > 0 && keyIDLen <= 4096 {
				kidBuf := make([]byte, keyIDLen)
				if _, err := io.ReadFull(r, kidBuf); err == nil {
					kmsKeyID = string(kidBuf)
				}
			}
			if ctLen > 0 && ctLen <= 65536 {
				ctBuf := make([]byte, ctLen)
				if _, err := io.ReadFull(r, ctBuf); err == nil {
					kmsCiphertext = ctBuf
				}
			}
		}
	}

	return &ENCR{
		CipherID:      cipherID,
		MasterIV:      masterIV,
		Slots:         slots,
		KMSKeyID:      kmsKeyID,
		KMSCiphertext: kmsCiphertext,
	}, nil
}

// UnlockWithKMS attempts to decrypt the container using the provided KMS client.
func (e *ENCR) UnlockWithKMS(ctx context.Context, client KMSClient) (*VaultDescriptor, error) {
	if client == nil || len(e.KMSCiphertext) == 0 {
		return nil, errors.New("crypto: no KMS credentials or ciphertext available")
	}

	kek, err := client.Decrypt(ctx, e.KMSKeyID, e.KMSCiphertext)
	if err != nil {
		return nil, fmt.Errorf("kms decrypt: %w", err)
	}

	for _, slot := range e.Slots {
		if slot.SlotType == spec.SlotTypeKMS || slot.SlotType == spec.SlotTypeVault {
			vd, err := slot.UnlockWithKEK(kek, e.CipherID, e.MasterIV)
			if err == nil {
				return vd, nil
			}
		}
	}

	return nil, errors.New("crypto: KMS key unlocked but slot unwrapping failed")
}

// Unlock attempts to decrypt the first matching VaultDescriptor across all active key slots.
func (e *ENCR) Unlock(password string) (*VaultDescriptor, error) {
	if len(e.Slots) == 0 {
		return nil, spec.ErrInvalidPassword
	}

	for _, slot := range e.Slots {
		vd, err := slot.Unlock(password, e.CipherID, e.MasterIV)
		if err == nil {
			return vd, nil
		}
	}

	return nil, spec.ErrInvalidPassword
}

// UnlockAll attempts to unlock all slots matching any of the provided passphrases.
func (e *ENCR) UnlockAll(passwords ...string) ([]*VaultDescriptor, error) {
	if len(e.Slots) == 0 || len(passwords) == 0 {
		return nil, spec.ErrInvalidPassword
	}

	var results []*VaultDescriptor
	seen := make(map[uint8]bool)

	for _, pwd := range passwords {
		if pwd == "" {
			continue
		}
		for _, slot := range e.Slots {
			if seen[slot.SlotIndex] {
				continue
			}
			vd, err := slot.Unlock(pwd, e.CipherID, e.MasterIV)
			if err == nil {
				seen[slot.SlotIndex] = true
				results = append(results, vd)
			}
		}
	}

	if len(results) == 0 {
		return nil, spec.ErrInvalidPassword
	}

	return results, nil
}

// AddVaultSlot creates and seals a new key slot for the specified vault descriptor.
func (e *ENCR) AddVaultSlot(password string, vd *VaultDescriptor, memKiB, iter uint32, threads uint8) (*KeySlot, error) {
	if len(e.Slots) >= 16 {
		return nil, fmt.Errorf("maximum key slots (16) reached")
	}

	var salt [32]byte
	if _, err := io.ReadFull(rand.Reader, salt[:]); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	slotIdx := uint8(len(e.Slots))
	slot := KeySlot{
		SlotIndex:      slotIdx,
		SlotType:       spec.SlotTypeArgon2id,
		SlotFlags:      uint16(vd.VaultFlags),
		Salt:           salt,
		KDFMemoryKiB:   memKiB,
		KDFIterations:  iter,
		KDFParallelism: threads,
	}

	if err := slot.Wrap(password, e.CipherID, e.MasterIV, vd); err != nil {
		return nil, err
	}

	e.Slots = append(e.Slots, slot)
	return &e.Slots[len(e.Slots)-1], nil
}

// CreateENCR generates an initial ENCR record with a randomly generated 256-bit DEK wrapped in Slot 0 (default vault).
func CreateENCR(password string, cipherID uint8, memKiB, iter uint32, threads uint8) (*ENCR, *VaultDescriptor, error) {
	var dek [32]byte
	if _, err := io.ReadFull(rand.Reader, dek[:]); err != nil {
		return nil, nil, fmt.Errorf("generate dek: %w", err)
	}

	var masterIV [16]byte
	if _, err := io.ReadFull(rand.Reader, masterIV[:]); err != nil {
		return nil, nil, fmt.Errorf("generate master iv: %w", err)
	}

	encr := &ENCR{
		CipherID: cipherID,
		MasterIV: masterIV,
	}

	vd := &VaultDescriptor{
		VaultID:    0,
		VaultFlags: 0,
		VaultName:  "default",
		DEK:        dek,
	}

	_, err := encr.AddVaultSlot(password, vd, memKiB, iter, threads)
	if err != nil {
		return nil, nil, err
	}

	return encr, vd, nil
}

// AddKMSSlot wraps the VaultDescriptor using an envelope KEK encrypted via the provided KMS client.
func (e *ENCR) AddKMSSlot(ctx context.Context, client KMSClient, keyID string, vd *VaultDescriptor) (*KeySlot, error) {
	if len(e.Slots) >= 16 {
		return nil, fmt.Errorf("maximum key slots (16) reached")
	}
	if client == nil {
		return nil, errors.New("crypto: nil KMSClient provided")
	}

	var kek [32]byte
	if _, err := io.ReadFull(rand.Reader, kek[:]); err != nil {
		return nil, fmt.Errorf("generate kek: %w", err)
	}

	kmsCiphertext, err := client.Encrypt(ctx, keyID, kek[:])
	if err != nil {
		return nil, fmt.Errorf("kms encrypt kek: %w", err)
	}

	e.KMSKeyID = keyID
	e.KMSCiphertext = kmsCiphertext

	slotType := uint8(spec.SlotTypeKMS)
	if strings.HasPrefix(keyID, "vault://") {
		slotType = spec.SlotTypeVault
	}

	slotIdx := uint8(len(e.Slots))
	slot := KeySlot{
		SlotIndex: slotIdx,
		SlotType:  slotType,
		SlotFlags: uint16(vd.VaultFlags),
	}

	if err := slot.WrapWithKEK(kek[:], e.CipherID, e.MasterIV, vd); err != nil {
		return nil, err
	}

	e.Slots = append(e.Slots, slot)
	return &e.Slots[len(e.Slots)-1], nil
}

// CreateKMSENCR generates an initial ENCR record with a randomly generated 256-bit DEK wrapped in a KMS KeySlot.
func CreateKMSENCR(ctx context.Context, client KMSClient, keyID string, cipherID uint8) (*ENCR, *VaultDescriptor, error) {
	var dek [32]byte
	if _, err := io.ReadFull(rand.Reader, dek[:]); err != nil {
		return nil, nil, fmt.Errorf("generate dek: %w", err)
	}

	var masterIV [16]byte
	if _, err := io.ReadFull(rand.Reader, masterIV[:]); err != nil {
		return nil, nil, fmt.Errorf("generate master iv: %w", err)
	}

	encr := &ENCR{
		CipherID: cipherID,
		MasterIV: masterIV,
	}

	vd := &VaultDescriptor{
		VaultID:    0,
		VaultFlags: 0,
		VaultName:  "default",
		DEK:        dek,
	}

	_, err := encr.AddKMSSlot(ctx, client, keyID, vd)
	if err != nil {
		return nil, nil, err
	}

	return encr, vd, nil
}

