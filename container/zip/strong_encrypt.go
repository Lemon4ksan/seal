// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"compress/flate"
	"crypto/aes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/foundation/binkit"

	"github.com/lemon4ksan/seal/crypto"
)

// ArchiveExtraData represents the PKWARE 0x08064b50 record preceding an encrypted Central Directory.
type ArchiveExtraData struct {
	ExtraFieldLength uint32
	ExtraField       []byte
}

// ReadArchiveExtraData parses the Archive Extra Data record from src.
func ReadArchiveExtraData(src io.Reader) (ArchiveExtraData, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(src, lenBuf[:]); err != nil {
		return ArchiveExtraData{}, fmt.Errorf("read archive extra data length: %w", err)
	}
	extraLen := binary.LittleEndian.Uint32(lenBuf[:])
	extra := make([]byte, extraLen)
	if _, err := io.ReadFull(src, extra); err != nil {
		return ArchiveExtraData{}, fmt.Errorf("read archive extra data payload: %w", err)
	}
	return ArchiveExtraData{ExtraFieldLength: extraLen, ExtraField: extra}, nil
}

// EncodeArchiveExtraData serializes the Archive Extra Data record.
func EncodeArchiveExtraData(extra []byte) []byte {
	w := binkit.NewWriter(nil, 8+len(extra))
	w.U32LE(ArchiveExtraDataSignature).
		U32LE(uint32(len(extra))).
		Raw(extra)
	return w.Bytes()
}

// StrongEncryptionHeader represents the parsed Tag 0x0017 Strong Encryption extra field.
type StrongEncryptionHeader struct {
	FormatVersion uint16
	AlgId         uint16 // AlgAES128, AlgAES192, AlgAES256, etc.
	BitLen        uint16 // 128, 192, 256
	Flags         uint16 // Bit 0: Password, Bit 1: Certificate
	ErdData       []byte // Encrypted Random Data
	Validation    []byte // Validation Data
	ValidationCRC uint32
}

// ParseStrongEncryptionHeader extracts parameters from Tag 0x0017 extra field payload.
func ParseStrongEncryptionHeader(data []byte) (StrongEncryptionHeader, bool) {
	if len(data) >= 4 && binary.LittleEndian.Uint16(data[0:2]) == TagStrongEncryptHeader {
		data = data[4:]
	}
	if len(data) < 8 {
		return StrongEncryptionHeader{}, false
	}
	r := binkit.NewReader(data)
	h := StrongEncryptionHeader{
		FormatVersion: r.U16LE(),
		AlgId:         r.U16LE(),
		BitLen:        r.U16LE(),
		Flags:         r.U16LE(),
	}
	if r.Remaining() >= 2 {
		erdSize := int(r.U16LE())
		if r.Remaining() >= erdSize {
			h.ErdData = r.Bytes(erdSize)
		}
	}
	if r.Remaining() >= 4 {
		r.Skip(4) // Reserved
	}
	if r.Remaining() >= 2 {
		vSize := int(r.U16LE())
		if r.Remaining() >= vSize {
			h.Validation = r.Bytes(vSize)
		}
	}
	if r.Remaining() >= 4 {
		h.ValidationCRC = r.U32LE()
	}
	return h, true
}

// EncodeStrongEncryptionHeader formats Tag 0x0017 Strong Encryption extra field.
func EncodeStrongEncryptionHeader(algId, bitLen uint16, erdData, vData []byte, vCRC uint32) []byte {
	dataLen := 2 + 2 + 2 + 2 + 2 + len(erdData) + 4 + 2 + len(vData) + 4
	w := binkit.NewWriter(nil, 4+dataLen)
	w.U16LE(TagStrongEncryptHeader).
		U16LE(uint16(dataLen)).
		U16LE(3).      // FormatVersion = 3
		U16LE(algId).  // AlgId
		U16LE(bitLen). // BitLen
		U16LE(0x0001). // Flags (Password encrypted)
		U16LE(uint16(len(erdData))).
		Raw(erdData).
		U32LE(0). // Reserved
		U16LE(uint16(len(vData))).
		Raw(vData).
		U32LE(vCRC)
	return w.Bytes()
}

// EncryptCentralDirectory compresses and encrypts the Central Directory payload using AES-256 (v6.2+).
func EncryptCentralDirectory(
	cdBytes []byte,
	password string,
	compress bool,
) (encBytes []byte, method uint16, uncompSize, compSize int64, err error) {
	uncompSize = int64(len(cdBytes))
	payload := cdBytes
	method = MethodStore

	if compress {
		var compBuf bytes.Buffer
		zw, err := flate.NewWriter(&compBuf, flate.DefaultCompression)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		if _, err := zw.Write(cdBytes); err != nil {
			return nil, 0, 0, 0, err
		}
		if err := zw.Close(); err != nil {
			return nil, 0, 0, 0, err
		}
		payload = compBuf.Bytes()
		method = MethodDeflate
	}

	// Generate 16-byte salt and 2-byte password verification value
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, 0, 0, 0, err
	}

	key := crypto.PBKDF2([]byte(password), salt, 1000, 32+32+2, sha1.New)
	encKey := key[:32]
	authKey := key[32:64]
	pvv := key[64:66]

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	// Encrypt payload with AES-CTR
	ctr := crypto.NewLittleEndianCTR(block)
	ciphertext := make([]byte, len(payload))
	ctr.XORKeyStream(ciphertext, payload)

	// Compute HMAC-SHA1 authentication code
	mac := hmac.New(sha1.New, authKey)
	mac.Write(ciphertext)
	authCode := mac.Sum(nil)[:10]

	// Output format: [Salt:16B][PVV:2B][Ciphertext][AuthCode:10B]
	var out bytes.Buffer
	out.Write(salt)
	out.Write(pvv)
	out.Write(ciphertext)
	out.Write(authCode)

	return out.Bytes(), method, uncompSize, int64(out.Len()), nil
}

// DecryptCentralDirectory decrypts and decompresses the encrypted Central Directory payload.
func DecryptCentralDirectory(encBytes []byte, password string, method uint16) ([]byte, error) {
	if len(encBytes) < 16+2+10 {
		return nil, errors.New("zip: encrypted central directory payload too short")
	}

	salt := encBytes[:16]
	pvv := encBytes[16:18]
	ciphertext := encBytes[18 : len(encBytes)-10]
	authCode := encBytes[len(encBytes)-10:]

	key := crypto.PBKDF2([]byte(password), salt, 1000, 32+32+2, sha1.New)
	encKey := key[:32]
	authKey := key[32:64]
	expectedPVV := key[64:66]

	if !bytes.Equal(pvv, expectedPVV) {
		return nil, errors.New("zip: invalid password for encrypted central directory")
	}

	mac := hmac.New(sha1.New, authKey)
	mac.Write(ciphertext)
	expectedAuth := mac.Sum(nil)[:10]

	if !bytes.Equal(authCode, expectedAuth) {
		return nil, errors.New("zip: central directory authentication check failed")
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}

	ctr := crypto.NewLittleEndianCTR(block)
	plaintext := make([]byte, len(ciphertext))
	ctr.XORKeyStream(plaintext, ciphertext)

	if method == MethodDeflate {
		zr := flate.NewReader(bytes.NewReader(plaintext))
		defer zr.Close()
		decompressed, err := io.ReadAll(zr)
		if err != nil {
			return nil, fmt.Errorf("decompress central directory: %w", err)
		}
		return decompressed, nil
	}

	return plaintext, nil
}
