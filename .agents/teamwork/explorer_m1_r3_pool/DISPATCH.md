# Dispatch: explorer_m1_r3_pool

## 2026-09-30T19:17:00Z
You are explorer_m1_r3_pool (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\explorer_m1_r3_pool
Target Codebase: d:\CodingProjects\przm

Read:
- ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
- PROJECT.md at: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md
- Challenger Rejection Report at: d:\CodingProjects\seal\.agents\teamwork\challenger_m1_2_r2\handoff.md

Problem:
In `BenchmarkSheafTrellis_Decompress`, `MachineCode64K` and `MachineCode256K` report non-zero byte allocations (`866 B/op` and `3377 B/op`).
The cause is `sheafDecPool` in `sheaf_trellis.go:398-401`:
```go
var sheafDecPool = generic.NewPool(func() *[]byte {
    b := bufkit.AlignedBytes(MaxBlockSize+64, bufkit.CacheLineSize)
    return &b
})
```
`generic.NewPool` wraps `sync.Pool`, which Go's GC sweeps. When GC runs, the pool is purged, and subsequent decompression calls allocate 16 MiB (`MaxBlockSize+64`) on the heap!

Task:
1. Investigate how to eliminate `sync.Pool` GC purging for `sheafDecPool`.
2. Evaluate non-GC-swept alternatives:
   - A persistent, bounded channel pool (e.g. `make(chan *[]byte, 64)`) that is NEVER swept by GC.
   - Sizing the buffer to `origLen + 64` or a tiered pool instead of 16 MiB.
   - Per-P / thread-local scratchpads.
3. Formulate concrete, non-allocating design recommendations for the implementation worker.
Write report to `report.md` and handoff to `handoff.md`. Notify me via send_message when done.
