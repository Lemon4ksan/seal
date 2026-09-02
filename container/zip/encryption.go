// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"io"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/crypto"
	"github.com/lemon4ksan/seal/file"
)

// EncryptionResult contains output stream wrapping details produced by a CipherHandler.
type EncryptionResult struct {
	Writer     io.WriteCloser // Wrapped encrypting writer (or nil if just generating header info)
	WireMethod uint16         // Compression method placed in binary header (e.g. 99 for WinZip AES, original for ZipCrypto)
	ExtraField []byte         // Extra field bytes generated for the cipher (e.g. tag 0x9901 for AES)
	BitFlags   uint16         // Bit flags to set on header (e.g. FlagEncrypted)
}

func getAESStrength(enc file.EncryptionMethod) byte {
	switch enc {
	case file.AES128:
		return AESStrength128
	case file.AES192:
		return AESStrength192
	default:
		return AESStrength256
	}
}

// CipherHandler encapsulates format-specific cryptography, metadata encoding, and stream encryption/decryption.
type CipherHandler interface {
	Method() file.EncryptionMethod
	Encrypt(w io.Writer, password string, method codec.Method, strength, checkByte byte) (EncryptionResult, error)
	Decrypt(
		r io.Reader,
		password string,
		compSize int64,
		rawExtra []byte,
		flags uint16,
		crc32Val uint32,
		dosTime uint16,
	) (io.Reader, error)
	Detect(flags, wireMethod uint16, rawExtra []byte) (file.EncryptionMethod, codec.Method, bool)
}

type winZipAESHandler struct{}

func (h *winZipAESHandler) Method() file.EncryptionMethod {
	return file.AES256
}

func (h *winZipAESHandler) Encrypt(
	w io.Writer,
	password string,
	method codec.Method,
	strength, _ byte,
) (EncryptionResult, error) {
	wireMethod := MethodWinZipAES
	extra := EncodeAESExtraField(uint16(method), strength)
	flags := FlagEncrypted

	if w == nil {
		return EncryptionResult{
			WireMethod: wireMethod,
			ExtraField: extra,
			BitFlags:   flags,
		}, nil
	}

	encW, err := NewAESWriter(w, password, strength)
	if err != nil {
		return EncryptionResult{}, err
	}

	return EncryptionResult{
		Writer:     encW,
		WireMethod: wireMethod,
		ExtraField: extra,
		BitFlags:   flags,
	}, nil
}

func (h *winZipAESHandler) Decrypt(
	r io.Reader,
	password string,
	compSize int64,
	rawExtra []byte,
	_ uint16,
	_ uint32,
	_ uint16,
) (io.Reader, error) {
	strength := AESStrength256
	if aesField, ok := FindExtraField(rawExtra, AESEncryptionTag); ok && len(aesField) >= 9 {
		strength = aesField[8]
	}
	return NewAESReader(r, password, compSize, strength)
}

func (h *winZipAESHandler) Detect(
	flags, wireMethod uint16,
	rawExtra []byte,
) (file.EncryptionMethod, codec.Method, bool) {
	if wireMethod == MethodWinZipAES {
		if aesField, ok := FindExtraField(rawExtra, AESEncryptionTag); ok && len(aesField) >= 11 {
			st := aesField[8]
			enc := file.AES256
			switch st {
			case AESStrength128:
				enc = file.AES128
			case AESStrength192:
				enc = file.AES192
			}
			actualMethod := codec.Method(uint16(aesField[9]) | (uint16(aesField[10]) << 8))
			return enc, actualMethod, true
		}
	}
	return file.NotEncrypted, 0, false
}

type zipCryptoHandler struct{}

func (h *zipCryptoHandler) Method() file.EncryptionMethod {
	return file.ZipCrypto
}

func (h *zipCryptoHandler) Encrypt(
	w io.Writer,
	password string,
	method codec.Method,
	_, checkByte byte,
) (EncryptionResult, error) {
	flags := FlagEncrypted
	if w == nil {
		return EncryptionResult{
			WireMethod: uint16(method),
			BitFlags:   flags,
		}, nil
	}
	encW, err := NewZipCryptoWriter(w, password, checkByte)
	if err != nil {
		return EncryptionResult{}, err
	}
	return EncryptionResult{
		Writer:     encW,
		WireMethod: uint16(method),
		BitFlags:   flags,
	}, nil
}

func (h *zipCryptoHandler) Decrypt(
	r io.Reader,
	password string,
	_ int64,
	_ []byte,
	flags uint16,
	crc32Val uint32,
	dosTime uint16,
) (io.Reader, error) {
	return NewZipCryptoReader(r, password, flags, crc32Val, dosTime)
}

func (h *zipCryptoHandler) Detect(
	flags, wireMethod uint16,
	rawExtra []byte,
) (file.EncryptionMethod, codec.Method, bool) {
	if flags&FlagEncrypted != 0 && wireMethod != MethodWinZipAES {
		return file.ZipCrypto, codec.Method(wireMethod), true
	}
	return file.NotEncrypted, 0, false
}

var (
	aesCipherHandler       = &winZipAESHandler{}
	zipCryptoCipherHandler = &zipCryptoHandler{}
)

// ResolveCipher returns the appropriate CipherHandler for an encryption method.
func ResolveCipher(enc file.EncryptionMethod) CipherHandler {
	switch enc {
	case file.AES128, file.AES192, file.AES256:
		return aesCipherHandler
	case file.ZipCrypto:
		return zipCryptoCipherHandler
	default:
		return nil
	}
}

// DetectCipher inspects header flags, method ID, and raw extra fields to detect cipher and underlying compression.
func DetectCipher(
	flags, wireMethod uint16,
	rawExtra []byte,
) (CipherHandler, file.EncryptionMethod, codec.Method, bool) {
	if enc, realMethod, ok := aesCipherHandler.Detect(flags, wireMethod, rawExtra); ok {
		return aesCipherHandler, enc, realMethod, true
	}
	if enc, realMethod, ok := zipCryptoCipherHandler.Detect(flags, wireMethod, rawExtra); ok {
		return zipCryptoCipherHandler, enc, realMethod, true
	}
	return nil, file.NotEncrypted, codec.Method(wireMethod), false
}

// Sentinel encryption errors alias.
var (
	ErrPasswordMismatch   = crypto.ErrPasswordMismatch
	ErrAuthenticationFail = crypto.ErrAuthenticationFail
	ErrPayloadTooShort    = crypto.ErrPayloadTooShort
)

// WinZip AES constants.
const (
	AESStrength128 byte = 0x01
	AESStrength192 byte = 0x02
	AESStrength256 byte = 0x03
)

func getAESParams(strength byte) (keySize, saltSize int) {
	switch strength {
	case AESStrength128:
		return 16, 8
	case AESStrength192:
		return 24, 12
	default:
		return 32, 16
	}
}

// NewAESWriter constructs an [io.WriteCloser] encrypting data using WinZip AES standard for the given strength.
func NewAESWriter(dest io.Writer, password string, strength byte) (io.WriteCloser, error) {
	keySize, saltSize := getAESParams(strength)
	return crypto.NewAESCTRWriter(dest, password, keySize, saltSize)
}

// NewAES256Writer constructs an [io.WriteCloser] encrypting data using WinZip AES-256 standard.
func NewAES256Writer(dest io.Writer, password string) (io.WriteCloser, error) {
	return NewAESWriter(dest, password, AESStrength256)
}

// NewAES128Writer constructs an [io.WriteCloser] encrypting data using WinZip AES-128 standard.
func NewAES128Writer(dest io.Writer, password string) (io.WriteCloser, error) {
	return NewAESWriter(dest, password, AESStrength128)
}

// NewAESReader constructs an [io.Reader] decrypting WinZip AES payloads for the given strength.
func NewAESReader(src io.Reader, password string, compressedSize int64, strength byte) (io.Reader, error) {
	keySize, saltSize := getAESParams(strength)
	return crypto.NewAESCTRReader(src, password, compressedSize, keySize, saltSize)
}

// NewAES256Reader constructs an [io.Reader] decrypting WinZip AES-256 standard payloads.
func NewAES256Reader(src io.Reader, password string, compressedSize int64) (io.Reader, error) {
	return NewAESReader(src, password, compressedSize, AESStrength256)
}

// NewAES128Reader constructs an [io.Reader] decrypting WinZip AES-128 standard payloads.
func NewAES128Reader(src io.Reader, password string, compressedSize int64) (io.Reader, error) {
	return NewAESReader(src, password, compressedSize, AESStrength128)
}

// NewZipCryptoWriter creates an [io.WriteCloser] encrypting via legacy ZipCrypto.
func NewZipCryptoWriter(dest io.Writer, password string, checkByte byte) (io.WriteCloser, error) {
	return crypto.NewZipCryptoWriter(dest, password, checkByte)
}

// NewZipCryptoReader creates an [io.Reader] decrypting legacy ZipCrypto streams.
func NewZipCryptoReader(
	src io.Reader,
	password string,
	flags uint16,
	crc32Val uint32,
	modTime uint16,
) (io.Reader, error) {
	return crypto.NewZipCryptoReader(src, password, flags, crc32Val, modTime)
}
