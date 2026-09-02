// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"errors"
	"fmt"
	"hash"
	"io"
)

// Sentinel encryption errors.
var (
	ErrPasswordMismatch   = errors.New("crypto: invalid or missing password")
	ErrAuthenticationFail = errors.New("crypto: HMAC authentication verification failed")
	ErrPayloadTooShort    = errors.New("crypto: encrypted payload is too small")
)

// LittleEndianCTR implements the 16-byte Little-Endian AES-CTR mode used in WinZip AE-1 and AE-2.
type LittleEndianCTR struct {
	b       cipher.Block
	ctr     [16]byte
	out     [16]byte
	outUsed int
}

// NewLittleEndianCTR initializes a Little-Endian CTR stream.
func NewLittleEndianCTR(b cipher.Block) *LittleEndianCTR {
	c := &LittleEndianCTR{b: b, outUsed: 16}
	c.ctr[0] = 1 // Initial 1-based counter per WinZip AE spec
	return c
}

func (c *LittleEndianCTR) XORKeyStream(dst, src []byte) {
	for i := range src {
		if c.outUsed == 16 {
			c.b.Encrypt(c.out[:], c.ctr[:])
			c.outUsed = 0
			// Increment 16-byte Little-Endian counter
			for j := range 16 {
				c.ctr[j]++
				if c.ctr[j] != 0 {
					break
				}
			}
		}
		dst[i] = src[i] ^ c.out[c.outUsed]
		c.outUsed++
	}
}

// AESCTRWriter wraps an [io.Writer] with WinZip AES-CTR encryption and HMAC-SHA1 authentication.
type AESCTRWriter struct {
	w       io.Writer
	stream  cipher.Stream
	mac     hash.Hash
	written int64
}

// NewAESCTRWriter constructs a WinZip AE-2 encrypting writer.
func NewAESCTRWriter(dest io.Writer, password string, keySize, saltSize int) (io.WriteCloser, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate AES salt: %w", err)
	}

	dk := PBKDF2([]byte(password), salt, 1000, 2*keySize+2, sha1.New)
	encKey := dk[:keySize]
	authKey := dk[keySize : 2*keySize]
	pvv := dk[2*keySize : 2*keySize+2]

	if _, err := dest.Write(salt); err != nil {
		return nil, fmt.Errorf("write AES salt: %w", err)
	}
	if _, err := dest.Write(pvv); err != nil {
		return nil, fmt.Errorf("write AES pvv: %w", err)
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}

	return &AESCTRWriter{
		w:      dest,
		stream: NewLittleEndianCTR(block),
		mac:    hmac.New(sha1.New, authKey),
	}, nil
}

func (w *AESCTRWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	buf := make([]byte, len(p))
	w.stream.XORKeyStream(buf, p)
	w.mac.Write(buf)
	n, err := w.w.Write(buf)
	w.written += int64(n)
	return n, err
}

func (w *AESCTRWriter) Close() error {
	fullMac := w.mac.Sum(nil)
	truncMac := fullMac[:10]
	_, err := w.w.Write(truncMac)
	return err
}

// AESCTRReader wraps an [io.Reader] with WinZip AES-CTR decryption and HMAC-SHA1 verification.
type AESCTRReader struct {
	r          io.Reader
	stream     cipher.Stream
	mac        hash.Hash
	payloadRem int64
	storedMAC  [10]byte
}

// NewAESCTRReader constructs a WinZip AE-2 decrypting reader.
func NewAESCTRReader(src io.Reader, password string, compSize int64, keySize, saltSize int) (io.Reader, error) {
	if compSize > 0 && compSize < int64(saltSize+2+10) {
		return nil, ErrPayloadTooShort
	}

	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(src, salt); err != nil {
		return nil, fmt.Errorf("read AES salt: %w", err)
	}

	var pvv [2]byte
	if _, err := io.ReadFull(src, pvv[:]); err != nil {
		return nil, fmt.Errorf("read AES pvv: %w", err)
	}

	dk := PBKDF2([]byte(password), salt, 1000, 2*keySize+2, sha1.New)
	encKey := dk[:keySize]
	authKey := dk[keySize : 2*keySize]
	expectedPvv := dk[2*keySize : 2*keySize+2]

	if subtle.ConstantTimeCompare(pvv[:], expectedPvv) != 1 {
		return nil, ErrPasswordMismatch
	}

	block, err := aes.NewCipher(encKey)
	if err != nil {
		return nil, err
	}

	payloadRem := int64(0)
	if compSize > 0 {
		payloadRem = compSize - int64(saltSize+2+10)
	}

	return &AESCTRReader{
		r:          src,
		stream:     NewLittleEndianCTR(block),
		mac:        hmac.New(sha1.New, authKey),
		payloadRem: payloadRem,
	}, nil
}

func (r *AESCTRReader) Read(p []byte) (int, error) {
	if r.payloadRem == 0 {
		return 0, io.EOF
	}

	toRead := min(int64(len(p)), r.payloadRem)

	n, err := r.r.Read(p[:toRead])
	if n > 0 {
		r.mac.Write(p[:n])
		r.stream.XORKeyStream(p[:n], p[:n])
		r.payloadRem -= int64(n)
	}

	if r.payloadRem == 0 && (err == nil || err == io.EOF) {
		if _, macErr := io.ReadFull(r.r, r.storedMAC[:]); macErr != nil {
			return n, fmt.Errorf("read AES auth tag: %w", macErr)
		}
		computedMAC := r.mac.Sum(nil)[:10]
		if subtle.ConstantTimeCompare(r.storedMAC[:], computedMAC) != 1 {
			return n, ErrAuthenticationFail
		}
		return n, io.EOF
	}

	return n, err
}

// AESCBCWriter wraps an output stream with 7-Zip AES-256-CBC encryption.
type AESCBCWriter struct {
	w        io.Writer
	mode     cipher.BlockMode
	pending  [16]byte
	nPending int
}

// NewAESCBCWriter creates an AES-256-CBC writer.
func NewAESCBCWriter(w io.Writer, key [32]byte, iv [16]byte) (io.WriteCloser, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return &AESCBCWriter{
		w:    w,
		mode: cipher.NewCBCEncrypter(block, iv[:]),
	}, nil
}

func (cw *AESCBCWriter) Write(p []byte) (int, error) {
	written := 0
	if cw.nPending > 0 {
		toFill := 16 - cw.nPending
		if len(p) < toFill {
			copy(cw.pending[cw.nPending:], p)
			cw.nPending += len(p)
			return len(p), nil
		}
		copy(cw.pending[cw.nPending:], p[:toFill])
		cw.mode.CryptBlocks(cw.pending[:], cw.pending[:])
		if _, err := cw.w.Write(cw.pending[:]); err != nil {
			return 0, err
		}
		p = p[toFill:]
		written += toFill
		cw.nPending = 0
	}

	aligned := (len(p) / 16) * 16
	if aligned > 0 {
		buf := make([]byte, aligned)
		cw.mode.CryptBlocks(buf, p[:aligned])
		if _, err := cw.w.Write(buf); err != nil {
			return written, err
		}
		written += aligned
		p = p[aligned:]
	}

	if len(p) > 0 {
		copy(cw.pending[:], p)
		cw.nPending = len(p)
		written += len(p)
	}

	return written, nil
}

func (cw *AESCBCWriter) Close() error {
	if cw.nPending > 0 {
		for i := cw.nPending; i < 16; i++ {
			cw.pending[i] = 0
		}
		cw.mode.CryptBlocks(cw.pending[:], cw.pending[:])
		if _, err := cw.w.Write(cw.pending[:]); err != nil {
			return err
		}
		cw.nPending = 0
	}
	if c, ok := cw.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// AESCBCReader wraps an input stream with 7-Zip AES-256-CBC decryption.
type AESCBCReader struct {
	r      io.Reader
	mode   cipher.BlockMode
	buf    []byte
	bufPos int
}

// NewAESCBCReader creates an AES-256-CBC decrypting reader.
func NewAESCBCReader(r io.Reader, key [32]byte, iv [16]byte) (io.Reader, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return &AESCBCReader{
		r:    r,
		mode: cipher.NewCBCDecrypter(block, iv[:]),
	}, nil
}

func (cr *AESCBCReader) Read(p []byte) (int, error) {
	if len(cr.buf) > cr.bufPos {
		n := copy(p, cr.buf[cr.bufPos:])
		cr.bufPos += n
		return n, nil
	}

	reqBlocks := max((len(p)+15)/16, 32)
	toRead := reqBlocks * 16
	tmp := make([]byte, toRead)

	nRead := 0
	for nRead < 16 {
		n, err := cr.r.Read(tmp[nRead:])
		if n > 0 {
			nRead += n
		}
		if err != nil {
			if nRead == 0 {
				return 0, err
			}
			break
		}
	}

	aligned := (nRead / 16) * 16
	if aligned == 0 {
		return 0, io.EOF
	}

	cr.mode.CryptBlocks(tmp[:aligned], tmp[:aligned])
	cr.buf = tmp[:aligned]
	cr.bufPos = 0

	n := copy(p, cr.buf)
	cr.bufPos += n
	return n, nil
}
