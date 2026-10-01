# Dispatch for challenger_m2_2

## Identity
- Archetype: teamwork_preview_challenger
- Role: Milestone 2 Concurrency, Zero-Alloc & C Stress Challenger 2
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m2_2
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m2\handoff.md` (Worker M2 handoff)
- `d:\CodingProjects\przm\heisenberg.go`, `d:\CodingProjects\przm\c_interop_test.go`, `d:\CodingProjects\przm\src\przm_dec.c`

## Objective
Adversarially challenge Milestone 2:
1. Concurrency stress: 100 concurrent goroutines compressing/decompressing Heisenberg frames with race detector enabled (`go test -race`).
2. Allocation assertion: Verify strictly 0 heap allocations during decompression (`0 B/op, 0 allocs/op`).
3. Pure C cross-decompression stress: Generate 50 random Heisenberg frames in Go (stride 3 and 4), save/pipe to `test_przm_dec.exe`, and assert 100% bit-exact lossless decoding with 0 allocations.
4. Verify throughput: benchmark decompressor line rate (> 20 GB/s).
5. Deliver verdict: `APPROVE` or `REQUEST_CHANGES`.

## Deliverables
Write report to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m2_2\challenge.md` and deliver `handoff.md`.

## 2026-10-01T11:42:12Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m2_2. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m2_2\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, PROJECT.md, and worker_m2 handoff. Adversarially challenge Milestone 2: 100-goroutine concurrency stress under -race, zero-allocation assertions, and Pure C test_przm_dec.exe decoding 50 random frames. Deliver challenge.md and handoff.md with APPROVE or REQUEST_CHANGES verdict.
