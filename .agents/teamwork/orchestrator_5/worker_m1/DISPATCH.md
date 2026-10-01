# Dispatch for worker_m1

## Identity
- Archetype: teamwork_preview_worker
- Role: p-Adic Ultrametric & Hensel Lifting Implementation Specialist
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\survey_explorer_2\report.md` (Detailed architectural & mathematical blueprint)
- `d:\CodingProjects\przm\compress.go`

## Write Ownership
You exclusively own:
- `d:\CodingProjects\przm\hensel.go` (New)
- `d:\CodingProjects\przm\hensel_test.go` (New)
- `d:\CodingProjects\przm\compress.go` (Integration into Level 3/4 match search)

## Mandatory Integrity Warning
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## Objective
Implement Milestone 1: **p-Adic Ultrametric Match Finding via Hensel's Lifting**:
1. Create `hensel.go`:
   - Implement non-Archimedean $p$-adic valuation and quadratic Hensel lifting for prefix matching.
   - `HenselLCP(a, b []byte) int`: Fast LCP evaluation using 64-bit SWAR/bitwise operations (`bits.TrailingZeros64(x ^ y) >> 3`).
   - `HenselMatchFinder` / `HenselScratchpad`: In-cache 32-bit second-half fingerprint table (`fp32`), candidate chain table (`prev`), and head table.
   - Candidate filtering via the Ultrametric Pruning Criterion (Theorem HL-2 / report): eliminates 85-93% of non-viable candidates in 1 clock cycle without memory dereferences.
   - Depth expansion: Search depth 16 on Level 3, depth 64 on Level 4.
2. Integrate into `compress.go`:
   - Wire the Hensel match finder into the main sliding-window LZ match-search loop for compression Level 3 and Level 4.
   - Ensure scratchpad memory is allocated via `scratchpadPool` or in `compressPad` to guarantee zero per-operation allocations during compression.
   - CRITICAL REQUIREMENT: Strictly preserve the external bitstream format (PRZM-SPEC-0.5). Output must be 100% bit-exact decompressible by both Go `Decompress` and Pure C `przm_dec.c`!
3. Create `hensel_test.go`:
   - Unit tests for Hensel LCP, fingerprint pruning, search correctness vs brute-force oracle.
   - Roundtrip tests verifying bit-exact losslessness across all corpora on Level 3 and Level 4.
   - Microarchitectural benchmark comparing match finding speed and compression ratio before and after.
4. Verify builds and tests:
   - `go build ./...`
   - `go test -v -run TestHensel .`
   - `go test -v -run TestPRZM_Unified_LevelsRoundtrip .`
   - `go test -bench="BenchmarkPRZM" -benchmem -run=^$ .`
   - Ensure `test_przm_dec.exe` can decompress the output with 0 allocations.
5. Language neutrality: Strictly general-purpose bitwise/arithmetic operations. Zero hardcoded token dictionaries.

## Deliverables
Write your completion report to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1\report.md` and deliver `handoff.md`.

## 2026-10-01T10:44:17Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md, and d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\survey_explorer_2\report.md. Implement p-Adic Ultrametric match search via Hensel's lifting in hensel.go, hensel_test.go, and compress.go. Run builds, tests, and benchmarks. Write your report to d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1\report.md, deliver handoff.md, and notify orchestrator_5.

