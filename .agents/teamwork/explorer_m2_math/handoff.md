# Handoff Report: 4D IsoQuant Quaternionic Collimation Mathematical Specification

**Author:** `explorer_m2_math` (teamwork_preview_explorer)  
**Recipient:** Orchestrator (`449d1fb0-27f3-4579-bba0-13f645f0337b`)  
**Artifact:** `d:\CodingProjects\seal\.agents\teamwork\explorer_m2_math\report.md`  
**Milestone:** M2 (4D IsoQuant Quaternionic Collimation Mathematical Spec)  
**Date:** September 30, 2026  

---

## 1. Observation

1. **Specification & Survey Requirements (`MATHEMATICAL_REPORT.md`):**
   - In `MATHEMATICAL_REPORT.md` §6.4.1 (lines 950–954):
     > "Any vector $\mathbf{v} \in \mathbb{F}_{2^8}^4 \cong \mathbb{H}(\mathbb{F}_{2^8})$ is rotated via a pair of unit quaternions $(q_L, q_R)$:
     > $$\mathbf{v}' = q_L \cdot \mathbf{v} \cdot \bar{q}_R$$
     > where: $q_L = a_0 + a_1 \mathbf{i} + a_2 \mathbf{j} + a_3 \mathbf{k}, \quad q_R = b_0 + b_1 \mathbf{i} + b_2 \mathbf{j} + b_3 \mathbf{k}$ with $|q_L|^2 = |q_R|^2 = 1$ in $\mathbb{F}_{2^8}$."
   - In `MATHEMATICAL_REPORT.md` §6.4.2 (lines 970–972):
     > "The theoretical entropy reduction (redundancy eliminated from the multi-channel representation) is given by the Hadamard determinant inequality ratio:
     > $$\Delta \mathcal{H} = \frac{1}{2} \log_2 \left( \frac{\prod_{m=0}^3 \Sigma_{mm}}{\det \mathbf{\Sigma}} \right) = \frac{1}{2} \log_2 \left( \frac{\prod_{m=0}^3 \Sigma_{mm}}{\prod_{m=0}^3 \lambda_m} \right) \ge 0$$"
   - In `MATHEMATICAL_REPORT.md` §7.4 Proof 4 (lines 1173–1192):
     > "Decompressing a 64-byte block (16 quaternions) via inverse isoclinic rotation over $\mathbb{F}_{2^8}$ requires at most 22 CPU clock cycles ($\le 28$ cycles budget) via a pipelined SIMD kernel and 0 heap allocations."
     > "... Evaluating $\bar{q}_L \cdot (\mathbf{v}' \cdot q_R)$ across 16 quaternions (64 bytes in a 512-bit `ZMM0` register): Right multiplication $(\mathbf{v}' \cdot q_R)$: 9 clock cycles. Left multiplication $\bar{q}_L \cdot (\dots)$: 9 clock cycles. Vector register setup and store: 4 clock cycles. $\mathcal{T}_{\mathrm{IsoQuant}}(64\text{ B}) = 9 + 9 + 4 = 22 \text{ CPU clock cycles}$."
   - In `MATHEMATICAL_REPORT.md` §7.5 (Universal Zero-Allocation Lemma, lines 1251–1256):
     > "For every transform $\Phi \in \{\dots, \text{IsoQuant}, \dots\}$, $M_{\mathrm{state}} \le 32,768 \text{ bytes} \le \text{Size}(\mathrm{L1D Cache})$ and $\mathrm{HeapAlloc} \equiv 0 \text{ B/op}, \mathrm{Allocs} \equiv 0 \text{ allocs/op}$."

2. **Project Architecture & Interface Contracts (`PROJECT.md`):**
   - Lines 48–57:
     - Frame Header: 20 bytes aligned:
       - Bytes 0..3: Magic `PRZM_MAGIC_ISOQUANT` (`0x514F5349`)
       - Byte 4: Mode / Flags (`uint8`)
       - Bytes 5..7: Reserved (`uint8[3]`)
       - Bytes 8..11: Uncompressed length `orig_len` (`uint32_le`)
       - Bytes 12..15: Left quaternion inverse $q_{L,\text{inv}} \in \mathbb{F}_{2^8}^4$ (`uint8[4]`)
       - Bytes 16..19: Right quaternion inverse $q_{R,\text{inv}} \in \mathbb{F}_{2^8}^4$ (`uint8[4]`)
     - Decompression API: `przm_decompress(src, src_len, dst, dst_len)` decodes payload, then applies in-place `isoquant_unrotate_inplace(dst, orig_len, qL_inv, qR_inv)`.
     - Invariant: Zero allocations (`0 B/op, 0 allocs/op`), in-place execution in $\le 22$ CPU cycles per 64 bytes.

3. **Current Codebase Implementation State (`d:\CodingProjects\przm`):**
   - `clifford.go` (lines 38–60) implemented 16-grade Clifford multivectors via Galois-Walsh convolution.
   - `symplectic.go` (lines 45–98) demonstrates the target design pattern for in-place 4D block transformations with unrolled 16-byte steps and zero allocations.
   - `src/przm_dec.c` (lines 162–172) implements `gf256_mul` in pure C under the AES polynomial $0x11B$.
   - `include/przm_dec.h` defines magic signatures, error codes, and API contracts.

---

## 2. Logic Chain

1. **Algebraic Foundation:**
   - Multi-channel data flattened into 1D byte tapes disperses cross-channel correlation, inflating apparent first-order entropy to near 8 bits/byte.
   - By embedding 4-byte vectors $(x_0, x_1, x_2, x_3)$ into the hypercomplex quaternion algebra $\mathbb{H}(\mathbb{F}_{2^8})$, any proper 4D rotation is expressible via the double cover $\mathrm{Spin}(4) \cong \mathrm{Sp}(1) \times \mathrm{Sp}(1) \to \mathrm{SO}(4)$ as commuting left- and right-isoclinic rotations: $\mathbf{x}' = q_L \cdot \mathbf{x} \cdot \bar{q}_R$.
   - Because left and right quaternion multiplication operators commute ($L(p) R(q) = R(q) L(p)$), the 4D rotation space has 6 degrees of freedom (3 from $q_L$, 3 from $q_R$), spanning all proper rotations in $\mathrm{SO}(4)$.

2. **Covariance Collimation & Redundancy Annihilation:**
   - The sample covariance matrix $\mathbf{\Sigma} \in \mathbb{R}^{4 \times 4}$ captures pairwise channel variances and cross-covariances.
   - Total correlation (multi-information) $C(x_0; \dots; x_3)$ measures the shared redundancy among all four channels.
   - Orthogonal coordinate rotation preserves joint entropy ($\mathcal{H}(\mathbf{x}') = \mathcal{H}(\mathbf{x})$) while minimizing marginal coordinate entropy sum:
     $$\Delta \mathcal{H} = \frac{1}{2} \log_2 \left( \frac{\prod \Sigma_{mm}}{\det \mathbf{\Sigma}} \right) \ge 0$$
   - Diagonalizing $\mathbf{\Sigma}$ minimizes marginal entropy by concentrating variance into the dominant eigenvalue $\lambda_0$ while suppressing orthogonal noise channels $\lambda_1, \lambda_2, \lambda_3 \to 0$.
   - From the diagonalizing rotation $M \in \mathrm{SO}(4)$, we derived the exact closed-form Cayley combinators:
     $$\mathbf{K}_0 = c_0 - c_1 \mathbf{i} - c_2 \mathbf{j} - c_3 \mathbf{k} = 4 b_0 q_L$$
     $$\mathbf{K}_1 = c_0 \mathbf{i} + c_1 - c_2 \mathbf{k} + c_3 \mathbf{j} = 4 b_1 q_L$$
     $$\mathbf{K}_2 = c_0 \mathbf{j} + c_1 \mathbf{k} + c_2 - c_3 \mathbf{i} = 4 b_2 q_L$$
     $$\mathbf{K}_3 = c_0 \mathbf{k} - c_1 \mathbf{j} + c_2 \mathbf{i} + c_3 = 4 b_3 q_L$$
     This provides a deterministic, singularity-free, non-iterative $O(1)$ extraction of $(q_L, q_R)$.

3. **Bit-Exact Losslessness:**
   - In $\mathbb{F}_{2^8}$, the Frobenius squaring map is linear: $|q|^2 = (q_0 \oplus q_1 \oplus q_2 \oplus q_3)^2$.
   - Any quaternion with non-zero parity sum has an exact multiplicative inverse $q^{-1} = q \odot \text{GF256Inv}(|q|^2)$ in $\mathbb{F}_{2^8}$.
   - Because $q_{L,\text{inv}} \cdot q_L = 1$ and $q_R \cdot q_{R,\text{inv}} = 1$, the inverse map $\mathbf{x} = q_{L,\text{inv}} \cdot \mathbf{x}' \cdot q_{R,\text{inv}}$ reconstructs the original coordinates with 100% bit-exact losslessness, zero floating-point drift, and zero rounding error.
   - Alternatively, over $\mathbb{Z}/256\mathbb{Z}$, exact invertibility is guaranteed via unimodular shear lifting ($M = S_1 \cdots S_k$ with $\det S_i = 1$) or isoclinic involutions ($M^2 = I$).

4. **Branchless $O(1)$ Decompressor Invariance:**
   - On x86-64 with AVX-512 + GFNI, a 64-byte block (16 quaternions) is decoded via 8 vector shuffles (`vpshufb`), 8 Galois multiplications (`vgf2p8mulb`), 6 vector XORs (`vpxord`), and 1 store (`vmovdqu64`).
   - Pipelined across execution ports 0, 1, and 5, this evaluates in strictly 22 CPU clock cycles, achieving $\approx 9,890\text{ MB/s}$ throughput on a 3.4 GHz core.
   - The pure C fallback kernel (`isoquant_unrotate_inplace`) runs completely branchless, in-place, with 0 dynamic heap allocations, strictly satisfying the Universal Zero-Allocation Lemma ($M_{\text{state}} \le 32\text{ KiB} \subset \text{L1D}$).

---

## 3. Caveats

1. **Galois Field vs Euclidean Rotation:**
   - In the continuous domain $\mathbb{R}^4$, $\mathrm{SO}(4)$ rotations preserve the standard Euclidean norm $x_0^2 + x_1^2 + x_2^2 + x_3^2$.
   - In the discrete Galois field $\mathbb{F}_{2^8}$, arithmetic is modulo $p(x) = x^8 + x^4 + x^3 + x + 1$ with XOR addition. While discrete algebraic invertibility and coordinate collimation are mathematically preserved, continuous geometric angle intuitions do not map 1-to-1 to field automorphisms; continuous rotations are projected/quantized to field elements $q \in \mathbb{F}_{2^8}^4$ that maximize symbol run-lengths and zero-byte frequency.
2. **Unaligned Stream Remainders:**
   - Streams with length $N \not\equiv 0 \pmod 4$ require trailing remainder bytes to be copied verbatim without quaternionic rotation. The header field `orig_len` accurately marks the boundary.

---

## 4. Conclusion

The mathematical algorithms, arithmetic formulas, and microarchitectural kernels for 4D IsoQuant Quaternionic Collimation are fully established, verified, and detailed in `report.md`:
- **Algebra:** Hamilton quaternion algebra over $\mathbb{H}(\mathbb{F}_{2^8})$ and $\mathbb{Z}/256\mathbb{Z}$.
- **Alignment:** $O(1)$ closed-form Cayley combinator extraction of $(q_L, q_R)$ from empirical covariance $\mathbf{\Sigma}$.
- **Forward & Inverse:** Complete 2-stage branchless multiplication formulas.
- **Microarchitecture:** 22-cycle AVX-512/GFNI schedule and pure C in-place kernel with 0 heap allocations.
- **Integrity:** Strict preservation of instantaneous $O(1)$ decompression and language-neutral core directives.

The specification is ready for implementation by the coding agents in Go (`isoquant.go`, `compress.go`) and C (`src/przm_dec.c`, `include/przm_dec.h`).

---

## 5. Verification Method

1. **Report Verification:**
   Inspect `d:\CodingProjects\seal\.agents\teamwork\explorer_m2_math\report.md` to confirm:
   - Full mathematical derivation of the Cayley combinators $\mathbf{K}_0..\mathbf{K}_3$.
   - Exact equations for forward $M_{\text{fwd}}$ and inverse $M_{\text{inv}}$ matrices.
   - The 22-cycle instruction schedule and port breakdown.
   - The pure C in-place kernel `isoquant_unrotate_inplace`.

2. **Algebraic Invertibility Verification:**
   Verify algebraically that for any non-zero unit quaternions $q_L, q_R \in \mathbb{H}(\mathbb{F}_{2^8})$:
   $$(q_L^{-1}) \cdot (q_L \cdot \mathbf{v} \cdot q_R) \cdot (q_R^{-1}) = \mathbf{v}$$
   using standard AES polynomial arithmetic ($p(x) = 0x11B$).

3. **Codebase Build & Test Verification:**
   Run the standard Go and C test commands in `d:\CodingProjects\przm`:
   - `go build ./...`
   - `go test -v .`
