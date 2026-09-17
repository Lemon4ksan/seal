// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ppmd

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"
)

func TestPPMd_Roundtrip(t *testing.T) {
	original := []byte("Prediction by Partial Matching PPMd compression engine test in Seal archiver substrate.")

	comp := NewCompressor(DefaultOrder, DefaultMemSize)
	var compressed bytes.Buffer
	n, err := comp.Compress(bytes.NewReader(original), &compressed)
	require.NoError(t, err)
	assert.Equal(t, int64(len(original)), n)

	decomp := NewDecompressor(DefaultOrder, DefaultMemSize)
	rc, err := decomp.Decompress(&compressed, uint64(len(original)))
	require.NoError(t, err)
	defer rc.Close()

	decoded, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestPPMd_Properties(t *testing.T) {
	props := Properties{Order: 8, MemSize: 32 * 1024 * 1024, RestorationMethod: 0}
	encoded := props.EncodeProperties()
	parsed, err := ParseProperties(encoded)
	require.NoError(t, err)
	assert.Equal(t, props.Order, parsed.Order)
	assert.Equal(t, props.MemSize, parsed.MemSize)
}

func BenchmarkPPMd_Compress(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB sample
	_, _ = rand.Read(data)
	comp := NewCompressor(DefaultOrder, DefaultMemSize)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_, _ = comp.Compress(bytes.NewReader(data), &buf)
	}
}

func BenchmarkPPMd_Decompress(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB sample
	_, _ = rand.Read(data)
	comp := NewCompressor(DefaultOrder, DefaultMemSize)
	var compBuf bytes.Buffer
	_, _ = comp.Compress(bytes.NewReader(data), &compBuf)
	compBytes := compBuf.Bytes()

	decomp := NewDecompressor(DefaultOrder, DefaultMemSize)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rc, _ := decomp.Decompress(bytes.NewReader(compBytes), uint64(len(data)))
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
}
