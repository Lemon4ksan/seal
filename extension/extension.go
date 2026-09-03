// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package extension provides a universal, format-agnostic extension SDK for embedding
// and extracting arbitrary metadata, signatures, telemetry, and recovery records
// stealthily inside standard archive container headers without third-party tool corruption.
package extension

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"github.com/lemon4ksan/seal/container"
)

// ID uniquely namespaces an archive extension (e.g., "seal.checkpoint", "acme.license").
type ID string

const (
	// Magic is the 4-byte marker identifying a multi-extension bundle.
	Magic = "SEXT"
	// Version denotes the current binary envelope specification version.
	Version uint16 = 1

	// LegacyCheckpointPrefix is checked for backward compatibility with pre-bundle checkpoints.
	LegacyCheckpointPrefix = "SEAL_CHECKPOINT_V1"
)

// Common errors.
var (
	ErrCorruptedBundle = errors.New("extension: corrupted extension bundle payload")
	ErrExtensionNotFound = errors.New("extension: requested extension not found in archive")
)

// ArchiveInfo conveys metadata about the current archive operation to extensions.
type ArchiveInfo struct {
	Format     container.Type
	SourceDir  string
	TotalFiles int
	DoneFiles  int
	Comment    string
}

// Extension defines the lifecycle callbacks required to inject and inspect archive extensions.
type Extension interface {
	// ID returns the unique identifier for this extension.
	ID() ID

	// OnWrite is called before finalizing the archive to produce the extension payload bytes.
	// Returning nil, nil skips writing this extension.
	OnWrite(ctx context.Context, info ArchiveInfo) ([]byte, error)

	// OnRead is called when matching extension metadata is found in an archive.
	OnRead(ctx context.Context, data []byte, info ArchiveInfo) error
}

// RawExtension is a simple in-memory implementation of Extension holding static raw bytes.
type RawExtension struct {
	ExtID ID
	Data  []byte
}

// NewRaw constructs an Extension with a static payload.
func NewRaw(id ID, data []byte) *RawExtension {
	return &RawExtension{ExtID: id, Data: data}
}

// ID returns the extension identifier.
func (r *RawExtension) ID() ID { return r.ExtID }

// OnWrite returns the static payload bytes.
func (r *RawExtension) OnWrite(_ context.Context, _ ArchiveInfo) ([]byte, error) {
	return r.Data, nil
}

// OnRead copies the read data into Data.
func (r *RawExtension) OnRead(_ context.Context, data []byte, _ ArchiveInfo) error {
	r.Data = bytes.Clone(data)
	return nil
}

// EncodeBundle serializes a map of extension payloads into the compact binary SEXT envelope.
func EncodeBundle(entries map[ID][]byte) []byte {
	if len(entries) == 0 {
		return nil
	}

	var buf bytes.Buffer
	buf.WriteString(Magic)

	var hdr [4]byte
	binary.LittleEndian.PutUint16(hdr[0:2], Version)
	binary.LittleEndian.PutUint16(hdr[2:4], uint16(len(entries)))
	buf.Write(hdr[:])

	for id, data := range entries {
		idBytes := []byte(id)
		var entryHdr [6]byte
		binary.LittleEndian.PutUint16(entryHdr[0:2], uint16(len(idBytes)))
		binary.LittleEndian.PutUint32(entryHdr[2:6], uint32(len(data)))
		buf.Write(entryHdr[:])
		buf.Write(idBytes)
		buf.Write(data)
	}

	return buf.Bytes()
}

// DecodeBundle parses an extension bundle from raw bytes.
// It supports both modern SEXT binary envelopes and legacy SEAL_CHECKPOINT_V1 payloads.
func DecodeBundle(data []byte) (map[ID][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// 1. If modern SEXT binary envelope is present, decode it
	idx := bytes.Index(data, []byte(Magic))
	if idx != -1 {
		r := bytes.NewReader(data[idx+4:])

		var hdr [4]byte
		if _, err := r.Read(hdr[:]); err != nil {
			return nil, fmt.Errorf("%w: header truncated", ErrCorruptedBundle)
		}

		version := binary.LittleEndian.Uint16(hdr[0:2])
		if version != Version {
			return nil, fmt.Errorf("extension: unsupported bundle version %d", version)
		}
		count := binary.LittleEndian.Uint16(hdr[2:4])

		result := make(map[ID][]byte, count)
		for i := 0; i < int(count); i++ {
			var entryHdr [6]byte
			if _, err := r.Read(entryHdr[:]); err != nil {
				return nil, fmt.Errorf("%w: entry %d truncated", ErrCorruptedBundle, i)
			}
			idLen := binary.LittleEndian.Uint16(entryHdr[0:2])
			dataLen := binary.LittleEndian.Uint32(entryHdr[2:6])

			idBuf := make([]byte, idLen)
			if _, err := r.Read(idBuf); err != nil {
				return nil, fmt.Errorf("%w: entry ID truncated", ErrCorruptedBundle)
			}

			payload := make([]byte, dataLen)
			if _, err := r.Read(payload); err != nil {
				return nil, fmt.Errorf("%w: entry payload truncated", ErrCorruptedBundle)
			}

			result[ID(idBuf)] = payload
		}

		return result, nil
	}

	// 2. Backward compatibility fallback for legacy checkpoints
	if strings.Contains(string(data), LegacyCheckpointPrefix) {
		trimmed := bytes.TrimSpace(data)
		if bytes.HasPrefix(trimmed, []byte("{")) {
			return map[ID][]byte{
				"seal.checkpoint": trimmed,
			}, nil
		}
		return map[ID][]byte{
			"seal.checkpoint": data,
		}, nil
	}

	return nil, ErrCorruptedBundle
}

// StripBundle removes any embedded SEXT bundle marker from text comments (such as ZIP comments).
func StripBundle(comment string) string {
	idx := strings.Index(comment, Magic)
	if idx == -1 {
		return comment
	}
	return strings.TrimSpace(comment[:idx])
}
