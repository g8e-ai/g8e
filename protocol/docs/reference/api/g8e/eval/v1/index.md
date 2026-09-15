# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [g8e/eval/v1/eval.proto](#g8e_eval_v1_eval-proto)
    - [EvaluationAssertion](#g8e-eval-v1-EvaluationAssertion)
    - [EvaluationAttempt](#g8e-eval-v1-EvaluationAttempt)
    - [EvaluationDeploymentIdentity](#g8e-eval-v1-EvaluationDeploymentIdentity)
    - [EvaluationMetric](#g8e-eval-v1-EvaluationMetric)
    - [EvaluationObservation](#g8e-eval-v1-EvaluationObservation)
    - [EvaluationReport](#g8e-eval-v1-EvaluationReport)
    - [EvaluationRun](#g8e-eval-v1-EvaluationRun)
    - [EvaluationRuntimeBoundary](#g8e-eval-v1-EvaluationRuntimeBoundary)
    - [EvaluationTargetState](#g8e-eval-v1-EvaluationTargetState)
    - [EvaluationValue](#g8e-eval-v1-EvaluationValue)
    - [EvaluationVerdict](#g8e-eval-v1-EvaluationVerdict)
  
    - [EvaluationAttemptStatus](#g8e-eval-v1-EvaluationAttemptStatus)
    - [EvaluationComparator](#g8e-eval-v1-EvaluationComparator)
    - [EvaluationEvidenceAuthority](#g8e-eval-v1-EvaluationEvidenceAuthority)
    - [EvaluationGovernancePosture](#g8e-eval-v1-EvaluationGovernancePosture)
    - [EvaluationLane](#g8e-eval-v1-EvaluationLane)
    - [EvaluationMetricDirection](#g8e-eval-v1-EvaluationMetricDirection)
    - [EvaluationMetricUnit](#g8e-eval-v1-EvaluationMetricUnit)
    - [EvaluationMissingDataPolicy](#g8e-eval-v1-EvaluationMissingDataPolicy)
    - [EvaluationObservationSource](#g8e-eval-v1-EvaluationObservationSource)
    - [EvaluationRuntimeComponent](#g8e-eval-v1-EvaluationRuntimeComponent)
    - [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus)
  
- [Scalar Value Types](#scalar-value-types)



<a name="g8e_eval_v1_eval-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## g8e/eval/v1/eval.proto



<a name="g8e-eval-v1-EvaluationAssertion"></a>

### EvaluationAssertion



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| assertion_id | [string](#string) |  |  |
| assertion_version | [string](#string) |  |  |
| comparator | [EvaluationComparator](#g8e-eval-v1-EvaluationComparator) |  |  |
| expected | [EvaluationValue](#g8e-eval-v1-EvaluationValue) |  |  |
| required_observation_types | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) | repeated |  |
| required_authorities | [EvaluationEvidenceAuthority](#g8e-eval-v1-EvaluationEvidenceAuthority) | repeated |  |






<a name="g8e-eval-v1-EvaluationAttempt"></a>

### EvaluationAttempt



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| attempt_id | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| scenario_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| status | [EvaluationAttemptStatus](#g8e-eval-v1-EvaluationAttemptStatus) |  |  |
| started_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| completed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| transaction_id | [string](#string) |  |  |
| execution_id | [string](#string) |  |  |
| observation_refs | [string](#string) | repeated |  |
| assertion_refs | [string](#string) | repeated |  |
| verdict_refs | [string](#string) | repeated |  |
| failure_detail | [string](#string) |  |  |






<a name="g8e-eval-v1-EvaluationDeploymentIdentity"></a>

### EvaluationDeploymentIdentity



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| deployment_id | [string](#string) |  |  |
| topology_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| runtime_boundaries | [EvaluationRuntimeBoundary](#g8e-eval-v1-EvaluationRuntimeBoundary) | repeated |  |
| controlled_target | [string](#string) |  |  |
| independent_observer | [string](#string) |  |  |






<a name="g8e-eval-v1-EvaluationMetric"></a>

### EvaluationMetric



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| metric_id | [string](#string) |  |  |
| metric_version | [string](#string) |  |  |
| numerator | [int64](#int64) |  |  |
| denominator | [int64](#int64) |  |  |
| value | [double](#double) |  |  |
| unit | [EvaluationMetricUnit](#g8e-eval-v1-EvaluationMetricUnit) |  |  |
| direction | [EvaluationMetricDirection](#g8e-eval-v1-EvaluationMetricDirection) |  |  |
| eligible_population_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| missing_data_policy | [EvaluationMissingDataPolicy](#g8e-eval-v1-EvaluationMissingDataPolicy) |  |  |
| source_verdict_refs | [string](#string) | repeated |  |
| evidence_refs | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) | repeated |  |






<a name="g8e-eval-v1-EvaluationObservation"></a>

### EvaluationObservation



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| observation_id | [string](#string) |  |  |
| observation_type | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| source | [EvaluationObservationSource](#g8e-eval-v1-EvaluationObservationSource) |  |  |
| observed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| run_id | [string](#string) |  |  |
| scenario_id | [string](#string) |  |  |
| attempt_id | [string](#string) |  |  |
| authority | [EvaluationEvidenceAuthority](#g8e-eval-v1-EvaluationEvidenceAuthority) |  |  |
| value | [EvaluationValue](#g8e-eval-v1-EvaluationValue) |  |  |
| evidence_refs | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) | repeated |  |






<a name="g8e-eval-v1-EvaluationReport"></a>

### EvaluationReport



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| run | [EvaluationRun](#g8e-eval-v1-EvaluationRun) |  |  |
| attempts | [EvaluationAttempt](#g8e-eval-v1-EvaluationAttempt) | repeated |  |
| observations | [EvaluationObservation](#g8e-eval-v1-EvaluationObservation) | repeated |  |
| assertions | [EvaluationAssertion](#g8e-eval-v1-EvaluationAssertion) | repeated |  |
| verdicts | [EvaluationVerdict](#g8e-eval-v1-EvaluationVerdict) | repeated |  |
| metrics | [EvaluationMetric](#g8e-eval-v1-EvaluationMetric) | repeated |  |
| evidence_refs | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) | repeated |  |
| summary_status | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| required_verdict_count | [uint32](#uint32) |  |  |
| passed_verdict_count | [uint32](#uint32) |  |  |
| summary | [string](#string) |  |  |






<a name="g8e-eval-v1-EvaluationRun"></a>

### EvaluationRun



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| suite_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| deployment | [EvaluationDeploymentIdentity](#g8e-eval-v1-EvaluationDeploymentIdentity) |  |  |
| active_posture | [EvaluationGovernancePosture](#g8e-eval-v1-EvaluationGovernancePosture) |  |  |
| lane | [EvaluationLane](#g8e-eval-v1-EvaluationLane) |  |  |
| target_operator_id | [string](#string) |  |  |
| target_operator_session_id | [string](#string) |  |  |
| started_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| completed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| attempt_refs | [string](#string) | repeated |  |
| final_verification_report_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |






<a name="g8e-eval-v1-EvaluationRuntimeBoundary"></a>

### EvaluationRuntimeBoundary



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| component | [EvaluationRuntimeComponent](#g8e-eval-v1-EvaluationRuntimeComponent) |  |  |
| process_identity | [string](#string) |  |  |
| runtime_namespace | [string](#string) |  |  |
| mounted_filesystems | [string](#string) | repeated |  |
| persistent_store | [string](#string) |  |  |
| endpoint | [string](#string) |  |  |
| authenticated_identity | [string](#string) |  |  |
| execution_owner_operator_id | [string](#string) |  |  |






<a name="g8e-eval-v1-EvaluationTargetState"></a>

### EvaluationTargetState



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| scenario_id | [string](#string) |  |  |
| attempt_id | [string](#string) |  |  |
| target_resource | [string](#string) |  |  |
| observed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| present | [bool](#bool) |  |  |
| content | [bytes](#bytes) |  |  |






<a name="g8e-eval-v1-EvaluationValue"></a>

### EvaluationValue



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| boolean_value | [bool](#bool) |  |  |
| integer_value | [int64](#int64) |  |  |
| string_value | [string](#string) |  |  |
| artifact_reference | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |






<a name="g8e-eval-v1-EvaluationVerdict"></a>

### EvaluationVerdict



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| verdict_id | [string](#string) |  |  |
| assertion_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| status | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| observed_refs | [string](#string) | repeated |  |
| evidence_refs | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) | repeated |  |
| grader_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| evaluated_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| failure_reason | [string](#string) |  |  |





 


<a name="g8e-eval-v1-EvaluationAttemptStatus"></a>

### EvaluationAttemptStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_ATTEMPT_STATUS_UNSPECIFIED | 0 |  |
| EVALUATION_ATTEMPT_STATUS_COMPLETED | 1 |  |
| EVALUATION_ATTEMPT_STATUS_REJECTED | 2 |  |
| EVALUATION_ATTEMPT_STATUS_FAILED | 3 |  |
| EVALUATION_ATTEMPT_STATUS_UNAVAILABLE | 4 |  |
| EVALUATION_ATTEMPT_STATUS_UNSUPPORTED | 5 |  |
| EVALUATION_ATTEMPT_STATUS_INVALID_EVIDENCE | 6 |  |



<a name="g8e-eval-v1-EvaluationComparator"></a>

### EvaluationComparator


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_COMPARATOR_UNSPECIFIED | 0 |  |
| EVALUATION_COMPARATOR_EQUAL | 1 |  |
| EVALUATION_COMPARATOR_NOT_EQUAL | 2 |  |
| EVALUATION_COMPARATOR_GREATER_THAN | 3 |  |
| EVALUATION_COMPARATOR_GREATER_THAN_OR_EQUAL | 4 |  |
| EVALUATION_COMPARATOR_LESS_THAN | 5 |  |
| EVALUATION_COMPARATOR_LESS_THAN_OR_EQUAL | 6 |  |



<a name="g8e-eval-v1-EvaluationEvidenceAuthority"></a>

### EvaluationEvidenceAuthority


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_EVIDENCE_AUTHORITY_UNSPECIFIED | 0 |  |
| EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE | 1 |  |
| EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE | 2 |  |
| EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION | 3 |  |
| EVALUATION_EVIDENCE_AUTHORITY_HARNESS_EXCHANGE | 4 |  |



<a name="g8e-eval-v1-EvaluationGovernancePosture"></a>

### EvaluationGovernancePosture


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_GOVERNANCE_POSTURE_UNSPECIFIED | 0 |  |
| EVALUATION_GOVERNANCE_POSTURE_DOCTRINE | 1 |  |
| EVALUATION_GOVERNANCE_POSTURE_CONSENSUS | 2 |  |
| EVALUATION_GOVERNANCE_POSTURE_RATIFY | 3 |  |
| EVALUATION_GOVERNANCE_POSTURE_NOTARY | 4 |  |



<a name="g8e-eval-v1-EvaluationLane"></a>

### EvaluationLane


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_LANE_UNSPECIFIED | 0 |  |
| EVALUATION_LANE_PLATFORM | 1 |  |



<a name="g8e-eval-v1-EvaluationMetricDirection"></a>

### EvaluationMetricDirection


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_METRIC_DIRECTION_UNSPECIFIED | 0 |  |
| EVALUATION_METRIC_DIRECTION_HIGHER_IS_BETTER | 1 |  |
| EVALUATION_METRIC_DIRECTION_LOWER_IS_BETTER | 2 |  |
| EVALUATION_METRIC_DIRECTION_TARGET_IS_BETTER | 3 |  |



<a name="g8e-eval-v1-EvaluationMetricUnit"></a>

### EvaluationMetricUnit


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_METRIC_UNIT_UNSPECIFIED | 0 |  |
| EVALUATION_METRIC_UNIT_COUNT | 1 |  |
| EVALUATION_METRIC_UNIT_RATIO | 2 |  |



<a name="g8e-eval-v1-EvaluationMissingDataPolicy"></a>

### EvaluationMissingDataPolicy


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_MISSING_DATA_POLICY_UNSPECIFIED | 0 |  |
| EVALUATION_MISSING_DATA_POLICY_FAIL | 1 |  |
| EVALUATION_MISSING_DATA_POLICY_EXCLUDE | 2 |  |
| EVALUATION_MISSING_DATA_POLICY_UNAVAILABLE | 3 |  |



<a name="g8e-eval-v1-EvaluationObservationSource"></a>

### EvaluationObservationSource


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_OBSERVATION_SOURCE_UNSPECIFIED | 0 |  |
| EVALUATION_OBSERVATION_SOURCE_TARGET_OBSERVER | 1 |  |
| EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT | 2 |  |
| EVALUATION_OBSERVATION_SOURCE_GATEWAY_ADMISSION | 3 |  |
| EVALUATION_OBSERVATION_SOURCE_HARNESS_EXCHANGE | 4 |  |



<a name="g8e-eval-v1-EvaluationRuntimeComponent"></a>

### EvaluationRuntimeComponent


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_RUNTIME_COMPONENT_UNSPECIFIED | 0 |  |
| EVALUATION_RUNTIME_COMPONENT_EVALUATOR | 1 |  |
| EVALUATION_RUNTIME_COMPONENT_GATEWAY | 2 |  |
| EVALUATION_RUNTIME_COMPONENT_OPERATOR | 3 |  |
| EVALUATION_RUNTIME_COMPONENT_CONTROLLED_TARGET | 4 |  |



<a name="g8e-eval-v1-EvaluationVerdictStatus"></a>

### EvaluationVerdictStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_VERDICT_STATUS_UNSPECIFIED | 0 |  |
| EVALUATION_VERDICT_STATUS_PASS | 1 |  |
| EVALUATION_VERDICT_STATUS_FAIL | 2 |  |
| EVALUATION_VERDICT_STATUS_UNAVAILABLE | 3 |  |
| EVALUATION_VERDICT_STATUS_UNSUPPORTED | 4 |  |
| EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE | 5 |  |


 

 

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

