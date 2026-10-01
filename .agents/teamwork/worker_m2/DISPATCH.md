# Dispatch: worker_m2

## 2026-09-30T19:54:00Z
You are worker_m2 (teamwork_preview_worker).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\worker_m2
Target Codebase: d:\CodingProjects\przm

Read:
- ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
- PROJECT.md at: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md
- Mathematics Explorer Report at: d:\CodingProjects\seal\.agents\teamwork\explorer_m2_math\report.md (and handoff.md)
- Go Engine Explorer Report at: d:\CodingProjects\seal\.agents\teamwork\explorer_m2_go\report.md (and handoff.md)

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

Architectural Directive:
The core mathematical implementation must remain strictly general-purpose and language-neutral. Do not hardcode syntax dictionaries or language-specific tokens.

Tasks:
1. Implement Go 4D IsoQuant Quaternionic Collimation:
   - Create `isoquant.go`:
     - Define `Quaternion4` ($q = (q_0, q_1, q_2, q_3)$).
     - Implement multiplication `Mul(a, b Quaternion4) Quaternion4` over Galois field $\mathbb{F}_{2^8}$ (polynomial `0x11B`) and exact inversion `Inv(q Quaternion4) Quaternion4`.
     - Implement forward isoclinic rotation `IsoQuantCollimation4D(dst, src []byte, qL, qR Quaternion4)`.
     - Implement in-place inverse isoclinic rotation `IsoQuantUncollimation4DInplace(dst []byte, qLInv, qRInv Quaternion4)`.
     - Implement `EstimateOptimalIsoQuant(src []byte) (qL, qR Quaternion4, benefited bool)` analyzing 4-channel cross-covariance.
     - Frame Header: 20 bytes aligned:
       - Bytes 0..3: Magic `PRZM_MAGIC_ISOQUANT` (`0x514F5349`)
       - Byte 4: Mode (`uint8`): Mode 0 (Raw/Direct), Mode 1 (Collimated), Mode 3 (Collimated + PWS/LZ cascade)
       - Bytes 5..7: Reserved (`uint8[3]`)
       - Bytes 8..11: Uncompressed length `orig_len` (`uint32_le`)
       - Bytes 12..15: $q_{L,\text{inv}}$ (`uint8[4]`)
       - Bytes 16..19: $q_{R,\text{inv}}$ (`uint8[4]`)
     - Implement `CompressIsoQuant(src []byte) ([]byte, error)` and `DecompressIsoQuant(src, dst []byte) ([]byte, error)`.
     - Decompression MUST strictly maintain 0 heap allocations (`0 B/op, 0 allocs/op`).
   - Integrate into `compress.go`:
     - In `Compress`, evaluate `isCandidateShaderOrVec4` / `hasChannelCorrelation4`.
     - Include `isoCandidate` in Pareto candidate arbitration (`MinNonEmpty`).
   - Integrate into `decompress.go`:
     - Dispatch `PRZM_MAGIC_ISOQUANT` (`0x514F5349`) to `DecompressIsoQuant`.
   - Create `isoquant_test.go`:
     - Test algebraic invertibility and losslessness across random, boundary, multi-channel, tabular, and shader streams.
     - Test decompression zero allocations (`0 B/op, 0 allocs/op`).
     - Test compression ratio improvement on multi-channel/shader streams.

2. Implement Pure C Reference Decompressor for 4D IsoQuant:
   - In `include/przm_dec.h`:
     - Add `#define PRZM_MAGIC_ISOQUANT 0x514F5349`.
     - Declare `przm_status_t przm_isoquant_unrotate_inplace(uint8_t *dst, size_t len, const uint8_t qL_inv[4], const uint8_t qR_inv[4]);`.
   - In `src/przm_dec.c`:
     - Implement branchless vector decode `przm_isoquant_unrotate_inplace` using precomputed effective quaternion $q_{\text{eff}} = q_{L,\text{inv}} \cdot q_{R,\text{inv}}$ or 2-stage multiplication over $\mathbb{F}_{2^8}$ via `gf256_mul`.
     - In `przm_get_uncompressed_size`: handle `PRZM_MAGIC_ISOQUANT` reading `orig_len` at offset 8.
     - In `przm_decompress`: dispatch `PRZM_MAGIC_ISOQUANT` to decode payload, then apply `przm_isoquant_unrotate_inplace(dst, orig_len, qL_inv, qR_inv)`.
     - Invariant: 0 heap allocations, $\le 22$ CPU cycles per 64 bytes.
   - Update `tests/test_przm_dec.c`:
     - Add `test_isoquant_uncollimation` test case verifying bit-exact inverse isoclinic rotation.
     - Rebuild `test_przm_dec.exe` via GCC: `gcc -O3 -Wall -Wextra -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe`.
   - Update `c_interop_test.go`:
     - Add Go-compressed IsoQuant frames to `TestPureCDecompressorInterop` to verify cross-language bit-exact losslessness.

3. Build, Test, and Benchmark:
   - `cd d:\CodingProjects\przm; go build ./...`
   - `cd d:\CodingProjects\przm; go test -v .`
   - `cd d:\CodingProjects\przm; .\test_przm_dec.exe`
   - `cd d:\CodingProjects\przm; go test -v -run TestPureCDecompressorInterop .`
   - `cd d:\CodingProjects\przm; go test -bench="BenchmarkIsoQuant|BenchmarkSheafTrellis|BenchmarkGGTC|BenchmarkPWS|BenchmarkPRZM" -benchmem .`
   - Record explicit Before/After comparative metrics (MB/s throughput, compression ratio, allocation count).

Write your report to `report.md` and handoff to `handoff.md`. Notify me via send_message when done.
