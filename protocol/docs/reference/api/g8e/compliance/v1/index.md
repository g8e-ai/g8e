# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [g8e/compliance/v1/compliance.proto](#g8e_compliance_v1_compliance-proto)
    - [AssessmentScope](#g8e-compliance-v1-AssessmentScope)
    - [ChecksumEntry](#g8e-compliance-v1-ChecksumEntry)
    - [ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference)
    - [ComplianceReportManifest](#g8e-compliance-v1-ComplianceReportManifest)
    - [ComplianceVerificationReport](#g8e-compliance-v1-ComplianceVerificationReport)
    - [ComponentInventoryEntry](#g8e-compliance-v1-ComponentInventoryEntry)
    - [ControlAssertionAssessment](#g8e-compliance-v1-ControlAssertionAssessment)
    - [ControlAssertionCatalog](#g8e-compliance-v1-ControlAssertionCatalog)
    - [ControlAssertionDefinition](#g8e-compliance-v1-ControlAssertionDefinition)
    - [ControlCrosswalk](#g8e-compliance-v1-ControlCrosswalk)
    - [ControlCrosswalkCatalog](#g8e-compliance-v1-ControlCrosswalkCatalog)
    - [DemoManifest](#g8e-compliance-v1-DemoManifest)
    - [DemoScenarioDefinition](#g8e-compliance-v1-DemoScenarioDefinition)
    - [DemoScenarioResult](#g8e-compliance-v1-DemoScenarioResult)
    - [DemoStepResult](#g8e-compliance-v1-DemoStepResult)
    - [EvidenceEncryptionMetadata](#g8e-compliance-v1-EvidenceEncryptionMetadata)
    - [FrameworkCatalog](#g8e-compliance-v1-FrameworkCatalog)
    - [FrameworkControlAssessment](#g8e-compliance-v1-FrameworkControlAssessment)
    - [FrameworkControlDefinition](#g8e-compliance-v1-FrameworkControlDefinition)
    - [FrameworkControlReference](#g8e-compliance-v1-FrameworkControlReference)
    - [FrameworkDefinition](#g8e-compliance-v1-FrameworkDefinition)
    - [NamedDigest](#g8e-compliance-v1-NamedDigest)
    - [ReportSignature](#g8e-compliance-v1-ReportSignature)
    - [VerificationFailure](#g8e-compliance-v1-VerificationFailure)
    - [VersionedReference](#g8e-compliance-v1-VersionedReference)
  
- [Scalar Value Types](#scalar-value-types)



<a name="g8e_compliance_v1_compliance-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## g8e/compliance/v1/compliance.proto



<a name="g8e-compliance-v1-AssessmentScope"></a>

### AssessmentScope



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| scope_id | [string](#string) |  |  |
| organization_id | [string](#string) |  |  |
| deployment_id | [string](#string) |  |  |
| product_version | [string](#string) |  |  |
| build_identity | [string](#string) |  |  |
| source_revision | [string](#string) |  |  |
| image_digests | [NamedDigest](#g8e-compliance-v1-NamedDigest) | repeated |  |
| component_inventory | [ComponentInventoryEntry](#g8e-compliance-v1-ComponentInventoryEntry) | repeated |  |
| network_topology_hash | [string](#string) |  |  |
| configuration_hashes | [NamedDigest](#g8e-compliance-v1-NamedDigest) | repeated |  |
| doctrine_bundle_hashes | [NamedDigest](#g8e-compliance-v1-NamedDigest) | repeated |  |
| consensus_policy_hashes | [NamedDigest](#g8e-compliance-v1-NamedDigest) | repeated |  |
| trust_anchor_ids | [string](#string) | repeated |  |
| cryptographic_mode | [string](#string) |  |  |
| assessment_window_start | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| assessment_window_end | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| excluded_components | [string](#string) | repeated |  |
| customer_responsibilities | [string](#string) | repeated |  |






<a name="g8e-compliance-v1-ChecksumEntry"></a>

### ChecksumEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| bundle_path | [string](#string) |  |  |
| sha256 | [string](#string) |  |  |






<a name="g8e-compliance-v1-ComplianceEvidenceReference"></a>

### ComplianceEvidenceReference



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| artifact_id | [string](#string) |  |  |
| artifact_type | [string](#string) |  |  |
| sha256 | [string](#string) |  |  |
| media_type | [string](#string) |  |  |
| schema_ref | [string](#string) |  |  |
| producer_identity | [string](#string) |  |  |
| produced_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| scope_id | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| attempt_id | [string](#string) |  |  |
| scenario_id | [string](#string) |  |  |
| transaction_id | [string](#string) |  |  |
| verification_status | [string](#string) |  |  |
| verifier_id | [string](#string) |  |  |
| verifier_version | [string](#string) |  |  |
| verified_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| bundle_path | [string](#string) |  |  |
| encryption | [EvidenceEncryptionMetadata](#g8e-compliance-v1-EvidenceEncryptionMetadata) |  |  |






<a name="g8e-compliance-v1-ComplianceReportManifest"></a>

### ComplianceReportManifest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| report_id | [string](#string) |  |  |
| report_schema_version | [string](#string) |  |  |
| generated_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| generator_identity | [string](#string) |  |  |
| generator_version | [string](#string) |  |  |
| scope_ref | [string](#string) |  |  |
| framework_refs | [VersionedReference](#g8e-compliance-v1-VersionedReference) | repeated |  |
| assertion_catalog_ref | [string](#string) |  |  |
| crosswalk_refs | [string](#string) | repeated |  |
| assessment_refs | [string](#string) | repeated |  |
| evidence_index_ref | [string](#string) |  |  |
| checksum_root | [string](#string) |  |  |
| signature | [ReportSignature](#g8e-compliance-v1-ReportSignature) |  |  |






<a name="g8e-compliance-v1-ComplianceVerificationReport"></a>

### ComplianceVerificationReport



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| report_id | [string](#string) |  |  |
| valid | [bool](#bool) |  |  |
| verified_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| verifier_id | [string](#string) |  |  |
| verifier_version | [string](#string) |  |  |
| failures | [VerificationFailure](#g8e-compliance-v1-VerificationFailure) | repeated |  |
| reproduced_checksum_root | [string](#string) |  |  |






<a name="g8e-compliance-v1-ComponentInventoryEntry"></a>

### ComponentInventoryEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| component_id | [string](#string) |  |  |
| component_type | [string](#string) |  |  |
| version | [string](#string) |  |  |
| digest | [string](#string) |  |  |






<a name="g8e-compliance-v1-ControlAssertionAssessment"></a>

### ControlAssertionAssessment



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| assessment_id | [string](#string) |  |  |
| scope_id | [string](#string) |  |  |
| assertion_ref | [VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| status | [string](#string) |  |  |
| evidence_level | [string](#string) |  |  |
| evaluated_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| verifier_ref | [VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| evidence_refs | [string](#string) | repeated |  |
| metric_refs | [string](#string) | repeated |  |
| freshness_status | [string](#string) |  |  |
| failure_reason | [string](#string) |  |  |
| limitations | [string](#string) | repeated |  |






<a name="g8e-compliance-v1-ControlAssertionCatalog"></a>

### ControlAssertionCatalog



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| catalog_id | [string](#string) |  |  |
| catalog_version | [string](#string) |  |  |
| sha256 | [string](#string) |  |  |
| assertions | [ControlAssertionDefinition](#g8e-compliance-v1-ControlAssertionDefinition) | repeated |  |






<a name="g8e-compliance-v1-ControlAssertionDefinition"></a>

### ControlAssertionDefinition



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| assertion_id | [string](#string) |  |  |
| assertion_version | [string](#string) |  |  |
| title | [string](#string) |  |  |
| statement | [string](#string) |  |  |
| category | [string](#string) |  |  |
| component_scope | [string](#string) | repeated |  |
| responsibility | [string](#string) |  |  |
| applicable_action_classes | [string](#string) | repeated |  |
| applicable_arms | [string](#string) | repeated |  |
| required_evidence_types | [string](#string) | repeated |  |
| required_grader_refs | [VersionedReference](#g8e-compliance-v1-VersionedReference) | repeated |  |
| required_verifier_refs | [VersionedReference](#g8e-compliance-v1-VersionedReference) | repeated |  |
| minimum_evidence_level | [string](#string) |  |  |
| validation_cycle | [string](#string) |  |  |
| missing_evidence_policy | [string](#string) |  |  |
| passing_rule | [string](#string) |  |  |
| exclusions | [string](#string) | repeated |  |






<a name="g8e-compliance-v1-ControlCrosswalk"></a>

### ControlCrosswalk



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| crosswalk_id | [string](#string) |  |  |
| crosswalk_version | [string](#string) |  |  |
| framework_ref | [VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| control_id | [string](#string) |  |  |
| assertion_refs | [VersionedReference](#g8e-compliance-v1-VersionedReference) | repeated |  |
| mapping_type | [string](#string) |  |  |
| rationale | [string](#string) |  |  |
| applicability_conditions | [string](#string) | repeated |  |
| responsibility | [string](#string) |  |  |
| required_evidence_level | [string](#string) |  |  |
| reviewed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| reviewer_identity | [string](#string) |  |  |






<a name="g8e-compliance-v1-ControlCrosswalkCatalog"></a>

### ControlCrosswalkCatalog



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| catalog_id | [string](#string) |  |  |
| catalog_version | [string](#string) |  |  |
| sha256 | [string](#string) |  |  |
| mappings | [ControlCrosswalk](#g8e-compliance-v1-ControlCrosswalk) | repeated |  |






<a name="g8e-compliance-v1-DemoManifest"></a>

### DemoManifest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| demo_id | [string](#string) |  |  |
| demo_version | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| scope_id | [string](#string) |  |  |
| generated_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| scenario_definition_refs | [VersionedReference](#g8e-compliance-v1-VersionedReference) | repeated |  |
| provenance_hashes | [NamedDigest](#g8e-compliance-v1-NamedDigest) | repeated |  |
| required_environment | [string](#string) | repeated |  |
| framework_control_refs | [FrameworkControlReference](#g8e-compliance-v1-FrameworkControlReference) | repeated |  |
| supported_lanes | [string](#string) | repeated |  |






<a name="g8e-compliance-v1-DemoScenarioDefinition"></a>

### DemoScenarioDefinition



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| scenario_id | [string](#string) |  |  |
| scenario_version | [string](#string) |  |  |
| display_number | [string](#string) |  |  |
| title | [string](#string) |  |  |
| purpose | [string](#string) |  |  |
| risk_category | [string](#string) |  |  |
| expected_action_classes | [string](#string) | repeated |  |
| expected_outcome | [string](#string) |  |  |
| expected_rejection_layer | [string](#string) |  |  |
| initial_state_fixture_ref | [string](#string) |  |  |
| terminal_state_assertions | [string](#string) | repeated |  |
| required_receipts | [string](#string) | repeated |  |
| required_deterministic_stages | [string](#string) | repeated |  |
| assertion_refs | [VersionedReference](#g8e-compliance-v1-VersionedReference) | repeated |  |
| framework_control_refs | [FrameworkControlReference](#g8e-compliance-v1-FrameworkControlReference) | repeated |  |
| required_evidence_types | [string](#string) | repeated |  |
| required_evidence_level | [string](#string) |  |  |
| timeout_seconds | [uint32](#uint32) |  |  |
| failure_policy | [string](#string) |  |  |
| harness_scenario | [string](#string) |  |  |






<a name="g8e-compliance-v1-DemoScenarioResult"></a>

### DemoScenarioResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| result_id | [string](#string) |  |  |
| scenario_ref | [VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| demo_id | [string](#string) |  |  |
| scope_id | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| started_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| completed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| status | [string](#string) |  |  |
| investigation_ids | [string](#string) | repeated |  |
| transaction_ids | [string](#string) | repeated |  |
| receipt_refs | [string](#string) | repeated |  |
| state_observation_refs | [string](#string) | repeated |  |
| metric_refs | [string](#string) | repeated |  |
| ksi_refs | [string](#string) | repeated |  |
| assertion_refs | [VersionedReference](#g8e-compliance-v1-VersionedReference) | repeated |  |
| framework_control_refs | [FrameworkControlReference](#g8e-compliance-v1-FrameworkControlReference) | repeated |  |
| step_results | [DemoStepResult](#g8e-compliance-v1-DemoStepResult) | repeated |  |
| verification_status | [string](#string) |  |  |
| failure | [string](#string) |  |  |
| limitations | [string](#string) | repeated |  |
| display_number | [string](#string) |  |  |
| title | [string](#string) |  |  |
| metrics_summary | [string](#string) |  |  |






<a name="g8e-compliance-v1-DemoStepResult"></a>

### DemoStepResult



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| step_id | [string](#string) |  |  |
| operation | [string](#string) |  |  |
| started_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| completed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| status | [string](#string) |  |  |
| exit_code | [int32](#int32) |  |  |
| protocol_result | [string](#string) |  |  |
| evidence_refs | [string](#string) | repeated |  |
| failure | [string](#string) |  |  |
| required | [bool](#bool) |  |  |






<a name="g8e-compliance-v1-EvidenceEncryptionMetadata"></a>

### EvidenceEncryptionMetadata



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| algorithm | [string](#string) |  |  |
| key_id | [string](#string) |  |  |
| authorization_scope | [string](#string) |  |  |
| plaintext_sha256 | [string](#string) |  |  |
| authenticated_metadata_sha256 | [string](#string) |  |  |






<a name="g8e-compliance-v1-FrameworkCatalog"></a>

### FrameworkCatalog



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| catalog_id | [string](#string) |  |  |
| catalog_version | [string](#string) |  |  |
| sha256 | [string](#string) |  |  |
| frameworks | [FrameworkDefinition](#g8e-compliance-v1-FrameworkDefinition) | repeated |  |






<a name="g8e-compliance-v1-FrameworkControlAssessment"></a>

### FrameworkControlAssessment



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| assessment_id | [string](#string) |  |  |
| scope_id | [string](#string) |  |  |
| framework_ref | [VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| control_id | [string](#string) |  |  |
| status | [string](#string) |  |  |
| responsibility | [string](#string) |  |  |
| mapping_refs | [string](#string) | repeated |  |
| assertion_assessment_refs | [string](#string) | repeated |  |
| customer_attestation_refs | [string](#string) | repeated |  |
| evidence_level | [string](#string) |  |  |
| findings | [string](#string) | repeated |  |
| limitations | [string](#string) | repeated |  |
| remediation | [string](#string) | repeated |  |






<a name="g8e-compliance-v1-FrameworkControlDefinition"></a>

### FrameworkControlDefinition



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| control_id | [string](#string) |  |  |
| title | [string](#string) |  |  |
| description | [string](#string) |  |  |
| applicability_rules | [string](#string) | repeated |  |
| responsibility | [string](#string) |  |  |
| source_reference | [string](#string) |  |  |
| support_status | [string](#string) |  |  |
| support_rationale | [string](#string) |  |  |






<a name="g8e-compliance-v1-FrameworkControlReference"></a>

### FrameworkControlReference



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| framework_ref | [VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| control_id | [string](#string) |  |  |






<a name="g8e-compliance-v1-FrameworkDefinition"></a>

### FrameworkDefinition



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| framework_id | [string](#string) |  |  |
| framework_version | [string](#string) |  |  |
| title | [string](#string) |  |  |
| publisher | [string](#string) |  |  |
| source | [string](#string) |  |  |
| catalog_sha256 | [string](#string) |  |  |
| effective_date | [string](#string) |  |  |
| controls | [FrameworkControlDefinition](#g8e-compliance-v1-FrameworkControlDefinition) | repeated |  |






<a name="g8e-compliance-v1-NamedDigest"></a>

### NamedDigest



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| sha256 | [string](#string) |  |  |






<a name="g8e-compliance-v1-ReportSignature"></a>

### ReportSignature



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key_id | [string](#string) |  |  |
| algorithm | [string](#string) |  |  |
| signed_sha256 | [string](#string) |  |  |
| signature | [string](#string) |  |  |






<a name="g8e-compliance-v1-VerificationFailure"></a>

### VerificationFailure



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| code | [string](#string) |  |  |
| subject_ref | [string](#string) |  |  |
| reason | [string](#string) |  |  |






<a name="g8e-compliance-v1-VersionedReference"></a>

### VersionedReference



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  |  |
| version | [string](#string) |  |  |





 

 

 

 



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

