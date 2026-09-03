// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ar implements the Unix AR archive format (.ar, .a, .deb), supporting GNU and BSD variants.
package ar

import (
	"errors"
	"time"
)

// AR format constants.
const (
	// Magic defines the 8-byte global archive signature ("!<arch>\n").
	Magic = "!<arch>\n"

	// HeaderSize is the fixed 60-byte size of each member header.
	HeaderSize = 60

	// HeaderTrailer is the 2-byte end-of-header signature ("`\n").
	HeaderTrailer = "`\n"
)

// Common AR errors.
var (
	ErrInvalidMagic    = errors.New("ar: invalid archive magic signature")
	ErrCorruptedHeader = errors.New("ar: corrupted or malformed member header")
)

// Header represents a parsed AR member header.
type Header struct {
	Name    string
	ModTime time.Time
	UID     int
	GID     int
	Mode    int64
	Size    int64
}
