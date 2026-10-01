# Dispatch for reviewer_m2_1

## Identity
- Archetype: teamwork_preview_reviewer
- Role: Milestone 2 Heisenberg Code & Algebra Reviewer 1
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m2_1
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m2\handoff.md` (Worker M2 handoff)
- `d:\CodingProjects\przm\heisenberg.go`, `d:\CodingProjects\przm\heisenberg_test.go`, `d:\CodingProjects\przm\compress.go`, `d:\CodingProjects\przm\decompress.go`

## Objective
Review Milestone 2 (Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ Foliation):
1. Review algebraic correctness:
   - Group multiplication: $(x, y, z) \circ (x', y', z') = (x \oplus x', y \oplus y', z \oplus z' \oplus (x \odot y'))$ over $\mathbb{F}_{2^8}$.
   - Exact algebraic inverse: $H^{-1}(x, y, z) = H(x, y, z \oplus (x \odot y))$.
   - In-place foliation and unfoliation implementations (`HeisenbergFoliateInplace`, `HeisenbergUnfoliateInplace`) for Stride 3 and Stride 4.
2. Review integration into Go codec:
   - Pareto arbitration in `compress.go` (`heisenbergCandidate` in `MinNonEmpty`).
   - Frame dispatch in `decompress.go` (`Decompress` and `DecompressTo` for magic `0x53494548`).
3. Verify zero-allocation decompression invariant (`0 B/op, 0 allocs/op`).
4. Verify language-neutrality: zero hardcoded language tokens or syntax dictionaries.
5. Execute verification commands:
   - `go build ./...`
   - `go test -v -run TestHeisenberg .`
   - `go test -v -run TestPureCDecompressorInterop .`
6. Issue a clear gate verdict: `APPROVE` or `REQUEST_CHANGES`.

## Deliverables
Write review to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m2_1\review.md` and deliver `handoff.md`.

## 2026-10-01T11:42:07Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m2_1. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m2_1\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, PROJECT.md, and worker_m2 handoff. Review Milestone 2 Heisenberg code and algebraic correctness in Go. Execute tests and deliver review.md and handoff.md with APPROVE or REQUEST_CHANGES verdict.
