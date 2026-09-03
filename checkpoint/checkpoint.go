// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package checkpoint implements transient state serialization for resumable archives in Seal.
package checkpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// Magic identifies a valid Seal checkpoint header.
	Magic = "SEAL_CHECKPOINT_V1"

	// CurrentVersion defines the schema revision of State.
	CurrentVersion = 1

	// ZipExtraTag defines the 16-bit PKWARE Extra Field tag for Seal checkpoints (0x534C = 'S','L').
	ZipExtraTag uint16 = 0x534C

	// SevenZipPropID defines the 7-Zip ArchiveProperties property byte for Seal checkpoints (0x53 = 'S').
	SevenZipPropID byte = 0x53
)

var (
	// ErrInvalidMagic indicates that the data does not contain a recognized Seal checkpoint header.
	ErrInvalidMagic = errors.New("checkpoint: invalid or missing magic signature")

	// ErrUnsupportedVersion indicates an incompatible checkpoint schema version.
	ErrUnsupportedVersion = errors.New("checkpoint: unsupported schema version")
)

// State encapsulates the complete checkpoint metadata allowing Seal to resume interrupted operations.
type State struct {
	Magic          string    `json:"magic"`
	Version        uint32    `json:"version"`
	SessionID      string    `json:"session_id"`
	CreatedAt      time.Time `json:"created_at"`
	FrozenAt       time.Time `json:"frozen_at"`
	SourceDir      string    `json:"source_dir"`
	Format         string    `json:"format"`
	TotalFiles     int       `json:"total_files"`
	DoneFiles      int       `json:"done_files"`
	RemainingFiles []string  `json:"remaining_files"`
	SourceHash     string    `json:"source_hash,omitempty"`
}

// NewState constructs a new checkpoint State with initialized timestamps and schema version.
func NewState(sessionID, sourceDir, format string, totalFiles, doneFiles int, remaining []string) *State {
	now := time.Now()
	return &State{
		Magic:          Magic,
		Version:        CurrentVersion,
		SessionID:      sessionID,
		CreatedAt:      now,
		FrozenAt:       now,
		SourceDir:      sourceDir,
		Format:         format,
		TotalFiles:     totalFiles,
		DoneFiles:      doneFiles,
		RemainingFiles: remaining,
	}
}

// Encode serializes the State to a compact JSON byte slice.
func (s *State) Encode() ([]byte, error) {
	if s.Magic == "" {
		s.Magic = Magic
	}
	if s.Version == 0 {
		s.Version = CurrentVersion
	}
	return json.Marshal(s)
}

// Decode deserializes a State from a byte slice and validates its integrity.
func Decode(data []byte) (*State, error) {
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("checkpoint: unmarshal json: %w", err)
	}

	if s.Magic != Magic {
		return nil, ErrInvalidMagic
	}

	if s.Version > CurrentVersion {
		return nil, fmt.Errorf("%w: got %d, max supported %d", ErrUnsupportedVersion, s.Version, CurrentVersion)
	}

	return &s, nil
}

// StripCheckpoint removes any embedded checkpoint signature and payload from a comment string.
func StripCheckpoint(comment string) string {
	idx := strings.Index(comment, Magic)
	if idx == -1 {
		return comment
	}
	if idx == 0 {
		_, after, ok := strings.Cut(comment, "\n")
		if !ok {
			return ""
		}
		return strings.TrimSpace(after)
	}
	return strings.TrimSpace(comment[:idx])
}
