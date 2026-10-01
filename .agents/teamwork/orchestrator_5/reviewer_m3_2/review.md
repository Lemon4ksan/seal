# Milestone 3 Pure C Reference Decompressor & Interoperability Review

**Reviewer:** `reviewer_m3_2` (Teamwork Reviewer & Adversarial Critic)  
**Date:** October 1, 2026  
**Target Codebase:** `d:\CodingProjects\przm`  
**Parent Orchestrator:** `orchestrator_5` (Conv ID: `1eb7944e-37f1-4b94-a209-66caae11aab3`)  

---

## 1. Review Summary

**Verdict: REQUEST_CHANGES**

Milestone 3 implementation of Cellular Sheaves with Discrete Hodge Laplacians (CS-DHL) includes substantial mathematical modeling and compiles cleanly with strict compiler flags (`gcc -O3 -Wall -Wextra -Werror`). Lean 4 formal verification builds with zero errors (`lake build`).

However, two **Critical** defects and two **Major** defects were identified during empirical verification and adversarial stress-testing, causing `go test -count=1 .` to **FAIL**:
1. **Critical Divergence**: Pure C reference decompressor `przm_cellular_sheaf_decompress` in `src/przm_dec.c` and Go `DecompressCellularSheaf` produce conflicting, corrupted outputs when decompressing frames with Galois twist $\alpha > 1$. The C decompressor fails to advance vertical vector components across columns.
2. **Zero-Allocation Invariant Violation**: In `cellular_sheaf.go`, `DecompressCellularSheaf` invokes `PWSDecode(payload)` when `CSHDFlagCascadedPWS` is active, resulting in 1 heap allocation per decompression call (`1 allocs/op`), directly violating the project-wide `0 B/op, 0 allocs/op` contract.
3. **Full Regression Failure**: `go test -count=1 .` exits with code 1 due to the above two defects.
4. **Sparse Curl Robustness Gap**: In `src/przm_dec.c`, sparse curl deserialization lacks integer overflow protection on 32-bit systems (`pos + (size_t)nnz * 5`) and lacks curl index monotonicity validation.

---

## 2. Findings

### [Critical] Finding 1: Pure C vs Go Decompressor Divergence on Galois Twist $\alpha > 1$

- **What**: When decompressing a Cellular Sheaf frame with `Twist > 1`, Go and Pure C produce different outputs. The Pure C reference decompressor generates corrupt data.
- **Where**: `d:\CodingProjects\przm\src\przm_dec.c`: Lines 1278–1282 and lines 1294–1315.
- **Why**: In `cellular_sheaf.go`, for a cellular sheaf with twist $\alpha$, the vertical boundary vector $v$ on row $i$ satisfies the coboundary restriction:
  $$v(j+1) = \alpha \odot v(j) \oplus C(i, j)$$
  In Go `ReconstructCellularSheaf` (lines 499–503 and lines 538, 546):
  ```go
  curV := v
  for j := 0; j < W; j++ {
      dst[rowCurr+j] = GF256Mul(twist, dst[rowPrev+j]) ^ curV
      curV = GF256Mul(twist, curV)
  }
  ```
  However, in `src/przm_dec.c` (lines 1278–1282):
  ```c
  for (uint16_t j = 0; j < W; j++) {
      dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ v;
  }
  ```
  In C, `v` is never multiplied by `twist` across columns! It remains stuck at `Ev[i-1]`.
  Similarly, in the sparse curl loop (lines 1302–1315), when `twist > 1`, C only executes `v ^= next_c;` without evolving `v` by `gf256_mul(twist, v)`.
- **Empirical Evidence**:
  Running `go test -v -run TestCellularSheaf_TwistGreaterThanOne_GoVsPureC .`:
  ```text
  Go Dec: 01 12 04 38 06 2C 18 50 04 48 10 E0
  C  Dec: 01 12 04 38 06 20 0C 74 04 48 10 E0
  CRITICAL BUG FOUND: Go and Pure C disagree on decompressing Cellular Sheaf frame with twist=2!
  ```
- **Suggestion**: Update `src/przm_dec.c`:
  1. In curl-free mode (`twist > 1`), maintain `cur_v` and update `cur_v = gf256_mul(twist, cur_v)` after each column.
  2. In sparse-curl mode (`twist > 1`), update `cur_v = gf256_mul(twist, cur_v) ^ next_c` when matching, and `cur_v = gf256_mul(twist, cur_v)` when not matching.

---

### [Critical] Finding 2: Zero-Allocation Violation in `DecompressCellularSheaf` with Cascaded PWS

- **What**: `DecompressCellularSheaf` in `cellular_sheaf.go` allocates heap memory during decompression whenever `CSHDFlagCascadedPWS` is set.
- **Where**: `d:\CodingProjects\przm\cellular_sheaf.go`: Lines 813–819.
- **Why**: When `flags & CSHDFlagCascadedPWS` is true:
  ```go
  if (flags & CSHDFlagCascadedPWS) != 0 {
      innerDec, err := PWSDecode(payload)
      if err != nil {
          return nil, err
      }
      payload = innerDec
  }
  ```
  `PWSDecode(payload)` in `pws.go` line 368 instantiates `GrammarH_DAG5` tables (`make([]RuleL1, numL1)`, etc.) and allocates `dst := make([]byte, origSize+32)`.
  This results in `1 allocs/op`, directly violating:
  - DISPATCH R4: "0 heap allocations (`0 B/op, 0 allocs/op`) during decompression"
  - PROJECT.md Feature 12: "Zero-Allocation Verification: Formal test verification of `0 B/op, 0 allocs/op` on all decompressor paths"
- **Empirical Evidence**:
  Running `go test -v -run TestCellularSheaf_ZeroAllocDecompression_AllModes .`:
  ```text
  --- FAIL: TestCellularSheaf_ZeroAllocDecompression_AllModes/SparseCurl_32x32 (0.00s)
      cellular_sheaf_adversarial_test.go:244: SparseCurl_32x32: violated zero-allocation invariant: 1.000000 allocs/op (expected 0)
  --- FAIL: TestCellularSheaf_ZeroAllocDecompression_AllModes/TrailingRemainder_64x64_plus_31 (0.00s)
      cellular_sheaf_adversarial_test.go:244: TrailingRemainder_64x64_plus_31: violated zero-allocation invariant: 1.000000 allocs/op (expected 0)
  ```
- **Suggestion**:
  Either:
  1. Remove PWS cascading from `CompressCellularSheaf` (cellular sheaf boundary representations already collapse entropy; cascading PWS complicates zero-allocation guarantees).
  2. Or provide an in-place / pre-allocated scratchpad decoding variant `PWSDecodeScratchpad(src, scratch []byte)` that does not allocate heap memory.

---

### [Major] Finding 3: Full Regression Suite Failure (`go test -count=1 .`)

- **What**: The project test suite fails when run with `-count=1`.
- **Where**: Package `github.com/lemon4ksan/przm`.
- **Why**: Due to Finding 1 (`TestCellularSheaf_TwistGreaterThanOne_GoVsPureC`) and Finding 2 (`TestCellularSheaf_ZeroAllocDecompression_AllModes`), running `go test -count=1 .` exits with code 1.
- **Empirical Evidence**:
  Command: `go test -count=1 .`
  Output:
  ```text
  --- FAIL: TestCellularSheaf_ZeroAllocDecompression_AllModes (0.00s)
  --- FAIL: TestCellularSheaf_TwistGreaterThanOne_GoVsPureC (0.02s)
  FAIL
  FAIL	github.com/lemon4ksan/przm	29.895s
  ```
- **Suggestion**: Remediate Findings 1 and 2 and ensure `go test -count=1 .` exits with 0.

---

### [Major] Finding 4: Incomplete Verification and Validation of Sparse Curl Input

- **What**: In `src/przm_dec.c` line 1231, integer overflow can occur on 32-bit systems when checking `pos + (size_t)nnz * 5 > payload_len`. Additionally, out-of-order or duplicate curl indices are not validated.
- **Where**: `d:\CodingProjects\przm\src\przm_dec.c`: Lines 1227–1234, lines 1302–1315.
- **Why**:
  1. If `nnz` is large, `(size_t)nnz * 5` can overflow 32-bit `size_t`. The safe check is:
     `if (nnz > (payload_len - pos) / 5) return PRZM_ERR_CORRUPTED_FRAME;`
  2. The sparse curl loop uses `if (curl_idx < nnz && cur_k == next_k)` instead of validating that `next_k` is strictly monotonically increasing and bounded by $(M-1)(W-1)-1$. Malformed frames could cause erratic decode behavior.
- **Suggestion**: Add strict monotonic index validation and safe bounds checking in both Go and C decoders.

---

## 3. Verified Claims

| Claim from Worker M3 / Dispatch | Verification Method | Result | Notes |
|---|---|---|---|
| Clean GCC build with strict flags | `gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe` | **PASS** | Exited 0 with zero warnings. |
| Pure C test suite pass | `.\test_przm_dec.exe` | **PASS** | All 10 tests passed including `cellular_sheaf_decompression`. |
| Lean 4 Formal Build | `lake build` in `d:\CodingProjects\przm` | **PASS** | Exited 0, "Build completed successfully." |
| Go unit tests in `cellular_sheaf_test.go` | `go test -v -run TestCellularSheaf_CoboundaryNilpotency .` | **PASS** | Nilpotency verified. |
| 157.5x compression ratio on harmonic 2D field | `go test -v -run TestCellularSheaf_CompressionRatioAdvantage .` | **PASS** | Measured 157.54x ratio (4096B -> 26B). |
| Standard Go $\leftrightarrow$ C interop (twist=1) | `go test -v -run TestCellularSheaf_PureCDecompressorInterop .` | **PASS** | All 3 standard test grids pass with twist=1. |
| Zero-allocation decompression on all modes | `go test -v -run TestCellularSheaf_ZeroAllocDecompression_AllModes .` | **FAIL** | Failed on `SparseCurl_32x32` and `TrailingRemainder` due to PWS cascade. |
| Pure C $\leftrightarrow$ Go interop on twist $\alpha > 1$ | `go test -v -run TestCellularSheaf_TwistGreaterThanOne_GoVsPureC .` | **FAIL** | C decompressor diverges and produces corrupted output. |
| Full package regression | `go test -count=1 .` | **FAIL** | Failed due to zero-allocation and twist>1 divergence. |

---

## 4. Adversarial Challenge Summary

- **Overall Risk Assessment**: **HIGH**
- **Challenged Assumptions**:
  1. *Assumption*: Pure C reference decompressor `przm_cellular_sheaf_decompress` correctly decodes any valid CSHD frame.  
     *Result*: **FALSIFIED**. When `twist > 1`, C fails to propagate Galois multiplication on vertical vector components, resulting in bitstream corruption.
  2. *Assumption*: `DecompressCellularSheaf` adheres to the zero-heap allocation contract across all compression paths.  
     *Result*: **FALSIFIED**. Cascaded PWS frames trigger `PWSDecode`, causing heap allocations.
  3. *Assumption*: C parser safely bounds sparse curl indices.  
     *Result*: **VULNERABLE** to integer overflow on 32-bit systems and unvalidated non-monotonic curl indices.

---

## 5. Gate Verdict

**REQUEST_CHANGES**

Milestone 3 cannot be approved until:
1. `src/przm_dec.c` is updated to correctly evolve vertical 1-forms with `twist` in curl-free and sparse-curl loops.
2. `cellular_sheaf.go` decompression is made strictly zero-allocation (0 allocs/op) across all modes, including when PWS cascade is evaluated.
3. `go test -count=1 .` executes completely cleanly with exit code 0.
