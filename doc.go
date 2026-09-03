// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package seal provides a unified, high-performance universal archive and compression engine for Go.
//
// It consolidates PKWARE ZIP, POSIX TAR, GZIP TAR (.tar.gz), WinZip AES-256 encryption, and multi-algorithm
// compression codecs into a single, profile-driven, 100% lock-free zero-friction architecture.
//
// # Dual Engines under a Unified Architecture
//
// Seal provides two execution layers under a single conceptual framework:
//   - [Archiver] / 1-Line Solvers - Task-oriented filesystem orchestrators ([ArchiveDir], [Unarchive], [ReadFile], [Verify]).
//   - [Archive] ([github.com/lemon4ksan/seal/archive]) - In-memory mutable archive builder and [io/fs.FS] virtual filesystem.
//
// # The Three Tiers of Usage (Zero-Friction DX)
//
// Seal is designed for maximum simplicity for common tasks while supporting extreme control for high-load systems:
//
//  1. Tier 1: Single-Line Zero Config (Quick Tasks & Scripts):
//     Compress, extract, read, and verify archives with zero boilerplate or manual setup:
//
//     err := seal.ArchiveDir(ctx, "./dist", seal.ToFilePath("bundle.zip"))
//     err = seal.Unarchive(ctx, seal.FromFilePath("bundle.zip"), "./out")
//     data, err := seal.ReadFile(ctx, seal.FromFilePath("bundle.zip"), "app.json")
//     err = seal.Verify(ctx, seal.FromFilePath("bundle.zip"))
//
//  2. Tier 2: Fluent In-Memory Programmatic Builder (Microservices & APIs):
//     Construct dynamic archive bundles in memory with lazy streams, collision handling, and VFS export:
//
//     a := seal.NewArchive(seal.WithContainer(container.Zip), seal.WithPassword("secret"))
//     a.AddBytes([]byte("..."), "data.json")
//     a.AddLazy("stream.log", logReaderFactory)
//     a.Rename("data.json", "docs/data.json")
//     _, err := a.WriteTo(ctx, destWriter)
//
//  3. Tier 3: Raw Container Streaming Engines (Ultra High Performance):
//     Directly control wire-level serialization and decompression framing via [github.com/lemon4ksan/seal/container]:
//
//     engine, _ := seal.DefaultContainer(container.Zip)
//     files, comment, err := engine.ReadIndex(r, rAt, size, password, reg)
//
// # Domain Architecture & Subpackages
//
//   - [github.com/lemon4ksan/seal/archive] - In-memory mutable archive builder, conflict resolver, and VFS provider.
//   - [github.com/lemon4ksan/seal/container] - Container abstraction, dynamic sniffer, and format registry.
//   - [github.com/lemon4ksan/seal/container/zip] - High-throughput ZIP container with Central Directory & WinZip AES-256.
//   - [github.com/lemon4ksan/seal/container/tar] - POSIX / GNU / USTAR tape archive streaming engine.
//   - [github.com/lemon4ksan/seal/container/targz] - GZIP-compressed TAR container streaming engine.
//   - [github.com/lemon4ksan/seal/file] - Canonical 100% lock-free file entry entity backed by hardware atomics.
//   - [github.com/lemon4ksan/seal/codec] - Universal codec registry (Deflate, Zstandard, Brotli, Gzip, Store).
//   - [github.com/lemon4ksan/seal/vfs] - Path traversal guards (Zip Slip defense), ASCII tree renderer, and [fs.FS] adapter.
//   - [github.com/lemon4ksan/seal/option] - Stack-allocated functional configuration options.
//
// # Concurrency & Thread-Safety Invariants
//
// The core file entity ([file.File]) is 100% lock-free, utilizing hardware atomic pointers ([sync/atomic.Pointer])
// with Copy-On-Write (COW) semantics for metadata updates. Multiple goroutines can concurrently read,
// decompress, inspect metadata, and serialize entries without mutex contention or data races.
//
// # Error Inspection Suite
//
// Error handling is fully integrated with standard Go [errors.Is] and single-line predicate helpers:
//
//	data, err := seal.ReadFile(ctx, seal.FromFilePath("bundle.zip"), "config.json")
//	if seal.IsNotFound(err) {
//	    // Entry does not exist in archive
//	}
//	if seal.IsPasswordMismatch(err) {
//	    // Incorrect or missing decryption password
//	}
//	if seal.IsZipSlip(err) {
//	    // Path traversal attack detected
//	}
//	if seal.IsCorrupted(err) {
//	    // CRC-32 checksum mismatch or header corruption
//	}
//
// # Basic Usage
//
//	package main
//
//	import (
//		"context"
//		"log"
//
//		"github.com/lemon4ksan/seal"
//	)
//
//	func main() {
//		ctx := context.Background()
//
//		// Pack a directory into a zip archive
//		if err := seal.ArchiveDir(ctx, "./dist", seal.ToFilePath("bundle.zip")); err != nil {
//			log.Fatalf("pack failed: %v", err)
//		}
//
//		// Extract archive with safety guarantees
//		if err := seal.Unarchive(ctx, seal.FromFilePath("bundle.zip"), "./extracted"); err != nil {
//			log.Fatalf("unpack failed: %v", err)
//		}
//	}
package seal
