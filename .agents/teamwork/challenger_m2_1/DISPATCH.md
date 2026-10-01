# DISPATCH: challenger_m2_1

- **Role**: IsoQuant Adversarial Stress Challenger (teamwork_preview_challenger)
- **Target Codebase**: `d:\CodingProjects\przm`
- **Assigned Milestone**: Milestone 2 — Stress, Invertibility & Boundary Verification
- **Working Directory**: `d:\CodingProjects\seal\.agents\teamwork\challenger_m2_1`
- **Inputs**:
  - `d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md`
  - `d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md`
  - `d:\CodingProjects\seal\.agents\teamwork\worker_m2\handoff.md`

## Mandate
1. Adversarially stress test IsoQuant 4D Collimation across Go and Pure C:
   - Run tests: `go test -v -run TestIsoQuant .`
   - Test non-multiple-of-4 buffer lengths ($N \pmod 4 \ne 0$), zero-length inputs, single-vector inputs.
   - Run pure C decompressor tests: `.\test_przm_dec.exe`.
   - Run cross-language Go-C interop: `go test -v -run TestPureCDecompressorInterop .`.
   - Run full Go test suite: `go test -v .`.
2. Confirm 100% bit-exact losslessness and zero panics or buffer overruns.
3. State explicit verdict: **APPROVE** or **REJECT** in `handoff.md` and notify orchestrator via `send_message`.

## 2026-09-30T20:06:29Z
You are challenger_m2_1 (teamwork_preview_challenger).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\challenger_m2_1
Read your dispatch at: d:\CodingProjects\seal\.agents\teamwork\challenger_m2_1\DISPATCH.md
Read ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
Read PROJECT.md at: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md
Read worker handoff at: d:\CodingProjects\seal\.agents\teamwork\worker_m2\handoff.md
Target codebase: d:\CodingProjects\przm

Adversarially challenge Milestone 2 implementation across Go and Pure C:
1. Run full test suite: `cd d:\CodingProjects\przm; go test -v .`
2. Run edge tests: unaligned lengths (N % 4 != 0), empty buffers, 1-byte, 2-byte, 3-byte inputs.
3. Run pure C suite: `cd d:\CodingProjects\przm; .\test_przm_dec.exe`
4. Run cross-language interop: `cd d:\CodingProjects\przm; go test -v -run TestPureCDecompressorInterop .`
5. Verify 100% losslessness and zero panics.
State your explicit verdict: APPROVE or REJECT in handoff.md and notify me via send_message.
