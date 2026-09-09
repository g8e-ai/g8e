# Eval Analysis — v2.1.8

- Run ID: `run-golden-1`
- Analysis schema version: `1.1.0`
- Analysis computation version: `1.1.0`

## Input Summary

- Tasks: 1
- Attempts: 1
- Observations: 0
- Receipts: 0
- Stages: 0
- Metric observations: 1
- Input content hash: `abf5771405f8a5cbedaccb08e96a0782bd0129a8a454d5582d2afb046a91f601`

## Missingness Breakdown

| Status | Count |
| --- | --- |
| Completed | 1 |
| Model failed | 0 |
| Governance rejected | 0 |
| Human denied | 0 |
| Timed out | 0 |
| Infrastructure failed | 0 |
| Invalid evidence | 0 |
| **Total** | **1** |

## Receipt Coverage

- Eligible attempts: 1
- Receipt-bound attempts: 0
- Receipt-verified attempts: 0
- Coverage: 0%
- Verification: 0%

## Arms

- `doctrine`

## Metric Results

| Metric | Version | Arm | Domain | Direction | Value | Numerator | Denominator | Eligible | Missing |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| receipt_integrity | 1.0.0 | doctrine | governance | binary_pass_fail | 1 | 1 | 1 | 1 | 0 |

## Domain-Stratified Results

| Arm | Domain | Metrics | Passing | Failing | Not Applicable |
| --- | --- | --- | --- | --- | --- |
| doctrine | governance | 1 | 1 | 0 | 0 |

## Gate Decisions

| Metric | Version | Arm | Status | Measured | Threshold | NI Margin | Reason |
| --- | --- | --- | --- | --- | --- | --- | --- |
| receipt_integrity | 1.0.0 | doctrine | pass | 1 | 1 | 0 | Release-blocker threshold 1.0 met: measured 1.0. |

## Unsupported Claims

- certification
- complete_ifeval_import
- heterogeneous_l2_reasoning
- human_semantic_grading
- independent_quorum_error_reduction
- legal_compliance
- reasoner_independence
- recurring_operating_effectiveness
