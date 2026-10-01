# Progress — Challenger 1 (Empirical & Complexity Challenger)

Last visited: 2026-09-30T15:36:00Z

## Status
- Step 1: Read ORIGINAL_REQUEST.md — COMPLETED
- Step 2: Read & inspect Sections 5, 6, 7, 8 of MATHEMATICAL_REPORT.md — COMPLETED
- Step 3: Cross-check explorer reports (explorer_code_3_gen2, explorer_code_1_gen2) — COMPLETED
- Step 4: Formulate empirical challenges & write/execute empirical test harnesses — COMPLETED
  - Tested Tropical Prefix Scan vs Sequential DP: Prefix scan requires 16.54x MORE total work than sequential Viterbi! Falsifies claim of 4.2x speedup on single-thread/CPU.
  - Tested 4D IsoQuant Determinant formula: det(Sigma) / prod(lambda_m) == 1.0 identically by fundamental linear algebra, rendering Delta H == 0.0 bits. Falsifies entropy reduction formula.
  - Measured NSS-CE throughput: Measured 281.55 MB/s vs 28,150 MB/s claimed in Table 8.2 (100x discrepancy).
  - Measured decompression memory allocations: VMDecode (1 alloc), NSSCEDecode (1 alloc), DECHHDDecode (3 to 17 allocs), Decompress (1 to 3 allocs) vs claimed 0 allocs/op across all kernels.
  - Verified comparative benchmark tables: Models PRZM + Sheaf & IsoQuant, PRZM + Confluent Rewrite, PRZM 4D IsoQuant do not exist in the codebase, and zstd/lz4/snappy/brotli are not linked in the benchmark suite.
- Step 5: Synthesize findings and write handoff.md — IN PROGRESS
- Verdict: REQUEST_CHANGES
