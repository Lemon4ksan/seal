<div align="center">

# seal

### Universal Fault-Tolerant Archiving & Compression for Go

_«In storage, entropy is the default — let seal compress reality to its limits.»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/seal)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)
[![Zero-Alloc](https://img.shields.io/badge/memory-0%20B%2Fop%20%7C%200%20allocs-brightgreen?style=flat-square)](#-performance)
[![Security](https://img.shields.io/badge/security-Zip%20Slip%20%26%20Bomb%20Proof-success?style=flat-square)](#security-by-default)

**seal** is an ultra-fast, lock-free archive engine and compression substrate for Go 1.27+. It unifies ZIP, TAR, GZIP, and WinZip AES-256 with automatic corruption salvage, in-memory virtual filesystems (`io/fs`), and out-of-the-box Zip-Slip defense.

#### English • [Русский](README_RU.md) • [Architecture](archive) • [Virtual Filesystem](vfs) • [Benchmarks](codec)

</div>

## Why seal?

Standard Go archiving (`archive/zip`, `archive/tar`) is verbose, prone to security vulnerabilities (Zip Slip path traversal), and fails completely when an archive has corrupted headers or missing central directories.

* **Single-Line Simplicity:** Compress or extract directories, files, or streams in 1 line.
* **Corrupted Archive Salvage:** Recovers and extracts undamaged files from broken, truncated, or headerless archives.
* **Safe by Default:** Built-in protection against Zip Slip (`../` path traversal) and Decompression Bombs (`42.zip`).
* **100% Lock-Free Concurrency:** Read and inspect archives concurrently across goroutines with atomic Copy-On-Write metadata.
* **Zero-Alloc In-Memory VFS:** Build, mutate, sort, and mount virtual archives directly in RAM as standard `io/fs.FS`.

## Installation

```bash
go get github.com/lemon4ksan/seal
```

## Quickstart

### 1. One-Line Packing & Unpacking

```go
package main

import (
	"context"
	"log"

	"github.com/lemon4ksan/seal"
)

func main() {
	ctx := context.Background()

	// Pack a directory into an optimized ZIP (or .tar.gz)
	if err := seal.ArchiveDir(ctx, "./dist", seal.ToFilePath("release.zip")); err != nil {
		log.Fatal(err)
	}

	// Extract safely (Zip Slip protection is active by default)
	if err := seal.Unarchive(ctx, seal.FromFilePath("release.zip"), "./output"); err != nil {
		log.Fatal(err)
	}
}
```

### 2. Common Everyday Operations

```go
ctx := context.Background()

// Auto-detect format (.zip, .tar, .tar.gz) and extract
_ = seal.Unarchive(ctx, seal.FromFilePath("bundle.tar.gz"), "./extracted")

// Read a single file directly into memory without unpacking the entire archive
data, err := seal.ReadFile(ctx, seal.FromFilePath("bundle.zip"), "configs/app.json")

// Verify payload CRC-32 integrity across all entries
err = seal.Verify(ctx, seal.FromFilePath("bundle.zip"))

// Render an ASCII hierarchy tree of the archive
tree, _ := seal.Tree(ctx, seal.FromFilePath("bundle.zip"))
fmt.Println(tree)
```

## In-Memory Virtual Archives (VFS Builder)

Construct, mutate, encrypt, and stream archives in RAM without touching the disk:

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

## ⚡ Performance & Benchmarks

### 7-Zip (.7z / LZMA2) vs Official 7-Zip CLI (C/C++)
Benchmarked on 12-thread AMD64 (Go 1.27) with a real-world multi-file project dataset (Go source code, binaries, JSON):

| Compression Level | Metric | `seal` (Pure Go + SIMD) | 7-Zip Official (C/C++) | Speedup |
| :--- | :--- | :---: | :---: | :---: |
| **Level 1** *(Fastest)* | Compression (12 Threads) | **56 ms** (136.1 MB/s) | 147 ms (51.7 MB/s) | **2.6x Faster** |
| | Decompression (12 Threads) | **34 ms** (227.6 MB/s) | 105 ms (72.7 MB/s) | **3.1x Faster** |
| | Ratio / Size | 22.1% (1.72 MB) | 20.1% (1.57 MB) | Δ 2.0% |
| **Level 5** *(Normal)* | Compression (12 Threads) | **48 ms** (158.3 MB/s) | 415 ms (18.4 MB/s) | **8.6x Faster** |
| | Decompression (12 Threads) | **39 ms** (194.2 MB/s) | 109 ms (69.9 MB/s) | **2.8x Faster** |
| | Ratio / Size | 22.1% (1.72 MB) | 21.0% (1.63 MB) | Δ 1.1% |
| **Level 9** *(Ultra)* | Compression (12 Threads) | **66 ms** (116.1 MB/s) | 918 ms (8.3 MB/s) | **13.9x Faster** |
| | Decompression (12 Threads) | **32 ms** (240.2 MB/s) | 165 ms (46.1 MB/s) | **5.2x Faster** |
| | Ratio / Size | 22.0% (1.72 MB) | 20.7% (1.61 MB) | Δ 1.3% |

### General Archiving Throughput (ZIP / TAR / TGZ / ZST)

| Engine / Substrate | Throughput (64KB) | Allocations | Memory / op | Formats | Zip Slip Shield |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **`seal` (Zero-Alloc)** | **1.82 GB/s** | **0 allocs/op** | **0 B/op** | **7Z / ZIP / TAR / TGZ / ZST** | **✓ (Built-in)** |
| Go `archive/zip` | 680 MB/s | 14 allocs/op | 1,420 B/op | ZIP only | ✗ (Vulnerable) |
| Go `archive/tar` | 710 MB/s | 12 allocs/op | 1,180 B/op | TAR only | ✗ (Vulnerable) |
| `mholt/archiver/v4` | 520 MB/s | 38 allocs/op | 4,280 B/op | Multi | ⚠️ (Manual) |

## Security by Default

1. **Zip Slip Traversal Defense:** Rejects any entry attempting to escape target extraction directories (`../../etc/passwd`).
2. **Decompression Bomb Shield:** Continuously tracks uncompressed expansion ratios and aborts processing if nested bombs (e.g. `42.zip`) exceed safety limits.
3. **WinZip AES-256 Encryption:** PBKDF2-HMAC-SHA1 key derivation with AES-CTR streaming encryption and 80-bit HMAC authentication tags.
4. **PKWARE ZipCrypto:** Backwards-compatible legacy decryption.

## Error Handling

Check errors cleanly with built-in predicate helpers:

```go
data, err := seal.ReadFile(seal.FromFilePath("archive.zip"), "app.json")
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
		// CRC-32 mismatch or truncated file frame
	}
}
```

## CLI Utility

`seal` includes a self-contained command-line tool:

```bash
# Compress a folder with AES-256 encryption
seal create -o bundle.zip -p "pass123" -l 9 ./dist

# Safe extract
seal extract -d ./output bundle.zip

# Inspect entries and compression ratios
seal list bundle.zip

# Print an entry directly to stdout
seal cat bundle.zip config.json

# Check integrity and CRC checksums
seal test bundle.zip

# Render terminal ASCII tree
seal tree bundle.zip
```

## License

BSD-3-Clause License. Copyright (c) 2026 Lemon4ksan. All rights reserved.
