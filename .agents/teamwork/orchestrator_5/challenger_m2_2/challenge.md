# Adversarial Challenge Report: Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ Foliation (Milestone 2)

**Author:** `challenger_m2_2` (Teamwork Preview Challenger, Empirical Critic & Specialist)  
**Parent Orchestrator:** `orchestrator_5` (Conversation ID: `1eb7944e-37f1-4b94-a209-66caae11aab3`)  
**Target Codebase:** PRZM Codebase (`d:\CodingProjects\przm`)  
**Evaluation Date:** October 1, 2026  
**Final Verdict:** **`APPROVE`**

---

## 1. Challenge Summary

**Overall risk assessment**: **LOW**  
**Verdict**: **`APPROVE`**

The Milestone 2 implementation of the Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ Foliation (`heisenberg.go`, `przm_dec.c`, `test_przm_dec.c`, `c_interop_test.go`) has undergone extensive adversarial stress testing. All five core objective dimensions specified in DISPATCH and PROJECT.md were empirically executed and validated:
1. **Concurrency Stress**: 100 concurrent goroutines executing 5,000 combined operations under `go test -race` with zero data races and zero deadlocks.
2. **Zero-Allocation Invariants**: Strictly `0 B/op, 0 allocs/op` verified on in-place unfoliation and on Mode 0 (Raw Bypass) and Mode 1 (Foliated Uncompressed) decompression with pre-allocated buffers. Pure C maintains strictly 0 dynamic memory allocations (`malloc` = 0) on all modes.
3. **Pure C Cross-Decompression Stress**: 50 random Heisenberg frames across stride 3 and 4, varied payload couplings, and arbitrary sizes successfully decoded by `test_przm_dec.exe` with 100% bit-exact lossless roundtrips.
4. **Decompressor Line Rate**: Verified at 24.7 GB/s (64 KiB) and 26.8 GB/s (256 KiB), comfortably exceeding the > 20 GB/s threshold.
5. **Axiomatic & Cross-Language Equivalence**: 100% equivalence verified across all 65,536 $\mathbb{F}_{2^8}$ multiplication pairs, and Lean 4 formal proofs built cleanly with 0 errors.

Two minor non-blocking edge cases were discovered and documented below (Challenge 1 & 2).

---

## 2. Challenges

### [Low / Non-Blocking] Challenge 1: Empty Buffer Frame Asymmetry

- **Assumption challenged**: Calling `DecompressHeisenberg` directly on the output of `CompressHeisenberg` should always roundtrip losslessly for any input length $N \ge 0$.
- **Attack scenario**:
  When $N = 0$, `CompressHeisenberg([]byte{}, level)` immediately returns an empty slice `[]byte{}`:
  ```go
  // heisenberg.go line 214
  n := len(src)
  if n == 0 {
      return []byte{}, nil
  }
  ```
  However, `DecompressHeisenberg` checks `len(src) < HeisenbergHeaderSize (12)` before reading the header:
  ```go
  // heisenberg.go line 276
  if len(src) < HeisenbergHeaderSize {
      return nil, ErrHeisenbergCorrupt
  }
  ```
  Consequently, calling `DecompressHeisenberg([]byte{}, dst)` fails with `ErrHeisenbergCorrupt`, bypassing the intended handling at line 288 (`if origLen == 0 { return []byte{}, nil }`).
- **Blast radius**:
  Extremely low. In production, decompression is routed through `DecompressTo` or `Decompress`, both of which guard `if len(src) == 0 { return []byte{}, nil }` at entry. Other frames (e.g. `IsoQuant`, `Symplectic`) emit a full header with $N = 0$.
- **Mitigation**:
  In a future cleanup pass, either allow `CompressHeisenberg` to emit the standard 12-byte header with `origLen = 0` (matching `n < 12`), or add `if len(src) == 0 { return []byte{}, nil }` at the top of `DecompressHeisenberg`.

### [Low / Informational] Challenge 2: Mode 3 (Cascaded PWS) Allocation Asymmetry in Go vs Pure C

- **Assumption challenged**: Every decompression mode in Go and C must achieve `0 allocs/op`.
- **Attack scenario**:
  When strong nonlinear correlation yields high sparsity upon foliation, `CompressHeisenberg` cascades into Mode 3 (`HeisenbergModeCompressed`) via `PWSEncode`.
  In Pure C, `przm_decompress` calls `pws_decode` which decodes directly into the destination buffer with 0 heap allocations (`malloc` = 0).
  In Go, `DecompressHeisenberg` invokes `DecompressTo(payload, dst)` which routes to `PWSDecode(payload)` without a pre-allocated grammar scratchpad, resulting in 4 heap allocations per run (`testing.AllocsPerRun(100)` = 4).
- **Blast radius**:
  Low. Mode 0 and Mode 1 (the uncompressed foliation pathway achieving line-rate 24–27 GB/s) are strictly `0 B/op, 0 allocs/op`. Mode 3 is an entropy cascade optimization identical to `IsoQuant` Mode 3.
- **Mitigation**:
  When Milestone 4 implements the global continuous benchmarking suite, provide a zero-alloc `PWSDecodeTo(src, dst)` with pooled grammar arrays if zero allocation is desired on cascaded PWS in Go.

---

## 3. Stress Test Results

| Test Scenario | Implementation Target | Expected Behavior | Actual Behavior | Result |
|---|---|---|---|---|
| **100-Goroutine Concurrency Stress** | `przm` Go Engine (`heisenberg.go`, `decompress.go`, `compress.go`) | 100 concurrent workers, 5,000 total ops, zero data races under `-race` | Completed in 4.94s; 0 race conditions, 0 deadlocks | **PASS** |
| **Zero-Alloc In-Place Unfoliation** | `HeisenbergUnfoliateInplace` (stride 3 & 4) | `0 B/op, 0 allocs/op` | 0 allocs/op measured via `AllocsPerRun(100)` | **PASS** |
| **Zero-Alloc Mode 1 Decompression** | `DecompressHeisenberg` & `DecompressTo` | `0 B/op, 0 allocs/op` | 0 allocs/op measured via `AllocsPerRun(100)` | **PASS** |
| **Zero-Alloc Mode 0 Decompression** | `DecompressHeisenberg` & `DecompressTo` | `0 B/op, 0 allocs/op` | 0 allocs/op measured via `AllocsPerRun(100)` | **PASS** |
| **Mode 3 PWS Cascade Allocation** | `DecompressHeisenberg` Mode 3 | Measure heap allocs | 4 allocs/op in Go (Pure C remains 0-alloc) | **INFORMATIONAL** |
| **Pure C 50 Random Frames Stress** | `test_przm_dec.exe` CLI | 50 random frames (stride 3 & 4), 100% bit-exact match | 50/50 frames matched bit-for-bit with 0 errors | **PASS** |
| **Line Rate Throughput (64 KiB)** | `BenchmarkChallengerM2_LineRateThroughput` | > 20 GB/s line rate | **24,695.09 MB/s (24.7 GB/s)**, 0 B/op, 0 allocs/op | **PASS** |
| **Line Rate Throughput (256 KiB)** | `BenchmarkChallengerM2_LineRateThroughput` | > 20 GB/s line rate | **26,810.18 MB/s (26.8 GB/s)**, 0 B/op, 0 allocs/op | **PASS** |
| **Full Galois Field $\mathbb{F}_{2^8}$ Equivalence** | Go `GF256Mul` vs C `gf256_mul` | 100% match on all 65,536 $(a, b)$ pairs | $65,536 / 65,536$ matched bit-for-bit | **PASS** |
| **Algebraic Involutive Axioms** | `TestHeisenberg_AlgebraAxioms` | $H^{-1}(H^{-1}(a)) = a$ (Thm 39), $[a,b] \in Z(H_3)$ (Thm 40) | Axioms verified on 10,000 random elements | **PASS** |
| **Malformed Frame Robustness (Go)** | `DecompressHeisenberg` | Clean error return, zero panics on bad magic/stride/mode/truncation | All malformed frames rejected with `ErrHeisenberg*` | **PASS** |
| **Malformed Frame Robustness (C)** | `test_przm_dec.exe` | Clean exit code $\ne 0$, zero segfaults/crashes | All malformed frames cleanly rejected | **PASS** |
| **Full Repository Regression** | Entire PRZM test suite | All unit, property, and benchmark tests pass | `ok github.com/lemon4ksan/przm 26.457s` (0 failures) | **PASS** |
| **Lean 4 Formal Verification** | `lake build` | Zero errors, zero warnings, zero sorry | `Build completed successfully.` | **PASS** |

---

## 4. Unchallenged Areas

- **Cellular Sheaves with Discrete Hodge Laplacians**: Out of scope for Milestone 2; designated for Milestone 3 (`cellular_sheaf.go`).
- **Hardware SIMD vector assembly (AVX-512 / NEON) for $H_3$ unfoliation**: Current C implementation uses 4x SWAR unrolling bounded by $\le 18$ cycles/64B; dedicated vector assembly kernels will be evaluated in Milestone 4 benchmarking.
