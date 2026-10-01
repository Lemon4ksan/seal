# Dispatch for explorer_m3_iter2

## Identity
- Archetype: teamwork_preview_explorer
- Role: Milestone 3 Iteration 2 Remediation Explorer
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\explorer_m3_iter2
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read thoroughly before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\auditor_m3\audit.md` (FULL FORENSIC AUDITOR EVIDENCE REPORT - MUST READ IN FULL)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\auditor_m3\handoff.md` (Auditor handoff)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m3_1\challenge.md` (Boundary challenger evidence report)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m3_2\challenge.md` (C stress challenger evidence report)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m3_2\review.md` (Reviewer 2 evidence report)
- `d:\CodingProjects\przm\cellular_sheaf.go`
- `d:\CodingProjects\przm\src\przm_dec.c`

## Objective & Non-Negotiable Remediation Scope
Investigate the specific integrity violations and defects identified by `auditor_m3`, `challenger_m3_1`, and `challenger_m3_2`, and formulate a complete, mathematically authentic remediation plan:

1. **Mathematical Defect in `Adjoint0`**:
   - In `cellular_sheaf.go`, `Coboundary0` defines $(d_0 \phi)(e) = \phi(v_1) \oplus \alpha \odot \phi(v_0)$.
   - The matrix transpose adjoint $d_0^T$ with respect to the standard bilinear pairing $\langle x, y \rangle = \bigoplus x_i \odot y_i$ must weight outgoing edges from $v_0$ by $\alpha$ and incoming edges into $v_1$ by 1:
     $$(d_0^T w)(v) = \left( \bigoplus_{e = (v, u)} \alpha \odot w(e) \right) \oplus \left( \bigoplus_{e = (u, v)} w(e) \right)$$
   - Formulate the exact fix in `Adjoint0` to satisfy $\langle d_0 \phi, w \rangle \equiv \langle \phi, d_0^T w \rangle$ for all $\alpha \in \mathbb{F}_{2^8}$ and all dimensions.
2. **Pure C Reference Decompressor Data Corruption on Twist $\alpha > 1$**:
   - In `src/przm_dec.c` (`przm_cellular_sheaf_decompress`), analyze why vertical edge potentials fail to evolve with $\alpha$ across columns during row sweep.
   - Formulate the exact C code fix for propagating `gf256_mul(twist, cur_v)` to ensure 100% bit-exact parity between Go and C across all twists.
3. **Zero-Allocation Decompression Invariant Breach on Cascaded PWS**:
   - Analyze why `DecompressCellularSheaf` incurs heap allocations via `PWSDecode(payload)`.
   - Formulate a zero-allocation decompression strategy: caller-provided destination slice, scratchpad-buffered decoding, or uncascaded byte streaming so `testing.AllocsPerRun(100)` strictly yields `0 B/op, 0 allocs/op`.
4. **Authentic Hodge Laplacian Projection onto $\ker \Delta_1$**:
   - Formulate genuine harmonic 1-form projection solving or projecting against the harmonic 1-cycle basis $\ker(\Delta_1) = \ker(d_1) \cap \ker(d_0^T)$ of the cell complex, eliminating any facade or dummy boundary check.

## Deliverables
Write your comprehensive remediation report to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\explorer_m3_iter2\report.md` and deliver `handoff.md`.

## 2026-10-01T12:14:51Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\explorer_m3_iter2. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\explorer_m3_iter2\DISPATCH.md. Also read auditor_m3/audit.md, challenger_m3_1/challenge.md, and cellular_sheaf.go. Formulate the concrete mathematical and code remediation plan for Adjoint0, Pure C decompressor twist propagation, zero-allocation decompression, and authentic Hodge harmonic projection. Deliver report.md and handoff.md.

