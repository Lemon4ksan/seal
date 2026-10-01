# BRIEFING — 2026-09-30T20:05:00Z

## Mission
Implement 4D IsoQuant Quaternionic Collimation across Go and Pure C in przm, verify instantaneous O(1) zero-allocation decompression, and benchmark results.

## 🔒 My Identity
- Archetype: teamwork_preview_worker
- Roles: implementer, qa, specialist
- Working directory: d:\CodingProjects\seal\.agents\teamwork\worker_m2
- Original parent: 449d1fb0-27f3-4579-bba0-13f645f0337b
- Milestone: M2 (4D IsoQuant Quaternionic Collimation)

## 🔒 Key Constraints
- DO NOT CHEAT: Genuine implementation only, no hardcoded test results, no dummy facades, no circumventing intended task.
- Strictly general-purpose and language-neutral core mathematical implementation; no hardcoded syntax dictionaries or language-specific tokens.
- Instantaneous decompression invariant: strictly 0 heap allocations (0 B/op, 0 allocs/op) and <= 22 CPU clock cycles per 64 bytes.
- Frame Header: 20 bytes aligned with magic PRZM_MAGIC_ISOQUANT (0x514F5349).
- Pure C reference decompressor must build with GCC and pass all tests including C interop.

## Current Parent
- Conversation ID: 449d1fb0-27f3-4579-bba0-13f645f0337b
- Updated: not yet

## Task Summary
- **What to build**: 
  1. `isoquant.go` in `d:\CodingProjects\przm`: Quaternion4, Mul, Inv, IsoQuantCollimation4D, IsoQuantUncollimation4DInplace, EstimateOptimalIsoQuant, PRZM_MAGIC_ISOQUANT 0x514F5349 20-byte header, CompressIsoQuant, DecompressIsoQuant.
  2. Integration into `compress.go` and `decompress.go`.
  3. `isoquant_test.go` with unit, boundary, losslessness, zero-alloc, and compression ratio tests.
  4. Pure C decompressor updates in `include/przm_dec.h`, `src/przm_dec.c`, `tests/test_przm_dec.c`.
  5. Build `test_przm_dec.exe` via GCC and update `c_interop_test.go`.
  6. Run build, test, and benchmark commands, record Before/After comparative metrics.
- **Success criteria**: All Go and C tests pass, 0 allocs/op in DecompressIsoQuant, verified ratio improvement on multi-channel/shader streams, zero regressions.
- **Interface contracts**: `d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md` § Interface Contracts
- **Code layout**: `d:\CodingProjects\przm`

## Key Decisions Made
- Use Walsh-Hadamard index convolution for Hamilton quaternion multiplication over F_{2^8}, exploiting group algebra F_{2^8}[Z_2 x Z_2].
- Inversion over F_{2^8} using s = q0^q1^q2^q3 != 0 and sSq = GF256Mul(s,s), inv = GF256Inv(sSq).
- In decompressor, pre-multiply qEff = qL_inv * qR_inv once per frame to halve inner loop work and guarantee microarchitectural budget (<= 22 cycles / 64 bytes).
- Strictly adhere to 20-byte aligned frame header and zero heap allocation in decompressor.

## Artifact Index
- `report.md` — Technical report of implementation, verification, and benchmarks.
- `handoff.md` — 5-component handoff report.
- `progress.md` — Liveness heartbeat and milestone progress.

## Change Tracker
- **Files modified**:
  - `isoquant.go`: 4D IsoQuant Quaternionic Collimation module implementation.
  - `isoquant_test.go`: Unit, roundtrip, boundary, zero-alloc, and benchmark tests.
  - `compress.go`: Integrated isCandidateShaderOrVec4, CompressIsoQuant, and Pareto MinNonEmpty arbitration.
  - `decompress.go`: Dispatched IsoQuantMagic (0x514F5349) to DecompressIsoQuant.
  - `include/przm_dec.h`: Added przm_status_t, PRZM_MAGIC_ISOQUANT, PRZM_ISOQUANT_HEADER_SIZE, and przm_isoquant_unrotate_inplace prototype.
  - `src/przm_dec.c`: Implemented przm_isoquant_unrotate_inplace, size reading, and decompress dispatch.
  - `tests/test_przm_dec.c`: Added test_isoquant_uncollimation and size test.
  - `c_interop_test.go`: Added IsoQuantMultiChannelRGBA, IsoQuantShaderSPIRV, and DirectIsoQuantFrame interop tests.
- **Build status**: PASS (go build ./..., go test -v ., .\test_przm_dec.exe, go test -v -run TestPureCDecompressorInterop . all pass).
- **Pending issues**: Awaiting final comparative benchmark suite completion.

## Quality Status
- **Build/test result**: All Go unit/roundtrip tests PASS; all Pure C tests PASS; cross-language interop PASS.
- **Lint status**: Clean (no compiler warnings under gcc -O3 -Wall -Wextra, go build clean).
- **Tests added/modified**: 11 new Go unit/roundtrip tests in `isoquant_test.go`, 1 new C unit test in `test_przm_dec.c`, 3 new C-interop test cases in `c_interop_test.go`.

## Loaded Skills
- None.
