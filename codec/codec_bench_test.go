// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec_test

import (
	"bytes"
	stdflate "compress/flate"
	stdgzip "compress/gzip"
	"io"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/borrow"

	"github.com/lemon4ksan/seal/codec"
)

// Generate sample test payloads (small 1KB, medium 64KB, large 1MB)
var (
	payloadSmall  = []byte(strings.Repeat("Zero allocation compression benchmark in seal engine. ", 16))     // ~1 KB
	payloadMedium = []byte(strings.Repeat("High performance silicon substrate compression in seal. ", 1024)) // ~64 KB
)

// --- GZIP BENCHMARKS: Stdlib vs seal/codec ---

func BenchmarkGzip_Stdlib_Decompress_1KB(b *testing.B) {
	compressed := createGzipData(b, payloadSmall)
	b.SetBytes(int64(len(payloadSmall)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r, err := stdgzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.ReadAll(r)
		_ = r.Close()
	}
}

func BenchmarkGzip_SealCodec_Decompress_1KB(b *testing.B) {
	compressed := createGzipData(b, payloadSmall)
	dst := make([]byte, 0, len(payloadSmall))
	b.SetBytes(int64(len(payloadSmall)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = codec.Gunzip(compressed, dst)
	}
}

func BenchmarkGzip_Stdlib_Decompress_64KB(b *testing.B) {
	compressed := createGzipData(b, payloadMedium)
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r, err := stdgzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.ReadAll(r)
		_ = r.Close()
	}
}

func BenchmarkGzip_SealCodec_Decompress_64KB(b *testing.B) {
	compressed := createGzipData(b, payloadMedium)
	dst := make([]byte, 0, len(payloadMedium))
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = codec.Gunzip(compressed, dst)
	}
}

func BenchmarkGzip_SealCodec_DecompressScoped_64KB(b *testing.B) {
	compressed := createGzipData(b, payloadMedium)
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	s := borrow.AcquireScope()
	defer s.Release()

	for b.Loop() {
		_, _ = codec.GunzipScoped(s, compressed)
	}
}

// --- DEFLATE BENCHMARKS: Stdlib vs seal/codec ---

func BenchmarkDeflate_Stdlib_Decompress_64KB(b *testing.B) {
	compressed := createDeflateData(b, payloadMedium)
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r := stdflate.NewReader(bytes.NewReader(compressed))
		_, _ = io.ReadAll(r)
		_ = r.Close()
	}
}

func BenchmarkDeflate_SealCodec_Decompress_64KB(b *testing.B) {
	compressed := createDeflateData(b, payloadMedium)
	dst := make([]byte, 0, len(payloadMedium))
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = codec.Inflate(compressed, dst)
	}
}

// --- ZSTD & BROTLI BENCHMARKS ---

func BenchmarkZstd_SealCodec_Decompress_64KB(b *testing.B) {
	compressed := createZstdRawBlock(payloadMedium)
	dst := make([]byte, 0, len(payloadMedium))
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = codec.Unzstd(compressed, dst)
	}
}

func BenchmarkBrotli_SealCodec_Decompress_64KB(b *testing.B) {
	compressed, _ := codec.CompressBrotli(payloadMedium, nil)
	dst := make([]byte, 0, len(payloadMedium))
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = codec.Unbrotli(compressed, dst)
	}
}

// --- COMPRESSION BENCHMARKS: Gzip / Deflate / Brotli ---

func BenchmarkGzip_Stdlib_Compress_64KB(b *testing.B) {
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var buf bytes.Buffer
		w := stdgzip.NewWriter(&buf)
		_, _ = w.Write(payloadMedium)
		_ = w.Close()
	}
}

func BenchmarkGzip_SealCodec_Compress_64KB(b *testing.B) {
	dst := make([]byte, 0, len(payloadMedium))
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = codec.CompressGzip(payloadMedium, dst)
	}
}

func BenchmarkBrotli_SealCodec_Compress_64KB(b *testing.B) {
	dst := make([]byte, 0, len(payloadMedium))
	b.SetBytes(int64(len(payloadMedium)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = codec.CompressBrotli(payloadMedium, dst)
	}
}
