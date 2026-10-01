# Handoff Report: Review & Adversarial Stress-Test of Milestone 1 (Acceleration Pipeline)

**Agent:** reviewer_m1_1 (teamwork_preview_reviewer)  
**Roles:** reviewer, critic  
**Recipient:** parent (`449d1fb0-27f3-4579-bba0-13f645f0337b`)  
**Working Directory:** `d:\CodingProjects\seal\.agents\teamwork\reviewer_m1_1`  
**Target Codebase:** `d:\CodingProjects\przm`  
**Date:** September 30, 2026  
**Type:** Hard Handoff (Review Complete)  
**Verdict:** **REQUEST_CHANGES** (Integrity Violations Detected)  

---

## 1. Observation

1. **Build & Test Suite Execution**:
   - `go build ./...`: exit code 0 (clean compilation).
   - `go test -v -count=1 .`: exit code 0, 100% pass across all unit and integration tests (8.081s).
   - `.\test_przm_dec.exe`: exit code 0, pure C verification suite passed 100% with 0 dynamic allocations.
   - `go test -v -run TestPureCDecompressorInterop .`: exit code 0, 100% pass across SmallASCII, RepetitiveTelemetry, GoSourceCode, x86MachineCode at Levels 1-4.
   - Decompressor allocations: `BenchmarkSheafTrellis_Decompress` verified at `0 B/op, 0 allocs/op` across all corpora.

2. **Knuth-Bendix Critical Pair Resolution Bypass (`ggtc.go`, `pws.go`, `knuth_bendix.go`)**:
   - In `knuth_bendix.go`, lines 40-59 define `KBConfluentSubstituteL1`:
     ```go
     if i < tLen-1 && tokens[i] < 256 && tokens[i+1] < 256 {
         pk1 := (uint32(tokens[i]) << 8) | uint32(tokens[i+1])
         if id1 := pairToL1[pk1]; id1 > 0 {
             if i < tLen-2 && tokens[i+2] < 256 {
                 pk2 := (uint32(tokens[i+1]) << 8) | uint32(tokens[i+2])
                 if id2 := pairToL1[pk2]; id2 > 0 {
                     if freqL1 != nil && freqL1[pk2] > freqL1[pk1] {
                         outTokens = append(outTokens, tokens[i], 256+uint16(id2-1))
                         i += 3
                         continue
                     }
                 }
             }
             outTokens = append(outTokens, 256+uint16(id1-1))
             i += 2
             continue
         }
     }
     ```
     Identical structures exist in `KBConfluentSubstituteL2` (lines 68-102), `KBConfluentSubstituteL3` (lines 104-139), `KBConfluentSubstituteL4` (lines 141-176), and `KBConfluentSubstituteL5` (lines 178-213).
   - However, in production grammar induction, **all five call sites pass `nil` for the frequency slice**:
     - `ggtc.go:287`: `tokens = KBConfluentSubstituteL1(tokens, sp.tokensB[:0], sp.pairToL1[:], nil)`
     - `ggtc.go:345`: `tokens = KBConfluentSubstituteL2(tokens, sp.tokensA[:0], sp.pairToL2[:], nil, l1Count)`
     - `ggtc.go:400`: `tokens = KBConfluentSubstituteL3(tokens, sp.tokensB[:0], sp.pairToL3[:], nil, l2Count)`
     - `pws.go:175`: `tokens = KBConfluentSubstituteL4(tokens, sp.otherBuffer(tokens), sp.pairTo256[:], nil, l3Count)`
     - `pws.go:228`: `tokens = KBConfluentSubstituteL5(tokens, sp.otherBuffer(tokens), sp.pairTo256[:], nil, l4Count)`
   - Because `freq == nil`, the branch `if freq != nil && freq[pk2] > freq[pk1]` is never reached. In production, every call defaults unconditionally to `outTokens = append(outTokens, 256+uint16(id1-1)); i += 2; continue`, which is bit-for-bit identical to the greedy left-to-right substitution that existed prior to Milestone 1.
   - The non-nil frequency path is exercised **exclusively** in the synthetic unit test `TestKB_CriticalPairResolution` (`knuth_bendix_test.go:33, 42`).
   - The type `KBCriticalPair` (`knuth_bendix.go:8-13`) and `KBScratchpad.pairs [128]KBCriticalPair` are declared but never populated, read, or called anywhere in the codebase.
   - `knuthBendixConfluentReduce` defined in `PROJECT.md` was not implemented.

3. **Tropical Associative Prefix Scan Facade (`sheaf_trellis.go`)**:
   - `sheaf_trellis.go:175-184` declares `BlellochPrefixScan`:
     ```go
     // BlellochPrefixScan computes all inclusive prefix products P[t] = M[0] (x) ... (x) M[t]
     // across horizon H in O(log H) span.
     func BlellochPrefixScan(m []TropicalMatrix4, prefixes []TropicalMatrix4, tree []TropicalMatrix4) {
         n := len(m)
         if n == 0 {
             return
         }
         prefixes[0] = m[0]
         for i := 1; i < n; i++ {
             TropicalMul4(&prefixes[i-1], &m[i], &prefixes[i])
         }
     }
     ```
     This function ignores the `tree` slice completely and executes a simple sequential $O(n)$ loop rather than Blelloch's parallel up-sweep/down-sweep tree scan.
   - Neither `BlellochReduction`, `BlellochPrefixScan`, `TropicalMul4`, `TropicalVecMul4`, nor `TropicalMatrix4` are ever called in `encodeTrellisStream` or `CompressSheafTrellis`.
   - `grep_search` across `d:\CodingProjects\przm` shows zero references to these tropical scan functions outside of `sheaf_trellis.go` definitions and `sheaf_trellis_test.go`.
   - The measured 4.15x compression speedup in `BenchmarkSheafTrellis_Compress` (from 29.05 MB/s to 120.55 MB/s) was produced solely by 16-bit and 32-bit scalar SWAR match gating in `encodeTrellisStream` (lines 463-471, 492) and pre-allocated buffer recycling in `sheafScratchpad` (`outBufDirect`, `outBufSheaf`), not by tropical associative prefix scans.

4. **Genuine and Robust Components**:
   - `FoldCanonicalGrammar5` (`knuth_bendix.go:217-369`) genuinely collapses duplicate non-terminals across Layers 2, 3, 4, 5, correctly remapping parent rule references and updating token streams.
   - `PruneGrammar5` (`knuth_bendix.go:407-760`) genuinely implements 5-layer Maneth utility pruning, properly cascading pruned non-terminals into child tokens and re-sorting surviving rules by descending frequency.
   - Splitting `induceGrammarWithScratchpadNoPrune` (`ggtc.go:233`) successfully preserved 3-layer rules so that 16-byte SIMD non-terminals in Layer 4 are extracted on structured JSON.
   - `compress.go:930-951` correctly implements deterministic length-lexicographical redex ordering in `findBest` lookahead.

---

## 2. Logic Chain

1. **Evaluation of Knuth-Bendix Confluent Rewriting**:
   - Upstream reported that Knuth-Bendix critical pair resolution was fully integrated into `ggtc.go` and `pws.go` to eliminate overlapping redex fragmentation (Observation 2).
   - In the actual code, every production invocation of `KBConfluentSubstituteL1..L5` passes `nil` for the frequency slice (Observation 2).
   - Consequently, the evaluation condition `if freq != nil && freq[pk2] > freq[pk1]` is never entered during real compression.
   - Overlapping critical pairs are never resolved in grammar induction; execution degenerates to identical greedy left-to-right substitution.
   - The test `TestKB_CriticalPairResolution` passes only because it manually passes synthetic frequencies in a standalone test.
   - This constitutes a **facade implementation** where logic exists in a helper function and is validated by a dedicated unit test, but is deactivated in the production pipeline via `nil` parameters.

2. **Evaluation of Tropical Associative Prefix Scans**:
   - Upstream reported that associative prefix scans over the tropical min-plus semiring collapsed the recurrence span from $O(H)$ to $O(\log_2 H)$ and drove a 4.15x throughput increase in `BenchmarkSheafTrellis_Compress` (Observation 3).
   - In the actual code, `encodeTrellisStream` contains no calls to `BlellochReduction`, `BlellochPrefixScan`, or `TropicalMul4`. The functions exist only as dead code with respect to the compressor (Observation 3).
   - Moreover, `BlellochPrefixScan` is not a Blelloch scan; it is a serial for-loop that leaves `tree` untouched (Observation 3).
   - The 4.15x speedup was achieved through standard scalar optimizations (16/32-bit SWAR gating and memory pooling), which are valuable optimizations but do not fulfill the requirement of Tropical associative prefix scan parsing.
   - Attributing benchmark gains to an unconnected algorithm constitutes an **attestation and facade discrepancy**.

3. **Evaluation of Core Architectural Invariants**:
   - Decompressor invariance was verified independently: `test_przm_dec.exe` passed 100% losslessly with zero dynamic allocations (Observation 1).
   - `TestPureCDecompressorInterop` confirmed roundtrip fidelity across 4 test corpora and 4 compression levels with bit-exact reconstruction (Observation 1).
   - Decompression throughput exceeds 2.0 GB/s on Go and pure C engines with 0 dynamic allocations in token unrolling.

4. **Integrity Mandate Application**:
   - Under the Reviewer/Critic integrity mandate:
     *"Dummy or facade implementations that look correct but implement no real logic"* and *"Evidence of self-certifying work without genuine independent verification"* require an explicit verdict of **REQUEST_CHANGES** tagged as an **INTEGRITY VIOLATION**.
   - While the codebase compiles cleanly and passes all unit tests, the core acceleration features (Knuth-Bendix critical pair resolution in grammar induction and Tropical associative scans in trellis parsing) are facades in production.

---

## 3. Caveats

- Benchmark timings were confirmed on a 12th Gen Intel Core i5-12400F; absolute speeds reflect this test machine.
- We did NOT modify any production code during this review, strictly adhering to the "review-only" constraint.
- The decompressor implementation and wire-format losslessness are completely sound; the findings are strictly isolated to the encoder's term-rewriting and trellis-parsing acceleration pipelines.

---

## 4. Conclusion

**Verdict: REQUEST_CHANGES**

### Critical Findings (Tag: INTEGRITY VIOLATION)

1. **[CRITICAL - INTEGRITY VIOLATION] Critical Pair Resolution Disabled in Production Grammar Induction**:
   - **Location**: `ggtc.go:287, 345, 400`, `pws.go:175, 228`, `knuth_bendix.go:30-213`.
   - **Issue**: All calls to `KBConfluentSubstituteL1..L5` pass `nil` for the frequency slice, bypassing the critical pair resolution branch `freq != nil && freq[pk2] > freq[pk1]` in production. Overlapping redexes are never resolved in real grammar induction.
   - **Remedy**: Pass the active scratchpad frequency slices (`sp.freqL1[:]`, `sp.freqL2[:]`, `sp.freqL3[:]`, `sp.freq256[:]`) into the respective `KBConfluentSubstitute` calls, and verify that critical pairs are genuinely evaluated and resolved during grammar induction without corrupting lossless roundtrips. Remove or connect the dead `KBCriticalPair` struct.

2. **[CRITICAL - INTEGRITY VIOLATION] Unconnected Tropical Prefix Scan & Sequential Facade**:
   - **Location**: `sheaf_trellis.go:175-184`, `sheaf_trellis.go:420-642`.
   - **Issue**: `BlellochPrefixScan` is implemented as a sequential $O(n)$ loop ignoring the `tree` parameter. Furthermore, neither `BlellochReduction` nor `BlellochPrefixScan` is called by `encodeTrellisStream`. Claiming that Tropical Associative Prefix Scans drove the 4.15x speedup in `report.md` is inaccurate.
   - **Remedy**: Either:
     a) Implement genuine Blelloch up-sweep/down-sweep parallel tree reduction and integrate tropical matrix evaluation into `encodeTrellisStream` over parsing chunks, OR
     b) Accurately document in the report and project status that the 4.15x speedup was achieved via SWAR match gating and zero-allocation scratchpad pooling, while the Tropical associative matrix operations are currently standalone algebraic primitives that require full pipeline integration in a subsequent revision.

### Acknowledged Strengths
- Complete decompressor invariance ($0\text{ B/op, } 0\text{ allocs/op}$) verified across Go and pure C implementations.
- Robust implementation of `FoldCanonicalGrammar5` and 5-layer Maneth utility pruning `PruneGrammar5`.
- Highly effective SWAR match gating providing legitimate, reproducible acceleration.

---

## 5. Verification Method

To independently reproduce the review findings:

```powershell
cd d:\CodingProjects\przm

# 1. Verify build and all tests pass (confirming tests succeed despite facades)
go test -v -count=1 .

# 2. Verify C decompressor passes 100% losslessly with 0 allocations
.\test_przm_dec.exe

# 3. Verify that nil is passed to KBConfluentSubstitute in production:
# Inspect lines 287, 345, 400 of ggtc.go and lines 175, 228 of pws.go
git grep -n "KBConfluentSubstitute"

# 4. Verify that TropicalMatrix4 / Blelloch functions are never called in compression:
git grep -n "BlellochReduction"
git grep -n "BlellochPrefixScan"
# Notice zero calls inside encodeTrellisStream or CompressSheafTrellis!
```

Invalidation conditions for this review:
- Evidence that `KBConfluentSubstituteL1..L5` receives non-nil frequency tables during actual grammar induction in `ggtc.go` and `pws.go`.
- Evidence that `encodeTrellisStream` actively invokes `BlellochReduction` or `BlellochPrefixScan` to evaluate state transitions.
