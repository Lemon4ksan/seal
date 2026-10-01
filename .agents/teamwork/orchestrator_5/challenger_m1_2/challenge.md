# Milestone 1 Adversarial Challenge Report

**Author:** challenger_m1_2 (Milestone 1 Concurrency & Microarchitectural Challenger 2)  
**Parent Orchestrator:** orchestrator_5 (`1eb7944e-37f1-4b94-a209-66caae11aab3`)  
**Target Codebase:** PRZM Compression Engine (`d:\CodingProjects\przm`)  
**Date:** October 1, 2026  
**Gate Verdict:** **REQUEST_CHANGES**

---

## Challenge Summary

**Overall Risk Assessment:** **CRITICAL**

While the Hensel match finder implementation (`hensel.go`) demonstrates exceptional microarchitectural efficiency (1.81x–11.0x speedup over naive linear chaining with 0 heap allocations in isolation), rigorous empirical challenge across 100 concurrent goroutines, randomized payloads, race instrumentation, and allocation audits uncovered **two critical bugs** and **one invariant violation** that block production approval:

1. **CRITICAL Memory Safety / UB Bug**: In `decompress.go` lines 780–784, direct literal runs (opcodes `0xF3..0xFA`) execute unaligned 8-byte loads (`*(*uint64)(pSrc)`) without validating that `litPtr + 8 <= len(lits)`. Under Go `-race` / `-d=checkptr`, this panics with `fatal error: checkptr: converted pointer straddles multiple allocations`. In production, this causes out-of-bounds heap/stack reads and potential segmentation faults (`0xC0000005`).
2. **HIGH Interoperability Defect**: Go `Compress(src, 4)` selects Galois Superoptimized JIT & Trace Annihilation (`CompressGCTR`, magic `0x47435452` "GCTR") for multi-frequency/tensor payloads. However, the Pure C reference decompressor `test_przm_dec.exe` / `przm_dec.c` does NOT implement GCTR decompression. When fed Level 4 streams on these payloads, `test_przm_dec.exe` rejects the frame with `error -2` (`PRZM_ERR_INVALID_MAGIC`), breaking lossless roundtrip guarantees.
3. **MEDIUM Decompression Allocation Invariant Violation**: Worker M1 claimed strictly 0 heap allocations during decompression. While Pure C is zero-alloc, Go `Decompress` incurs 3.00 allocs/op on JSON, Go code, and Tabular telemetry due to unpooled allocations in `VANSDecode4x` (`*VANSModel` in `DeserializeMatrix` and `make([]byte, origLen)`).

---

## Challenges

### [Critical] Challenge 1: Unchecked 8-Byte Pointer Read in `decompress.go` (checkptr Crash)

- **Assumption Challenged**: That decompressor literal decoding is memory-safe and passes Go race/pointer integrity checks (`go test -race`).
- **Attack Scenario**: Emitting a compressed frame whose literal section terminates with a small direct literal run (1..7 bytes, opcodes `0xF3..0xFA`). Opcode `0xF3` decodes 1 literal (`runLen = 1`).
  At line 780–782 of `decompress.go`:
  ```go
  pDst := unsafe.Add(baseDst, dstPos)
  pSrc := unsafe.Pointer(&lits[litPtr])
  *(*uint64)(pDst) = *(*uint64)(pSrc) // Blind 8-byte load!
  ```
  If `lits` terminates at `litPtr + 1`, this reads 7 bytes past the allocation boundary of `lits`.
- **Empirical Evidence**: Running `go test -race` on concurrency stress triggered:
  ```
  fatal error: checkptr: converted pointer straddles multiple allocations
  goroutine 24 gp=0xc0000ff680 m=10 mp=0xc000238808 [running]:
  runtime.checkptrAlignment(...)
      C:/Program Files/Go/src/runtime/checkptr.go:26
  github.com/lemon4ksan/przm.Decompress(...)
      D:/CodingProjects/przm/decompress.go:782 +0x2dd8
  ```
- **Blast Radius**: Arbitrary out-of-bounds memory read during decompression. If `lits` ends near an unmapped page boundary, immediate process termination via access violation (`0xC0000005`).
- **Mitigation**: Add the same length guard present on lines 346, 421, and 466:
  ```go
  if litPtr+8 <= len(lits) {
      *(*uint64)(pDst) = *(*uint64)(pSrc)
  } else {
      copy(dst[dstPos:dstPos+runLen], lits[litPtr:litPtr+runLen])
  }
  ```

---

### [High] Challenge 2: Pure C Decompressor Lacks GCTR Decoder on Level 4 Streams

- **Assumption Challenged**: That `przm_dec.c` can decompress all bitstreams emitted by Go `przm.Compress` across Levels 1..4.
- **Attack Scenario**: Compressing a multi-frequency sinusoidal payload (`size = 1207 B`) at Level 4 (`przm.Compress(data, 4)`). The Pareto selection chooses `gctrCandidate` (`magic = 0x47435452`, "GCTR") because it achieves superior compression over standard LZ. When serialized to disk and passed to `test_przm_dec.exe in.przm out.bin`, `przm_get_uncompressed_size` encounters an unknown magic.
- **Empirical Evidence**: Reproduced in `TestChallenger_M1_CDecompressor_InteropGap_Reproducer`:
  ```
  Payload compressed with Level 4: Magic=0x47435452 (ASCII: RTCG)
  Pure C decompressor failed on Level 4 output (exit status 2):
  Output: Invalid PRZM frame header: error -2
  ```
- **Blast Radius**: Complete roundtrip decompression failure on Level 4 payloads when interoperating between Go encoder and Pure C decompressor.
- **Mitigation**: Implement `przm_gctr_decompress` in `src/przm_dec.c` and register `PRZM_MAGIC_GCTR = 0x47435452` in `przm_get_uncompressed_size` and `przm_decompress`, OR disable GCTR selection when language-neutral pure C compatibility is targeted.

---

### [Medium] Challenge 3: Go Decompression Heap Allocation Invariant Violation (3 allocs/op)

- **Assumption Challenged**: That decompression strictly adheres to `0 heap allocations (0 B/op, 0 allocs/op)` across all standard corpora.
- **Attack Scenario**: Decompressing Level 3 or Level 4 streams on structured corpora (`JSON_64K`, `SourceCode_64K`, `Tabular_64K`). Because these corpora have high literal entropy patterns, the encoder engages V-ANS literal encoding (`VANSEncode4x`).
  During decompression, `VANSDecode4x` is invoked:
  - `DeserializeMatrix` allocates `*VANSModel` on the heap (unpooled).
  - Line 451: `dst := make([]byte, origLen)` allocates a new slice on the heap.
  - Line 271: `Decompress` allocates the destination slice `dst := bufkit.AlignedBytes(...)`.
- **Empirical Evidence**: In `TestChallenger_M1_DecompressAllocations_Audit`:
  - `JSON_64K Lvl 3`: **3.00 allocs/op**
  - `SourceCode_64K Lvl 3`: **3.00 allocs/op**
  - `Tabular_64K Lvl 3`: **3.00 allocs/op**
  - `MachineCode_64K Lvl 3`: **1.00 allocs/op** (no V-ANS, only return slice)
  - `RLE_64K Lvl 3`: **1.00 allocs/op** (no V-ANS, only return slice)
- **Blast Radius**: Violates the zero-allocation invariant in Go runtime environments, causing garbage collector pressure during high-throughput stream ingestion.
- **Mitigation**:
  1. Add a `sync.Pool` for `*VANSModel` in `vans.go`.
  2. Provide a `DecompressTo(src, dst []byte)` entry point that accepts a preallocated destination buffer and passes pooled scratchpads down to `VANSDecode4x`.

---

## Stress Test Results

| Test Case | Scenario / Harness | Expected Behavior | Actual Behavior | Result |
|---|---|---|---|---|
| **Hensel MatchFinder Isolation** | 1,000 insertions, 100 queries (`testing.AllocsPerRun`) | 0 allocs/op, 0 B/op | **0.00 allocs/op, 0 B/op** | **PASS** |
| **Microarchitectural Speedup (Isolated)** | Hensel Depth 64 vs Naive Chaining Depth 64 | Speedup >= 1.50x | **38.17 ns vs 69.26 ns (1.81x speedup)** | **PASS** |
| **Microarchitectural Speedup (Heavy Collision)** | 80 periodic motifs in 64 KiB, 100K queries | Speedup >= 1.50x | **706.09 ns vs 5559.44 ns (7.87x–11.06x speedup)** | **PASS** |
| **Concurrency Stress (100 goroutines)** | 100 goroutines, 1,000 operations, random/structured/boundary | Zero data corruption, 100% lossless | **1,000 ops completed losslessly with scratchpad isolation** | **PASS** |
| **Race Detector / Pointer Safety** | `go test -race` on concurrency stress | Zero races, zero panics | **`fatal error: checkptr: converted pointer straddles multiple allocations` at `decompress.go:782`** | **FAIL** |
| **Pure C Decompressor 50 Payload Stress** | 50 distinct payloads on Level 3 standard frames | 100% lossless, 0 errors | **50/50 decoded 100% losslessly by `test_przm_dec.exe`** | **PASS** |
| **Pure C Decompressor Level 4 GCTR Interop** | Payload 34 (sinusoidal pattern) compressed at Lvl 4 | Lossless decode in Pure C | **Failed: `error -2 (PRZM_ERR_INVALID_MAGIC)` — GCTR missing in C** | **FAIL** |
| **Decompression Allocations (Corpora Audit)** | `testing.AllocsPerRun` on Level 3/4 decompression | <= 1 alloc/op (or 0 with preallocated buffer) | **3.00 allocs/op on JSON/Code/Tabular (V-ANS allocations)** | **FAIL** |

---

## Unchallenged Areas

- **Discrete Heisenberg Group $H_3(\mathbb{F}_{2^8})$**: Milestone 2 scope (not yet implemented in codebase).
- **Cellular Sheaves with Discrete Hodge Laplacians**: Milestone 3 scope (not yet implemented in codebase).
- **Lean 4 formal verification**: Lake build verified clean (`lake build` exit code 0); formal proof lemmas will be evaluated in Milestone 4.

---

## Recommendations & Required Remediations

1. **Immediate Remediation in `decompress.go` line 780**:
   Add length check `if litPtr+8 <= len(lits)` before executing `*(*uint64)(pDst) = *(*uint64)(pSrc)` in opcode `0xF3..0xFA` handler to eliminate out-of-bounds reads and resolve checkptr panics.
2. **Remediation in C Decompressor / Codec Pareto Selection**:
   Either implement GCTR frame handling in `przm_dec.c` or restrict GCTR from being selected on general Level 4 streams when cross-language C parity is required.
3. **Decompression Allocation Invariant Alignment**:
   Introduce `DecompressTo(src, dst []byte)` in Go to achieve true 0-allocation decompression matching the C decompressor contract, and pool the `VANSModel` in `vans.go`.
