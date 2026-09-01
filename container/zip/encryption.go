// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"

	"github.com/lemon4ksan/seal/file"
)

// EncryptionMethod identifies the payload encryption algorithm.
type EncryptionMethod = file.EncryptionMethod

// Supported encryption algorithms according to ZIP and WinZip specifications.
const (
	NotEncrypted = file.NotEncrypted
	ZipCrypto    = file.ZipCrypto
	AES256       = file.AES256
)

// Sentinel encryption errors.
var (
	ErrPasswordMismatch   = errors.New("zip: invalid or missing password")
	ErrAuthenticationFail = errors.New("zip: AES HMAC authentication verification failed")
	ErrPayloadTooShort    = errors.New("zip: encrypted payload is too small")
)

// WinZip AES constants.
const (
	aes256KeySize  = 32 // 256 bits
	aes256SaltSize = 16 // 128 bits
	aesMacSize     = 10 // HMAC-SHA1 truncated to 10 bytes
	aesPvvSize     = 2  // Password verification value length
	aesOverhead    = aes256SaltSize + aesPvvSize + aesMacSize
)

// NewAES256Writer constructs an [io.WriteCloser] encrypting data using WinZip AES-256 standard.
func NewAES256Writer(dest io.Writer, password string) (io.WriteCloser, error) {
	var salt [aes256SaltSize]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return nil, fmt.Errorf("generate AES salt: %w", err)
	}

	keys := deriveAESKeys(password, salt[:])

	if _, err := dest.Write(salt[:]); err != nil {
		return nil, fmt.Errorf("write AES salt: %w", err)
	}
	if _, err := dest.Write(keys.pvv); err != nil {
		return nil, fmt.Errorf("write AES pvv: %w", err)
	}

	block, err := aes.NewCipher(keys.encKey)
	if err != nil {
		return nil, err
	}

	return &aesWriter{
		dest:   dest,
		stream: newWinZipCTR(block),
		mac:    hmac.New(sha1.New, keys.macKey),
	}, nil
}

type aesWriter struct {
	dest    io.Writer
	stream  *winZipCTR
	mac     hash.Hash
	scratch []byte
}

func (w *aesWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if cap(w.scratch) < len(p) {
		w.scratch = make([]byte, len(p))
	}
	buf := w.scratch[:len(p)]

	w.stream.XORKeyStream(buf, p)
	w.mac.Write(buf) // Encrypt-then-MAC on ciphertext

	return w.dest.Write(buf)
}

func (w *aesWriter) Close() error {
	sum := w.mac.Sum(nil)
	if _, err := w.dest.Write(sum[:aesMacSize]); err != nil {
		return fmt.Errorf("write AES authentication code: %w", err)
	}
	if c, ok := w.dest.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// NewAES256Reader constructs an [io.Reader] decrypting WinZip AES-256 payloads.
func NewAES256Reader(src io.Reader, password string, compressedSize int64) (io.Reader, error) {
	if compressedSize < aesOverhead {
		return nil, ErrPayloadTooShort
	}

	var salt [aes256SaltSize]byte
	if _, err := io.ReadFull(src, salt[:]); err != nil {
		return nil, fmt.Errorf("read AES salt: %w", err)
	}

	keys := deriveAESKeys(password, salt[:])

	pvv := make([]byte, aesPvvSize)
	if _, err := io.ReadFull(src, pvv); err != nil {
		return nil, fmt.Errorf("read AES pvv: %w", err)
	}
	if !bytes.Equal(pvv, keys.pvv) {
		return nil, ErrPasswordMismatch
	}

	block, err := aes.NewCipher(keys.encKey)
	if err != nil {
		return nil, err
	}

	dataSize := compressedSize - aesOverhead
	return &aesReader{
		payloadReader: io.LimitReader(src, dataSize),
		macSource:     src,
		stream:        newWinZipCTR(block),
		mac:           hmac.New(sha1.New, keys.macKey),
	}, nil
}

type aesReader struct {
	payloadReader io.Reader
	macSource     io.Reader
	stream        *winZipCTR
	mac           hash.Hash
}

func (r *aesReader) Read(p []byte) (int, error) {
	n, err := r.payloadReader.Read(p)
	if n > 0 {
		r.mac.Write(p[:n])
		r.stream.XORKeyStream(p[:n], p[:n])
	}

	if err == io.EOF {
		var expected [aesMacSize]byte
		if _, macErr := io.ReadFull(r.macSource, expected[:]); macErr != nil {
			return n, fmt.Errorf("read auth tag: %w", macErr)
		}
		calc := r.mac.Sum(nil)[:aesMacSize]
		if subtle.ConstantTimeCompare(calc, expected[:]) != 1 {
			return n, ErrAuthenticationFail
		}
	}

	return n, err
}

type aesKeys struct {
	encKey []byte
	macKey []byte
	pvv    []byte
}

func deriveAESKeys(password string, salt []byte) aesKeys {
	const keyLen = 2*aes256KeySize + aesPvvSize
	dk := pbkdf2([]byte(password), salt, 1000, keyLen, sha1.New)
	return aesKeys{
		encKey: dk[:aes256KeySize],
		macKey: dk[aes256KeySize : 2*aes256KeySize],
		pvv:    dk[2*aes256KeySize : 2*aes256KeySize+aesPvvSize],
	}
}

// winZipCTR implements cipher.Stream with Little Endian 128-bit counter increment (WinZip specification).
type winZipCTR struct {
	block   cipher.Block
	counter [aes.BlockSize]byte
	buffer  [aes.BlockSize]byte
	pos     int
}

func newWinZipCTR(block cipher.Block) *winZipCTR {
	c := &winZipCTR{block: block}
	c.counter[0] = 1
	return c
}

func (c *winZipCTR) XORKeyStream(dst, src []byte) {
	for len(src) > 0 {
		if c.pos == len(c.buffer) || c.pos == 0 {
			c.block.Encrypt(c.buffer[:], c.counter[:])
			c.incrementCounter()
			c.pos = 0
		}
		n := len(src)
		if remain := len(c.buffer) - c.pos; n > remain {
			n = remain
		}
		subtle.XORBytes(dst[:n], src[:n], c.buffer[c.pos:])
		dst = dst[n:]
		src = src[n:]
		c.pos += n
	}
}

func (c *winZipCTR) incrementCounter() {
	for j := range aes.BlockSize {
		c.counter[j]++
		if c.counter[j] != 0 {
			break
		}
	}
}

// pbkdf2 standard derivation function (RFC 2898).
func pbkdf2(password, salt []byte, iter, keyLen int, h func() hash.Hash) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)
	u := make([]byte, hashLen)

	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)
		prf.Write(buf[:4])
		dk = prf.Sum(dk)

		t := dk[len(dk)-hashLen:]
		copy(u, t)

		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = u[:0]
			u = prf.Sum(u)
			for x := range u {
				t[x] ^= u[x]
			}
		}
	}
	return dk[:keyLen]
}

// Legacy ZipCrypto Implementation.

const cipherMagic = 134775813

type zipCipher struct {
	k0, k1, k2 uint32
}

func newZipCipher(password string) *zipCipher {
	z := &zipCipher{
		k0: 0x12345678,
		k1: 0x23456789,
		k2: 0x34567890,
	}
	for i := 0; i < len(password); i++ {
		z.updateKeys(password[i])
	}
	return z
}

func (z *zipCipher) updateKeys(b byte) {
	z.k0 = crc32.IEEETable[(z.k0^uint32(b))&0xff] ^ (z.k0 >> 8)
	z.k1 += (z.k0 & 0xff)
	z.k1 = z.k1*cipherMagic + 1
	z.k2 = crc32.IEEETable[(z.k2^uint32(byte(z.k1>>24)))&0xff] ^ (z.k2 >> 8)
}

func (z *zipCipher) magicByte() byte {
	t := z.k2 | 2
	return byte((t * (t ^ 1)) >> 8)
}

func (z *zipCipher) Encrypt(buf []byte) {
	for i, b := range buf {
		c := b ^ z.magicByte()
		z.updateKeys(b)
		buf[i] = c
	}
}

func (z *zipCipher) Decrypt(buf []byte) {
	for i, c := range buf {
		k := z.magicByte()
		b := c ^ k
		z.updateKeys(b)
		buf[i] = b
	}
}

// NewZipCryptoWriter creates an [io.WriteCloser] encrypting via legacy ZipCrypto.
func NewZipCryptoWriter(dest io.Writer, password string, checkByte byte) (io.WriteCloser, error) {
	cipher := newZipCipher(password)
	var header [12]byte
	if _, err := rand.Read(header[:]); err != nil {
		return nil, fmt.Errorf("read crypto random bytes: %w", err)
	}

	header[11] = checkByte
	cipher.Encrypt(header[:])

	if _, err := dest.Write(header[:]); err != nil {
		return nil, fmt.Errorf("write crypto header: %w", err)
	}

	return &zipCryptoWriter{dest: dest, cipher: cipher}, nil
}

type zipCryptoWriter struct {
	dest   io.Writer
	cipher *zipCipher
}

func (w *zipCryptoWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	buf := make([]byte, len(p))
	copy(buf, p)
	w.cipher.Encrypt(buf)
	return w.dest.Write(buf)
}

func (w *zipCryptoWriter) Close() error {
	if c, ok := w.dest.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// NewZipCryptoReader creates an [io.Reader] decrypting legacy ZipCrypto streams.
func NewZipCryptoReader(
	src io.Reader,
	password string,
	flags uint16,
	crc32Val uint32,
	modTime uint16,
) (io.Reader, error) {
	cipher := newZipCipher(password)
	var header [12]byte
	if _, err := io.ReadFull(src, header[:]); err != nil {
		return nil, fmt.Errorf("read crypto header: %w", err)
	}
	cipher.Decrypt(header[:])

	var expected byte
	if flags&0x8 != 0 {
		expected = byte(modTime >> 8)
	} else {
		expected = byte(crc32Val >> 24)
	}

	if header[11] != expected {
		return nil, ErrPasswordMismatch
	}

	return &zipCryptoReader{source: src, cipher: cipher}, nil
}

type zipCryptoReader struct {
	source io.Reader
	cipher *zipCipher
}

func (r *zipCryptoReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if n > 0 {
		r.cipher.Decrypt(p[:n])
	}
	return n, err
}
