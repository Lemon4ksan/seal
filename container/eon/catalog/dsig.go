// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package catalog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/container/eon/spec"
)

// ErrInvalidSignatureMagic indicates that the DSIG signature record header magic did not match 'DSIG'.
var ErrInvalidSignatureMagic = errors.New("catalog: invalid signature record magic")

// DSIG represents a cryptographic digital signature over the Central Directory (CTOC and INDX).
type DSIG struct {
	AlgorithmID uint16
	KeyID       string
	PublicKey   []byte
	Signature   []byte
}

// Write serializes the DSIG record into w.
func (d *DSIG) Write(w io.Writer) (int64, error) {
	keyIDBytes := []byte(d.KeyID)
	var hdr [spec.DSIGHeaderSize]byte

	copy(hdr[0:4], spec.MagicDSIG[:])
	binary.LittleEndian.PutUint16(hdr[4:6], d.AlgorithmID)
	binary.LittleEndian.PutUint16(hdr[6:8], 0) // Reserved
	binary.LittleEndian.PutUint16(hdr[8:10], uint16(len(keyIDBytes)))
	binary.LittleEndian.PutUint16(hdr[10:12], uint16(len(d.PublicKey)))
	binary.LittleEndian.PutUint16(hdr[12:14], uint16(len(d.Signature)))

	if _, err := w.Write(hdr[:]); err != nil {
		return 0, err
	}
	total := int64(spec.DSIGHeaderSize)

	if len(keyIDBytes) > 0 {
		if _, err := w.Write(keyIDBytes); err != nil {
			return total, err
		}
		total += int64(len(keyIDBytes))
	}

	if len(d.PublicKey) > 0 {
		if _, err := w.Write(d.PublicKey); err != nil {
			return total, err
		}
		total += int64(len(d.PublicKey))
	}

	if len(d.Signature) > 0 {
		if _, err := w.Write(d.Signature); err != nil {
			return total, err
		}
		total += int64(len(d.Signature))
	}

	return total, nil
}

// ReadDSIG parses a DSIG record from r.
func ReadDSIG(r io.Reader) (*DSIG, error) {
	var hdr [spec.DSIGHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, fmt.Errorf("read dsig header: %w", err)
	}

	if binary.LittleEndian.Uint32(hdr[0:4]) != spec.MagicDSIGLE {
		return nil, ErrInvalidSignatureMagic
	}

	algoID := binary.LittleEndian.Uint16(hdr[4:6])
	keyIDLen := int(binary.LittleEndian.Uint16(hdr[8:10]))
	pubKeyLen := int(binary.LittleEndian.Uint16(hdr[10:12]))
	sigLen := int(binary.LittleEndian.Uint16(hdr[12:14]))

	var keyID string
	if keyIDLen > 0 {
		buf := make([]byte, keyIDLen)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("read dsig keyID: %w", err)
		}
		keyID = string(buf)
	}

	pubKey := make([]byte, pubKeyLen)
	if pubKeyLen > 0 {
		if _, err := io.ReadFull(r, pubKey); err != nil {
			return nil, fmt.Errorf("read dsig publicKey: %w", err)
		}
	}

	sig := make([]byte, sigLen)
	if sigLen > 0 {
		if _, err := io.ReadFull(r, sig); err != nil {
			return nil, fmt.Errorf("read dsig signature: %w", err)
		}
	}

	return &DSIG{
		AlgorithmID: algoID,
		KeyID:       keyID,
		PublicKey:   pubKey,
		Signature:   sig,
	}, nil
}
