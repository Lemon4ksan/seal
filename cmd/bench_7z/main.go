// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/option"
)

const sevenZipExe = `C:\Program Files\7-Zip\7z.exe`

func generateDataset(dir string) (int64, error) {
	if err := os.MkdirAll(filepath.Join(dir, "src", "pkg"), 0o755); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		return 0, err
	}

	var total int64

	// Generate text/source code with keyword repetitions
	var textBuf bytes.Buffer
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(
			&textBuf,
			"package main\n\nfunc Function%d(param string) (int, error) {\n\tif len(param) > 0 {\n\t\treturn %d, nil\n\t}\n\treturn 0, fmt.Errorf(\"empty parameter in function %d\")\n}\n\n",
			i,
			i*2,
			i,
		)
	}
	p1 := filepath.Join(dir, "src", "code.go")
	if err := os.WriteFile(p1, textBuf.Bytes(), 0o644); err != nil {
		return 0, err
	}
	total += int64(textBuf.Len())

	// Generate synthetic executable binary with x86 relative branch opcodes (CALL/JMP)
	binData := make([]byte, 2*1024*1024) // 2 MB
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
	p2 := filepath.Join(dir, "bin", "app.exe")
	if err := os.WriteFile(p2, binData, 0o644); err != nil {
		return 0, err
	}
	total += int64(len(binData))

	// Generate structured JSON payload simulating microservice audit logs
	var jsonBuf bytes.Buffer
	for i := range 15000 {
		fmt.Fprintf(
			&jsonBuf,
			`{"timestamp":"2026-09-01T21:00:00Z","level":"INFO","event_id":%d,"service":"gateway-auth","message":"User login succeeded from remote IP 192.168.1.%d","latency_ms":%d}`+"\n",
			i,
			i%250,
			(i*37)%150,
		)
	}
	p3 := filepath.Join(dir, "data", "events.json")
	if err := os.WriteFile(p3, jsonBuf.Bytes(), 0o644); err != nil {
		return 0, err
	}
	total += int64(jsonBuf.Len())

	return total, nil
}

func runBench(modeName string, level, threads int, datasetDir, tempDir string, totalUncompressed int64) {
	origProcs := runtime.GOMAXPROCS(threads)
	defer runtime.GOMAXPROCS(origProcs)

	fmt.Printf("\n========================================================================\n")
	fmt.Printf("   BENCHMARK: %s | LEVEL %d (-mx=%d) (%d Thread)\n", modeName, level, level, threads)
	fmt.Printf("========================================================================\n")

	// Execute Seal 7z compression
	sealArchive := filepath.Join(tempDir, fmt.Sprintf("seal_lvl%d_%d.7z", level, threads))
	archiver := seal.NewArchiver(
		option.WithContainer(container.SevenZip),
		option.WithCompressionLevel(level),
		option.WithWorkers(threads),
	)

	startSealCompress := time.Now()
	if err := archiver.ArchiveDir(datasetDir, seal.ToFilePath(sealArchive)); err != nil {
		panic(fmt.Sprintf("Seal ArchiveDir failed: %v", err))
	}
	sealCompressDuration := time.Since(startSealCompress)

	sealStat, err := os.Stat(sealArchive)
	if err != nil {
		panic(err)
	}
	sealCompressedSize := sealStat.Size()

	// Execute official 7-Zip CLI compression for baseline comparison
	sevenZipArchive := filepath.Join(tempDir, fmt.Sprintf("7zip_lvl%d_%d.7z", level, threads))
	mmtFlag := fmt.Sprintf("-mmt=%d", threads)
	mxFlag := fmt.Sprintf("-mx=%d", level)
	cmdCompress := exec.Command(
		sevenZipExe,
		"a",
		"-t7z",
		mxFlag,
		"-ms=on",
		mmtFlag,
		"-bso0",
		sevenZipArchive,
		datasetDir+`\*`,
	)
	start7zCompress := time.Now()
	if out, err := cmdCompress.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("7-Zip compression failed: %v, output: %s", err, string(out)))
	}
	sevenZipCompressDuration := time.Since(start7zCompress)

	sevenZipStat, err := os.Stat(sevenZipArchive)
	if err != nil {
		panic(err)
	}
	sevenZipCompressedSize := sevenZipStat.Size()

	// Execute Seal 7z extraction
	sealExtractDir := filepath.Join(tempDir, fmt.Sprintf("seal_extract_lvl%d_%d", level, threads))
	startSealDecompress := time.Now()
	if err := seal.Unarchive(seal.FromFilePath(sealArchive), sealExtractDir); err != nil {
		panic(fmt.Sprintf("Seal Unarchive failed: %v", err))
	}
	sealDecompressDuration := time.Since(startSealDecompress)

	// Cross-validate Seal archive by decompressing via official 7-Zip CLI
	sevenZipExtractDir := filepath.Join(tempDir, fmt.Sprintf("7zip_extract_lvl%d_%d", level, threads))
	cmdDecompress := exec.Command(sevenZipExe, "x", "-y", mmtFlag, "-bso0", "-o"+sevenZipExtractDir, sealArchive)
	start7zDecompress := time.Now()
	if out, err := cmdDecompress.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("7-Zip decompression failed: %v, output: %s", err, string(out)))
	}
	sevenZipDecompressDuration := time.Since(start7zDecompress)

	// Results Table
	fmt.Printf(
		"%-25s | %-22s | %-22s\n",
		"Metric",
		"Seal (Pure Go + SIMD)",
		fmt.Sprintf("7-Zip Official (-mx=%d)", level),
	)
	fmt.Printf("--------------------------+------------------------+-----------------------\n")
	fmt.Printf(
		"%-25s | %-22s | %-22s\n",
		"Archive Size",
		fmt.Sprintf("%.2f KB (%d B)", float64(sealCompressedSize)/1024, sealCompressedSize),
		fmt.Sprintf("%.2f KB (%d B)", float64(sevenZipCompressedSize)/1024, sevenZipCompressedSize),
	)
	fmt.Printf(
		"%-25s | %-22.1f%% | %-22.1f%%\n",
		"Compression Ratio",
		float64(sealCompressedSize)/float64(totalUncompressed)*100,
		float64(sevenZipCompressedSize)/float64(totalUncompressed)*100,
	)
	fmt.Printf(
		"%-25s | %-22s | %-22s\n",
		"Compression Time",
		sealCompressDuration.Round(time.Millisecond),
		sevenZipCompressDuration.Round(time.Millisecond),
	)
	fmt.Printf(
		"%-25s | %-22s | %-22s\n",
		"Compression Throughput",
		fmt.Sprintf("%.2f MB/s", (float64(totalUncompressed)/(1024*1024))/sealCompressDuration.Seconds()),
		fmt.Sprintf("%.2f MB/s", (float64(totalUncompressed)/(1024*1024))/sevenZipCompressDuration.Seconds()),
	)
	fmt.Printf(
		"%-25s | %-22s | %-22s\n",
		"Decompression Time",
		sealDecompressDuration.Round(time.Millisecond),
		sevenZipDecompressDuration.Round(time.Millisecond),
	)
	fmt.Printf(
		"%-25s | %-22s | %-22s\n",
		"Decompression Throughput",
		fmt.Sprintf("%.2f MB/s", (float64(totalUncompressed)/(1024*1024))/sealDecompressDuration.Seconds()),
		fmt.Sprintf("%.2f MB/s", (float64(totalUncompressed)/(1024*1024))/sevenZipDecompressDuration.Seconds()),
	)
	fmt.Printf("========================================================================\n")
	fmt.Printf("Cross-Verification: 7-Zip successfully extracted the archive created by Seal!\n")
}

func main() {
	tempDir, err := os.MkdirTemp("", "seal_7z_bench_*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)

	datasetDir := filepath.Join(tempDir, "dataset")
	totalUncompressed, err := generateDataset(datasetDir)
	if err != nil {
		panic(err)
	}

	fmt.Printf(
		"Total Uncompressed Dataset: %.2f MB (%d bytes)\n",
		float64(totalUncompressed)/(1024*1024),
		totalUncompressed,
	)

	numCPU := runtime.NumCPU()

	// 1. Level 1 (Fastest) - 1 Core & Multi Core
	runBench("SINGLE-THREADED", 1, 1, datasetDir, tempDir, totalUncompressed)
	runBench("MULTI-THREADED", 1, numCPU, datasetDir, tempDir, totalUncompressed)

	// 2. Level 5 (Normal) - 1 Core & Multi Core
	runBench("SINGLE-THREADED", 5, 1, datasetDir, tempDir, totalUncompressed)
	runBench("MULTI-THREADED", 5, numCPU, datasetDir, tempDir, totalUncompressed)

	// 3. Level 9 (Ultra) - 1 Core & Multi Core
	runBench("SINGLE-THREADED", 9, 1, datasetDir, tempDir, totalUncompressed)
	runBench("MULTI-THREADED", 9, numCPU, datasetDir, tempDir, totalUncompressed)
}
