# DISPATCH: explorer_survey_c

## Mission
You are a read-only exploration agent (`teamwork_preview_explorer`).
Your working directory is: `d:\CodingProjects\seal\.agents\teamwork\explorer_survey_c`
Read `ORIGINAL_REQUEST.md` at: `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md`
Target Codebase: `d:\CodingProjects\przm`

## Objectives
1. Thoroughly investigate the C reference decompressor and verification tests in `d:\CodingProjects\przm`.
2. Analyze `przm_dec.c`, `test_przm_dec.c` (or equivalent test harness), build instructions/scripts/makefiles for `test_przm_dec.exe`.
3. Inspect the decompression loop, table lookups, branchless structure, and how it guarantees instantaneous O(1) decompression and 0 heap allocations.
4. Determine how 4D IsoQuant quaternionic collimation vector decoding can be implemented branchlessly in C without heap allocations.
5. Survey how C decompressor tests are currently compiled and run, verifying their pass criteria.
6. Write your detailed exploration report to `d:\CodingProjects\seal\.agents\teamwork\explorer_survey_c\report.md` and your final handoff to `d:\CodingProjects\seal\.agents\teamwork\explorer_survey_c\handoff.md`.

## 2026-09-30T16:18:43Z
Sender: 449d1fb0-27f3-4579-bba0-13f645f0337b
Content:
You are explorer_survey_c (codebase explorer).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\explorer_survey_c
Read your dispatch at: d:\CodingProjects\seal\.agents\teamwork\explorer_survey_c\DISPATCH.md
Read ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
Target codebase: d:\CodingProjects\przm

Investigate the C reference decompressor:
1. `przm_dec.c`, `test_przm_dec.c` (or equivalent test harness), build instructions and scripts for `test_przm_dec.exe`.
2. Existing decompression loop, table lookups, branchless structure, 0 heap allocation guarantee.
3. Integration design for 4D IsoQuant quaternionic collimation branchless vector decode.
4. Existing test harness execution, verification checks, and pass/fail criteria.

Write your complete analysis to d:\CodingProjects\seal\.agents\teamwork\explorer_survey_c\report.md and your final handoff to d:\CodingProjects\seal\.agents\teamwork\explorer_survey_c\handoff.md. When done, notify me via send_message.

