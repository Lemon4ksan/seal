# Dispatch to Project Orchestrator (orchestrator_3)

## 2026-09-30T16:15:20Z

You are the Project Orchestrator for this mission.
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3
The authoritative user request is recorded at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md

Project Workspace: d:\CodingProjects\seal
Target Codebase & Working Directory: d:\CodingProjects\przm
Integrity Mode: development

## Mission Overview
Implement and benchmark the prioritized mathematical optimization proposals from `MATHEMATICAL_REPORT.md` in the PRZM codebase (`d:\CodingProjects\przm`). Follow a phased approach: first implement the acceleration pipeline (Knuth-Bendix term rewriting and Tropical associative scans), followed by the compression ratio enhancement (4D IsoQuant quaternionic collimation). The implementation targets the Go compression engine and the lightweight C decompressor, with continuous benchmark verification ensuring zero heap allocations and instantaneous O(1) decompression.

## Requirements

### R1. Acceleration Pipeline Implementation
Implement Knuth-Bendix deterministic rewrite automata and Tropical associative prefix scans in the Go encoder to replace heuristic match lookups, minimizing critical path latency during compression.

### R2. Ratio Enhancement Pipeline Implementation
Implement 4D IsoQuant quaternionic collimation across multi-channel and shader/binary streams in Go, with corresponding branchless vector decode support in the C decompressor (`przm_dec.c`).

### R3. Continuous Benchmarking and Invariant Verification
Provide automated benchmark execution and reporting across iterations, measuring throughput (MB/s) and compression ratio while strictly verifying zero heap allocations and bit-exact roundtrip losslessness.

## Acceptance Criteria

### Compilation & Test Integrity
- [ ] The Go codebase builds cleanly (`go build ./...`) and all existing unit and roundtrip tests pass (`go test -v .`).
- [ ] The C reference decompressor builds and passes verification tests (`test_przm_dec.exe`).

### Performance & Optimization Verification
- [ ] The acceleration pipeline demonstrates a measured, reproducible compression speedup over the existing baseline on standard benchmark benchmarks.
- [ ] 4D IsoQuant quaternionic collimation demonstrates improved compression ratio on structured/binary/shader test data.
- [ ] Instantaneous decompression invariant is strictly preserved: 0 heap allocations (`0 B/op, 0 allocs/op`) and execution speed bounded by L1D / memory bus rate.
- [ ] Each progress report includes an explicit Before/After comparative benchmark table detailing throughput (MB/s), compression ratio, and allocation counts.

## Key Instructions & Protocol
- Decompose the mission across your specialized subagents (implementers, testers, benchmarkers, reviewers).
- Maintain persistent state in your working directory `d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\BRIEFING.md` and keep `progress.md` updated continuously with status, milestone progress, and benchmark tables.
- Target codebase is located at `d:\CodingProjects\przm`.
- When all requirements are met and verified, deliver your victory report to the Sentinel.

## 2026-09-30T16:39:51Z

The server experienced a restart. Your persistent state files (BRIEFING.md, PROJECT.md, progress.md) in d:\CodingProjects\seal\.agents\teamwork\orchestrator_3 are intact.

Please resume orchestration immediately:
1. Revive your active subagents (or check their handoffs/status) and re-register your internal heartbeat timer.
2. Advance Milestone 1: Acceleration Pipeline (Knuth-Bendix deterministic rewrite automata and Tropical associative prefix scans in the Go encoder).
3. Proceed to Milestone 2: Ratio Enhancement Pipeline (4D IsoQuant quaternionic collimation in Go and branchless vector decode in C decompressor przm_dec.c).
4. Conduct Milestone 3: Continuous Benchmarking and Invariant Verification (zero heap allocations, 0 B/op, 0 allocs/op, bit-exact roundtrips, Before/After performance tables).
5. Report progress continuously in progress.md and deliver a final victory report upon completion.

## 2026-09-30T17:19:32Z

ARCHITECTURAL DIRECTIVE FROM USER (recorded in ORIGINAL_REQUEST.md):
The user raised an important architectural critique regarding hardcoded schema dictionaries (DictID = 1 JSON, DictID = 2 Go source code):
1. Hardcoding language-specific tokens (specifically Go syntax like `if err != nil`) inside the core codec engine and bitstream specification is architectural tech debt, breaks language-neutrality, and permanently freezes these lexicons.
2. The team must ensure that all new mathematical modules currently being implemented (Knuth-Bendix automata, Tropical associative scans, and 4D IsoQuant) remain strictly general-purpose and completely decoupled from hardcoded token dictionaries.
3. For future dictionary evolution, plan the architectural decoupling: reserve low DictIDs (DictID = 1) strictly for universal standards like JSON, while routing domain-specific dictionaries through an external/custom dictionary training interface (similar to `zstd --train` or container-level dictionaries in the EON substrate where DictID >= 0x1000).

Please ensure this architectural guidance is immediately recorded in your engineering documentation (e.g., `PROJECT.md`), and verify that all current and upcoming implementations (M1, M2, M3) adhere strictly to general-purpose mathematical formulations without language-specific hardcoding.

## 2026-09-30T17:27:01Z

The server experienced a restart. Your persistent state in d:\CodingProjects\seal\.agents\teamwork\orchestrator_3 is preserved.
Sentinel monitoring crons (Progress Reporting, Liveness Check) have been restored.

Please resume execution immediately:
1. Revive or check completion status of your M1 Iteration 2 Remediation Explorers (explorer_m1_knuth_r2, explorer_m1_tropical_r2, explorer_m1_integration_r2).
2. Synthesize findings and dispatch the M1 Iteration 2 Remediation Worker to implement genuine Knuth-Bendix frequency propagation and a true tree-based Blelloch parallel prefix scan in sheaf_trellis.go directly connected to encodeTrellisStream.
3. Note succession protocol: you are at spawn count 15/16; prepare succession when hitting the 16 limit according to protocol.
4. Continue with Milestone 2 (4D IsoQuant in Go and C) and Milestone 3 (benchmarking & invariant verification).

## 2026-09-30T19:21:05Z

The server experienced a restart. Your persistent state in d:\CodingProjects\seal\.agents\teamwork\orchestrator_3 is preserved.
Monitoring crons have been restored.

Please resume execution immediately:
1. Finalize the Milestone 1 Iteration 3 zero-allocation remediation for sheafDecPool and SheafModeÉtale4, verifying 0 B/op, 0 allocs/op uniformly across all benchmarks.
2. Transition directly to Milestone 2: Ratio Enhancement Pipeline (4D IsoQuant quaternionic collimation across multi-channel and shader/binary streams in Go, with branchless vector decode in C decompressor przm_dec.c).
3. Continue through Milestone 3: Continuous Benchmarking and Invariant Verification (comparative Before/After tables detailing throughput, compression ratio, and zero allocations).
4. Re-register your internal heartbeat timer and proceed.

## 2026-09-30T19:52:51Z

The server experienced a restart. Your persistent state in d:\CodingProjects\seal\.agents\teamwork\orchestrator_3 is preserved.
Monitoring crons have been restored.

Please resume execution immediately:
1. Revive or collect reports from your active Milestone 2 explorers (explorer_m2_math, explorer_m2_go, explorer_m2_c).
2. Synthesize the M2 specifications and dispatch the implementation worker for 4D IsoQuant quaternionic collimation across multi-channel and shader/binary streams in Go, and branchless vector decode in C decompressor przm_dec.c.
3. Advance to Milestone 3: Continuous Benchmarking and Invariant Verification (comparative Before/After tables detailing throughput, compression ratio, and zero allocations).
4. Re-register your internal heartbeat timer and proceed.

## 2026-10-01T06:05:24Z

The server experienced a restart. Your persistent state in d:\CodingProjects\seal\.agents\teamwork\orchestrator_3 is preserved.
Sentinel monitoring crons have been restored.

Please resume execution immediately:
1. Collect the delivered handoff from reviewer_m2_2 and revive challenger_m2_2 and auditor_m2 to conclude the Milestone 2 Quality Gate.
2. Record Milestone 2 PASS in GATE_STATUS.md, PROJECT.md, and progress.md.
3. Advance directly to Milestone 3: Continuous Benchmarking and Invariant Verification, producing the required Before/After comparative benchmark table detailing throughput (MB/s), compression ratio, and 0-allocation verification across all datasets.
4. Re-register your internal heartbeat timer and proceed to final completion.
