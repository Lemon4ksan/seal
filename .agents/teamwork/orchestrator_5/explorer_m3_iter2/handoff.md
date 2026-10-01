# Handoff Report: Milestone 3 Iteration 2 Remediation Plan

**Agent:** `explorer_m3_iter2`  
**Working Directory:** `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\explorer_m3_iter2`  
**Target Codebase:** `d:\CodingProjects\przm`  
**Date:** October 1, 2026  
**Type:** Hard Handoff (Investigation & Remediation Blueprint Complete)  
**Parent Conversation ID:** `1eb7944e-37f1-4b94-a209-66caae11aab3`  

---

## 1. Observation

1. **Adjoint0 Formal Adjoint Mismatch (`cellular_sheaf.go`, lines 184–208):**
   - Command: `go test -v -run TestAdversarial_HodgeDecomposition_AdjointProperty .`
   - Verbatim output:
     ```text
     === RUN   TestAdversarial_HodgeDecomposition_AdjointProperty
         cellular_sheaf_adversarial_test.go:187: Adjoint0 mismatch: M=2, W=2, Twist=0x02: <d0 phi, w>=0x4D != <phi, d0^T w>=0xE5
         cellular_sheaf_adversarial_test.go:221: Adjoint property summary: przm.Adjoint0 failures = 836, ReferenceAdjoint0 failures = 0, Adjoint1 failures = 0
         cellular_sheaf_adversarial_test.go:233: [BUG CONFIRMED] przm.Adjoint0 has inverted incoming/outgoing Galois multipliers for twist > 1! Failures: 836 (im(d0) ⟂ ker(d0^T) violated)
     --- FAIL: TestAdversarial_HodgeDecomposition_AdjointProperty (0.00s)
     ```
   - Inspection of `cellular_sheaf.go:185-208` reveals outgoing edges (`u.Eh[hRow+j]`, `u.Ev[i*W+j]`) are unscaled, while incoming edges (`u.Eh[hRow+j-1]`, `u.Ev[(i-1)*W+j]`) are multiplied by `alpha`.

2. **Pure C Decompressor Data Corruption on Twist $\alpha > 1$ (`src/przm_dec.c`, lines 1278–1282, 1302–1314):**
   - Command: `go test -v -run TestAdversarial_PureC_TwistCompatibility .`
   - Verbatim output:
     ```text
     --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist2_Harmonic (0.02s)
         cellular_sheaf_adversarial_test.go:564: [BUG CONFIRMED] Pure C decompressor produces bit-level corruption on Twist2_Harmonic (twist=2)!
             Pure C does not perform Galois vector scaling during row sweep!
     --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist3_Harmonic (0.02s)
     --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist7_Harmonic (0.02s)
     --- FAIL: TestAdversarial_PureC_TwistCompatibility/Twist2_SparseCurl (0.02s)
     ```
   - Command: `go test -v -run TestMilestone3_Twist2_NonZeroEv_EndToEndInterop .`
   - Verbatim output:
     ```text
     PURE C FAILED TO DECOMPRESS TWIST=2 FIELD LOSSLESSLY!
         Orig: 03 06 0C 18 30 60 C0 9B 03 06 0C 18 30 60 C0 9B
         Go:   03 06 0C 18 30 60 C0 9B 03 06 0C 18 30 60 C0 9B
         C:    03 06 0C 18 30 60 C0 9B 03 09 1D 35 65 C5 9E 28
     ```
   - Inspection of `src/przm_dec.c:1279-1282` shows `dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ v;` where `v` is never scaled across columns by `twist`.

3. **Zero-Allocation Invariant Breach on Cascaded Frames (`cellular_sheaf.go`, lines 753–766, 813–820):**
   - Command: `go test -v -run TestAdversarial_ZeroAllocDecompression_AllVariants .`
   - Verbatim output:
     ```text
     === RUN   TestAdversarial_ZeroAllocDecompression_AllVariants
         cellular_sheaf_adversarial_test.go:769: SparseCurl_16x16 violated zero-allocation contract: 1.000000 allocs/op
     --- FAIL: TestAdversarial_ZeroAllocDecompression_AllVariants (0.00s)
     ```
   - Inspection shows `CompressCellularSheaf` sets `CSHDFlagCascadedPWS` via `PWSEncode(payload)`, and `DecompressCellularSheaf` calls `PWSDecode(payload)` which executes `dst := make([]byte, origSize+32)` on the heap.

4. **Facade Harmonic Projection & Dead Hodge Laplacians (`cellular_sheaf.go`, lines 280–347):**
   - `ProjectHarmonic1` only tests `u.Eh[j] == u.Eh[0]` and `u.Ev[i*W] == u.Ev[0]`, performing no cycle annihilation or projection against $\ker \Delta_1$.
   - `HodgeLaplacian0` and `HodgeLaplacian1` are uncalled dead code not invoked anywhere in the codec pipeline.
   - `TestCellularSheaf_HodgeLaplacian_HarmonicCharacterization` in `cellular_sheaf_test.go:101` only asserts `len(delta0) != M*W`.

5. **Full Repository Regression Scope:**
   - Command: `go test -count=1 .` in `d:\CodingProjects\przm`
   - Result: Failed with exit code 1 after 32.439s. The only failing tests are the 6 adversarial/M3 tests corresponding to the 4 defects above; all previous milestones (Hensel lifting, Heisenberg group, Knuth-Bendix, Tropical scans, IsoQuant, Trellis) pass completely.

---

## 2. Logic Chain

1. From Observation 1, `Coboundary0` defines $(d_0 \phi)(e) = \phi(v_1) \oplus \alpha \odot \phi(v_0)$. Under the canonical inner product $\langle u, w \rangle = \bigoplus u_e \odot w_e$, the formal adjoint $d_0^T$ (matrix transpose) must place $\alpha$ on the source vertex (outgoing edges) and $1$ on the target vertex (incoming edges). Swapping them in `Adjoint0` breaks $\langle d_0 \phi, w \rangle \equiv \langle \phi, d_0^T w \rangle$ for all $\alpha > 1$. Therefore, weighting outgoing edges by $\alpha$ and incoming edges by $1$ is mathematically necessary and sufficient to restore the adjoint property and Hodge orthogonality $\mathrm{im}(d_0) \perp \ker(d_0^T)$.
2. From Observation 2, under twist $\alpha$, discrete 1-coboundary nilpotency requires vertical increments across columns to evolve geometrically as $v_{j+1} = \alpha \odot v_j \oplus C(i, j)$. Go implements this via `curV = GF256Mul(twist, curV)`, but Pure C uses static $v$. Therefore, updating `cur_v = gf256_mul(twist, cur_v)` in C across columns in both curl-free and sparse-curl paths is necessary and sufficient to achieve bit-exact Go $\leftrightarrow$ C parity.
3. From Observation 3, `PWSDecode` dynamically allocates slice headers and grammar rule arrays on the Go runtime heap. Cascading PWS on boundary representations (which are already compact, $\sim 20$–$50$ bytes) saves negligible bytes while breaking the mandatory 0 allocs/op decompression invariant. Therefore, emitting uncascaded boundary frames in `CompressCellularSheaf` while supporting cascaded frames via a stack buffer in `DecompressCellularSheaf` restores the zero-allocation invariant (`0 B/op, 0 allocs/op`) with 100% backward compatibility.
4. From Observation 4, `ProjectHarmonic1` and Hodge Laplacians must be grounded in genuine combinatorial Hodge theory. Implementing true harmonic projection onto $\ker(\Delta_1) = \ker(d_1) \cap \ker(d_0^T)$, harmonic cycle annihilation, and the 3-way orthogonal decomposition $u = u_{\text{exact}} \oplus u_{\text{coexact}} \oplus u_{\text{harmonic}}$ replaces the facade and completes the mathematical model.
5. From Observation 5, resolving these 4 defects will yield a 100% clean test pass (`go test -count=1 .` exit 0).

---

## 3. Caveats

1. **Cellular Sheaf Candidate Search Strides**: Candidate search in `CompressCellularSheaf` is restricted to $W \in \{4, 8, 16, 32, 64\}$ to match CPU L1 cache lines; arbitrary non-power-of-two widths are decoded cleanly by the decompressor but not emitted by the default compressor.
2. **Lean 4 Proofs**: Formal verification in `PRZM/CellularSheaf.lean` is officially designated for Milestone 4; current Lean 4 build (`lake build`) is clean.

---

## 4. Conclusion

The mathematical derivations, root-cause analyses, and exact drop-in code remediations have been completely formulated and documented in:
`d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\explorer_m3_iter2\report.md`

`worker_m3` can execute the remediation in 5 discrete steps:
1. Update `Adjoint0` in `cellular_sheaf.go`.
2. Update `przm_cellular_sheaf_decompress` in `src/przm_dec.c` and augment `tests/test_przm_dec.c`.
3. Eliminate PWS cascading in `CompressCellularSheaf` and decode via stack buffer in `DecompressCellularSheaf`.
4. Replace facade `ProjectHarmonic1` with authentic Hodge Laplacian projection, cycle annihilation, and 3-way decomposition.
5. Recompile C binary and run full regression suite `go test -count=1 .`.

---

## 5. Verification Method

To independently verify the complete remediation:

1. **Adjoint0 Verification:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestAdversarial_HodgeDecomposition_AdjointProperty .
   ```
   *Expected Output*: `przm.Adjoint0 failures = 0`, Test PASS.

2. **Pure C Reference Decompressor Twist Verification:**
   ```powershell
   cd d:\CodingProjects\przm
   gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe
   .\test_przm_dec.exe
   go test -v -run TestAdversarial_PureC_TwistCompatibility .
   go test -v -run TestMilestone3_Twist2_NonZeroEv_EndToEndInterop .
   ```
   *Expected Output*: All tests PASS with bit-exact Go $\leftrightarrow$ C match across all twists.

3. **Zero-Allocation Decompression Invariant Verification:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestAdversarial_ZeroAllocDecompression_AllVariants .
   ```
   *Expected Output*: `0.000000 allocs/op`, Test PASS.

4. **Authentic Hodge Laplacian & Harmonic Characterization:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestCellularSheaf_HodgeLaplacian_RigorousProperties .
   ```
   *Expected Output*: Test PASS.

5. **Full Repository Regression:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -count=1 .
   ```
   *Expected Output*: `PASS` across all packages with exit code 0.

6. **Invalidation Condition:**
   Any non-zero allocation count, any Go $\leftrightarrow$ C byte divergence on $\alpha > 1$, or any mismatch in $\langle d_0 \phi, w \rangle \equiv \langle \phi, d_0^T w \rangle$ immediately invalidates the fix.
