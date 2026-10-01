# Dispatch for worker_m2

## Identity
- Archetype: teamwork_preview_worker
- Role: Milestone 2 Heisenberg Foliation Implementation Specialist
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m2
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\survey_explorer_3\report.md` (Comprehensive specification of Heisenberg group $H_3(\mathbb{F}_{2^8})$ foliation, C SWAR decode, and Go integration)
- `d:\CodingProjects\przm\isoquant.go`, `d:\CodingProjects\przm\compress.go`, `d:\CodingProjects\przm\decompress.go`, `d:\CodingProjects\przm\src\przm_dec.c`, `d:\CodingProjects\przm\include\przm_dec.h`

## Write Ownership
You exclusively own:
- `d:\CodingProjects\przm\heisenberg.go` (New)
- `d:\CodingProjects\przm\heisenberg_test.go` (New)
- `d:\CodingProjects\przm\compress.go` (Add Heisenberg candidate gating & Pareto arbitration)
- `d:\CodingProjects\przm\decompress.go` (Dispatch magic `0x53494548`)
- `d:\CodingProjects\przm\include\przm_dec.h` (Magic `0x53494548`, prototype `przm_heisenberg_unfoliate_inplace`)
- `d:\CodingProjects\przm\src\przm_dec.c` (Implement `przm_heisenberg_unfoliate_inplace` and dispatch in `przm_decompress`)
- `d:\CodingProjects\przm\tests\test_przm_dec.c` (Add test case for Heisenberg unfoliation)
- `d:\CodingProjects\przm\c_interop_test.go` (Add interop test for Heisenberg frames)

## Mandatory Integrity Warning
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## Objective
Implement Milestone 2: **Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$ Foliation**:
1. Implement `heisenberg.go`:
   - Group multiplication: $(x, y, z) \circ (x', y', z') = (x \oplus x', y \oplus y', z \oplus z' \oplus (x \odot y'))$ over $\mathbb{F}_{2^8}$ (modulo AES polynomial `0x11B`).
   - Algebraic inversion: $H^{-1}(x, y, z) = H(x, y, z \oplus (x \odot y))$.
   - In-place foliation and unfoliation functions:
     `HeisenbergFoliateInplace(data []byte, stride int)`
     `HeisenbergUnfoliateInplace(data []byte, stride int)`
     Stride supported: 3 (RGB, 3-component tensors) and 4 (RGBA, 4-component tensors, with channel 3 pass-through).
   - Heuristic covariance estimator: `EstimateOptimalHeisenberg(src []byte) (stride int, shouldApply bool)`
   - `CompressHeisenberg(src []byte, level int) ([]byte, error)` and `DecompressHeisenberg(src, dst []byte) ([]byte, error)`.
   - Magic: `0x53494548` ("HEIS", Little-Endian).
   - Header (12 bytes): `[0:4]` Magic, `[4:8]` UncompressedSize (uint32), `[8]` Stride (uint8), `[9]` Flags (uint8), `[10:12]` Reserved (0).
   - Frame payload compressed via inner standard/PWS encoder.
2. Integrate into `compress.go` and `decompress.go`:
   - In `compress.go`: evaluate `CompressHeisenberg` for structured/tensor/color-channel data and route through `MinNonEmpty` Pareto arbitration.
   - In `decompress.go`: add `case PRZM_MAGIC_HEISENBERG:` in `Decompress` and `DecompressTo`.
3. Implement Pure C reference decompressor extension:
   - In `include/przm_dec.h`: define `PRZM_MAGIC_HEISENBERG 0x53494548` and `PRZM_HEISENBERG_HEADER_SIZE 12`.
   - Prototype `int przm_heisenberg_unfoliate_inplace(uint8_t *dst, size_t len, uint8_t stride);`.
   - In `src/przm_dec.c`: implement `przm_heisenberg_unfoliate_inplace` using SWAR unrolling and in-place Galois multiplication (`gf256_mul(x, y)`), with strictly zero heap allocations (`malloc`/`free` = 0).
   - In `przm_decompress`: dispatch `PRZM_MAGIC_HEISENBERG` frames.
   - In `tests/test_przm_dec.c`: add unit test `heisenberg_unfoliation` verifying bit-exact roundtrip against test vectors.
4. Comprehensive test coverage:
   - In `heisenberg_test.go`: test involution property ($H^{-1} \circ H = \mathrm{id}$), stride 3 and 4 roundtrips, boundary lengths (unaligned lengths, short payloads), 0 heap allocations during decompression (`0 B/op, 0 allocs/op`), and compression ratio gain on 3D/4D tensor and color-channel data.
   - In `c_interop_test.go`: verify Go-compressed Heisenberg frames are decoded 100% losslessly by `test_przm_dec.exe`.
5. Verify builds and tests:
   - `go build ./...`
   - `go test -v -run TestHeisenberg .`
   - `go test -v -run TestPureCDecompressorInterop .`
   - `gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe && .\test_przm_dec.exe`
   - `lake build`
6. Language neutrality: Strictly general-purpose mathematical transforms. Zero hardcoded language dictionaries.

## Deliverables
Write your completion report to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m2\report.md` and deliver `handoff.md`.

## 2026-10-01T11:24:54Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m2. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m2\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md, and survey_explorer_3 report. Implement Discrete Heisenberg Group H_3(F_{2^8}) foliation in Go (heisenberg.go, compress.go, decompress.go) and Pure C reference decompressor (przm_dec.h, przm_dec.c, test_przm_dec.c). Run all tests and builds. Deliver report.md and handoff.md.

## 2026-10-01T11:40:22Z
**Context**: Milestone 2 Heisenberg Implementation Status Check
**Content**: Please report your current progress on implementing `heisenberg.go`, Pure C decompressor updates, and tests.
**Action**: Provide a status update and estimated completion.


