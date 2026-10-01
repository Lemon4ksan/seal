# DISPATCH: worker_m3

- **Role**: Continuous Benchmarking & Invariant Verification Worker (teamwork_preview_worker)
- **Target Codebase**: `d:\CodingProjects\przm`
- **Assigned Milestone**: Milestone 3 — Continuous Benchmarking & Invariant Verification
- **Working Directory**: `d:\CodingProjects\seal\.agents\teamwork\worker_m3`
- **Inputs**:
  - `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md`
  - `d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md`
  - `d:\CodingProjects\seal\.agents\teamwork\worker_m1_r3\report.md` (M1 baseline & remediation metrics)
  - `d:\CodingProjects\seal\.agents\teamwork\worker_m2\report.md` (M2 IsoQuant metrics)
  - `d:\CodingProjects\przm\MATHEMATICAL_REPORT.md` (Original baseline benchmarks & theoretical bounds)

## Mandate
1. Run the comprehensive benchmark suite across all corpora (JSON, Tabular, MachineCode, Telemetry, Shader):
   ```powershell
   cd d:\CodingProjects\przm
   go test -bench="BenchmarkPRZM|BenchmarkSheafTrellis|BenchmarkGGTC|BenchmarkPWS|BenchmarkIsoQuant" -benchmem .
   ```
2. Run pure C decompressor verification and verify memory allocation count:
   ```powershell
   cd d:\CodingProjects\przm
   .\test_przm_dec.exe
   ```
3. Run cross-language Go-to-C interop verification:
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestPureCDecompressorInterop .
   ```
4. Verify all invariants:
   - Zero heap allocations (`0 B/op, 0 allocs/op`) across all decompressor implementations (Sheaf Trellis, PWS, IsoQuant, fused decompressors).
   - Instantaneous O(1) cycle ceiling: $\le 384$ CPU clock cycles per 64-byte block for Sheaf Trellis, $\le 22$ CPU clock cycles per 64-byte block for IsoQuant.
   - Bit-exact roundtrip losslessness (100% losslessness across all test corpora).
5. Compile comprehensive Before/After comparative performance tables:
   - Throughput (MB/s) Before vs. After (Compression & Decompression)
   - Compression Ratio Before vs. After (JSON, Tabular, MachineCode, Telemetry, Multi-Channel/Shader)
   - Heap Allocations (`B/op`, `allocs/op`) Before vs. After
   - Hardware cycle counts and microarchitectural L1D cache constraints
6. Write full engineering benchmark report to `d:\CodingProjects\seal\.agents\teamwork\worker_m3\report.md` and handoff to `d:\CodingProjects\seal\.agents\teamwork\worker_m3\handoff.md`. Notify orchestrator via `send_message`.

## 2026-10-01T06:15:31Z
Execute Milestone 3: Continuous Benchmarking and Invariant Verification:
1. Run full benchmark suite: `cd d:\CodingProjects\przm; go test -bench="BenchmarkPRZM|BenchmarkSheafTrellis|BenchmarkGGTC|BenchmarkPWS|BenchmarkIsoQuant" -benchmem .`
2. Run pure C verification: `cd d:\CodingProjects\przm; .\test_przm_dec.exe`
3. Run cross-language interop: `cd d:\CodingProjects\przm; go test -v -run TestPureCDecompressorInterop .`
4. Verify all invariants:
   - 0 heap allocations (0 B/op, 0 allocs/op) across all decompressors.
   - Instantaneous decompression O(1) cycle ceiling (<= 384 cycles/64B, <= 22 cycles/64B for IsoQuant).
   - Bit-exact losslessness across all corpora.
5. Generate comprehensive Before/After comparative performance tables:
   - Throughput (MB/s) Before vs. After (Compression & Decompression).
   - Compression Ratio Before vs. After across all datasets.
   - Allocation count Before vs. After.
6. Write detailed report to report.md and handoff to handoff.md. Notify me via send_message.

