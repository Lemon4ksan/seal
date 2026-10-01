# Project: PRZM Mathematical Models Extension (orchestrator_5)

## Architecture
PRZM (PrismLZ) is a high-throughput manifold compression engine treating digital data as discrete fiber bundles and cell complexes over Galois fields $\mathbb{F}_{2^8}$, residue rings $\mathbb{Z}/2^k\mathbb{Z}$, and binary vector spaces. This milestone implements three advanced mathematical models:
1. **Cellular Sheaves with Discrete Hodge Laplacians (CS-DHL)**: Detects multi-dimensional connectivity and homology voids in structured and graph-like data streams. Projects out harmonic 1-forms ($\ker \Delta_1$) and collapses 2D fields into compact boundary representations with $\le 10$ CPU cycles/block row-sweep decode.
2. **p-Adic Ultrametric Match Search via Hensel's Lifting**: Employs non-Archimedean $p$-adic valuation and quadratic Hensel lifting to prune 85–93% of candidates in 1 cycle, expanding search depth up to 16 (Level 3) and 64 (Level 4) while strictly preserving external bitstream format and $O(1)$ zero-allocation decompression.
3. **Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ Foliation**: Extends the manifold atlas with nilpotent Lie group projections $H(x, y, z)$ over $\mathbb{F}_{2^8}$ to factor non-commutative spatial correlations in multi-dimensional/tensor data, with branchless, zero-allocation algebraic inversion ($H^{-1}(x,y,z) = H(x, y, z \oplus (x \odot y))$) in both Go and Pure C (`przm_dec.c`).

All modules preserve the strict zero-allocation $O(1)$ decompression contract (`0 B/op, 0 allocs/op`), compile cleanly in Pure C with `-O3 -Wall -Wextra -Werror`, build Lean 4 formal invariants with zero errors/warnings/sorry, and maintain language neutrality (no hardcoded language token dictionaries).

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | p-Adic Hensel Match Finding | Quadratic Hensel lifting and LCP evaluation via TrailingZeros64 in Go encoder | M1 | DISPATCH R2 |
| 2 | Ultrametric Match Pruning | In-cache 32-bit fingerprint filtering expanding search depth to 16/64 | M1 | DISPATCH R2 |
| 3 | Bitstream Invariance | Zero modifications to external PRZM-SPEC-0.5 framing / decompressor | M1 | DISPATCH R2 |
| 4 | Heisenberg Group Go Module | $H_3(\mathbb{F}_{2^8})$ forward foliation, covariance gating, and Pareto arbitration | M2 | DISPATCH R3 |
| 5 | Heisenberg Pure C Decompressor | Branchless, zero-allocation algebraic inversion in `przm_dec.c` with 12/16B SWAR | M2 | DISPATCH R3 |
| 6 | Heisenberg C Interop Tests | Lossless roundtrip tests across Go and Pure C reference decompressor | M2 | DISPATCH R3 |
| 7 | Cellular Sheaf Stalk & Complex | 2D cell complex over $\mathbb{F}_{2^8}$ with restriction maps and coboundaries $d_0, d_1$ | M3 | DISPATCH R1 |
| 8 | Discrete Hodge Laplacian & Homology | $\Delta_0, \Delta_1$ computation, harmonic 1-form cycle annihilation | M3 | DISPATCH R1 |
| 9 | Boundary Representation & Vector Decode | Compact boundary encoding and row-sweep vector decode in Go & Pure C | M3 | DISPATCH R1 |
| 10 | Lean 4 Formal Verification | Formal proofs for Heisenberg involution and Cellular Sheaf coboundary identity | M4 | DISPATCH R4 |
| 11 | Continuous Benchmarking Suite | Before/After comparative benchmark tables measuring throughput & ratio | M4 | DISPATCH R4 |
| 12 | Zero-Allocation Verification | Formal test verification of `0 B/op, 0 allocs/op` on all decompressor paths | M4 | DISPATCH R4 |
| 13 | Language-Neutrality Compliance | Verification of zero hardcoded language/syntax token dictionaries | M1-M4 | User Directive |
| 14 | Research Tracker Update | Hypotheses H-24, H-25, H-26 added to `tracker.md` with verified metrics | M4 | DISPATCH R4 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| 1 | p-Adic Ultrametric Match Finding via Hensel's Lifting | `hensel.go`, `hensel_test.go`, integration into `compress.go` Level 3/4 | none | DONE |
| 2 | Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ Foliation | `heisenberg.go`, `heisenberg_test.go`, `compress.go`, `decompress.go`, `przm_dec.h`, `przm_dec.c`, `test_przm_dec.c`, `c_interop_test.go` | none | DONE |
| 3 | Cellular Sheaves with Discrete Hodge Laplacians | `cellular_sheaf.go`, `cellular_sheaf_test.go`, `compress.go`, `decompress.go`, `przm_dec.h`, `przm_dec.c`, `test_przm_dec.c` | none | IN_PROGRESS |
| 4 | Formal Lean 4 Verification & Continuous Benchmarking | `PRZM/Heisenberg.lean`, `PRZM/CellularSheaf.lean`, `PRZM.lean`, `przm_theorems.lean`, comparative benchmarks, `tracker.md` | M1, M2, M3 | PLANNED |
| 5 | Victory Audit & Final Sign-Off | Full independent forensic audit across all acceptance criteria | M4 | PLANNED |

## Interface Contracts
### Hensel Match Finder $\leftrightarrow$ PRZM Compressor
- `NewHenselMatchFinder(windowSize int) *HenselMatchFinder`
- `FindMatchHensel(src []byte, pos int, maxDepth int, minLen int) (bestOffset int, bestLen int)`
- Operates inside encoder scratchpad; produces standard PRZM match offsets and lengths.

### Heisenberg Foliation $\leftrightarrow$ PRZM Codec & Pure C
- Frame Magic: `PRZM_MAGIC_HEISENBERG = 0x53494548` ("HEIS", Little-Endian).
- Header (12 bytes): `[0:4]` Magic, `[4:8]` UncompressedSize (uint32), `[8]` Stride (uint8: 3 or 4), `[9]` Flags (uint8), `[10:12]` Reserved (uint16 = 0).
- Transformation: $z' = z \oplus (x \odot y)$ over $\mathbb{F}_{2^8}$ (modulo AES polynomial `0x11B`).
- Inversion: $z = z' \oplus (x \odot y)$ (exact algebraic involution).
- Pure C Signature:
  `int przm_heisenberg_unfoliate_inplace(uint8_t *dst, size_t len, uint8_t stride)`
- Decompressor Dispatch: `przm_decompress` in `src/przm_dec.c` and `Decompress` in `decompress.go`.

### Cellular Sheaves $\leftrightarrow$ PRZM Codec & Pure C
- Frame Magic: `PRZM_MAGIC_CELLULAR_SHEAF = 0x44485343` ("CSHD", Little-Endian).
- Header (24 bytes): Magic (4B), UncompressedSize (4B), Width W (2B), Height M (2B), Anchor (1B), GaloisTwist (1B), Betti1 (2B), Reserved (8B).
- Decompressor row sweep: $\phi(i, j) = \phi(i-1, j) \oplus V(i, j)$.
- Pure C Signature:
  `int przm_cellular_sheaf_decompress(const uint8_t *src, size_t src_len, uint8_t *dst, size_t dst_cap)`

## Code Layout
- Go Codebase Root: `d:\CodingProjects\przm`
  - Match Finding: `hensel.go`, `hensel_test.go`
  - Heisenberg Foliation: `heisenberg.go`, `heisenberg_test.go`
  - Cellular Sheaves: `cellular_sheaf.go`, `cellular_sheaf_test.go`
  - Core Codec: `compress.go`, `decompress.go`, `frame.go`, `c_interop_test.go`
- Pure C Reference Decompressor:
  - Header: `d:\CodingProjects\przm\include\przm_dec.h`
  - Source: `d:\CodingProjects\przm\src\przm_dec.c`
  - Test Suite: `d:\CodingProjects\przm\tests\test_przm_dec.c`
- Lean 4 Formal Proof Library:
  - `d:\CodingProjects\przm\PRZM\Heisenberg.lean`
  - `d:\CodingProjects\przm\PRZM\CellularSheaf.lean`
  - `d:\CodingProjects\przm\PRZM.lean`
  - `d:\CodingProjects\przm\przm_theorems.lean`
- Benchmarks & Tracking:
  - `d:\CodingProjects\przm\tracker.md`
