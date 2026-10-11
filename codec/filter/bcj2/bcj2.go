// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bcj2 implements the 4-stream Branch-Call-Jump (BCJ2) bytecode filter.
// BCJ2 splits binary executables into four parallel streams:
//  1. Main Stream (StreamMain = 0): original bytecode without branch target offsets
//  2. Call Stream (StreamCall = 1): 32-bit converted absolute addresses for CALL instructions
//  3. Jump Stream (StreamJump = 2): 32-bit converted absolute addresses for JMP instructions
//  4. Range Coder Status Stream (StreamRC = 3): single-bit entropy stream distinguishing literal bytes from branch opcodes
package bcj2

import (
	"errors"
)

// Stream indices for the 4 parallel streams used in BCJ2.
const (
	StreamMain = iota
	StreamCall
	StreamJump
	StreamRC
	NumStreams
)

// Internal Decoder States.
const (
	decStateOrig0 = NumStreams + iota
	decStateOrig1
	decStateOrig2
	decStateOrig3
	decStateOrig
	decStateError
)

// Internal Encoder States.
const (
	encStateOrig     = NumStreams + iota
	encStateFinished = NumStreams
)

// FinishMode determines how the encoder handles the end of the current input buffer.
type FinishMode int

const (
	// EncFinishModeContinue expects more input data.
	EncFinishModeContinue FinishMode = iota
	// EncFinishModeEndBlock finishes the current block (flushes streams but not Range Coder).
	EncFinishModeEndBlock
	// EncFinishModeEndStream finishes the stream completely (flushes Range Coder).
	EncFinishModeEndStream
)

// Probability model constants for the Range Coder.
const (
	kTopValue            = 1 << 24
	numBitModelTotalBits = 11
	bitModelTotal        = 1 << numBitModelTotalBits
	kNumMoveBits         = 5
)

// ErrData indicates corrupted or invalid data encountered during BCJ2 decoding.
var ErrData = errors.New("bcj2: data error")

func is32BitStream(s uint32) bool {
	return s == StreamCall || s == StreamJump
}
