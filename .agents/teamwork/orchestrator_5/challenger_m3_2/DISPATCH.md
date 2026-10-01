# Dispatch for challenger_m3_2

## Identity
- Archetype: teamwork_preview_challenger
- Role: Milestone 3 Concurrency, Zero-Alloc & C Stress Challenger 2
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m3_2
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m3\handoff.md` (Worker M3 handoff)
- `d:\CodingProjects\przm\cellular_sheaf.go`, `d:\CodingProjects\przm\src\przm_dec.c`, `d:\CodingProjects\przm\include\przm_dec.h`

## Objective
Adversarially challenge Milestone 3:
1. Concurrency stress: 100 concurrent goroutines compressing/decompressing Cellular Sheaf frames under Go race detector (`go test -race`).
2. Allocation assertion: Verify strictly 0 heap allocations during decompression (`0 B/op, 0 allocs/op`) in Go and 0 `malloc`/`free` calls in C.
3. Pure C cross-decompression stress: Generate 50 random Cellular Sheaf frames (varied strides, varied curl densities, varied dimensions) and verify `test_przm_dec.exe` decodes all 50 losslessly with 0 errors.
4. Microarchitectural throughput verification: Measure decompression line rate (> 20 GB/s, $\le 10$ CPU cycles per 64-byte block).
5. Deliver verdict: `APPROVE` or `REQUEST_CHANGES`.

## Deliverables
Write report to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m3_2\challenge.md` and deliver `handoff.md`.

## 2026-10-01T12:06:38Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m3_2. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m3_2\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, PROJECT.md, and worker_m3 handoff. Adversarially challenge Milestone 3: 100-goroutine concurrency stress under -race, zero-allocation assertions, and Pure C test_przm_dec.exe decoding 50 random frames. Deliver challenge.md and handoff.md with APPROVE or REQUEST_CHANGES verdict.

