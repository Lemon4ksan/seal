# Project: PRZM Mathematical Optimization Pipeline (Acceleration & Ratio Enhancement)

## Architecture
- **Target Codebase**: `d:\CodingProjects\przm`
- **Component 1 (Go Compression Engine)**:
  - `compress.go`: Primary compression entry point, mode selection, heuristic lookahead replacement.
  - `sheaf_trellis.go`: Étale Sheaf & Tropical Trellis stream encoder (`encodeTrellisStream`).
  - `ggtc.go` & `pws.go`: Hierarchical DAG grammar induction with flat unrolling tables (`ExpL1`..`ExpL5`).
  - `isoquant.go`: 4D IsoQuant Quaternionic Collimation module over $\mathbb{H}(\mathbb{F}_{2^8})$ for multi-channel/shader streams.
- **Component 2 (Pure C Reference Decompressor)**:
  - `include/przm_dec.h`: Public API, error codes, frame magic `PRZM_MAGIC_ISOQUANT` (`0x514F5349`).
  - `src/przm_dec.c`: Zero-allocation decompressor, in-place branchless inverse quaternionic rotation `isoquant_unrotate_inplace`.
  - `tests/test_przm_dec.c`: Pure C unit verification harness.
- **Component 3 (Continuous Benchmarking & Verification)**:
  - `c_interop_test.go`: Go-to-C roundtrip interoperability tests.
  - Benchmark suite: `go test -bench=. -benchmem` measuring throughput (MB/s), compression ratio, and heap allocations (`0 B/op, 0 allocs/op`).

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| F1 | Knuth-Bendix Term Rewriting Automata | Confluent string term rewriting for grammar induction in `ggtc.go`/`pws.go`, resolving critical pairs under depth $D \le 5$ ordering, replacing redundant greedy digram lookups. | M1 | Survey (`MATHEMATICAL_REPORT.md` §6.2) |
| F2 | Tropical Associative Prefix Scans | $(\min, +)$ semiring associative prefix scan formulation in `sheaf_trellis.go` (`encodeTrellisStream`), evaluating trellis transitions via SIMD/parallel associative reduction, collapsing critical path latency. | M1 | Survey (`MATHEMATICAL_REPORT.md` §6.1) |
| F3 | 4D IsoQuant Quaternionic Collimation (Go) | Hamilton quaternion algebra over $\mathbb{H}(\mathbb{F}_{2^8}) \cong \mathbb{F}_{2^8}^4$, covariance diagonalization via isoclinic pair $(q_L, q_R)$, entropy reduction on multi-channel/shader/binary streams. | M2 | Survey (`MATHEMATICAL_REPORT.md` §6.4) |
| F4 | Branchless Vector Decode in C (`przm_dec.c`) | In-place inverse isoclinic rotation $\mathbf{v} = \bar{q}_L \cdot \mathbf{v}' \cdot q_R$ in `src/przm_dec.c` and `include/przm_dec.h` under magic `PRZM_MAGIC_ISOQUANT` (`0x514F5349`), 22 CPU cycles/64B, 0 heap allocations. | M2 | Survey (`MATHEMATICAL_REPORT.md` §7.4 Proof 4) |
| F5 | Instantaneous O(1) Decompression & Zero Allocations | Strict preservation of $\le 384$ CPU clock cycles per 64-byte block, $18.86\text{ KiB} < 32\text{ KiB}$ L1D cache footprint, $0\text{ B/op, } 0\text{ allocs/op}$ in C decompressor and core Go decompression. | M1, M2, M3 | Survey (`MATHEMATICAL_REPORT.md` §7.1, §7.5) |
| F6 | Continuous Benchmarking & Comparative Verification | Automated verification of Go unit tests, C unit tests, interop tests, and Before/After comparative benchmark tables detailing throughput, compression ratio, and allocation counts. | M3 | Dispatch Requirement R3 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | Acceleration Pipeline Implementation | Implement Knuth-Bendix deterministic rewrite automata and Tropical associative prefix scans in Go (`ggtc.go`, `pws.go`, `sheaf_trellis.go`, `compress.go`), minimizing critical path latency while preserving instant O(1) decoding. | None | DONE |
| M2 | Ratio Enhancement Pipeline Implementation | Implement 4D IsoQuant quaternionic collimation in Go (`isoquant.go`, `compress.go`) and corresponding branchless vector decode in C (`src/przm_dec.c`, `include/przm_dec.h`, `tests/test_przm_dec.c`). | M1 | DONE |
| M3 | Continuous Benchmarking & Invariant Verification | Comprehensive benchmarking across all corpora, verification of 0 heap allocations, bit-exact roundtrip losslessness, and generation of comparative Before/After performance tables. | M1, M2 | DONE |

## Interface Contracts
### Knuth-Bendix Rewriting Engine ↔ Grammar Induction (`ggtc.go`, `pws.go`)
- Function: `knuthBendixConfluentReduce(rules []Rule, maxDepth int) []Rule`
- Input: Set of induced grammar production pairs $(L \to R)$.
- Output: Confluent, minimal canonical rule set with critical pairs resolved; pre-expanded flat lookup tables for decompressor.
- Complexity: $O(N + |\mathcal{R}| \log |\mathcal{R}|)$ induction time; decompressor expansion strictly $O(1)$ flat vector stores (`ExpL1`..`ExpL5`).

### Tropical Associative Prefix Scan ↔ Trellis Encoder (`sheaf_trellis.go`)
- Operator: $\oplus = \min$, $\otimes = +$ over weights $\mathbb{Z}_{\ge 0} \cup \{\infty\}$.
- Function: Vectorized/associative reduction for state transitions in `encodeTrellisStream`.
- Guarantee: Identical optimal path selection with zero decompressor changes ($O(1)$ instantaneous token decode preserved).

### Go IsoQuant Encoder ↔ C IsoQuant Decompressor
- Frame Header: 20 bytes aligned
  - Bytes 0..3: Magic `PRZM_MAGIC_ISOQUANT` (`0x514F5349`)
  - Byte 4: Mode / Flags (`uint8`)
  - Bytes 5..7: Reserved (`uint8[3]`)
  - Bytes 8..11: Uncompressed length `orig_len` (`uint32_le`)
  - Bytes 12..15: Left quaternion inverse $q_{L,\text{inv}} \in \mathbb{F}_{2^8}^4$ (`uint8[4]`)
  - Bytes 16..19: Right quaternion inverse $q_{R,\text{inv}} \in \mathbb{F}_{2^8}^4$ (`uint8[4]`)
- Payload: Collimated stream compressed via PRZM standard LZ/ANS encoding.
- Decompression API: `przm_decompress(src, src_len, dst, dst_len)` decodes payload, then applies in-place `isoquant_unrotate_inplace(dst, orig_len, qL_inv, qR_inv)`.
- Invariant: Zero allocations (`0 B/op, 0 allocs/op`), in-place execution in $\le 22$ CPU cycles per 64 bytes.

## Code Layout
- Existing files in `d:\CodingProjects\przm`:
  - `compress.go`: Core compression entry and pipeline dispatch.
  - `decompress.go`: Core Go decompressor.
  - `sheaf_trellis.go`: Sheaf trellis encoder/decoder.
  - `ggtc.go`: 3-layer H-DAG grammar induction.
  - `pws.go`: 5-layer H-DAG grammar induction.
  - `isoquant.go`: 4D IsoQuant Quaternionic Collimation (to be added/integrated).
  - `src/przm_dec.c`: Pure C decompressor.
  - `include/przm_dec.h`: C decompressor headers.
  - `tests/test_przm_dec.c`: Pure C test harness.
  - `c_interop_test.go`: Go-C interop tests.

## Architectural Directive: Language-Neutrality & Dictionary Decoupling
1. **General-Purpose Mathematical Core**: All mathematical optimization modules (Knuth-Bendix term rewriting, Tropical associative prefix scans, and 4D IsoQuant quaternionic collimation) MUST remain strictly general-purpose, operating purely on mathematical abstractions (semirings, oriented terms, Hamilton quaternions) and completely decoupled from hardcoded language-specific token dictionaries.
2. **Dictionary Evolution Strategy**:
   - Hardcoding specific language syntax (e.g., Go `if err != nil`) in the core engine is identified as technical debt and shall not be expanded.
   - Low DictIDs (DictID = 1) remain reserved strictly for universal, language-agnostic standards (e.g., JSON grammar rules).
   - Domain-specific/language-specific vocabularies are planned to be routed through an external/custom dictionary training interface (similar to `zstd --train` or container-level dictionary substrates with DictID >= 0x1000).
   - No new hardcoded programming language tokens shall be added to the bitstream format or decompressor.
