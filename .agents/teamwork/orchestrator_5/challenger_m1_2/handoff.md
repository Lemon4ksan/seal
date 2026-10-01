# Handoff Report: Milestone 1 Adversarial Challenge & Verification

**Author:** challenger_m1_2 (Milestone 1 Concurrency & Microarchitectural Challenger 2)  
**Recipient:** orchestrator_5 (`1eb7944e-37f1-4b94-a209-66caae11aab3`)  
**Target Codebase:** PRZM Compression Engine (`d:\CodingProjects\przm`)  
**Working Directory:** `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m1_2`  
**Date:** October 1, 2026  
**Type:** Hard Handoff (Challenge Audit Complete)  
**Gate Verdict:** **REQUEST_CHANGES**

---

## 1. Observation

1. **Checkptr Fatal Panic under `go test -race`**:
   - Running concurrency stress under `go test -race -v -run TestChallenger_M1_ConcurrencyStress100 .` produced a fatal runtime error:
     ```
     fatal error: checkptr: converted pointer straddles multiple allocations
     goroutine 24 gp=0xc0000ff680 m=10 mp=0xc000238808 [running]:
     runtime.checkptrAlignment(0xc011f7c600?, 0xc011f6dd08?, 0x1?)
         C:/Program Files/Go/src/runtime/checkptr.go:26 +0x5b
     github.com/lemon4ksan/przm.Decompress({0xc000384400, 0x7f, 0x7f})
         D:/CodingProjects/przm/decompress.go:782 +0x2dd8
     ```
   - Inspection of `d:\CodingProjects\przm\decompress.go` lines 780–784:
     ```go
     775: case op >= 0xF3 && op <= 0xFA:
     776:     // Direct 1-byte Literal Run (1..8 literals)
     777:     runLen := int(op - 0xF2)
     778:     if litPtr+runLen > len(lits) {
     779:         return nil, fmt.Errorf("%w: literal stream truncated", ErrCorruptedFrame)
     780:     }
     781:     pDst := unsafe.Add(baseDst, dstPos)
     782:     pSrc := unsafe.Pointer(&lits[litPtr])
     783:     *(*uint64)(pDst) = *(*uint64)(pSrc)
     784:     litPtr += runLen
     ```
     `pSrc` is unconditionally loaded as an 8-byte `*(*uint64)` even when `runLen < 8` and `len(lits) < litPtr + 8`. Unlike lines 346, 421, and 466, line 782 lacks a `litPtr + 8 <= len(lits)` boundary check.

2. **Pure C Decompressor Interoperability Breakdown on Level 4 Streams**:
   - In `TestChallenger_M1_CDecompressor_InteropGap_Reproducer`, generating a sinusoidal payload of 1207 bytes and compressing at Level 4 (`przm.Compress(data, 4)`) emitted a frame with magic `0x47435452` ("GCTR"):
     ```
     Payload compressed with Level 4: Magic=0x47435452 (ASCII: RTCG)
     Pure C decompressor failed on Level 4 output (exit status 2):
     Output: Invalid PRZM frame header: error -2
     ```
   - Inspection of `d:\CodingProjects\przm\src\przm_dec.c` line 994 reveals that `przm_get_uncompressed_size` recognizes `PRZM_MAGIC_SYMPLECTIC`, `ARM64`, `SHEAF`, `MAHLER`, `ISOQUANT`, `PWS`, `VM`, `GGTC`, and `STANDARD`, but does NOT support `PRZM_MAGIC_GCTR` (`0x47435452`).

3. **Decompression Allocation Invariant Violation in Go**:
   - In `TestChallenger_M1_DecompressAllocations_Audit`, measuring `testing.AllocsPerRun(50, ...)` on `przm.Decompress` for streams compressed at Level 3 and Level 4 yielded:
     - `JSON_64K`: 3.00 allocs/op
     - `SourceCode_64K`: 3.00 allocs/op
     - `Tabular_64K`: 3.00 allocs/op
     - `MachineCode_64K`: 1.00 allocs/op
     - `RLE_64K`: 1.00 allocs/op
   - Root cause in `d:\CodingProjects\przm\vans.go`:
     - Line 433: `model, matBytesRead, err := DeserializeMatrix(src[8:])` dynamically allocates `*VANSModel` on each decompression.
     - Line 451: `dst := make([]byte, origLen)` dynamically allocates a temporary slice.
     - Line 271 of `decompress.go`: `dst := bufkit.AlignedBytes(allocSize+64, bufkit.CacheLineSize)` allocates the return slice.
   - Pure C (`src/przm_dec.c`) has 0 malloc calls because it decodes `VANSModel` directly on the stack (line 720) and targets static thread-local storage `tls_lits_scratch` (line 1106).

4. **Hensel MatchFinder Microarchitectural Speedup Verified**:
   - In `TestChallenger_M1_HenselMatchFinder_ZeroAllocAndSpeedup`:
     - Isolated MatchFinder search incurs strictly `0.00 allocs/op` (`0 B/op`).
     - Depth 64 isolated search: 38.17 ns/op (vs 69.26 ns/op for naive chaining) = **1.81x speedup**.
     - Depth 64 dense collision search: 706.09 ns/op (vs 5,559.44 ns/op for naive chaining) = **7.87x speedup**.
     - Theorem HL-2 pruning criterion produces 0 false negatives.

5. **Pure C Decompressor 50 Payload Stress on Hensel Streams Verified**:
   - In `TestChallenger_M1_CDecompressor_HenselStandardStreams_50Payloads`: 50 distinct randomized payloads compressed with standard PRZM frames using Hensel at Level 3 were serialized and passed to `test_przm_dec.exe`. All 50 decoded 100% losslessly with exit code 0.

6. **Scratchpad Isolation Verified**:
   - In `TestChallenger_M1_ConcurrencyStress100_ScratchpadIsolation`: 100 concurrent goroutines executing 1,000 compress/decompress operations completed with zero data corruption, verifying thread safety of `padPool` and `HenselScratchpad`.

---

## 2. Logic Chain

1. **From Observation 1**: Opcode `0xF3..0xFA` loads 8 bytes from `&lits[litPtr]` without checking if `litPtr + 8 <= len(lits)`. When a block ends with a short literal run (1..7 bytes), `pSrc` reads unallocated memory beyond the slice boundary. This directly violates memory safety, causes fatal checkptr panics under race detection, and risks access violation segfaults (`0xC0000005`) in production.
2. **From Observation 2**: Go `przm.Compress(src, 4)` selects `CompressGCTR` whenever GCTR achieves the highest compression ratio. Because `src/przm_dec.c` does not recognize or decode GCTR frames, cross-language interoperability between Go Level 4 compression and Pure C decompression fails on these inputs with `error -2`.
3. **From Observation 3**: The user specification requires strictly 0 heap allocations during decompression (`0 B/op, 0 allocs/op`). Go `Decompress` incurs 3 heap allocations on text/JSON streams because `VANSDecode4x` does not pool its model or destination buffers.
4. **From Observations 4, 5, and 6**: The mathematical foundation and core algorithmic implementation of Hensel match finding (`hensel.go`) is sound, highly optimized (1.81x–7.87x faster than naive chaining), and thread-safe.
5. **From Logic Steps 1–3**: Because memory safety and cross-language Level 4 decompression are broken, Milestone 1 cannot be approved in its current state. A gate verdict of `REQUEST_CHANGES` is mandatory.

---

## 3. Caveats

- **Scope Boundary**: The checkptr bug in `decompress.go` line 782 was pre-existing in PRZM-SPEC-0.5 framing logic rather than introduced solely by `hensel.go`. However, because Hensel match discovery directly drives literal run emission and triggers this code path under concurrency stress, it constitutes an active blocker for Milestone 1 sign-off.
- **Pure C GCTR Support**: GCTR was introduced in PRZM v0.3. If GCTR is considered deprecated or out of scope for Pure C, the Go compressor must be configured not to emit GCTR frames when C compatibility is required.
- **No caveats** regarding the mathematical proofs or correctness of Hensel's lifting in `hensel.go`.

---

## 4. Conclusion

**Gate Verdict: REQUEST_CHANGES**

Milestone 1's core mathematical model (`hensel.go`) is sound, achieves confirmed 1.81x–7.87x microarchitectural speedups, and passes scratchpad isolation stress. However, approval is withheld pending remediation of:
1. The out-of-bounds 8-byte pointer read at `decompress.go:782` (checkptr crash).
2. The missing GCTR frame handling in `przm_dec.c` causing Level 4 C decompressor failures.
3. Heap allocations in Go decompression when V-ANS is engaged (3.00 allocs/op on JSON/Code).

---

## 5. Verification Method

To independently verify these findings, execute:
```powershell
cd d:\CodingProjects\przm

# 1. Verify Hensel MatchFinder isolation and microarchitectural speedup (PASS)
go test -v -run TestChallenger_M1_HenselMatchFinder_ZeroAllocAndSpeedup .

# 2. Reproduce the Checkptr Memory Safety Bug (FAIL under -race)
go test -race -v -run TestChallenger_M1_ConcurrencyStress100 .

# 3. Reproduce the Pure C Decompressor Level 4 Interoperability Gap (FAIL)
go test -v -run TestChallenger_M1_CDecompressor_InteropGap_Reproducer .

# 4. Audit Decompression Heap Allocations (Demonstrates 3.00 allocs/op on JSON)
go test -v -run TestChallenger_M1_DecompressAllocations_Audit .

# 5. Verify 50 Randomized Standard Payload Roundtrips in Pure C (PASS)
go test -v -run TestChallenger_M1_CDecompressor_HenselStandardStreams_50Payloads .
```

Invalidation conditions:
- If `go test -race` runs without checkptr panic after line 782 is guarded.
- If `test_przm_dec.exe` successfully decodes Level 4 GCTR frames or GCTR is gated.
- If Go decompression allocations on JSON/Code streams drop to `0 B/op, 0 allocs/op`.
