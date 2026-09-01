import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class VersionedReference(_message.Message):
    __slots__ = ("id", "version")
    ID_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    id: str
    version: str
    def __init__(self, id: _Optional[str] = ..., version: _Optional[str] = ...) -> None: ...

class NamedDigest(_message.Message):
    __slots__ = ("name", "sha256")
    NAME_FIELD_NUMBER: _ClassVar[int]
    SHA256_FIELD_NUMBER: _ClassVar[int]
    name: str
    sha256: str
    def __init__(self, name: _Optional[str] = ..., sha256: _Optional[str] = ...) -> None: ...

class ControlAssertionDefinition(_message.Message):
    __slots__ = ("assertion_id", "assertion_version", "title", "statement", "category", "component_scope", "responsibility", "applicable_action_classes", "applicable_arms", "required_evidence_types", "required_grader_refs", "required_verifier_refs", "minimum_evidence_level", "validation_cycle", "missing_evidence_policy", "passing_rule", "exclusions")
    ASSERTION_ID_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_VERSION_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    STATEMENT_FIELD_NUMBER: _ClassVar[int]
    CATEGORY_FIELD_NUMBER: _ClassVar[int]
    COMPONENT_SCOPE_FIELD_NUMBER: _ClassVar[int]
    RESPONSIBILITY_FIELD_NUMBER: _ClassVar[int]
    APPLICABLE_ACTION_CLASSES_FIELD_NUMBER: _ClassVar[int]
    APPLICABLE_ARMS_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_EVIDENCE_TYPES_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_GRADER_REFS_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_VERIFIER_REFS_FIELD_NUMBER: _ClassVar[int]
    MINIMUM_EVIDENCE_LEVEL_FIELD_NUMBER: _ClassVar[int]
    VALIDATION_CYCLE_FIELD_NUMBER: _ClassVar[int]
    MISSING_EVIDENCE_POLICY_FIELD_NUMBER: _ClassVar[int]
    PASSING_RULE_FIELD_NUMBER: _ClassVar[int]
    EXCLUSIONS_FIELD_NUMBER: _ClassVar[int]
    assertion_id: str
    assertion_version: str
    title: str
    statement: str
    category: str
    component_scope: _containers.RepeatedScalarFieldContainer[str]
    responsibility: str
    applicable_action_classes: _containers.RepeatedScalarFieldContainer[str]
    applicable_arms: _containers.RepeatedScalarFieldContainer[str]
    required_evidence_types: _containers.RepeatedScalarFieldContainer[str]
    required_grader_refs: _containers.RepeatedCompositeFieldContainer[VersionedReference]
    required_verifier_refs: _containers.RepeatedCompositeFieldContainer[VersionedReference]
    minimum_evidence_level: str
    validation_cycle: str
    missing_evidence_policy: str
    passing_rule: str
    exclusions: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, assertion_id: _Optional[str] = ..., assertion_version: _Optional[str] = ..., title: _Optional[str] = ..., statement: _Optional[str] = ..., category: _Optional[str] = ..., component_scope: _Optional[_Iterable[str]] = ..., responsibility: _Optional[str] = ..., applicable_action_classes: _Optional[_Iterable[str]] = ..., applicable_arms: _Optional[_Iterable[str]] = ..., required_evidence_types: _Optional[_Iterable[str]] = ..., required_grader_refs: _Optional[_Iterable[_Union[VersionedReference, _Mapping]]] = ..., required_verifier_refs: _Optional[_Iterable[_Union[VersionedReference, _Mapping]]] = ..., minimum_evidence_level: _Optional[str] = ..., validation_cycle: _Optional[str] = ..., missing_evidence_policy: _Optional[str] = ..., passing_rule: _Optional[str] = ..., exclusions: _Optional[_Iterable[str]] = ...) -> None: ...

class ControlAssertionCatalog(_message.Message):
    __slots__ = ("catalog_id", "catalog_version", "sha256", "assertions")
    CATALOG_ID_FIELD_NUMBER: _ClassVar[int]
    CATALOG_VERSION_FIELD_NUMBER: _ClassVar[int]
    SHA256_FIELD_NUMBER: _ClassVar[int]
    ASSERTIONS_FIELD_NUMBER: _ClassVar[int]
    catalog_id: str
    catalog_version: str
    sha256: str
    assertions: _containers.RepeatedCompositeFieldContainer[ControlAssertionDefinition]
    def __init__(self, catalog_id: _Optional[str] = ..., catalog_version: _Optional[str] = ..., sha256: _Optional[str] = ..., assertions: _Optional[_Iterable[_Union[ControlAssertionDefinition, _Mapping]]] = ...) -> None: ...

class FrameworkControlDefinition(_message.Message):
    __slots__ = ("control_id", "title", "description", "applicability_rules", "responsibility", "source_reference", "support_status", "support_rationale")
    CONTROL_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    APPLICABILITY_RULES_FIELD_NUMBER: _ClassVar[int]
    RESPONSIBILITY_FIELD_NUMBER: _ClassVar[int]
    SOURCE_REFERENCE_FIELD_NUMBER: _ClassVar[int]
    SUPPORT_STATUS_FIELD_NUMBER: _ClassVar[int]
    SUPPORT_RATIONALE_FIELD_NUMBER: _ClassVar[int]
    control_id: str
    title: str
    description: str
    applicability_rules: _containers.RepeatedScalarFieldContainer[str]
    responsibility: str
    source_reference: str
    support_status: str
    support_rationale: str
    def __init__(self, control_id: _Optional[str] = ..., title: _Optional[str] = ..., description: _Optional[str] = ..., applicability_rules: _Optional[_Iterable[str]] = ..., responsibility: _Optional[str] = ..., source_reference: _Optional[str] = ..., support_status: _Optional[str] = ..., support_rationale: _Optional[str] = ...) -> None: ...

class FrameworkDefinition(_message.Message):
    __slots__ = ("framework_id", "framework_version", "title", "publisher", "source", "catalog_sha256", "effective_date", "controls")
    FRAMEWORK_ID_FIELD_NUMBER: _ClassVar[int]
    FRAMEWORK_VERSION_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    PUBLISHER_FIELD_NUMBER: _ClassVar[int]
    SOURCE_FIELD_NUMBER: _ClassVar[int]
    CATALOG_SHA256_FIELD_NUMBER: _ClassVar[int]
    EFFECTIVE_DATE_FIELD_NUMBER: _ClassVar[int]
    CONTROLS_FIELD_NUMBER: _ClassVar[int]
    framework_id: str
    framework_version: str
    title: str
    publisher: str
    source: str
    catalog_sha256: str
    effective_date: str
    controls: _containers.RepeatedCompositeFieldContainer[FrameworkControlDefinition]
    def __init__(self, framework_id: _Optional[str] = ..., framework_version: _Optional[str] = ..., title: _Optional[str] = ..., publisher: _Optional[str] = ..., source: _Optional[str] = ..., catalog_sha256: _Optional[str] = ..., effective_date: _Optional[str] = ..., controls: _Optional[_Iterable[_Union[FrameworkControlDefinition, _Mapping]]] = ...) -> None: ...

class FrameworkCatalog(_message.Message):
    __slots__ = ("catalog_id", "catalog_version", "sha256", "frameworks")
    CATALOG_ID_FIELD_NUMBER: _ClassVar[int]
    CATALOG_VERSION_FIELD_NUMBER: _ClassVar[int]
    SHA256_FIELD_NUMBER: _ClassVar[int]
    FRAMEWORKS_FIELD_NUMBER: _ClassVar[int]
    catalog_id: str
    catalog_version: str
    sha256: str
    frameworks: _containers.RepeatedCompositeFieldContainer[FrameworkDefinition]
    def __init__(self, catalog_id: _Optional[str] = ..., catalog_version: _Optional[str] = ..., sha256: _Optional[str] = ..., frameworks: _Optional[_Iterable[_Union[FrameworkDefinition, _Mapping]]] = ...) -> None: ...

class ControlCrosswalk(_message.Message):
    __slots__ = ("crosswalk_id", "crosswalk_version", "framework_ref", "control_id", "assertion_refs", "mapping_type", "rationale", "applicability_conditions", "responsibility", "required_evidence_level", "reviewed_at", "reviewer_identity")
    CROSSWALK_ID_FIELD_NUMBER: _ClassVar[int]
    CROSSWALK_VERSION_FIELD_NUMBER: _ClassVar[int]
    FRAMEWORK_REF_FIELD_NUMBER: _ClassVar[int]
    CONTROL_ID_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_REFS_FIELD_NUMBER: _ClassVar[int]
    MAPPING_TYPE_FIELD_NUMBER: _ClassVar[int]
    RATIONALE_FIELD_NUMBER: _ClassVar[int]
    APPLICABILITY_CONDITIONS_FIELD_NUMBER: _ClassVar[int]
    RESPONSIBILITY_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_EVIDENCE_LEVEL_FIELD_NUMBER: _ClassVar[int]
    REVIEWED_AT_FIELD_NUMBER: _ClassVar[int]
    REVIEWER_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    crosswalk_id: str
    crosswalk_version: str
    framework_ref: VersionedReference
    control_id: str
    assertion_refs: _containers.RepeatedCompositeFieldContainer[VersionedReference]
    mapping_type: str
    rationale: str
    applicability_conditions: _containers.RepeatedScalarFieldContainer[str]
    responsibility: str
    required_evidence_level: str
    reviewed_at: _timestamp_pb2.Timestamp
    reviewer_identity: str
    def __init__(self, crosswalk_id: _Optional[str] = ..., crosswalk_version: _Optional[str] = ..., framework_ref: _Optional[_Union[VersionedReference, _Mapping]] = ..., control_id: _Optional[str] = ..., assertion_refs: _Optional[_Iterable[_Union[VersionedReference, _Mapping]]] = ..., mapping_type: _Optional[str] = ..., rationale: _Optional[str] = ..., applicability_conditions: _Optional[_Iterable[str]] = ..., responsibility: _Optional[str] = ..., required_evidence_level: _Optional[str] = ..., reviewed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., reviewer_identity: _Optional[str] = ...) -> None: ...

class ControlCrosswalkCatalog(_message.Message):
    __slots__ = ("catalog_id", "catalog_version", "sha256", "mappings")
    CATALOG_ID_FIELD_NUMBER: _ClassVar[int]
    CATALOG_VERSION_FIELD_NUMBER: _ClassVar[int]
    SHA256_FIELD_NUMBER: _ClassVar[int]
    MAPPINGS_FIELD_NUMBER: _ClassVar[int]
    catalog_id: str
    catalog_version: str
    sha256: str
    mappings: _containers.RepeatedCompositeFieldContainer[ControlCrosswalk]
    def __init__(self, catalog_id: _Optional[str] = ..., catalog_version: _Optional[str] = ..., sha256: _Optional[str] = ..., mappings: _Optional[_Iterable[_Union[ControlCrosswalk, _Mapping]]] = ...) -> None: ...

class ComponentInventoryEntry(_message.Message):
    __slots__ = ("component_id", "component_type", "version", "digest")
    COMPONENT_ID_FIELD_NUMBER: _ClassVar[int]
    COMPONENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    DIGEST_FIELD_NUMBER: _ClassVar[int]
    component_id: str
    component_type: str
    version: str
    digest: str
    def __init__(self, component_id: _Optional[str] = ..., component_type: _Optional[str] = ..., version: _Optional[str] = ..., digest: _Optional[str] = ...) -> None: ...

class AssessmentScope(_message.Message):
    __slots__ = ("scope_id", "organization_id", "deployment_id", "product_version", "build_identity", "source_revision", "image_digests", "component_inventory", "network_topology_hash", "configuration_hashes", "doctrine_bundle_hashes", "consensus_policy_hashes", "trust_anchor_ids", "cryptographic_mode", "assessment_window_start", "assessment_window_end", "excluded_components", "customer_responsibilities")
    SCOPE_ID_FIELD_NUMBER: _ClassVar[int]
    ORGANIZATION_ID_FIELD_NUMBER: _ClassVar[int]
    DEPLOYMENT_ID_FIELD_NUMBER: _ClassVar[int]
    PRODUCT_VERSION_FIELD_NUMBER: _ClassVar[int]
    BUILD_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    SOURCE_REVISION_FIELD_NUMBER: _ClassVar[int]
    IMAGE_DIGESTS_FIELD_NUMBER: _ClassVar[int]
    COMPONENT_INVENTORY_FIELD_NUMBER: _ClassVar[int]
    NETWORK_TOPOLOGY_HASH_FIELD_NUMBER: _ClassVar[int]
    CONFIGURATION_HASHES_FIELD_NUMBER: _ClassVar[int]
    DOCTRINE_BUNDLE_HASHES_FIELD_NUMBER: _ClassVar[int]
    CONSENSUS_POLICY_HASHES_FIELD_NUMBER: _ClassVar[int]
    TRUST_ANCHOR_IDS_FIELD_NUMBER: _ClassVar[int]
    CRYPTOGRAPHIC_MODE_FIELD_NUMBER: _ClassVar[int]
    ASSESSMENT_WINDOW_START_FIELD_NUMBER: _ClassVar[int]
    ASSESSMENT_WINDOW_END_FIELD_NUMBER: _ClassVar[int]
    EXCLUDED_COMPONENTS_FIELD_NUMBER: _ClassVar[int]
    CUSTOMER_RESPONSIBILITIES_FIELD_NUMBER: _ClassVar[int]
    scope_id: str
    organization_id: str
    deployment_id: str
    product_version: str
    build_identity: str
    source_revision: str
    image_digests: _containers.RepeatedCompositeFieldContainer[NamedDigest]
    component_inventory: _containers.RepeatedCompositeFieldContainer[ComponentInventoryEntry]
    network_topology_hash: str
    configuration_hashes: _containers.RepeatedCompositeFieldContainer[NamedDigest]
    doctrine_bundle_hashes: _containers.RepeatedCompositeFieldContainer[NamedDigest]
    consensus_policy_hashes: _containers.RepeatedCompositeFieldContainer[NamedDigest]
    trust_anchor_ids: _containers.RepeatedScalarFieldContainer[str]
    cryptographic_mode: str
    assessment_window_start: _timestamp_pb2.Timestamp
    assessment_window_end: _timestamp_pb2.Timestamp
    excluded_components: _containers.RepeatedScalarFieldContainer[str]
    customer_responsibilities: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, scope_id: _Optional[str] = ..., organization_id: _Optional[str] = ..., deployment_id: _Optional[str] = ..., product_version: _Optional[str] = ..., build_identity: _Optional[str] = ..., source_revision: _Optional[str] = ..., image_digests: _Optional[_Iterable[_Union[NamedDigest, _Mapping]]] = ..., component_inventory: _Optional[_Iterable[_Union[ComponentInventoryEntry, _Mapping]]] = ..., network_topology_hash: _Optional[str] = ..., configuration_hashes: _Optional[_Iterable[_Union[NamedDigest, _Mapping]]] = ..., doctrine_bundle_hashes: _Optional[_Iterable[_Union[NamedDigest, _Mapping]]] = ..., consensus_policy_hashes: _Optional[_Iterable[_Union[NamedDigest, _Mapping]]] = ..., trust_anchor_ids: _Optional[_Iterable[str]] = ..., cryptographic_mode: _Optional[str] = ..., assessment_window_start: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., assessment_window_end: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., excluded_components: _Optional[_Iterable[str]] = ..., customer_responsibilities: _Optional[_Iterable[str]] = ...) -> None: ...

class EvidenceEncryptionMetadata(_message.Message):
    __slots__ = ("algorithm", "key_id", "authorization_scope", "plaintext_sha256", "authenticated_metadata_sha256")
    ALGORITHM_FIELD_NUMBER: _ClassVar[int]
    KEY_ID_FIELD_NUMBER: _ClassVar[int]
    AUTHORIZATION_SCOPE_FIELD_NUMBER: _ClassVar[int]
    PLAINTEXT_SHA256_FIELD_NUMBER: _ClassVar[int]
    AUTHENTICATED_METADATA_SHA256_FIELD_NUMBER: _ClassVar[int]
    algorithm: str
    key_id: str
    authorization_scope: str
    plaintext_sha256: str
    authenticated_metadata_sha256: str
    def __init__(self, algorithm: _Optional[str] = ..., key_id: _Optional[str] = ..., authorization_scope: _Optional[str] = ..., plaintext_sha256: _Optional[str] = ..., authenticated_metadata_sha256: _Optional[str] = ...) -> None: ...

class ComplianceEvidenceReference(_message.Message):
    __slots__ = ("artifact_id", "artifact_type", "sha256", "media_type", "schema_ref", "producer_identity", "produced_at", "scope_id", "run_id", "attempt_id", "scenario_id", "transaction_id", "verification_status", "verifier_id", "verifier_version", "verified_at", "bundle_path", "encryption")
    ARTIFACT_ID_FIELD_NUMBER: _ClassVar[int]
    ARTIFACT_TYPE_FIELD_NUMBER: _ClassVar[int]
    SHA256_FIELD_NUMBER: _ClassVar[int]
    MEDIA_TYPE_FIELD_NUMBER: _ClassVar[int]
    SCHEMA_REF_FIELD_NUMBER: _ClassVar[int]
    PRODUCER_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    PRODUCED_AT_FIELD_NUMBER: _ClassVar[int]
    SCOPE_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_ID_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_ID_FIELD_NUMBER: _ClassVar[int]
    VERIFICATION_STATUS_FIELD_NUMBER: _ClassVar[int]
    VERIFIER_ID_FIELD_NUMBER: _ClassVar[int]
    VERIFIER_VERSION_FIELD_NUMBER: _ClassVar[int]
    VERIFIED_AT_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_PATH_FIELD_NUMBER: _ClassVar[int]
    ENCRYPTION_FIELD_NUMBER: _ClassVar[int]
    artifact_id: str
    artifact_type: str
    sha256: str
    media_type: str
    schema_ref: str
    producer_identity: str
    produced_at: _timestamp_pb2.Timestamp
    scope_id: str
    run_id: str
    attempt_id: str
    scenario_id: str
    transaction_id: str
    verification_status: str
    verifier_id: str
    verifier_version: str
    verified_at: _timestamp_pb2.Timestamp
    bundle_path: str
    encryption: EvidenceEncryptionMetadata
    def __init__(self, artifact_id: _Optional[str] = ..., artifact_type: _Optional[str] = ..., sha256: _Optional[str] = ..., media_type: _Optional[str] = ..., schema_ref: _Optional[str] = ..., producer_identity: _Optional[str] = ..., produced_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., scope_id: _Optional[str] = ..., run_id: _Optional[str] = ..., attempt_id: _Optional[str] = ..., scenario_id: _Optional[str] = ..., transaction_id: _Optional[str] = ..., verification_status: _Optional[str] = ..., verifier_id: _Optional[str] = ..., verifier_version: _Optional[str] = ..., verified_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., bundle_path: _Optional[str] = ..., encryption: _Optional[_Union[EvidenceEncryptionMetadata, _Mapping]] = ...) -> None: ...

class ControlAssertionAssessment(_message.Message):
    __slots__ = ("assessment_id", "scope_id", "assertion_ref", "status", "evidence_level", "evaluated_at", "verifier_ref", "evidence_refs", "metric_refs", "freshness_status", "failure_reason", "limitations")
    ASSESSMENT_ID_FIELD_NUMBER: _ClassVar[int]
    SCOPE_ID_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_REF_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_LEVEL_FIELD_NUMBER: _ClassVar[int]
    EVALUATED_AT_FIELD_NUMBER: _ClassVar[int]
    VERIFIER_REF_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_REFS_FIELD_NUMBER: _ClassVar[int]
    METRIC_REFS_FIELD_NUMBER: _ClassVar[int]
    FRESHNESS_STATUS_FIELD_NUMBER: _ClassVar[int]
    FAILURE_REASON_FIELD_NUMBER: _ClassVar[int]
    LIMITATIONS_FIELD_NUMBER: _ClassVar[int]
    assessment_id: str
    scope_id: str
    assertion_ref: VersionedReference
    status: str
    evidence_level: str
    evaluated_at: _timestamp_pb2.Timestamp
    verifier_ref: VersionedReference
    evidence_refs: _containers.RepeatedScalarFieldContainer[str]
    metric_refs: _containers.RepeatedScalarFieldContainer[str]
    freshness_status: str
    failure_reason: str
    limitations: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, assessment_id: _Optional[str] = ..., scope_id: _Optional[str] = ..., assertion_ref: _Optional[_Union[VersionedReference, _Mapping]] = ..., status: _Optional[str] = ..., evidence_level: _Optional[str] = ..., evaluated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., verifier_ref: _Optional[_Union[VersionedReference, _Mapping]] = ..., evidence_refs: _Optional[_Iterable[str]] = ..., metric_refs: _Optional[_Iterable[str]] = ..., freshness_status: _Optional[str] = ..., failure_reason: _Optional[str] = ..., limitations: _Optional[_Iterable[str]] = ...) -> None: ...

class FrameworkControlAssessment(_message.Message):
    __slots__ = ("assessment_id", "scope_id", "framework_ref", "control_id", "status", "responsibility", "mapping_refs", "assertion_assessment_refs", "customer_attestation_refs", "evidence_level", "findings", "limitations", "remediation")
    ASSESSMENT_ID_FIELD_NUMBER: _ClassVar[int]
    SCOPE_ID_FIELD_NUMBER: _ClassVar[int]
    FRAMEWORK_REF_FIELD_NUMBER: _ClassVar[int]
    CONTROL_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    RESPONSIBILITY_FIELD_NUMBER: _ClassVar[int]
    MAPPING_REFS_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_ASSESSMENT_REFS_FIELD_NUMBER: _ClassVar[int]
    CUSTOMER_ATTESTATION_REFS_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_LEVEL_FIELD_NUMBER: _ClassVar[int]
    FINDINGS_FIELD_NUMBER: _ClassVar[int]
    LIMITATIONS_FIELD_NUMBER: _ClassVar[int]
    REMEDIATION_FIELD_NUMBER: _ClassVar[int]
    assessment_id: str
    scope_id: str
    framework_ref: VersionedReference
    control_id: str
    status: str
    responsibility: str
    mapping_refs: _containers.RepeatedScalarFieldContainer[str]
    assertion_assessment_refs: _containers.RepeatedScalarFieldContainer[str]
    customer_attestation_refs: _containers.RepeatedScalarFieldContainer[str]
    evidence_level: str
    findings: _containers.RepeatedScalarFieldContainer[str]
    limitations: _containers.RepeatedScalarFieldContainer[str]
    remediation: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, assessment_id: _Optional[str] = ..., scope_id: _Optional[str] = ..., framework_ref: _Optional[_Union[VersionedReference, _Mapping]] = ..., control_id: _Optional[str] = ..., status: _Optional[str] = ..., responsibility: _Optional[str] = ..., mapping_refs: _Optional[_Iterable[str]] = ..., assertion_assessment_refs: _Optional[_Iterable[str]] = ..., customer_attestation_refs: _Optional[_Iterable[str]] = ..., evidence_level: _Optional[str] = ..., findings: _Optional[_Iterable[str]] = ..., limitations: _Optional[_Iterable[str]] = ..., remediation: _Optional[_Iterable[str]] = ...) -> None: ...

class ChecksumEntry(_message.Message):
    __slots__ = ("bundle_path", "sha256")
    BUNDLE_PATH_FIELD_NUMBER: _ClassVar[int]
    SHA256_FIELD_NUMBER: _ClassVar[int]
    bundle_path: str
    sha256: str
    def __init__(self, bundle_path: _Optional[str] = ..., sha256: _Optional[str] = ...) -> None: ...

class ReportSignature(_message.Message):
    __slots__ = ("key_id", "algorithm", "signed_sha256", "signature")
    KEY_ID_FIELD_NUMBER: _ClassVar[int]
    ALGORITHM_FIELD_NUMBER: _ClassVar[int]
    SIGNED_SHA256_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    key_id: str
    algorithm: str
    signed_sha256: str
    signature: str
    def __init__(self, key_id: _Optional[str] = ..., algorithm: _Optional[str] = ..., signed_sha256: _Optional[str] = ..., signature: _Optional[str] = ...) -> None: ...

class ComplianceReportManifest(_message.Message):
    __slots__ = ("report_id", "report_schema_version", "generated_at", "generator_identity", "generator_version", "scope_ref", "framework_refs", "assertion_catalog_ref", "crosswalk_refs", "assessment_refs", "evidence_index_ref", "checksum_root", "signature")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    REPORT_SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    GENERATED_AT_FIELD_NUMBER: _ClassVar[int]
    GENERATOR_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    GENERATOR_VERSION_FIELD_NUMBER: _ClassVar[int]
    SCOPE_REF_FIELD_NUMBER: _ClassVar[int]
    FRAMEWORK_REFS_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_CATALOG_REF_FIELD_NUMBER: _ClassVar[int]
    CROSSWALK_REFS_FIELD_NUMBER: _ClassVar[int]
    ASSESSMENT_REFS_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_INDEX_REF_FIELD_NUMBER: _ClassVar[int]
    CHECKSUM_ROOT_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    report_schema_version: str
    generated_at: _timestamp_pb2.Timestamp
    generator_identity: str
    generator_version: str
    scope_ref: str
    framework_refs: _containers.RepeatedCompositeFieldContainer[VersionedReference]
    assertion_catalog_ref: str
    crosswalk_refs: _containers.RepeatedScalarFieldContainer[str]
    assessment_refs: _containers.RepeatedScalarFieldContainer[str]
    evidence_index_ref: str
    checksum_root: str
    signature: ReportSignature
    def __init__(self, report_id: _Optional[str] = ..., report_schema_version: _Optional[str] = ..., generated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., generator_identity: _Optional[str] = ..., generator_version: _Optional[str] = ..., scope_ref: _Optional[str] = ..., framework_refs: _Optional[_Iterable[_Union[VersionedReference, _Mapping]]] = ..., assertion_catalog_ref: _Optional[str] = ..., crosswalk_refs: _Optional[_Iterable[str]] = ..., assessment_refs: _Optional[_Iterable[str]] = ..., evidence_index_ref: _Optional[str] = ..., checksum_root: _Optional[str] = ..., signature: _Optional[_Union[ReportSignature, _Mapping]] = ...) -> None: ...

class VerificationFailure(_message.Message):
    __slots__ = ("code", "subject_ref", "reason")
    CODE_FIELD_NUMBER: _ClassVar[int]
    SUBJECT_REF_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    code: str
    subject_ref: str
    reason: str
    def __init__(self, code: _Optional[str] = ..., subject_ref: _Optional[str] = ..., reason: _Optional[str] = ...) -> None: ...

class ComplianceVerificationReport(_message.Message):
    __slots__ = ("report_id", "valid", "verified_at", "verifier_id", "verifier_version", "failures", "reproduced_checksum_root")
    REPORT_ID_FIELD_NUMBER: _ClassVar[int]
    VALID_FIELD_NUMBER: _ClassVar[int]
    VERIFIED_AT_FIELD_NUMBER: _ClassVar[int]
    VERIFIER_ID_FIELD_NUMBER: _ClassVar[int]
    VERIFIER_VERSION_FIELD_NUMBER: _ClassVar[int]
    FAILURES_FIELD_NUMBER: _ClassVar[int]
    REPRODUCED_CHECKSUM_ROOT_FIELD_NUMBER: _ClassVar[int]
    report_id: str
    valid: bool
    verified_at: _timestamp_pb2.Timestamp
    verifier_id: str
    verifier_version: str
    failures: _containers.RepeatedCompositeFieldContainer[VerificationFailure]
    reproduced_checksum_root: str
    def __init__(self, report_id: _Optional[str] = ..., valid: _Optional[bool] = ..., verified_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., verifier_id: _Optional[str] = ..., verifier_version: _Optional[str] = ..., failures: _Optional[_Iterable[_Union[VerificationFailure, _Mapping]]] = ..., reproduced_checksum_root: _Optional[str] = ...) -> None: ...

class FrameworkControlReference(_message.Message):
    __slots__ = ("framework_ref", "control_id")
    FRAMEWORK_REF_FIELD_NUMBER: _ClassVar[int]
    CONTROL_ID_FIELD_NUMBER: _ClassVar[int]
    framework_ref: VersionedReference
    control_id: str
    def __init__(self, framework_ref: _Optional[_Union[VersionedReference, _Mapping]] = ..., control_id: _Optional[str] = ...) -> None: ...

class DemoManifest(_message.Message):
    __slots__ = ("demo_id", "demo_version", "run_id", "scope_id", "generated_at", "scenario_definition_refs", "provenance_hashes", "required_environment", "framework_control_refs", "supported_lanes")
    DEMO_ID_FIELD_NUMBER: _ClassVar[int]
    DEMO_VERSION_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    SCOPE_ID_FIELD_NUMBER: _ClassVar[int]
    GENERATED_AT_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_DEFINITION_REFS_FIELD_NUMBER: _ClassVar[int]
    PROVENANCE_HASHES_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_ENVIRONMENT_FIELD_NUMBER: _ClassVar[int]
    FRAMEWORK_CONTROL_REFS_FIELD_NUMBER: _ClassVar[int]
    SUPPORTED_LANES_FIELD_NUMBER: _ClassVar[int]
    demo_id: str
    demo_version: str
    run_id: str
    scope_id: str
    generated_at: _timestamp_pb2.Timestamp
    scenario_definition_refs: _containers.RepeatedCompositeFieldContainer[VersionedReference]
    provenance_hashes: _containers.RepeatedCompositeFieldContainer[NamedDigest]
    required_environment: _containers.RepeatedScalarFieldContainer[str]
    framework_control_refs: _containers.RepeatedCompositeFieldContainer[FrameworkControlReference]
    supported_lanes: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, demo_id: _Optional[str] = ..., demo_version: _Optional[str] = ..., run_id: _Optional[str] = ..., scope_id: _Optional[str] = ..., generated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., scenario_definition_refs: _Optional[_Iterable[_Union[VersionedReference, _Mapping]]] = ..., provenance_hashes: _Optional[_Iterable[_Union[NamedDigest, _Mapping]]] = ..., required_environment: _Optional[_Iterable[str]] = ..., framework_control_refs: _Optional[_Iterable[_Union[FrameworkControlReference, _Mapping]]] = ..., supported_lanes: _Optional[_Iterable[str]] = ...) -> None: ...

class DemoScenarioDefinition(_message.Message):
    __slots__ = ("scenario_id", "scenario_version", "display_number", "title", "purpose", "risk_category", "expected_action_classes", "expected_outcome", "expected_rejection_layer", "initial_state_fixture_ref", "terminal_state_assertions", "required_receipts", "required_deterministic_stages", "assertion_refs", "framework_control_refs", "required_evidence_types", "required_evidence_level", "timeout_seconds", "failure_policy", "harness_scenario")
    SCENARIO_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_VERSION_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NUMBER_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    PURPOSE_FIELD_NUMBER: _ClassVar[int]
    RISK_CATEGORY_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_ACTION_CLASSES_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_OUTCOME_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_REJECTION_LAYER_FIELD_NUMBER: _ClassVar[int]
    INITIAL_STATE_FIXTURE_REF_FIELD_NUMBER: _ClassVar[int]
    TERMINAL_STATE_ASSERTIONS_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_RECEIPTS_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_DETERMINISTIC_STAGES_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_REFS_FIELD_NUMBER: _ClassVar[int]
    FRAMEWORK_CONTROL_REFS_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_EVIDENCE_TYPES_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_EVIDENCE_LEVEL_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_SECONDS_FIELD_NUMBER: _ClassVar[int]
    FAILURE_POLICY_FIELD_NUMBER: _ClassVar[int]
    HARNESS_SCENARIO_FIELD_NUMBER: _ClassVar[int]
    scenario_id: str
    scenario_version: str
    display_number: str
    title: str
    purpose: str
    risk_category: str
    expected_action_classes: _containers.RepeatedScalarFieldContainer[str]
    expected_outcome: str
    expected_rejection_layer: str
    initial_state_fixture_ref: str
    terminal_state_assertions: _containers.RepeatedScalarFieldContainer[str]
    required_receipts: _containers.RepeatedScalarFieldContainer[str]
    required_deterministic_stages: _containers.RepeatedScalarFieldContainer[str]
    assertion_refs: _containers.RepeatedCompositeFieldContainer[VersionedReference]
    framework_control_refs: _containers.RepeatedCompositeFieldContainer[FrameworkControlReference]
    required_evidence_types: _containers.RepeatedScalarFieldContainer[str]
    required_evidence_level: str
    timeout_seconds: int
    failure_policy: str
    harness_scenario: str
    def __init__(self, scenario_id: _Optional[str] = ..., scenario_version: _Optional[str] = ..., display_number: _Optional[str] = ..., title: _Optional[str] = ..., purpose: _Optional[str] = ..., risk_category: _Optional[str] = ..., expected_action_classes: _Optional[_Iterable[str]] = ..., expected_outcome: _Optional[str] = ..., expected_rejection_layer: _Optional[str] = ..., initial_state_fixture_ref: _Optional[str] = ..., terminal_state_assertions: _Optional[_Iterable[str]] = ..., required_receipts: _Optional[_Iterable[str]] = ..., required_deterministic_stages: _Optional[_Iterable[str]] = ..., assertion_refs: _Optional[_Iterable[_Union[VersionedReference, _Mapping]]] = ..., framework_control_refs: _Optional[_Iterable[_Union[FrameworkControlReference, _Mapping]]] = ..., required_evidence_types: _Optional[_Iterable[str]] = ..., required_evidence_level: _Optional[str] = ..., timeout_seconds: _Optional[int] = ..., failure_policy: _Optional[str] = ..., harness_scenario: _Optional[str] = ...) -> None: ...

class DemoStepResult(_message.Message):
    __slots__ = ("step_id", "operation", "started_at", "completed_at", "status", "exit_code", "protocol_result", "evidence_refs", "failure", "required")
    STEP_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    STARTED_AT_FIELD_NUMBER: _ClassVar[int]
    COMPLETED_AT_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    EXIT_CODE_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_RESULT_FIELD_NUMBER: _ClassVar[int]
    EVIDENCE_REFS_FIELD_NUMBER: _ClassVar[int]
    FAILURE_FIELD_NUMBER: _ClassVar[int]
    REQUIRED_FIELD_NUMBER: _ClassVar[int]
    step_id: str
    operation: str
    started_at: _timestamp_pb2.Timestamp
    completed_at: _timestamp_pb2.Timestamp
    status: str
    exit_code: int
    protocol_result: str
    evidence_refs: _containers.RepeatedScalarFieldContainer[str]
    failure: str
    required: bool
    def __init__(self, step_id: _Optional[str] = ..., operation: _Optional[str] = ..., started_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., completed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., status: _Optional[str] = ..., exit_code: _Optional[int] = ..., protocol_result: _Optional[str] = ..., evidence_refs: _Optional[_Iterable[str]] = ..., failure: _Optional[str] = ..., required: _Optional[bool] = ...) -> None: ...

class DemoScenarioResult(_message.Message):
    __slots__ = ("result_id", "scenario_ref", "demo_id", "scope_id", "run_id", "started_at", "completed_at", "status", "investigation_ids", "transaction_ids", "receipt_refs", "state_observation_refs", "metric_refs", "ksi_refs", "assertion_refs", "framework_control_refs", "step_results", "verification_status", "failure", "limitations", "display_number", "title", "metrics_summary")
    RESULT_ID_FIELD_NUMBER: _ClassVar[int]
    SCENARIO_REF_FIELD_NUMBER: _ClassVar[int]
    DEMO_ID_FIELD_NUMBER: _ClassVar[int]
    SCOPE_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    STARTED_AT_FIELD_NUMBER: _ClassVar[int]
    COMPLETED_AT_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    INVESTIGATION_IDS_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_IDS_FIELD_NUMBER: _ClassVar[int]
    RECEIPT_REFS_FIELD_NUMBER: _ClassVar[int]
    STATE_OBSERVATION_REFS_FIELD_NUMBER: _ClassVar[int]
    METRIC_REFS_FIELD_NUMBER: _ClassVar[int]
    KSI_REFS_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_REFS_FIELD_NUMBER: _ClassVar[int]
    FRAMEWORK_CONTROL_REFS_FIELD_NUMBER: _ClassVar[int]
    STEP_RESULTS_FIELD_NUMBER: _ClassVar[int]
    VERIFICATION_STATUS_FIELD_NUMBER: _ClassVar[int]
    FAILURE_FIELD_NUMBER: _ClassVar[int]
    LIMITATIONS_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NUMBER_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    METRICS_SUMMARY_FIELD_NUMBER: _ClassVar[int]
    result_id: str
    scenario_ref: VersionedReference
    demo_id: str
    scope_id: str
    run_id: str
    started_at: _timestamp_pb2.Timestamp
    completed_at: _timestamp_pb2.Timestamp
    status: str
    investigation_ids: _containers.RepeatedScalarFieldContainer[str]
    transaction_ids: _containers.RepeatedScalarFieldContainer[str]
    receipt_refs: _containers.RepeatedScalarFieldContainer[str]
    state_observation_refs: _containers.RepeatedScalarFieldContainer[str]
    metric_refs: _containers.RepeatedScalarFieldContainer[str]
    ksi_refs: _containers.RepeatedScalarFieldContainer[str]
    assertion_refs: _containers.RepeatedCompositeFieldContainer[VersionedReference]
    framework_control_refs: _containers.RepeatedCompositeFieldContainer[FrameworkControlReference]
    step_results: _containers.RepeatedCompositeFieldContainer[DemoStepResult]
    verification_status: str
    failure: str
    limitations: _containers.RepeatedScalarFieldContainer[str]
    display_number: str
    title: str
    metrics_summary: str
    def __init__(self, result_id: _Optional[str] = ..., scenario_ref: _Optional[_Union[VersionedReference, _Mapping]] = ..., demo_id: _Optional[str] = ..., scope_id: _Optional[str] = ..., run_id: _Optional[str] = ..., started_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., completed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., status: _Optional[str] = ..., investigation_ids: _Optional[_Iterable[str]] = ..., transaction_ids: _Optional[_Iterable[str]] = ..., receipt_refs: _Optional[_Iterable[str]] = ..., state_observation_refs: _Optional[_Iterable[str]] = ..., metric_refs: _Optional[_Iterable[str]] = ..., ksi_refs: _Optional[_Iterable[str]] = ..., assertion_refs: _Optional[_Iterable[_Union[VersionedReference, _Mapping]]] = ..., framework_control_refs: _Optional[_Iterable[_Union[FrameworkControlReference, _Mapping]]] = ..., step_results: _Optional[_Iterable[_Union[DemoStepResult, _Mapping]]] = ..., verification_status: _Optional[str] = ..., failure: _Optional[str] = ..., limitations: _Optional[_Iterable[str]] = ..., display_number: _Optional[str] = ..., title: _Optional[str] = ..., metrics_summary: _Optional[str] = ...) -> None: ...
