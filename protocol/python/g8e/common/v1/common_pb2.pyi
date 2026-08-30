import datetime

from google.protobuf import descriptor_pb2 as _descriptor_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Component(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    COMPONENT_UNSPECIFIED: _ClassVar[Component]
    COMPONENT_AGENT: _ClassVar[Component]
    COMPONENT_G8EO: _ClassVar[Component]
    COMPONENT_CLIENT: _ClassVar[Component]

class PlatformComponentKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    PLATFORM_COMPONENT_KIND_UNSPECIFIED: _ClassVar[PlatformComponentKind]
    PLATFORM_COMPONENT_KIND_DASHBOARD: _ClassVar[PlatformComponentKind]
    PLATFORM_COMPONENT_KIND_ENSEMBLE: _ClassVar[PlatformComponentKind]
    PLATFORM_COMPONENT_KIND_OPERATOR: _ClassVar[PlatformComponentKind]

class PlatformEnrollmentDecision(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    PLATFORM_ENROLLMENT_DECISION_UNSPECIFIED: _ClassVar[PlatformEnrollmentDecision]
    PLATFORM_ENROLLMENT_DECISION_APPROVE: _ClassVar[PlatformEnrollmentDecision]
    PLATFORM_ENROLLMENT_DECISION_DENY: _ClassVar[PlatformEnrollmentDecision]
COMPONENT_UNSPECIFIED: Component
COMPONENT_AGENT: Component
COMPONENT_G8EO: Component
COMPONENT_CLIENT: Component
PLATFORM_COMPONENT_KIND_UNSPECIFIED: PlatformComponentKind
PLATFORM_COMPONENT_KIND_DASHBOARD: PlatformComponentKind
PLATFORM_COMPONENT_KIND_ENSEMBLE: PlatformComponentKind
PLATFORM_COMPONENT_KIND_OPERATOR: PlatformComponentKind
PLATFORM_ENROLLMENT_DECISION_UNSPECIFIED: PlatformEnrollmentDecision
PLATFORM_ENROLLMENT_DECISION_APPROVE: PlatformEnrollmentDecision
PLATFORM_ENROLLMENT_DECISION_DENY: PlatformEnrollmentDecision
FORBIDDEN_PATTERNS_FIELD_NUMBER: _ClassVar[int]
forbidden_patterns: _descriptor.FieldDescriptor

class L1Metadata(_message.Message):
    __slots__ = ("validated", "violations")
    VALIDATED_FIELD_NUMBER: _ClassVar[int]
    VIOLATIONS_FIELD_NUMBER: _ClassVar[int]
    validated: bool
    violations: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, validated: _Optional[bool] = ..., violations: _Optional[_Iterable[str]] = ...) -> None: ...

class L2Vote(_message.Message):
    __slots__ = ("signer_key_id", "consensus_signature", "decision")
    SIGNER_KEY_ID_FIELD_NUMBER: _ClassVar[int]
    CONSENSUS_SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    DECISION_FIELD_NUMBER: _ClassVar[int]
    signer_key_id: str
    consensus_signature: str
    decision: bool
    def __init__(self, signer_key_id: _Optional[str] = ..., consensus_signature: _Optional[str] = ..., decision: _Optional[bool] = ...) -> None: ...

class L2Metadata(_message.Message):
    __slots__ = ("consensus_set_id", "votes")
    CONSENSUS_SET_ID_FIELD_NUMBER: _ClassVar[int]
    VOTES_FIELD_NUMBER: _ClassVar[int]
    consensus_set_id: str
    votes: _containers.RepeatedCompositeFieldContainer[L2Vote]
    def __init__(self, consensus_set_id: _Optional[str] = ..., votes: _Optional[_Iterable[_Union[L2Vote, _Mapping]]] = ...) -> None: ...

class L3Proof(_message.Message):
    __slots__ = ("client_data_json", "authenticator_data", "signature", "credential_id", "mtls_cert_fingerprint", "cli_signature")
    CLIENT_DATA_JSON_FIELD_NUMBER: _ClassVar[int]
    AUTHENTICATOR_DATA_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_ID_FIELD_NUMBER: _ClassVar[int]
    MTLS_CERT_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    CLI_SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    client_data_json: str
    authenticator_data: str
    signature: str
    credential_id: str
    mtls_cert_fingerprint: str
    cli_signature: str
    def __init__(self, client_data_json: _Optional[str] = ..., authenticator_data: _Optional[str] = ..., signature: _Optional[str] = ..., credential_id: _Optional[str] = ..., mtls_cert_fingerprint: _Optional[str] = ..., cli_signature: _Optional[str] = ...) -> None: ...

class L3Metadata(_message.Message):
    __slots__ = ("proof",)
    PROOF_FIELD_NUMBER: _ClassVar[int]
    proof: L3Proof
    def __init__(self, proof: _Optional[_Union[L3Proof, _Mapping]] = ...) -> None: ...

class GovernanceMetadata(_message.Message):
    __slots__ = ("l1", "l2", "l3")
    L1_FIELD_NUMBER: _ClassVar[int]
    L2_FIELD_NUMBER: _ClassVar[int]
    L3_FIELD_NUMBER: _ClassVar[int]
    l1: L1Metadata
    l2: L2Metadata
    l3: L3Metadata
    def __init__(self, l1: _Optional[_Union[L1Metadata, _Mapping]] = ..., l2: _Optional[_Union[L2Metadata, _Mapping]] = ..., l3: _Optional[_Union[L3Metadata, _Mapping]] = ...) -> None: ...

class GovernanceEnvelope(_message.Message):
    __slots__ = ("id", "timestamp", "expires_at", "source_component", "operator_id", "operator_session_id", "web_session_id", "cli_session_id", "requestor_user_id", "acting_app_id", "event_type", "payload", "intent_data", "action_type", "target_resource", "state_merkle_root", "nonce", "transaction_hash", "protocol_version", "governance", "case_id", "investigation_id", "task_id", "system_fingerprint", "tenant_id", "binding_persona", "posture")
    ID_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    SOURCE_COMPONENT_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    CLI_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    REQUESTOR_USER_ID_FIELD_NUMBER: _ClassVar[int]
    ACTING_APP_ID_FIELD_NUMBER: _ClassVar[int]
    EVENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    INTENT_DATA_FIELD_NUMBER: _ClassVar[int]
    ACTION_TYPE_FIELD_NUMBER: _ClassVar[int]
    TARGET_RESOURCE_FIELD_NUMBER: _ClassVar[int]
    STATE_MERKLE_ROOT_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_HASH_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_VERSION_FIELD_NUMBER: _ClassVar[int]
    GOVERNANCE_FIELD_NUMBER: _ClassVar[int]
    CASE_ID_FIELD_NUMBER: _ClassVar[int]
    INVESTIGATION_ID_FIELD_NUMBER: _ClassVar[int]
    TASK_ID_FIELD_NUMBER: _ClassVar[int]
    SYSTEM_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    BINDING_PERSONA_FIELD_NUMBER: _ClassVar[int]
    POSTURE_FIELD_NUMBER: _ClassVar[int]
    id: str
    timestamp: _timestamp_pb2.Timestamp
    expires_at: _timestamp_pb2.Timestamp
    source_component: Component
    operator_id: str
    operator_session_id: str
    web_session_id: str
    cli_session_id: str
    requestor_user_id: str
    acting_app_id: str
    event_type: str
    payload: bytes
    intent_data: _struct_pb2.Struct
    action_type: str
    target_resource: str
    state_merkle_root: str
    nonce: str
    transaction_hash: str
    protocol_version: str
    governance: GovernanceMetadata
    case_id: str
    investigation_id: str
    task_id: str
    system_fingerprint: str
    tenant_id: str
    binding_persona: str
    posture: str
    def __init__(self, id: _Optional[str] = ..., timestamp: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., source_component: _Optional[_Union[Component, str]] = ..., operator_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., web_session_id: _Optional[str] = ..., cli_session_id: _Optional[str] = ..., requestor_user_id: _Optional[str] = ..., acting_app_id: _Optional[str] = ..., event_type: _Optional[str] = ..., payload: _Optional[bytes] = ..., intent_data: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., action_type: _Optional[str] = ..., target_resource: _Optional[str] = ..., state_merkle_root: _Optional[str] = ..., nonce: _Optional[str] = ..., transaction_hash: _Optional[str] = ..., protocol_version: _Optional[str] = ..., governance: _Optional[_Union[GovernanceMetadata, _Mapping]] = ..., case_id: _Optional[str] = ..., investigation_id: _Optional[str] = ..., task_id: _Optional[str] = ..., system_fingerprint: _Optional[str] = ..., tenant_id: _Optional[str] = ..., binding_persona: _Optional[str] = ..., posture: _Optional[str] = ...) -> None: ...

class PlatformEnrollmentFingerprints(_message.Message):
    __slots__ = ("app", "operator", "cli")
    APP_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_FIELD_NUMBER: _ClassVar[int]
    CLI_FIELD_NUMBER: _ClassVar[int]
    app: str
    operator: str
    cli: str
    def __init__(self, app: _Optional[str] = ..., operator: _Optional[str] = ..., cli: _Optional[str] = ...) -> None: ...

class PlatformEnrollmentGovernancePayload(_message.Message):
    __slots__ = ("action", "intent", "request_id", "component_kind", "instance_id", "actor_user_id", "decision", "fingerprints", "target_collection", "target_document_id", "operator_id", "operator_session_id", "cli_session_id", "policy_id", "certificate_serial", "certificate_fingerprint", "owner_user_id")
    ACTION_FIELD_NUMBER: _ClassVar[int]
    INTENT_FIELD_NUMBER: _ClassVar[int]
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    COMPONENT_KIND_FIELD_NUMBER: _ClassVar[int]
    INSTANCE_ID_FIELD_NUMBER: _ClassVar[int]
    ACTOR_USER_ID_FIELD_NUMBER: _ClassVar[int]
    DECISION_FIELD_NUMBER: _ClassVar[int]
    FINGERPRINTS_FIELD_NUMBER: _ClassVar[int]
    TARGET_COLLECTION_FIELD_NUMBER: _ClassVar[int]
    TARGET_DOCUMENT_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    CLI_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    POLICY_ID_FIELD_NUMBER: _ClassVar[int]
    CERTIFICATE_SERIAL_FIELD_NUMBER: _ClassVar[int]
    CERTIFICATE_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    OWNER_USER_ID_FIELD_NUMBER: _ClassVar[int]
    action: str
    intent: str
    request_id: str
    component_kind: PlatformComponentKind
    instance_id: str
    actor_user_id: str
    decision: PlatformEnrollmentDecision
    fingerprints: PlatformEnrollmentFingerprints
    target_collection: str
    target_document_id: str
    operator_id: str
    operator_session_id: str
    cli_session_id: str
    policy_id: str
    certificate_serial: str
    certificate_fingerprint: str
    owner_user_id: str
    def __init__(self, action: _Optional[str] = ..., intent: _Optional[str] = ..., request_id: _Optional[str] = ..., component_kind: _Optional[_Union[PlatformComponentKind, str]] = ..., instance_id: _Optional[str] = ..., actor_user_id: _Optional[str] = ..., decision: _Optional[_Union[PlatformEnrollmentDecision, str]] = ..., fingerprints: _Optional[_Union[PlatformEnrollmentFingerprints, _Mapping]] = ..., target_collection: _Optional[str] = ..., target_document_id: _Optional[str] = ..., operator_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., cli_session_id: _Optional[str] = ..., policy_id: _Optional[str] = ..., certificate_serial: _Optional[str] = ..., certificate_fingerprint: _Optional[str] = ..., owner_user_id: _Optional[str] = ...) -> None: ...

class PlatformEnrollmentCompletionTranscript(_message.Message):
    __slots__ = ("protocol_version", "request_id", "token_hash", "component_kind", "instance_id", "fingerprints")
    PROTOCOL_VERSION_FIELD_NUMBER: _ClassVar[int]
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    TOKEN_HASH_FIELD_NUMBER: _ClassVar[int]
    COMPONENT_KIND_FIELD_NUMBER: _ClassVar[int]
    INSTANCE_ID_FIELD_NUMBER: _ClassVar[int]
    FINGERPRINTS_FIELD_NUMBER: _ClassVar[int]
    protocol_version: str
    request_id: str
    token_hash: str
    component_kind: PlatformComponentKind
    instance_id: str
    fingerprints: PlatformEnrollmentFingerprints
    def __init__(self, protocol_version: _Optional[str] = ..., request_id: _Optional[str] = ..., token_hash: _Optional[str] = ..., component_kind: _Optional[_Union[PlatformComponentKind, str]] = ..., instance_id: _Optional[str] = ..., fingerprints: _Optional[_Union[PlatformEnrollmentFingerprints, _Mapping]] = ...) -> None: ...

class CommandIntent(_message.Message):
    __slots__ = ("operator_id", "operator_session_id", "requestor_user_id", "event_type", "action_type", "target_resource", "payload", "case_id", "investigation_id", "task_id", "web_session_id", "cli_session_id")
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    REQUESTOR_USER_ID_FIELD_NUMBER: _ClassVar[int]
    EVENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    ACTION_TYPE_FIELD_NUMBER: _ClassVar[int]
    TARGET_RESOURCE_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_FIELD_NUMBER: _ClassVar[int]
    CASE_ID_FIELD_NUMBER: _ClassVar[int]
    INVESTIGATION_ID_FIELD_NUMBER: _ClassVar[int]
    TASK_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    CLI_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    operator_id: str
    operator_session_id: str
    requestor_user_id: str
    event_type: str
    action_type: str
    target_resource: str
    payload: bytes
    case_id: str
    investigation_id: str
    task_id: str
    web_session_id: str
    cli_session_id: str
    def __init__(self, operator_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., requestor_user_id: _Optional[str] = ..., event_type: _Optional[str] = ..., action_type: _Optional[str] = ..., target_resource: _Optional[str] = ..., payload: _Optional[bytes] = ..., case_id: _Optional[str] = ..., investigation_id: _Optional[str] = ..., task_id: _Optional[str] = ..., web_session_id: _Optional[str] = ..., cli_session_id: _Optional[str] = ...) -> None: ...
