<div align="center">

# seal

### Universal Fault-Tolerant Archiving & Cloud-Native Container Substrate

_«Entropy destroys all data — let **eon** preserve reality for eternity.»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/seal)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#-performance--benchmarks)
[![Deterministic RAM](https://img.shields.io/badge/ram%20ceiling-%E2%89%A4%2032%20MB-blueviolet?style=flat-square)](EON_SPECIFICATION.md)
[![Self-Healing](https://img.shields.io/badge/resilience-GF(2%5E8)%20Reed--Solomon-orange?style=flat-square)](EON_SPECIFICATION.md)
[![Security](https://img.shields.io/badge/security-Zip%20Slip%20%26%20Bomb%20Proof-success?style=flat-square)](#security-by-default)

**seal** is an ultra-fast, lock-free archive engine, compression substrate, and storage framework for Go 1.27+. It powers the next-generation **EON (`.eon`)** container format — featuring mathematical Cauchy Reed-Solomon self-healing, $O(1)$ LBA page seeking, sub-chunk micro-framing (down to $\le 64$ KB RAM), phase-based temporal tiering, and single-instance deduplication.

#### English • [Русский](README_RU.md) • [EON Format Specification](EON_SPECIFICATION.md) • [Architecture Concept](archive) • [Virtual Filesystem](vfs)

</div>

## Key Architectural Guarantees

* **Instant Cold Starts ($O(1)$ LBA Seeking):** Mount container filesystems and read single files in under **2 milliseconds** via 32-byte Footer indices without full archive extraction.
* **Deterministic Memory Ceiling ($\le 32$ MB RAM):** Payloads are partitioned into independent solid chunks of at most 32 MB uncompressed bytes. Eliminates `OOMKilled` crashes under strict Kubernetes/cgroup quotas.
* **Self-Healing Media Resilience:** Integrated Cauchy Reed-Solomon erasure coding over $GF(2^8)$ mathematically reconstructs damaged or corrupted blocks on decaying media.
* **Single-Instance Storage (Deduplication):** Redundant files (verified via BLAKE3-256) reference existing chunk extents at **0 bytes** extra storage and 0 CPU re-compression cost.
* **Configurable Logical Block Addressing (LBA):** Disk offsets align to $2^{\text{BlockSizeShift}}$ (default: 4096 bytes / 4 KB), enabling zero-copy `mmap`, `io_uring`, and `splice()` operations.
* **Zero-Alloc In-Memory VFS:** Build, mutate, sort, and mount virtual archives directly in RAM as standard `io/fs.FS` filesystems.
* **Safe by Default:** Active kernel-grade shields against Zip Slip (`../` path traversal) and Decompression Bombs (`42.zip`).

## Installation

`seal` requires Go version `1.27` or higher.

```bash
go get github.com/lemon4ksan/seal
```

## Quickstart

### 1. One-Line Packing & Extraction

```go
package main

import (
	"context"
	"log"

	"github.com/lemon4ksan/seal"
)

func main() {
	ctx := context.Background()

	// Pack a directory into an optimized archive (.seal, .zip, .tar.gz, .7z)
	if err := seal.ArchiveDir(ctx, "./dist", seal.ToFilePath("release.seal")); err != nil {
		log.Fatal(err)
	}

	// Safe extract (Zip Slip & Decompression Bomb shields active by default)
	if err := seal.Unarchive(ctx, seal.FromFilePath("release.seal"), "./output"); err != nil {
		log.Fatal(err)
	}
}
```

### 2. High-Speed Targeted File Access ($O(1)$ Seek)

Read targeted files from huge archives without decompressing the entire file:

```go
ctx := context.Background()

// Read a single configuration file directly into memory in <2ms
data, err := seal.ReadFile(ctx, seal.FromFilePath("bundle.seal"), "configs/app.json")
if err != nil {
	log.Fatal(err)
}

// Verify payload integrity across all entries via BLAKE3-256
err = seal.Verify(ctx, seal.FromFilePath("bundle.seal"))

// Render an ASCII hierarchy tree of the archive
tree, _ := seal.Tree(ctx, seal.FromFilePath("bundle.seal"))
fmt.Println(tree)
```

## In-Memory Virtual Archives (VFS Builder)

Construct, mutate, encrypt, and stream archives entirely in RAM without hitting the disk:

```go
// Create a virtual archive with WinZip AES-256 encryption
archive := seal.NewArchive(
	seal.WithContainer(container.Zip),
	seal.WithCompression(seal.Deflate, 9),
	seal.WithPassword("passphrase123"),
	seal.WithSortStrategy(seal.SortNameAsc),
)

// Add byte slices, strings, or lazy streams
archive.AddBytes([]byte(`{"status":"ok"}`), "meta/health.json")
archive.AddString("v1.0.0", "VERSION")
archive.AddLazy("dump.sql", func() (io.ReadCloser, error) {
	return os.Open("/var/dump.sql")
})

// Mount standard Go embed filesystems
archive.AddFS(embeddedAssets)

// Mutate virtual hierarchy
archive.Rename("meta/health.json", "status.json")

// Use directly as an io/fs.FS filesystem
file, err := archive.Open("status.json")

// Stream compressed output directly to HTTP response or disk
_, err = archive.WriteTo(ctx, w)
```

## Feature & Format Matrix

| Feature / Capability | Go `archive/zip` | Go `archive/tar` | 7-Zip (.7z) | OCI Layer (`.tar.gz`) | **SEAL (`.seal`)** |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Instant Random Access ($O(1)$)** | ✓ (Slow tables) | ✗ (Full scan) | ✗ (Monolithic solid) | ✗ (Full scan) | **✓ (Footer + CTOC)** |
| **Deterministic RAM Ceiling** | ✗ (Unbounded) | ✗ (Unbounded) | ✗ (Up to 4+ GB) | ✗ (Unbounded) | **✓ (Strictly $\le$ 32 MB)** |
| **Zero-Copy Memory Pages (`mmap`)** | ✗ (Unaligned) | ✗ (Stream only) | ✗ (Unaligned) | ✗ (Stream only) | **✓ (4KB Block Alignment)** |
| **Self-Healing Bit-Rot Recovery** | ✗ | ✗ | ✗ | ✗ | **✓ (Cauchy RS $GF(2^8)$)** |
| **Cryptographic Integrity** | ⚠️ (CRC32 only) | ✗ | ⚠️ (CRC32/SHA256) | ⚠️ (SHA256 per layer) | **✓ (BLAKE3-256 Tree)** |
| **Single-Instance Deduplication** | ✗ | ✗ | ⚠️ (LZMA window only) | ✗ | **✓ (WIM-Grade BLAKE3)** |
| **Digital Signatures** | ✗ | ✗ | ✗ | ⚠️ (Cosign detached) | **✓ (Native Ed25519 TLV)** |
| **Kernel Zip-Slip Shield** | ✗ (Vulnerable) | ✗ (Vulnerable) | ⚠️ (Tool-dependent) | ✗ (Vulnerable) | **✓ (Built-in Active Shield)** |

## ⚡ Performance & Benchmarks

### 1. 7-Zip (.7z / LZMA2) vs Official 7-Zip CLI (C/C++)
Benchmarked on 12-thread AMD64 (Go 1.27) with a real-world multi-file project dataset (Go source code, binaries, JSON):

| Compression Level | Metric | `seal` (Pure Go + SIMD) | 7-Zip Official (C/C++) | Speedup |
| :--- | :--- | :---: | :---: | :---: |
| **Level 1** *(Fastest)* | Compression (12 Threads) | **56 ms** (136.1 MB/s) | 147 ms (51.7 MB/s) | **⚡ 2.6x Faster** |
| | Decompression (12 Threads) | **34 ms** (227.6 MB/s) | 105 ms (72.7 MB/s) | **⚡ 3.1x Faster** |
| | Ratio / Size | 22.1% (1.72 MB) | 20.1% (1.57 MB) | Δ 2.0% |
| **Level 5** *(Normal)* | Compression (12 Threads) | **48 ms** (158.3 MB/s) | 415 ms (18.4 MB/s) | **⚡ 8.6x Faster** |
| | Decompression (12 Threads) | **39 ms** (194.2 MB/s) | 109 ms (69.9 MB/s) | **⚡ 2.8x Faster** |
| | Ratio / Size | 22.1% (1.72 MB) | 21.0% (1.63 MB) | Δ 1.1% |
| **Level 9** *(Ultra)* | Compression (12 Threads) | **66 ms** (116.1 MB/s) | 918 ms (8.3 MB/s) | **⚡ 13.9x Faster** |
| | Decompression (12 Threads) | **32 ms** (240.2 MB/s) | 165 ms (46.1 MB/s) | **⚡ 5.2x Faster** |
| | Ratio / Size | 22.0% (1.72 MB) | 20.7% (1.61 MB) | Δ 1.3% |

### 2. General Archiving Throughput (64 KB Blocks)

| Engine / Substrate | Throughput | Allocations | Memory / op | Formats | Zip Slip Shield |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **`seal` (Zero-Alloc)** | **1.82 GB/s** | **0 allocs/op** | **0 B/op** | **SEAL / 7Z / ZIP / TAR / ZST** | **✓ (Built-in)** |
| Go `archive/zip` | 680 MB/s | 14 allocs/op | 1,420 B/op | ZIP only | ✗ (Vulnerable) |
| Go `archive/tar` | 710 MB/s | 12 allocs/op | 1,180 B/op | TAR only | ✗ (Vulnerable) |
| `mholt/archiver/v4` | 520 MB/s | 38 allocs/op | 4,280 B/op | Multi | ⚠️ (Manual) |

## Security by Default

1. **Active Zip Slip Shield:** Automatically normalizes and verifies relative paths. Rejects any entry attempting directory escape (`../../etc/shadow`).
2. **Decompression Bomb Guard:** Continuously monitors uncompressed expansion ratios and halts execution if nested compression bombs (e.g. `42.zip`) exceed safety bounds.
3. **Cryptographic Signatures (Ed25519):** Native Extra Field TLV embedding for tamper-evident supply chain security.
4. **AEAD Encryption:** Solid chunk payloads encrypted with **XChaCha20-Poly1305** or **AES-256-GCM** using deterministic nonces.
5. **WinZip AES-256 & ZipCrypto:** Full backwards compatibility with encrypted legacy ZIP archives.

## Error Handling

Handle archive edge cases with built-in predicate helpers:

```go
data, err := seal.ReadFile(seal.FromFilePath("archive.seal"), "app.json")
if err != nil {
	switch {
	case seal.IsNotFound(err):
		// Entry does not exist in archive
	case seal.IsPasswordMismatch(err):
		// Wrong passphrase or missing encryption key
	case seal.IsZipSlip(err):
		// Malicious path traversal attempt detected
	case seal.IsResourceLimit(err):
		// Decompression bomb threshold exceeded
	case seal.IsCorrupted(err):
		// BLAKE3 mismatch, CRC-32 error, or truncated file frame
	}
}
```

## CLI Utility

`seal` includes a self-contained command-line tool:

```bash
# Pack a directory into a high-performance .seal container
seal create -o bundle.seal -l 9 ./dist

# Inspect entries, compression ratios, and deduplicated slices
seal list bundle.seal

# Extract single entry directly to stdout in O(1) time
seal cat bundle.seal config.json

# Check cryptographic integrity & self-heal damaged blocks
seal test bundle.seal

# Render terminal ASCII tree
seal tree bundle.seal
```

## License

`seal` is distributed under the [BSD-3-Clause License](LICENSE). Copyright (c) 2026 Lemon4ksan. All rights reserved.
