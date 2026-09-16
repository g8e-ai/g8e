import datetime

from g8e.compliance.v1 import compliance_pb2 as _compliance_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class EvaluationLane(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_LANE_UNSPECIFIED: _ClassVar[EvaluationLane]
    EVALUATION_LANE_PLATFORM: _ClassVar[EvaluationLane]
    EVALUATION_LANE_MODEL_ROLE: _ClassVar[EvaluationLane]
    EVALUATION_LANE_SYSTEM: _ClassVar[EvaluationLane]

class ModelCampaignRole(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    MODEL_CAMPAIGN_ROLE_UNSPECIFIED: _ClassVar[ModelCampaignRole]
    MODEL_CAMPAIGN_ROLE_PRIMARY: _ClassVar[ModelCampaignRole]
    MODEL_CAMPAIGN_ROLE_ASSISTANT: _ClassVar[ModelCampaignRole]
    MODEL_CAMPAIGN_ROLE_LITE: _ClassVar[ModelCampaignRole]

class EvaluationScenarioCategory(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_SCENARIO_CATEGORY_UNSPECIFIED: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_TECHNICAL_ANALYSIS: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_VERIFICATION: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_RECOVERY: _ClassVar[EvaluationScenarioCategory]
    EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE: _ClassVar[EvaluationScenarioCategory]

class EvaluationAssignmentLifecycleStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNSPECIFIED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_ESCALATED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_STOPPED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_GRADER_FAILED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_POLICY_REJECTED: _ClassVar[EvaluationAssignmentLifecycleStatus]
    EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNAVAILABLE: _ClassVar[EvaluationAssignmentLifecycleStatus]

class EvaluationGradingMethod(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_GRADING_METHOD_UNSPECIFIED: _ClassVar[EvaluationGradingMethod]
    EVALUATION_GRADING_METHOD_DETERMINISTIC: _ClassVar[EvaluationGradingMethod]
    EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE: _ClassVar[EvaluationGradingMethod]

class ModelCapabilityKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    MODEL_CAPABILITY_KIND_UNSPECIFIED: _ClassVar[ModelCapabilityKind]
    MODEL_CAPABILITY_KIND_COMPLETION: _ClassVar[ModelCapabilityKind]
    MODEL_CAPABILITY_KIND_TOOL_CALLING: _ClassVar[ModelCapabilityKind]
    MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT: _ClassVar[ModelCapabilityKind]
    MODEL_CAPABILITY_KIND_THINKING: _ClassVar[ModelCapabilityKind]
    MODEL_CAPABILITY_KIND_CONTEXT_LIMIT: _ClassVar[ModelCapabilityKind]

class EvaluationLoadState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_LOAD_STATE_UNSPECIFIED: _ClassVar[EvaluationLoadState]
    EVALUATION_LOAD_STATE_COLD: _ClassVar[EvaluationLoadState]
    EVALUATION_LOAD_STATE_WARM: _ClassVar[EvaluationLoadState]
    EVALUATION_LOAD_STATE_UNAVAILABLE: _ClassVar[EvaluationLoadState]

class EvaluationUsageAvailability(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_USAGE_AVAILABILITY_UNSPECIFIED: _ClassVar[EvaluationUsageAvailability]
    EVALUATION_USAGE_AVAILABILITY_REPORTED: _ClassVar[EvaluationUsageAvailability]
    EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE: _ClassVar[EvaluationUsageAvailability]

class EvaluationGovernancePosture(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_GOVERNANCE_POSTURE_UNSPECIFIED: _ClassVar[EvaluationGovernancePosture]
    EVALUATION_GOVERNANCE_POSTURE_DOCTRINE: _ClassVar[EvaluationGovernancePosture]
    EVALUATION_GOVERNANCE_POSTURE_CONSENSUS: _ClassVar[EvaluationGovernancePosture]
    EVALUATION_GOVERNANCE_POSTURE_RATIFY: _ClassVar[EvaluationGovernancePosture]
    EVALUATION_GOVERNANCE_POSTURE_NOTARY: _ClassVar[EvaluationGovernancePosture]

class EvaluationRuntimeComponent(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_RUNTIME_COMPONENT_UNSPECIFIED: _ClassVar[EvaluationRuntimeComponent]
    EVALUATION_RUNTIME_COMPONENT_EVALUATOR: _ClassVar[EvaluationRuntimeComponent]
    EVALUATION_RUNTIME_COMPONENT_GATEWAY: _ClassVar[EvaluationRuntimeComponent]
    EVALUATION_RUNTIME_COMPONENT_OPERATOR: _ClassVar[EvaluationRuntimeComponent]
    EVALUATION_RUNTIME_COMPONENT_CONTROLLED_TARGET: _ClassVar[EvaluationRuntimeComponent]

class EvaluationAttemptStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_ATTEMPT_STATUS_UNSPECIFIED: _ClassVar[EvaluationAttemptStatus]
    EVALUATION_ATTEMPT_STATUS_COMPLETED: _ClassVar[EvaluationAttemptStatus]
    EVALUATION_ATTEMPT_STATUS_REJECTED: _ClassVar[EvaluationAttemptStatus]
    EVALUATION_ATTEMPT_STATUS_FAILED: _ClassVar[EvaluationAttemptStatus]
    EVALUATION_ATTEMPT_STATUS_UNAVAILABLE: _ClassVar[EvaluationAttemptStatus]
    EVALUATION_ATTEMPT_STATUS_UNSUPPORTED: _ClassVar[EvaluationAttemptStatus]
    EVALUATION_ATTEMPT_STATUS_INVALID_EVIDENCE: _ClassVar[EvaluationAttemptStatus]

class EvaluationObservationSource(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_OBSERVATION_SOURCE_UNSPECIFIED: _ClassVar[EvaluationObservationSource]
    EVALUATION_OBSERVATION_SOURCE_TARGET_OBSERVER: _ClassVar[EvaluationObservationSource]
    EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT: _ClassVar[EvaluationObservationSource]
    EVALUATION_OBSERVATION_SOURCE_GATEWAY_ADMISSION: _ClassVar[EvaluationObservationSource]
    EVALUATION_OBSERVATION_SOURCE_HARNESS_EXCHANGE: _ClassVar[EvaluationObservationSource]

class EvaluationEvidenceAuthority(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_EVIDENCE_AUTHORITY_UNSPECIFIED: _ClassVar[EvaluationEvidenceAuthority]
    EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE: _ClassVar[EvaluationEvidenceAuthority]
    EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE: _ClassVar[EvaluationEvidenceAuthority]
    EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION: _ClassVar[EvaluationEvidenceAuthority]
    EVALUATION_EVIDENCE_AUTHORITY_HARNESS_EXCHANGE: _ClassVar[EvaluationEvidenceAuthority]

class EvaluationComparator(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_COMPARATOR_UNSPECIFIED: _ClassVar[EvaluationComparator]
    EVALUATION_COMPARATOR_EQUAL: _ClassVar[EvaluationComparator]
    EVALUATION_COMPARATOR_NOT_EQUAL: _ClassVar[EvaluationComparator]
    EVALUATION_COMPARATOR_GREATER_THAN: _ClassVar[EvaluationComparator]
    EVALUATION_COMPARATOR_GREATER_THAN_OR_EQUAL: _ClassVar[EvaluationComparator]
    EVALUATION_COMPARATOR_LESS_THAN: _ClassVar[EvaluationComparator]
    EVALUATION_COMPARATOR_LESS_THAN_OR_EQUAL: _ClassVar[EvaluationComparator]

class EvaluationVerdictStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_VERDICT_STATUS_UNSPECIFIED: _ClassVar[EvaluationVerdictStatus]
    EVALUATION_VERDICT_STATUS_PASS: _ClassVar[EvaluationVerdictStatus]
    EVALUATION_VERDICT_STATUS_FAIL: _ClassVar[EvaluationVerdictStatus]
    EVALUATION_VERDICT_STATUS_UNAVAILABLE: _ClassVar[EvaluationVerdictStatus]
    EVALUATION_VERDICT_STATUS_UNSUPPORTED: _ClassVar[EvaluationVerdictStatus]
    EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE: _ClassVar[EvaluationVerdictStatus]

class EvaluationMetricDirection(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_METRIC_DIRECTION_UNSPECIFIED: _ClassVar[EvaluationMetricDirection]
    EVALUATION_METRIC_DIRECTION_HIGHER_IS_BETTER: _ClassVar[EvaluationMetricDirection]
    EVALUATION_METRIC_DIRECTION_LOWER_IS_BETTER: _ClassVar[EvaluationMetricDirection]
    EVALUATION_METRIC_DIRECTION_TARGET_IS_BETTER: _ClassVar[EvaluationMetricDirection]

class EvaluationMissingDataPolicy(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_MISSING_DATA_POLICY_UNSPECIFIED: _ClassVar[EvaluationMissingDataPolicy]
    EVALUATION_MISSING_DATA_POLICY_FAIL: _ClassVar[EvaluationMissingDataPolicy]
    EVALUATION_MISSING_DATA_POLICY_EXCLUDE: _ClassVar[EvaluationMissingDataPolicy]
    EVALUATION_MISSING_DATA_POLICY_UNAVAILABLE: _ClassVar[EvaluationMissingDataPolicy]

class EvaluationMetricUnit(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EVALUATION_METRIC_UNIT_UNSPECIFIED: _ClassVar[EvaluationMetricUnit]
    EVALUATION_METRIC_UNIT_COUNT: _ClassVar[EvaluationMetricUnit]
    EVALUATION_METRIC_UNIT_RATIO: _ClassVar[EvaluationMetricUnit]
EVALUATION_LANE_UNSPECIFIED: EvaluationLane
EVALUATION_LANE_PLATFORM: EvaluationLane
EVALUATION_LANE_MODEL_ROLE: EvaluationLane
EVALUATION_LANE_SYSTEM: EvaluationLane
MODEL_CAMPAIGN_ROLE_UNSPECIFIED: ModelCampaignRole
MODEL_CAMPAIGN_ROLE_PRIMARY: ModelCampaignRole
MODEL_CAMPAIGN_ROLE_ASSISTANT: ModelCampaignRole
MODEL_CAMPAIGN_ROLE_LITE: ModelCampaignRole
EVALUATION_SCENARIO_CATEGORY_UNSPECIFIED: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_INSTRUCTION_ADHERENCE: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_TOOL_SELECTION: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_TOOL_ARGUMENT: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_TECHNICAL_ANALYSIS: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_ROUTING_DELEGATION: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_VERIFICATION: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_SECURITY_POLICY: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_RECOVERY: EvaluationScenarioCategory
EVALUATION_SCENARIO_CATEGORY_FINAL_RESPONSE: EvaluationScenarioCategory
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNSPECIFIED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_QUEUED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_RUNNING: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_COMPLETED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_FAILED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PARTIAL: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_ESCALATED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_STOPPED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_PROVIDER_FAILED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_GRADER_FAILED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_POLICY_REJECTED: EvaluationAssignmentLifecycleStatus
EVALUATION_ASSIGNMENT_LIFECYCLE_STATUS_UNAVAILABLE: EvaluationAssignmentLifecycleStatus
EVALUATION_GRADING_METHOD_UNSPECIFIED: EvaluationGradingMethod
EVALUATION_GRADING_METHOD_DETERMINISTIC: EvaluationGradingMethod
EVALUATION_GRADING_METHOD_SEMANTIC_JUDGE: EvaluationGradingMethod
MODEL_CAPABILITY_KIND_UNSPECIFIED: ModelCapabilityKind
MODEL_CAPABILITY_KIND_COMPLETION: ModelCapabilityKind
MODEL_CAPABILITY_KIND_TOOL_CALLING: ModelCapabilityKind
MODEL_CAPABILITY_KIND_STRUCTURED_OUTPUT: ModelCapabilityKind
MODEL_CAPABILITY_KIND_THINKING: ModelCapabilityKind
MODEL_CAPABILITY_KIND_CONTEXT_LIMIT: ModelCapabilityKind
EVALUATION_LOAD_STATE_UNSPECIFIED: EvaluationLoadState
EVALUATION_LOAD_STATE_COLD: EvaluationLoadState
EVALUATION_LOAD_STATE_WARM: EvaluationLoadState
EVALUATION_LOAD_STATE_UNAVAILABLE: EvaluationLoadState
EVALUATION_USAGE_AVAILABILITY_UNSPECIFIED: EvaluationUsageAvailability
EVALUATION_USAGE_AVAILABILITY_REPORTED: EvaluationUsageAvailability
EVALUATION_USAGE_AVAILABILITY_UNAVAILABLE: EvaluationUsageAvailability
EVALUATION_GOVERNANCE_POSTURE_UNSPECIFIED: EvaluationGovernancePosture
EVALUATION_GOVERNANCE_POSTURE_DOCTRINE: EvaluationGovernancePosture
EVALUATION_GOVERNANCE_POSTURE_CONSENSUS: EvaluationGovernancePosture
EVALUATION_GOVERNANCE_POSTURE_RATIFY: EvaluationGovernancePosture
EVALUATION_GOVERNANCE_POSTURE_NOTARY: EvaluationGovernancePosture
EVALUATION_RUNTIME_COMPONENT_UNSPECIFIED: EvaluationRuntimeComponent
EVALUATION_RUNTIME_COMPONENT_EVALUATOR: EvaluationRuntimeComponent
EVALUATION_RUNTIME_COMPONENT_GATEWAY: EvaluationRuntimeComponent
EVALUATION_RUNTIME_COMPONENT_OPERATOR: EvaluationRuntimeComponent
EVALUATION_RUNTIME_COMPONENT_CONTROLLED_TARGET: EvaluationRuntimeComponent
EVALUATION_ATTEMPT_STATUS_UNSPECIFIED: EvaluationAttemptStatus
EVALUATION_ATTEMPT_STATUS_COMPLETED: EvaluationAttemptStatus
EVALUATION_ATTEMPT_STATUS_REJECTED: EvaluationAttemptStatus
EVALUATION_ATTEMPT_STATUS_FAILED: EvaluationAttemptStatus
EVALUATION_ATTEMPT_STATUS_UNAVAILABLE: EvaluationAttemptStatus
EVALUATION_ATTEMPT_STATUS_UNSUPPORTED: EvaluationAttemptStatus
EVALUATION_ATTEMPT_STATUS_INVALID_EVIDENCE: EvaluationAttemptStatus
EVALUATION_OBSERVATION_SOURCE_UNSPECIFIED: EvaluationObservationSource
EVALUATION_OBSERVATION_SOURCE_TARGET_OBSERVER: EvaluationObservationSource
EVALUATION_OBSERVATION_SOURCE_OPERATOR_RECEIPT: EvaluationObservationSource
EVALUATION_OBSERVATION_SOURCE_GATEWAY_ADMISSION: EvaluationObservationSource
EVALUATION_OBSERVATION_SOURCE_HARNESS_EXCHANGE: EvaluationObservationSource
EVALUATION_EVIDENCE_AUTHORITY_UNSPECIFIED: EvaluationEvidenceAuthority
EVALUATION_EVIDENCE_AUTHORITY_INDEPENDENT_TARGET_STATE: EvaluationEvidenceAuthority
EVALUATION_EVIDENCE_AUTHORITY_OPERATOR_AUTHORITATIVE: EvaluationEvidenceAuthority
EVALUATION_EVIDENCE_AUTHORITY_GATEWAY_COORDINATION: EvaluationEvidenceAuthority
EVALUATION_EVIDENCE_AUTHORITY_HARNESS_EXCHANGE: EvaluationEvidenceAuthority
EVALUATION_COMPARATOR_UNSPECIFIED: EvaluationComparator
EVALUATION_COMPARATOR_EQUAL: EvaluationComparator
EVALUATION_COMPARATOR_NOT_EQUAL: EvaluationComparator
EVALUATION_COMPARATOR_GREATER_THAN: EvaluationComparator
EVALUATION_COMPARATOR_GREATER_THAN_OR_EQUAL: EvaluationComparator
EVALUATION_COMPARATOR_LESS_THAN: EvaluationComparator
EVALUATION_COMPARATOR_LESS_THAN_OR_EQUAL: EvaluationComparator
EVALUATION_VERDICT_STATUS_UNSPECIFIED: EvaluationVerdictStatus
EVALUATION_VERDICT_STATUS_PASS: EvaluationVerdictStatus
EVALUATION_VERDICT_STATUS_FAIL: EvaluationVerdictStatus
EVALUATION_VERDICT_STATUS_UNAVAILABLE: EvaluationVerdictStatus
EVALUATION_VERDICT_STATUS_UNSUPPORTED: EvaluationVerdictStatus
EVALUATION_VERDICT_STATUS_INVALID_EVIDENCE: EvaluationVerdictStatus
EVALUATION_METRIC_DIRECTION_UNSPECIFIED: EvaluationMetricDirection
EVALUATION_METRIC_DIRECTION_HIGHER_IS_BETTER: EvaluationMetricDirection
EVALUATION_METRIC_DIRECTION_LOWER_IS_BETTER: EvaluationMetricDirection
EVALUATION_METRIC_DIRECTION_TARGET_IS_BETTER: EvaluationMetricDirection
EVALUATION_MISSING_DATA_POLICY_UNSPECIFIED: EvaluationMissingDataPolicy
EVALUATION_MISSING_DATA_POLICY_FAIL: EvaluationMissingDataPolicy
EVALUATION_MISSING_DATA_POLICY_EXCLUDE: EvaluationMissingDataPolicy
EVALUATION_MISSING_DATA_POLICY_UNAVAILABLE: EvaluationMissingDataPolicy
EVALUATION_METRIC_UNIT_UNSPECIFIED: EvaluationMetricUnit
EVALUATION_METRIC_UNIT_COUNT: EvaluationMetricUnit
EVALUATION_METRIC_UNIT_RATIO: EvaluationMetricUnit

class EvaluationRuntimeBoundary(_message.Message):
    __slots__ = ("component", "process_identity", "runtime_namespace", "mounted_filesystems", "persistent_store", "endpoint", "authenticated_identity", "execution_owner_operator_id")
    COMPONENT_FIELD_NUMBER: _ClassVar[int]
    PROCESS_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_NAMESPACE_FIELD_NUMBER: _ClassVar[int]
    MOUNTED_FILESYSTEMS_FIELD_NUMBER: _ClassVar[int]
    PERSISTENT_STORE_FIELD_NUMBER: _ClassVar[int]
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    AUTHENTICATED_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_OWNER_OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    component: EvaluationRuntimeComponent
    process_identity: str
    runtime_namespace: str
    mounted_filesystems: _containers.RepeatedScalarFieldContainer[str]
    persistent_store: str
    endpoint: str
    authenticated_identity: str
    execution_owner_operator_id: str
    def __init__(self, component: _Optional[_Union[EvaluationRuntimeComponent, str]] = ..., process_identity: _Optional[str] = ..., runtime_namespace: _Optional[str] = ..., mounted_filesystems: _Optional[_Iterable[str]] = ..., persistent_store: _Optional[str] = ..., endpoint: _Optional[str] = ..., authenticated_identity: _Optional[str] = ..., execution_owner_operator_id: _Optional[str] = ...) -> None: ...

class EvaluationDeploymentIdentity(_message.Message):
    __slots__ = ("deployment_id", "topology_ref", "runtime_boundaries", "controlled_target", "independent_observer")
    DEPLOYMENT_ID_FIELD_NUMBER: _ClassVar[int]
    TOPOLOGY_REF_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_BOUNDARIES_FIELD_NUMBER: _ClassVar[int]
    CONTROLLED_TARGET_FIELD_NUMBER: _ClassVar[int]
    INDEPENDENT_OBSERVER_FIELD_NUMBER: _ClassVar[int]
    deployment_id: str
    topology_ref: _compliance_pb2.VersionedReference
    runtime_boundaries: _containers.RepeatedCompositeFieldContainer[EvaluationRuntimeBoundary]
    controlled_target: str
    independent_observer: str
    def __init__(self, deployment_id: _Optional[str] = ..., topology_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., runtime_boundaries: _Optional[_Iterable[_Union[EvaluationRuntimeBoundary, _Mapping]]] = ..., controlled_target: _Optional[str] = ..., independent_observer: _Optional[str] = ...) -> None: ...

class EvaluationValue(_message.Message):
    __slots__ = ("boolean_value", "integer_value", "string_value", "artifact_reference")
    BOOLEAN_VALUE_FIELD_NUMBER: _ClassVar[int]
    INTEGER_VALUE_FIELD_NUMBER: _ClassVar[int]
    STRING_VALUE_FIELD_NUMBER: _ClassVar[int]
    ARTIFACT_REFERENCE_FIELD_NUMBER: _ClassVar[int]
    boolean_value: bool
    integer_value: int
    string_value: str
    artifact_reference: _compliance_pb2.ComplianceEvidenceReference
    def __init__(self, boolean_value: _Optional[bool] = ..., integer_value: _Optional[int] = ..., string_value: _Optional[str] = ..., artifact_reference: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ...) -> None: ...

class EvaluationRun(_message.Message):
    __slots__ = ("schema_version", "run_id", "suite_ref", "deployment", "active_posture", "lane", "target_operator_id", "target_operator_session_id", "started_at", "completed_at", "attempt_refs", "final_verification_report_ref", "campaign_binding")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SUITE_REF_FIELD_NUMBER: _ClassVar[int]
    DEPLOYMENT_FIELD_NUMBER: _ClassVar[int]
    ACTIVE_POSTURE_FIELD_NUMBER: _ClassVar[int]
    LANE_FIELD_NUMBER: _ClassVar[int]
    TARGET_OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    TARGET_OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    STARTED_AT_FIELD_NUMBER: _ClassVar[int]
    COMPLETED_AT_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_REFS_FIELD_NUMBER: _ClassVar[int]
    FINAL_VERIFICATION_REPORT_REF_FIELD_NUMBER: _ClassVar[int]
    CAMPAIGN_BINDING_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    run_id: str
    suite_ref: _compliance_pb2.VersionedReference
    deployment: EvaluationDeploymentIdentity
    active_posture: EvaluationGovernancePosture
    lane: EvaluationLane
    target_operator_id: str
    target_operator_session_id: str
    started_at: _timestamp_pb2.Timestamp
    completed_at: _timestamp_pb2.Timestamp
    attempt_refs: _containers.RepeatedScalarFieldContainer[str]
    final_verification_report_ref: _compliance_pb2.ComplianceEvidenceReference
    campaign_binding: ModelCampaignBinding
    def __init__(self, schema_version: _Optional[str] = ..., run_id: _Optional[str] = ..., suite_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., deployment: _Optional[_Union[EvaluationDeploymentIdentity, _Mapping]] = ..., active_posture: _Optional[_Union[EvaluationGovernancePosture, str]] = ..., lane: _Optional[_Union[EvaluationLane, str]] = ..., target_operator_id: _Optional[str] = ..., target_operator_session_id: _Optional[str] = ..., started_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., completed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., attempt_refs: _Optional[_Iterable[str]] = ..., final_verification_report_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ..., campaign_binding: _Optional[_Union[ModelCampaignBinding, _Mapping]] = ...) -> None: ...

class EvaluationAttempt(_message.Message):
    __slots__ = ("attempt_id", "run_id", "scenario_ref", "status", "started_at", "completed_at", "transaction_id", "execution_id", "observation_refs", "assertion_refs", "verdict_refs", "failure_detail")
    ATTEMPT_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_REF_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    STARTED_AT_FIELD_NUMBER: _ClassVar[int]
    COMPLETED_AT_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_ID_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    OBSERVATION_REFS_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_REFS_FIELD_NUMBER: _ClassVar[int]
    VERDICT_REFS_FIELD_NUMBER: _ClassVar[int]
    FAILURE_DETAIL_FIELD_NUMBER: _ClassVar[int]
    attempt_id: str
    run_id: str
    scenario_ref: _compliance_pb2.VersionedReference
    status: EvaluationAttemptStatus
    started_at: _timestamp_pb2.Timestamp
    completed_at: _timestamp_pb2.Timestamp
    transaction_id: str
    execution_id: str
    observation_refs: _containers.RepeatedScalarFieldContainer[str]
    assertion_refs: _containers.RepeatedScalarFieldContainer[str]
    verdict_refs: _containers.RepeatedScalarFieldContainer[str]
    failure_detail: str
    def __init__(self, attempt_id: _Optional[str] = ..., run_id: _Optional[str] = ..., scenario_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., status: _Optional[_Union[EvaluationAttemptStatus, str]] = ..., started_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., completed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., transaction_id: _Optional[str] = ..., execution_id: _Optional[str] = ..., observation_refs: _Optional[_Iterable[str]] = ..., assertion_refs: _Optional[_Iterable[str]] = ..., verdict_refs: _Optional[_Iterable[str]] = ..., failure_detail: _Optional[str] = ...) -> None: ...

class EvaluationObservation(_message.Message):
    __slots__ = ("observation_id", "observation_type", "source", "observed_at", "run_id", "scenario_id", "attempt_id", "authority", "value", "evidence_refs")
    OBSERVATION_ID_FIELD_NUMBER: _ClassVar[int]
    OBSERVATION_TYPE_FIELD_NUMBER: _ClassVar[int]
    SOURCE_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_AT_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_ID_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_ID_FIELD_NUMBER: _ClassVar[int]
    AUTHORITY_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_REFS_FIELD_NUMBER: _ClassVar[int]
    observation_id: str
    observation_type: _compliance_pb2.VersionedReference
    source: EvaluationObservationSource
    observed_at: _timestamp_pb2.Timestamp
    run_id: str
    scenario_id: str
    attempt_id: str
    authority: EvaluationEvidenceAuthority
    value: EvaluationValue
    evidence_refs: _containers.RepeatedCompositeFieldContainer[_compliance_pb2.ComplianceEvidenceReference]
    def __init__(self, observation_id: _Optional[str] = ..., observation_type: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., source: _Optional[_Union[EvaluationObservationSource, str]] = ..., observed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., run_id: _Optional[str] = ..., scenario_id: _Optional[str] = ..., attempt_id: _Optional[str] = ..., authority: _Optional[_Union[EvaluationEvidenceAuthority, str]] = ..., value: _Optional[_Union[EvaluationValue, _Mapping]] = ..., evidence_refs: _Optional[_Iterable[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]]] = ...) -> None: ...

class EvaluationTargetState(_message.Message):
    __slots__ = ("schema_version", "run_id", "scenario_id", "attempt_id", "target_resource", "observed_at", "present", "content")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_ID_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_ID_FIELD_NUMBER: _ClassVar[int]
    TARGET_RESOURCE_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_AT_FIELD_NUMBER: _ClassVar[int]
    PRESENT_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    run_id: str
    scenario_id: str
    attempt_id: str
    target_resource: str
    observed_at: _timestamp_pb2.Timestamp
    present: bool
    content: bytes
    def __init__(self, schema_version: _Optional[str] = ..., run_id: _Optional[str] = ..., scenario_id: _Optional[str] = ..., attempt_id: _Optional[str] = ..., target_resource: _Optional[str] = ..., observed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., present: _Optional[bool] = ..., content: _Optional[bytes] = ...) -> None: ...

class EvaluationAssertion(_message.Message):
    __slots__ = ("assertion_id", "assertion_version", "comparator", "expected", "required_observation_types", "required_authorities")
    ASSERTION_ID_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_VERSION_FIELD_NUMBER: _ClassVar[int]
    COMPARATOR_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_OBSERVATION_TYPES_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_AUTHORITIES_FIELD_NUMBER: _ClassVar[int]
    assertion_id: str
    assertion_version: str
    comparator: EvaluationComparator
    expected: EvaluationValue
    required_observation_types: _containers.RepeatedCompositeFieldContainer[_compliance_pb2.VersionedReference]
    required_authorities: _containers.RepeatedScalarFieldContainer[EvaluationEvidenceAuthority]
    def __init__(self, assertion_id: _Optional[str] = ..., assertion_version: _Optional[str] = ..., comparator: _Optional[_Union[EvaluationComparator, str]] = ..., expected: _Optional[_Union[EvaluationValue, _Mapping]] = ..., required_observation_types: _Optional[_Iterable[_Union[_compliance_pb2.VersionedReference, _Mapping]]] = ..., required_authorities: _Optional[_Iterable[_Union[EvaluationEvidenceAuthority, str]]] = ...) -> None: ...

class EvaluationVerdict(_message.Message):
    __slots__ = ("verdict_id", "assertion_ref", "status", "observed_refs", "evidence_refs", "grader_ref", "evaluated_at", "failure_reason")
    VERDICT_ID_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_REF_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_REFS_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_REFS_FIELD_NUMBER: _ClassVar[int]
    GRADER_REF_FIELD_NUMBER: _ClassVar[int]
    EVALUATED_AT_FIELD_NUMBER: _ClassVar[int]
    FAILURE_REASON_FIELD_NUMBER: _ClassVar[int]
    verdict_id: str
    assertion_ref: _compliance_pb2.VersionedReference
    status: EvaluationVerdictStatus
    observed_refs: _containers.RepeatedScalarFieldContainer[str]
    evidence_refs: _containers.RepeatedCompositeFieldContainer[_compliance_pb2.ComplianceEvidenceReference]
    grader_ref: _compliance_pb2.VersionedReference
    evaluated_at: _timestamp_pb2.Timestamp
    failure_reason: str
    def __init__(self, verdict_id: _Optional[str] = ..., assertion_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., status: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., observed_refs: _Optional[_Iterable[str]] = ..., evidence_refs: _Optional[_Iterable[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]]] = ..., grader_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., evaluated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., failure_reason: _Optional[str] = ...) -> None: ...

class EvaluationMetric(_message.Message):
    __slots__ = ("metric_id", "metric_version", "numerator", "denominator", "value", "unit", "direction", "eligible_population_ref", "missing_data_policy", "source_verdict_refs", "evidence_refs")
    METRIC_ID_FIELD_NUMBER: _ClassVar[int]
    METRIC_VERSION_FIELD_NUMBER: _ClassVar[int]
    NUMERATOR_FIELD_NUMBER: _ClassVar[int]
    DENOMINATOR_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    UNIT_FIELD_NUMBER: _ClassVar[int]
    DIRECTION_FIELD_NUMBER: _ClassVar[int]
    ELIGIBLE_POPULATION_REF_FIELD_NUMBER: _ClassVar[int]
    MISSING_DATA_POLICY_FIELD_NUMBER: _ClassVar[int]
    SOURCE_VERDICT_REFS_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_REFS_FIELD_NUMBER: _ClassVar[int]
    metric_id: str
    metric_version: str
    numerator: int
    denominator: int
    value: float
    unit: EvaluationMetricUnit
    direction: EvaluationMetricDirection
    eligible_population_ref: _compliance_pb2.VersionedReference
    missing_data_policy: EvaluationMissingDataPolicy
    source_verdict_refs: _containers.RepeatedScalarFieldContainer[str]
    evidence_refs: _containers.RepeatedCompositeFieldContainer[_compliance_pb2.ComplianceEvidenceReference]
    def __init__(self, metric_id: _Optional[str] = ..., metric_version: _Optional[str] = ..., numerator: _Optional[int] = ..., denominator: _Optional[int] = ..., value: _Optional[float] = ..., unit: _Optional[_Union[EvaluationMetricUnit, str]] = ..., direction: _Optional[_Union[EvaluationMetricDirection, str]] = ..., eligible_population_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., missing_data_policy: _Optional[_Union[EvaluationMissingDataPolicy, str]] = ..., source_verdict_refs: _Optional[_Iterable[str]] = ..., evidence_refs: _Optional[_Iterable[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]]] = ...) -> None: ...

class EvaluationReport(_message.Message):
    __slots__ = ("schema_version", "run", "attempts", "observations", "assertions", "verdicts", "metrics", "evidence_refs", "summary_status", "required_verdict_count", "passed_verdict_count", "summary", "assignment_results", "campaign_verification_report")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    RUN_FIELD_NUMBER: _ClassVar[int]
    ATTEMPTS_FIELD_NUMBER: _ClassVar[int]
    OBSERVATIONS_FIELD_NUMBER: _ClassVar[int]
    ASSERTIONS_FIELD_NUMBER: _ClassVar[int]
    VERDICTS_FIELD_NUMBER: _ClassVar[int]
    METRICS_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_REFS_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_STATUS_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_VERDICT_COUNT_FIELD_NUMBER: _ClassVar[int]
    PASSED_VERDICT_COUNT_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_RESULTS_FIELD_NUMBER: _ClassVar[int]
    CAMPAIGN_VERIFICATION_REPORT_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    run: EvaluationRun
    attempts: _containers.RepeatedCompositeFieldContainer[EvaluationAttempt]
    observations: _containers.RepeatedCompositeFieldContainer[EvaluationObservation]
    assertions: _containers.RepeatedCompositeFieldContainer[EvaluationAssertion]
    verdicts: _containers.RepeatedCompositeFieldContainer[EvaluationVerdict]
    metrics: _containers.RepeatedCompositeFieldContainer[EvaluationMetric]
    evidence_refs: _containers.RepeatedCompositeFieldContainer[_compliance_pb2.ComplianceEvidenceReference]
    summary_status: EvaluationVerdictStatus
    required_verdict_count: int
    passed_verdict_count: int
    summary: str
    assignment_results: _containers.RepeatedCompositeFieldContainer[EvaluationAssignmentResult]
    campaign_verification_report: EvaluationVerificationReport
    def __init__(self, schema_version: _Optional[str] = ..., run: _Optional[_Union[EvaluationRun, _Mapping]] = ..., attempts: _Optional[_Iterable[_Union[EvaluationAttempt, _Mapping]]] = ..., observations: _Optional[_Iterable[_Union[EvaluationObservation, _Mapping]]] = ..., assertions: _Optional[_Iterable[_Union[EvaluationAssertion, _Mapping]]] = ..., verdicts: _Optional[_Iterable[_Union[EvaluationVerdict, _Mapping]]] = ..., metrics: _Optional[_Iterable[_Union[EvaluationMetric, _Mapping]]] = ..., evidence_refs: _Optional[_Iterable[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]]] = ..., summary_status: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., required_verdict_count: _Optional[int] = ..., passed_verdict_count: _Optional[int] = ..., summary: _Optional[str] = ..., assignment_results: _Optional[_Iterable[_Union[EvaluationAssignmentResult, _Mapping]]] = ..., campaign_verification_report: _Optional[_Union[EvaluationVerificationReport, _Mapping]] = ...) -> None: ...

class ModelCampaignBinding(_message.Message):
    __slots__ = ("campaign_id", "campaign_digest", "catalog_ref", "catalog_digest", "model_registry_digest", "inference_operator_session_id", "data_operator_session_id")
    CAMPAIGN_ID_FIELD_NUMBER: _ClassVar[int]
    CAMPAIGN_DIGEST_FIELD_NUMBER: _ClassVar[int]
    CATALOG_REF_FIELD_NUMBER: _ClassVar[int]
    CATALOG_DIGEST_FIELD_NUMBER: _ClassVar[int]
    MODEL_REGISTRY_DIGEST_FIELD_NUMBER: _ClassVar[int]
    INFERENCE_OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    DATA_OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    campaign_id: str
    campaign_digest: str
    catalog_ref: _compliance_pb2.VersionedReference
    catalog_digest: str
    model_registry_digest: str
    inference_operator_session_id: str
    data_operator_session_id: str
    def __init__(self, campaign_id: _Optional[str] = ..., campaign_digest: _Optional[str] = ..., catalog_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., catalog_digest: _Optional[str] = ..., model_registry_digest: _Optional[str] = ..., inference_operator_session_id: _Optional[str] = ..., data_operator_session_id: _Optional[str] = ...) -> None: ...

class EvaluationCampaignSpec(_message.Message):
    __slots__ = ("schema_version", "campaign_id", "catalog_ref", "catalog_digest", "model_registry", "model_registry_digest", "campaign_digest", "governance_posture", "scenario_count", "repetition_count")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    CAMPAIGN_ID_FIELD_NUMBER: _ClassVar[int]
    CATALOG_REF_FIELD_NUMBER: _ClassVar[int]
    CATALOG_DIGEST_FIELD_NUMBER: _ClassVar[int]
    MODEL_REGISTRY_FIELD_NUMBER: _ClassVar[int]
    MODEL_REGISTRY_DIGEST_FIELD_NUMBER: _ClassVar[int]
    CAMPAIGN_DIGEST_FIELD_NUMBER: _ClassVar[int]
    GOVERNANCE_POSTURE_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_COUNT_FIELD_NUMBER: _ClassVar[int]
    REPETITION_COUNT_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    campaign_id: str
    catalog_ref: _compliance_pb2.VersionedReference
    catalog_digest: str
    model_registry: _containers.RepeatedCompositeFieldContainer[ModelVariant]
    model_registry_digest: str
    campaign_digest: str
    governance_posture: EvaluationGovernancePosture
    scenario_count: int
    repetition_count: int
    def __init__(self, schema_version: _Optional[str] = ..., campaign_id: _Optional[str] = ..., catalog_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., catalog_digest: _Optional[str] = ..., model_registry: _Optional[_Iterable[_Union[ModelVariant, _Mapping]]] = ..., model_registry_digest: _Optional[str] = ..., campaign_digest: _Optional[str] = ..., governance_posture: _Optional[_Union[EvaluationGovernancePosture, str]] = ..., scenario_count: _Optional[int] = ..., repetition_count: _Optional[int] = ...) -> None: ...

class EvaluationScenarioCatalog(_message.Message):
    __slots__ = ("schema_version", "catalog_ref", "catalog_digest", "scenarios")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    CATALOG_REF_FIELD_NUMBER: _ClassVar[int]
    CATALOG_DIGEST_FIELD_NUMBER: _ClassVar[int]
    SCENARIOS_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    catalog_ref: _compliance_pb2.VersionedReference
    catalog_digest: str
    scenarios: _containers.RepeatedCompositeFieldContainer[EvaluationScenarioDefinition]
    def __init__(self, schema_version: _Optional[str] = ..., catalog_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., catalog_digest: _Optional[str] = ..., scenarios: _Optional[_Iterable[_Union[EvaluationScenarioDefinition, _Mapping]]] = ...) -> None: ...

class EvaluationScenarioDefinition(_message.Message):
    __slots__ = ("scenario_id", "scenario_version", "category", "public_description", "grading_method", "allowed_tools", "expected_tools", "forbidden_tools", "input_fixture_ref", "required_concepts", "gold_criteria_ref")
    SCENARIO_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_VERSION_FIELD_NUMBER: _ClassVar[int]
    CATEGORY_FIELD_NUMBER: _ClassVar[int]
    PUBLIC_DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    GRADING_METHOD_FIELD_NUMBER: _ClassVar[int]
    ALLOWED_TOOLS_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_TOOLS_FIELD_NUMBER: _ClassVar[int]
    FORBIDDEN_TOOLS_FIELD_NUMBER: _ClassVar[int]
    INPUT_FIXTURE_REF_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_CONCEPTS_FIELD_NUMBER: _ClassVar[int]
    GOLD_CRITERIA_REF_FIELD_NUMBER: _ClassVar[int]
    scenario_id: str
    scenario_version: str
    category: EvaluationScenarioCategory
    public_description: str
    grading_method: EvaluationGradingMethod
    allowed_tools: _containers.RepeatedScalarFieldContainer[str]
    expected_tools: _containers.RepeatedScalarFieldContainer[str]
    forbidden_tools: _containers.RepeatedScalarFieldContainer[str]
    input_fixture_ref: _compliance_pb2.ComplianceEvidenceReference
    required_concepts: _containers.RepeatedScalarFieldContainer[str]
    gold_criteria_ref: _compliance_pb2.ComplianceEvidenceReference
    def __init__(self, scenario_id: _Optional[str] = ..., scenario_version: _Optional[str] = ..., category: _Optional[_Union[EvaluationScenarioCategory, str]] = ..., public_description: _Optional[str] = ..., grading_method: _Optional[_Union[EvaluationGradingMethod, str]] = ..., allowed_tools: _Optional[_Iterable[str]] = ..., expected_tools: _Optional[_Iterable[str]] = ..., forbidden_tools: _Optional[_Iterable[str]] = ..., input_fixture_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ..., required_concepts: _Optional[_Iterable[str]] = ..., gold_criteria_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ...) -> None: ...

class ModelVariant(_message.Message):
    __slots__ = ("variant_id", "provider_class", "served_model_tag", "model_digest", "model_family", "parameter_count", "quantization", "context_limit", "capability_observations")
    VARIANT_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_CLASS_FIELD_NUMBER: _ClassVar[int]
    SERVED_MODEL_TAG_FIELD_NUMBER: _ClassVar[int]
    MODEL_DIGEST_FIELD_NUMBER: _ClassVar[int]
    MODEL_FAMILY_FIELD_NUMBER: _ClassVar[int]
    PARAMETER_COUNT_FIELD_NUMBER: _ClassVar[int]
    QUANTIZATION_FIELD_NUMBER: _ClassVar[int]
    CONTEXT_LIMIT_FIELD_NUMBER: _ClassVar[int]
    CAPABILITY_OBSERVATIONS_FIELD_NUMBER: _ClassVar[int]
    variant_id: str
    provider_class: str
    served_model_tag: str
    model_digest: str
    model_family: str
    parameter_count: int
    quantization: str
    context_limit: int
    capability_observations: _containers.RepeatedCompositeFieldContainer[ModelCapabilityObservation]
    def __init__(self, variant_id: _Optional[str] = ..., provider_class: _Optional[str] = ..., served_model_tag: _Optional[str] = ..., model_digest: _Optional[str] = ..., model_family: _Optional[str] = ..., parameter_count: _Optional[int] = ..., quantization: _Optional[str] = ..., context_limit: _Optional[int] = ..., capability_observations: _Optional[_Iterable[_Union[ModelCapabilityObservation, _Mapping]]] = ...) -> None: ...

class ModelCapabilityObservation(_message.Message):
    __slots__ = ("capability", "outcome", "observation_detail")
    CAPABILITY_FIELD_NUMBER: _ClassVar[int]
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    OBSERVATION_DETAIL_FIELD_NUMBER: _ClassVar[int]
    capability: ModelCapabilityKind
    outcome: EvaluationVerdictStatus
    observation_detail: str
    def __init__(self, capability: _Optional[_Union[ModelCapabilityKind, str]] = ..., outcome: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., observation_detail: _Optional[str] = ...) -> None: ...

class RoleAssignment(_message.Message):
    __slots__ = ("designated_role", "variant_id")
    DESIGNATED_ROLE_FIELD_NUMBER: _ClassVar[int]
    VARIANT_ID_FIELD_NUMBER: _ClassVar[int]
    designated_role: ModelCampaignRole
    variant_id: str
    def __init__(self, designated_role: _Optional[_Union[ModelCampaignRole, str]] = ..., variant_id: _Optional[str] = ...) -> None: ...

class HeterogeneousStackDefinition(_message.Message):
    __slots__ = ("stack_id", "stack_digest", "primary_slot", "assistant_slot", "lite_slot")
    STACK_ID_FIELD_NUMBER: _ClassVar[int]
    STACK_DIGEST_FIELD_NUMBER: _ClassVar[int]
    PRIMARY_SLOT_FIELD_NUMBER: _ClassVar[int]
    ASSISTANT_SLOT_FIELD_NUMBER: _ClassVar[int]
    LITE_SLOT_FIELD_NUMBER: _ClassVar[int]
    stack_id: str
    stack_digest: str
    primary_slot: RoleAssignment
    assistant_slot: RoleAssignment
    lite_slot: RoleAssignment
    def __init__(self, stack_id: _Optional[str] = ..., stack_digest: _Optional[str] = ..., primary_slot: _Optional[_Union[RoleAssignment, _Mapping]] = ..., assistant_slot: _Optional[_Union[RoleAssignment, _Mapping]] = ..., lite_slot: _Optional[_Union[RoleAssignment, _Mapping]] = ...) -> None: ...

class EvaluationAssignment(_message.Message):
    __slots__ = ("schema_version", "assignment_id", "deterministic_identity", "campaign_id", "run_id", "scenario_ref", "scenario_id", "lane", "lifecycle_status", "repetition", "queued_at", "started_at", "completed_at", "homogeneous", "heterogeneous")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    DETERMINISTIC_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    CAMPAIGN_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_REF_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_ID_FIELD_NUMBER: _ClassVar[int]
    LANE_FIELD_NUMBER: _ClassVar[int]
    LIFECYCLE_STATUS_FIELD_NUMBER: _ClassVar[int]
    REPETITION_FIELD_NUMBER: _ClassVar[int]
    QUEUED_AT_FIELD_NUMBER: _ClassVar[int]
    STARTED_AT_FIELD_NUMBER: _ClassVar[int]
    COMPLETED_AT_FIELD_NUMBER: _ClassVar[int]
    HOMOGENEOUS_FIELD_NUMBER: _ClassVar[int]
    HETEROGENEOUS_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    assignment_id: str
    deterministic_identity: str
    campaign_id: str
    run_id: str
    scenario_ref: _compliance_pb2.VersionedReference
    scenario_id: str
    lane: EvaluationLane
    lifecycle_status: EvaluationAssignmentLifecycleStatus
    repetition: int
    queued_at: _timestamp_pb2.Timestamp
    started_at: _timestamp_pb2.Timestamp
    completed_at: _timestamp_pb2.Timestamp
    homogeneous: HomogeneousAssignmentTarget
    heterogeneous: HeterogeneousAssignmentTarget
    def __init__(self, schema_version: _Optional[str] = ..., assignment_id: _Optional[str] = ..., deterministic_identity: _Optional[str] = ..., campaign_id: _Optional[str] = ..., run_id: _Optional[str] = ..., scenario_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., scenario_id: _Optional[str] = ..., lane: _Optional[_Union[EvaluationLane, str]] = ..., lifecycle_status: _Optional[_Union[EvaluationAssignmentLifecycleStatus, str]] = ..., repetition: _Optional[int] = ..., queued_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., started_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., completed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., homogeneous: _Optional[_Union[HomogeneousAssignmentTarget, _Mapping]] = ..., heterogeneous: _Optional[_Union[HeterogeneousAssignmentTarget, _Mapping]] = ...) -> None: ...

class HomogeneousAssignmentTarget(_message.Message):
    __slots__ = ("candidate_variant", "designated_role")
    CANDIDATE_VARIANT_FIELD_NUMBER: _ClassVar[int]
    DESIGNATED_ROLE_FIELD_NUMBER: _ClassVar[int]
    candidate_variant: ModelVariant
    designated_role: ModelCampaignRole
    def __init__(self, candidate_variant: _Optional[_Union[ModelVariant, _Mapping]] = ..., designated_role: _Optional[_Union[ModelCampaignRole, str]] = ...) -> None: ...

class HeterogeneousAssignmentTarget(_message.Message):
    __slots__ = ("stack",)
    STACK_FIELD_NUMBER: _ClassVar[int]
    stack: HeterogeneousStackDefinition
    def __init__(self, stack: _Optional[_Union[HeterogeneousStackDefinition, _Mapping]] = ...) -> None: ...

class ModelInferenceRecord(_message.Message):
    __slots__ = ("inference_record_id", "provider_attempt_id", "assignment_id", "evaluation_attempt_id", "model_role", "agent_persona", "call_site", "model_variant", "temperature", "top_p", "top_k", "seed", "max_output_tokens", "input_hash", "output_hash", "usage_availability", "prompt_tokens", "completion_tokens", "thinking_tokens", "cache_tokens", "request_started_at_unix_nanos", "first_token_at_unix_nanos", "generation_duration_nanos", "total_duration_nanos", "load_duration_nanos", "load_state", "retry_count", "finish_reason", "privacy_attested", "governed_receipt_ref", "result_digest")
    INFERENCE_RECORD_ID_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_ATTEMPT_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    EVALUATION_ATTEMPT_ID_FIELD_NUMBER: _ClassVar[int]
    MODEL_ROLE_FIELD_NUMBER: _ClassVar[int]
    AGENT_PERSONA_FIELD_NUMBER: _ClassVar[int]
    CALL_SITE_FIELD_NUMBER: _ClassVar[int]
    MODEL_VARIANT_FIELD_NUMBER: _ClassVar[int]
    TEMPERATURE_FIELD_NUMBER: _ClassVar[int]
    TOP_P_FIELD_NUMBER: _ClassVar[int]
    TOP_K_FIELD_NUMBER: _ClassVar[int]
    SEED_FIELD_NUMBER: _ClassVar[int]
    MAX_OUTPUT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    INPUT_HASH_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_HASH_FIELD_NUMBER: _ClassVar[int]
    USAGE_AVAILABILITY_FIELD_NUMBER: _ClassVar[int]
    PROMPT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    COMPLETION_TOKENS_FIELD_NUMBER: _ClassVar[int]
    THINKING_TOKENS_FIELD_NUMBER: _ClassVar[int]
    CACHE_TOKENS_FIELD_NUMBER: _ClassVar[int]
    REQUEST_STARTED_AT_UNIX_NANOS_FIELD_NUMBER: _ClassVar[int]
    FIRST_TOKEN_AT_UNIX_NANOS_FIELD_NUMBER: _ClassVar[int]
    GENERATION_DURATION_NANOS_FIELD_NUMBER: _ClassVar[int]
    TOTAL_DURATION_NANOS_FIELD_NUMBER: _ClassVar[int]
    LOAD_DURATION_NANOS_FIELD_NUMBER: _ClassVar[int]
    LOAD_STATE_FIELD_NUMBER: _ClassVar[int]
    RETRY_COUNT_FIELD_NUMBER: _ClassVar[int]
    FINISH_REASON_FIELD_NUMBER: _ClassVar[int]
    PRIVACY_ATTESTED_FIELD_NUMBER: _ClassVar[int]
    GOVERNED_RECEIPT_REF_FIELD_NUMBER: _ClassVar[int]
    RESULT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    inference_record_id: str
    provider_attempt_id: str
    assignment_id: str
    evaluation_attempt_id: str
    model_role: ModelCampaignRole
    agent_persona: str
    call_site: str
    model_variant: ModelVariant
    temperature: float
    top_p: float
    top_k: int
    seed: int
    max_output_tokens: int
    input_hash: str
    output_hash: str
    usage_availability: EvaluationUsageAvailability
    prompt_tokens: int
    completion_tokens: int
    thinking_tokens: int
    cache_tokens: int
    request_started_at_unix_nanos: int
    first_token_at_unix_nanos: int
    generation_duration_nanos: int
    total_duration_nanos: int
    load_duration_nanos: int
    load_state: EvaluationLoadState
    retry_count: int
    finish_reason: str
    privacy_attested: bool
    governed_receipt_ref: _compliance_pb2.ComplianceEvidenceReference
    result_digest: str
    def __init__(self, inference_record_id: _Optional[str] = ..., provider_attempt_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., evaluation_attempt_id: _Optional[str] = ..., model_role: _Optional[_Union[ModelCampaignRole, str]] = ..., agent_persona: _Optional[str] = ..., call_site: _Optional[str] = ..., model_variant: _Optional[_Union[ModelVariant, _Mapping]] = ..., temperature: _Optional[float] = ..., top_p: _Optional[float] = ..., top_k: _Optional[int] = ..., seed: _Optional[int] = ..., max_output_tokens: _Optional[int] = ..., input_hash: _Optional[str] = ..., output_hash: _Optional[str] = ..., usage_availability: _Optional[_Union[EvaluationUsageAvailability, str]] = ..., prompt_tokens: _Optional[int] = ..., completion_tokens: _Optional[int] = ..., thinking_tokens: _Optional[int] = ..., cache_tokens: _Optional[int] = ..., request_started_at_unix_nanos: _Optional[int] = ..., first_token_at_unix_nanos: _Optional[int] = ..., generation_duration_nanos: _Optional[int] = ..., total_duration_nanos: _Optional[int] = ..., load_duration_nanos: _Optional[int] = ..., load_state: _Optional[_Union[EvaluationLoadState, str]] = ..., retry_count: _Optional[int] = ..., finish_reason: _Optional[str] = ..., privacy_attested: _Optional[bool] = ..., governed_receipt_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ..., result_digest: _Optional[str] = ...) -> None: ...

class ToolDecisionRecord(_message.Message):
    __slots__ = ("decision_id", "assignment_id", "tool_name", "recognized", "selected", "permission_compliant", "unnecessary", "outcome")
    DECISION_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    TOOL_NAME_FIELD_NUMBER: _ClassVar[int]
    RECOGNIZED_FIELD_NUMBER: _ClassVar[int]
    SELECTED_FIELD_NUMBER: _ClassVar[int]
    PERMISSION_COMPLIANT_FIELD_NUMBER: _ClassVar[int]
    UNNECESSARY_FIELD_NUMBER: _ClassVar[int]
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    decision_id: str
    assignment_id: str
    tool_name: str
    recognized: bool
    selected: bool
    permission_compliant: bool
    unnecessary: bool
    outcome: EvaluationVerdictStatus
    def __init__(self, decision_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., tool_name: _Optional[str] = ..., recognized: _Optional[bool] = ..., selected: _Optional[bool] = ..., permission_compliant: _Optional[bool] = ..., unnecessary: _Optional[bool] = ..., outcome: _Optional[_Union[EvaluationVerdictStatus, str]] = ...) -> None: ...

class ToolCallRecord(_message.Message):
    __slots__ = ("call_id", "assignment_id", "tool_name", "arguments_hash", "schema_outcome", "semantic_outcome", "governed_binding_ref")
    CALL_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    TOOL_NAME_FIELD_NUMBER: _ClassVar[int]
    ARGUMENTS_HASH_FIELD_NUMBER: _ClassVar[int]
    SCHEMA_OUTCOME_FIELD_NUMBER: _ClassVar[int]
    SEMANTIC_OUTCOME_FIELD_NUMBER: _ClassVar[int]
    GOVERNED_BINDING_REF_FIELD_NUMBER: _ClassVar[int]
    call_id: str
    assignment_id: str
    tool_name: str
    arguments_hash: str
    schema_outcome: EvaluationVerdictStatus
    semantic_outcome: EvaluationVerdictStatus
    governed_binding_ref: _compliance_pb2.ComplianceEvidenceReference
    def __init__(self, call_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., tool_name: _Optional[str] = ..., arguments_hash: _Optional[str] = ..., schema_outcome: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., semantic_outcome: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., governed_binding_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ...) -> None: ...

class EscalationRecord(_message.Message):
    __slots__ = ("escalation_id", "assignment_id", "from_role", "to_role", "justified", "reason_code")
    ESCALATION_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    FROM_ROLE_FIELD_NUMBER: _ClassVar[int]
    TO_ROLE_FIELD_NUMBER: _ClassVar[int]
    JUSTIFIED_FIELD_NUMBER: _ClassVar[int]
    REASON_CODE_FIELD_NUMBER: _ClassVar[int]
    escalation_id: str
    assignment_id: str
    from_role: ModelCampaignRole
    to_role: ModelCampaignRole
    justified: bool
    reason_code: str
    def __init__(self, escalation_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., from_role: _Optional[_Union[ModelCampaignRole, str]] = ..., to_role: _Optional[_Union[ModelCampaignRole, str]] = ..., justified: _Optional[bool] = ..., reason_code: _Optional[str] = ...) -> None: ...

class HandoffRecord(_message.Message):
    __slots__ = ("handoff_id", "assignment_id", "from_role", "to_role", "reason_code")
    HANDOFF_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    FROM_ROLE_FIELD_NUMBER: _ClassVar[int]
    TO_ROLE_FIELD_NUMBER: _ClassVar[int]
    REASON_CODE_FIELD_NUMBER: _ClassVar[int]
    handoff_id: str
    assignment_id: str
    from_role: ModelCampaignRole
    to_role: ModelCampaignRole
    reason_code: str
    def __init__(self, handoff_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., from_role: _Optional[_Union[ModelCampaignRole, str]] = ..., to_role: _Optional[_Union[ModelCampaignRole, str]] = ..., reason_code: _Optional[str] = ...) -> None: ...

class RecoveryRecord(_message.Message):
    __slots__ = ("recovery_id", "assignment_id", "recovery_kind", "outcome", "detail")
    RECOVERY_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    RECOVERY_KIND_FIELD_NUMBER: _ClassVar[int]
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    recovery_id: str
    assignment_id: str
    recovery_kind: str
    outcome: EvaluationVerdictStatus
    detail: str
    def __init__(self, recovery_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., recovery_kind: _Optional[str] = ..., outcome: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., detail: _Optional[str] = ...) -> None: ...

class GovernedActionBinding(_message.Message):
    __slots__ = ("binding_id", "assignment_id", "transaction_id", "operator_id", "operator_session_id", "receipt_ref", "persistence_attestation_ref", "policy_decision", "effect_observation_ref")
    BINDING_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    RECEIPT_REF_FIELD_NUMBER: _ClassVar[int]
    PERSISTENCE_ATTESTATION_REF_FIELD_NUMBER: _ClassVar[int]
    POLICY_DECISION_FIELD_NUMBER: _ClassVar[int]
    EFFECT_OBSERVATION_REF_FIELD_NUMBER: _ClassVar[int]
    binding_id: str
    assignment_id: str
    transaction_id: str
    operator_id: str
    operator_session_id: str
    receipt_ref: _compliance_pb2.ComplianceEvidenceReference
    persistence_attestation_ref: _compliance_pb2.ComplianceEvidenceReference
    policy_decision: str
    effect_observation_ref: _compliance_pb2.ComplianceEvidenceReference
    def __init__(self, binding_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., transaction_id: _Optional[str] = ..., operator_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., receipt_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ..., persistence_attestation_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ..., policy_decision: _Optional[str] = ..., effect_observation_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ...) -> None: ...

class DeterministicGrade(_message.Message):
    __slots__ = ("grade_id", "criterion_id", "status", "score", "detail")
    GRADE_ID_FIELD_NUMBER: _ClassVar[int]
    CRITERION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    SCORE_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    grade_id: str
    criterion_id: str
    status: EvaluationVerdictStatus
    score: float
    detail: str
    def __init__(self, grade_id: _Optional[str] = ..., criterion_id: _Optional[str] = ..., status: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., score: _Optional[float] = ..., detail: _Optional[str] = ...) -> None: ...

class SemanticGrade(_message.Message):
    __slots__ = ("grade_id", "criterion_id", "status", "judge_variant_id", "grader_call_ref", "detail")
    GRADE_ID_FIELD_NUMBER: _ClassVar[int]
    CRITERION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    JUDGE_VARIANT_ID_FIELD_NUMBER: _ClassVar[int]
    GRADER_CALL_REF_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    grade_id: str
    criterion_id: str
    status: EvaluationVerdictStatus
    judge_variant_id: str
    grader_call_ref: _compliance_pb2.ComplianceEvidenceReference
    detail: str
    def __init__(self, grade_id: _Optional[str] = ..., criterion_id: _Optional[str] = ..., status: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., judge_variant_id: _Optional[str] = ..., grader_call_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ..., detail: _Optional[str] = ...) -> None: ...

class DecomposedScoreRecord(_message.Message):
    __slots__ = ("score_id", "dimension", "value", "unit", "direction", "missing_data_policy")
    SCORE_ID_FIELD_NUMBER: _ClassVar[int]
    DIMENSION_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    UNIT_FIELD_NUMBER: _ClassVar[int]
    DIRECTION_FIELD_NUMBER: _ClassVar[int]
    MISSING_DATA_POLICY_FIELD_NUMBER: _ClassVar[int]
    score_id: str
    dimension: str
    value: float
    unit: EvaluationMetricUnit
    direction: EvaluationMetricDirection
    missing_data_policy: EvaluationMissingDataPolicy
    def __init__(self, score_id: _Optional[str] = ..., dimension: _Optional[str] = ..., value: _Optional[float] = ..., unit: _Optional[_Union[EvaluationMetricUnit, str]] = ..., direction: _Optional[_Union[EvaluationMetricDirection, str]] = ..., missing_data_policy: _Optional[_Union[EvaluationMissingDataPolicy, str]] = ...) -> None: ...

class GraderModelCallRecord(_message.Message):
    __slots__ = ("grader_call_id", "assignment_id", "judge_variant_id", "inference_record_ref")
    GRADER_CALL_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    JUDGE_VARIANT_ID_FIELD_NUMBER: _ClassVar[int]
    INFERENCE_RECORD_REF_FIELD_NUMBER: _ClassVar[int]
    grader_call_id: str
    assignment_id: str
    judge_variant_id: str
    inference_record_ref: _compliance_pb2.ComplianceEvidenceReference
    def __init__(self, grader_call_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., judge_variant_id: _Optional[str] = ..., inference_record_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ...) -> None: ...

class EvaluationAssignmentResult(_message.Message):
    __slots__ = ("schema_version", "assignment_id", "run_id", "campaign_id", "lane", "lifecycle_status", "result_digest", "model_inferences", "tool_decisions", "tool_calls", "escalations", "handoffs", "recoveries", "governed_actions", "deterministic_grades", "semantic_grades", "decomposed_scores", "grader_calls", "evidence_refs", "completed_at")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    CAMPAIGN_ID_FIELD_NUMBER: _ClassVar[int]
    LANE_FIELD_NUMBER: _ClassVar[int]
    LIFECYCLE_STATUS_FIELD_NUMBER: _ClassVar[int]
    RESULT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    MODEL_INFERENCES_FIELD_NUMBER: _ClassVar[int]
    TOOL_DECISIONS_FIELD_NUMBER: _ClassVar[int]
    TOOL_CALLS_FIELD_NUMBER: _ClassVar[int]
    ESCALATIONS_FIELD_NUMBER: _ClassVar[int]
    HANDOFFS_FIELD_NUMBER: _ClassVar[int]
    RECOVERIES_FIELD_NUMBER: _ClassVar[int]
    GOVERNED_ACTIONS_FIELD_NUMBER: _ClassVar[int]
    DETERMINISTIC_GRADES_FIELD_NUMBER: _ClassVar[int]
    SEMANTIC_GRADES_FIELD_NUMBER: _ClassVar[int]
    DECOMPOSED_SCORES_FIELD_NUMBER: _ClassVar[int]
    GRADER_CALLS_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_REFS_FIELD_NUMBER: _ClassVar[int]
    COMPLETED_AT_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    assignment_id: str
    run_id: str
    campaign_id: str
    lane: EvaluationLane
    lifecycle_status: EvaluationAssignmentLifecycleStatus
    result_digest: str
    model_inferences: _containers.RepeatedCompositeFieldContainer[ModelInferenceRecord]
    tool_decisions: _containers.RepeatedCompositeFieldContainer[ToolDecisionRecord]
    tool_calls: _containers.RepeatedCompositeFieldContainer[ToolCallRecord]
    escalations: _containers.RepeatedCompositeFieldContainer[EscalationRecord]
    handoffs: _containers.RepeatedCompositeFieldContainer[HandoffRecord]
    recoveries: _containers.RepeatedCompositeFieldContainer[RecoveryRecord]
    governed_actions: _containers.RepeatedCompositeFieldContainer[GovernedActionBinding]
    deterministic_grades: _containers.RepeatedCompositeFieldContainer[DeterministicGrade]
    semantic_grades: _containers.RepeatedCompositeFieldContainer[SemanticGrade]
    decomposed_scores: _containers.RepeatedCompositeFieldContainer[DecomposedScoreRecord]
    grader_calls: _containers.RepeatedCompositeFieldContainer[GraderModelCallRecord]
    evidence_refs: _containers.RepeatedCompositeFieldContainer[_compliance_pb2.ComplianceEvidenceReference]
    completed_at: _timestamp_pb2.Timestamp
    def __init__(self, schema_version: _Optional[str] = ..., assignment_id: _Optional[str] = ..., run_id: _Optional[str] = ..., campaign_id: _Optional[str] = ..., lane: _Optional[_Union[EvaluationLane, str]] = ..., lifecycle_status: _Optional[_Union[EvaluationAssignmentLifecycleStatus, str]] = ..., result_digest: _Optional[str] = ..., model_inferences: _Optional[_Iterable[_Union[ModelInferenceRecord, _Mapping]]] = ..., tool_decisions: _Optional[_Iterable[_Union[ToolDecisionRecord, _Mapping]]] = ..., tool_calls: _Optional[_Iterable[_Union[ToolCallRecord, _Mapping]]] = ..., escalations: _Optional[_Iterable[_Union[EscalationRecord, _Mapping]]] = ..., handoffs: _Optional[_Iterable[_Union[HandoffRecord, _Mapping]]] = ..., recoveries: _Optional[_Iterable[_Union[RecoveryRecord, _Mapping]]] = ..., governed_actions: _Optional[_Iterable[_Union[GovernedActionBinding, _Mapping]]] = ..., deterministic_grades: _Optional[_Iterable[_Union[DeterministicGrade, _Mapping]]] = ..., semantic_grades: _Optional[_Iterable[_Union[SemanticGrade, _Mapping]]] = ..., decomposed_scores: _Optional[_Iterable[_Union[DecomposedScoreRecord, _Mapping]]] = ..., grader_calls: _Optional[_Iterable[_Union[GraderModelCallRecord, _Mapping]]] = ..., evidence_refs: _Optional[_Iterable[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]]] = ..., completed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class EvaluationVerificationReport(_message.Message):
    __slots__ = ("schema_version", "report_id", "run_id", "assignment_id", "status", "failure_count", "failure_reasons", "report_digest_ref", "verified_at")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    FAILURE_COUNT_FIELD_NUMBER: _ClassVar[int]
    FAILURE_REASONS_FIELD_NUMBER: _ClassVar[int]
    REPORT_DIGEST_REF_FIELD_NUMBER: _ClassVar[int]
    VERIFIED_AT_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    report_id: str
    run_id: str
    assignment_id: str
    status: EvaluationVerdictStatus
    failure_count: int
    failure_reasons: _containers.RepeatedScalarFieldContainer[str]
    report_digest_ref: _compliance_pb2.ComplianceEvidenceReference
    verified_at: _timestamp_pb2.Timestamp
    def __init__(self, schema_version: _Optional[str] = ..., report_id: _Optional[str] = ..., run_id: _Optional[str] = ..., assignment_id: _Optional[str] = ..., status: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., failure_count: _Optional[int] = ..., failure_reasons: _Optional[_Iterable[str]] = ..., report_digest_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ..., verified_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class PublicCampaignIdentity(_message.Message):
    __slots__ = ("campaign_id", "campaign_digest", "catalog_id", "catalog_version", "catalog_digest", "model_registry_digest", "lane")
    CAMPAIGN_ID_FIELD_NUMBER: _ClassVar[int]
    CAMPAIGN_DIGEST_FIELD_NUMBER: _ClassVar[int]
    CATALOG_ID_FIELD_NUMBER: _ClassVar[int]
    CATALOG_VERSION_FIELD_NUMBER: _ClassVar[int]
    CATALOG_DIGEST_FIELD_NUMBER: _ClassVar[int]
    MODEL_REGISTRY_DIGEST_FIELD_NUMBER: _ClassVar[int]
    LANE_FIELD_NUMBER: _ClassVar[int]
    campaign_id: str
    campaign_digest: str
    catalog_id: str
    catalog_version: str
    catalog_digest: str
    model_registry_digest: str
    lane: EvaluationLane
    def __init__(self, campaign_id: _Optional[str] = ..., campaign_digest: _Optional[str] = ..., catalog_id: _Optional[str] = ..., catalog_version: _Optional[str] = ..., catalog_digest: _Optional[str] = ..., model_registry_digest: _Optional[str] = ..., lane: _Optional[_Union[EvaluationLane, str]] = ...) -> None: ...

class PublicModelVariantIdentity(_message.Message):
    __slots__ = ("variant_id", "served_model_tag", "model_digest", "model_family", "quantization")
    VARIANT_ID_FIELD_NUMBER: _ClassVar[int]
    SERVED_MODEL_TAG_FIELD_NUMBER: _ClassVar[int]
    MODEL_DIGEST_FIELD_NUMBER: _ClassVar[int]
    MODEL_FAMILY_FIELD_NUMBER: _ClassVar[int]
    QUANTIZATION_FIELD_NUMBER: _ClassVar[int]
    variant_id: str
    served_model_tag: str
    model_digest: str
    model_family: str
    quantization: str
    def __init__(self, variant_id: _Optional[str] = ..., served_model_tag: _Optional[str] = ..., model_digest: _Optional[str] = ..., model_family: _Optional[str] = ..., quantization: _Optional[str] = ...) -> None: ...

class PublicAssignmentLifecycleRecord(_message.Message):
    __slots__ = ("assignment_id", "run_id", "scenario_id", "scenario_category", "lane", "designated_role", "variant_id", "stack_id", "lifecycle_status", "repetition", "observed_at")
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_CATEGORY_FIELD_NUMBER: _ClassVar[int]
    LANE_FIELD_NUMBER: _ClassVar[int]
    DESIGNATED_ROLE_FIELD_NUMBER: _ClassVar[int]
    VARIANT_ID_FIELD_NUMBER: _ClassVar[int]
    STACK_ID_FIELD_NUMBER: _ClassVar[int]
    LIFECYCLE_STATUS_FIELD_NUMBER: _ClassVar[int]
    REPETITION_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_AT_FIELD_NUMBER: _ClassVar[int]
    assignment_id: str
    run_id: str
    scenario_id: str
    scenario_category: EvaluationScenarioCategory
    lane: EvaluationLane
    designated_role: ModelCampaignRole
    variant_id: str
    stack_id: str
    lifecycle_status: EvaluationAssignmentLifecycleStatus
    repetition: int
    observed_at: _timestamp_pb2.Timestamp
    def __init__(self, assignment_id: _Optional[str] = ..., run_id: _Optional[str] = ..., scenario_id: _Optional[str] = ..., scenario_category: _Optional[_Union[EvaluationScenarioCategory, str]] = ..., lane: _Optional[_Union[EvaluationLane, str]] = ..., designated_role: _Optional[_Union[ModelCampaignRole, str]] = ..., variant_id: _Optional[str] = ..., stack_id: _Optional[str] = ..., lifecycle_status: _Optional[_Union[EvaluationAssignmentLifecycleStatus, str]] = ..., repetition: _Optional[int] = ..., observed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class PublicModelCallSummary(_message.Message):
    __slots__ = ("assignment_id", "inference_record_id", "model_role", "agent_persona", "variant_id", "usage_availability", "prompt_tokens", "completion_tokens", "first_token_at_unix_nanos", "generation_duration_nanos", "load_state", "finish_reason", "input_hash", "output_hash")
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    INFERENCE_RECORD_ID_FIELD_NUMBER: _ClassVar[int]
    MODEL_ROLE_FIELD_NUMBER: _ClassVar[int]
    AGENT_PERSONA_FIELD_NUMBER: _ClassVar[int]
    VARIANT_ID_FIELD_NUMBER: _ClassVar[int]
    USAGE_AVAILABILITY_FIELD_NUMBER: _ClassVar[int]
    PROMPT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    COMPLETION_TOKENS_FIELD_NUMBER: _ClassVar[int]
    FIRST_TOKEN_AT_UNIX_NANOS_FIELD_NUMBER: _ClassVar[int]
    GENERATION_DURATION_NANOS_FIELD_NUMBER: _ClassVar[int]
    LOAD_STATE_FIELD_NUMBER: _ClassVar[int]
    FINISH_REASON_FIELD_NUMBER: _ClassVar[int]
    INPUT_HASH_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_HASH_FIELD_NUMBER: _ClassVar[int]
    assignment_id: str
    inference_record_id: str
    model_role: ModelCampaignRole
    agent_persona: str
    variant_id: str
    usage_availability: EvaluationUsageAvailability
    prompt_tokens: int
    completion_tokens: int
    first_token_at_unix_nanos: int
    generation_duration_nanos: int
    load_state: EvaluationLoadState
    finish_reason: str
    input_hash: str
    output_hash: str
    def __init__(self, assignment_id: _Optional[str] = ..., inference_record_id: _Optional[str] = ..., model_role: _Optional[_Union[ModelCampaignRole, str]] = ..., agent_persona: _Optional[str] = ..., variant_id: _Optional[str] = ..., usage_availability: _Optional[_Union[EvaluationUsageAvailability, str]] = ..., prompt_tokens: _Optional[int] = ..., completion_tokens: _Optional[int] = ..., first_token_at_unix_nanos: _Optional[int] = ..., generation_duration_nanos: _Optional[int] = ..., load_state: _Optional[_Union[EvaluationLoadState, str]] = ..., finish_reason: _Optional[str] = ..., input_hash: _Optional[str] = ..., output_hash: _Optional[str] = ...) -> None: ...

class PublicAssignmentResultProjection(_message.Message):
    __slots__ = ("assignment_id", "run_id", "scenario_id", "scenario_category", "lane", "designated_role", "variant_id", "lifecycle_status", "summary_status", "decomposed_scores", "result_digest", "verification_status", "unavailable_metric_reasons", "completed_at")
    ASSIGNMENT_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_CATEGORY_FIELD_NUMBER: _ClassVar[int]
    LANE_FIELD_NUMBER: _ClassVar[int]
    DESIGNATED_ROLE_FIELD_NUMBER: _ClassVar[int]
    VARIANT_ID_FIELD_NUMBER: _ClassVar[int]
    LIFECYCLE_STATUS_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_STATUS_FIELD_NUMBER: _ClassVar[int]
    DECOMPOSED_SCORES_FIELD_NUMBER: _ClassVar[int]
    RESULT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    VERIFICATION_STATUS_FIELD_NUMBER: _ClassVar[int]
    UNAVAILABLE_METRIC_REASONS_FIELD_NUMBER: _ClassVar[int]
    COMPLETED_AT_FIELD_NUMBER: _ClassVar[int]
    assignment_id: str
    run_id: str
    scenario_id: str
    scenario_category: EvaluationScenarioCategory
    lane: EvaluationLane
    designated_role: ModelCampaignRole
    variant_id: str
    lifecycle_status: EvaluationAssignmentLifecycleStatus
    summary_status: EvaluationVerdictStatus
    decomposed_scores: _containers.RepeatedCompositeFieldContainer[DecomposedScoreRecord]
    result_digest: str
    verification_status: str
    unavailable_metric_reasons: _containers.RepeatedScalarFieldContainer[str]
    completed_at: _timestamp_pb2.Timestamp
    def __init__(self, assignment_id: _Optional[str] = ..., run_id: _Optional[str] = ..., scenario_id: _Optional[str] = ..., scenario_category: _Optional[_Union[EvaluationScenarioCategory, str]] = ..., lane: _Optional[_Union[EvaluationLane, str]] = ..., designated_role: _Optional[_Union[ModelCampaignRole, str]] = ..., variant_id: _Optional[str] = ..., lifecycle_status: _Optional[_Union[EvaluationAssignmentLifecycleStatus, str]] = ..., summary_status: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., decomposed_scores: _Optional[_Iterable[_Union[DecomposedScoreRecord, _Mapping]]] = ..., result_digest: _Optional[str] = ..., verification_status: _Optional[str] = ..., unavailable_metric_reasons: _Optional[_Iterable[str]] = ..., completed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...
