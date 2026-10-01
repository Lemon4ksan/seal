// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/lemon4ksan/przm"
)

type BenchmarkCase struct {
	Category    string
	Name        string
	Description string
	Data        []byte
}

type BenchResult struct {
	OriginalSize   int
	CompressedSize int
	Ratio          float64
	CompMBs        float64
	DecompMBs      float64
	DecompAllocs   uint64
	RoundtripOK    bool
}

func main() {
	fmt.Println("==========================================================================================")
	fmt.Println("       PRZM LIVE INTEGRATION EVALUATION: AES-256 & SHADER / GPU PIPELINE                  ")
	fmt.Println("==========================================================================================")

	cases := []BenchmarkCase{
		generatePureAES256CTR(256 * 1024),
		generateAES256GCMContainer(256 * 1024),
		generateAES256CBCPadding(256 * 1024),
		generateShaderVec4Buffer(256 * 1024),
		generateSPIRVShaderBytecode(256 * 1024),
		generate4ChannelQuantizedNormals(256 * 1024),
	}

	for _, bc := range cases {
		fmt.Printf("\n>>> [%s] %s (%d KiB)\n", bc.Category, bc.Name, len(bc.Data)/1024)
		fmt.Printf("    Description: %s\n", bc.Description)
		fmt.Printf("    %-22s | %-12s | %-12s | %-8s | %-12s | %-12s | %-7s\n",
			"PRZM Configuration", "Raw (Bytes)", "Comp (Bytes)", "Ratio", "Comp (MB/s)", "Decomp(MB/s)", "Allocs")
		fmt.Println("    -----------------------+--------------+--------------+----------+--------------+--------------+--------")

		// 1. Standard PRZM Level 1 (Fast)
		runBenchmark("PRZM (L1 Stream)", bc.Data, func(src []byte) ([]byte, error) {
			return przm.Compress(src, 1)
		}, func(comp []byte) ([]byte, error) {
			return przm.Decompress(comp)
		})

		// 2. Standard PRZM Level 3 (Default Balance)
		runBenchmark("PRZM (L3 Balanced)", bc.Data, func(src []byte) ([]byte, error) {
			return przm.Compress(src, 3)
		}, func(comp []byte) ([]byte, error) {
			return przm.Decompress(comp)
		})

		// 3. PRZM 4D IsoQuant (Quaternionic Collimation)
		runBenchmark("PRZM (4D IsoQuant)", bc.Data, func(src []byte) ([]byte, error) {
			return przm.CompressIsoQuant(src)
		}, func(comp []byte) ([]byte, error) {
			dst := make([]byte, len(bc.Data)+128)
			return przm.DecompressIsoQuant(comp, dst)
		})
	}
}

func runBenchmark(name string, src []byte, compFn func([]byte) ([]byte, error), decompFn func([]byte) ([]byte, error)) {
	// Warm-up & roundtrip verification
	compressed, err := compFn(src)
	if err != nil {
		fmt.Printf("    %-22s | ERROR: %v\n", name, err)
		return
	}

	decompressed, err := decompFn(compressed)
	if err != nil {
		fmt.Printf("    %-22s | DECOMP ERROR: %v\n", name, err)
		return
	}

	roundtripOK := bytes.Equal(src, decompressed)
	if !roundtripOK {
		fmt.Printf("    %-22s | BITSTREAM MISMATCH!\n", name)
		return
	}

	// Benchmark Compression Speed
	compIters := 20
	if len(src) > 512*1024 {
		compIters = 5
	}
	startComp := time.Now()
	for i := 0; i < compIters; i++ {
		_, _ = compFn(src)
	}
	compDur := time.Since(startComp)
	compMBs := (float64(len(src)*compIters) / (1024 * 1024)) / compDur.Seconds()

	// Benchmark Decompression Speed & Allocations
	decompIters := 50
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)
	startDecomp := time.Now()
	for i := 0; i < decompIters; i++ {
		_, _ = decompFn(compressed)
	}
	decompDur := time.Since(startDecomp)
	runtime.ReadMemStats(&m2)

	decompMBs := (float64(len(src)*decompIters) / (1024 * 1024)) / decompDur.Seconds()
	allocsPerOp := (m2.Mallocs - m1.Mallocs) / uint64(decompIters)
	ratio := float64(len(src)) / float64(len(compressed))

	fmt.Printf("    %-22s | %-12d | %-12d | %-7.4fx | %-10.1f MB/s | %-10.1f MB/s | %-6d\n",
		name, len(src), len(compressed), ratio, compMBs, decompMBs, allocsPerOp)
}

// -----------------------------------------------------------------------------
// Dataset Generators
// -----------------------------------------------------------------------------

func generatePureAES256CTR(size int) BenchmarkCase {
	key := make([]byte, 32)
	iv := make([]byte, 16)
	_, _ = rand.Read(key)
	_, _ = rand.Read(iv)

	block, _ := aes.NewCipher(key)
	stream := cipher.NewCTR(block, iv)

	// Encrypt a structured text buffer so any pattern found is purely inside the cipher/stream
	plaintext := bytes.Repeat([]byte("QuantumSymmetryPRZMInformationEntropyCollimation"), size/48+1)[:size]
	ciphertext := make([]byte, size)
	stream.XORKeyStream(ciphertext, plaintext)

	return BenchmarkCase{
		Category:    "CRYPTOGRAPHY",
		Name:        "Pure AES-256-CTR Ciphertext",
		Description: "Raw 14-round AES-256 keystream output without container headers or padding",
		Data:        ciphertext,
	}
}

func generateAES256GCMContainer(size int) BenchmarkCase {
	key := make([]byte, 32)
	nonce := make([]byte, 12)
	_, _ = rand.Read(key)
	_, _ = rand.Read(nonce)

	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)

	sample := []byte("{\"metric\":\"temperature_sensor_gpu\",\"val\":42.5,\"status\":\"OK\"}\n")
	plaintext := bytes.Repeat(sample, size/len(sample)+1)[:size]
	sealed := gcm.Seal(nil, nonce, plaintext, []byte("TLS_FRAME_HEADER_METADATA_AUTH"))

	return BenchmarkCase{
		Category:    "CRYPTOGRAPHY",
		Name:        "AES-256-GCM Container Payload",
		Description: "Authenticated TLS/Storage container: includes IV, GHASH 128-bit MAC tag & ciphertext",
		Data:        sealed,
	}
}

func generateAES256CBCPadding(size int) BenchmarkCase {
	key := make([]byte, 32)
	iv := make([]byte, 16)
	_, _ = rand.Read(key)
	_, _ = rand.Read(iv)

	block, _ := aes.NewCipher(key)
	mode := cipher.NewCBCEncrypter(block, iv)

	// Multiple chunks with PKCS#7-like padding blocks interspersed (like disk encryption sectors)
	buf := make([]byte, size)
	sectorSize := 4096
	for offset := 0; offset < size; offset += sectorSize {
		chunk := buf[offset : offset+sectorSize]
		// Fill half with structured data, half with padding
		copy(chunk, []byte("SECTOR_RECORD_HEADER_SYSTEM_LOG_AUDIT_DATA"))
		for i := 256; i < sectorSize; i++ {
			chunk[i] = 0x10 // standard padding
		}
		mode.CryptBlocks(chunk, chunk)
	}

	return BenchmarkCase{
		Category:    "CRYPTOGRAPHY",
		Name:        "AES-256-CBC Sector Blocks",
		Description: "Simulates encrypted disk pages/database blocks with periodic CBC state and PKCS#7 runs",
		Data:        buf,
	}
}

func generateShaderVec4Buffer(size int) BenchmarkCase {
	// 4-Channel RGBA / Vec4 vertex attributes: Position (X, Y, Z, W) with continuous surface curvature
	buf := make([]byte, size)
	numVecs := size / 16
	for i := 0; i < numVecs; i++ {
		t := float64(i) * 0.05
		// Parametric 3D curve with correlated components
		x := float32(math.Sin(t))
		y := float32(math.Cos(t))
		z := float32(math.Sin(t * 0.5))
		w := float32(1.0) // Homogeneous coordinate

		xb := math.Float32bits(x)
		yb := math.Float32bits(y)
		zb := math.Float32bits(z)
		wb := math.Float32bits(w)

		base := i * 16
		buf[base+0] = byte(xb)
		buf[base+1] = byte(xb >> 8)
		buf[base+2] = byte(xb >> 16)
		buf[base+3] = byte(xb >> 24)

		buf[base+4] = byte(yb)
		buf[base+5] = byte(yb >> 8)
		buf[base+6] = byte(yb >> 16)
		buf[base+7] = byte(yb >> 24)

		buf[base+8] = byte(zb)
		buf[base+9] = byte(zb >> 8)
		buf[base+10] = byte(zb >> 16)
		buf[base+11] = byte(zb >> 24)

		buf[base+12] = byte(wb)
		buf[base+13] = byte(wb >> 8)
		buf[base+14] = byte(wb >> 16)
		buf[base+15] = byte(wb >> 24)
	}

	return BenchmarkCase{
		Category:    "GRAPHICS / SHADER",
		Name:        "4D Vec4 Vertex Stream (Float32)",
		Description: "3D Graphics vertex buffer: X, Y, Z, W coordinates with strong quaternionic rotation correlation",
		Data:        buf,
	}
}

func generateSPIRVShaderBytecode(size int) BenchmarkCase {
	// Emulate SPIR-V / GPU SASS bytecode: 4-byte words, opcodes, operand IDs, relative registers
	var buf bytes.Buffer
	for buf.Len() < size {
		// OpCapability Shader (0x00020011)
		buf.Write([]byte{0x11, 0x00, 0x02, 0x00, 0x01, 0x00, 0x00, 0x00})
		// OpMemoryModel Logical GLSL450 (0x0003000E)
		buf.Write([]byte{0x0E, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00})
		// OpTypeFloat 32 (0x00030016)
		buf.Write([]byte{0x16, 0x00, 0x03, 0x00, 0x02, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00})
		// OpTypeVector 4 (0x00040017)
		buf.Write([]byte{0x17, 0x00, 0x04, 0x00, 0x03, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00})
		// OpFMul / OpFAdd arithmetic instructions with repeating register patterns
		buf.Write([]byte{0x85, 0x00, 0x05, 0x00, 0x03, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0x06, 0x00, 0x00, 0x00})
	}

	return BenchmarkCase{
		Category:    "GRAPHICS / SHADER",
		Name:        "SPIR-V GPU Shader Bytecode",
		Description: "Compiled Vulkan/DirectX shader stream with 32-bit word alignment and recurring ALU opcodes",
		Data:        buf.Bytes()[:size],
	}
}

func generate4ChannelQuantizedNormals(size int) BenchmarkCase {
	// Tangent/Normal/Bitangent (TBN) 4-channel 8-bit quantized maps: Nx, Ny, Nz, Curvature
	buf := make([]byte, size)
	for i := 0; i < size; i += 4 {
		phi := float64(i) * 0.02
		nx := uint8((math.Sin(phi)*0.5 + 0.5) * 255)
		ny := uint8((math.Cos(phi)*0.5 + 0.5) * 255)
		nz := uint8((math.Sin(phi*2)*0.3 + 0.7) * 255)
		curv := uint8(128)

		buf[i+0] = nx
		buf[i+1] = ny
		buf[i+2] = nz
		buf[i+3] = curv
	}

	return BenchmarkCase{
		Category:    "GRAPHICS / SHADER",
		Name:        "4-Channel TBN Normal Map",
		Description: "Compressed normal maps (Nx, Ny, Nz, Curvature) with spherical manifold constraint",
		Data:        buf,
	}
}
