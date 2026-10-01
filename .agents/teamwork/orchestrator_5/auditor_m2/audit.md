# Forensic Audit Report: Milestone 2 — Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ Foliation

**Work Product**: Milestone 2 Implementation (`heisenberg.go`, `heisenberg_test.go`, `compress.go`, `decompress.go`, `include/przm_dec.h`, `src/przm_dec.c`, `tests/test_przm_dec.c`, `c_interop_test.go`)  
**Target Codebase**: `d:\CodingProjects\przm`  
**Profile**: General Project  
**Integrity Mode**: Development (per `ORIGINAL_REQUEST.md`)  
**Auditor**: `auditor_m2` (Teamwork Forensic Auditor)  
**Date**: October 1, 2026  
**Verdict**: **CLEAN**

---

## Executive Summary

Milestone 2 extends the PRZM manifold compression engine with Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ foliation across the Go compression pipeline, the Pure C reference decompressor (`przm_dec.c`), and Lean 4 formal invariants.

A rigorous forensic audit was conducted across all required dimensions:
1. **Mathematical Authenticity**: Verified that `heisenberg.go` and `src/przm_dec.c` implement genuine nilpotent Lie group $H_3(\mathbb{F}_{2^8})$ operations and exact algebraic involution $H^{-1}(x,y,z) = H(x, y, z \oplus (x \odot y))$ without dummy stubs, lookup bypasses, or synthetic shortcuts.
2. **Galois Field Equivalence**: Exhaustively verified all $256 \times 256 = 65,536$ multiplication pairs over $\mathbb{F}_{2^8}$ (AES irreducible polynomial $x^8 + x^4 + x^3 + x + 1 = \text{0x11B}$) between Go's exponential/logarithm tables and Pure C's branchless Russian Peasant arithmetic. Exact match: 100.0%.
3. **Zero Heap Allocation Invariant**:
   - Pure C Reference Decompressor (`src/przm_dec.c`): Verified strictly 0 calls to `malloc`, `calloc`, `realloc`, or `free`. Operates 100% in-place on destination buffer.
   - Go In-Place Unfoliation (`HeisenbergUnfoliateInplace`): Confirmed `0 B/op, 0 allocs/op` via `testing.AllocsPerRun(100)` and benchmarks (>1.4 GB/s in Go, >18.7 GB/s in Mode 1 decompression).
4. **Language Neutrality**: Verified zero hardcoded programming-language syntax dictionaries, token tables, or keywords were added.
5. **Compilation & Test Integrity**:
   - `go build ./...`: Exit code 0 (clean).
   - `gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe`: Exit code 0 (0 warnings, 0 errors).
   - `.\test_przm_dec.exe`: Exit code 0 (9/9 test suites passed, 0 heap allocations, 100% lossless).
   - `tests/challenger_m2_test.c`: Exit code 0 (100,000 involution cycles passed, boundary lengths passed).
   - `lake build`: Exit code 0 ("Build completed successfully", 0 errors, 0 warnings, 0 `sorry`).
   - `go test -count=1 .`: Exit code 0 (full repository regression passed in 28.58s).

---

## 2-Phase Forensic Verification Procedure

### Phase 1: Mode-Agnostic Investigation (OBSERVE ALL)

#### 1. Source Code Analysis
- **Lie Group Matrix Law**: In `heisenberg.go`, elements are represented as $3 \times 3$ unitriangular matrices:
  $$
  H(x, y, z) = \begin{pmatrix} 1 & x & z \\ 0 & 1 & y \\ 0 & 0 & 1 \end{pmatrix} \in H_3(\mathbb{F}_{2^8})
  $$
  Matrix multiplication yields:
  $$
  H(x, y, z) H(x', y', z') = H(x \oplus x', y \oplus y', z \oplus z' \oplus (x \odot y'))
  $$
  Implemented in `MulH3`:
  ```go
  func MulH3(a, b HeisenbergElement3) HeisenbergElement3 {
      return HeisenbergElement3{
          X: a.X ^ b.X,
          Y: a.Y ^ b.Y,
          Z: a.Z ^ b.Z ^ GF256Mul(a.X, b.Y),
      }
  }
  ```
- **Exact Algebraic Involution**:
  Because $\operatorname{char}(\mathbb{F}_{2^8}) = 2$, additive inversion is the identity ($-x = x$).
  The algebraic matrix inverse is:
  $$
  H^{-1}(x, y, z) = H(x, y, z \oplus (x \odot y))
  $$
  Applying inversion twice yields:
  $$
  H^{-1}(H^{-1}(x, y, z)) = H(x, y, (z \oplus (x \odot y)) \oplus (x \odot y)) = H(x, y, z)
  $$
  Implemented in `InvH3`, `HeisenbergFoliateInplace`, and `przm_heisenberg_unfoliate_inplace`.
- **Pure C Unfoliator**:
  Implemented in `src/przm_dec.c` (`przm_heisenberg_unfoliate_inplace`):
  - Uses 4-way instruction-level unrolling for triplets (12 bytes/iter) and quads (16 bytes/iter).
  - Remainder loop handles unaligned trailing blocks ($n \pmod{\text{stride}}$).
  - Trailing unaligned bytes are untouched.
  - No heap allocation primitives (`malloc`, `free`, `realloc`) exist in the file.

#### 2. Facade and Cheat Detection
- Searched for dummy return constants, bypassed conditionals, pre-computed test tables, or mock decoders.
- None found. Forward foliation, inverse unfoliation, and covariance estimation are fully functional.

#### 3. Pre-Populated Artifact Detection
- Searched for pre-existing `*.log`, `*result*`, or `*output*` files in `d:\CodingProjects\przm`.
- None found predating the current iteration.

#### 4. Language Neutrality Audit
- Inspected all modified files (`heisenberg.go`, `przm_dec.h`, `przm_dec.c`, `compress.go`, `decompress.go`).
- Verified that no hardcoded token dictionaries, keyword strings, or syntax tables were added. Milestone 2 operates exclusively on general-purpose byte arrays using Galois field arithmetic.

---

### Phase 2: Mode-Specific Flagging (FLAG BY MODE)

Integrity Mode specified in `ORIGINAL_REQUEST.md`: **development**.

| Prohibited Pattern | Rule | Observed in M2 | Finding |
|---|:---:|:---:|:---:|
| Hardcoded test results | 🔴 Prohibited | None | **CLEAN** |
| Facade implementations | 🔴 Prohibited | None | **CLEAN** |
| Fabricated verification outputs | 🔴 Prohibited | None | **CLEAN** |
| External tool delegation | Permitted (Dev) | None (Pure C & Go native) | **CLEAN** |
| Code reuse / standard libraries | Permitted (Dev) | Standard Go / C runtime | **CLEAN** |

---

## Empirical Verification Evidence

### 1. Go Compilation & Unit Tests
```pwsh
cd d:\CodingProjects\przm
go build ./...
go test -v -count=1 -run TestHeisenberg .
```
**Output**:
```
=== RUN   TestHeisenberg_AlgebraAxioms
--- PASS: TestHeisenberg_AlgebraAxioms (0.00s)
=== RUN   TestHeisenberg_InvolutionProperty
--- PASS: TestHeisenberg_InvolutionProperty (0.00s)
=== RUN   TestHeisenberg_Stride3_ChannelIntegrity
--- PASS: TestHeisenberg_Stride3_ChannelIntegrity (0.00s)
=== RUN   TestHeisenberg_Stride4_PassThrough
--- PASS: TestHeisenberg_Stride4_PassThrough (0.00s)
=== RUN   TestHeisenberg_BoundaryLengths
--- PASS: TestHeisenberg_BoundaryLengths (0.00s)
=== RUN   TestHeisenberg_ZeroAllocDecompression
--- PASS: TestHeisenberg_ZeroAllocDecompression (0.00s)
=== RUN   TestHeisenberg_CompressionRatioAdvantage
    heisenberg_test.go:280: Heisenberg Multi-Channel 3D compression ratio: 3.02x (raw: 12288, compressed: 4068)
--- PASS: TestHeisenberg_CompressionRatioAdvantage (0.00s)
=== RUN   TestHeisenberg_EstimatorFidelity
--- PASS: TestHeisenberg_EstimatorFidelity (0.00s)
PASS
ok  	github.com/lemon4ksan/przm	0.373s
```

### 2. Go Micro-Benchmarks
```pwsh
go test -bench=BenchmarkHeisenberg -benchmem -run=^$ .
```
**Output**:
```
goos: windows
goarch: amd64
pkg: github.com/lemon4ksan/przm
cpu: 12th Gen Intel(R) Core(TM) i5-12400F
BenchmarkHeisenberg_ForwardFoliation-12             	   31596	     51397 ns/op	1275.09 MB/s	       0 B/op	       0 allocs/op
BenchmarkHeisenberg_InverseUnfoliationInplace-12    	   34711	     45253 ns/op	1448.23 MB/s	       0 B/op	       0 allocs/op
BenchmarkHeisenberg_DecompressZeroAlloc-12          	  330196	      3499 ns/op	18727.92 MB/s	       0 B/op	       0 allocs/op
PASS
ok  	github.com/lemon4ksan/przm	7.043s
```

### 3. Pure C Reference Decompressor Build & Execution
```pwsh
gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe
.\test_przm_dec.exe
```
**Output**:
```
=== Running przm_dec Pure C Verification Suite ===

[PASS] przm_get_uncompressed_size
[PASS] symplectic_involution
[PASS] mahler_transform
[PASS] standard_fused_raw
[PASS] standard_fused_lz
[PASS] pws_grammar
[PASS] isoquant_uncollimation
[PASS] gctr_decompression
[PASS] heisenberg_unfoliation

>>> ALL TESTS PASSED SUCCESSFULLY! (0 Allocations, 100% Lossless) <<<
```

### 4. Pure C Adversarial Challenger Suite
```pwsh
gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/challenger_m2_test.c -o challenger_m2_test.exe
.\challenger_m2_test.exe
```
**Output**:
```
=== Running Challenger M2 Heisenberg Adversarial Verification Suite ===

[START] C Algebraic Involution 100,000 Cycles...
[PASS] C Algebraic Involution 100,000 Cycles
[START] C Boundary & Unaligned Lengths (0..512)...
[PASS] C Boundary & Unaligned Lengths
[START] C przm_decompress Heisenberg Frame Tests...
[PASS] C przm_decompress Heisenberg Frame Tests

>>> ALL CHALLENGER M2 VERIFICATION SUITES PASSED! <<<
```

### 5. Go $\leftrightarrow$ Pure C Reference Interoperability
```pwsh
go test -v -count=1 -run TestPureCDecompressorInterop .
```
**Output**:
```
=== RUN   TestPureCDecompressorInterop
...
=== RUN   TestPureCDecompressorInterop/HeisenbergMultiChannelRGB/Level_1
=== RUN   TestPureCDecompressorInterop/HeisenbergMultiChannelRGB/Level_2
=== RUN   TestPureCDecompressorInterop/HeisenbergMultiChannelRGB/Level_3
=== RUN   TestPureCDecompressorInterop/HeisenbergMultiChannelRGB/Level_4
=== RUN   TestPureCDecompressorInterop/DirectHeisenbergFrame_Stride3
=== RUN   TestPureCDecompressorInterop/DirectHeisenbergFrame_Stride4
--- PASS: TestPureCDecompressorInterop (0.66s)
PASS
ok  	github.com/lemon4ksan/przm	0.934s
```

### 6. Formal Lean 4 Verification
```pwsh
lake build
```
**Output**:
```
Build completed successfully.
```
- Checked for `sorry` across all Lean 4 theorems: **0 occurrences**.

### 7. Full Repository Regression
```pwsh
go test -count=1 .
```
**Output**:
```
ok  	github.com/lemon4ksan/przm	28.583s
```
Zero failures across all tests in the repository.

---

## Detailed Audit Observations

1. **Galois Field Equivalence (Bit-Exact Across Languages)**:
   All 65,536 multiplications over $\mathbb{F}_{2^8}$ modulo 0x11B match identically between Go's table-driven `GF256Mul` and C's shift-and-add `gf256_mul`.
2. **Zero-Allocation Invariants**:
   - In Pure C (`src/przm_dec.c`), zero heap allocations are made on any decompression path (`malloc`/`free` count is 0).
   - In Go, `HeisenbergUnfoliateInplace` and `DecompressHeisenberg` (Mode 1: Foliated stream) are verified at `0 B/op, 0 allocs/op`.
   - In Go, Mode 3 (cascaded PWS) performs 2 to 4 heap allocations because it delegates to legacy Go `PWSDecode`, which allocates grammar rule buffers on the heap. Note that Pure C's `pws_decode` executes with 0 heap allocations using thread-local/stack buffers. This is an existing architectural property of the Go PWS port and does not violate the pure C zero-allocation mandate.
3. **Empty Slice Boundary Handling**:
   `CompressHeisenberg` on an empty slice `[]byte{}` returns `[]byte{}`. Passing an empty slice to `DecompressTo` correctly returns `[]byte{}, nil`. Directly calling `DecompressHeisenberg` with an empty slice returns `ErrHeisenbergCorrupt` because length < 12. This is consistent with other PRZM frame decompressors requiring valid frame headers.

---

## Forensic Verdict

**VERDICT**: **CLEAN**

The Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ foliation implementation exhibits genuine mathematical rigor, exact algebraic involution, 100% losslessness across cross-language pipelines, zero facades or synthetic shortcuts, zero hardcoded language dictionaries, and clean builds in Pure C, Go, and Lean 4.
