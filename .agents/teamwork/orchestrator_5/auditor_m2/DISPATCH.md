# Dispatch for auditor_m2

## Identity
- Archetype: teamwork_preview_auditor
- Role: Milestone 2 Forensic Integrity Auditor
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\auditor_m2
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m2\handoff.md` (Worker M2 handoff)
- `d:\CodingProjects\przm\heisenberg.go`, `d:\CodingProjects\przm\src\przm_dec.c`, `d:\CodingProjects\przm\include\przm_dec.h`

## Objective
Perform strict forensic integrity audit on Milestone 2 (Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ Foliation):
1. **Integrity Forensics**:
   - Verify that `heisenberg.go` implements genuine nilpotent Lie group projections and exact algebraic involution $H^{-1}(x,y,z) = H(x, y, z \oplus (x \odot y))$ without dummy facades, lookup shortcuts, or synthetic bypasses.
   - Verify that `przm_heisenberg_unfoliate_inplace` in `src/przm_dec.c` genuinely executes branchless Galois arithmetic and unfoliation in C with 0 heap allocations (`malloc`/`free` = 0).
   - Verify genuine runtime gating and Pareto arbitration in `compress.go` and frame dispatch in `decompress.go`.
2. **Language Neutrality Audit**:
   - Check that NO new hardcoded programming-language syntax dictionaries, token tables, or keywords were introduced.
3. **Decompression & Build Verification**:
   - Clean compilation: `go build ./...` and `gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe`.
   - Verify zero heap allocations in decompression.
   - Verify Lean 4 build: `lake build`.
4. **Verdict**:
   - Must issue either `CLEAN` or `INTEGRITY VIOLATION`.
   - Any cheat or facade is a non-negotiable hard veto.

## Deliverables
Write report to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\auditor_m2\audit.md` and deliver `handoff.md`.

## 2026-10-01T11:42:13Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\auditor_m2. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\auditor_m2\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, PROJECT.md, and worker_m2 handoff. Perform forensic integrity audit on Milestone 2: verify genuine Heisenberg math, zero facades, language neutrality, zero allocations, and clean builds. Deliver audit.md and handoff.md with CLEAN or INTEGRITY VIOLATION verdict.

