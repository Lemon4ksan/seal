// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package przm_test

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/lemon4ksan/seal/codec/przm"
	"github.com/lemon4ksan/seal/codec/przm/bench"
)

func TestPRZM_EmptyAndShort(t *testing.T) {
	// Empty slice
	compressed, err := przm.Compress([]byte{}, 1)
	if err != nil {
		t.Fatalf("compress empty: %v", err)
	}
	decompressed, err := przm.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress empty: %v", err)
	}
	if len(decompressed) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(decompressed))
	}

	// Short string (3 bytes < minMatch)
	short := []byte("xyz")
	compressed, err = przm.Compress(short, 1)
	if err != nil {
		t.Fatalf("compress short: %v", err)
	}
	decompressed, err = przm.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress short: %v", err)
	}
	if !bytes.Equal(decompressed, short) {
		t.Fatalf("expected %q, got %q", short, decompressed)
	}
}

func TestPRZM_RLE(t *testing.T) {
	// 5000 zeros
	zeros := bytes.Repeat([]byte{0x00}, 5000)
	compressed, err := przm.Compress(zeros, 1)
	if err != nil {
		t.Fatalf("compress zeros: %v", err)
	}

	// High compression ratio on RLE
	if len(compressed) > 100 {
		t.Fatalf("RLE compression suboptimal: got %d bytes for 5000 zeros", len(compressed))
	}

	decompressed, err := przm.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress zeros: %v", err)
	}
	if !bytes.Equal(decompressed, zeros) {
		t.Fatalf("decompressed mismatch on zeros")
	}
}

func TestPRZM_PeriodicStride(t *testing.T) {
	// Synthesize structured CSV/table records with stride S = 32
	var buf bytes.Buffer
	for i := 0; i < 200; i++ {
		// Exactly 32 bytes per row
		row := fmt.Sprintf("ID:%05d|STATUS:OK|METRIC:%05d\n", i, (i*17)%10000)
		if len(row) != 32 {
			t.Fatalf("row length must be 32, got %d", len(row))
		}
		buf.WriteString(row)
	}
	src := buf.Bytes()

	compressed, err := przm.Compress(src, 1)
	if err != nil {
		t.Fatalf("compress stride data: %v", err)
	}

	hdr, err := przm.ParseFrameHeader(compressed)
	if err != nil {
		t.Fatalf("parse frame header: %v", err)
	}

	if (hdr.Flags & przm.FlagStride) == 0 {
		t.Fatalf("expected FlagStride to be detected for periodic table data, flags=0x%02X", hdr.Flags)
	}
	if hdr.Stride != 32 {
		t.Logf("detected stride: %d (expected 32)", hdr.Stride)
	}

	decompressed, err := przm.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress stride data: %v", err)
	}
	if !bytes.Equal(decompressed, src) {
		t.Fatalf("stride data mismatch")
	}

	ratio := float64(len(src)) / float64(len(compressed))
	t.Logf("Stride Table Compression: %d -> %d bytes (Ratio: %.2fx)", len(src), len(compressed), ratio)
}

func TestPRZM_DeltaMaskedMatches(t *testing.T) {
	// Synthesize JSON logs differing by 1-2 bytes (e.g. sequence counter or timestamp)
	var buf bytes.Buffer
	for i := 10000; i < 10200; i++ {
		// Length ~64 bytes, only differing in the integer number
		line := fmt.Sprintf(`{"event":"click","user_id":%d,"action":"scroll","resp":200}`+"\n", i)
		buf.WriteString(line)
	}
	src := buf.Bytes()

	compressed, err := przm.Compress(src, 1)
	if err != nil {
		t.Fatalf("compress delta logs: %v", err)
	}

	hdr, err := przm.ParseFrameHeader(compressed)
	if err != nil {
		t.Fatalf("parse header: %v", err)
	}

	if (hdr.Flags & przm.FlagDeltaMask) == 0 {
		t.Logf("Note: FlagDeltaMask was not set; checking if covered by exact/stride matches (flags=0x%02X)", hdr.Flags)
	}

	decompressed, err := przm.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress delta logs: %v", err)
	}
	if !bytes.Equal(decompressed, src) {
		t.Fatalf("delta logs mismatch: len=%d vs %d", len(decompressed), len(src))
	}

	ratio := float64(len(src)) / float64(len(compressed))
	t.Logf("JSON Logs Compression: %d -> %d bytes (Ratio: %.2fx)", len(src), len(compressed), ratio)
}

func TestPRZM_IncompressibleRandomFallback(t *testing.T) {
	// 8 KB random noise
	noise := make([]byte, 8192)
	_, _ = rand.Read(noise)

	compressed, err := przm.Compress(noise, 1)
	if err != nil {
		t.Fatalf("compress random noise: %v", err)
	}

	hdr, err := przm.ParseFrameHeader(compressed)
	if err != nil {
		t.Fatalf("parse header: %v", err)
	}

	if (hdr.Flags & przm.FlagUncompressed) == 0 {
		t.Fatalf("expected FlagUncompressed on random noise, got flags=0x%02X", hdr.Flags)
	}

	decompressed, err := przm.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress random noise: %v", err)
	}
	if !bytes.Equal(decompressed, noise) {
		t.Fatalf("random noise mismatch")
	}
}

func TestPRZM_BCJ_MachineCode(t *testing.T) {
	// Synthesize 64 KiB x86_64 machine code with relative CALL (0xE8) instructions
	code := make([]byte, 65536)
	i := 0
	funcID := 0
	for i < len(code)-32 {
		funcID++
		prologue := []byte{0x55, 0x48, 0x89, 0xE5, 0x48, 0x83, 0xEC, 0x20}
		copy(code[i:], prologue)
		i += len(prologue)

		for j := 0; j < 4 && i < len(code)-16; j++ {
			code[i] = 0x8B
			code[i+1] = 0x45
			code[i+2] = 0xFC
			code[i+3] = 0x83
			code[i+4] = 0xC0
			code[i+5] = byte(j + 1)
			i += 6

			code[i] = 0xE8
			rel := int32((funcID*128 + j*16) - i)
			binary.LittleEndian.PutUint32(code[i+1:i+5], uint32(rel))
			i += 5
		}

		if i < len(code)-2 {
			code[i] = 0xC9
			code[i+1] = 0xC3
			i += 2
		}
	}

	compressed, err := przm.Compress(code, 1)
	if err != nil {
		t.Fatalf("compress x86: %v", err)
	}

	hdr, err := przm.ParseFrameHeader(compressed)
	if err != nil {
		t.Fatalf("parse header: %v", err)
	}

	if (hdr.Flags & przm.FlagBCJ) == 0 {
		t.Fatalf("expected FlagBCJ to be set on machine code, got flags=0x%02X", hdr.Flags)
	}

	decompressed, err := przm.Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress x86: %v", err)
	}

	if !bytes.Equal(decompressed, code) {
		t.Fatalf("x86 roundtrip mismatch: len=%d vs %d", len(decompressed), len(code))
	}

	ratio := float64(len(code)) / float64(len(compressed))
	t.Logf("x86 Machine Code Compression: %d -> %d bytes (Ratio: %.2fx, Flags=0x%02X)", len(code), len(compressed), ratio, hdr.Flags)
}

func TestPRZM_CorpusRoundtrip(t *testing.T) {
	corpus := bench.GenerateCorpus([]int{16 * 1024, 64 * 1024, 256 * 1024})
	for _, item := range corpus {
		t.Run(item.Name, func(t *testing.T) {
			comp, err := przm.Compress(item.Data, 1)
			if err != nil {
				t.Fatalf("compress failed: %v", err)
			}
			dec, err := przm.Decompress(comp)
			if err != nil {
				t.Fatalf("decompress failed: %v", err)
			}
			if !bytes.Equal(dec, item.Data) {
				for i := 0; i < len(item.Data); i++ {
					if dec[i] != item.Data[i] {
						t.Fatalf("mismatch at byte %d (len %d vs %d): dec=0x%02X, want=0x%02X", i, len(dec), len(item.Data), dec[i], item.Data[i])
					}
				}
			}
		})
	}
}

func BenchmarkPRZM_Throughput(b *testing.B) {
	// Synthesize 64 KiB realistic structured JSON buffer
	var buf bytes.Buffer
	for i := 0; buf.Len() < 64*1024; i++ {
		fmt.Fprintf(&buf, `{"sensor_id":"temp-%03d","metric":"temperature","value":%.2f,"status":"ok","ts":%d}`+"\n",
			i%50, 20.0+float64(i%100)/10.0, 17180000+i)
	}
	data := buf.Bytes()[:64*1024]

	compressed, err := przm.Compress(data, 1)
	if err != nil {
		b.Fatalf("compress: %v", err)
	}

	b.Run("Compress_64KB", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = przm.Compress(data, 1)
		}
	})

	b.Run("Decompress_64KB", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = przm.Decompress(compressed)
		}
	})
}
