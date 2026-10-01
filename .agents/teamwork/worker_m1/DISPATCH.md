# DISPATCH: worker_m1

## Mission
You are the Implementation Worker (`teamwork_preview_worker`) for Milestone 1: Acceleration Pipeline Implementation.
Your working directory is: `d:\CodingProjects\seal\.agents\teamwork\worker_m1`
Read `ORIGINAL_REQUEST.md` at: `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md`
Read `PROJECT.md` at: `d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md`
Read Explorer Findings at:
- `d:\CodingProjects\seal\.agents\teamwork\explorer_m1_knuth\report.md` (and `handoff.md`)
- `d:\CodingProjects\seal\.agents\teamwork\explorer_m1_tropical\report.md` (and `handoff.md`)

Target Codebase: `d:\CodingProjects\przm`

## MANDATORY INTEGRITY WARNING
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## Write Ownership
You exclusively own and may edit or create the following files in `d:\CodingProjects\przm`:
- `knuth_bendix.go` (new implementation file)
- `knuth_bendix_test.go` (new unit test file)
- `sheaf_trellis.go`
- `ggtc.go`
- `pws.go`
- `compress.go`
- Any corresponding test files (`sheaf_trellis_test.go`, `pws_test.go`, `ggtc_test.go`)

## Implementation Tasks

### Task 1: Tropical Associative Prefix Scans in `sheaf_trellis.go`
1. Implement SWAR match gating in `encodeTrellisStream` (16-bit prefix checks for RepQueue and 32-bit prefix checks for explicit matches) to reject non-matching candidate probes in 1 CPU cycle.
2. Formulate state transitions over the tropical min-plus semiring $(\mathbb{Z} \cup \{\infty\}, \min, +)$ across the 4-slot `RepQueue4`.
3. Implement the parallel associative reduction tree / SIMD Viterbi DP evaluation for horizon parsing, collapsing critical path span from $O(H)$ to $O(\log_2 H)$.
4. Ensure zero heap allocations during parsing via pre-allocated scratch arrays in `sheafScratchpad`.
5. Strictly preserve Phase 14 token syntax and decompressor invariance ($O(1)$ instantaneous token decode in decompressor, 0 B/op, 0 allocs/op).

### Task 2: Knuth-Bendix Confluent Term Rewriting
1. Implement `knuth_bendix.go` with:
   - Stratified term rewriting system $R = (\Sigma, \to)$ bounded by termination ordering $D \le 5$, terminal length $\le 32$ B.
   - Critical pair resolution $\langle \ell_1, \ell_2 \rangle$ ($A \to BC, C \to DE \implies A \to BDE$) to remove redundant non-terminals and resolve overlapping digram conflicts.
   - `KBScratchpad`, `KBConfluentSubstitute`, `FoldCanonicalGrammar5`, and unified 5-layer Maneth utility pruning `PruneGrammar5`.
2. Integrate into `ggtc.go` (`induceGrammarWithScratchpad`) and `pws.go` (`induceGrammar5WithScratchpad`).
3. Refine `compress.go:926-954` (`findBest`) to resolve match redex conflicts via deterministic reduction ordering.
4. Guarantee that decompressor expansion unrolls into flat static tables (`ExpL1`..`ExpL5`) in $\le 6$ CPU clock cycles with 0 recursion and 0 heap allocations.

### Task 3: Build, Verify & Benchmark
1. Run `go build ./...` and ensure clean compilation.
2. Run `go test -v .` and verify 100% pass across all unit tests and lossless roundtrip tests.
3. Run `.\test_przm_dec.exe` and `go test -v -run TestPureCDecompressorInterop .` to verify pure C decompressor passes 100% losslessly with 0 allocations.
4. Run benchmarks:
   - `go test -bench='BenchmarkSheafTrellis|BenchmarkGGTC|BenchmarkPWS|BenchmarkPRZM' -benchmem .`
5. Record explicit Before/After comparative metrics: throughput (MB/s), compression ratio, and allocation counts.
6. Write your comprehensive report to `d:\CodingProjects\seal\.agents\teamwork\worker_m1\report.md` and handoff to `d:\CodingProjects\seal\.agents\teamwork\worker_m1\handoff.md`.
