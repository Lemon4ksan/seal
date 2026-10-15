// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package signature implements cryptographic archive signing and tamper verification
// using standard Ed25519 digital signatures embedded into stealth archive extensions.
package signature

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lemon4ksan/seal/extension"
)

const (
	// ExtensionID is the unique namespace for embedded archive signatures.
	ExtensionID extension.ID = "seal.signature"

	// CurrentVersion defines the signature payload version.
	CurrentVersion uint16 = 1
)

// Common signature errors.
var (
	ErrInvalidKey         = errors.New("signature: invalid or missing Ed25519 key")
	ErrSignatureFailed    = errors.New("signature: digital signature verification failed")
	ErrHashMismatch       = errors.New("signature: archive content hash mismatch (tampered archive)")
	ErrUnsupportedVersion = errors.New("signature: unsupported signature version")
)

// State holds the complete cryptographic proof and signer metadata.
type State struct {
	Version     uint16    `json:"version"`
	Signer      string    `json:"signer"`
	SignedAt    time.Time `json:"signed_at"`
	PublicKey   []byte    `json:"public_key"`
	Signature   []byte    `json:"signature"`
	ContentHash []byte    `json:"content_hash,omitempty"`
}

// Signer implements [extension.Extension] to attach an Ed25519 digital signature to an archive.
type Signer struct {
	privKey     ed25519.PrivateKey
	signerInfo  string
	contentHash []byte
	state       *State
}

// NewSigner constructs an archive signer using an Ed25519 private key.
func NewSigner(privKey ed25519.PrivateKey, signerInfo string) (*Signer, error) {
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, ErrInvalidKey
	}
	return &Signer{
		privKey:    privKey,
		signerInfo: signerInfo,
	}, nil
}

// WithContentHash attaches an optional precomputed content hash (such as SHA-256) to bind to the signature.
func (s *Signer) WithContentHash(hash []byte) *Signer {
	s.contentHash = bytes.Clone(hash)
	return s
}

// ID returns the extension identifier.
func (s *Signer) ID() extension.ID {
	return ExtensionID
}

// OnWrite signs the archive metadata and returns the serialized JSON signature payload.
func (s *Signer) OnWrite(_ context.Context, info extension.ArchiveInfo) ([]byte, error) {
	pubKey, ok := s.privKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, ErrInvalidKey
	}

	// Sign message combining format, total files, and content hash
	msg := formatSignatureMessage(info, s.contentHash)
	sig := ed25519.Sign(s.privKey, msg)

	st := &State{
		Version:     CurrentVersion,
		Signer:      s.signerInfo,
		SignedAt:    time.Now().UTC(),
		PublicKey:   pubKey,
		Signature:   sig,
		ContentHash: s.contentHash,
	}

	s.state = st
	return json.Marshal(st)
}

// OnRead deserializes a signature state from payload bytes.
func (s *Signer) OnRead(_ context.Context, data []byte, _ extension.ArchiveInfo) error {
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("signature: unmarshal: %w", err)
	}
	s.state = &st
	return nil
}

// State returns the parsed signature state after read or write.
func (s *Signer) State() *State {
	return s.state
}

// Verify validates that the signature in State is cryptographically authentic for the given message.
func (s *State) Verify(expectedPubKey ed25519.PublicKey, info extension.ArchiveInfo) error {
	if s.Version > CurrentVersion {
		return fmt.Errorf("%w: got %d, max supported %d", ErrUnsupportedVersion, s.Version, CurrentVersion)
	}

	keyToUse := expectedPubKey
	if len(keyToUse) == 0 {
		keyToUse = s.PublicKey
	} else if len(s.PublicKey) > 0 && !bytes.Equal(s.PublicKey, keyToUse) {
		return errors.New("signature: public key mismatch")
	}

	if len(keyToUse) != ed25519.PublicKeySize {
		return ErrInvalidKey
	}

	msg := formatSignatureMessage(info, s.ContentHash)
	if !ed25519.Verify(keyToUse, msg, s.Signature) {
		return ErrSignatureFailed
	}

	return nil
}

// DecodeState parses a serialized signature State from JSON bytes.
func DecodeState(data []byte) (*State, error) {
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("signature: decode: %w", err)
	}
	return &st, nil
}

func formatSignatureMessage(info extension.ArchiveInfo, contentHash []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("SEAL-ED25519-V1:")
	buf.WriteString(string(info.Format))
	fmt.Fprintf(&buf, ":%d:", info.TotalFiles)
	buf.Write(contentHash)
	return buf.Bytes()
}
