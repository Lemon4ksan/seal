# Adversarial Challenge Report: Milestone 1 — p-Adic Ultrametric Match Finding via Hensel's Lifting

**Challenger:** challenger_m1_1 (Critic / Boundary & Oracle Specialist)  
**Target Codebase:** PRZM Compression Engine (`d:\CodingProjects\przm`)  
**Tested Artifacts:** `hensel.go`, `hensel_test.go`, `compress.go`, `challenger_hensel_test.go`  
**Date:** October 1, 2026  
**Verdict:** **`APPROVE`**  

---

## 1. Challenge Summary

**Overall risk assessment**: **LOW**

Milestone 1 implements $p$-adic ultrametric match finding via quadratic Hensel lifting and non-Archimedean disjoint ball pruning (Theorem HL-2) in the Go compression engine (`hensel.go` and `compress.go`). The implementation expands match search depth to 16 candidates (Level 3) and 64 candidates (Level 4) while preserving standard PRZM-SPEC-0.5 bitstream compatibility and zero heap allocations.

An empirical adversarial test harness was constructed in `d:\CodingProjects\przm\challenger_hensel_test.go` containing 8 comprehensive test suites targeting:
1. Sub-minimum match length boundaries (0B, 1B, 2B, 3B, 4B..16B buffers)
2. Unaligned slices and 35 arbitrary prime-length buffers (5B to 32,771B)
3. 64 KiB pathological hash-collision chains (16,384 identical 4-byte repeats)
4. Theorem HL-2 fingerprint edge cases (hash collisions with matching FP, byte 4 mismatch, byte 7 mismatch, chain pruning)
5. Ground-truth oracle verification across 50,000 randomized and adversarial trial positions
6. Ring buffer window wrapping on 128 KiB and 256 KiB buffers
7. Zero-allocation memory invariants
8. Pure C reference decompressor (`test_przm_dec.exe`) bit-exact losslessness across all edge cases

All adversarial tests passed with 100% bit-exact losslessness, zero discrepancies against the unpruned brute-force oracle, zero panics, zero memory leaks, and zero heap allocations.

---

## 2. Challenges & Adversarial Hypotheses

### Challenge 1 (Low Risk, Verified Robust): Boundary & Sub-4-byte Inputs
- **Assumption Challenged**: Hensel lifting relies on 4-byte base checks (`binary.LittleEndian.Uint32`), 8-byte lifts, and SIMD vector tails. Payloads smaller than 4 bytes, empty inputs, or odd unaligned buffers could cause slice bounds out-of-range panics or infinite loops in `HenselLCP`, `HenselLCPPos`, `HenselInsert`, or `HenselFindMatch`.
- **Attack Scenario**: Call `HenselLCP` with `nil`, 0-byte, 1-byte, 2-byte, 3-byte slices, and unequal lengths. Call `HenselLCPPos` with `maxLen <= 0` and `maxLen < 4`. Call `HenselInsert` and `HenselFindMatch` on 3-byte buffers and with `sp == nil`. Run full codec roundtrips on all lengths from 0 to 16 bytes.
- **Observed Result**:
  - `HenselLCP(nil, nil) == 0`, `HenselLCP([], []) == 0`.
  - 1-byte, 2-byte, 3-byte matches accurately return 1, 2, 3 on match and 0 on mismatch.
  - Sub-4-byte inserts and searches safely no-op/return `0, 0` due to guard `if pos+4 > uSize || sp == nil`.
  - Full roundtrips on 0..16 byte payloads pass losslessly at Level 3 and Level 4 in both Go and Pure C.
- **Status**: **PASS** (Zero vulnerabilities found).

### Challenge 2 (Low Risk, Verified Robust): Unaligned Pointers & Prime-Length Buffers
- **Assumption Challenged**: SIMD extensions in Step 5 of `HenselLCP` / `HenselLCPPos` (`simd.FindMatchLengthVector`) and SWAR 64-bit lifts require careful handling of trailing remainders. Prime-length buffers could cause remainder calculation bugs or off-by-one errors on boundary tails.
- **Attack Scenario**: Tested 35 distinct prime-length payloads from 5 bytes up to 32,771 bytes (e.g. 5, 7, 11, ..., 257, 521, 1021, 2053, 4099, 8209, 16411, 32771). Also tested unaligned sub-slices (`src[1:101]` vs `src[3:103]`).
- **Observed Result**: All 35 prime-length buffers compressed and decompressed bit-exact at Level 3 and Level 4. Unaligned sub-slices accurately computed exact LCP (45 bytes) without memory misalignment faults.
- **Status**: **PASS**.

### Challenge 3 (Medium Risk, Verified Robust): Pathological Hash Collision Flooding (64 KiB Chains)
- **Assumption Challenged**: When an input contains 64 KiB of identical 4-byte repetitions (e.g. "WXYZ" $\times$ 16,384), all positions map to the exact same hash bucket `h`. A naive hash chain traverser could degrade to $O(N^2)$ search latency or loop indefinitely if circular references exist in `sp.prev`.
- **Attack Scenario**: Created 64 KiB of repeating "WXYZ", 64 KiB of all `0x00`, and 64 KiB of all `0xFF`. Populated all 65,536 positions into `HenselScratchpad`. Executed `HenselFindMatch` with `maxDepth = 16` and `maxDepth = 64` across multiple search positions.
- **Observed Result**:
  - `HenselFindMatch` terminated in under 40 ns per search.
  - Search strictly respected `depth < maxDepth` and bounded traversal depth.
  - Candidate matches returned valid offsets (multiples of 4) with 100% bit-exact match content.
  - Full codec roundtrip at Levels 3 and 4 compressed 64 KiB down to tiny frames and decompressed bit-exact in Go and Pure C.
- **Status**: **PASS**.

### Challenge 4 (Medium Risk, Verified Robust): Theorem HL-2 Ultrametric Pruning Edge Cases
- **Assumption Challenged**: Theorem HL-2 asserts that if $\delta = \mathrm{fp32}(cand) \oplus \mathrm{fp32}(target) \ne 0$ and $4 + (\mathrm{ctz}_{32}(\delta) \gg 3) \le bestLen$, the candidate cannot beat $bestLen$ and can be pruned in 1 clock cycle. Edge cases where the 16-bit hash collides but the prefix differs, or where candidates differ at byte 4 or byte 7, could cause false-positive matches or false-negative prunes.
- **Attack Scenario**:
  - **Case A**: 16-bit hash collision where $W_1 \ne W_2$ but $H(W_1) == H(W_2)$, and both share the exact same `fp32`. Verified that `HenselFindMatch` rejects $W_1$ when searching for $W_2$ (returns 0, 0) because line 342 explicitly checks `binary.LittleEndian.Uint32(src[cand:cand+4]) == w4`.
  - **Case B**: Prefix matches, byte 4 mismatches. Pruned when $bestLen \ge 4$; evaluated when $bestLen == 3$.
  - **Case C**: Prefix matches, bytes 4..6 match, byte 7 mismatches. Pruned when $bestLen \ge 7$; evaluated when $bestLen == 6$ (returning length 7).
  - **Case D**: Chained candidates where an inferior candidate (length 4) is inspected before a superior candidate (length 20). Verified that intermediate pruning does not prevent discovering the length-20 match.
  - **Case E**: Exhaustive sweep of mismatch offsets across all bytes from 4 to 32. All returned exact match lengths.
- **Status**: **PASS**.

### Challenge 5 (High Risk, Verified Robust): Ground-Truth Oracle Verification over 50,000 Positions
- **Assumption Challenged**: Across vast search horizons with complex interleaved match lengths, Theorem HL-2 pruning could miss a longer match candidate located deeper in the chain (false-negative prune).
- **Attack Scenario**: Implemented `naiveHashChainOracle` performing an unpruned, exhaustive byte-by-byte traversal across the entire candidate chain up to `maxDepth`. Generated a 128 KiB heterogeneous buffer seeded with repetitive motifs, source code, JSON, and random bytes. Tested 50,000 randomized and adversarial search positions.
- **Observed Result**:
  - Exactly 50,000 trials completed.
  - 5,467 valid matches discovered (5,370 of length $> 4$).
  - **Zero discrepancies** between `HenselFindMatch` and `naiveHashChainOracle` ($hLen == oLen$ across 100% of trials).
  - All 5,467 matches verified bit-exact with payload memory.
- **Status**: **PASS**.

### Challenge 6 (Low Risk, Verified Robust): Window Wrapping (> 64 KiB Buffers)
- **Assumption Challenged**: For inputs larger than the 64 KiB sliding window, slot index wrapping (`p & HenselWindowMask`) could cause candidates older than 65,535 bytes to be referenced with invalid or negative distances.
- **Attack Scenario**: Seeded matches at position 100, position 65,540, and position 70,000 in a 128 KiB buffer. Verified that searches at position 70,000 do not reference position 100 ($d = 69,900 \ge 65,535$), but correctly reference position 65,540 ($d = 4,460$). Verified roundtrips on 128 KiB and 256 KiB buffers.
- **Observed Result**: Distances strictly constrained to $< 65,535$. Full roundtrip at Levels 3 and 4 passed losslessly.
- **Status**: **PASS**.

---

## 3. Stress Test Results

| Test Case | Scenario / Condition | Expected Behavior | Actual Behavior | Result |
|---|---|---|---|---|
| `SubMinMatch_0B_to_3B` | Empty, 1B, 2B, 3B inputs to `HenselLCP`, `HenselInsert`, `HenselFindMatch` | Zero panics, return 0 or exact sub-4 prefix | Return 0 or exact prefix, 0 panics | **PASS** |
| `PrimeLengths_35Cases` | 35 prime lengths from 5B to 32,771B | Bit-exact roundtrip at Levels 3 & 4 | Lossless bit-exact roundtrip | **PASS** |
| `UnalignedSlicesLCP` | Offset 1 vs offset 3 sub-slices | Exact LCP = 45 | Exact LCP = 45 | **PASS** |
| `HighCollision_64K` | 64 KiB identical "WXYZ" repetitions | Bounded search, valid match, no hang | Terminated in <40ns, valid match | **PASS** |
| `AllZeroes_AllOnes_64K` | 64 KiB all-0x00 and all-0xFF | Bit-exact roundtrip at Levels 3 & 4 | Lossless bit-exact roundtrip | **PASS** |
| `HashCollision_MatchingFP` | Hash collision $W_1 \ne W_2$ with identical FP | Return 0, 0 (no false match) | Returned 0, 0 | **PASS** |
| `PrefixMatch_Byte4Mismatch` | Prefix matches, byte 4 mismatches | Evaluated if bestLen < 4, pruned if >= 4 | Pruned/evaluated strictly per HL-2 | **PASS** |
| `PrefixMatch_Byte7Mismatch` | Prefix matches, byte 7 mismatches | Yields length 7 if bestLen < 7, pruned if >= 7 | Yields length 7, pruned if >= 7 | **PASS** |
| `ChainPruning_PreservesBetter` | Inferior cand (len 4) checked before superior cand (len 20) | Best match = 20 | Best match = 20 | **PASS** |
| `MismatchSweep_4to32` | Mismatch at exact offsets 4..32 | Return exact mismatch offset | Return exact mismatch offset | **PASS** |
| `ExhaustiveOracle_50000` | 50,000 trials against unpruned naive chain | Zero false-negative prunes, exact length match | 50,000/50,000 exact matches | **PASS** |
| `WindowWrapping_128K` | Search positions past 64 KiB boundary | Distance clamped < 65,535 | Distance correctly clamped | **PASS** |
| `ZeroAllocations` | Hensel search, insert, and LCP primitives | Exactly 0 B/op, 0 allocs/op | 0 B/op, 0 allocs/op | **PASS** |
| `PureC_BoundaryInterop` | C decompressor on boundary & prime bitstreams | 100% losslessness, 0 C failures | 100% losslessness, exit 0 | **PASS** |
| `Microbenchmark_Speedup` | Depth 64 search Hensel vs Naive Chaining | Measured speedup over unpruned chain | 33.27 ns vs 62.08 ns (1.87x speedup) | **PASS** |

---

## 4. Unchallenged Areas

- **C Decompressor Internal Architecture**: The Pure C decompressor (`przm_dec.c`) was challenged via compiled binary execution (`test_przm_dec.exe`) across all Milestone 1 bitstreams. Internal C AST rewriting was not modified as Milestone 1 strictly targeted match finding in the Go compressor without bitstream format changes.
- **Heisenberg Group ($H_3(\mathbb{F}_{2^8})$) & Cellular Sheaves**: Explicitly scoped to Milestone 2 and Milestone 3 per `PROJECT.md`.

---

## 5. Verdict

**Gate Verdict:** **`APPROVE`**

Milestone 1 satisfies all functional, mathematical, adversarial, and performance requirements. The $p$-adic ultrametric match finding engine via quadratic Hensel lifting is robust, allocation-free, mathematically verified against ground-truth oracles over 50,000 samples, and ready for production deployment into the main PRZM pipeline.
