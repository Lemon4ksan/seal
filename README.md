<img src="assets/logo.svg" align="right" width="128" height="128" alt="seal logo" />

# seal

_«Entropy destroys all data — let eon preserve reality for eternity»_

[![Go Version](https://img.shields.io/badge/go-1.27%2B-007d9c?logo=go&logoColor=white&style=flat-square)](https://go.dev/)
[![Go Reference](https://img.shields.io/badge/godoc-reference-007d9c?style=flat-square)](https://pkg.go.dev/github.com/lemon4ksan/seal)
[![Architecture](https://img.shields.io/badge/arch-EON%20%7C%20Reed--Solomon%20%7C%20LBA-0d9488?style=flat-square)](#rules)
[![Ecosystem](https://img.shields.io/badge/ecosystem-foundation%20%7C%20ctrl-0284c7?style=flat-square)](https://github.com/lemon4ksan/foundation)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue?style=flat-square)](LICENSE)

**[English](README.md)** | **[Русский](README_RU.md)** | **[EON Format Specification](docs/EON_SPEC.md)**

Fault-tolerant archive engine and container format implementation for Go 1.27+. Powers the **EON (`.eon`)** container format alongside `.7z`, `.zip`, `.tar`, and `.zst`, combining Cauchy Reed-Solomon erasure coding over $GF(2^8)$, $O(1)$ LBA page seeking, deterministic $\le 32\text{ MB}$ RAM ceilings, and BLAKE3-256 single-instance deduplication.

## Design tradeoffs

| `archive/zip` / `archive/tar` / monolithic `.7z` | What `seal` (`.eon`) does instead | What you pay |
| :--- | :--- | :--- |
| `.tar.gz` and solid `.7z` archives require linear scanning or decompressing gigabytes to read a single file | Stores a 32-byte Footer and Compressed Table of Contents (`CTOC`) with $2^{\text{BlockSizeShift}}$ (4 KB) LBA alignment for $<2\text{ ms}$ $O(1)$ random access and `mmap` | 4 KB LBA block alignment introduces small padding gaps between independent solid chunks |
| Large solid LZMA2 dictionaries can consume $1\text{–}4\text{ GB}$ of RAM during extraction, triggering `OOMKilled` in containers | Partitions payloads into independent solid chunks bounded at $\le 32\text{ MB}$ uncompressed (with sub-chunk micro-framing down to $\le 64\text{ KB}$) | Bounding solid chunks to 32 MB trades ~1–2% compression ratio on multi-gigabyte homogeneous inputs compared to unbounded monolithic dictionaries |
| Single flipped bit or bad sector corrupts the entire compressed stream | Computes Cauchy Reed-Solomon recovery blocks over $GF(2^8)$ and verifies extents via a BLAKE3-256 Merkle tree | Self-healing parity shards add configurable storage and CPU encoding overhead when enabled |
| Standard library extractors trust archive paths and uncompressed size headers | Enforces active Zip Slip path normalization and decompression bomb expansion guards by default | Non-canonical paths escaping the target root (`../`) are rejected unless explicitly remapped via VFS |

## Rules

1. **Deterministic memory ceiling.** Archive packing and extraction never exceed the configured chunk ceiling ($\le 32\text{ MB}$ RAM by default), preventing unbounded heap growth on untrusted archives.
2. **Zero-allocation steady-state streaming.** Block readers, checksum verifiers, and VFS iterators reuse pooled buffers (`0 B/op` on 64 KB streaming turns).
3. **Safe extraction by default.** Every extraction path validates relative paths against Zip Slip (`../`), symlink escapes, and decompression bomb amplification ratios before touching the host filesystem.
4. **Content-addressed deduplication.** Identical file payloads (matched by BLAKE3-256) reference existing chunk extents at zero additional storage or re-compression cost.

## Layout

- [`seal` (root)](.) — High-level `Compress`, `Extract`, `ReadFile`, generic `ReadTo[T]`, `Verify`, `Tree`, `NewArchive`, and typed error predicates (`IsNotFound`, `IsZipSlip`, `IsCorrupted`).
- [`archive`](archive) — Multi-format container codecs (`EON`, `7z` / `LZMA2`, `ZIP`, `TAR`), Cauchy Reed-Solomon $GF(2^8)$ recovery, BLAKE3-256 integrity trees, Ed25519 TLV signatures, and XChaCha20-Poly1305 / AES-256-GCM encryption.
- [`vfs`](vfs) — In-memory virtual filesystem implementing `io/fs.FS` for constructing, mutating, sorting, and streaming archives without intermediate disk files.
- [`cmd/seal`](cmd/seal) — Standalone CLI utility built on [`ctrl`](https://github.com/lemon4ksan/ctrl) (`create`, `extract`, `list`, `cat`, `test`, `tree`).
- [`docs/EON_SPEC.md`](docs/EON_SPEC.md) — Binary wire layout, LBA alignment, CTOC, and erasure coding specification for `.eon`.

## Installation

```bash
go get github.com/lemon4ksan/seal
```

## Usage

### Packing, Safe Extraction & $O(1)$ Random File Read

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/lemon4ksan/seal"
)

type AppConfig struct {
	Port int `json:"port"`
}

func main() {
	ctx := context.Background()

	// Pack directory (.eon, .zip, .tar.gz, .7z inferred from extension)
	if err := seal.Compress(ctx, "./dist", "release.eon"); err != nil {
		log.Fatal(err)
	}

	// Read and unmarshal a single file in O(1) time without full extraction
	cfg, err := seal.ReadTo[AppConfig](ctx, "release.eon", "configs/app.json")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Port: %d\n", cfg.Port)

	// Extract with active Zip Slip and decompression bomb protection
	if err := seal.Extract(ctx, "release.eon", "./output"); err != nil {
		log.Fatal(err)
	}
}
```

### In-Memory Virtual Archive (`vfs` / `io/fs.FS`)

```go
archive := seal.NewArchive(
	seal.WithContainer(container.Zip),
	seal.WithCompression(seal.Deflate, 9),
	seal.WithPassword("passphrase123"),
	seal.WithSortStrategy(seal.SortNameAsc),
)

archive.AddBytes([]byte(`{"status":"ok"}`), "meta/health.json")
archive.AddString("v1.0.0", "VERSION")
archive.Rename("meta/health.json", "status.json")

// Mount directly as a standard Go io/fs.FS or stream to an io.Writer
file, _ := archive.Open("status.json")
defer file.Close()
```

### CLI

```bash
# Pack a directory into an .eon container
seal create -o bundle.eon -l 9 ./dist

# Inspect entries, compression ratios, and deduplicated extents
seal list bundle.eon

# Extract a single entry to stdout in O(1) time
seal cat bundle.eon configs/app.json

# Verify BLAKE3-256 tree and self-heal damaged sectors via Reed-Solomon parity
seal test bundle.eon
```

## Benchmarks

Measured on 12-thread AMD64 (Go 1.27) across a multi-file source/binary dataset and 64 KB streaming blocks:

| Workload | Metric | `seal` | Reference Baseline |
| :--- | :--- | ---: | ---: |
| `.7z` / LZMA2 Level 1 (12T) | Compress / Decompress | `136.1 MB/s` / `227.6 MB/s` | `51.7 MB/s` / `72.7 MB/s` (7-Zip CLI) |
| `.7z` / LZMA2 Level 5 (12T) | Compress / Decompress | `158.3 MB/s` / `194.2 MB/s` | `18.4 MB/s` / `69.9 MB/s` (7-Zip CLI) |
| `.7z` / LZMA2 Level 9 (12T) | Compress / Decompress | `116.1 MB/s` / `240.2 MB/s` | `8.3 MB/s` / `46.1 MB/s` (7-Zip CLI) |
| 64 KB Streaming Pipeline | Throughput / Allocations | `1,820 MB/s` (`0 B/op`, `0 allocs`) | `680 MB/s` (`1,420 B/op`, `archive/zip`) |

## License

BSD 3-Clause License. See [LICENSE](LICENSE) for full text.
