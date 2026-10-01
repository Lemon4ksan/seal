# Dispatch: explorer_m2_math

## 2026-09-30T19:45:00Z
You are explorer_m2_math (teamwork_preview_explorer).
Your working directory is: d:\CodingProjects\seal\.agents\teamwork\explorer_m2_math
Target Codebase & Spec: `d:\CodingProjects\przm`, `d:\CodingProjects\przm\MATHEMATICAL_REPORT.md`

Read:
- ORIGINAL_REQUEST.md at: d:\CodingProjects\seal\.agents\teamwork\ORIGINAL_REQUEST.md
- PROJECT.md at: d:\CodingProjects\seal\.agents\teamwork\orchestrator_3\PROJECT.md
- MATHEMATICAL_REPORT.md at: d:\CodingProjects\przm\MATHEMATICAL_REPORT.md (§6.4, §7.4 Proof 4, §7.5)

Task:
Extract and detail the mathematical algorithms and exact arithmetic formulas for 4D IsoQuant Quaternionic Collimation:
1. 4D vector representation: Each 4-byte chunk $(x_0, x_1, x_2, x_3)$ treated as a Hamilton quaternion $x = x_0 + x_1 \mathbf{i} + x_2 \mathbf{j} + x_3 \mathbf{k}$ or vector over $\mathbb{R}^4$ (or modulo integer arithmetic).
2. Covariance and principal axis alignment: How the isoclinic rotation pair $(q_L, q_R)$ is computed or selected to collimate 4D coordinates and minimize entropy.
3. Forward collimation: $\mathbf{x}' = q_L \cdot \mathbf{x} \cdot \bar{q}_R$.
4. Branchless inverse collimation: $\mathbf{x} = \bar{q}_L \cdot \mathbf{x}' \cdot q_R$ and how it executes branchlessly in $\le 22$ CPU cycles per 64 bytes.
5. Invertibility & exact losslessness: Ensure the transformation is bit-exact lossless over integer/byte coordinates (or unimodular integer rotation matrix / modular isoclinic involution).
Write your full report to `report.md` and handoff to `handoff.md`. Notify me via send_message when done.
