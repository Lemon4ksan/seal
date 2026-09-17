// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"bytes"
	"testing"

	"github.com/lemon4ksan/foundation/codec/compress/brotli"
	"github.com/lemon4ksan/foundation/codec/compress/gzip"
	"github.com/lemon4ksan/foundation/codec/compress/lzma"
	"github.com/lemon4ksan/foundation/codec/filter/bcj"
	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/seal/codec/ppmd"
)

// generateTextCorpus generates a repetitive source code / text sample.
func generateTextCorpus(size int) []byte {
	snippet := []byte(`package main

import (
	"fmt"
	"sync"
	"time"
)

// WorkerProcess represents an asynchronous concurrent task worker in Seal engine.
type WorkerProcess struct {
	ID        int
	Payload   []byte
	Timestamp time.Time
	mu        sync.Mutex
}

func (w *WorkerProcess) Execute(task string) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fmt.Printf("Worker %d processing task: %s\n", w.ID, task)
	return len(task) * 42, nil
}
`)
	var buf bytes.Buffer
	for buf.Len() < size {
		buf.Write(snippet)
	}
	return buf.Bytes()[:size]
}

// generateBinaryCorpus generates a simulated x86/x64 executable binary code sample.
func generateBinaryCorpus(size int) []byte {
	var buf bytes.Buffer
	for buf.Len() < size {
		buf.Write([]byte{
			0x55, 0x48, 0x89, 0xE5, 0x48, 0x83, 0xEC, 0x20,
			0xE8, 0x40, 0x12, 0x00, 0x00, 0x48, 0x85, 0xC0,
			0x74, 0x0C, 0x48, 0x8B, 0x05, 0x23, 0x45, 0x00,
			0x00, 0xE9, 0x10, 0x34, 0x00, 0x00, 0x90, 0xC3,
		})
	}
	return buf.Bytes()[:size]
}

func TestBenchmarkSuite_Sanity(t *testing.T) {
	data := generateTextCorpus(64 * 1024)
	assert.Len(t, data, 64*1024)
}

func BenchmarkCodecs_Text_1MB(b *testing.B) {
	data := generateTextCorpus(1024 * 1024)

	b.Run("LZMA2_Level1_Fastest", func(b *testing.B) {
		opts := lzma.OptionsForLevel(lzma.LevelFastest)
		comp := lzma.NewCompressor2WithOptions(opts)
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			_, _ = comp.Compress(bytes.NewReader(data), &out)
		}
	})

	b.Run("LZMA2_Level5_Normal", func(b *testing.B) {
		opts := lzma.OptionsForLevel(lzma.LevelNormal)
		comp := lzma.NewCompressor2WithOptions(opts)
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			_, _ = comp.Compress(bytes.NewReader(data), &out)
		}
	})

	b.Run("LZMA2_Level9_Ultra", func(b *testing.B) {
		opts := lzma.OptionsForLevel(lzma.LevelUltra)
		comp := lzma.NewCompressor2WithOptions(opts)
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			_, _ = comp.Compress(bytes.NewReader(data), &out)
		}
	})

	b.Run("PPMd_Order6", func(b *testing.B) {
		comp := ppmd.NewCompressor(6, 16*1024*1024)
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			_, _ = comp.Compress(bytes.NewReader(data), &out)
		}
	})

	b.Run("GZIP_Level1_Fastest", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			w, _ := gzip.NewWriterLevel(&out, gzip.BestSpeed)
			_, _ = w.Write(data)
			_ = w.Close()
		}
	})

	b.Run("GZIP_Level6_Default", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			w, _ := gzip.NewWriterLevel(&out, gzip.DefaultCompression)
			_, _ = w.Write(data)
			_ = w.Close()
		}
	})

	b.Run("Brotli_Quality4", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			w := brotli.NewWriterOptions(&out, brotli.WriterOptions{Quality: 4})
			_, _ = w.Write(data)
			_ = w.Close()
		}
	})
}

func BenchmarkCodecs_Binary_1MB(b *testing.B) {
	data := generateBinaryCorpus(1024 * 1024)

	b.Run("LZMA2_Level1", func(b *testing.B) {
		opts := lzma.OptionsForLevel(lzma.LevelFastest)
		comp := lzma.NewCompressor2WithOptions(opts)
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			_, _ = comp.Compress(bytes.NewReader(data), &out)
		}
	})

	b.Run("LZMA2_BCJ_Level1", func(b *testing.B) {
		opts := lzma.OptionsForLevel(lzma.LevelFastest)
		comp := lzma.NewCompressor2WithOptions(opts)

		filtered := make([]byte, len(data))
		copy(filtered, data)
		bcj.Filter(bcj.X86, filtered, 0, true)

		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			_, _ = comp.Compress(bytes.NewReader(filtered), &out)
		}
	})

	b.Run("LZMA2_Level5", func(b *testing.B) {
		opts := lzma.OptionsForLevel(lzma.LevelNormal)
		comp := lzma.NewCompressor2WithOptions(opts)
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			_, _ = comp.Compress(bytes.NewReader(data), &out)
		}
	})

	b.Run("LZMA2_BCJ_Level5", func(b *testing.B) {
		opts := lzma.OptionsForLevel(lzma.LevelNormal)
		comp := lzma.NewCompressor2WithOptions(opts)

		filtered := make([]byte, len(data))
		copy(filtered, data)
		bcj.Filter(bcj.X86, filtered, 0, true)

		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var out bytes.Buffer
			_, _ = comp.Compress(bytes.NewReader(filtered), &out)
		}
	})
}
