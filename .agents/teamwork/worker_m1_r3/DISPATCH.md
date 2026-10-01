# Dispatch: worker_m1_r3

## 2026-09-30T19:30:00Z
You are worker_m1_r3 (teamwork_preview_worker).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\worker_m1_r3
Target Codebase: d:\CodingProjects\przm

Read:
- ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
- PROJECT.md at: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md
- Challenger Rejection Report at: d:\CodingProjects\seal\.agents\teamwork\challenger_m1_2_r2\handoff.md
- Remediation handoff at: d:\CodingProjects\seal\.agents\teamwork\worker_m1_r2\handoff.md

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

Root Cause:
In `BenchmarkSheafTrellis_Decompress`, `MachineCode64K` and `MachineCode256K` report non-zero byte allocations (`866 B/op` and `3377 B/op`).
The cause is `sheafDecPool` in `sheaf_trellis.go:398-401`:
```go
var sheafDecPool = generic.NewPool(func() *[]byte {
    b := bufkit.AlignedBytes(MaxBlockSize+64, bufkit.CacheLineSize)
    return &b
})
```
`generic.NewPool` wraps `sync.Pool`, which Go's GC sweeps during large benchmark runs. When GC purges the pool, `sheafDecPool.Get()` misses and allocates a full 16 MiB (`MaxBlockSize+64`) on the heap! Furthermore, `sheaf_trellis_test.go` lacks a warmup call before `b.ResetTimer()`.

Task:
1. Fix `sheafDecPool` in `sheaf_trellis.go`:
   - Replace `generic.NewPool` / `sync.Pool` with a deterministic, non-GC-swept persistent buffer pool:
     Use a buffered channel with non-reclaimable capacity:
     ```go
     var sheafDecChan = make(chan []byte, 32)
     func getSheafDecBuf(minCap int) []byte {
         select {
         case b := <-sheafDecChan:
             if cap(b) >= minCap {
                 return b
             }
             return bufkit.AlignedBytes(minCap, bufkit.CacheLineSize)
         default:
             return bufkit.AlignedBytes(minCap, bufkit.CacheLineSize)
         }
     }
     func putSheafDecBuf(b []byte) {
         select {
         case sheafDecChan <- b:
         default:
         }
     }
     ```
     Or size buffers to `origLen + 64` (not 16 MiB static over-allocation), or maintain persistent per-goroutine / thread-safe scratchpads.
2. In `sheaf_trellis_test.go:BenchmarkSheafTrellis_Decompress`:
   - Add a warmup run `_, _ = przm.DecompressSheafTrellis(comp, dst)` right before `b.ResetTimer()` to ensure any one-time pool population is warmed up.
3. Build and test:
   - `cd d:\CodingProjects\przm; go build ./...`
   - `cd d:\CodingProjects\przm; go test -v .`
   - `cd d:\CodingProjects\przm; .\test_przm_dec.exe`
   - `cd d:\CodingProjects\przm; go test -v -run TestPureCDecompressorInterop .`
4. Run benchmarks and verify strictly ZERO heap allocations:
   - `cd d:\CodingProjects\przm; go test -bench="BenchmarkSheafTrellis_Decompress" -benchmem .`
   - Verify that **EVERY** sub-benchmark (`MachineCode64K`, `MachineCode256K`, `JSON64K`, `JSON256K`, `Tabular64K`) reports strictly:
     `0 B/op, 0 allocs/op`
5. Write your report to `report.md` and handoff to `handoff.md`. Notify me via send_message when done.
