# Dispatch for reviewer_m1_1

## Identity
- Archetype: teamwork_preview_reviewer
- Role: Milestone 1 Code & Correctness Reviewer 1
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m1_1
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1\handoff.md` (Worker handoff)
- `d:\CodingProjects\przm\hensel.go`, `d:\CodingProjects\przm\hensel_test.go`, `d:\CodingProjects\przm\compress.go`

## Objective
Review Milestone 1 (p-Adic Ultrametric Match Finding via Hensel's Lifting):
1. Verify code correctness, algorithm fidelity to Theorem HL-1 and HL-2, bounds checking, and memory safety.
2. Verify strict preservation of bitstream format: decompressor must be completely unmodified and capable of decompressing Level 3/4 output.
3. Verify zero-allocation contract in decompression.
4. Verify language-neutrality: zero hardcoded language tokens or syntax dictionaries.
5. Execute verification commands:
   - `go build ./...`
   - `go test -v -run TestHensel .`
   - `go test -v -run TestPRZM_Unified_LevelsRoundtrip .`
   - `gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe && .\test_przm_dec.exe`
6. Issue a clear gate verdict: `APPROVE` or `REQUEST_CHANGES`.

## Deliverables
Write review to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m1_1\review.md` and deliver `handoff.md`.

## 2026-10-01T10:55:54Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m1_1. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\reviewer_m1_1\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md, and worker_m1 handoff. Review Milestone 1 code and correctness in hensel.go, hensel_test.go, and compress.go. Execute builds and tests. Deliver review.md and handoff.md with APPROVE or REQUEST_CHANGES verdict.
