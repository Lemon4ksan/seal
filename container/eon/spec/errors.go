// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package spec

import "errors"

// Sentinel errors across the EON specification domain.
var (
	ErrInvalidHeader  = errors.New("eon: invalid archive header signature")
	ErrInvalidTrailer = errors.New("eon: invalid archive trailer signature")
	ErrInvalidChunk   = errors.New("eon: invalid chunk header signature")
	ErrInvalidCTOC    = errors.New("eon: invalid chunk table of contents signature")
	ErrInvalidINDX    = errors.New("eon: invalid extent catalog signature")
	ErrChecksumFailed   = errors.New("eon: checksum or hash mismatch")
	ErrTruncatedData    = errors.New("eon: unexpected EOF or truncated structure")
	ErrInvalidSignature = errors.New("eon: invalid or forged cryptographic digital signature")
	ErrSignatureMissing = errors.New("eon: container is not digitally signed")
	ErrPasswordRequired = errors.New("eon: container is encrypted; password required")
	ErrInvalidPassword  = errors.New("eon: invalid password or decryption failed across all key slots")
	ErrUnsupportedCipher = errors.New("eon: unsupported encryption cipher")
	ErrInvalidENCR      = errors.New("eon: invalid encryption header signature")
)
