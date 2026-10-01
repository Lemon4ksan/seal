# Dispatch: explorer_m1_r3_collate

## 2026-09-30T19:17:00Z
You are explorer_m1_r3_collate (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\explorer_m1_r3_collate
Target Codebase: d:\CodingProjects\przm

Read:
- ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
- PROJECT.md at: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md
- Challenger Rejection Report at: d:\CodingProjects\seal\.agents\teamwork\challenger_m1_2_r2\handoff.md

Problem:
In `sheaf_trellis.go:841-869`, `SheafModeÉtale4` currently decodes fiber streams into an intermediate buffer `sheafDec` obtained from `sheafDecPool`, then calls `SheafCollate4(dst, f0, f1, f2, f3)`.
If `sheafDecPool` is needed, it introduces memory management overhead.

Task:
1. Analyze whether `SheafCollate4` or fiber decoding can be performed directly into `dst` (in-place or interleaved decode) without needing an intermediate 16 MiB buffer at all!
2. Compare with `src/przm_dec.c:545` where C uses `tls_split_scratch`.
3. Check the memory layout of `dst`: `dst` has capacity `origLen`.
4. Formulate recommendations for the worker to achieve strictly zero allocations.
Write report to `report.md` and handoff to `handoff.md`. Notify me via send_message when done.
