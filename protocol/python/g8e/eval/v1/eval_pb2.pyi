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
    __slots__ = ("schema_version", "run_id", "suite_ref", "deployment", "active_posture", "lane", "target_operator_id", "target_operator_session_id", "started_at", "completed_at", "attempt_refs", "final_verification_report_ref")
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
    def __init__(self, schema_version: _Optional[str] = ..., run_id: _Optional[str] = ..., suite_ref: _Optional[_Union[_compliance_pb2.VersionedReference, _Mapping]] = ..., deployment: _Optional[_Union[EvaluationDeploymentIdentity, _Mapping]] = ..., active_posture: _Optional[_Union[EvaluationGovernancePosture, str]] = ..., lane: _Optional[_Union[EvaluationLane, str]] = ..., target_operator_id: _Optional[str] = ..., target_operator_session_id: _Optional[str] = ..., started_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., completed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., attempt_refs: _Optional[_Iterable[str]] = ..., final_verification_report_ref: _Optional[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]] = ...) -> None: ...

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
    __slots__ = ("schema_version", "run", "attempts", "observations", "assertions", "verdicts", "metrics", "evidence_refs", "summary_status", "required_verdict_count", "passed_verdict_count", "summary")
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
    def __init__(self, schema_version: _Optional[str] = ..., run: _Optional[_Union[EvaluationRun, _Mapping]] = ..., attempts: _Optional[_Iterable[_Union[EvaluationAttempt, _Mapping]]] = ..., observations: _Optional[_Iterable[_Union[EvaluationObservation, _Mapping]]] = ..., assertions: _Optional[_Iterable[_Union[EvaluationAssertion, _Mapping]]] = ..., verdicts: _Optional[_Iterable[_Union[EvaluationVerdict, _Mapping]]] = ..., metrics: _Optional[_Iterable[_Union[EvaluationMetric, _Mapping]]] = ..., evidence_refs: _Optional[_Iterable[_Union[_compliance_pb2.ComplianceEvidenceReference, _Mapping]]] = ..., summary_status: _Optional[_Union[EvaluationVerdictStatus, str]] = ..., required_verdict_count: _Optional[int] = ..., passed_verdict_count: _Optional[int] = ..., summary: _Optional[str] = ...) -> None: ...
