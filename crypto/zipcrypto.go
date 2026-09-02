// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// ZipCryptoState maintains the internal 3-key state of PKWARE ZipCrypto cipher.
type ZipCryptoState struct {
	key0 uint32
	key1 uint32
	key2 uint32
}

// NewZipCryptoState initializes the ZipCrypto cipher state using the password.
func NewZipCryptoState(password string) *ZipCryptoState {
	s := &ZipCryptoState{
		key0: 0x12345678,
		key1: 0x23456789,
		key2: 0x34567890,
	}
	for i := 0; i < len(password); i++ {
		s.UpdateKeys(password[i])
	}
	return s
}

func (s *ZipCryptoState) UpdateKeys(b byte) {
	s.key0 = crc32.Update(s.key0, crc32.IEEETable, []byte{b})
	s.key1 = (s.key1+(s.key0&0xFF))*134775813 + 1
	s.key2 = crc32.Update(s.key2, crc32.IEEETable, []byte{byte(s.key1 >> 24)})
}

func (s *ZipCryptoState) DecryptByte() byte {
	temp := uint16(s.key2 | 2)
	return byte((uint32(temp) * (uint32(temp) ^ 1)) >> 8)
}

func (s *ZipCryptoState) EncryptByte(b byte) byte {
	k := s.DecryptByte()
	s.UpdateKeys(b)
	return b ^ k
}

// ZipCryptoWriter encrypts data using PKWARE traditional ZipCrypto.
type ZipCryptoWriter struct {
	w     io.Writer
	state *ZipCryptoState
}

// NewZipCryptoWriter constructs a ZipCrypto encrypting writer with 12-byte header.
func NewZipCryptoWriter(dest io.Writer, password string, checkByte byte) (io.WriteCloser, error) {
	state := NewZipCryptoState(password)

	var header [12]byte
	if _, err := rand.Read(header[:11]); err != nil {
		return nil, fmt.Errorf("generate zipcrypto header: %w", err)
	}
	header[11] = checkByte

	for i := 0; i < 12; i++ {
		header[i] = state.EncryptByte(header[i])
	}

	if _, err := dest.Write(header[:]); err != nil {
		return nil, fmt.Errorf("write zipcrypto header: %w", err)
	}

	return &ZipCryptoWriter{
		w:     dest,
		state: state,
	}, nil
}

func (zw *ZipCryptoWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	buf := make([]byte, len(p))
	for i, b := range p {
		buf[i] = zw.state.EncryptByte(b)
	}
	return zw.w.Write(buf)
}

func (zw *ZipCryptoWriter) Close() error {
	if c, ok := zw.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// ZipCryptoReader decrypts PKWARE traditional ZipCrypto stream.
type ZipCryptoReader struct {
	r     io.Reader
	state *ZipCryptoState
}

// NewZipCryptoReader constructs a ZipCrypto decrypting reader.
func NewZipCryptoReader(src io.Reader, password string, flags uint16, crc uint32, dosTime uint16) (io.Reader, error) {
	state := NewZipCryptoState(password)

	var header [12]byte
	if _, err := io.ReadFull(src, header[:]); err != nil {
		return nil, fmt.Errorf("read zipcrypto header: %w", err)
	}

	for i := 0; i < 12; i++ {
		b := header[i] ^ state.DecryptByte()
		state.UpdateKeys(b)
		header[i] = b
	}

	// Password verification
	var expectedCheck byte
	if flags&0x0008 != 0 {
		expectedCheck = byte(dosTime >> 8)
	} else {
		expectedCheck = byte(crc >> 24)
	}

	if header[11] != expectedCheck {
		return nil, errors.New("crypto: invalid ZipCrypto password")
	}

	return &ZipCryptoReader{
		r:     src,
		state: state,
	}, nil
}

func (zr *ZipCryptoReader) Read(p []byte) (int, error) {
	n, err := zr.r.Read(p)
	for i := 0; i < n; i++ {
		b := p[i] ^ zr.state.DecryptByte()
		zr.state.UpdateKeys(b)
		p[i] = b
	}
	return n, err
}
