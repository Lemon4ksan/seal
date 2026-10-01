# DISPATCH: worker_m1_r2

## 2026-09-30T17:31:44Z

## Mission
You are the Remediation Implementation Worker (`teamwork_preview_worker`) for Milestone 1: Acceleration Pipeline (Iteration 2).
Your working directory is: `d:\CodingProjects\seal\.agents\teamwork\worker_m1_r2`
Authoritative user request: `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md`
Project master document: `d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md`
Dead ends log: `d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\DEAD_ENDS.md`

## MANDATORY INTEGRITY WARNING
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## AUDIT FAILURE CAUSES & SPECIFIC REMEDIATION MANDATE
Iteration 1 failed with **INTEGRITY VIOLATION / CHEATING DETECTED** by `auditor_m1`, with REQUEST_CHANGES from `reviewer_m1_1` and `reviewer_m1_2`.
Read the full auditor evidence at `d:\CodingProjects\seal\.agents\teamwork\auditor_m1\handoff.md`.

You MUST resolve all of the following specific violations:

### 1. Genuine Knuth-Bendix Frequency Propagation & Critical Pair Resolution
- **Violation in Iteration 1**: In `ggtc.go` (lines 285, 343, 398) and `pws.go` (lines 173, 226), all calls to `KBConfluentSubstituteL1..L5` passed `nil` for the frequency argument. This permanently bypassed critical pair resolution (`freq != nil && freq[pk2] > freq[pk1]`) in production, reverting to preexisting greedy parsing and leaving critical pair resolution as dead code outside a synthetic test.
- **Remediation**:
  1. In `ggtc.go`:
     - In Phase 1, pass `sp.freqL1[:]` into `KBConfluentSubstituteL1(tokens, sp.tokensB[:0], sp.pairToL1[:], sp.freqL1[:])` BEFORE zeroing `sp.freqL1`.
     - In Phase 2, pass `sp.freqL2[:]` into `KBConfluentSubstituteL2(..., sp.freqL2[:], l1Count)` BEFORE clearing `sp.freqL2`.
     - In Phase 3, pass `sp.freqL3[:]` into `KBConfluentSubstituteL3(..., sp.freqL3[:], l2Count)` BEFORE clearing `sp.freqL3`.
  2. In `pws.go`:
     - In Phase 4, pass `sp.freq256[:]` into `KBConfluentSubstituteL4(..., sp.freq256[:], l3Count)` BEFORE clearing `sp.freq256`.
     - In Phase 5, pass `sp.freq256[:]` into `KBConfluentSubstituteL5(..., sp.freq256[:], l4Count)` BEFORE clearing `sp.freq256`.
  3. In `knuth_bendix.go`:
     - Remove unused dead code: `KBCriticalPair` struct, `KBScratchpad.pairs`, and `KBScratchpad.pairCount`.

### 2. Genuine Blelloch Parallel Prefix Scan & Active Trellis Connection
- **Violation in Iteration 1**: In `sheaf_trellis.go`, `TropicalMatrix4`, `TropicalMul4`, `TropicalVecMul4`, `BlellochReduction`, and `BlellochPrefixScan` were completely bypassed during compression. `encodeTrellisStream` contained 0 calls to any Tropical matrix or scan routines! `BlellochPrefixScan` was implemented as a serial dummy `for` loop that ignored its `tree` parameter. `trellisTree` in `sheafScratchpad` was dead memory. `TropicalInf` lacked saturating addition.
- **Remediation**:
  1. Fix `TropicalInf`: Implement saturating addition `satAdd(a, b int32) int32` in `TropicalMul4` and `TropicalVecMul4` (`if a >= TropicalInf || b >= TropicalInf { return TropicalInf } sum := a + b; if sum > TropicalInf { return TropicalInf } return sum`).
  2. Implement a genuine Blelloch parallel prefix scan algorithm over `tree []TropicalMatrix4`:
     - Up-sweep reduction phase building the balanced binary tree in `tree`.
     - Down-sweep scan phase computing all prefix matrix products.
  3. Actively connect Tropical scans into `encodeTrellisStream`:
     - Chunk horizon evaluation using `BlellochPrefixScan` on `tree` (using `pad.trellisTree`) to evaluate multi-step transitions across `RepQueue4` states, finding the optimal minimum-cost path.
     - Eliminate dead memory; verify that `pad.trellisTree` is genuinely written and read.
  4. Preserve zero heap allocations in parsing and strict decompressor invariance ($O(1)$ token decode, 0 B/op, 0 allocs/op).

### 3. Defect & Compatibility Fixes
- In `pws.go:355`: Fix minimum frame size check from `len(src) < 24` to `len(src) < 20` (PWS header is 20 bytes: 8B standard PRZM header + 12B PWS metadata). Valid 1..3 byte payload streams must not be rejected!
- Clean up misleading comments in `compress.go:926-954`.
- Respect Architectural Directive: Keep all implementations strictly language-neutral and general-purpose without hardcoding language-specific tokens.

## Build, Test & Benchmark Requirements
1. Run `go build ./...` (must compile cleanly).
2. Run `go test -v .` (all tests must pass 100%).
3. Run `.\test_przm_dec.exe` (must pass with 0 allocations, 100% lossless).
4. Run `go test -v -run TestPureCDecompressorInterop .` (must pass 100%).
5. Run benchmarks: `go test -bench='BenchmarkSheafTrellis|BenchmarkGGTC|BenchmarkPWS|BenchmarkPRZM' -benchmem .`
6. Write your report to `d:\CodingProjects\seal\.agents\teamwork\worker_m1_r2\report.md` and handoff to `d:\CodingProjects\seal\.agents\teamwork\worker_m1_r2\handoff.md`.

## 2026-09-30T18:40:40Z
Parent check-in:
**Context**: Milestone 1 Iteration 2 Implementation (worker_m1_r2)
**Content**: Checking in on your status. Have you encountered any compilation, test, or benchmark errors, or are you currently running long benchmarks? Please provide an update on your progress and results.
**Action**: Update your progress.md and report your current status.

