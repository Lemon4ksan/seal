# GEMINI.md — Development Guidelines & AI Assistant Protocol

This document outlines the architecture, coding standards, build/test commands, and interaction rules for AI assistants and software engineers working in the **seal** repository.

## 1. Project Overview

**`seal`** is an ultra-fast, lock-free archive engine, compression substrate, and storage framework for Go 1.27+. It powers the next-generation **EON (`.eon`)** container format — featuring mathematical Cauchy Reed-Solomon self-healing, $O(1)$ LBA page seeking, sub-chunk micro-framing (down to $\le 64$ KB RAM), phase-based temporal tiering, and single-instance deduplication.

> **Core Engineering Manifesto**:
> _«Entropy destroys all data — let **eon** preserve reality for eternity.»_

> **The "seal v1" Compatibility & Forever-Frozen Core Manifesto**:
> _"Containers formatted with **seal v1.0.0** will remain decodable and bit-for-bit recoverable 50 years from now. The EON container specification, mathematical GF(2^8) Galois field arithmetic, and LBA page indexing are permanently frozen."_

### Key Capabilities & Architectural Pillars
- **Deterministic RAM Ceiling**: Operates within strict $\le 32$ MB RAM budgets, streaming sub-chunk micro-frames down to 64 KB workspaces.
- **Mathematical Self-Healing**: Hardware-vectorized Cauchy Reed-Solomon erasure coding ($GF(2^8)$) with transparent repair of corrupted sectors.
- **$O(1)$ LBA Page Seeking**: Instant random-access lookups without scanning preceding container chunks.
- **Enterprise Security by Default**: Native cryptographic validation, built-in Zip-Slip defenses, and path-traversal sanitization.

---

## 2. Repository Layout

```text
seal/
├── seal.go, doc.go                   // Core API entry points and package documentation
├── archive/                          // EON container framing, pack/unpack engines
├── reed_solomon/                     // SIMD-accelerated Galois Field GF(2^8) erasure coding
├── vfs/                              // Virtual file system and block-device page caching
├── dedup/                            // Single-instance content-addressable deduplication
└── .agents/                          // AI assistant protocol and developer guidelines
```

---

## 3. Core Architectural Principles & Technical Constraints

1. **Strict Deterministic RAM Budgets**:
   - Container reading and writing must honor fixed RAM boundaries. Never buffer entire container streams into memory.
2. **Mathematical Invariant Precision**:
   - Galois Field matrix inversion and Cauchy generator matrix properties must maintain exact bitwise accuracy across all architectures.
3. **Security Invariants**:
   - Archive extraction must sanitize file paths against Zip-Slip, directory traversal (`..`), and absolute path escapes.

---

## 4. Code Style & Quality Standards

- **Go Version**: 1.27+.
- **Documentation**: All public symbols must have clear Godoc documentation.
- **Linters**: Pass `golangci-lint run` with 0 warnings.

---

## 5. Development & Testing Commands

| Command | Purpose |
| :--- | :--- |
| `go test ./...` | Fast unit tests |
| `go test -race ./...` | Unit tests with race detector |
| `go test -bench=. -benchmem ./...` | Run throughput and erasure coding benchmarks |
| `make lint` | Run `golangci-lint` verification |
| `make format` | Format code and verify license headers |

---

## 6. Commit & Pull Request Guidelines

### Conventional Commits Format:
```text
<type>(<scope>): <short summary>

<detailed description of changes>
```

Commit Types:
- `feat`: New archive feature or container capability
- `fix`: Bug fix in framing or Reed-Solomon decoding
- `impr`: SIMD acceleration or allocation reduction
- `docs`: Documentation updates
- `test`: Unit, fuzz, or corrupted sector recovery tests
