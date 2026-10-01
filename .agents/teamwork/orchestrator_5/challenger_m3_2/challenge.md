# Adversarial Challenge Report: Milestone 3 Cellular Sheaves with Discrete Hodge Laplacians

**Challenger Agent:** `challenger_m3_2` (Concurrency, Zero-Alloc & C Stress Challenger 2)  
**Target Codebase:** `d:\CodingProjects\przm`  
**Date:** October 1, 2026  
**Final Gate Verdict:** **`REQUEST_CHANGES`**

---

## 1. Executive Summary

Milestone 3 implements 2D Cellular Sheaves with Discrete Hodge Laplacians over $\mathbb{F}_{2^8}$ (`cellular_sheaf.go`, `src/przm_dec.c`, `include/przm_dec.h`).
We conducted an adversarial empirical stress evaluation focusing on:
1. Concurrency stress: 100 concurrent goroutines executing simultaneous compression/decompression under the Go race detector (`go test -race`).
2. Instantaneous Zero-Allocation contract: `testing.AllocsPerRun(100)` checking strictly `0 B/op, 0 allocs/op` during decompression and zero `malloc`/`free` calls in C.
3. Pure C Reference Decompressor Cross-Interop: Generating 50 random Cellular Sheaf frames (varied strides, varied dimensions, varied curl densities, raw passthrough) decoded by `test_przm_dec.exe`.
4. Microarchitectural throughput and CPU cycle benchmarks.
5. Non-trivial Galois twist ($\alpha > 1$) edge case and Hodge cochain evolution analysis.

While the 100-goroutine concurrency under `-race` passed flawlessly and all 50 random frames with canonical twist $\alpha = 1$ decompressed bit-for-bit losslessly in Pure C, **adversarial stress testing uncovered a critical mathematical divergence and data corruption bug in the Pure C decompressor when $\alpha > 1$ and boundary vertical 1-forms are non-zero ($Ev \ne 0$)**, along with a heap allocation leak in Go decompression when payload PWS cascading is triggered (`CSHDFlagCascadedPWS`).

Because PRZM frames with $\alpha > 1$ compressed by Go cannot be decompressed correctly by the Pure C reference decompressor, this milestone **cannot be approved in its current state**.

---

## 2. Challenge Findings

### [CRITICAL] Challenge 1: Pure C Decompressor Corrupts Output on Non-Trivial Galois Twists ($\alpha > 1$)

- **Assumption Challenged:** Worker M3 claimed in `worker_m3/handoff.md` Section 1.2 and Section 2.3 that `src/przm_dec.c` faithfully implements the mathematical inverse row sweep for all cellular sheaf frames.
- **Attack Scenario:**
  1. Under the discrete 1-coboundary relation $(d_1 u)_{i, j} \equiv 0$ with twist $\alpha \in \mathbb{F}_{2^8}$, when horizontal cochains vanish or are constant, the vertical cochain advances across columns according to:
     $$u^v_{i, j} = \alpha \cdot u^v_{i, j-1} = \alpha^j \cdot u^v_{i, 0}$$
  2. In Go (`cellular_sheaf.go` lines 498–504 and 531–554), this is correctly implemented by multiplying the vertical accumulator by $\alpha$ at every column:
     ```go
     curV := v
     for j := 0; j < W; j++ {
         dst[rowCurr+j] = GF256Mul(twist, dst[rowPrev+j]) ^ curV
         curV = GF256Mul(twist, curV)
     }
     ```
  3. In Pure C (`src/przm_dec.c` lines 1278–1282 and 1302–1314), the inner loop fails to evolve `cur_v`:
     ```c
     } else {
         for (uint16_t j = 0; j < W; j++) {
             dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ v;
         }
     }
     ```
     `v` is never multiplied by `twist` across columns!
- **Empirical Proof & Reproduction:**
  We constructed an end-to-end test case (`TestMilestone3_Twist2_NonZeroEv_EndToEndInterop` in `cellular_sheaf_concurrency_c_test.go`) with $W=8, M=16$, Twist $\alpha = 2$, and $Ev \ne 0$.
  - Original 128 bytes was compressed by `CompressCellularSheaf` to 26 bytes (`Twist=2, Flags=0x000D`).
  - Go `DecompressCellularSheaf`: **PASS** (100% bit-exact lossless roundtrip).
  - Pure C `test_przm_dec.exe`: **FAIL** with output data corruption starting at row 1 column 1:
    ```text
    Orig: 03 06 0C 18 30 60 C0 9B 03 06 0C 18 30 60 C0 9B
    Go:   03 06 0C 18 30 60 C0 9B 03 06 0C 18 30 60 C0 9B
    C:    03 06 0C 18 30 60 C0 9B 03 09 1D 35 65 C5 9E 28
    ```
- **Blast Radius:** Any data stream exhibiting multiplicative progression in $\mathbb{F}_{2^8}$ compressed with level $\ge 3$ where the encoder discovers $\alpha > 1$ will be silently corrupted upon decompression in Pure C!
- **Mitigation Required:**
  In `src/przm_dec.c`:
  1. For Curl-Free mode when `twist > 1`:
     ```c
     } else {
         uint8_t cur_v = v;
         for (uint16_t j = 0; j < W; j++) {
             dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ cur_v;
             cur_v = gf256_mul(twist, cur_v);
         }
     }
     ```
  2. For Sparse Curl mode when `twist > 1`:
     Update `cur_v` with `cur_v = gf256_mul(twist, cur_v) ^ next_c` when curl index matches, and `cur_v = gf256_mul(twist, cur_v)` otherwise.
  3. Add explicit test cases for `Twist > 1` with non-zero $Ev$ in `tests/test_przm_dec.c`.

---

### [MEDIUM] Challenge 2: Heap Allocation Contract Violation in Go Decompressor on Cascaded PWS Frames

- **Assumption Challenged:** Worker M3 claimed that `DecompressCellularSheaf` strictly satisfies `0 B/op, 0 allocs/op` for all frames when `dst` is pre-allocated.
- **Attack Scenario:**
  1. In `cellular_sheaf.go` lines 753–766, `CompressCellularSheaf` conditionally applies `PWSEncode` to the boundary representation payload. If PWS achieves a size reduction, it sets `CSHDFlagCascadedPWS` (`0x0010`).
  2. In `cellular_sheaf.go` lines 813–819:
     ```go
     if (flags & CSHDFlagCascadedPWS) != 0 {
         innerDec, err := PWSDecode(payload)
         if err != nil {
             return nil, err
         }
         payload = innerDec
     }
     ```
  3. `PWSDecode(payload)` in `pws.go` line 368 dynamically allocates grammar rule slices (`make([]RuleL1, numL1)`) and allocates the destination slice `innerDec` on the heap.
- **Empirical Proof & Reproduction:**
  In `cellular_sheaf_concurrency_c_test.go` and `cellular_sheaf_adversarial_test.go`:
  - When decompressing a frame where `CSHDFlagCascadedPWS` is set (e.g. 64 KiB structured grid with 1086-byte payload cascaded via PWS), `testing.AllocsPerRun(100)` measured:
    `2304 B/op, 2 allocs/op` (violation of the strict 0 allocs/op invariant).
  - Note: Uncascaded frames (both curl-free and sparse curl) strictly achieve `0 B/op, 0 allocs/op`.
- **Blast Radius:** Applications relying on the zero-allocation guarantee on latency-critical paths experience GC pressure when processing cascaded cellular sheaf frames.
- **Mitigation Required:**
  Either:
  - Add `PWSDecodeInPlace(src, scratch []byte) ([]byte, error)` using thread-local / pre-allocated scratch memory (mirroring C's `tls_lits_scratch`), OR
  - In `CompressCellularSheaf`, avoid PWS cascading unless explicit zero-allocation bypass flags are disabled, OR ensure `DecompressCellularSheaf` uses a pre-allocated stack/TLS buffer for the small boundary payload before unpacking.

---

### [LOW] Challenge 3: Inadequate Test Coverage in C Reference Verification Suite

- **Assumption Challenged:** Worker M3 claimed full verification in `tests/test_przm_dec.c`.
- **Attack Scenario:**
  In `tests/test_przm_dec.c` lines 424–565, all 4 test functions (`test_cellular_sheaf_decompression`) hardcode:
  - Line 435: `frame_raw[13] = 1; /* Twist */`
  - Line 466: `frame_cf[13] = 1;  /* Twist = 1 */`
  - Line 505: `frame_const[13] = 1; /* Twist = 1 */`
  - Line 547: `frame_sp[13] = 1;    /* Twist = 1 */`
  Zero tests evaluated non-trivial Galois twists $\alpha > 1$.
- **Mitigation Required:** Expand `tests/test_przm_dec.c` with at least 2 test cases covering $\alpha = 2$ and $\alpha = 0x1B$.

---

## 3. Empirical Stress Test Results

| Test Scenario | Target | Measured Result | Verdict |
|---------------|--------|-----------------|---------|
| **100-Goroutine Concurrency under `-race`** | 0 data races, 100% lossless | 100 goroutines $\times$ 50 iterations (5,000 roundtrips), 0 races, 0.59s | **PASS** |
| **Go Zero-Alloc (Uncascaded Frames)** | Strictly 0 B/op, 0 allocs/op | 0 B/op, 0 allocs/op on CurlFree, ConstBoundaries, Raw | **PASS** |
| **Go Zero-Alloc (Cascaded PWS Frames)** | Strictly 0 B/op, 0 allocs/op | 2304 B/op, 2 allocs/op via `PWSDecode` | **FAIL** (Finding 2) |
| **Pure C Decompressor Dynamic Memory** | Strictly 0 `malloc`/`free` | 0 `malloc`, 0 `free` in `przm_cellular_sheaf_decompress` | **PASS** |
| **Pure C 50 Random Frames ($\alpha = 1$)** | 50/50 bit-exact lossless | 50/50 frames decoded losslessly (varied strides, dimensions, curls) | **PASS** |
| **Pure C Non-Trivial Twist ($\alpha > 1, Ev \ne 0$)** | Bit-exact lossless | C produces corrupted output at row 1 column 1+ | **FAIL** (Finding 1) |
| **Single-Core C Decompression Line Rate** | Line rate / low cycles | 5.49 GB/s, 0.423 cycles/byte, 27.07 cycles/64B block | **PASS** |
| **Single-Core Go SWAR Row Sweep Line Rate** | Line rate / low cycles | 6.08 GB/s, 29.39 cycles/64B block, 0 B/op, 0 allocs/op | **PASS** |

---

## 4. Unchallenged Areas

- **Lean 4 Formal Proofs:** Formal proofs in `PRZM/` for algebraic theorems build cleanly (`lake build`). Mathematical nilpotency $d_1 \circ d_0 \equiv 0$ was fuzz-verified over 50,000 iterations by challenger 1.
- **Maximum Block Size Boundary:** Block sizes near 16 MB were spot-checked; extreme multi-gigabyte memory exhaustion was not tested as MaxBlockSize is 16 MB.

---

## 5. Gate Recommendation

**Verdict: `REQUEST_CHANGES`**

Milestone 3 is mechanically well-structured and displays high microarchitectural throughput (5.5–6.1 GB/s, 0.42 cycles/byte) and clean concurrency scaling. However, upstream integration must be blocked until Worker M3 applies the following two fixes:
1. Fix `src/przm_dec.c` lines 1280 and 1302–1314 to evolve `cur_v = gf256_mul(twist, cur_v)` across columns when `twist > 1`, resolving the C decompressor corruption bug.
2. Address the heap allocation in Go's `DecompressCellularSheaf` when `CSHDFlagCascadedPWS` is active, ensuring strictly 0 heap allocations across all frame types.
