# Handoff Report: Milestone 3 Pure C Reference Decompressor & Interoperability Review

**Agent:** `reviewer_m3_2`  
**Working Directory:** `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m3_2`  
**Target Codebase:** `d:\CodingProjects\przm`  
**Date:** October 1, 2026  
**Type:** Hard Handoff (Review Complete)  
**Verdict:** `REQUEST_CHANGES`  

---

## 1. Observation

1. **Pure C Build and C Test Suite Execution:**
   - Command: `gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe; .\test_przm_dec.exe`
   - Result: Exited with code 0.
   - Output:
     ```text
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
     [PASS] cellular_sheaf_decompression
     >>> ALL TESTS PASSED SUCCESSFULLY! (0 Allocations, 100% Lossless) <<<
     ```

2. **Lean 4 Formal Verification Build:**
   - Command: `lake build` (in `d:\CodingProjects\przm`)
   - Result: Exited with code 0. Output: `Build completed successfully.`

3. **Go Cross-Language Interop Test (Twist=1):**
   - Command: `go test -v -run TestCellularSheaf_PureCDecompressorInterop .`
   - Result: Exited with code 0 (PASS across `CurlFree_Harmonic_8x8`, `SparseCurl_8x8`, `ConstantBoundaries_16x16`).

4. **Go Test Suite and Full Regression Failure:**
   - Command: `go test -v -run TestCellularSheaf .`
   - Output:
     ```text
     --- FAIL: TestCellularSheaf_ZeroAllocDecompression_AllModes (0.00s)
         --- FAIL: TestCellularSheaf_ZeroAllocDecompression_AllModes/SparseCurl_32x32 (0.00s)
             cellular_sheaf_adversarial_test.go:244: SparseCurl_32x32: violated zero-allocation invariant: 1.000000 allocs/op (expected 0)
         --- FAIL: TestCellularSheaf_ZeroAllocDecompression_AllModes/TrailingRemainder_64x64_plus_31 (0.00s)
             cellular_sheaf_adversarial_test.go:244: TrailingRemainder_64x64_plus_31: violated zero-allocation invariant: 1.000000 allocs/op (expected 0)
     --- FAIL: TestCellularSheaf_TwistGreaterThanOne_GoVsPureC (0.02s)
         cellular_sheaf_adversarial_test.go:449: Go Dec: 01 12 04 38 06 2C 18 50 04 48 10 E0
         cellular_sheaf_adversarial_test.go:450: C  Dec: 01 12 04 38 06 20 0C 74 04 48 10 E0
         cellular_sheaf_adversarial_test.go:453: CRITICAL BUG FOUND: Go and Pure C disagree on decompressing Cellular Sheaf frame with twist=2!
             Go: 01 12 04 38 06 2C 18 50 04 48 10 E0
              C: 01 12 04 38 06 20 0C 74 04 48 10 E0
     FAIL
     FAIL	github.com/lemon4ksan/przm	1.583s
     ```
   - Command: `go test -count=1 .`
   - Output: Exited with code 1 (`FAIL github.com/lemon4ksan/przm 29.895s`).

5. **Direct Source Inspection:**
   - In `src/przm_dec.c` lines 1278–1282:
     ```c
     } else {
         for (uint16_t j = 0; j < W; j++) {
             dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ v;
         }
     }
     ```
     `v` is held constant instead of evolving as $v(j+1) = \alpha \odot v(j)$.
   - In `src/przm_dec.c` lines 1302–1314:
     `v ^= next_c;` without multiplying `v` by `twist`.
   - In `cellular_sheaf.go` lines 813–819:
     `innerDec, err := PWSDecode(payload)` dynamically allocates memory when `CSHDFlagCascadedPWS` is present.
   - In `src/przm_dec.c` line 1231:
     `pos + (size_t)nnz * 5 > payload_len` lacks integer overflow check against 32-bit `size_t`.

---

## 2. Logic Chain

1. From Observation 1, the C code compiles cleanly with `-Wall -Wextra -Werror` and passes its self-contained tests because all tests in `test_przm_dec.c` set `Twist = 1`.
2. From Observation 4 and 5, when a Cellular Sheaf frame with Galois twist $\alpha > 1$ is decompressed, the mathematical model requires vertical boundary vector elements along column $j$ to satisfy the coboundary relation:
   $$v(j+1) = \alpha \odot v(j) \oplus C(i, j)$$
   Go `ReconstructCellularSheaf` implements this via `curV = GF256Mul(twist, curV)`. In contrast, `src/przm_dec.c` leaves `v` invariant across all columns. Therefore, the Pure C reference decompressor generates mathematically corrupted outputs whenever $\alpha > 1$.
3. From Observation 4 and 5, when `CompressCellularSheaf` applies `PWSEncode` to the payload (`CSHDFlagCascadedPWS`), `DecompressCellularSheaf` in Go calls `PWSDecode(payload)`. `PWSDecode` calls `make([]RuleL1, ...)`, `make([]RuleL2, ...)`, and `make([]byte, origSize+32)`, performing heap allocations. `testing.AllocsPerRun(100)` detects `1.000000 allocs/op`, violating the strict zero-allocation decompression invariant (`0 B/op, 0 allocs/op`).
4. From Observation 4, the full regression suite `go test -count=1 .` fails due to these two issues.
5. In accordance with project acceptance criteria ("Go codebase builds cleanly and all unit/roundtrip tests pass", "zero heap allocations", "Pure C reference decompressor passes all verification suites"), the required verdict is `REQUEST_CHANGES`.

---

## 3. Caveats

- We did NOT modify any implementation code, adhering to the review-only constraint.
- The mathematical foundations (coboundary nilpotency $d_1 \circ d_0 \equiv 0$, discrete Hodge Laplacians $\Delta_0, \Delta_1$, harmonic cycle projection, and 157.5x compression ratio on harmonic 2D manifolds) are sound and pass all verification tests when `twist = 1`.
- The defects are localized to:
  1. The column-evolution of `cur_v` in `src/przm_dec.c` when `twist > 1`.
  2. The PWS cascade path in `cellular_sheaf.go` which violates zero-allocation decompression.

---

## 4. Conclusion

Milestone 3 cannot be approved in its current state. The verdict is **`REQUEST_CHANGES`**.

Required remediation actions:
1. **Fix `src/przm_dec.c`**: In `przm_cellular_sheaf_decompress`, update the `twist > 1` loop paths (both curl-free and sparse-curl) to evolve `cur_v = gf256_mul(twist, cur_v)` matching Go `ReconstructCellularSheaf`.
2. **Fix `cellular_sheaf.go`**: Ensure `DecompressCellularSheaf` never allocates heap memory. Either disable PWS cascade or implement a zero-allocation `PWSDecode` that utilizes caller-provided scratchpads.
3. **Fix Sparse Curl Validation in `src/przm_dec.c`**: Prevent 32-bit overflow via `nnz > (payload_len - pos) / 5` and validate monotonic index progression.
4. **Pass Full Regression Suite**: Ensure `go test -count=1 .` exits with code 0.

---

## 5. Verification Method

1. **Verify Twist > 1 Divergence:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestCellularSheaf_TwistGreaterThanOne_GoVsPureC .
   ```
   *Expected Failure:* Divergence between Go and Pure C decompression outputs on `twist=2`.

2. **Verify Zero-Allocation Violation:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestCellularSheaf_ZeroAllocDecompression_AllModes .
   ```
   *Expected Failure:* Subtests `SparseCurl_32x32` and `TrailingRemainder_64x64_plus_31` fail with `1.000000 allocs/op`.

3. **Verify Full Regression Status:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -count=1 .
   ```
   *Expected Failure:* Exit code 1.
