# Dispatch for worker_m3

## Identity
- Archetype: teamwork_preview_worker
- Role: Milestone 3 Cellular Sheaves & Discrete Hodge Laplacians Specialist
- Working directory: d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m3
- Parent Orchestrator: orchestrator_5 (conv ID: 1eb7944e-37f1-4b94-a209-66caae11aab3)
- Target codebase: d:\CodingProjects\przm

## Mandatory References
Read before starting work:
- `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md` (Verbatim user request history)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\PROJECT.md` (Project specification)
- `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\survey_explorer_1\report.md` (Comprehensive specification of Cellular Sheaves with Discrete Hodge Laplacians, boundary operators, harmonic projection, and vector row sweep decode)
- `d:\CodingProjects\przm\dec_hhd.go`, `d:\CodingProjects\przm\tda.go`, `d:\CodingProjects\przm\compress.go`, `d:\CodingProjects\przm\decompress.go`, `d:\CodingProjects\przm\src\przm_dec.c`, `d:\CodingProjects\przm\include\przm_dec.h`

## Write Ownership
You exclusively own:
- `d:\CodingProjects\przm\cellular_sheaf.go` (New)
- `d:\CodingProjects\przm\cellular_sheaf_test.go` (New)
- `d:\CodingProjects\przm\compress.go` (Add Cellular Sheaf candidate gating & Pareto arbitration)
- `d:\CodingProjects\przm\decompress.go` (Dispatch magic `0x44485343` "CSHD")
- `d:\CodingProjects\przm\include\przm_dec.h` (Magic `0x44485343`, prototype `przm_cellular_sheaf_decompress`)
- `d:\CodingProjects\przm\src\przm_dec.c` (Implement `przm_cellular_sheaf_decompress` and dispatch in `przm_decompress`)
- `d:\CodingProjects\przm\tests\test_przm_dec.c` (Add test case for Cellular Sheaf decompression)

## Mandatory Integrity Warning
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

## Objective
Implement Milestone 3: **Cellular Sheaves with Discrete Hodge Laplacians**:
1. Implement `cellular_sheaf.go`:
   - Construct 2D regular cell complex $X = (X_0, X_1, X_2)$ of dimensions $M \times W$ from data streams.
   - Define restriction maps and coboundaries: 0-coboundary $d_0$ and 1-coboundary $d_1$ over $\mathbb{F}_{2^8}$ (modulo AES polynomial `0x11B`), satisfying $d_1 \circ d_0 \equiv 0$.
   - Compute discrete Hodge Laplacians: $\Delta_0 = d_0^T d_0$ and $\Delta_1 = d_1^T d_1 + d_0 d_0^T$.
   - Project out harmonic 1-forms ($\ker \Delta_1 = H^1(X; \mathcal{F})$) to annihilate circulating cycles, mapping the residual to a pure gradient $d_0 \phi$.
   - Encode into compact boundary representation: 1 anchor byte, boundary 1-forms, homology coefficients, and sparse face curls.
   - Decode via vectorized row sweep: $\phi(i, j) = \phi(i-1, j) \oplus V(i, j)$.
   - Frame Magic: `PRZM_MAGIC_CELLULAR_SHEAF = 0x44485343` ("CSHD", Little-Endian).
   - 24-byte header: `[0:4]` Magic, `[4:8]` UncompressedSize (uint32), `[8:10]` Width (uint16), `[10:12]` Height (uint16), `[12]` Anchor (uint8), `[13]` GaloisTwist (uint8), `[14:16]` Betti1 (uint16), `[16:18]` Flags (uint16), `[18:24]` Reserved (6B = 0).
   - `CompressCellularSheaf(src []byte, level int) ([]byte, error)` and `DecompressCellularSheaf(src, dst []byte) ([]byte, error)`.
2. Integrate into `compress.go` and `decompress.go`:
   - In `compress.go`: evaluate `CompressCellularSheaf` for structured/tabular/graph-like data, participating in `MinNonEmpty` Pareto arbitration.
   - In `decompress.go`: dispatch `case PRZM_MAGIC_CELLULAR_SHEAF:` in `Decompress` and `DecompressTo`.
3. Implement Pure C reference decompressor extension:
   - In `include/przm_dec.h`: define `PRZM_MAGIC_CELLULAR_SHEAF 0x44485343` and `PRZM_CELLULAR_SHEAF_HEADER_SIZE 24`.
   - Prototype `int64_t przm_cellular_sheaf_decompress(const uint8_t *src, size_t src_len, uint8_t *dst, size_t dst_cap);`.
   - In `src/przm_dec.c`: implement `przm_cellular_sheaf_decompress` with unrolled vector row sweeps and strictly 0 heap allocations (`malloc`/`free` = 0).
   - In `przm_decompress`: dispatch `PRZM_MAGIC_CELLULAR_SHEAF`.
   - In `tests/test_przm_dec.c`: add unit test `test_cellular_sheaf_decompression`.
4. Comprehensive test coverage:
   - In `cellular_sheaf_test.go`: verify coboundary identity $d_1 \circ d_0 \equiv 0$, Hodge Laplacian harmonic projection, cycle annihilation, boundary representation recovery, 0 heap allocations during decompression (`0 B/op, 0 allocs/op`), line rate (> 20 GB/s), and compression ratio gains.
5. Verify builds and tests:
   - `go build ./...`
   - `go test -v -run TestCellularSheaf .`
   - `gcc -O3 -Wall -Wextra -Werror -Iinclude src/przm_dec.c tests/test_przm_dec.c -o test_przm_dec.exe && .\test_przm_dec.exe`
   - `lake build`
   - Full regression: `go test -count=1 .`
6. Language neutrality: Strictly general-purpose mathematical algorithms. Zero hardcoded language dictionaries.

## Deliverables
Write your completion report to `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m3\report.md` and deliver `handoff.md`.

## 2026-10-01T11:53:22Z
Resume work at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m3. Read your DISPATCH.md at d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\worker_m3\DISPATCH.md. Also read d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md, PROJECT.md, and survey_explorer_1 report. Implement Cellular Sheaves with Discrete Hodge Laplacians in Go (cellular_sheaf.go, compress.go, decompress.go) and Pure C reference decompressor (przm_dec.h, przm_dec.c, test_przm_dec.c). Run all tests and builds. Deliver report.md and handoff.md.

