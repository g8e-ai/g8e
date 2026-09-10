# Eval Analysis — v2.1.8

- Run ID: `run-golden-1`
- Analysis schema version: `1.2.0`
- Analysis computation version: `1.3.0`

## Input Summary

- Tasks: 1
- Attempts: 1
- Observations: 0
- Receipts: 0
- Stages: 0
- Metric observations: 1
- Input content hash: `663a8620c243bd6d1f24358b9e193ad0635498ecbad8595ad8c552ed902b448a`

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

| Metric | Version | Cohort | Arm | Domain | Direction | Value | Numerator | Denominator | Eligible | Missing |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| human_wait_seconds | 1.0.0 |  | doctrine | telemetry | neutral | N/A | 0 | 0 | 1 | 1 |
| local_resource_cpu_seconds | 1.0.0 |  | doctrine | telemetry | neutral | N/A | 0 | 0 | 1 | 1 |
| local_resource_peak_memory_bytes | 1.0.0 |  | doctrine | telemetry | neutral | N/A | 0 | 0 | 1 | 1 |
| provider_cost_usd | 1.0.0 |  | doctrine | economics | neutral | N/A | 0 | 0 | 1 | 1 |
| provider_usage_tokens | 1.0.0 |  | doctrine | telemetry | neutral | N/A | 0 | 0 | 1 | 1 |
| receipt_integrity | 1.0.0 |  | doctrine | governance | binary_pass_fail | 1 | 1 | 1 | 1 | 0 |
| stage_latency_seconds | 1.0.0 |  | doctrine | telemetry | neutral | N/A | 0 | 0 | 1 | 1 |

## Domain-Stratified Results

| Cohort | Arm | Domain | Metrics | Passing | Failing | Not Applicable |
| --- | --- | --- | --- | --- | --- | --- |
|  | doctrine | economics | 1 | 0 | 0 | 0 |
|  | doctrine | governance | 1 | 1 | 0 | 0 |
|  | doctrine | telemetry | 5 | 0 | 0 | 0 |

## Gate Decisions

| Metric | Version | Cohort | Arm | Status | Measured | Threshold | NI Margin | Reason |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| human_wait_seconds | 1.0.0 |  | doctrine | insufficient_data | N/A | N/A | N/A | 1 of 1 eligible attempts are missing observations. |
| local_resource_cpu_seconds | 1.0.0 |  | doctrine | insufficient_data | N/A | N/A | N/A | 1 of 1 eligible attempts are missing observations. |
| local_resource_peak_memory_bytes | 1.0.0 |  | doctrine | insufficient_data | N/A | N/A | N/A | 1 of 1 eligible attempts are missing observations. |
| provider_cost_usd | 1.0.0 |  | doctrine | insufficient_data | N/A | N/A | N/A | 1 of 1 eligible attempts are missing observations. |
| provider_usage_tokens | 1.0.0 |  | doctrine | insufficient_data | N/A | N/A | N/A | 1 of 1 eligible attempts are missing observations. |
| receipt_integrity | 1.0.0 |  | doctrine | pass | 1 | 1 | 0 | Release-blocker threshold 1.0 met: measured 1.0. |
| stage_latency_seconds | 1.0.0 |  | doctrine | insufficient_data | N/A | N/A | N/A | 1 of 1 eligible attempts are missing observations. |

## Unsupported Claims

- certification
- complete_ifeval_import
- heterogeneous_l2_reasoning
- human_semantic_grading
- independent_quorum_error_reduction
- legal_compliance
- reasoner_independence
- recurring_operating_effectiveness
