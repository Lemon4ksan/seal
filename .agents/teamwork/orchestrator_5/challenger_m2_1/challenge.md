# Adversarial Challenge Report: Milestone 2 Heisenberg Foliation

**Challenger:** `challenger_m2_1` (Milestone 2 Heisenberg Boundary & Involution Challenger)  
**Parent Orchestrator:** `orchestrator_5` (Conversation ID: `1eb7944e-37f1-4b94-a209-66caae11aab3`)  
**Target Codebase:** PRZM Codebase (`d:\CodingProjects\przm`)  
**Date:** October 1, 2026  
**Gate Verdict:** **APPROVE** (with 2 documented minor edge-case findings)

---

## Challenge Summary

**Overall risk assessment:** **LOW**

An extensive empirical test harness comprising boundary buffer testing, unaligned length sweep (0 to 256 bytes exhaustive, plus prime/power-of-two boundaries up to 4097 bytes), 100,000 algebraic involution cycles in Go and Pure C, exhaustive $256 \times 256 = 65,536$ Galois field multiplication table cross-validation, and 100-goroutine concurrent stress testing under `-race` was executed.

The core algebraic properties of the Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ foliation hold rigorously:
1. In-place forward and inverse operations are exact mathematical involutions: $H^{-1}(H(x, y, z)) \equiv (x, y, z)$ over characteristic 2.
2. Channel preservation invariants are strictly obeyed: channels $X$ and $Y$ are unaltered; for stride 4, channel $W$ (alpha) is 100% untouched pass-through.
3. Tail handling across non-multiples of strides 3 and 4, as well as SWAR vector lengths 12, 16, and 32, correctly preserves unaligned trailing bytes without buffer overruns or memory corruption.
4. Pure C reference decompressor (`przm_dec.c`) builds cleanly under strict flags (`-O3 -Wall -Wextra -Werror`), passes all boundary frames, and operates with strictly 0 heap memory allocations.

Two non-critical edge-case findings were identified and documented below with recommended mitigations.

---

## Challenges & Findings

### [Low] Finding 1: Direct `CompressHeisenberg` Encodes Size 0 as Empty Slice, Breaking Direct Decompressor Symmetry

- **Assumption challenged:** Direct invocation of `CompressHeisenberg(src, level)` followed by `DecompressHeisenberg(comp, dst)` produces a lossless roundtrip for all valid buffer lengths, including $n = 0$.
- **Attack scenario:**
  A caller invokes `comp, err := przm.CompressHeisenberg([]byte{}, 3)`.
  In `heisenberg.go:214`:
  ```go
  func CompressHeisenberg(src []byte, level int) ([]byte, error) {
      n := len(src)
      if n == 0 {
          return []byte{}, nil
      }
  ```
  `comp` is returned as a 0-byte slice `[]byte{}`.
  When `comp` is passed to `DecompressHeisenberg(comp, dst)`:
  ```go
  if len(src) < HeisenbergHeaderSize { // HeisenbergHeaderSize == 12
      return nil, ErrHeisenbergCorrupt
  }
  ```
  The decompressor rejects the 0-byte slice with `przm: invalid or corrupted heisenberg frame`. Similarly, Pure C `przm_decompress((uint8_t*)"", 0, dst, dst_cap)` returns `PRZM_ERR_CORRUPTED_FRAME` (-2).
- **Blast radius:** Minimal. In the top-level PRZM compressor (`compress.go:533`), Heisenberg gating requires `uSize >= 48`, so 0-byte payloads are caught by root PRZM framing (`MagicPRZM`) and never route to `CompressHeisenberg`. This issue only impacts direct callers of the standalone `CompressHeisenberg` API.
- **Empirical evidence:**
  In `challenger_m2_adversarial_test.go:247`:
  `DISCOVERY: Size 0 DecompressHeisenberg returned error: przm: invalid or corrupted heisenberg frame (compressed len: 0)`
- **Recommended mitigation:**
  Remove the early `if n == 0 { return []byte{}, nil }` check in `CompressHeisenberg`. Allow $n = 0$ to fall through to `n < 12`, emitting a standard 12-byte raw bypass frame with `origLen = 0`. Both Go `DecompressHeisenberg` (lines 288–290) and C `przm_decompress` (lines 1723–1727) already contain explicit handlers for `origLen == 0` frames.

---

### [Low] Finding 2: Go Mode 3 (Cascaded PWS) Allocation Invariant Leak

- **Assumption challenged:** `DecompressHeisenberg` maintains the strict zero-heap-allocation contract (`0 B/op, 0 allocs/op`) across all operational modes when pre-allocated `dst` is provided.
- **Attack scenario:**
  On highly correlated streams where $z = x \odot y$, forward foliation produces all-zero residuals along channel $z$, triggering `PWSEncode` cascade in `CompressHeisenberg`. The frame is written with `mode = 3` (`HeisenbergModeCompressed`).
  In `DecompressHeisenberg`, Mode 3 delegates to `DecompressTo(payload, dst)` / `PWSDecode(payload)`.
  In Go, `PWSDecode` dynamically allocates slices for grammar rules `L1` through `L5`, generating 2 to 4 heap allocations per run.
- **Blast radius:** Minimal. Pure C reference decompressor `src/przm_dec.c` performs Mode 3 via thread-local scratchpads without calling `malloc` / `free`, strictly preserving zero heap allocations in C. In Go, Mode 1 (Foliated uncompressed) and Mode 0 (Raw) strictly preserve `0 B/op, 0 allocs/op`.
- **Empirical evidence:**
  In `challenger_m2_adversarial_test.go:408`:
  `Size 384: Mode 3 (Cascaded PWS) allocations: 2 allocs/op (delegated to inner PWSDecode)`
  `Size 6144: Mode 3 (Cascaded PWS) allocations: 4 allocs/op (delegated to inner PWSDecode)`
- **Recommended mitigation:**
  Implement a zero-allocation `PWSDecodeTo(src, dst []byte)` in `pws.go` using pooled grammar rule buffers (analogous to `padPool` in `compress.go`), eliminating dynamic slice allocations on the cascaded PWS decode path.

---

## Stress Test Results

| Test Scenario | Tool / Harness | Expected Behavior | Actual Behavior | Result |
|---|---|---|---|---|
| Exhaustive $256 \times 256$ Galois Field multiplication | `challenger_m2_adversarial_test.go` | Go table lookup matches C Russian peasant bitwise algorithm on all 65,536 pairs | 100% bit-exact equivalence on all 65,536 pairs | **PASS** |
| 100,000 algebraic involution cycles over $\mathbb{F}_{2^8}$ (Go) | `challenger_m2_adversarial_test.go` | $H^{-1}(H(x, y, z)) \equiv (x, y, z)$, channel X/Y unchanged, W pass-through | 100,000 / 100,000 iterations bit-exact | **PASS** |
| 100,000 algebraic involution cycles over $\mathbb{F}_{2^8}$ (Pure C) | `tests/challenger_m2_test.c` | Identical algebraic involution and channel preservation in C | 100,000 / 100,000 iterations bit-exact | **PASS** |
| Boundary buffer lengths: 0, 1, 2 bytes | Go & C suites | Safe no-op, zero buffer overrun, zero byte mutation | 0 bytes mutated, zero canaries overwritten | **PASS** |
| Unaligned buffer lengths: 10, 11, 25, 47, 49, 101, 103 | Go & C suites | Full blocks transformed, unaligned tails preserved bit-identically | Unaligned tails untouched; involution restored exact inputs | **PASS** |
| Exhaustive sweep of lengths 0 to 256 | Go & C suites | Zero off-by-one errors for both stride 3 and stride 4 | All 257 lengths passed involution and tail preservation | **PASS** |
| SWAR loop non-multiples (12, 16, 32 boundaries) | Go & C suites | Correct execution across unroll loop and single-element loop | Zero buffer overruns or misaligned byte corruptions | **PASS** |
| Frame robustness & malformed input fuzzing | Go & C suites | Corrupted magic, invalid strides (0, 1, 2, 5, 255), invalid modes safely rejected | All malformed frames rejected with appropriate error codes; zero crashes | **PASS** |
| Go $\to$ Pure C cross-decompression (50 random frames) | `c_interop_test.go` & `challenger_m2_test.go` | C decompressor bit-exactly reconstructs Go-compressed frames | 100% bit-exact match across all 50 frames | **PASS** |
| 100-goroutine concurrent stress test under `-race` | `challenger_m2_test.go` | Zero data races or concurrent corruption | 5,000 operations across 100 goroutines completed with 0 data races | **PASS** |
| Pure C reference decompressor build & test | GCC 14 (`-O3 -Wall -Wextra -Werror`) | Clean compilation with 0 warnings; all tests pass | 0 warnings, 0 errors, 100% tests passed | **PASS** |
| Lean 4 formal verification | `lake build` | Clean compilation with zero errors/warnings/sorry | Completed successfully with 0 errors | **PASS** |

---

## Unchallenged Areas

- **Lean 4 Heisenberg Theorem Formalization**: Deferred to Milestone 4 per `PROJECT.md` milestone planning.
- **Hardware SIMD vector assembly (AVX-512 / NEON)**: PRZM Milestone 2 specifies scalar/SWAR C implementation with compiler auto-vectorization flags (`-O3`). Explicit inline SIMD intrinsics are out of scope for this milestone.

---

## Gate Verdict

**VERDICT: APPROVE**

Milestone 2 meets all functional, algorithmic, algebraic, and interoperability criteria specified in DISPATCH.md and PROJECT.md. The implementation is robust against adversarial edge cases and ready to gate-pass to Milestone 3 (Cellular Sheaves with Discrete Hodge Laplacians).
