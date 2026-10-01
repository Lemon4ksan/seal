# Dispatch: explorer_m2_c

## 2026-09-30T19:45:00Z
You are explorer_m2_c (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\explorer_m2_c
Target Codebase: `d:\CodingProjects\przm`

Read:
- ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
- PROJECT.md at: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md
- MATHEMATICAL_REPORT.md at: d:\CodingProjects\przm\MATHEMATICAL_REPORT.md (§6.4, §7.4 Proof 4, §7.5)

Task:
Investigate C reference decompressor integration for 4D IsoQuant branchless vector decode:
1. Header definitions in `include/przm_dec.h`:
   - Magic `PRZM_MAGIC_ISOQUANT` (`0x514F5349`).
   - Function declaration: `przm_status_t przm_isoquant_unrotate_inplace(uint8_t *dst, size_t len, const uint8_t qL_inv[4], const uint8_t qR_inv[4]);`.
2. Implementation in `src/przm_dec.c`:
   - Branchless vector decode using 16-byte SIMD / 64-bit SWAR unrolling.
   - Dispatch under `przm_get_uncompressed_size` and `przm_decompress`.
   - Strictly 0 heap allocations, in-place execution in $\le 22$ CPU clock cycles per 64 bytes.
3. C test harness updates in `tests/test_przm_dec.c` and Go interop test `c_interop_test.go`.
Write your full report to `report.md` and handoff to `handoff.md`. Notify me via send_message when done.
