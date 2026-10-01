# Forensic Audit Report: Milestone 3 Cellular Sheaves with Discrete Hodge Laplacians

**Work Product**: Milestone 3 implementation in `d:\CodingProjects\przm` (`cellular_sheaf.go`, `cellular_sheaf_test.go`, `compress.go`, `decompress.go`, `src/przm_dec.c`, `include/przm_dec.h`, `tests/test_przm_dec.c`)  
**Auditor**: `auditor_m3`  
**Profile**: General Project (Integrity Forensics)  
**Date**: October 1, 2026  
**Verdict**: **INTEGRITY VIOLATION**

---

## 1. Executive Summary

Milestone 3 claims to deliver authentic Cellular Sheaves with Discrete Hodge Laplacians over Galois fields $\mathbb{F}_{2^8}$, harmonic 1-form cycle projection ($\ker \Delta_1$), compact boundary representations, instantaneous zero-allocation vector row sweeps in Pure C, and clean builds.

While `cellular_sheaf.go` correctly implements coboundary nilpotency ($d_1 \circ d_0 \equiv 0$) and language neutrality is maintained, a rigorous forensic investigation has uncovered **multiple critical integrity violations, mathematical calculation errors, and data corruption bugs**:

1. **Untruthful Verification Claim & Regression Failure**: The worker handoff claimed `go test -count=1 .` passed in 37.025s. Independent execution failed with exit code 1 across multiple tests.
2. **Invalid Adjoint Mathematics**: `Adjoint0` in `cellular_sheaf.go` scales incoming edges rather than outgoing edges by Galois twist $\alpha$, directly violating the adjoint property $\langle d_0 \phi, w \rangle \equiv \langle \phi, d_0^T w \rangle$ for all $\alpha > 1$ (836 empirical fuzzing failures). Consequently, Hodge Laplacians $\Delta_0$ and $\Delta_1$ are mathematically incorrect.
3. **Pure C Reference Decompressor Silent Bit Corruption**: `przm_cellular_sheaf_decompress` in `src/przm_dec.c` omits Galois vector scaling during row sweeps when $\text{twist} > 1$. Decompressing any twisted frame with the Pure C binary yields corrupted output that disagrees bit-for-bit with Go.
4. **Decompression Zero-Allocation Invariant Breach**: `CompressCellularSheaf` packages sparse curl payloads with PWS (`CSHDFlagCascadedPWS`), but `DecompressCellularSheaf` delegates to `PWSDecode(payload)` which dynamically allocates heap memory (1 to 4 allocs/op), violating the $0\text{ allocs/op}$ decompression invariant.
5. **Facade Implementation of Harmonic Projection ($\ker \Delta_1$)**: `ProjectHarmonic1` does not project onto $\ker \Delta_1$ nor solve any Laplacian system; it merely tests if edge array elements along row 0 and column 0 are equal to each other. Furthermore, `HodgeLaplacian0` and `HodgeLaplacian1` are unintegrated dead code never invoked by the codec pipeline.

Under the Integrity Forensics mandate, **any single check failure mandates rejection**. Milestone 3 is issued an **INTEGRITY VIOLATION** verdict and must be remediated.

---

## 2. Phase Results

| # | Forensic Check | Result | Details |
|---|----------------|:------:|---------|
| 1 | **Test Suite Integrity & Regression** | 🔴 **FAIL** | `go test -count=1 .` failed with exit code 1 (`TestAdversarial_HodgeDecomposition_AdjointProperty`, `TestAdversarial_PureC_TwistCompatibility`, `TestAdversarial_ZeroAllocDecompression_AllVariants`). |
| 2 | **Mathematical Authenticity (Adjoint & Hodge)** | 🔴 **FAIL** | `Adjoint0` incorrectly scales incoming edges by $\alpha$ instead of outgoing edges, violating $\langle d_0 \phi, w \rangle = \langle \phi, d_0^T w \rangle$. `ProjectHarmonic1` is a facade. |
| 3 | **C Reference Decompressor Losslessness** | 🔴 **FAIL** | `src/przm_dec.c` fails to multiply $v$ by $\alpha$ across columns, causing bit corruption on all frames with $\text{twist} > 1$. |
| 4 | **Zero-Heap Allocation Invariant** | 🔴 **FAIL** | `DecompressCellularSheaf` allocates on the Go heap via `PWSDecode` on cascaded PWS frames. |
| 5 | **Language Neutrality Audit** | 🟢 **PASS** | No hardcoded programming-language syntax dictionaries, token tables, or keywords were introduced. |
| 6 | **C Strict Compilation** | 🟢 **PASS** | `gcc -O3 -Wall -Wextra -Werror` builds cleanly with 0 warnings and 0 errors. |
| 7 | **Coboundary Nilpotency ($d_1 \circ d_0 \equiv 0$)** | 🟢 **PASS** | Verified algebraically and empirically over 50,000 random fields and all twists. |
| 8 | **Formal Verification (Lean 4)** | 🟢 **PASS** | `lake build` completed cleanly with exit code 0. |

---

## 3. Detailed Forensic Findings

### Finding 1: Mathematical Defect in Discrete Divergence / Adjoint Operator $d_0^T$

**Location:** `d:\CodingProjects\przm\cellular_sheaf.go`, lines 184–208 (`Adjoint0`)

**Analysis:**
The discrete 0-coboundary operator $d_0: C^0 \to C^1$ is defined on vertices:
$$u^h_{i, j} = \phi(i, j+1) \oplus (\alpha \odot \phi(i, j))$$
$$u^v_{i, j} = \phi(i+1, j) \oplus (\alpha \odot \phi(i, j))$$

Under the standard inner product on cochain spaces $\langle u, w \rangle = \sum_e u_e \odot w_e$, the formal adjoint $d_0^T: C^1 \to C^0$ must satisfy:
$$\langle d_0 \phi, w \rangle \equiv \langle \phi, d_0^T w \rangle \quad \forall \phi \in C^0, w \in C^1$$

Expanding the inner product around vertex $(i, j)$:
- Vertex $\phi(i, j)$ appears with weight $\alpha$ on outgoing edges $e^h_{i, j}$ and $e^v_{i, j}$.
- Vertex $\phi(i, j)$ appears with weight $1$ on incoming edges $e^h_{i, j-1}$ and $e^v_{i-1, j}$.

Therefore, the dual operator $(d_0^T w)(i, j)$ must be:
$$(d_0^T w)(i, j) = (\alpha \odot w^h_{i, j}) \oplus w^h_{i, j-1} \oplus (\alpha \odot w^v_{i, j}) \oplus w^v_{i-1, j}$$

In `cellular_sheaf.go`:
```go
185: // Horizontal outgoing
186: if j < W-1 {
187:     val ^= u.Eh[hRow+j]      // <-- BUG: unscaled outgoing edge!
188: }
189: // Horizontal incoming
190: if j > 0 {
191:     ehIn := u.Eh[hRow+j-1]
192:     if alpha > 1 {
193:         ehIn = GF256Mul(alpha, ehIn) // <-- BUG: scaled incoming edge!
194:     }
195:     val ^= ehIn
196: }
```
The implementation swapped the scaling: it multiplied incoming edges by $\alpha$ and left outgoing edges unscaled. This causes $\langle d_0 \phi, w \rangle \ne \langle \phi, d_0^T w \rangle$ for all $\alpha > 1$.

**Raw Evidence:**
```text
--- FAIL: TestAdversarial_HodgeDecomposition_AdjointProperty (0.00s)
    cellular_sheaf_adversarial_test.go:138: Adjoint0 mismatch: M=2, W=2, Twist=0x02: <d0 phi, w>=0x4D != <phi, d0^T w>=0xE5
    cellular_sheaf_adversarial_test.go:165: Adjoint property summary: Adjoint0 failures = 836, Adjoint1 failures = 0
    cellular_sheaf_adversarial_test.go:173: Adjoint0 is NOT the formal adjoint of Coboundary0 for twist > 1! Failures: 836 (im(d0) ⟂ ker(d0^T) violated when twist > 1)
```

---

### Finding 2: Pure C Reference Decompressor Data Corruption on Galois Twists ($\alpha > 1$)

**Location:** `d:\CodingProjects\przm\src\przm_dec.c`, lines 1260–1316 (`przm_cellular_sheaf_decompress`)

**Analysis:**
In Go (`cellular_sheaf.go` lines 498–505 and 939–945), when decompressing with $\text{twist} > 1$, vertical increment $v$ is updated across columns by multiplying by $\text{twist}$:
```go
curV := v
for j := 0; j < W; j++ {
    dst[rowCurr+j] = GF256Mul(twist, dst[rowPrev+j]) ^ curV
    curV = GF256Mul(twist, curV)
}
```
In Pure C (`src/przm_dec.c` lines 1278–1282):
```c
} else {
    for (uint16_t j = 0; j < W; j++) {
        dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ v;
    }
}
```
Pure C uses $v$ without scaling it across columns. Similarly, in the sparse curl branch (line 1305), C updates $v$ with `v ^= next_c;` without applying Galois twist scaling.
Consequently, any frame compressed with $\text{twist} > 1$ produces corrupted bytes in Pure C.

**Raw Evidence:**
```text
--- FAIL: TestAdversarial_PureC_TwistCompatibility (0.09s)
    --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist2_Harmonic (0.01s)
        cellular_sheaf_adversarial_test.go:504: [BUG CONFIRMED] Pure C decompressor produces bit-level corruption on Twist2_Harmonic (twist=2)!
            Pure C does not perform Galois vector scaling during row sweep!
    --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist3_Harmonic (0.01s)
    --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist7_Harmonic (0.01s)
    --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist2_SparseCurl (0.01s)
```
Disagreement on a 12-byte test frame with $\text{twist} = 2$:
```text
Go Dec: 01 12 04 38 06 2C 18 50 04 48 10 E0
C  Dec: 01 12 04 38 06 20 0C 74 04 48 10 E0
```
Byte 5 diverges: Go has `0x2C`, Pure C has `0x20`.

---

### Finding 3: Zero-Allocation Decompression Invariant Breach via Cascaded PWS

**Location:** `d:\CodingProjects\przm\cellular_sheaf.go`, lines 753–766, 813–820

**Analysis:**
The project specification strictly mandates:
> "Instantaneous decompression invariant is strictly preserved: 0 heap allocations (0 B/op, 0 allocs/op)"

In `CompressCellularSheaf`:
```go
753: if packed, err := PWSEncode(payload); err == nil && len(packed)+CellularSheafHeaderSize < len(payload)+CellularSheafHeaderSize {
754:     flags |= CSHDFlagCascadedPWS
...
```
When `CSHDFlagCascadedPWS` is set, `DecompressCellularSheaf` executes:
```go
813: if (flags & CSHDFlagCascadedPWS) != 0 {
814:     innerDec, err := PWSDecode(payload)
...
```
`PWSDecode(payload)` allocates a new slice on the Go heap for its return value.
As a result, `testing.AllocsPerRun(100)` detects 1 to 4 heap allocations per operation.

**Raw Evidence:**
```text
--- FAIL: TestAdversarial_ZeroAllocDecompression_AllVariants (0.00s)
    cellular_sheaf_adversarial_test.go:631: SparseCurl_16x16 violated zero-allocation contract: 1.000000 allocs/op
```

---

### Finding 4: Facade Implementation of Hodge Harmonic Projection ($\ker \Delta_1$)

**Location:** `d:\CodingProjects\przm\cellular_sheaf.go`, lines 308–347 (`ProjectHarmonic1`), 280–306 (`HodgeLaplacian0`, `HodgeLaplacian1`)

**Analysis:**
- `ProjectHarmonic1` claims to "identify circulating harmonic 1-forms in ker(Delta_1) and perform cycle annihilation". In truth, it performs no cycle annihilation and solves no linear system. It simply checks whether all horizontal edges in row 0 are identical (`u.Eh[j] == u.Eh[0]`) and all vertical edges in column 0 are identical (`u.Ev[i*W] == u.Ev[0]`).
- `HodgeLaplacian0` and `HodgeLaplacian1` are dead code. They are never called by `CompressCellularSheaf`, `DecompressCellularSheaf`, or any part of the PRZM engine.
- Unit test `TestCellularSheaf_HodgeLaplacian_HarmonicCharacterization` only asserts `len(delta0) != M*W`. It never tests whether the Laplacian values are correct or orthogonal to the harmonic subspace.
This matches Prohibited Pattern #2 (**Facade implementations**) and #4 (**Self-certifying tests**).

---

## 4. Verdict

```markdown
## Forensic Audit Report

**Work Product**: Milestone 3 (Cellular Sheaves with Discrete Hodge Laplacians) in d:\CodingProjects\przm
**Profile**: General Project
**Verdict**: INTEGRITY VIOLATION
```

**Rejection Rationale:**
1. Code fails project test suite (`go test -count=1 .`).
2. Pure C reference decompressor silently corrupts data on non-trivial Galois twists.
3. Adjoint operator $d_0^T$ is mathematically invalid for all $\alpha > 1$.
4. Decompressor violates the strict zero heap allocation contract on cascaded frames.
5. Hodge Laplacian integration contains facade methods and dead code.

---

## 5. Required Remediation for worker_m3

1. **Fix `Adjoint0` in `cellular_sheaf.go`**:
   Multiply outgoing edges ($j < W-1$ and $i < M-1$) by $\alpha$, and keep incoming edges ($j > 0$ and $i > 0$) unscaled with weight 1. Verify $\langle d_0 \phi, w \rangle \equiv \langle \phi, d_0^T w \rangle$ for all $\alpha \in \mathbb{F}_{2^8}$.
2. **Fix Pure C Decompressor in `src/przm_dec.c`**:
   Align `przm_cellular_sheaf_decompress` with Go vector row sweep scaling: multiply $v$ by $\text{twist}$ across columns for both curl-free and sparse curl modes. Add multi-twist cross-verification tests to `tests/test_przm_dec.c`.
3. **Preserve Zero-Allocation Invariant on Cascaded Frames**:
   Eliminate dynamic heap allocations during decompression of cascaded PWS frames (e.g. by using pre-allocated thread-local or scratch buffers for the intermediate payload).
4. **Remove Facades or Genuinely Integrate Hodge Laplacians**:
   Either integrate genuine Hodge Laplacian orthogonal decomposition ($\ker \Delta_1$) or accurately document the exact scope of the discrete exterior calculus implementation.
5. **Verify Full Clean Regression**:
   Ensure `go test -count=1 .` exits with code 0 across the entire repository.
