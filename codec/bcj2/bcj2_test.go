// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bcj2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"
)

func TestBCJ2_Roundtrip(t *testing.T) {
	// Generate mock x86 binary code with CALL (E8) and JMP (E9) instructions
	original := make([]byte, 16*1024)
	for i := range original {
		original[i] = byte(i & 0xFF)
	}

	// Inject CALL and JMP with 4-byte relative offsets
	for i := 100; i < len(original)-64; i += 64 {
		original[i] = 0xE8
		original[i+1] = byte(i)
		original[i+2] = byte(i >> 8)
		original[i+3] = 0x00
		original[i+4] = 0x00

		original[i+32] = 0xE9
		original[i+33] = byte(i + 10)
		original[i+34] = byte((i + 10) >> 8)
		original[i+35] = 0x00
		original[i+36] = 0x00
	}

	var mainBuf, callBuf, jumpBuf, rcBuf bytes.Buffer

	// Encode into 4 streams
	err := EncodeStreams(bytes.NewReader(original), &mainBuf, &callBuf, &jumpBuf, &rcBuf)
	require.NoError(t, err)
	assert.Greater(t, mainBuf.Len(), 0, "Main stream is empty")

	// Decode back from 4 streams
	var decoded bytes.Buffer
	err = DecodeStreams(&mainBuf, &callBuf, &jumpBuf, &rcBuf, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original, decoded.Bytes())
}

func TestBCJ2_ComplexOpcodePatterns(t *testing.T) {
	var srcBuf bytes.Buffer
	r := rand.New(rand.NewSource(42))

	// Inject 0xE8 (CALL) pattern
	srcBuf.WriteByte(0xE8)
	binary.Write(&srcBuf, binary.LittleEndian, uint32(100))

	// Inject random noise
	noise := make([]byte, 1000)
	r.Read(noise)
	srcBuf.Write(noise)

	// Inject 0xE9 (JMP) pattern
	srcBuf.WriteByte(0xE9)
	binary.Write(&srcBuf, binary.LittleEndian, uint32(200))

	// Inject 0x0F 0x84 (JE / conditional jump) pattern
	srcBuf.WriteByte(0x0F)
	srcBuf.WriteByte(0x84)
	binary.Write(&srcBuf, binary.LittleEndian, uint32(300))

	r.Read(noise)
	srcBuf.Write(noise)

	originalData := srcBuf.Bytes()

	var mainBuf, callBuf, jumpBuf, rcBuf bytes.Buffer
	err := EncodeStreams(bytes.NewReader(originalData), &mainBuf, &callBuf, &jumpBuf, &rcBuf)
	require.NoError(t, err)

	var decoded bytes.Buffer
	err = DecodeStreams(&mainBuf, &callBuf, &jumpBuf, &rcBuf, &decoded)
	require.NoError(t, err)

	assert.Equal(t, originalData, decoded.Bytes())
}

func TestBCJ2_ChunkedStreaming(t *testing.T) {
	data := make([]byte, 500)
	r := rand.New(rand.NewSource(99))
	r.Read(data)
	data[10] = 0xE8
	data[50] = 0xE9

	var mainBuf, callBuf, jumpBuf, rcBuf bytes.Buffer
	err := EncodeStreams(bytes.NewReader(data), &mainBuf, &callBuf, &jumpBuf, &rcBuf)
	require.NoError(t, err)

	dec := NewStreamDecoder(&mainBuf, &callBuf, &jumpBuf, &rcBuf)
	var decoded bytes.Buffer
	buf := make([]byte, 7) // Small 7-byte chunks to stress buffer alignment
	for {
		n, err := dec.Read(buf)
		if n > 0 {
			decoded.Write(buf[:n])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
	}

	assert.Equal(t, data, decoded.Bytes())
}

func TestBCJ2_EmptyStream(t *testing.T) {
	var mainBuf, callBuf, jumpBuf, rcBuf bytes.Buffer

	err := EncodeStreams(bytes.NewReader(nil), &mainBuf, &callBuf, &jumpBuf, &rcBuf)
	require.NoError(t, err)

	var decoded bytes.Buffer
	err = DecodeStreams(&mainBuf, &callBuf, &jumpBuf, &rcBuf, &decoded)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("DecodeStreams on empty streams failed: %v", err)
	}

	assert.Equal(t, 0, decoded.Len())
}
