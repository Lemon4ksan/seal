// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cdc_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/lemon4ksan/seal/container/eon/cdc"
)

func TestFastCDC_BasicRoundtrip(t *testing.T) {
	data := make([]byte, 2*1024*1024)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}

	c, err := cdc.NewFastCDC(cdc.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	var reconstructed []byte
	chunkCount := 0

	err = c.Split(data, func(chunk []byte) error {
		chunkCount++
		if len(chunk) < cdc.DefaultMinSize && len(reconstructed)+len(chunk) < len(data) {
			t.Fatalf("chunk %d size %d below MinSize %d", chunkCount, len(chunk), cdc.DefaultMinSize)
		}
		if len(chunk) > cdc.DefaultMaxSize {
			t.Fatalf("chunk %d size %d above MaxSize %d", chunkCount, len(chunk), cdc.DefaultMaxSize)
		}
		reconstructed = append(reconstructed, chunk...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data, reconstructed) {
		t.Fatal("reconstructed data does not match original")
	}

	if chunkCount == 0 {
		t.Fatal("expected at least 1 chunk")
	}
}

func TestFastCDC_ShiftResistance(t *testing.T) {
	// Base data: 4 MiB
	base := make([]byte, 4*1024*1024)
	if _, err := rand.Read(base); err != nil {
		t.Fatal(err)
	}

	// Shifted data: 123 bytes inserted at offset 0
	shifted := make([]byte, 123+len(base))
	copy(shifted[0:123], []byte("SHIFTED_PREFIX_DATA_INSERTED_AT_HEAD_TO_TEST_BOUNDARY_STABILITY_AND_CONTENT_DEFINED_CHUNKING_SHIFT_RESISTANCE!"))
	copy(shifted[123:], base)

	c, err := cdc.NewFastCDC(cdc.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}

	baseChunks := make(map[string]bool)
	_ = c.Split(base, func(chunk []byte) error {
		baseChunks[string(chunk)] = true
		return nil
	})

	matches := 0
	totalShifted := 0
	_ = c.Split(shifted, func(chunk []byte) error {
		totalShifted++
		if baseChunks[string(chunk)] {
			matches++
		}
		return nil
	})

	// With FastCDC, after the first chunk absorbs the shift, remaining chunks should match!
	ratio := float64(matches) / float64(totalShifted)
	if ratio < 0.70 {
		t.Fatalf("shift resistance too low: %d/%d (%.1f%%) chunks matched", matches, totalShifted, ratio*100)
	}
}

func TestFastCDC_StreamingChunker(t *testing.T) {
	data := make([]byte, 3*1024*1024)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}

	opts := cdc.DefaultOptions()
	cMem, err := cdc.NewFastCDC(opts)
	if err != nil {
		t.Fatal(err)
	}

	var memChunks [][]byte
	_ = cMem.Split(data, func(chunk []byte) error {
		memChunks = append(memChunks, chunk)
		return nil
	})

	chunker, err := cdc.NewChunker(bytes.NewReader(data), opts)
	if err != nil {
		t.Fatal(err)
	}

	var streamChunks [][]byte
	for {
		chunk, err := chunker.Next()
		if err != nil {
			break
		}
		streamChunks = append(streamChunks, chunk)
	}

	if len(memChunks) != len(streamChunks) {
		t.Fatalf("chunk count mismatch: mem=%d stream=%d", len(memChunks), len(streamChunks))
	}

	for i := range memChunks {
		if !bytes.Equal(memChunks[i], streamChunks[i]) {
			t.Fatalf("chunk %d mismatch between in-memory split and streaming chunker", i)
		}
	}
}

func BenchmarkFastCDC(b *testing.B) {
	data := make([]byte, 4*1024*1024)
	_, _ = rand.Read(data)

	c, _ := cdc.NewFastCDC(cdc.DefaultOptions())

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = c.Split(data, func(chunk []byte) error {
			return nil
		})
	}
}
