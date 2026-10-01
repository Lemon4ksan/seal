# Dispatch: explorer_m2_go

## 2026-09-30T19:45:00Z
You are explorer_m2_go (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\explorer_m2_go
Target Codebase: `d:\CodingProjects\przm`

Read:
- ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
- PROJECT.md at: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md
- MATHEMATICAL_REPORT.md at: d:\CodingProjects\przm\MATHEMATICAL_REPORT.md (§6.4, §7.4, §7.5)

Task:
Investigate Go compression engine integration for 4D IsoQuant Collimation:
1. Design `isoquant.go`:
   - Data structures, functions (`IsoQuantCollimation4D`, `IsoQuantUncollimation4D`).
   - Frame header format: magic `PRZM_MAGIC_ISOQUANT` (`0x514F5349`), 20-byte header with $q_L, q_R$ inverse parameters and uncompressed length.
2. Integration into `compress.go` and `decompress.go`:
   - Mode gating: auto-detection on multi-channel, tabular-4, shader, and binary streams when collimation yields higher compression ratio.
   - Decompression path: zero dynamic heap allocations (`0 B/op, 0 allocs/op`).
3. Unit and roundtrip tests (`isoquant_test.go`):
   - Bit-exact lossless roundtrip across multiple test patterns.
   - Compression ratio improvement verification.
Write your full report to `report.md` and handoff to `handoff.md`. Notify me via send_message when done.
