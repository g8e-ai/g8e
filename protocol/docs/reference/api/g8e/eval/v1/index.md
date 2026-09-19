# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [g8e/eval/v1/eval.proto](#g8e_eval_v1_eval-proto)
    - [DecomposedScoreRecord](#g8e-eval-v1-DecomposedScoreRecord)
    - [DeterministicGrade](#g8e-eval-v1-DeterministicGrade)
    - [EscalationRecord](#g8e-eval-v1-EscalationRecord)
    - [EvaluationAssertion](#g8e-eval-v1-EvaluationAssertion)
    - [EvaluationAssignment](#g8e-eval-v1-EvaluationAssignment)
    - [EvaluationAssignmentResult](#g8e-eval-v1-EvaluationAssignmentResult)
    - [EvaluationAttempt](#g8e-eval-v1-EvaluationAttempt)
    - [EvaluationCampaignSpec](#g8e-eval-v1-EvaluationCampaignSpec)
    - [EvaluationDeploymentIdentity](#g8e-eval-v1-EvaluationDeploymentIdentity)
    - [EvaluationMetric](#g8e-eval-v1-EvaluationMetric)
    - [EvaluationObservation](#g8e-eval-v1-EvaluationObservation)
    - [EvaluationReport](#g8e-eval-v1-EvaluationReport)
    - [EvaluationRun](#g8e-eval-v1-EvaluationRun)
    - [EvaluationRuntimeBoundary](#g8e-eval-v1-EvaluationRuntimeBoundary)
    - [EvaluationScenarioCatalog](#g8e-eval-v1-EvaluationScenarioCatalog)
    - [EvaluationScenarioDefinition](#g8e-eval-v1-EvaluationScenarioDefinition)
    - [EvaluationTargetState](#g8e-eval-v1-EvaluationTargetState)
    - [EvaluationValue](#g8e-eval-v1-EvaluationValue)
    - [EvaluationVerdict](#g8e-eval-v1-EvaluationVerdict)
    - [EvaluationVerificationReport](#g8e-eval-v1-EvaluationVerificationReport)
    - [GovernedActionBinding](#g8e-eval-v1-GovernedActionBinding)
    - [GraderModelCallRecord](#g8e-eval-v1-GraderModelCallRecord)
    - [HandoffRecord](#g8e-eval-v1-HandoffRecord)
    - [HeterogeneousAssignmentTarget](#g8e-eval-v1-HeterogeneousAssignmentTarget)
    - [HeterogeneousStackDefinition](#g8e-eval-v1-HeterogeneousStackDefinition)
    - [HomogeneousAssignmentTarget](#g8e-eval-v1-HomogeneousAssignmentTarget)
    - [ModelCampaignBinding](#g8e-eval-v1-ModelCampaignBinding)
    - [ModelCapabilityObservation](#g8e-eval-v1-ModelCapabilityObservation)
    - [ModelInferenceRecord](#g8e-eval-v1-ModelInferenceRecord)
    - [ModelProvenanceAttestationWindow](#g8e-eval-v1-ModelProvenanceAttestationWindow)
    - [ModelProvenanceObservationCommand](#g8e-eval-v1-ModelProvenanceObservationCommand)
    - [ModelProvenanceObservationCompleted](#g8e-eval-v1-ModelProvenanceObservationCompleted)
    - [ModelVariant](#g8e-eval-v1-ModelVariant)
    - [ModelWeightAttestation](#g8e-eval-v1-ModelWeightAttestation)
    - [ProviderBoundaryHardwareSample](#g8e-eval-v1-ProviderBoundaryHardwareSample)
    - [ProviderBoundaryObservationCommand](#g8e-eval-v1-ProviderBoundaryObservationCommand)
    - [ProviderBoundaryObservationCompleted](#g8e-eval-v1-ProviderBoundaryObservationCompleted)
    - [ProviderBoundaryObservationWindow](#g8e-eval-v1-ProviderBoundaryObservationWindow)
    - [PublicAssignmentLifecycleRecord](#g8e-eval-v1-PublicAssignmentLifecycleRecord)
    - [PublicAssignmentResultProjection](#g8e-eval-v1-PublicAssignmentResultProjection)
    - [PublicCampaignIdentity](#g8e-eval-v1-PublicCampaignIdentity)
    - [PublicModelCallSummary](#g8e-eval-v1-PublicModelCallSummary)
    - [PublicModelVariantIdentity](#g8e-eval-v1-PublicModelVariantIdentity)
    - [RecoveryRecord](#g8e-eval-v1-RecoveryRecord)
    - [RoleAssignment](#g8e-eval-v1-RoleAssignment)
    - [SemanticGrade](#g8e-eval-v1-SemanticGrade)
    - [ToolCallRecord](#g8e-eval-v1-ToolCallRecord)
    - [ToolDecisionRecord](#g8e-eval-v1-ToolDecisionRecord)
  
    - [EvaluationAssignmentLifecycleStatus](#g8e-eval-v1-EvaluationAssignmentLifecycleStatus)
    - [EvaluationAttemptStatus](#g8e-eval-v1-EvaluationAttemptStatus)
    - [EvaluationComparator](#g8e-eval-v1-EvaluationComparator)
    - [EvaluationEvidenceAuthority](#g8e-eval-v1-EvaluationEvidenceAuthority)
    - [EvaluationGovernancePosture](#g8e-eval-v1-EvaluationGovernancePosture)
    - [EvaluationGradingMethod](#g8e-eval-v1-EvaluationGradingMethod)
    - [EvaluationLane](#g8e-eval-v1-EvaluationLane)
    - [EvaluationLoadState](#g8e-eval-v1-EvaluationLoadState)
    - [EvaluationMetricDirection](#g8e-eval-v1-EvaluationMetricDirection)
    - [EvaluationMetricUnit](#g8e-eval-v1-EvaluationMetricUnit)
    - [EvaluationMissingDataPolicy](#g8e-eval-v1-EvaluationMissingDataPolicy)
    - [EvaluationObservationSource](#g8e-eval-v1-EvaluationObservationSource)
    - [EvaluationRuntimeComponent](#g8e-eval-v1-EvaluationRuntimeComponent)
    - [EvaluationScenarioCategory](#g8e-eval-v1-EvaluationScenarioCategory)
    - [EvaluationUsageAvailability](#g8e-eval-v1-EvaluationUsageAvailability)
    - [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus)
    - [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole)
    - [ModelCapabilityKind](#g8e-eval-v1-ModelCapabilityKind)
    - [ModelManifestVerificationStatus](#g8e-eval-v1-ModelManifestVerificationStatus)
    - [ModelProvenanceObservationAttemptStatus](#g8e-eval-v1-ModelProvenanceObservationAttemptStatus)
    - [ModelProvenanceObservationPhase](#g8e-eval-v1-ModelProvenanceObservationPhase)
    - [ProviderBoundaryObservationAttemptStatus](#g8e-eval-v1-ProviderBoundaryObservationAttemptStatus)
    - [ProviderBoundaryObservationPhase](#g8e-eval-v1-ProviderBoundaryObservationPhase)
    - [ProviderHardwareMetricAvailability](#g8e-eval-v1-ProviderHardwareMetricAvailability)
  
- [Scalar Value Types](#scalar-value-types)



<a name="g8e_eval_v1_eval-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## g8e/eval/v1/eval.proto



<a name="g8e-eval-v1-DecomposedScoreRecord"></a>

### DecomposedScoreRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| score_id | [string](#string) |  |  |
| dimension | [string](#string) |  |  |
| value | [double](#double) |  |  |
| unit | [EvaluationMetricUnit](#g8e-eval-v1-EvaluationMetricUnit) |  |  |
| direction | [EvaluationMetricDirection](#g8e-eval-v1-EvaluationMetricDirection) |  |  |
| missing_data_policy | [EvaluationMissingDataPolicy](#g8e-eval-v1-EvaluationMissingDataPolicy) |  |  |






<a name="g8e-eval-v1-DeterministicGrade"></a>

### DeterministicGrade



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| grade_id | [string](#string) |  |  |
| criterion_id | [string](#string) |  |  |
| status | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| score | [double](#double) |  |  |
| detail | [string](#string) |  |  |






<a name="g8e-eval-v1-EscalationRecord"></a>

### EscalationRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| escalation_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| from_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| to_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| justified | [bool](#bool) |  |  |
| reason_code | [string](#string) |  |  |






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






<a name="g8e-eval-v1-EvaluationAssignment"></a>

### EvaluationAssignment
EvaluationAssignment is one scheduled model-role or system-lane cell.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| deterministic_identity | [string](#string) |  |  |
| campaign_id | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| scenario_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| scenario_id | [string](#string) |  |  |
| lane | [EvaluationLane](#g8e-eval-v1-EvaluationLane) |  |  |
| lifecycle_status | [EvaluationAssignmentLifecycleStatus](#g8e-eval-v1-EvaluationAssignmentLifecycleStatus) |  |  |
| repetition | [uint32](#uint32) |  |  |
| queued_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| started_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| completed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |
| homogeneous | [HomogeneousAssignmentTarget](#g8e-eval-v1-HomogeneousAssignmentTarget) |  |  |
| heterogeneous | [HeterogeneousAssignmentTarget](#g8e-eval-v1-HeterogeneousAssignmentTarget) |  |  |






<a name="g8e-eval-v1-EvaluationAssignmentResult"></a>

### EvaluationAssignmentResult
EvaluationAssignmentResult is the private terminal result for one assignment.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| campaign_id | [string](#string) |  |  |
| lane | [EvaluationLane](#g8e-eval-v1-EvaluationLane) |  |  |
| lifecycle_status | [EvaluationAssignmentLifecycleStatus](#g8e-eval-v1-EvaluationAssignmentLifecycleStatus) |  |  |
| result_digest | [string](#string) |  |  |
| model_inferences | [ModelInferenceRecord](#g8e-eval-v1-ModelInferenceRecord) | repeated |  |
| tool_decisions | [ToolDecisionRecord](#g8e-eval-v1-ToolDecisionRecord) | repeated |  |
| tool_calls | [ToolCallRecord](#g8e-eval-v1-ToolCallRecord) | repeated |  |
| escalations | [EscalationRecord](#g8e-eval-v1-EscalationRecord) | repeated |  |
| handoffs | [HandoffRecord](#g8e-eval-v1-HandoffRecord) | repeated |  |
| recoveries | [RecoveryRecord](#g8e-eval-v1-RecoveryRecord) | repeated |  |
| governed_actions | [GovernedActionBinding](#g8e-eval-v1-GovernedActionBinding) | repeated |  |
| deterministic_grades | [DeterministicGrade](#g8e-eval-v1-DeterministicGrade) | repeated |  |
| semantic_grades | [SemanticGrade](#g8e-eval-v1-SemanticGrade) | repeated |  |
| decomposed_scores | [DecomposedScoreRecord](#g8e-eval-v1-DecomposedScoreRecord) | repeated |  |
| grader_calls | [GraderModelCallRecord](#g8e-eval-v1-GraderModelCallRecord) | repeated |  |
| evidence_refs | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) | repeated |  |
| completed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |






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






<a name="g8e-eval-v1-EvaluationCampaignSpec"></a>

### EvaluationCampaignSpec
EvaluationCampaignSpec is the frozen private campaign definition.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| campaign_id | [string](#string) |  |  |
| catalog_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| catalog_digest | [string](#string) |  |  |
| model_registry | [ModelVariant](#g8e-eval-v1-ModelVariant) | repeated |  |
| model_registry_digest | [string](#string) |  |  |
| campaign_digest | [string](#string) |  |  |
| governance_posture | [EvaluationGovernancePosture](#g8e-eval-v1-EvaluationGovernancePosture) |  |  |
| scenario_count | [uint32](#uint32) |  |  |
| repetition_count | [uint32](#uint32) |  |  |






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
| assignment_results | [EvaluationAssignmentResult](#g8e-eval-v1-EvaluationAssignmentResult) | repeated |  |
| campaign_verification_report | [EvaluationVerificationReport](#g8e-eval-v1-EvaluationVerificationReport) |  |  |






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
| campaign_binding | [ModelCampaignBinding](#g8e-eval-v1-ModelCampaignBinding) |  |  |






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






<a name="g8e-eval-v1-EvaluationScenarioCatalog"></a>

### EvaluationScenarioCatalog
EvaluationScenarioCatalog is the immutable 25-scenario private catalog.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| catalog_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| catalog_digest | [string](#string) |  |  |
| scenarios | [EvaluationScenarioDefinition](#g8e-eval-v1-EvaluationScenarioDefinition) | repeated |  |






<a name="g8e-eval-v1-EvaluationScenarioDefinition"></a>

### EvaluationScenarioDefinition
EvaluationScenarioDefinition describes one frozen typed scenario.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| scenario_id | [string](#string) |  |  |
| scenario_version | [string](#string) |  |  |
| category | [EvaluationScenarioCategory](#g8e-eval-v1-EvaluationScenarioCategory) |  |  |
| public_description | [string](#string) |  |  |
| grading_method | [EvaluationGradingMethod](#g8e-eval-v1-EvaluationGradingMethod) |  |  |
| allowed_tools | [string](#string) | repeated |  |
| expected_tools | [string](#string) | repeated |  |
| forbidden_tools | [string](#string) | repeated |  |
| input_fixture_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |
| required_concepts | [string](#string) | repeated |  |
| gold_criteria_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |






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






<a name="g8e-eval-v1-EvaluationVerificationReport"></a>

### EvaluationVerificationReport
EvaluationVerificationReport independently verifies one run or assignment.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| report_id | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| status | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| failure_count | [uint32](#uint32) |  |  |
| failure_reasons | [string](#string) | repeated |  |
| report_digest_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |
| verified_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |






<a name="g8e-eval-v1-GovernedActionBinding"></a>

### GovernedActionBinding



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| binding_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| transaction_id | [string](#string) |  |  |
| operator_id | [string](#string) |  |  |
| operator_session_id | [string](#string) |  |  |
| receipt_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |
| persistence_attestation_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |
| policy_decision | [string](#string) |  |  |
| effect_observation_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |






<a name="g8e-eval-v1-GraderModelCallRecord"></a>

### GraderModelCallRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| grader_call_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| judge_variant_id | [string](#string) |  |  |
| inference_record_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |






<a name="g8e-eval-v1-HandoffRecord"></a>

### HandoffRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| handoff_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| from_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| to_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| reason_code | [string](#string) |  |  |






<a name="g8e-eval-v1-HeterogeneousAssignmentTarget"></a>

### HeterogeneousAssignmentTarget



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| stack | [HeterogeneousStackDefinition](#g8e-eval-v1-HeterogeneousStackDefinition) |  |  |






<a name="g8e-eval-v1-HeterogeneousStackDefinition"></a>

### HeterogeneousStackDefinition
HeterogeneousStackDefinition binds Primary, Assistant, and Lite slots.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| stack_id | [string](#string) |  |  |
| stack_digest | [string](#string) |  |  |
| primary_slot | [RoleAssignment](#g8e-eval-v1-RoleAssignment) |  |  |
| assistant_slot | [RoleAssignment](#g8e-eval-v1-RoleAssignment) |  |  |
| lite_slot | [RoleAssignment](#g8e-eval-v1-RoleAssignment) |  |  |






<a name="g8e-eval-v1-HomogeneousAssignmentTarget"></a>

### HomogeneousAssignmentTarget



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| candidate_variant | [ModelVariant](#g8e-eval-v1-ModelVariant) |  |  |
| designated_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |






<a name="g8e-eval-v1-ModelCampaignBinding"></a>

### ModelCampaignBinding
ModelCampaignBinding pins immutable campaign identity on a run.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| campaign_id | [string](#string) |  |  |
| campaign_digest | [string](#string) |  |  |
| catalog_ref | [g8e.compliance.v1.VersionedReference](#g8e-compliance-v1-VersionedReference) |  |  |
| catalog_digest | [string](#string) |  |  |
| model_registry_digest | [string](#string) |  |  |
| inference_operator_session_id | [string](#string) |  |  |
| data_operator_session_id | [string](#string) |  |  |






<a name="g8e-eval-v1-ModelCapabilityObservation"></a>

### ModelCapabilityObservation
ModelCapabilityObservation records a descriptive probe result. It never
authorizes assignment exclusion.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| capability | [ModelCapabilityKind](#g8e-eval-v1-ModelCapabilityKind) |  |  |
| outcome | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| observation_detail | [string](#string) |  |  |






<a name="g8e-eval-v1-ModelInferenceRecord"></a>

### ModelInferenceRecord
ModelInferenceRecord captures one governed scored inference call.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| inference_record_id | [string](#string) |  |  |
| provider_attempt_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| evaluation_attempt_id | [string](#string) |  |  |
| model_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| agent_persona | [string](#string) |  |  |
| call_site | [string](#string) |  |  |
| model_variant | [ModelVariant](#g8e-eval-v1-ModelVariant) |  |  |
| temperature | [float](#float) |  |  |
| top_p | [float](#float) | optional |  |
| top_k | [int32](#int32) | optional |  |
| seed | [int32](#int32) | optional |  |
| max_output_tokens | [uint32](#uint32) |  |  |
| input_hash | [string](#string) |  |  |
| output_hash | [string](#string) |  |  |
| usage_availability | [EvaluationUsageAvailability](#g8e-eval-v1-EvaluationUsageAvailability) |  |  |
| prompt_tokens | [uint32](#uint32) |  |  |
| completion_tokens | [uint32](#uint32) |  |  |
| thinking_tokens | [uint32](#uint32) |  |  |
| cache_tokens | [uint32](#uint32) |  |  |
| request_started_at_unix_nanos | [uint64](#uint64) |  |  |
| first_token_at_unix_nanos | [uint64](#uint64) |  |  |
| generation_duration_nanos | [uint64](#uint64) |  |  |
| total_duration_nanos | [uint64](#uint64) |  |  |
| load_duration_nanos | [uint64](#uint64) |  |  |
| load_state | [EvaluationLoadState](#g8e-eval-v1-EvaluationLoadState) |  |  |
| retry_count | [uint32](#uint32) |  |  |
| finish_reason | [string](#string) |  |  |
| privacy_attested | [bool](#bool) |  |  |
| governed_receipt_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |
| result_digest | [string](#string) |  |  |
| provider_boundary_observation_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |






<a name="g8e-eval-v1-ModelProvenanceAttestationWindow"></a>

### ModelProvenanceAttestationWindow
ModelProvenanceAttestationWindow binds cryptographic model-weight evidence
to one governed inference provider attempt.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| provider_attempt_id | [string](#string) |  |  |
| provenance_operator_id | [string](#string) |  |  |
| served_model_tag | [string](#string) |  |  |
| expected_model_digest | [string](#string) |  |  |
| observed_model_digest | [string](#string) |  |  |
| manifest_digest | [string](#string) |  |  |
| manifest_verification_status | [ModelManifestVerificationStatus](#g8e-eval-v1-ModelManifestVerificationStatus) |  |  |
| weight_attestations | [ModelWeightAttestation](#g8e-eval-v1-ModelWeightAttestation) | repeated |  |
| attempt_started_at_unix_ms | [int64](#int64) |  |  |
| attempt_completed_at_unix_ms | [int64](#int64) |  |  |
| attested_at_unix_ms | [int64](#int64) |  |  |
| digest_match | [bool](#bool) |  |  |
| attestation_digest | [string](#string) |  |  |






<a name="g8e-eval-v1-ModelProvenanceObservationCommand"></a>

### ModelProvenanceObservationCommand
ModelProvenanceObservationCommand is the pubsub cmd-channel payload the
campaign Gateway sends to the remote Provenance Operator at the model
storage site.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| provider_attempt_id | [string](#string) |  |  |
| inference_transaction_id | [string](#string) |  |  |
| phase | [ModelProvenanceObservationPhase](#g8e-eval-v1-ModelProvenanceObservationPhase) |  |  |
| attempt_started_at_unix_ms | [int64](#int64) |  |  |
| attempt_completed_at_unix_ms | [int64](#int64) |  |  |
| attempt_status | [ModelProvenanceObservationAttemptStatus](#g8e-eval-v1-ModelProvenanceObservationAttemptStatus) |  |  |
| retry_count | [uint32](#uint32) |  |  |
| served_model_tag | [string](#string) |  |  |
| expected_model_digest | [string](#string) |  |  |
| model_registry_digest | [string](#string) |  |  |
| campaign_id | [string](#string) |  |  |






<a name="g8e-eval-v1-ModelProvenanceObservationCompleted"></a>

### ModelProvenanceObservationCompleted
ModelProvenanceObservationCompleted is the pubsub results-channel payload
published by the remote Provenance Operator after FINALIZE attestation.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| window | [ModelProvenanceAttestationWindow](#g8e-eval-v1-ModelProvenanceAttestationWindow) |  |  |






<a name="g8e-eval-v1-ModelVariant"></a>

### ModelVariant
ModelVariant is one frozen provider-backed model identity in a campaign.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| variant_id | [string](#string) |  |  |
| provider_class | [string](#string) |  |  |
| served_model_tag | [string](#string) |  |  |
| model_digest | [string](#string) |  |  |
| model_family | [string](#string) |  |  |
| parameter_count | [uint64](#uint64) |  |  |
| quantization | [string](#string) |  |  |
| context_limit | [uint32](#uint32) |  |  |
| capability_observations | [ModelCapabilityObservation](#g8e-eval-v1-ModelCapabilityObservation) | repeated |  |






<a name="g8e-eval-v1-ModelWeightAttestation"></a>

### ModelWeightAttestation
ModelWeightAttestation is one content-addressed model weight blob attested
by the storage-side Provenance Operator at the model file site.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| blob_digest | [string](#string) |  |  |
| size_bytes | [uint64](#uint64) |  |  |
| media_type | [string](#string) |  |  |






<a name="g8e-eval-v1-ProviderBoundaryHardwareSample"></a>

### ProviderBoundaryHardwareSample
ProviderBoundaryHardwareSample is one timestamped read-only hardware
observation from the external provider execution boundary.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| observed_at_unix_nanos | [uint64](#uint64) |  |  |
| device_pseudonym | [string](#string) |  |  |
| vram_bytes_availability | [ProviderHardwareMetricAvailability](#g8e-eval-v1-ProviderHardwareMetricAvailability) |  |  |
| vram_used_bytes | [uint64](#uint64) |  |  |
| vram_total_bytes | [uint64](#uint64) |  |  |
| gpu_utilization_availability | [ProviderHardwareMetricAvailability](#g8e-eval-v1-ProviderHardwareMetricAvailability) |  |  |
| gpu_utilization_percent | [float](#float) |  |  |
| temperature_availability | [ProviderHardwareMetricAvailability](#g8e-eval-v1-ProviderHardwareMetricAvailability) |  |  |
| temperature_celsius | [float](#float) |  |  |
| power_availability | [ProviderHardwareMetricAvailability](#g8e-eval-v1-ProviderHardwareMetricAvailability) |  |  |
| power_watts | [float](#float) |  |  |
| clock_availability | [ProviderHardwareMetricAvailability](#g8e-eval-v1-ProviderHardwareMetricAvailability) |  |  |
| clock_mhz | [uint32](#uint32) |  |  |
| host_ram_availability | [ProviderHardwareMetricAvailability](#g8e-eval-v1-ProviderHardwareMetricAvailability) |  |  |
| host_ram_used_bytes | [uint64](#uint64) |  |  |
| host_ram_total_bytes | [uint64](#uint64) |  |  |






<a name="g8e-eval-v1-ProviderBoundaryObservationCommand"></a>

### ProviderBoundaryObservationCommand
ProviderBoundaryObservationCommand is the pubsub cmd-channel payload the
campaign Gateway sends to the remote provider-boundary observer operator.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| provider_attempt_id | [string](#string) |  |  |
| inference_transaction_id | [string](#string) |  |  |
| phase | [ProviderBoundaryObservationPhase](#g8e-eval-v1-ProviderBoundaryObservationPhase) |  |  |
| attempt_started_at_unix_ms | [int64](#int64) |  |  |
| attempt_completed_at_unix_ms | [int64](#int64) |  |  |
| attempt_status | [ProviderBoundaryObservationAttemptStatus](#g8e-eval-v1-ProviderBoundaryObservationAttemptStatus) |  |  |
| retry_count | [uint32](#uint32) |  |  |






<a name="g8e-eval-v1-ProviderBoundaryObservationCompleted"></a>

### ProviderBoundaryObservationCompleted
ProviderBoundaryObservationCompleted is the pubsub results-channel payload
published by the remote observer operator after FINALIZE sampling completes.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| window | [ProviderBoundaryObservationWindow](#g8e-eval-v1-ProviderBoundaryObservationWindow) |  |  |






<a name="g8e-eval-v1-ProviderBoundaryObservationWindow"></a>

### ProviderBoundaryObservationWindow
ProviderBoundaryObservationWindow binds provider-boundary hardware samples
to one governed inference provider attempt.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| schema_version | [string](#string) |  |  |
| provider_attempt_id | [string](#string) |  |  |
| observer_id | [string](#string) |  |  |
| observer_clock_source | [string](#string) |  |  |
| window_started_at_unix_nanos | [uint64](#uint64) |  |  |
| window_completed_at_unix_nanos | [uint64](#uint64) |  |  |
| attempt_started_at_unix_ms | [int64](#int64) |  |  |
| attempt_completed_at_unix_ms | [int64](#int64) |  |  |
| clock_skew_nanos | [int64](#int64) |  |  |
| samples | [ProviderBoundaryHardwareSample](#g8e-eval-v1-ProviderBoundaryHardwareSample) | repeated |  |
| observation_digest | [string](#string) |  |  |






<a name="g8e-eval-v1-PublicAssignmentLifecycleRecord"></a>

### PublicAssignmentLifecycleRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| assignment_id | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| scenario_id | [string](#string) |  |  |
| scenario_category | [EvaluationScenarioCategory](#g8e-eval-v1-EvaluationScenarioCategory) |  |  |
| lane | [EvaluationLane](#g8e-eval-v1-EvaluationLane) |  |  |
| designated_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| variant_id | [string](#string) |  |  |
| stack_id | [string](#string) |  |  |
| lifecycle_status | [EvaluationAssignmentLifecycleStatus](#g8e-eval-v1-EvaluationAssignmentLifecycleStatus) |  |  |
| repetition | [uint32](#uint32) |  |  |
| observed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |






<a name="g8e-eval-v1-PublicAssignmentResultProjection"></a>

### PublicAssignmentResultProjection



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| assignment_id | [string](#string) |  |  |
| run_id | [string](#string) |  |  |
| scenario_id | [string](#string) |  |  |
| scenario_category | [EvaluationScenarioCategory](#g8e-eval-v1-EvaluationScenarioCategory) |  |  |
| lane | [EvaluationLane](#g8e-eval-v1-EvaluationLane) |  |  |
| designated_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| variant_id | [string](#string) |  |  |
| lifecycle_status | [EvaluationAssignmentLifecycleStatus](#g8e-eval-v1-EvaluationAssignmentLifecycleStatus) |  |  |
| summary_status | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| decomposed_scores | [DecomposedScoreRecord](#g8e-eval-v1-DecomposedScoreRecord) | repeated |  |
| result_digest | [string](#string) |  |  |
| verification_status | [string](#string) |  |  |
| unavailable_metric_reasons | [string](#string) | repeated |  |
| completed_at | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  |  |






<a name="g8e-eval-v1-PublicCampaignIdentity"></a>

### PublicCampaignIdentity



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| campaign_id | [string](#string) |  |  |
| campaign_digest | [string](#string) |  |  |
| catalog_id | [string](#string) |  |  |
| catalog_version | [string](#string) |  |  |
| catalog_digest | [string](#string) |  |  |
| model_registry_digest | [string](#string) |  |  |
| lane | [EvaluationLane](#g8e-eval-v1-EvaluationLane) |  |  |






<a name="g8e-eval-v1-PublicModelCallSummary"></a>

### PublicModelCallSummary



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| assignment_id | [string](#string) |  |  |
| inference_record_id | [string](#string) |  |  |
| model_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| agent_persona | [string](#string) |  |  |
| variant_id | [string](#string) |  |  |
| usage_availability | [EvaluationUsageAvailability](#g8e-eval-v1-EvaluationUsageAvailability) |  |  |
| prompt_tokens | [uint32](#uint32) |  |  |
| completion_tokens | [uint32](#uint32) |  |  |
| first_token_at_unix_nanos | [uint64](#uint64) |  |  |
| generation_duration_nanos | [uint64](#uint64) |  |  |
| load_state | [EvaluationLoadState](#g8e-eval-v1-EvaluationLoadState) |  |  |
| finish_reason | [string](#string) |  |  |
| input_hash | [string](#string) |  |  |
| output_hash | [string](#string) |  |  |






<a name="g8e-eval-v1-PublicModelVariantIdentity"></a>

### PublicModelVariantIdentity



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| variant_id | [string](#string) |  |  |
| served_model_tag | [string](#string) |  |  |
| model_digest | [string](#string) |  |  |
| model_family | [string](#string) |  |  |
| quantization | [string](#string) |  |  |






<a name="g8e-eval-v1-RecoveryRecord"></a>

### RecoveryRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| recovery_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| recovery_kind | [string](#string) |  |  |
| outcome | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| detail | [string](#string) |  |  |






<a name="g8e-eval-v1-RoleAssignment"></a>

### RoleAssignment
RoleAssignment binds one designated responsibility to one frozen variant.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| designated_role | [ModelCampaignRole](#g8e-eval-v1-ModelCampaignRole) |  |  |
| variant_id | [string](#string) |  |  |






<a name="g8e-eval-v1-SemanticGrade"></a>

### SemanticGrade



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| grade_id | [string](#string) |  |  |
| criterion_id | [string](#string) |  |  |
| status | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| judge_variant_id | [string](#string) |  |  |
| grader_call_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |
| detail | [string](#string) |  |  |






<a name="g8e-eval-v1-ToolCallRecord"></a>

### ToolCallRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| call_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| tool_name | [string](#string) |  |  |
| arguments_hash | [string](#string) |  |  |
| schema_outcome | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| semantic_outcome | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |
| governed_binding_ref | [g8e.compliance.v1.ComplianceEvidenceReference](#g8e-compliance-v1-ComplianceEvidenceReference) |  |  |






<a name="g8e-eval-v1-ToolDecisionRecord"></a>

### ToolDecisionRecord



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| decision_id | [string](#string) |  |  |
| assignment_id | [string](#string) |  |  |
| tool_name | [string](#string) |  |  |
| recognized | [bool](#bool) |  |  |
| selected | [bool](#bool) |  |  |
| permission_compliant | [bool](#bool) |  |  |
| unnecessary | [bool](#bool) |  |  |
| outcome | [EvaluationVerdictStatus](#g8e-eval-v1-EvaluationVerdictStatus) |  |  |





 


<a name="g8e-eval-v1-EvaluationAssignmentLifecycleStatus"></a>

### EvaluationAssignmentLifecycleStatus


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNSPECIFIED | 0 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED | 1 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING | 2 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED | 3 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED | 4 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL | 5 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_ESCALATED | 6 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_STOPPED | 7 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED | 8 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_GRADER_FAILED | 9 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_POLICY_REJECTED | 10 |  |
| EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNAVAILABLE | 11 |  |



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



<a name="g8e-eval-v1-EvaluationGradingMethod"></a>

### EvaluationGradingMethod


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_GRADING_METHOD_UNSPECIFIED | 0 |  |
| EVALUATION_GRADING_METHOD_DETERMINISTIC | 1 |  |
| EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE | 2 |  |



<a name="g8e-eval-v1-EvaluationLane"></a>

### EvaluationLane


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_LANE_UNSPECIFIED | 0 |  |
| EVALUATION_LANE_PLATFORM | 1 |  |
| EVALUATION_LANE_MODEL_ROLE | 2 |  |
| EVALUATION_LANE_SYSTEM | 3 |  |



<a name="g8e-eval-v1-EvaluationLoadState"></a>

### EvaluationLoadState


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_LOAD_STATE_UNSPECIFIED | 0 |  |
| EVALUATION_LOAD_STATE_COLD | 1 |  |
| EVALUATION_LOAD_STATE_WARM | 2 |  |
| EVALUATION_LOAD_STATE_UNAVAILABLE | 3 |  |



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



<a name="g8e-eval-v1-EvaluationScenarioCategory"></a>

### EvaluationScenarioCategory


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_SCENARIO_CATEGORY_UNSPECIFIED | 0 |  |
| EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE | 1 |  |
| EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION | 2 |  |
| EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT | 3 |  |
| EVALUATION_SCENARIO_CATEGORY_TECHNICAL_ANALYSIS | 4 |  |
| EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION | 5 |  |
| EVALUATION_SCENARIO_CATEGORY_VERIFICATION | 6 |  |
| EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY | 7 |  |
| EVALUATION_SCENARIO_CATEGORY_RECOVERY | 8 |  |
| EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE | 9 |  |



<a name="g8e-eval-v1-EvaluationUsageAvailability"></a>

### EvaluationUsageAvailability


| Name | Number | Description |
| ---- | ------ | ----------- |
| EVALUATION_USAGE_AVAILABILITY_UNSPECIFIED | 0 |  |
| EVALUATION_USAGE_AVAILABILITY_REPORTED | 1 |  |
| EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE | 2 |  |



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



<a name="g8e-eval-v1-ModelCampaignRole"></a>

### ModelCampaignRole


| Name | Number | Description |
| ---- | ------ | ----------- |
| MODEL_CAMPAIGN_ROLE_UNSPECIFIED | 0 |  |
| MODEL_CAMPAIGN_ROLE_PRIMARY | 1 |  |
| MODEL_CAMPAIGN_ROLE_ASSISTANT | 2 |  |
| MODEL_CAMPAIGN_ROLE_LITE | 3 |  |



<a name="g8e-eval-v1-ModelCapabilityKind"></a>

### ModelCapabilityKind


| Name | Number | Description |
| ---- | ------ | ----------- |
| MODEL_CAPABILITY_KIND_UNSPECIFIED | 0 |  |
| MODEL_CAPABILITY_KIND_COMPLETION | 1 |  |
| MODEL_CAPABILITY_KIND_TOOL_CALLING | 2 |  |
| MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT | 3 |  |
| MODEL_CAPABILITY_KIND_THINKING | 4 |  |
| MODEL_CAPABILITY_KIND_CONTEXT_LIMIT | 5 |  |



<a name="g8e-eval-v1-ModelManifestVerificationStatus"></a>

### ModelManifestVerificationStatus
ModelManifestVerificationStatus reports signed-manifest verification outcome.

| Name | Number | Description |
| ---- | ------ | ----------- |
| MODEL_MANIFEST_VERIFICATION_STATUS_UNSPECIFIED | 0 |  |
| MODEL_MANIFEST_VERIFICATION_STATUS_UNSIGNED | 1 |  |
| MODEL_MANIFEST_VERIFICATION_STATUS_VERIFIED | 2 |  |
| MODEL_MANIFEST_VERIFICATION_STATUS_FAILED | 3 |  |
| MODEL_MANIFEST_VERIFICATION_STATUS_UNAVAILABLE | 4 |  |



<a name="g8e-eval-v1-ModelProvenanceObservationAttemptStatus"></a>

### ModelProvenanceObservationAttemptStatus
ModelProvenanceObservationAttemptStatus mirrors the terminal provider-attempt
disposition carried in FINALIZE commands without importing operator.proto.

| Name | Number | Description |
| ---- | ------ | ----------- |
| MODEL_PROVENANCE_OBSERVATION_ATTEMPT_STATUS_UNSPECIFIED | 0 |  |
| MODEL_PROVENANCE_OBSERVATION_ATTEMPT_STATUS_IN_PROGRESS | 1 |  |
| MODEL_PROVENANCE_OBSERVATION_ATTEMPT_STATUS_COMPLETED | 2 |  |
| MODEL_PROVENANCE_OBSERVATION_ATTEMPT_STATUS_FAILED | 3 |  |



<a name="g8e-eval-v1-ModelProvenanceObservationPhase"></a>

### ModelProvenanceObservationPhase
ModelProvenanceObservationPhase identifies one lifecycle transition for a
governed provider attempt on the remote Provenance Operator.

| Name | Number | Description |
| ---- | ------ | ----------- |
| MODEL_PROVENANCE_OBSERVATION_PHASE_UNSPECIFIED | 0 |  |
| MODEL_PROVENANCE_OBSERVATION_PHASE_BEGIN | 1 |  |
| MODEL_PROVENANCE_OBSERVATION_PHASE_FINALIZE | 2 |  |



<a name="g8e-eval-v1-ProviderBoundaryObservationAttemptStatus"></a>

### ProviderBoundaryObservationAttemptStatus
ProviderBoundaryObservationAttemptStatus mirrors the terminal provider-attempt
disposition carried in FINALIZE commands without importing operator.proto.

| Name | Number | Description |
| ---- | ------ | ----------- |
| PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_UNSPECIFIED | 0 |  |
| PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_IN_PROGRESS | 1 |  |
| PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_COMPLETED | 2 |  |
| PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_FAILED | 3 |  |



<a name="g8e-eval-v1-ProviderBoundaryObservationPhase"></a>

### ProviderBoundaryObservationPhase
ProviderBoundaryObservationPhase identifies one lifecycle transition for a
governed provider attempt on the remote observer operator.

| Name | Number | Description |
| ---- | ------ | ----------- |
| PROVIDER_BOUNDARY_OBSERVATION_PHASE_UNSPECIFIED | 0 |  |
| PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN | 1 |  |
| PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE | 2 |  |



<a name="g8e-eval-v1-ProviderHardwareMetricAvailability"></a>

### ProviderHardwareMetricAvailability


| Name | Number | Description |
| ---- | ------ | ----------- |
| PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNSPECIFIED | 0 |  |
| PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED | 1 |  |
| PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE | 2 |  |


 

 

 



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

