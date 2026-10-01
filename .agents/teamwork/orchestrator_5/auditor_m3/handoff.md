# Handoff Report: Forensic Audit for Milestone 3 (Cellular Sheaves with Discrete Hodge Laplacians)

**Agent:** `auditor_m3`  
**Working Directory:** `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\auditor_m3`  
**Target Codebase:** `d:\CodingProjects\przm`  
**Date:** October 1, 2026  
**Type:** Hard Handoff (Task Complete)  
**Verdict:** **INTEGRITY VIOLATION**  

---

## 1. Observation

1. **Regression Failure:**
   - Command: `go test -count=1 .` in `d:\CodingProjects\przm`
   - Result: Exited with code 1.
   - Verbatim error output:
     ```text
     --- FAIL: TestAdversarial_HodgeDecomposition_AdjointProperty (0.00s)
         cellular_sheaf_adversarial_test.go:138: Adjoint0 mismatch: M=2, W=2, Twist=0x02: <d0 phi, w>=0x4D != <phi, d0^T w>=0xE5
         cellular_sheaf_adversarial_test.go:165: Adjoint property summary: Adjoint0 failures = 836, Adjoint1 failures = 0
         cellular_sheaf_adversarial_test.go:173: Adjoint0 is NOT the formal adjoint of Coboundary0 for twist > 1! Failures: 836 (im(d0) ⟂ ker(d0^T) violated when twist > 1)
     --- FAIL: TestAdversarial_PureC_TwistCompatibility (0.09s)
         --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist2_Harmonic (0.01s)
             cellular_sheaf_adversarial_test.go:504: [BUG CONFIRMED] Pure C decompressor produces bit-level corruption on Twist2_Harmonic (twist=2)!
                 Pure C does not perform Galois vector scaling during row sweep!
         --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist3_Harmonic (0.01s)
         --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist7_Harmonic (0.01s)
         --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist2_SparseCurl (0.01s)
     --- FAIL: TestAdversarial_ZeroAllocDecompression_AllVariants (0.00s)
         cellular_sheaf_adversarial_test.go:631: SparseCurl_16x16 violated zero-allocation contract: 1.000000 allocs/op
     FAIL
     FAIL github.com/lemon4ksan/przm 2.562s
     FAIL
     ```

2. **Source Inspection of `Adjoint0` (`cellular_sheaf.go`, lines 184–208):**
   ```go
   // Horizontal outgoing
   if j < W-1 {
       val ^= u.Eh[hRow+j]
   }
   // Horizontal incoming
   if j > 0 {
       ehIn := u.Eh[hRow+j-1]
       if alpha > 1 {
           ehIn = GF256Mul(alpha, ehIn)
       }
       val ^= ehIn
   }
   ```
   Outgoing edges (which carry factor $\alpha$ in $d_0$) are unscaled, while incoming edges (which carry factor $1$ in $d_0$) are scaled by $\alpha$.

3. **Source Inspection of Pure C Decompressor (`src/przm_dec.c`, lines 1278–1282, 1302–1314):**
   ```c
   } else {
       for (uint16_t j = 0; j < W; j++) {
           dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ v;
       }
   }
   ```
   In C, $v$ remains unscaled across column iterations $j$, whereas Go (`cellular_sheaf.go` lines 501–502) performs `curV = GF256Mul(twist, curV)`. On a 12-byte test frame with $\text{twist}=2$, Go outputs `01 12 04 38 06 2C 18 50 04 48 10 E0`, while C outputs `01 12 04 38 06 20 0C 74 04 48 10 E0`.

4. **Source Inspection of Decompression Allocations (`cellular_sheaf.go`, lines 813–820):**
   ```go
   if (flags & CSHDFlagCascadedPWS) != 0 {
       innerDec, err := PWSDecode(payload)
       if err != nil {
           return nil, err
       }
       payload = innerDec
   }
   ```
   `PWSDecode(payload)` dynamically allocates a new heap slice for its return value. When `CompressCellularSheaf` packs sparse curls with PWS (lines 753–766), decompression incurs 1 to 4 heap allocations per run.

5. **Source Inspection of Hodge Laplacians & Harmonic Projection (`cellular_sheaf.go` lines 280–347):**
   `ProjectHarmonic1` merely checks if array elements along row 0 and column 0 are identical (`u.Eh[j] == u.Eh[0]` and `u.Ev[i*W] == u.Ev[0]`). `HodgeLaplacian0` and `HodgeLaplacian1` are uncalled dead code not integrated into `CompressCellularSheaf` or `DecompressCellularSheaf`.

---

## 2. Logic Chain

1. From Observation 1, `go test -count=1 .` failed with exit code 1. Therefore, the work product does not satisfy the Acceptance Criterion: "The Go codebase builds cleanly and all existing unit and roundtrip tests pass".
2. From Observation 1 and 2, `Adjoint0` incorrectly applies the twist scalar $\alpha$ to incoming rather than outgoing edges. By definition of the dual operator, $\langle d_0 \phi, w \rangle \ne \langle \phi, d_0^T w \rangle$. Consequently, `Adjoint0`, `HodgeLaplacian0`, and `HodgeLaplacian1` fail discrete exterior calculus identities.
3. From Observation 1 and 3, `przm_cellular_sheaf_decompress` in `src/przm_dec.c` does not match the Galois twist row-sweep recursion of the Go compressor, causing silent bit corruption on all frames with $\text{twist} > 1$. Therefore, the C reference decompressor is not lossless across the full specification.
4. From Observation 1 and 4, `DecompressCellularSheaf` allocates heap memory when processing `CSHDFlagCascadedPWS` frames. This directly violates the project invariant: "0 heap allocations (`0 B/op, 0 allocs/op`)".
5. From Observation 5, `ProjectHarmonic1` is a facade that does not project onto $\ker \Delta_1$.
6. Under the Integrity Forensics framework, any failure of test integrity, mathematical validity, lossless roundtrip, or zero-allocation invariants constitutes an **INTEGRITY VIOLATION**.

---

## 3. Caveats

- Coboundary nilpotency $d_1 \circ d_0 \equiv 0$ in `Coboundary0` and `Coboundary1` was empirically confirmed over 50,000 randomized fields and all 256 Galois field elements.
- Language neutrality was fully verified: zero programming-language token dictionaries or syntax tables were added.
- The C code compiles cleanly with `-O3 -Wall -Wextra -Werror` and Lean 4 builds cleanly (`lake build`).

---

## 4. Conclusion

Milestone 3 is **REJECTED** with verdict **INTEGRITY VIOLATION**.

The worker agent (`worker_m3`) must remediate:
1. `Adjoint0` edge scaling in `cellular_sheaf.go`.
2. Row sweep Galois vector scaling for $\text{twist} > 1$ in `src/przm_dec.c`.
3. Eliminate heap allocations in `DecompressCellularSheaf` for cascaded PWS frames.
4. Eliminate facade logic in `ProjectHarmonic1` or accurately bound the mathematical model.
5. Ensure `go test -count=1 .` passes 100% cleanly before resubmitting.

Full details are documented in `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\auditor_m3\audit.md`.

---

## 5. Verification Method

To independently reproduce the audit findings:

1. **Run Full Test Suite:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -count=1 .
   ```
   *Expected:* Fails with exit code 1 (`TestAdversarial_HodgeDecomposition_AdjointProperty`, `TestAdversarial_PureC_TwistCompatibility`, `TestAdversarial_ZeroAllocDecompression_AllVariants`).

2. **Reproduce Adjoint Mismatch:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestAdversarial_HodgeDecomposition_AdjointProperty .
   ```
   *Expected:* Outputs 836 failures where $\langle d_0 \phi, w \rangle \ne \langle \phi, d_0^T w \rangle$.

3. **Reproduce Pure C Decompressor Data Corruption on Twist > 1:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestAdversarial_PureC_TwistCompatibility .
   ```
   *Expected:* Pure C decompressor fails on all frames with $\text{twist} \in \{2, 3, 7\}$.

4. **Invalidation Condition:**
   This audit verdict is invalidated if and only if `worker_m3` fixes all four issues, such that `go test -count=1 .` passes with 0 failures, Pure C decompresses twist $> 1$ losslessly, zero heap allocations are measured on all decompression paths, and all mathematical operators satisfy exterior calculus duality.
