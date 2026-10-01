# Milestone 3 Adversarial Challenge Report: Cellular Sheaves & Discrete Hodge Laplacians

**Date:** October 1, 2026  
**Challenger:** `challenger_m3_1` (Milestone 3 Cellular Sheaf Boundary & Nilpotency Challenger 1)  
**Target Codebase:** `d:\CodingProjects\przm`  
**Verdict:** **`REQUEST_CHANGES`**  

---

## Challenge Summary

**Overall risk assessment**: **HIGH**

While the Cellular Sheaf coboundary nilpotency ($d_1 \circ d_0 \equiv 0$) holds rigorously across 50,000 fuzzed iterations and small/unaligned buffer handling is robust, empirical stress-testing has identified two critical mathematical and interoperability flaws and one invariant defect:

1. **[CRITICAL] Pure C Reference Decompressor Data Corruption on Galois Twists $\alpha > 1$**:  
   In `src/przm_dec.c` (`przm_cellular_sheaf_decompress`), the vector row-sweep logic treats vertical edges as static constants across columns, completely omitting the required Galois field scaling $cur\_v = gf256\_mul(twist, cur\_v)$. When the Go compressor encounters structured or geometric data with dominant multiplicative ratios (e.g. ratio 2) and naturally emits a frame with `GaloisTwist = 2` (achieving 9.14x compression), the Pure C reference decompressor silently outputs corrupted bytes.
2. **[HIGH] Coboundary Adjoint Inversion & Hodge Orthogonality Violation**:  
   In `cellular_sheaf.go` (`Adjoint0`), the incoming and outgoing edge multipliers for Galois twist $\alpha > 1$ are inverted. For any edge $e = (v_0, v_1)$, `Coboundary0` defines $u_e = \phi(v_1) \oplus \alpha \phi(v_0)$, giving source vertex $v_0$ coefficient $\alpha$ and target vertex $v_1$ coefficient $1$. The formal adjoint $d_0^T$ (matrix transpose) must weight outgoing edges by $\alpha$ and incoming edges by $1$. `Adjoint0` incorrectly weights outgoing edges by $1$ and incoming edges by $\alpha$. This breaks the adjoint property $\langle d_0 \phi, w \rangle = \langle \phi, d_0^T w \rangle$ (836 failures out of 864 trials), invalidating the Hodge decomposition orthogonality $\mathrm{im}(d_0) \perp \ker(d_0^T)$ and distorting the Hodge Laplacians $\Delta_0$ and $\Delta_1$.
3. **[MEDIUM] Zero-Allocation Decompression Contract Violation on Cascaded PWS**:  
   When the encoder compresses sparse face curls and cascades with PWS (`CSHDFlagCascadedPWS`), `DecompressCellularSheaf` delegates to `PWSDecode(payload)`, which allocates a destination buffer on the heap (`make([]byte, origSize+32)` in `pws.go:419`). Adversarial benchmarking on `SparseCurl_16x16` measured 1.000000 allocs/op, violating the strict `0 B/op, 0 allocs/op` contract.

---

## Challenges

### [Critical] Challenge 1: Pure C Reference Decompressor Fails on Galois Twists $\alpha > 1$

- **Assumption Challenged**: Worker M3 claimed full bit-exact interoperability between Go encoder/decoder and Pure C decompressor `przm_cellular_sheaf_decompress` in `src/przm_dec.c`.
- **Attack Scenario**:
  1. Go `CompressCellularSheaf` autonomously discovers dominant multiplicative ratios in input data (lines 611–634 of `cellular_sheaf.go`) when `level >= 3` and $n \ge 64$. For geometric progressions in $\mathbb{F}_{2^8}$ (e.g. $x_{i+1} = 2 \cdot x_i$), it selects `twist = 2`, compressing 256 bytes down to 28 bytes with `CSHDFlagCurlFree` and `GaloisTwist = 2`.
  2. In `cellular_sheaf.go` (lines 499–504), Go decompressor performs the row sweep:
     ```go
     curV := v
     for j := 0; j < W; j++ {
         dst[rowCurr+j] = GF256Mul(twist, dst[rowPrev+j]) ^ curV
         curV = GF256Mul(twist, curV)
     }
     ```
  3. In `src/przm_dec.c` (lines 1279–1282 and 1302–1314), Pure C decompressor performs:
     ```c
     for (uint16_t j = 0; j < W; j++) {
         dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ v;
     }
     ```
     `v` is never scaled by `twist` across columns $j$.
  4. Passing this valid 28-byte frame to `test_przm_dec.exe` results in decompression data mismatch starting at column 1 of row 1.
- **Blast Radius**: Bit-level corruption and decompressor desynchronization on any Cellular Sheaf frame encoded with `twist > 1`.
- **Mitigation**:
  In `src/przm_dec.c`:
  - In the curl-free branch (lines 1279–1282), track `cur_v = v;` and update `cur_v = gf256_mul(twist, cur_v);` after each column:
    ```c
    uint8_t cur_v = v;
    for (uint16_t j = 0; j < W; j++) {
        dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ cur_v;
        cur_v = gf256_mul(twist, cur_v);
    }
    ```
  - In the sparse curl branch (lines 1302–1314), when `twist > 1`, update `cur_v` with `cur_v = gf256_mul(twist, cur_v)` at each step, matching Go's `cellular_sheaf.go:538,546`.

---

### [High] Challenge 2: `Adjoint0` Coefficient Inversion Violates Hodge Decomposition Orthogonality

- **Assumption Challenged**: Worker M3 claimed formal verification of Hodge decomposition orthogonality $\mathrm{im}(d_0) \perp \ker(d_0^T)$ and correct discrete divergence `Adjoint0`.
- **Attack Scenario**:
  1. `Coboundary0` defines $u^h_{i, j} = \phi(i, j+1) \oplus \alpha \phi(i, j)$. For horizontal edge $e = [(i, j), (i, j+1)]$, the matrix entry for source vertex $(i, j)$ is $\alpha$, and target vertex $(i, j+1)$ is $1$.
  2. The formal adjoint $d_0^T$ with respect to the standard bilinear pairing $\langle u, v \rangle = \sum_e u_e \cdot v_e$ is represented by the matrix transpose $A^T$. For vertex $v$, $(d_0^T u)_v$ must collect:
     $$(d_0^T u)_v = \sum_{e \text{ into } v} u_e \oplus \alpha \sum_{e \text{ out of } v} u_e$$
  3. In `cellular_sheaf.go` lines 185–208:
     ```go
     // Horizontal outgoing
     if j < W-1 { val ^= u.Eh[hRow+j] }
     // Horizontal incoming
     if j > 0 {
         ehIn := u.Eh[hRow+j-1]
         if alpha > 1 { ehIn = GF256Mul(alpha, ehIn) }
         val ^= ehIn
     }
     ```
     `Adjoint0` incorrectly multiplied incoming edges by $\alpha$ and outgoing edges by $1$.
  4. In `TestAdversarial_HodgeDecomposition_AdjointProperty`, evaluating $\langle d_0 \phi, w \rangle \stackrel{?}{=} \langle \phi, d_0^T w \rangle$ failed 836 times out of 864 trials across all twists $\alpha > 1$. Meanwhile, `ReferenceAdjoint0` (which correctly scales outgoing edges by $\alpha$) achieved 0 failures (100% pass rate).
- **Blast Radius**: Invalidates formal Hodge decomposition theorems over $\mathbb{F}_{2^8}$ with non-trivial twists, causing $\mathrm{im}(d_0) \not\perp \ker(d_0^T)$ and introducing numerical errors into $\Delta_0 = d_0^T d_0$ and $\Delta_1 = d_1^T d_1 + d_0 d_0^T$.
- **Mitigation**:
  In `cellular_sheaf.go` (`Adjoint0`):
  - Scale horizontal outgoing edges (`u.Eh[hRow+j]`) by `alpha` when `alpha > 1`.
  - Leave horizontal incoming edges (`u.Eh[hRow+j-1]`) unscaled (weight 1).
  - Scale vertical outgoing edges (`u.Ev[i*W+j]`) by `alpha` when `alpha > 1`.
  - Leave vertical incoming edges (`u.Ev[(i-1)*W+j]`) unscaled (weight 1).

---

### [Medium] Challenge 3: Heap Allocations During Cascaded PWS Decompression

- **Assumption Challenged**: Worker M3 claimed zero heap allocations (`0 B/op, 0 allocs/op`) across all decompression paths.
- **Attack Scenario**:
  1. When an input with sparse face curls is compressed, `CompressCellularSheaf` tests if PWS AVX-wave encoding further compresses the boundary payload (lines 752–766). If so, it sets `CSHDFlagCascadedPWS`.
  2. `DecompressCellularSheaf` (lines 813–819) calls `PWSDecode(payload)`.
  3. `PWSDecode` in Go allocates:
     - `dst := make([]byte, origSize+32)` (line 419 of `pws.go`).
     - Grammar rule slices: `make([]RuleL1, numL1)`, etc.
  4. `testing.AllocsPerRun(100)` on `SparseCurl_16x16` measured 1.000000 allocs/op.
- **Blast Radius**: Violates the zero-allocation $O(1)$ instantaneous decompression invariant on cascaded PWS frames in Go.
- **Mitigation**:
  Either:
  - Add an in-place / scratch-buffered `PWSDecodeInto(src, dst []byte)` overload in `pws.go` that operates directly on a stack/TLS buffer (matching C's `tls_lits_scratch`), or
  - Avoid cascading PWS in Go unless a zero-allocation scratchpad is provided.

---

## Stress Test Results

| Test ID | Scenario | Expected Behavior | Actual Behavior | Result |
|---|---|---|---|---|
| **ST-01** | Nilpotency fuzzing ($d_1 \circ d_0 \equiv 0$) over 50,000 random fields $\phi$ across all 256 twists $\alpha \in \mathbb{F}_{2^8}$ and geometries $2\times 2$ to $33\times 33$ | Every face curl is identically 0 | All 50,000 iterations passed with zero non-zero curl faces | **PASS** |
| **ST-02** | Hodge adjoint property $\langle d_0 \phi, w \rangle = \langle \phi, d_0^T w \rangle$ across 864 trials over 9 twists and 6 geometries | 0 mismatches | 836 mismatches in `przm.Adjoint0` (all $\alpha > 1$ failed); 0 mismatches in `ReferenceAdjoint0` | **FAIL** (Bug 2) |
| **ST-03** | Adjoint1 property $\langle d_1 u, c \rangle = \langle u, d_1^T c \rangle$ across 864 trials | 0 mismatches | 0 mismatches | **PASS** |
| **ST-04** | Small & boundary buffers: 0B, 1B, 2B, 3B, 7B, 15B, 16B, 17B, 31B, 32B, 33B, 63B, 64B, 65B | Exact lossless roundtrip via direct sheaf & full PRZM codec | 100% bit-exact roundtrip, clean bypass on $N < 16$ | **PASS** |
| **ST-05** | Prime-sized buffers (37B, 53B, 97B, 127B, 251B, 509B, 1021B, 2053B, 4093B, 8191B) | Exact lossless roundtrip | 100% bit-exact roundtrip | **PASS** |
| **ST-06** | Odd geometries ($3\times 3$, $5\times 5$, $7\times 7$, $9\times 9$, $3\times 7$, $7\times 3$, $5\times 11$, $11\times 5$) with remainders (0B to 7B) | Exact lossless roundtrip | 100% bit-exact roundtrip | **PASS** |
| **ST-07** | Boundary & dense face curls (corner perturbations, checkerboard vorticity, 100% noise) | Exact lossless roundtrip or clean bypass | 100% bit-exact roundtrip | **PASS** |
| **ST-08** | Pure C decompressor interop on $\alpha = 1$ frames (`Twist1_Harmonic`, `Twist1_SparseCurl`) | Bit-exact output matching Go | Exact bit-for-bit match | **PASS** |
| **ST-09** | Pure C decompressor interop on $\alpha > 1$ frames (`Twist2_Harmonic`, `Twist3_Harmonic`, `Twist7_Harmonic`, `Twist2_SparseCurl`) | Bit-exact output matching Go | Output corrupted starting at col 1 of row 1 | **FAIL** (Bug 1) |
| **ST-10** | End-to-end natural twist discovery: geometric progression ($x_{i+1} = 2 \cdot x_i$) compressed by Go and decompressed by Pure C | Pure C outputs exact 256 bytes | Go naturally selects `twist = 2` (28B); Pure C outputs corrupted data | **FAIL** (Bug 1) |
| **ST-11** | Malformed frame fuzzing (truncated headers 0..23B, corrupted magic, oversized origLen, $W \times M > origLen$, NNZ overflow) | Clean errors returned, zero panics or memory corruption | All 5 corruption classes safely rejected with errors | **PASS** |
| **ST-12** | Zero-allocation decompression invariant on harmonic curl-free fields | Strictly 0 allocs/op | 0 allocs/op | **PASS** |
| **ST-13** | Zero-allocation decompression invariant on cascaded PWS frames (`SparseCurl_16x16`) | Strictly 0 allocs/op | Measured 1.000000 allocs/op | **FAIL** (Bug 3) |

---

## Unchallenged Areas

- **Lean 4 Formal Proof Library (`PRZM/CellularSheaf.lean`)**: Scheduled for Milestone 4; formal verification of coboundary nilpotency in Lean 4 will be audited during Milestone 4.
- **Strides $W > 64$**: Encoder candidate search restricts $W \le 64$ by architectural design to align with L1D cache-line granularity.

---

## Final Recommendation

Issue gate verdict: **`REQUEST_CHANGES`**.

The worker must remediate:
1. Update `src/przm_dec.c` in both curl-free and sparse-curl row sweep paths to multiply `cur_v` by `twist` across columns when `twist > 1`.
2. Update `cellular_sheaf.go` (`Adjoint0`) to weight outgoing edges by `alpha` and incoming edges by `1`.
3. Address the heap allocation in `DecompressCellularSheaf` when `CSHDFlagCascadedPWS` is active.
