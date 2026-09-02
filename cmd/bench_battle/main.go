// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/codec/bcj"
	"github.com/lemon4ksan/seal/codec/brotli"
	_ "github.com/lemon4ksan/seal/codec/legacy"
	"github.com/lemon4ksan/seal/codec/lzma"
	"github.com/lemon4ksan/seal/codec/ppmd"
	"github.com/lemon4ksan/seal/codec/zstd"
)

type DataSet struct {
	Name        string
	Description string
	Data        []byte
}

type Result struct {
	Algorithm      string
	Generation     string
	CompressedSize int
	Ratio          float64
	CompSpeedMBs   float64
	DecompSpeedMBs float64
}

func main() {
	fmt.Println("=========================================================================================")
	fmt.Println("      SEAL COMPRESSION BATTLE: 35 YEARS OF ALGORITHMIC EVOLUTION (1989 - 2026)          ")
	fmt.Println("=========================================================================================")

	datasets := []DataSet{
		generateSourceCodeDataSet(),
		generateX86BinaryDataSet(),
		generateTabularCSVDataSet(),
		generateNaturalTextDataSet(),
		generateHighEntropyDataSet(),
	}

	for _, ds := range datasets {
		fmt.Printf("\n--- DATASET: %s (%d KB) ---\n", ds.Name, len(ds.Data)/1024)
		fmt.Printf("Description: %s\n\n", ds.Description)
		fmt.Printf("%-24s | %-15s | %-10s | %-8s | %-13s | %-14s\n",
			"Algorithm", "Era / Type", "Size (KB)", "Ratio", "Comp (MB/s)", "Decomp (MB/s)")
		fmt.Println(
			"-------------------------+-----------------+------------+----------+---------------+---------------",
		)

		results := runBattleOnDataSet(ds.Data)
		for _, r := range results {
			fmt.Printf("%-24s | %-15s | %8.1f KB | %7.2f%% | %10.1f MB/s | %10.1f MB/s\n",
				r.Algorithm, r.Generation, float64(r.CompressedSize)/1024.0, r.Ratio, r.CompSpeedMBs, r.DecompSpeedMBs)
		}
	}
}

func runBattleOnDataSet(data []byte) []Result {
	results := make([]Result, 0, 12)
	reg := codec.DefaultRegistry

	// 1. Generation 1: 1989 PKZIP 1.0 (Shrink)
	results = append(results, benchGeneric("Shrink (LZW 9-13b)", "1989 (DOS/ARC)", codec.Shrink, 0, data, reg))

	// 2. Generation 2: 1990s Classic (Deflate, Gzip)
	results = append(results, benchGeneric("Deflate Level 1 (Fast)", "1993 (Classic)", codec.Deflate, 1, data, reg))
	results = append(results, benchGeneric("Deflate Level 9 (Best)", "1993 (Classic)", codec.Deflate, 9, data, reg))
	results = append(results, benchGeneric("Gzip (RFC 1952)", "1996 (Web)", codec.Gzip, 6, data, reg))

	// 3. Generation 3: 2000s High Ratio (LZMA2, BCJ+LZMA2, PPMd)
	results = append(results, benchLZMA2("LZMA2 Level 1 (Fast)", "2000s (7-Zip)", data, lzma.LevelFastest, false))
	results = append(results, benchLZMA2("LZMA2 Level 9 (Ultra)", "2000s (7-Zip)", data, lzma.LevelUltra, false))
	results = append(results, benchLZMA2("BCJ + LZMA2 Level 9", "2000s (7-Zip)", data, lzma.LevelUltra, true))
	results = append(results, benchPPMd("PPMd Order 8", "2000s (PPM)", data))

	// 4. Generation 4: Modern Real-Time & Pareto Frontier (Zstandard, Brotli)
	results = append(results, benchZstandard("Zstandard Level 1", "2016 (Realtime)", data, zstd.SpeedFastest))
	results = append(results, benchZstandard("Zstandard Level 3", "2016 (Balanced)", data, zstd.SpeedDefault))
	results = append(results, benchBrotli("Brotli Level 4", "2015 (Web/RFC)", data, 4))
	results = append(results, benchBrotli("Brotli Level 9", "2015 (Web/RFC)", data, 9))

	return results
}

func benchGeneric(name, era string, method codec.Method, level int, data []byte, reg *codec.Registry) Result {
	comp, err := reg.Compressor(method, level)
	if err != nil {
		return Result{Algorithm: name, Generation: era}
	}

	var compBuf bytes.Buffer
	start := time.Now()
	_, err = comp.Compress(bytes.NewReader(data), &compBuf)
	compDuration := time.Since(start)
	if err != nil {
		return Result{Algorithm: name, Generation: era}
	}

	compressedBytes := compBuf.Bytes()
	compSpeed := float64(len(data)) / compDuration.Seconds() / (1024 * 1024)

	dec, err := reg.Decompressor(method)
	if err != nil {
		return Result{
			Algorithm: name, Generation: era,
			CompressedSize: len(compressedBytes),
			Ratio:          float64(len(compressedBytes)) / float64(len(data)) * 100,
			CompSpeedMBs:   compSpeed,
		}
	}

	start = time.Now()
	rc, err := dec.Decompress(bytes.NewReader(compressedBytes))
	var decompSpeed float64
	if err == nil {
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
		decompDuration := time.Since(start)
		decompSpeed = float64(len(data)) / decompDuration.Seconds() / (1024 * 1024)
	}

	return Result{
		Algorithm:      name,
		Generation:     era,
		CompressedSize: len(compressedBytes),
		Ratio:          float64(len(compressedBytes)) / float64(len(data)) * 100,
		CompSpeedMBs:   compSpeed,
		DecompSpeedMBs: decompSpeed,
	}
}

func benchLZMA2(name, era string, data []byte, level lzma.Level, useBCJ bool) Result {
	src := data
	if useBCJ {
		filtered := make([]byte, len(data))
		copy(filtered, data)
		bcj.Filter(bcj.X86, filtered, 0, true)
		src = filtered
	}

	var compBuf bytes.Buffer
	comp := lzma.NewCompressor2(8 * 1024 * 1024)
	comp.Options = lzma.OptionsForLevel(level)

	start := time.Now()
	_, _ = comp.Compress(bytes.NewReader(src), &compBuf)
	compDuration := time.Since(start)

	compBytes := compBuf.Bytes()
	compSpeed := float64(len(data)) / compDuration.Seconds() / (1024 * 1024)

	// Decompress
	dec := lzma.NewDecompressor2(8 * 1024 * 1024)
	start = time.Now()
	rc, err := dec.Decompress(bytes.NewReader(compBytes))
	var decompSpeed float64
	if err == nil {
		var decompBuf bytes.Buffer
		_, _ = io.Copy(&decompBuf, rc)
		_ = rc.Close()
		if useBCJ {
			decompRaw := decompBuf.Bytes()
			bcj.Filter(bcj.X86, decompRaw, 0, false)
		}
		decompDuration := time.Since(start)
		decompSpeed = float64(len(data)) / decompDuration.Seconds() / (1024 * 1024)
	}

	return Result{
		Algorithm:      name,
		Generation:     era,
		CompressedSize: len(compBytes),
		Ratio:          float64(len(compBytes)) / float64(len(data)) * 100,
		CompSpeedMBs:   compSpeed,
		DecompSpeedMBs: decompSpeed,
	}
}

func benchPPMd(name, era string, data []byte) Result {
	var compBuf bytes.Buffer
	comp := ppmd.NewCompressor(8, 8*1024*1024)

	start := time.Now()
	_, _ = comp.Compress(bytes.NewReader(data), &compBuf)
	compDuration := time.Since(start)

	compBytes := compBuf.Bytes()
	compSpeed := float64(len(data)) / compDuration.Seconds() / (1024 * 1024)

	dec := ppmd.NewDecompressor(8, 8*1024*1024)
	start = time.Now()
	rc, err := dec.Decompress(bytes.NewReader(compBytes), uint64(len(data)))
	var decompSpeed float64
	if err == nil {
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
		decompDuration := time.Since(start)
		decompSpeed = float64(len(data)) / decompDuration.Seconds() / (1024 * 1024)
	}

	return Result{
		Algorithm:      name,
		Generation:     era,
		CompressedSize: len(compBytes),
		Ratio:          float64(len(compBytes)) / float64(len(data)) * 100,
		CompSpeedMBs:   compSpeed,
		DecompSpeedMBs: decompSpeed,
	}
}

func benchZstandard(name, era string, data []byte, level zstd.EncoderLevel) Result {
	var compBuf bytes.Buffer
	enc, err := zstd.NewWriter(&compBuf, zstd.WithEncoderLevel(level))
	if err != nil {
		return Result{Algorithm: name, Generation: era}
	}

	start := time.Now()
	_, _ = enc.Write(data)
	_ = enc.Close()
	compDuration := time.Since(start)

	compBytes := compBuf.Bytes()
	compSpeed := float64(len(data)) / compDuration.Seconds() / (1024 * 1024)

	start = time.Now()
	dec, err := zstd.NewReader(bytes.NewReader(compBytes))
	var decompSpeed float64
	if err == nil {
		_, _ = io.Copy(io.Discard, dec)
		dec.Close()
		decompDuration := time.Since(start)
		decompSpeed = float64(len(data)) / decompDuration.Seconds() / (1024 * 1024)
	}

	return Result{
		Algorithm:      name,
		Generation:     era,
		CompressedSize: len(compBytes),
		Ratio:          float64(len(compBytes)) / float64(len(data)) * 100,
		CompSpeedMBs:   compSpeed,
		DecompSpeedMBs: decompSpeed,
	}
}

func benchBrotli(name, era string, data []byte, level int) Result {
	var compBuf bytes.Buffer
	bw := brotli.NewWriterLevel(&compBuf, level)

	start := time.Now()
	_, _ = bw.Write(data)
	_ = bw.Close()
	compDuration := time.Since(start)

	compBytes := compBuf.Bytes()
	compSpeed := float64(len(data)) / compDuration.Seconds() / (1024 * 1024)

	start = time.Now()
	br := brotli.NewReader(bytes.NewReader(compBytes))
	var decompSpeed float64
	_, _ = io.Copy(io.Discard, br)
	decompDuration := time.Since(start)
	decompSpeed = float64(len(data)) / decompDuration.Seconds() / (1024 * 1024)

	return Result{
		Algorithm:      name,
		Generation:     era,
		CompressedSize: len(compBytes),
		Ratio:          float64(len(compBytes)) / float64(len(data)) * 100,
		CompSpeedMBs:   compSpeed,
		DecompSpeedMBs: decompSpeed,
	}
}

func generateSourceCodeDataSet() DataSet {
	codeSnippet := `
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Server struct {
	mu     sync.RWMutex
	routes map[string]http.HandlerFunc
	cache  map[string][]byte
}

func NewServer() *Server {
	return &Server{
		routes: make(map[string]http.HandlerFunc),
		cache:  make(map[string][]byte),
	}
}

func (s *Server) Handle(pattern string, handler http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[pattern] = handler
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	h, ok := s.routes[r.URL.Path]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	h(w, r)
}
`
	var b strings.Builder
	for b.Len() < 512*1024 {
		b.WriteString(codeSnippet)
	}
	return DataSet{
		Name:        "Source Code / Go AST",
		Description: "High keyword density, structural indentation, repetitive identifiers",
		Data:        []byte(b.String()[:512*1024]),
	}
}

func generateX86BinaryDataSet() DataSet {
	var buf bytes.Buffer
	// Emulate x86_64 machine code with CALL (0xE8) and JMP (0xE9) relative offsets
	for buf.Len() < 512*1024 {
		// Function prologue
		buf.Write([]byte{0x55, 0x48, 0x89, 0xE5, 0x48, 0x83, 0xEC, 0x20})
		// CALL relative
		buf.Write([]byte{0xE8, 0x40, 0x12, 0x00, 0x00})
		// MOV, ADD, TEST
		buf.Write([]byte{0x8B, 0x45, 0xFC, 0x83, 0xC0, 0x01, 0x85, 0xC0})
		// JNZ relative
		buf.Write([]byte{0x0F, 0x85, 0x10, 0xFF, 0xFF, 0xFF})
		// Function epilogue
		buf.Write([]byte{0x48, 0x83, 0xC4, 0x20, 0x5D, 0xC3})
	}
	return DataSet{
		Name:        "x86_64 Machine Code",
		Description: "Binary instruction stream with relative CALL/JMP jump offsets",
		Data:        buf.Bytes()[:512*1024],
	}
}

func generateTabularCSVDataSet() DataSet {
	var b strings.Builder
	b.WriteString("id,timestamp,user_id,action,ip_address,status_code,response_time_ms\n")
	for i := 10000; b.Len() < 512*1024; i++ {
		fmt.Fprintf(&b, "%d,2026-09-02T12:00:%02dZ,user_%04d,QUERY_DATABASE,192.168.1.%d,200,%d.45\n",
			i, i%60, i%500, i%254+1, (i%15)+5)
	}
	return DataSet{
		Name:        "Tabular CSV / Server Logs",
		Description: "Semi-structured column-oriented records with recurring timestamps and IPs",
		Data:        []byte(b.String()[:512*1024]),
	}
}

func generateNaturalTextDataSet() DataSet {
	sample := "The quick brown fox jumps over the lazy dog. In computer science, data compression, source coding, or bit-rate reduction is the process of encoding information using fewer bits than the original representation. Any particular compression is either lossy or lossless. Lossless compression reduces bits by identifying and eliminating statistical redundancy. No information is lost in lossless compression. Lossy compression reduces bits by removing unnecessary or less important information. "
	var b strings.Builder
	for b.Len() < 512*1024 {
		b.WriteString(sample)
	}
	return DataSet{
		Name:        "Natural Language Text",
		Description: "English prose with standard grammatical syntax and letter distribution",
		Data:        []byte(b.String()[:512*1024]),
	}
}

func generateHighEntropyDataSet() DataSet {
	data := make([]byte, 256*1024)
	_, _ = rand.Read(data)
	return DataSet{
		Name:        "High-Entropy / Incompressible",
		Description: "Cryptographically random bytes simulating compressed media or encryption",
		Data:        data,
	}
}
