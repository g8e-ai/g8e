# Eval Analysis — v2.1.8

- Run ID: `run-golden-1`
- Analysis schema version: `1.2.0`
- Analysis computation version: `1.3.0`

## Input Summary

- Tasks: 2
- Attempts: 4
- Observations: 8
- Receipts: 2
- Stages: 6
- Metric observations: 4
- Input content hash: `abc123def456`

## Missingness Breakdown

| Status | Count |
| --- | --- |
| Completed | 3 |
| Model failed | 1 |
| Governance rejected | 0 |
| Human denied | 0 |
| Timed out | 0 |
| Infrastructure failed | 0 |
| Invalid evidence | 0 |
| **Total** | **4** |

## Receipt Coverage

- Eligible attempts: 2
- Receipt-bound attempts: 2
- Receipt-verified attempts: 1
- Coverage: 100%
- Verification: 50%

## Arms

- `direct`
- `doctrine`

## Metric Results

| Metric | Version | Arm | Domain | Direction | Value | Numerator | Denominator | Eligible | Missing |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| receipt_integrity | 1.0.0 | doctrine | governance | binary_pass_fail | 1 | 2 | 2 | 2 | 0 |

## Domain-Stratified Results

| Arm | Domain | Metrics | Passing | Failing | Not Applicable |
| --- | --- | --- | --- | --- | --- |
| doctrine | governance | 1 | 1 | 0 | 0 |

## Confusion Matrices (Arm-Level)

| Metric | Version | Arm | TP | FP | TN | FN | Accuracy | Balanced Accuracy | MCC |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| policy_outcome | 1.0.0 | doctrine | 3 | 1 | 2 | 0 | 0.8333333333 | 0.8333333333 | 0.7071067812 |

## Pooled Confusion Matrices

| Metric | Version | Arms | TP | FP | TN | FN | Accuracy | Balanced Accuracy | MCC | Pooling Method |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| policy_outcome | 1.0.0 | 1 (doctrine) | 3 | 1 | 2 | 0 | 0.8333333333 | 0.8333333333 | 0.7071067812 | preregistered_simple_summation_across_arms |

## Paired Comparisons

| Metric | Version | Baseline | Comparison | Paired | Baseline Val | Comparison Val | Abs Delta | Rel Delta | Effect Size | Direction | McNemar p | Paired t p | Wilcoxon p | Bootstrap CI | Holm p | NI Margin | Gate |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| receipt_integrity | 1.0.0 | direct | doctrine | 7 | 1 | 1 | 0 | 0 | N/A | neutral | N/A | N/A | N/A | [0, 0] | 1 | 0 | pass |

## Gate Decisions

| Metric | Version | Arm | Status | Measured | Threshold | NI Margin | Reason |
| --- | --- | --- | --- | --- | --- | --- | --- |
| receipt_integrity | 1.0.0 | doctrine | pass | 1 | 1 | 0 | Release-blocker threshold 1.0 met: measured 1.0. |

## Bridge Runs

| Bridge ID | Version | Old Label | New Label | Old Suite Hash | New Suite Hash | Model Cohort | Tasks |
| --- | --- | --- | --- | --- | --- | --- | --- |
| bridge-1 | 1.0.0 | v2.1.7 | v2.1.8 | `oldhash123` | `newhash456` | cohort-1 | 10 |

## Bridge Run Comparisons

| Bridge ID | Metric | Version | Old Value | New Value | Abs Delta | Gate | Reason |
| --- | --- | --- | --- | --- | --- | --- | --- |
| bridge-1 | receipt_integrity | 1.0.0 | 1 | 1 | 0 | pass | No change between versions. |

## Unsupported Claims

- complete_ifeval_import
- human_semantic_grading
- reasoner_independence
