// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/option"
)

func generateDataset(dir string) (int64, error) {
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		return 0, err
	}

	var total int64

	// Text source code (3 MB)
	var textBuf bytes.Buffer
	for i := range 30000 {
		fmt.Fprintf(
			&textBuf,
			"package main\n\nfunc Function%d(param string) (int, error) {\n\tif len(param) > 0 {\n\t\treturn %d, nil\n\t}\n\treturn 0, fmt.Errorf(\"empty parameter in function %d\")\n}\n\n",
			i,
			i*2,
			i,
		)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "code.go"), textBuf.Bytes(), 0o644); err != nil {
		return 0, err
	}
	total += int64(textBuf.Len())

	// Binary mock x86 opcodes (4 MB)
	binData := make([]byte, 4*1024*1024)
	_, _ = rand.Read(binData)
	for i := 100; i < len(binData)-64; i += 32 {
		binData[i] = 0xE8
		binData[i+1] = byte(i)
		binData[i+2] = byte(i >> 8)
		binData[i+3] = 0x00
		binData[i+4] = 0x00

		binData[i+16] = 0xE9
		binData[i+17] = byte(i + 4)
		binData[i+18] = byte((i + 4) >> 8)
		binData[i+19] = 0x00
		binData[i+20] = 0x00
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "app.exe"), binData, 0o644); err != nil {
		return 0, err
	}
	total += int64(len(binData))

	// JSON structured data (3 MB)
	var jsonBuf bytes.Buffer
	for i := range 20000 {
		fmt.Fprintf(
			&jsonBuf,
			`{"timestamp":"2026-09-01T21:00:00Z","level":"INFO","event_id":%d,"service":"gateway-auth","message":"User login succeeded from remote IP 192.168.1.%d","latency_ms":%d}`+"\n",
			i,
			i%250,
			(i*37)%150,
		)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "events.json"), jsonBuf.Bytes(), 0o644); err != nil {
		return 0, err
	}
	total += int64(jsonBuf.Len())

	return total, nil
}

func main() {
	tempDir, err := os.MkdirTemp("", "seal_prof_*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)

	datasetDir := filepath.Join(tempDir, "dataset")
	totalBytes, err := generateDataset(datasetDir)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Dataset generated: %.2f MB\n", float64(totalBytes)/(1024*1024))

	// Start CPU Profiling
	cpuF, err := os.Create("cpu_compress.pprof")
	if err != nil {
		panic(err)
	}
	defer cpuF.Close()

	if err := pprof.StartCPUProfile(cpuF); err != nil {
		panic(err)
	}

	sealArchive := filepath.Join(tempDir, "out.7z")
	archiver := seal.NewArchiver(option.WithContainer(container.SevenZip))

	start := time.Now()
	// Run compression iterations to get deep profiling data
	for iter := 0; iter < 5; iter++ {
		_ = os.Remove(sealArchive)
		if err := archiver.ArchiveDir(datasetDir, seal.ToFilePath(sealArchive)); err != nil {
			panic(err)
		}
	}
	dur := time.Since(start)
	pprof.StopCPUProfile()
	fmt.Printf("Compression 5 iterations took: %v (avg %.1f ms/iter)\n", dur, float64(dur.Milliseconds())/5.0)

	// Profiling Decompression
	cpuDecF, err := os.Create("cpu_decompress.pprof")
	if err != nil {
		panic(err)
	}
	defer cpuDecF.Close()

	if err := pprof.StartCPUProfile(cpuDecF); err != nil {
		panic(err)
	}
	startDec := time.Now()
	extractDir := filepath.Join(tempDir, "extracted")
	for range 10 {
		_ = os.RemoveAll(extractDir)
		if err := seal.Unarchive(seal.FromFilePath(sealArchive), extractDir); err != nil {
			panic(err)
		}
	}
	durDec := time.Since(startDec)
	pprof.StopCPUProfile()
	fmt.Printf("Decompression 10 iterations took: %v (avg %.1f ms/iter)\n", durDec, float64(durDec.Milliseconds())/10.0)

	// Heap Profile
	memF, err := os.Create("mem.pprof")
	if err != nil {
		panic(err)
	}
	defer memF.Close()
	_ = pprof.WriteHeapProfile(memF)
	fmt.Println("Profiles written: cpu_compress.pprof, cpu_decompress.pprof, mem.pprof")
}
