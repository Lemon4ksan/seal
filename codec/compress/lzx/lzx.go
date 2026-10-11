// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package lzx implements Microsoft LZX compression and decompression.
package lzx

import (
	"errors"
)

// LZX Constants
const (
	MinMatchLength = 2
	MaxMatchLength = 257

	NumChars       = 256
	NumLengthCodes = 249
	NumAligned     = 8
	NumPreTree     = 20

	BlockVerbatim = 1
	BlockAligned  = 2
	BlockUncomp   = 3

	DefaultWindowBits = 15 // 32 KB window (standard for WIM)
	MinWindowBits     = 15
	MaxWindowBits     = 21
)

var (
	// ErrCorrupt indicates invalid or malformed LZX bitstream.
	ErrCorrupt = errors.New("lzx: corrupted or invalid bitstream")
	// ErrBufferTooSmall indicates destination slice is insufficient.
	ErrBufferTooSmall = errors.New("lzx: destination buffer too small")
)

// Extra bits for position slots 0..49
var extraBits = [50]byte{
	0, 0, 0, 0, 1, 1, 2, 2,
	3, 3, 4, 4, 5, 5, 6, 6,
	7, 7, 8, 8, 9, 9, 10, 10,
	11, 11, 12, 12, 13, 13, 14, 14,
	15, 15, 16, 16, 17, 17, 17, 17,
	17, 17, 17, 17, 17, 17, 17, 17,
	17, 17,
}

// Base position offsets for slots 0..50
var basePosition = [51]uint32{
	0, 1, 2, 3, 4, 6, 8, 12,
	16, 24, 32, 48, 64, 96, 128, 192,
	256, 384, 512, 768, 1024, 1536, 2048, 3072,
	4096, 6144, 8192, 12288, 16384, 24576, 32768, 49152,
	65536, 98304, 131072, 196608, 262144, 393216, 524288, 655360,
	786432, 917504, 1048576, 1179648, 1310720, 1441792, 1572864, 1703936,
	1835008, 1966080, 2097152,
}

// GetNumPositionSlots returns the number of position slots for a given windowBits (15..21).
func GetNumPositionSlots(windowBits int) int {
	if windowBits < MinWindowBits {
		windowBits = MinWindowBits
	}
	if windowBits > MaxWindowBits {
		windowBits = MaxWindowBits
	}
	return 30 + (windowBits-15)*2
}

// GetMainTreeSize returns the number of symbols in the Main Tree for a given windowBits.
func GetMainTreeSize(windowBits int) int {
	return NumChars + GetNumPositionSlots(windowBits)*8
}
