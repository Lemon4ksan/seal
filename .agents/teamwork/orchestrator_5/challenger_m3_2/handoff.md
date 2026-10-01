# Handoff Report: Milestone 3 Concurrency, Zero-Alloc & C Stress Challenger 2

**Agent:** `challenger_m3_2`  
**Working Directory:** `d:\CodingProjects\seal\.agents\teamwork\orchestrator_5\challenger_m3_2`  
**Target Codebase:** `d:\CodingProjects\przm`  
**Date:** October 1, 2026  
**Type:** Hard Handoff (Adversarial Challenge Complete)  
**Verdict:** **`REQUEST_CHANGES`**

---

## 1. Observation

1. **Concurrency Stress Testing (100 Goroutines under `-race`):**
   - Command: `go test -v -race -run TestMilestone3_ConcurrencyStress_100Goroutines .`
   - Output:
     ```text
     === RUN   TestMilestone3_ConcurrencyStress_100Goroutines
     --- PASS: TestMilestone3_ConcurrencyStress_100Goroutines (0.59s)
     PASS
     ok  	github.com/lemon4ksan/przm	2.115s
     ```
   - Observed: 100 concurrent worker goroutines each performing 50 compression and decompression roundtrips across 5 diverse payload profiles (5,000 roundtrips total). 0 data races, 0 memory corruption, 0 deadlocks.

2. **Pure C Reference Decompressor Cross-Interop (50 Random Frames):**
   - Command: `go test -v -run TestMilestone3_PureC_50RandomFrames .`
   - Output:
     ```text
     === RUN   TestMilestone3_PureC_50RandomFrames
         cellular_sheaf_concurrency_c_test.go:355: PASS: Pure C reference decompressor decoded all 50 random frames losslessly with 0 errors.
     --- PASS: TestMilestone3_PureC_50RandomFrames (1.39s)
     PASS
     ```
   - Observed: 50 randomly generated Cellular Sheaf frames covering varied candidate strides ($W \in \{4, 8, 16, 32, 64\}$), varied heights ($M \in [2, 37]$), trailing unaligned remainders, curl-free fields, sparse curl fields, and dense noise were compiled, written to disk, and decoded by `test_przm_dec.exe`. All 50 frames with $\alpha = 1$ matched bit-for-bit.

3. **Pure C Decompressor Non-Trivial Twist Divergence Bug:**
   - In `d:\CodingProjects\przm\src\przm_dec.c` lines 1278–1282:
     ```c
     } else {
         for (uint16_t j = 0; j < W; j++) {
             dst[row_curr + j] = gf256_mul(twist, dst[row_prev + j]) ^ v;
         }
     }
     ```
   - In `d:\CodingProjects\przm\cellular_sheaf.go` lines 498–504:
     ```go
     } else {
         curV := v
         for j := 0; j < W; j++ {
             dst[rowCurr+j] = GF256Mul(twist, dst[rowPrev+j]) ^ curV
             curV = GF256Mul(twist, curV)
         }
     }
     ```
   - Test Command: `go test -v -run TestMilestone3_Twist2_NonZeroEv_EndToEndInterop .`
   - Output:
     ```text
     === RUN   TestMilestone3_Twist2_NonZeroEv_EndToEndInterop
         cellular_sheaf_concurrency_c_test.go:552: Twist=2, Flags=0x000D, CompLen=26 (orig 128)
         cellular_sheaf_concurrency_c_test.go:563: Go decompress: SUCCESS (Lossless)
         cellular_sheaf_concurrency_c_test.go:584: PURE C FAILED TO DECOMPRESS TWIST=2 FIELD LOSSLESSLY!
             Orig: 03 06 0C 18 30 60 C0 9B 03 06 0C 18 30 60 C0 9B
             Go:   03 06 0C 18 30 60 C0 9B 03 06 0C 18 30 60 C0 9B
             C:    03 06 0C 18 30 60 C0 9B 03 09 1D 35 65 C5 9E 28
     --- FAIL: TestMilestone3_Twist2_NonZeroEv_EndToEndInterop (0.02s)
     ```
   - Observed: Go `DecompressCellularSheaf` losslessly decompressed the frame, but Pure C reference decompressor `test_przm_dec.exe` produced data corruption on row 1 from column 1 onwards (`03 09 1D 35...` vs expected `03 06 0C 18...`).

4. **Zero-Allocation Assessment:**
   - In `cellular_sheaf_concurrency_c_test.go`:
     - `TestMilestone3_ZeroAllocDecompression_Assertion`:
       ```text
       === RUN   TestMilestone3_ZeroAllocDecompression_Assertion
       --- PASS: TestMilestone3_ZeroAllocDecompression_Assertion (0.00s)
           --- PASS: CurlFree_8x8 (0.00s)
           --- PASS: ConstBoundaries_16x16 (0.00s)
           --- PASS: Harmonic_64x64_4KB (0.00s)
           --- PASS: RawPassthrough_8B (0.00s)
       ```
       All uncascaded modes strictly satisfy `0 B/op, 0 allocs/op`.
     - In `cellular_sheaf_adversarial_test.go`:
       ```text
       === RUN   TestCellularSheaf_ZeroAllocDecompression_AllModes/SparseCurl_32x32
           cellular_sheaf_adversarial_test.go:244: SparseCurl_32x32: violated zero-allocation invariant: 1.000000 allocs/op (expected 0)
       ```
     - In `cellular_sheaf.go` lines 813–819:
       ```go
       if (flags & CSHDFlagCascadedPWS) != 0 {
           innerDec, err := PWSDecode(payload)
           if err != nil {
               return nil, err
           }
           payload = innerDec
       }
       ```
       When `CSHDFlagCascadedPWS` is active, `PWSDecode` allocates heap memory for rule slices and decompressed bytes, resulting in 1–2 heap allocations per operation.
   - In Pure C: Static search in `src/przm_dec.c` confirmed 0 calls to `malloc`, `calloc`, `realloc`, or `free`.

5. **Microarchitectural Throughput & CPU Cycles:**
   - Go benchmark (`BenchmarkMilestone3_PureRowSweepThroughput`):
     ```text
     BenchmarkMilestone3_PureRowSweepThroughput-12    125370    10031 ns/op    6533.06 MB/s    6.085 GB/s    29.39 cycles/64B    0 B/op    0 allocs/op
     ```
   - Pure C benchmark (`gcc -O3`, 64 KiB block, 500,000 iterations):
     ```text
     Throughput: 5.49 GB/s (5626.58 MB/s)
     CPU Cycles per 64B block: 27.07 cycles/64B
     CPU Cycles per byte: 0.423 cycles/B
     ```
   - Observed: Sub-cycle processing latency (0.423 cycles/byte) in Pure C and 6.08 GB/s in Go, strictly bounded by single-core L1D/L2 cache throughput.

---

## 2. Logic Chain

1. **Concurrency and Thread-Safety:**
   From Observation 1, `CompressCellularSheaf` and `DecompressCellularSheaf` use no shared mutable state, globals, or package-level caches. 100 concurrent goroutines executing 5,000 iterations under Go's ThreadSanitizer race detector completed cleanly without a single race or synchronization hazard.

2. **C Reference Decompressor Mathematical Inversion Defect:**
   - From Observation 3, in characteristic 2 discrete exterior calculus over $\mathbb{F}_{2^8}$, when $d_1 u \equiv 0$ on elementary faces, the vertical 1-form $u^v_{i, j}$ advances as $u^v_{i, j} = \alpha \cdot u^v_{i, j-1}$.
   - Go `ReconstructCellularSheaf` and `DecompressCellularSheaf` evolve `curV` by `curV = GF256Mul(twist, curV)`.
   - Pure C `przm_cellular_sheaf_decompress` in `src/przm_dec.c` fails to update `v`, leaving it constant across all columns $j$.
   - When $\alpha > 1$ and $Ev \ne 0$, C computes $dst(i, j) = \alpha \cdot dst(i-1, j) \oplus Ev[i-1]$ instead of $\alpha \cdot dst(i-1, j) \oplus (\alpha^j \cdot Ev[i-1])$, corrupting all columns $j \ge 1$.
   - This directly violates the bit-exact interoperability requirement between Go and Pure C.

3. **Decompression Zero-Allocation Contract:**
   - From Observation 4, uncascaded Cellular Sheaf decompression performs purely stack/register-based row sweeps with pre-allocated `dst`, achieving strictly `0 B/op, 0 allocs/op`.
   - However, when the encoder discovers that cascading PWS on the boundary payload reduces size, `flags & CSHDFlagCascadedPWS` is set.
   - Decompressing this frame in Go delegates to `PWSDecode(payload)`, which invokes `make([]RuleL1, numL1)` and allocates the output slice on the heap. This causes 1–2 heap allocations per decompression run, violating the $O(1)$ zero-allocation contract.

4. **Microarchitectural Throughput:**
   - From Observation 5, Pure C achieves 0.423 cycles/byte (27 cycles per 64-byte block, 5.49 GB/s).
   - This represents instantaneous decompression processing faster than 2 bytes per clock cycle on modern x86-64 hardware.

---

## 3. Caveats

- For canonical twist $\alpha = 1$, all 50 randomly generated frames and existing test vectors decode bit-for-bit losslessly across both Go and Pure C.
- The C defect only manifests when `twist > 1` (non-trivial Galois twist) and boundary vertical 1-forms are non-zero ($Ev \ne 0$).

---

## 4. Conclusion

Milestone 3 is rejected at this gate with verdict **`REQUEST_CHANGES`**. Upstream integration cannot proceed until Worker M3 implements the required fixes:
1. **Fix Pure C Decompressor (`src/przm_dec.c`):**
   - Update `cur_v = gf256_mul(twist, cur_v)` across columns in both Curl-Free (line 1280) and Sparse-Curl (lines 1302–1314) loops when `twist > 1`.
   - Add verification test cases for `Twist > 1` with non-zero $Ev$ in `tests/test_przm_dec.c`.
2. **Remediate Heap Allocations in Cascaded PWS Decompression (`cellular_sheaf.go`):**
   - Provide a zero-allocation scratch buffer path for `PWSDecode` in Go, or document/gate PWS payload cascade to ensure `0 allocs/op`.

---

## 5. Verification Method

To independently verify the defects and reproduce all findings:

1. **Reproduce the Pure C Twist Divergence Defect:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestMilestone3_Twist2_NonZeroEv_EndToEndInterop .
   ```
   *Expected Result:* FAILS with output mismatch between Go and Pure C on row 1 (`03 09 1D 35...` vs `03 06 0C 18...`).

2. **Verify 100-Goroutine Concurrency under `-race`:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -race -run TestMilestone3_ConcurrencyStress_100Goroutines .
   ```
   *Expected Result:* PASSES in ~0.6s with 0 race detector warnings.

3. **Verify Pure C 50 Random Frames for $\alpha = 1$:**
   ```powershell
   cd d:\CodingProjects\przm
   go test -v -run TestMilestone3_PureC_50RandomFrames .
   ```
   *Expected Result:* PASSES for 50/50 frames.
