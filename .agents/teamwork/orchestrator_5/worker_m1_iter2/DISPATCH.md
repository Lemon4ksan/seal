# Dispatch for worker_m1_iter2

## Identity
- Archetype: teamwork_preview_worker
- Role: Milestone 1 Iteration 2 Remediation Worker
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1_iter2
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m1_2\handoff.md` and `challenge.md`
- `d:\CodingProjects\przm\challenger_m1_verify_test.go`
- `d:\CodingProjects\przm\decompress.go`
- `d:\CodingProjects\przm\src\przm_dec.c` & `include\przm_dec.h`

## Mandatory Integrity Warning
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## Objective & Remediation Tasks
Remediate the two issues identified by `challenger_m1_2`:

1. **Fix Memory Safety / Checkptr Bug in `decompress.go`**:
   - In `d:\CodingProjects\przm\decompress.go` (around line 782, direct literal runs `0xF3..0xFA`):
     Validate slice bounds before executing unsafe 8-byte pointer reads:
     ```go
     if litPtr+8 <= len(lits) && dstPos+8 <= dstCap {
         *(*uint64)(pDst) = *(*uint64)(pSrc)
     } else {
         for k := 0; k < lLen; k++ {
             dst[dstPos+k] = lits[litPtr+k]
         }
     }
     ```
   - Verify that running `go test -race -run TestChallengerM1_ShortLiteralMemorySafety .` passes cleanly with ZERO checkptr crashes.

2. **Fix Pure C Decompressor Interoperability for GCTR Frames**:
   - Inspect GCTR frame format in `gctr.go` (magic `0x47435452` "GCTR").
   - Add GCTR decompression support in `src/przm_dec.c` and `include/przm_dec.h` (`przm_gctr_decompress` or handle GCTR frame in `przm_decompress`), or ensure C decompressor handles all frames emitted by Level 4.
   - Verify that running `go test -v -run TestChallengerM1_CDecompressorSinusoidalPayloads .` passes cleanly.

3. **Verify Full Repository Integrity**:
   - `go build ./...`
   - `go test -v -run TestHensel .`
   - `go test -v -run TestChallenger .`
   - `gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe && .\test_przm_dec.exe`
   - `lake build`

## Deliverables
Write report to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1_iter2\report.md` and deliver `handoff.md`.

## 2026-10-01T11:07:30Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1_iter2. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m1_iter2\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m1_2\handoff.md, and przm codebase. Remediate decompress.go checkptr short literal bug and C decompressor GCTR interoperability. Run tests with -race and verify test_przm_dec.exe. Deliver report.md and handoff.md.

