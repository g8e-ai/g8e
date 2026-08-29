from g8e.common.v1 import common_pb2 as _common_pb2
from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ExecutionStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    EXECUTION_STATUS_UNSPECIFIED: _ClassVar[ExecutionStatus]
    EXECUTION_STATUS_EXECUTING: _ClassVar[ExecutionStatus]
    EXECUTION_STATUS_COMPLETED: _ClassVar[ExecutionStatus]
    EXECUTION_STATUS_FAILED: _ClassVar[ExecutionStatus]
    EXECUTION_STATUS_CANCELLED: _ClassVar[ExecutionStatus]
    EXECUTION_STATUS_TIMEOUT: _ClassVar[ExecutionStatus]

class L3Status(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    L3_STATUS_UNSPECIFIED: _ClassVar[L3Status]
    L3_STATUS_NOT_REQUIRED: _ClassVar[L3Status]
    L3_STATUS_REQUIRED_VALID: _ClassVar[L3Status]
    L3_STATUS_REQUIRED_FAILED: _ClassVar[L3Status]

class L2Status(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    L2_STATUS_UNSPECIFIED: _ClassVar[L2Status]
    L2_STATUS_NOT_REQUIRED: _ClassVar[L2Status]
    L2_STATUS_REQUIRED_VALID: _ClassVar[L2Status]
    L2_STATUS_REQUIRED_FAILED: _ClassVar[L2Status]

class HeartbeatType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    HEARTBEAT_TYPE_UNSPECIFIED: _ClassVar[HeartbeatType]
    HEARTBEAT_TYPE_AUTOMATIC: _ClassVar[HeartbeatType]
    HEARTBEAT_TYPE_MANUAL: _ClassVar[HeartbeatType]

class DeterministicStageKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    DETERMINISTIC_STAGE_KIND_UNSPECIFIED: _ClassVar[DeterministicStageKind]
    DETERMINISTIC_STAGE_KIND_L1_DOCTRINE: _ClassVar[DeterministicStageKind]
    DETERMINISTIC_STAGE_KIND_PROTOCOL_L2: _ClassVar[DeterministicStageKind]
    DETERMINISTIC_STAGE_KIND_L3_NOTARY: _ClassVar[DeterministicStageKind]
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION: _ClassVar[DeterministicStageKind]
    DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE: _ClassVar[DeterministicStageKind]
    DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND: _ClassVar[DeterministicStageKind]
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION: _ClassVar[DeterministicStageKind]

class DeterministicStageOutcome(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    DETERMINISTIC_STAGE_OUTCOME_UNSPECIFIED: _ClassVar[DeterministicStageOutcome]
    DETERMINISTIC_STAGE_OUTCOME_VERIFIED: _ClassVar[DeterministicStageOutcome]
    DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED: _ClassVar[DeterministicStageOutcome]
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED: _ClassVar[DeterministicStageOutcome]
    DETERMINISTIC_STAGE_OUTCOME_FAILED: _ClassVar[DeterministicStageOutcome]
EXECUTION_STATUS_UNSPECIFIED: ExecutionStatus
EXECUTION_STATUS_EXECUTING: ExecutionStatus
EXECUTION_STATUS_COMPLETED: ExecutionStatus
EXECUTION_STATUS_FAILED: ExecutionStatus
EXECUTION_STATUS_CANCELLED: ExecutionStatus
EXECUTION_STATUS_TIMEOUT: ExecutionStatus
L3_STATUS_UNSPECIFIED: L3Status
L3_STATUS_NOT_REQUIRED: L3Status
L3_STATUS_REQUIRED_VALID: L3Status
L3_STATUS_REQUIRED_FAILED: L3Status
L2_STATUS_UNSPECIFIED: L2Status
L2_STATUS_NOT_REQUIRED: L2Status
L2_STATUS_REQUIRED_VALID: L2Status
L2_STATUS_REQUIRED_FAILED: L2Status
HEARTBEAT_TYPE_UNSPECIFIED: HeartbeatType
HEARTBEAT_TYPE_AUTOMATIC: HeartbeatType
HEARTBEAT_TYPE_MANUAL: HeartbeatType
DETERMINISTIC_STAGE_KIND_UNSPECIFIED: DeterministicStageKind
DETERMINISTIC_STAGE_KIND_L1_DOCTRINE: DeterministicStageKind
DETERMINISTIC_STAGE_KIND_PROTOCOL_L2: DeterministicStageKind
DETERMINISTIC_STAGE_KIND_L3_NOTARY: DeterministicStageKind
DETERMINISTIC_STAGE_KIND_L4_VERIFICATION: DeterministicStageKind
DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE: DeterministicStageKind
DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND: DeterministicStageKind
DETERMINISTIC_STAGE_KIND_L5_EXECUTION: DeterministicStageKind
DETERMINISTIC_STAGE_OUTCOME_UNSPECIFIED: DeterministicStageOutcome
DETERMINISTIC_STAGE_OUTCOME_VERIFIED: DeterministicStageOutcome
DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED: DeterministicStageOutcome
DETERMINISTIC_STAGE_OUTCOME_COMPLETED: DeterministicStageOutcome
DETERMINISTIC_STAGE_OUTCOME_FAILED: DeterministicStageOutcome

class CommandRequested(_message.Message):
    __slots__ = ("command", "execution_id", "justification", "vault_mode", "timeout_seconds", "intent", "environment", "working_directory")
    class EnvironmentEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    JUSTIFICATION_FIELD_NUMBER: _ClassVar[int]
    VAULT_MODE_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_SECONDS_FIELD_NUMBER: _ClassVar[int]
    INTENT_FIELD_NUMBER: _ClassVar[int]
    ENVIRONMENT_FIELD_NUMBER: _ClassVar[int]
    WORKING_DIRECTORY_FIELD_NUMBER: _ClassVar[int]
    command: str
    execution_id: str
    justification: str
    vault_mode: str
    timeout_seconds: int
    intent: str
    environment: _containers.ScalarMap[str, str]
    working_directory: str
    def __init__(self, command: _Optional[str] = ..., execution_id: _Optional[str] = ..., justification: _Optional[str] = ..., vault_mode: _Optional[str] = ..., timeout_seconds: _Optional[int] = ..., intent: _Optional[str] = ..., environment: _Optional[_Mapping[str, str]] = ..., working_directory: _Optional[str] = ...) -> None: ...

class CommandCancelRequested(_message.Message):
    __slots__ = ("execution_id",)
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    def __init__(self, execution_id: _Optional[str] = ...) -> None: ...

class FileEditRequested(_message.Message):
    __slots__ = ("file_path", "operation", "execution_id", "justification", "content", "old_content", "new_content", "insert_content", "insert_position", "start_line", "end_line", "patch_content", "create_backup", "create_if_missing", "vault_mode")
    FILE_PATH_FIELD_NUMBER: _ClassVar[int]
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    JUSTIFICATION_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    OLD_CONTENT_FIELD_NUMBER: _ClassVar[int]
    NEW_CONTENT_FIELD_NUMBER: _ClassVar[int]
    INSERT_CONTENT_FIELD_NUMBER: _ClassVar[int]
    INSERT_POSITION_FIELD_NUMBER: _ClassVar[int]
    START_LINE_FIELD_NUMBER: _ClassVar[int]
    END_LINE_FIELD_NUMBER: _ClassVar[int]
    PATCH_CONTENT_FIELD_NUMBER: _ClassVar[int]
    CREATE_BACKUP_FIELD_NUMBER: _ClassVar[int]
    CREATE_IF_MISSING_FIELD_NUMBER: _ClassVar[int]
    VAULT_MODE_FIELD_NUMBER: _ClassVar[int]
    file_path: str
    operation: str
    execution_id: str
    justification: str
    content: str
    old_content: str
    new_content: str
    insert_content: str
    insert_position: int
    start_line: int
    end_line: int
    patch_content: str
    create_backup: bool
    create_if_missing: bool
    vault_mode: str
    def __init__(self, file_path: _Optional[str] = ..., operation: _Optional[str] = ..., execution_id: _Optional[str] = ..., justification: _Optional[str] = ..., content: _Optional[str] = ..., old_content: _Optional[str] = ..., new_content: _Optional[str] = ..., insert_content: _Optional[str] = ..., insert_position: _Optional[int] = ..., start_line: _Optional[int] = ..., end_line: _Optional[int] = ..., patch_content: _Optional[str] = ..., create_backup: _Optional[bool] = ..., create_if_missing: _Optional[bool] = ..., vault_mode: _Optional[str] = ...) -> None: ...

class FsListRequested(_message.Message):
    __slots__ = ("path", "execution_id", "max_depth", "max_entries", "vault_mode")
    PATH_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    MAX_DEPTH_FIELD_NUMBER: _ClassVar[int]
    MAX_ENTRIES_FIELD_NUMBER: _ClassVar[int]
    VAULT_MODE_FIELD_NUMBER: _ClassVar[int]
    path: str
    execution_id: str
    max_depth: int
    max_entries: int
    vault_mode: str
    def __init__(self, path: _Optional[str] = ..., execution_id: _Optional[str] = ..., max_depth: _Optional[int] = ..., max_entries: _Optional[int] = ..., vault_mode: _Optional[str] = ...) -> None: ...

class FsReadRequested(_message.Message):
    __slots__ = ("path", "execution_id", "max_size", "vault_mode")
    PATH_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    MAX_SIZE_FIELD_NUMBER: _ClassVar[int]
    VAULT_MODE_FIELD_NUMBER: _ClassVar[int]
    path: str
    execution_id: str
    max_size: int
    vault_mode: str
    def __init__(self, path: _Optional[str] = ..., execution_id: _Optional[str] = ..., max_size: _Optional[int] = ..., vault_mode: _Optional[str] = ...) -> None: ...

class HeartbeatRequested(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class FsGrepRequested(_message.Message):
    __slots__ = ("path", "execution_id", "pattern", "includes", "max_matches", "vault_mode")
    PATH_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    PATTERN_FIELD_NUMBER: _ClassVar[int]
    INCLUDES_FIELD_NUMBER: _ClassVar[int]
    MAX_MATCHES_FIELD_NUMBER: _ClassVar[int]
    VAULT_MODE_FIELD_NUMBER: _ClassVar[int]
    path: str
    execution_id: str
    pattern: str
    includes: _containers.RepeatedScalarFieldContainer[str]
    max_matches: int
    vault_mode: str
    def __init__(self, path: _Optional[str] = ..., execution_id: _Optional[str] = ..., pattern: _Optional[str] = ..., includes: _Optional[_Iterable[str]] = ..., max_matches: _Optional[int] = ..., vault_mode: _Optional[str] = ...) -> None: ...

class CheckPortRequested(_message.Message):
    __slots__ = ("execution_id", "port", "host", "protocol")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    PORT_FIELD_NUMBER: _ClassVar[int]
    HOST_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    port: int
    host: str
    protocol: str
    def __init__(self, execution_id: _Optional[str] = ..., port: _Optional[int] = ..., host: _Optional[str] = ..., protocol: _Optional[str] = ...) -> None: ...

class FetchLogsRequested(_message.Message):
    __slots__ = ("execution_id", "vault_mode")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    VAULT_MODE_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    vault_mode: str
    def __init__(self, execution_id: _Optional[str] = ..., vault_mode: _Optional[str] = ...) -> None: ...

class FetchHistoryRequested(_message.Message):
    __slots__ = ("execution_id", "operator_session_id", "limit", "offset", "include_commands", "include_file_mutations")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    OFFSET_FIELD_NUMBER: _ClassVar[int]
    INCLUDE_COMMANDS_FIELD_NUMBER: _ClassVar[int]
    INCLUDE_FILE_MUTATIONS_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    operator_session_id: str
    limit: int
    offset: int
    include_commands: bool
    include_file_mutations: bool
    def __init__(self, execution_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., limit: _Optional[int] = ..., offset: _Optional[int] = ..., include_commands: _Optional[bool] = ..., include_file_mutations: _Optional[bool] = ...) -> None: ...

class FetchFileHistoryRequested(_message.Message):
    __slots__ = ("execution_id", "file_path", "limit", "operator_session_id")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    FILE_PATH_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    file_path: str
    limit: int
    operator_session_id: str
    def __init__(self, execution_id: _Optional[str] = ..., file_path: _Optional[str] = ..., limit: _Optional[int] = ..., operator_session_id: _Optional[str] = ...) -> None: ...

class FetchFileDiffRequested(_message.Message):
    __slots__ = ("execution_id", "diff_id", "operator_session_id", "file_path", "limit")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    DIFF_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    FILE_PATH_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    diff_id: str
    operator_session_id: str
    file_path: str
    limit: int
    def __init__(self, execution_id: _Optional[str] = ..., diff_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., file_path: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class RestoreFileRequested(_message.Message):
    __slots__ = ("execution_id", "file_path", "commit_hash", "operator_session_id")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    FILE_PATH_FIELD_NUMBER: _ClassVar[int]
    COMMIT_HASH_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    file_path: str
    commit_hash: str
    operator_session_id: str
    def __init__(self, execution_id: _Optional[str] = ..., file_path: _Optional[str] = ..., commit_hash: _Optional[str] = ..., operator_session_id: _Optional[str] = ...) -> None: ...

class DirectCommandAuditRequested(_message.Message):
    __slots__ = ("command", "execution_id", "operator_session_id", "type")
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    command: str
    execution_id: str
    operator_session_id: str
    type: str
    def __init__(self, command: _Optional[str] = ..., execution_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., type: _Optional[str] = ...) -> None: ...

class DirectCommandResultAuditRequested(_message.Message):
    __slots__ = ("command", "execution_id", "output", "stderr", "exit_code", "execution_time_seconds")
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_FIELD_NUMBER: _ClassVar[int]
    STDERR_FIELD_NUMBER: _ClassVar[int]
    EXIT_CODE_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_TIME_SECONDS_FIELD_NUMBER: _ClassVar[int]
    command: str
    execution_id: str
    output: str
    stderr: str
    exit_code: int
    execution_time_seconds: float
    def __init__(self, command: _Optional[str] = ..., execution_id: _Optional[str] = ..., output: _Optional[str] = ..., stderr: _Optional[str] = ..., exit_code: _Optional[int] = ..., execution_time_seconds: _Optional[float] = ...) -> None: ...

class AuditMsgRequested(_message.Message):
    __slots__ = ("content",)
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    content: str
    def __init__(self, content: _Optional[str] = ...) -> None: ...

class DocumentUpdateRequested(_message.Message):
    __slots__ = ("collection", "document_id", "updates", "merge")
    COLLECTION_FIELD_NUMBER: _ClassVar[int]
    DOCUMENT_ID_FIELD_NUMBER: _ClassVar[int]
    UPDATES_FIELD_NUMBER: _ClassVar[int]
    MERGE_FIELD_NUMBER: _ClassVar[int]
    collection: str
    document_id: str
    updates: _struct_pb2.Struct
    merge: bool
    def __init__(self, collection: _Optional[str] = ..., document_id: _Optional[str] = ..., updates: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., merge: _Optional[bool] = ...) -> None: ...

class DocumentDeleteRequested(_message.Message):
    __slots__ = ("collection", "document_id")
    COLLECTION_FIELD_NUMBER: _ClassVar[int]
    DOCUMENT_ID_FIELD_NUMBER: _ClassVar[int]
    collection: str
    document_id: str
    def __init__(self, collection: _Optional[str] = ..., document_id: _Optional[str] = ...) -> None: ...

class SignCertificateRequested(_message.Message):
    __slots__ = ("public_key_pem", "common_name", "organizational_unit", "validity_days")
    PUBLIC_KEY_PEM_FIELD_NUMBER: _ClassVar[int]
    COMMON_NAME_FIELD_NUMBER: _ClassVar[int]
    ORGANIZATIONAL_UNIT_FIELD_NUMBER: _ClassVar[int]
    VALIDITY_DAYS_FIELD_NUMBER: _ClassVar[int]
    public_key_pem: str
    common_name: str
    organizational_unit: str
    validity_days: int
    def __init__(self, public_key_pem: _Optional[str] = ..., common_name: _Optional[str] = ..., organizational_unit: _Optional[str] = ..., validity_days: _Optional[int] = ...) -> None: ...

class SignCertificateResult(_message.Message):
    __slots__ = ("success", "certificate_pem", "serial", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    CERTIFICATE_PEM_FIELD_NUMBER: _ClassVar[int]
    SERIAL_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    certificate_pem: str
    serial: str
    error: str
    def __init__(self, success: _Optional[bool] = ..., certificate_pem: _Optional[str] = ..., serial: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class RevokeCertificateRequested(_message.Message):
    __slots__ = ("serial", "reason")
    SERIAL_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    serial: str
    reason: str
    def __init__(self, serial: _Optional[str] = ..., reason: _Optional[str] = ...) -> None: ...

class RevokeCertificateResult(_message.Message):
    __slots__ = ("success", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    def __init__(self, success: _Optional[bool] = ..., error: _Optional[str] = ...) -> None: ...

class GetRevocationBundleRequested(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class GetRevocationBundleResult(_message.Message):
    __slots__ = ("success", "bundle_json", "signature", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_JSON_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    bundle_json: str
    signature: str
    error: str
    def __init__(self, success: _Optional[bool] = ..., bundle_json: _Optional[str] = ..., signature: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class CreateDeviceLinkRequested(_message.Message):
    __slots__ = ("user_id", "organization_id", "operator_id", "web_session_id", "name", "max_uses", "ttl_seconds")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ORGANIZATION_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    MAX_USES_FIELD_NUMBER: _ClassVar[int]
    TTL_SECONDS_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    organization_id: str
    operator_id: str
    web_session_id: str
    name: str
    max_uses: int
    ttl_seconds: int
    def __init__(self, user_id: _Optional[str] = ..., organization_id: _Optional[str] = ..., operator_id: _Optional[str] = ..., web_session_id: _Optional[str] = ..., name: _Optional[str] = ..., max_uses: _Optional[int] = ..., ttl_seconds: _Optional[int] = ...) -> None: ...

class DeviceLink(_message.Message):
    __slots__ = ("token", "user_id", "organization_id", "operator_id", "web_session_id", "name", "max_uses", "uses", "status", "created_at_unix_ms", "expires_at_unix_ms")
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ORGANIZATION_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    MAX_USES_FIELD_NUMBER: _ClassVar[int]
    USES_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    token: str
    user_id: str
    organization_id: str
    operator_id: str
    web_session_id: str
    name: str
    max_uses: int
    uses: int
    status: str
    created_at_unix_ms: int
    expires_at_unix_ms: int
    def __init__(self, token: _Optional[str] = ..., user_id: _Optional[str] = ..., organization_id: _Optional[str] = ..., operator_id: _Optional[str] = ..., web_session_id: _Optional[str] = ..., name: _Optional[str] = ..., max_uses: _Optional[int] = ..., uses: _Optional[int] = ..., status: _Optional[str] = ..., created_at_unix_ms: _Optional[int] = ..., expires_at_unix_ms: _Optional[int] = ...) -> None: ...

class DeviceLinkResult(_message.Message):
    __slots__ = ("success", "link", "operator_command", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    LINK_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_COMMAND_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    link: DeviceLink
    operator_command: str
    error: str
    def __init__(self, success: _Optional[bool] = ..., link: _Optional[_Union[DeviceLink, _Mapping]] = ..., operator_command: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class ListDeviceLinksRequested(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class ListDeviceLinksResult(_message.Message):
    __slots__ = ("success", "links", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    LINKS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    links: _containers.RepeatedCompositeFieldContainer[DeviceLink]
    error: str
    def __init__(self, success: _Optional[bool] = ..., links: _Optional[_Iterable[_Union[DeviceLink, _Mapping]]] = ..., error: _Optional[str] = ...) -> None: ...

class DeleteDeviceLinkRequested(_message.Message):
    __slots__ = ("token", "user_id")
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    token: str
    user_id: str
    def __init__(self, token: _Optional[str] = ..., user_id: _Optional[str] = ...) -> None: ...

class TerminateOperatorRequested(_message.Message):
    __slots__ = ("operator_id", "user_id", "reason")
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    operator_id: str
    user_id: str
    reason: str
    def __init__(self, operator_id: _Optional[str] = ..., user_id: _Optional[str] = ..., reason: _Optional[str] = ...) -> None: ...

class TerminateOperatorResult(_message.Message):
    __slots__ = ("success", "message", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    message: str
    error: str
    def __init__(self, success: _Optional[bool] = ..., message: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class ListOperatorSlotsRequested(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class ListOperatorSlotsResult(_message.Message):
    __slots__ = ("success", "operators", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    OPERATORS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    operators: _containers.RepeatedCompositeFieldContainer[OperatorDocument]
    error: str
    def __init__(self, success: _Optional[bool] = ..., operators: _Optional[_Iterable[_Union[OperatorDocument, _Mapping]]] = ..., error: _Optional[str] = ...) -> None: ...

class BindOperatorsRequested(_message.Message):
    __slots__ = ("operator_ids", "user_id", "web_session_id")
    OPERATOR_IDS_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    operator_ids: _containers.RepeatedScalarFieldContainer[str]
    user_id: str
    web_session_id: str
    def __init__(self, operator_ids: _Optional[_Iterable[str]] = ..., user_id: _Optional[str] = ..., web_session_id: _Optional[str] = ...) -> None: ...

class BindOperatorsResult(_message.Message):
    __slots__ = ("success", "bound_count", "failed_count", "bound_operator_ids", "failed_operator_ids", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    BOUND_COUNT_FIELD_NUMBER: _ClassVar[int]
    FAILED_COUNT_FIELD_NUMBER: _ClassVar[int]
    BOUND_OPERATOR_IDS_FIELD_NUMBER: _ClassVar[int]
    FAILED_OPERATOR_IDS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    bound_count: int
    failed_count: int
    bound_operator_ids: _containers.RepeatedScalarFieldContainer[str]
    failed_operator_ids: _containers.RepeatedScalarFieldContainer[str]
    error: str
    def __init__(self, success: _Optional[bool] = ..., bound_count: _Optional[int] = ..., failed_count: _Optional[int] = ..., bound_operator_ids: _Optional[_Iterable[str]] = ..., failed_operator_ids: _Optional[_Iterable[str]] = ..., error: _Optional[str] = ...) -> None: ...

class UnbindOperatorsRequested(_message.Message):
    __slots__ = ("operator_ids", "user_id", "web_session_id")
    OPERATOR_IDS_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    operator_ids: _containers.RepeatedScalarFieldContainer[str]
    user_id: str
    web_session_id: str
    def __init__(self, operator_ids: _Optional[_Iterable[str]] = ..., user_id: _Optional[str] = ..., web_session_id: _Optional[str] = ...) -> None: ...

class UnbindOperatorsResult(_message.Message):
    __slots__ = ("success", "unbound_count", "failed_count", "unbound_operator_ids", "failed_operator_ids", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    UNBOUND_COUNT_FIELD_NUMBER: _ClassVar[int]
    FAILED_COUNT_FIELD_NUMBER: _ClassVar[int]
    UNBOUND_OPERATOR_IDS_FIELD_NUMBER: _ClassVar[int]
    FAILED_OPERATOR_IDS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    unbound_count: int
    failed_count: int
    unbound_operator_ids: _containers.RepeatedScalarFieldContainer[str]
    failed_operator_ids: _containers.RepeatedScalarFieldContainer[str]
    error: str
    def __init__(self, success: _Optional[bool] = ..., unbound_count: _Optional[int] = ..., failed_count: _Optional[int] = ..., unbound_operator_ids: _Optional[_Iterable[str]] = ..., failed_operator_ids: _Optional[_Iterable[str]] = ..., error: _Optional[str] = ...) -> None: ...

class SetTargetContextRequested(_message.Message):
    __slots__ = ("operator_id", "user_id", "web_session_id")
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    operator_id: str
    user_id: str
    web_session_id: str
    def __init__(self, operator_id: _Optional[str] = ..., user_id: _Optional[str] = ..., web_session_id: _Optional[str] = ...) -> None: ...

class SetTargetContextResult(_message.Message):
    __slots__ = ("success", "operator_id", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    operator_id: str
    error: str
    def __init__(self, success: _Optional[bool] = ..., operator_id: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class OperatorDocument(_message.Message):
    __slots__ = ("id", "user_id", "organization_id", "component", "name", "status", "operator_session_id", "bound_web_session_id", "operator_cert", "operator_cert_serial", "slot_number", "is_slot", "claimed", "operator_type", "cloud_subtype", "system_fingerprint", "created_at_unix_ms", "updated_at_unix_ms")
    ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ORGANIZATION_ID_FIELD_NUMBER: _ClassVar[int]
    COMPONENT_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    BOUND_WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_CERT_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_CERT_SERIAL_FIELD_NUMBER: _ClassVar[int]
    SLOT_NUMBER_FIELD_NUMBER: _ClassVar[int]
    IS_SLOT_FIELD_NUMBER: _ClassVar[int]
    CLAIMED_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_TYPE_FIELD_NUMBER: _ClassVar[int]
    CLOUD_SUBTYPE_FIELD_NUMBER: _ClassVar[int]
    SYSTEM_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    id: str
    user_id: str
    organization_id: str
    component: str
    name: str
    status: str
    operator_session_id: str
    bound_web_session_id: str
    operator_cert: str
    operator_cert_serial: str
    slot_number: int
    is_slot: bool
    claimed: bool
    operator_type: str
    cloud_subtype: str
    system_fingerprint: str
    created_at_unix_ms: int
    updated_at_unix_ms: int
    def __init__(self, id: _Optional[str] = ..., user_id: _Optional[str] = ..., organization_id: _Optional[str] = ..., component: _Optional[str] = ..., name: _Optional[str] = ..., status: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., bound_web_session_id: _Optional[str] = ..., operator_cert: _Optional[str] = ..., operator_cert_serial: _Optional[str] = ..., slot_number: _Optional[int] = ..., is_slot: _Optional[bool] = ..., claimed: _Optional[bool] = ..., operator_type: _Optional[str] = ..., cloud_subtype: _Optional[str] = ..., system_fingerprint: _Optional[str] = ..., created_at_unix_ms: _Optional[int] = ..., updated_at_unix_ms: _Optional[int] = ...) -> None: ...

class ShutdownRequested(_message.Message):
    __slots__ = ("reason",)
    REASON_FIELD_NUMBER: _ClassVar[int]
    reason: str
    def __init__(self, reason: _Optional[str] = ...) -> None: ...

class EvalAnswerRequested(_message.Message):
    __slots__ = ("prompt_id", "benchmark", "answer", "model")
    PROMPT_ID_FIELD_NUMBER: _ClassVar[int]
    BENCHMARK_FIELD_NUMBER: _ClassVar[int]
    ANSWER_FIELD_NUMBER: _ClassVar[int]
    MODEL_FIELD_NUMBER: _ClassVar[int]
    prompt_id: str
    benchmark: str
    answer: str
    model: str
    def __init__(self, prompt_id: _Optional[str] = ..., benchmark: _Optional[str] = ..., answer: _Optional[str] = ..., model: _Optional[str] = ...) -> None: ...

class McpCallRequested(_message.Message):
    __slots__ = ("tool_name", "arguments_json", "execution_id")
    TOOL_NAME_FIELD_NUMBER: _ClassVar[int]
    ARGUMENTS_JSON_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    tool_name: str
    arguments_json: str
    execution_id: str
    def __init__(self, tool_name: _Optional[str] = ..., arguments_json: _Optional[str] = ..., execution_id: _Optional[str] = ...) -> None: ...

class A2aCallRequested(_message.Message):
    __slots__ = ("skill_name", "payload_json", "execution_id")
    SKILL_NAME_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_JSON_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    skill_name: str
    payload_json: str
    execution_id: str
    def __init__(self, skill_name: _Optional[str] = ..., payload_json: _Optional[str] = ..., execution_id: _Optional[str] = ...) -> None: ...

class McpResourceListRequested(_message.Message):
    __slots__ = ("execution_id",)
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    def __init__(self, execution_id: _Optional[str] = ...) -> None: ...

class McpResourceReadRequested(_message.Message):
    __slots__ = ("uri", "execution_id")
    URI_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    uri: str
    execution_id: str
    def __init__(self, uri: _Optional[str] = ..., execution_id: _Optional[str] = ...) -> None: ...

class McpPromptListRequested(_message.Message):
    __slots__ = ("execution_id",)
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    def __init__(self, execution_id: _Optional[str] = ...) -> None: ...

class McpPromptGetRequested(_message.Message):
    __slots__ = ("name", "execution_id")
    NAME_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    name: str
    execution_id: str
    def __init__(self, name: _Optional[str] = ..., execution_id: _Optional[str] = ...) -> None: ...

class DeterministicStageEvidence(_message.Message):
    __slots__ = ("stage_id", "kind", "monotonic_start_ns", "monotonic_end_ns", "clock_domain", "timing_source", "outcome", "transaction_id", "transaction_hash", "action_type", "operator_id", "operator_session_id", "requestor_user_id", "acting_app_id", "case_id", "investigation_id", "task_id", "state_root_before", "state_root_after", "signer_key_id", "receipt_signature_digest", "commitment_hash", "prior_commitment_hash", "l2_signature_digest", "l3_signature_digest", "audit_record_id", "parent_stage_id")
    STAGE_ID_FIELD_NUMBER: _ClassVar[int]
    KIND_FIELD_NUMBER: _ClassVar[int]
    MONOTONIC_START_NS_FIELD_NUMBER: _ClassVar[int]
    MONOTONIC_END_NS_FIELD_NUMBER: _ClassVar[int]
    CLOCK_DOMAIN_FIELD_NUMBER: _ClassVar[int]
    TIMING_SOURCE_FIELD_NUMBER: _ClassVar[int]
    OUTCOME_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_ID_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_HASH_FIELD_NUMBER: _ClassVar[int]
    ACTION_TYPE_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    REQUESTOR_USER_ID_FIELD_NUMBER: _ClassVar[int]
    ACTING_APP_ID_FIELD_NUMBER: _ClassVar[int]
    CASE_ID_FIELD_NUMBER: _ClassVar[int]
    INVESTIGATION_ID_FIELD_NUMBER: _ClassVar[int]
    TASK_ID_FIELD_NUMBER: _ClassVar[int]
    STATE_ROOT_BEFORE_FIELD_NUMBER: _ClassVar[int]
    STATE_ROOT_AFTER_FIELD_NUMBER: _ClassVar[int]
    SIGNER_KEY_ID_FIELD_NUMBER: _ClassVar[int]
    RECEIPT_SIGNATURE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    COMMITMENT_HASH_FIELD_NUMBER: _ClassVar[int]
    PRIOR_COMMITMENT_HASH_FIELD_NUMBER: _ClassVar[int]
    L2_SIGNATURE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    L3_SIGNATURE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    AUDIT_RECORD_ID_FIELD_NUMBER: _ClassVar[int]
    PARENT_STAGE_ID_FIELD_NUMBER: _ClassVar[int]
    stage_id: str
    kind: DeterministicStageKind
    monotonic_start_ns: int
    monotonic_end_ns: int
    clock_domain: str
    timing_source: str
    outcome: DeterministicStageOutcome
    transaction_id: str
    transaction_hash: str
    action_type: str
    operator_id: str
    operator_session_id: str
    requestor_user_id: str
    acting_app_id: str
    case_id: str
    investigation_id: str
    task_id: str
    state_root_before: str
    state_root_after: str
    signer_key_id: str
    receipt_signature_digest: str
    commitment_hash: str
    prior_commitment_hash: str
    l2_signature_digest: str
    l3_signature_digest: str
    audit_record_id: str
    parent_stage_id: str
    def __init__(self, stage_id: _Optional[str] = ..., kind: _Optional[_Union[DeterministicStageKind, str]] = ..., monotonic_start_ns: _Optional[int] = ..., monotonic_end_ns: _Optional[int] = ..., clock_domain: _Optional[str] = ..., timing_source: _Optional[str] = ..., outcome: _Optional[_Union[DeterministicStageOutcome, str]] = ..., transaction_id: _Optional[str] = ..., transaction_hash: _Optional[str] = ..., action_type: _Optional[str] = ..., operator_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., requestor_user_id: _Optional[str] = ..., acting_app_id: _Optional[str] = ..., case_id: _Optional[str] = ..., investigation_id: _Optional[str] = ..., task_id: _Optional[str] = ..., state_root_before: _Optional[str] = ..., state_root_after: _Optional[str] = ..., signer_key_id: _Optional[str] = ..., receipt_signature_digest: _Optional[str] = ..., commitment_hash: _Optional[str] = ..., prior_commitment_hash: _Optional[str] = ..., l2_signature_digest: _Optional[str] = ..., l3_signature_digest: _Optional[str] = ..., audit_record_id: _Optional[str] = ..., parent_stage_id: _Optional[str] = ...) -> None: ...

class ActionReceipt(_message.Message):
    __slots__ = ("transaction_id", "transaction_hash", "status", "result_summary", "state_root_before", "state_root_after", "executed_at_unix_ms", "signer_key_id", "signature", "l2_status", "l3_status", "deterministic_stage_evidence")
    TRANSACTION_ID_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_HASH_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    RESULT_SUMMARY_FIELD_NUMBER: _ClassVar[int]
    STATE_ROOT_BEFORE_FIELD_NUMBER: _ClassVar[int]
    STATE_ROOT_AFTER_FIELD_NUMBER: _ClassVar[int]
    EXECUTED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    SIGNER_KEY_ID_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    L2_STATUS_FIELD_NUMBER: _ClassVar[int]
    L3_STATUS_FIELD_NUMBER: _ClassVar[int]
    DETERMINISTIC_STAGE_EVIDENCE_FIELD_NUMBER: _ClassVar[int]
    transaction_id: str
    transaction_hash: str
    status: ExecutionStatus
    result_summary: str
    state_root_before: str
    state_root_after: str
    executed_at_unix_ms: int
    signer_key_id: str
    signature: str
    l2_status: L2Status
    l3_status: L3Status
    deterministic_stage_evidence: _containers.RepeatedCompositeFieldContainer[DeterministicStageEvidence]
    def __init__(self, transaction_id: _Optional[str] = ..., transaction_hash: _Optional[str] = ..., status: _Optional[_Union[ExecutionStatus, str]] = ..., result_summary: _Optional[str] = ..., state_root_before: _Optional[str] = ..., state_root_after: _Optional[str] = ..., executed_at_unix_ms: _Optional[int] = ..., signer_key_id: _Optional[str] = ..., signature: _Optional[str] = ..., l2_status: _Optional[_Union[L2Status, str]] = ..., l3_status: _Optional[_Union[L3Status, str]] = ..., deterministic_stage_evidence: _Optional[_Iterable[_Union[DeterministicStageEvidence, _Mapping]]] = ...) -> None: ...

class CommitmentAttestation(_message.Message):
    __slots__ = ("transaction_id", "transaction_hash", "prior_commitment_hash", "state_root_at_commit", "l2_signature_digest", "warden_intent_signature_digest", "human_signature_digest", "action_type", "target_resource", "committed_at_unix_ms", "auditor_key_id", "signature", "hash")
    TRANSACTION_ID_FIELD_NUMBER: _ClassVar[int]
    TRANSACTION_HASH_FIELD_NUMBER: _ClassVar[int]
    PRIOR_COMMITMENT_HASH_FIELD_NUMBER: _ClassVar[int]
    STATE_ROOT_AT_COMMIT_FIELD_NUMBER: _ClassVar[int]
    L2_SIGNATURE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    WARDEN_INTENT_SIGNATURE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    HUMAN_SIGNATURE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    ACTION_TYPE_FIELD_NUMBER: _ClassVar[int]
    TARGET_RESOURCE_FIELD_NUMBER: _ClassVar[int]
    COMMITTED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    AUDITOR_KEY_ID_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    HASH_FIELD_NUMBER: _ClassVar[int]
    transaction_id: str
    transaction_hash: str
    prior_commitment_hash: str
    state_root_at_commit: str
    l2_signature_digest: str
    warden_intent_signature_digest: str
    human_signature_digest: str
    action_type: str
    target_resource: str
    committed_at_unix_ms: int
    auditor_key_id: str
    signature: str
    hash: str
    def __init__(self, transaction_id: _Optional[str] = ..., transaction_hash: _Optional[str] = ..., prior_commitment_hash: _Optional[str] = ..., state_root_at_commit: _Optional[str] = ..., l2_signature_digest: _Optional[str] = ..., warden_intent_signature_digest: _Optional[str] = ..., human_signature_digest: _Optional[str] = ..., action_type: _Optional[str] = ..., target_resource: _Optional[str] = ..., committed_at_unix_ms: _Optional[int] = ..., auditor_key_id: _Optional[str] = ..., signature: _Optional[str] = ..., hash: _Optional[str] = ...) -> None: ...

class CommandResult(_message.Message):
    __slots__ = ("execution_id", "status", "stdout", "error", "stderr", "return_code", "execution_time_seconds", "start_time_unix_ms", "end_time_unix_ms")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    STDOUT_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    STDERR_FIELD_NUMBER: _ClassVar[int]
    RETURN_CODE_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_TIME_SECONDS_FIELD_NUMBER: _ClassVar[int]
    START_TIME_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    END_TIME_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    status: ExecutionStatus
    stdout: str
    error: str
    stderr: str
    return_code: int
    execution_time_seconds: float
    start_time_unix_ms: int
    end_time_unix_ms: int
    def __init__(self, execution_id: _Optional[str] = ..., status: _Optional[_Union[ExecutionStatus, str]] = ..., stdout: _Optional[str] = ..., error: _Optional[str] = ..., stderr: _Optional[str] = ..., return_code: _Optional[int] = ..., execution_time_seconds: _Optional[float] = ..., start_time_unix_ms: _Optional[int] = ..., end_time_unix_ms: _Optional[int] = ...) -> None: ...

class FsEntry(_message.Message):
    __slots__ = ("name", "is_dir", "size", "mode", "mod_time")
    NAME_FIELD_NUMBER: _ClassVar[int]
    IS_DIR_FIELD_NUMBER: _ClassVar[int]
    SIZE_FIELD_NUMBER: _ClassVar[int]
    MODE_FIELD_NUMBER: _ClassVar[int]
    MOD_TIME_FIELD_NUMBER: _ClassVar[int]
    name: str
    is_dir: bool
    size: int
    mode: int
    mod_time: int
    def __init__(self, name: _Optional[str] = ..., is_dir: _Optional[bool] = ..., size: _Optional[int] = ..., mode: _Optional[int] = ..., mod_time: _Optional[int] = ...) -> None: ...

class FsListResult(_message.Message):
    __slots__ = ("execution_id", "status", "path", "entries", "truncated", "total_count", "duration_seconds", "error_message", "error_type")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    ENTRIES_FIELD_NUMBER: _ClassVar[int]
    TRUNCATED_FIELD_NUMBER: _ClassVar[int]
    TOTAL_COUNT_FIELD_NUMBER: _ClassVar[int]
    DURATION_SECONDS_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    ERROR_TYPE_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    status: ExecutionStatus
    path: str
    entries: _containers.RepeatedCompositeFieldContainer[FsEntry]
    truncated: bool
    total_count: int
    duration_seconds: float
    error_message: str
    error_type: str
    def __init__(self, execution_id: _Optional[str] = ..., status: _Optional[_Union[ExecutionStatus, str]] = ..., path: _Optional[str] = ..., entries: _Optional[_Iterable[_Union[FsEntry, _Mapping]]] = ..., truncated: _Optional[bool] = ..., total_count: _Optional[int] = ..., duration_seconds: _Optional[float] = ..., error_message: _Optional[str] = ..., error_type: _Optional[str] = ...) -> None: ...

class FsReadResult(_message.Message):
    __slots__ = ("execution_id", "status", "path", "content", "size_bytes", "truncated", "duration_seconds", "error_message", "error_type")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    SIZE_BYTES_FIELD_NUMBER: _ClassVar[int]
    TRUNCATED_FIELD_NUMBER: _ClassVar[int]
    DURATION_SECONDS_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    ERROR_TYPE_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    status: ExecutionStatus
    path: str
    content: str
    size_bytes: int
    truncated: bool
    duration_seconds: float
    error_message: str
    error_type: str
    def __init__(self, execution_id: _Optional[str] = ..., status: _Optional[_Union[ExecutionStatus, str]] = ..., path: _Optional[str] = ..., content: _Optional[str] = ..., size_bytes: _Optional[int] = ..., truncated: _Optional[bool] = ..., duration_seconds: _Optional[float] = ..., error_message: _Optional[str] = ..., error_type: _Optional[str] = ...) -> None: ...

class FsGrepMatch(_message.Message):
    __slots__ = ("path", "line_number", "content", "before", "after")
    PATH_FIELD_NUMBER: _ClassVar[int]
    LINE_NUMBER_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    BEFORE_FIELD_NUMBER: _ClassVar[int]
    AFTER_FIELD_NUMBER: _ClassVar[int]
    path: str
    line_number: int
    content: str
    before: _containers.RepeatedScalarFieldContainer[str]
    after: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, path: _Optional[str] = ..., line_number: _Optional[int] = ..., content: _Optional[str] = ..., before: _Optional[_Iterable[str]] = ..., after: _Optional[_Iterable[str]] = ...) -> None: ...

class FsGrepResult(_message.Message):
    __slots__ = ("execution_id", "status", "path", "matches", "total_matches", "truncated", "duration_seconds", "error_message", "error_type")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    PATH_FIELD_NUMBER: _ClassVar[int]
    MATCHES_FIELD_NUMBER: _ClassVar[int]
    TOTAL_MATCHES_FIELD_NUMBER: _ClassVar[int]
    TRUNCATED_FIELD_NUMBER: _ClassVar[int]
    DURATION_SECONDS_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    ERROR_TYPE_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    status: ExecutionStatus
    path: str
    matches: _containers.RepeatedCompositeFieldContainer[FsGrepMatch]
    total_matches: int
    truncated: bool
    duration_seconds: float
    error_message: str
    error_type: str
    def __init__(self, execution_id: _Optional[str] = ..., status: _Optional[_Union[ExecutionStatus, str]] = ..., path: _Optional[str] = ..., matches: _Optional[_Iterable[_Union[FsGrepMatch, _Mapping]]] = ..., total_matches: _Optional[int] = ..., truncated: _Optional[bool] = ..., duration_seconds: _Optional[float] = ..., error_message: _Optional[str] = ..., error_type: _Optional[str] = ...) -> None: ...

class FileEditResult(_message.Message):
    __slots__ = ("execution_id", "status", "file_path", "operation", "duration_seconds", "bytes_written", "lines_changed", "backup_path", "error_message", "error_type", "content", "stdout_size", "stderr_size")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    FILE_PATH_FIELD_NUMBER: _ClassVar[int]
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    DURATION_SECONDS_FIELD_NUMBER: _ClassVar[int]
    BYTES_WRITTEN_FIELD_NUMBER: _ClassVar[int]
    LINES_CHANGED_FIELD_NUMBER: _ClassVar[int]
    BACKUP_PATH_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    ERROR_TYPE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    STDOUT_SIZE_FIELD_NUMBER: _ClassVar[int]
    STDERR_SIZE_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    status: ExecutionStatus
    file_path: str
    operation: str
    duration_seconds: float
    bytes_written: int
    lines_changed: int
    backup_path: str
    error_message: str
    error_type: str
    content: str
    stdout_size: int
    stderr_size: int
    def __init__(self, execution_id: _Optional[str] = ..., status: _Optional[_Union[ExecutionStatus, str]] = ..., file_path: _Optional[str] = ..., operation: _Optional[str] = ..., duration_seconds: _Optional[float] = ..., bytes_written: _Optional[int] = ..., lines_changed: _Optional[int] = ..., backup_path: _Optional[str] = ..., error_message: _Optional[str] = ..., error_type: _Optional[str] = ..., content: _Optional[str] = ..., stdout_size: _Optional[int] = ..., stderr_size: _Optional[int] = ...) -> None: ...

class ExecutionStatusUpdate(_message.Message):
    __slots__ = ("execution_id", "status", "command", "process_alive", "elapsed_seconds", "new_output", "new_stderr", "message")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    PROCESS_ALIVE_FIELD_NUMBER: _ClassVar[int]
    ELAPSED_SECONDS_FIELD_NUMBER: _ClassVar[int]
    NEW_OUTPUT_FIELD_NUMBER: _ClassVar[int]
    NEW_STDERR_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    status: ExecutionStatus
    command: str
    process_alive: bool
    elapsed_seconds: float
    new_output: str
    new_stderr: str
    message: str
    def __init__(self, execution_id: _Optional[str] = ..., status: _Optional[_Union[ExecutionStatus, str]] = ..., command: _Optional[str] = ..., process_alive: _Optional[bool] = ..., elapsed_seconds: _Optional[float] = ..., new_output: _Optional[str] = ..., new_stderr: _Optional[str] = ..., message: _Optional[str] = ...) -> None: ...

class PortCheckEntry(_message.Message):
    __slots__ = ("host", "port", "open", "latency_ms", "error")
    HOST_FIELD_NUMBER: _ClassVar[int]
    PORT_FIELD_NUMBER: _ClassVar[int]
    OPEN_FIELD_NUMBER: _ClassVar[int]
    LATENCY_MS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    host: str
    port: int
    open: bool
    latency_ms: float
    error: str
    def __init__(self, host: _Optional[str] = ..., port: _Optional[int] = ..., open: _Optional[bool] = ..., latency_ms: _Optional[float] = ..., error: _Optional[str] = ...) -> None: ...

class PortCheckResult(_message.Message):
    __slots__ = ("execution_id", "status", "results", "error_message", "error_type")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    RESULTS_FIELD_NUMBER: _ClassVar[int]
    ERROR_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    ERROR_TYPE_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    status: ExecutionStatus
    results: _containers.RepeatedCompositeFieldContainer[PortCheckEntry]
    error_message: str
    error_type: str
    def __init__(self, execution_id: _Optional[str] = ..., status: _Optional[_Union[ExecutionStatus, str]] = ..., results: _Optional[_Iterable[_Union[PortCheckEntry, _Mapping]]] = ..., error_message: _Optional[str] = ..., error_type: _Optional[str] = ...) -> None: ...

class FetchLogsResult(_message.Message):
    __slots__ = ("execution_id", "command", "return_code", "duration_ms", "stdout", "stderr", "stdout_size", "stderr_size", "timestamp", "vault_mode", "error")
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    COMMAND_FIELD_NUMBER: _ClassVar[int]
    RETURN_CODE_FIELD_NUMBER: _ClassVar[int]
    DURATION_MS_FIELD_NUMBER: _ClassVar[int]
    STDOUT_FIELD_NUMBER: _ClassVar[int]
    STDERR_FIELD_NUMBER: _ClassVar[int]
    STDOUT_SIZE_FIELD_NUMBER: _ClassVar[int]
    STDERR_SIZE_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    VAULT_MODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    execution_id: str
    command: str
    return_code: int
    duration_ms: int
    stdout: str
    stderr: str
    stdout_size: int
    stderr_size: int
    timestamp: str
    vault_mode: str
    error: str
    def __init__(self, execution_id: _Optional[str] = ..., command: _Optional[str] = ..., return_code: _Optional[int] = ..., duration_ms: _Optional[int] = ..., stdout: _Optional[str] = ..., stderr: _Optional[str] = ..., stdout_size: _Optional[int] = ..., stderr_size: _Optional[int] = ..., timestamp: _Optional[str] = ..., vault_mode: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class AuditWebSession(_message.Message):
    __slots__ = ("id", "title", "created_at", "user_identity")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    USER_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    id: str
    title: str
    created_at: str
    user_identity: str
    def __init__(self, id: _Optional[str] = ..., title: _Optional[str] = ..., created_at: _Optional[str] = ..., user_identity: _Optional[str] = ...) -> None: ...

class AuditFileMutation(_message.Message):
    __slots__ = ("id", "filepath", "operation", "ledger_hash_before", "ledger_hash_after", "diff_stat")
    ID_FIELD_NUMBER: _ClassVar[int]
    FILEPATH_FIELD_NUMBER: _ClassVar[int]
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    LEDGER_HASH_BEFORE_FIELD_NUMBER: _ClassVar[int]
    LEDGER_HASH_AFTER_FIELD_NUMBER: _ClassVar[int]
    DIFF_STAT_FIELD_NUMBER: _ClassVar[int]
    id: int
    filepath: str
    operation: str
    ledger_hash_before: str
    ledger_hash_after: str
    diff_stat: str
    def __init__(self, id: _Optional[int] = ..., filepath: _Optional[str] = ..., operation: _Optional[str] = ..., ledger_hash_before: _Optional[str] = ..., ledger_hash_after: _Optional[str] = ..., diff_stat: _Optional[str] = ...) -> None: ...

class AuditEvent(_message.Message):
    __slots__ = ("id", "operator_session_id", "timestamp", "type", "content_text", "command_raw", "command_exit_code", "command_stdout", "command_stderr", "execution_duration_ms", "stored_locally", "stdout_truncated", "stderr_truncated", "file_mutations")
    ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    TYPE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_TEXT_FIELD_NUMBER: _ClassVar[int]
    COMMAND_RAW_FIELD_NUMBER: _ClassVar[int]
    COMMAND_EXIT_CODE_FIELD_NUMBER: _ClassVar[int]
    COMMAND_STDOUT_FIELD_NUMBER: _ClassVar[int]
    COMMAND_STDERR_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_DURATION_MS_FIELD_NUMBER: _ClassVar[int]
    STORED_LOCALLY_FIELD_NUMBER: _ClassVar[int]
    STDOUT_TRUNCATED_FIELD_NUMBER: _ClassVar[int]
    STDERR_TRUNCATED_FIELD_NUMBER: _ClassVar[int]
    FILE_MUTATIONS_FIELD_NUMBER: _ClassVar[int]
    id: int
    operator_session_id: str
    timestamp: str
    type: str
    content_text: str
    command_raw: str
    command_exit_code: int
    command_stdout: str
    command_stderr: str
    execution_duration_ms: int
    stored_locally: bool
    stdout_truncated: bool
    stderr_truncated: bool
    file_mutations: _containers.RepeatedCompositeFieldContainer[AuditFileMutation]
    def __init__(self, id: _Optional[int] = ..., operator_session_id: _Optional[str] = ..., timestamp: _Optional[str] = ..., type: _Optional[str] = ..., content_text: _Optional[str] = ..., command_raw: _Optional[str] = ..., command_exit_code: _Optional[int] = ..., command_stdout: _Optional[str] = ..., command_stderr: _Optional[str] = ..., execution_duration_ms: _Optional[int] = ..., stored_locally: _Optional[bool] = ..., stdout_truncated: _Optional[bool] = ..., stderr_truncated: _Optional[bool] = ..., file_mutations: _Optional[_Iterable[_Union[AuditFileMutation, _Mapping]]] = ...) -> None: ...

class FetchHistoryResult(_message.Message):
    __slots__ = ("success", "execution_id", "operator_session_id", "web_session", "events", "total", "limit", "offset", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_FIELD_NUMBER: _ClassVar[int]
    EVENTS_FIELD_NUMBER: _ClassVar[int]
    TOTAL_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    OFFSET_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    execution_id: str
    operator_session_id: str
    web_session: AuditWebSession
    events: _containers.RepeatedCompositeFieldContainer[AuditEvent]
    total: int
    limit: int
    offset: int
    error: str
    def __init__(self, success: _Optional[bool] = ..., execution_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., web_session: _Optional[_Union[AuditWebSession, _Mapping]] = ..., events: _Optional[_Iterable[_Union[AuditEvent, _Mapping]]] = ..., total: _Optional[int] = ..., limit: _Optional[int] = ..., offset: _Optional[int] = ..., error: _Optional[str] = ...) -> None: ...

class FileHistoryEntry(_message.Message):
    __slots__ = ("commit_hash", "timestamp", "message")
    COMMIT_HASH_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    commit_hash: str
    timestamp: str
    message: str
    def __init__(self, commit_hash: _Optional[str] = ..., timestamp: _Optional[str] = ..., message: _Optional[str] = ...) -> None: ...

class FetchFileHistoryResult(_message.Message):
    __slots__ = ("success", "execution_id", "file_path", "history", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    FILE_PATH_FIELD_NUMBER: _ClassVar[int]
    HISTORY_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    execution_id: str
    file_path: str
    history: _containers.RepeatedCompositeFieldContainer[FileHistoryEntry]
    error: str
    def __init__(self, success: _Optional[bool] = ..., execution_id: _Optional[str] = ..., file_path: _Optional[str] = ..., history: _Optional[_Iterable[_Union[FileHistoryEntry, _Mapping]]] = ..., error: _Optional[str] = ...) -> None: ...

class RestoreFileResult(_message.Message):
    __slots__ = ("success", "execution_id", "file_path", "commit_hash", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    FILE_PATH_FIELD_NUMBER: _ClassVar[int]
    COMMIT_HASH_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    execution_id: str
    file_path: str
    commit_hash: str
    error: str
    def __init__(self, success: _Optional[bool] = ..., execution_id: _Optional[str] = ..., file_path: _Optional[str] = ..., commit_hash: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class FileDiffEntry(_message.Message):
    __slots__ = ("id", "timestamp", "file_path", "operation", "ledger_hash_before", "ledger_hash_after", "diff_stat", "diff_content", "diff_size", "operator_session_id")
    ID_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    FILE_PATH_FIELD_NUMBER: _ClassVar[int]
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    LEDGER_HASH_BEFORE_FIELD_NUMBER: _ClassVar[int]
    LEDGER_HASH_AFTER_FIELD_NUMBER: _ClassVar[int]
    DIFF_STAT_FIELD_NUMBER: _ClassVar[int]
    DIFF_CONTENT_FIELD_NUMBER: _ClassVar[int]
    DIFF_SIZE_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    timestamp: str
    file_path: str
    operation: str
    ledger_hash_before: str
    ledger_hash_after: str
    diff_stat: str
    diff_content: str
    diff_size: int
    operator_session_id: str
    def __init__(self, id: _Optional[str] = ..., timestamp: _Optional[str] = ..., file_path: _Optional[str] = ..., operation: _Optional[str] = ..., ledger_hash_before: _Optional[str] = ..., ledger_hash_after: _Optional[str] = ..., diff_stat: _Optional[str] = ..., diff_content: _Optional[str] = ..., diff_size: _Optional[int] = ..., operator_session_id: _Optional[str] = ...) -> None: ...

class FetchFileDiffResult(_message.Message):
    __slots__ = ("success", "execution_id", "diffs", "diff", "total", "operator_session_id", "error")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    EXECUTION_ID_FIELD_NUMBER: _ClassVar[int]
    DIFFS_FIELD_NUMBER: _ClassVar[int]
    DIFF_FIELD_NUMBER: _ClassVar[int]
    TOTAL_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    success: bool
    execution_id: str
    diffs: _containers.RepeatedCompositeFieldContainer[FileDiffEntry]
    diff: FileDiffEntry
    total: int
    operator_session_id: str
    error: str
    def __init__(self, success: _Optional[bool] = ..., execution_id: _Optional[str] = ..., diffs: _Optional[_Iterable[_Union[FileDiffEntry, _Mapping]]] = ..., diff: _Optional[_Union[FileDiffEntry, _Mapping]] = ..., total: _Optional[int] = ..., operator_session_id: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class HeartbeatResult(_message.Message):
    __slots__ = ("operator_id", "operator_session_id", "timestamp", "status", "event_type", "source_component", "case_id", "investigation_id", "system_identity", "network_info", "version_info", "uptime_info", "performance_metrics", "os_details", "user_details", "disk_details", "memory_details", "environment", "capability_flags", "fingerprint_details", "system_fingerprint", "api_key")
    OPERATOR_ID_FIELD_NUMBER: _ClassVar[int]
    OPERATOR_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    EVENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    SOURCE_COMPONENT_FIELD_NUMBER: _ClassVar[int]
    CASE_ID_FIELD_NUMBER: _ClassVar[int]
    INVESTIGATION_ID_FIELD_NUMBER: _ClassVar[int]
    SYSTEM_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    NETWORK_INFO_FIELD_NUMBER: _ClassVar[int]
    VERSION_INFO_FIELD_NUMBER: _ClassVar[int]
    UPTIME_INFO_FIELD_NUMBER: _ClassVar[int]
    PERFORMANCE_METRICS_FIELD_NUMBER: _ClassVar[int]
    OS_DETAILS_FIELD_NUMBER: _ClassVar[int]
    USER_DETAILS_FIELD_NUMBER: _ClassVar[int]
    DISK_DETAILS_FIELD_NUMBER: _ClassVar[int]
    MEMORY_DETAILS_FIELD_NUMBER: _ClassVar[int]
    ENVIRONMENT_FIELD_NUMBER: _ClassVar[int]
    CAPABILITY_FLAGS_FIELD_NUMBER: _ClassVar[int]
    FINGERPRINT_DETAILS_FIELD_NUMBER: _ClassVar[int]
    SYSTEM_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    API_KEY_FIELD_NUMBER: _ClassVar[int]
    operator_id: str
    operator_session_id: str
    timestamp: str
    status: str
    event_type: str
    source_component: str
    case_id: str
    investigation_id: str
    system_identity: SystemIdentity
    network_info: NetworkInfo
    version_info: VersionInfo
    uptime_info: UptimeInfo
    performance_metrics: PerformanceMetrics
    os_details: OSDetails
    user_details: UserDetails
    disk_details: DiskDetails
    memory_details: MemoryDetails
    environment: EnvironmentDetails
    capability_flags: CapabilityFlags
    fingerprint_details: FingerprintDetails
    system_fingerprint: str
    api_key: str
    def __init__(self, operator_id: _Optional[str] = ..., operator_session_id: _Optional[str] = ..., timestamp: _Optional[str] = ..., status: _Optional[str] = ..., event_type: _Optional[str] = ..., source_component: _Optional[str] = ..., case_id: _Optional[str] = ..., investigation_id: _Optional[str] = ..., system_identity: _Optional[_Union[SystemIdentity, _Mapping]] = ..., network_info: _Optional[_Union[NetworkInfo, _Mapping]] = ..., version_info: _Optional[_Union[VersionInfo, _Mapping]] = ..., uptime_info: _Optional[_Union[UptimeInfo, _Mapping]] = ..., performance_metrics: _Optional[_Union[PerformanceMetrics, _Mapping]] = ..., os_details: _Optional[_Union[OSDetails, _Mapping]] = ..., user_details: _Optional[_Union[UserDetails, _Mapping]] = ..., disk_details: _Optional[_Union[DiskDetails, _Mapping]] = ..., memory_details: _Optional[_Union[MemoryDetails, _Mapping]] = ..., environment: _Optional[_Union[EnvironmentDetails, _Mapping]] = ..., capability_flags: _Optional[_Union[CapabilityFlags, _Mapping]] = ..., fingerprint_details: _Optional[_Union[FingerprintDetails, _Mapping]] = ..., system_fingerprint: _Optional[str] = ..., api_key: _Optional[str] = ...) -> None: ...

class SystemIdentity(_message.Message):
    __slots__ = ("hostname", "os", "architecture", "pwd", "current_user", "cpu_count", "memory_mb")
    HOSTNAME_FIELD_NUMBER: _ClassVar[int]
    OS_FIELD_NUMBER: _ClassVar[int]
    ARCHITECTURE_FIELD_NUMBER: _ClassVar[int]
    PWD_FIELD_NUMBER: _ClassVar[int]
    CURRENT_USER_FIELD_NUMBER: _ClassVar[int]
    CPU_COUNT_FIELD_NUMBER: _ClassVar[int]
    MEMORY_MB_FIELD_NUMBER: _ClassVar[int]
    hostname: str
    os: str
    architecture: str
    pwd: str
    current_user: str
    cpu_count: int
    memory_mb: int
    def __init__(self, hostname: _Optional[str] = ..., os: _Optional[str] = ..., architecture: _Optional[str] = ..., pwd: _Optional[str] = ..., current_user: _Optional[str] = ..., cpu_count: _Optional[int] = ..., memory_mb: _Optional[int] = ...) -> None: ...

class NetworkInterface(_message.Message):
    __slots__ = ("name", "ip", "mtu")
    NAME_FIELD_NUMBER: _ClassVar[int]
    IP_FIELD_NUMBER: _ClassVar[int]
    MTU_FIELD_NUMBER: _ClassVar[int]
    name: str
    ip: str
    mtu: int
    def __init__(self, name: _Optional[str] = ..., ip: _Optional[str] = ..., mtu: _Optional[int] = ...) -> None: ...

class NetworkInfo(_message.Message):
    __slots__ = ("public_ip", "internal_ip", "interfaces", "connectivity_status")
    PUBLIC_IP_FIELD_NUMBER: _ClassVar[int]
    INTERNAL_IP_FIELD_NUMBER: _ClassVar[int]
    INTERFACES_FIELD_NUMBER: _ClassVar[int]
    CONNECTIVITY_STATUS_FIELD_NUMBER: _ClassVar[int]
    public_ip: str
    internal_ip: str
    interfaces: _containers.RepeatedScalarFieldContainer[str]
    connectivity_status: _containers.RepeatedCompositeFieldContainer[NetworkInterface]
    def __init__(self, public_ip: _Optional[str] = ..., internal_ip: _Optional[str] = ..., interfaces: _Optional[_Iterable[str]] = ..., connectivity_status: _Optional[_Iterable[_Union[NetworkInterface, _Mapping]]] = ...) -> None: ...

class CapabilityFlags(_message.Message):
    __slots__ = ("local_storage_enabled", "git_available", "ledger_mirror_enabled")
    LOCAL_STORAGE_ENABLED_FIELD_NUMBER: _ClassVar[int]
    GIT_AVAILABLE_FIELD_NUMBER: _ClassVar[int]
    LEDGER_MIRROR_ENABLED_FIELD_NUMBER: _ClassVar[int]
    local_storage_enabled: bool
    git_available: bool
    ledger_mirror_enabled: bool
    def __init__(self, local_storage_enabled: _Optional[bool] = ..., git_available: _Optional[bool] = ..., ledger_mirror_enabled: _Optional[bool] = ...) -> None: ...

class VersionInfo(_message.Message):
    __slots__ = ("operator_version", "status")
    OPERATOR_VERSION_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    operator_version: str
    status: str
    def __init__(self, operator_version: _Optional[str] = ..., status: _Optional[str] = ...) -> None: ...

class UptimeInfo(_message.Message):
    __slots__ = ("uptime", "uptime_seconds")
    UPTIME_FIELD_NUMBER: _ClassVar[int]
    UPTIME_SECONDS_FIELD_NUMBER: _ClassVar[int]
    uptime: str
    uptime_seconds: int
    def __init__(self, uptime: _Optional[str] = ..., uptime_seconds: _Optional[int] = ...) -> None: ...

class PerformanceMetrics(_message.Message):
    __slots__ = ("cpu_percent", "memory_percent", "disk_percent", "network_latency", "memory_used_mb", "memory_total_mb", "disk_used_gb", "disk_total_gb")
    CPU_PERCENT_FIELD_NUMBER: _ClassVar[int]
    MEMORY_PERCENT_FIELD_NUMBER: _ClassVar[int]
    DISK_PERCENT_FIELD_NUMBER: _ClassVar[int]
    NETWORK_LATENCY_FIELD_NUMBER: _ClassVar[int]
    MEMORY_USED_MB_FIELD_NUMBER: _ClassVar[int]
    MEMORY_TOTAL_MB_FIELD_NUMBER: _ClassVar[int]
    DISK_USED_GB_FIELD_NUMBER: _ClassVar[int]
    DISK_TOTAL_GB_FIELD_NUMBER: _ClassVar[int]
    cpu_percent: float
    memory_percent: float
    disk_percent: float
    network_latency: float
    memory_used_mb: int
    memory_total_mb: int
    disk_used_gb: float
    disk_total_gb: float
    def __init__(self, cpu_percent: _Optional[float] = ..., memory_percent: _Optional[float] = ..., disk_percent: _Optional[float] = ..., network_latency: _Optional[float] = ..., memory_used_mb: _Optional[int] = ..., memory_total_mb: _Optional[int] = ..., disk_used_gb: _Optional[float] = ..., disk_total_gb: _Optional[float] = ...) -> None: ...

class OSDetails(_message.Message):
    __slots__ = ("kernel", "distro", "version")
    KERNEL_FIELD_NUMBER: _ClassVar[int]
    DISTRO_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    kernel: str
    distro: str
    version: str
    def __init__(self, kernel: _Optional[str] = ..., distro: _Optional[str] = ..., version: _Optional[str] = ...) -> None: ...

class UserDetails(_message.Message):
    __slots__ = ("username", "uid", "gid", "home", "name", "shell")
    USERNAME_FIELD_NUMBER: _ClassVar[int]
    UID_FIELD_NUMBER: _ClassVar[int]
    GID_FIELD_NUMBER: _ClassVar[int]
    HOME_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    SHELL_FIELD_NUMBER: _ClassVar[int]
    username: str
    uid: int
    gid: int
    home: str
    name: str
    shell: str
    def __init__(self, username: _Optional[str] = ..., uid: _Optional[int] = ..., gid: _Optional[int] = ..., home: _Optional[str] = ..., name: _Optional[str] = ..., shell: _Optional[str] = ...) -> None: ...

class DiskDetails(_message.Message):
    __slots__ = ("total_gb", "used_gb", "free_gb", "percent")
    TOTAL_GB_FIELD_NUMBER: _ClassVar[int]
    USED_GB_FIELD_NUMBER: _ClassVar[int]
    FREE_GB_FIELD_NUMBER: _ClassVar[int]
    PERCENT_FIELD_NUMBER: _ClassVar[int]
    total_gb: float
    used_gb: float
    free_gb: float
    percent: float
    def __init__(self, total_gb: _Optional[float] = ..., used_gb: _Optional[float] = ..., free_gb: _Optional[float] = ..., percent: _Optional[float] = ...) -> None: ...

class MemoryDetails(_message.Message):
    __slots__ = ("total_mb", "available_mb", "used_mb", "percent")
    TOTAL_MB_FIELD_NUMBER: _ClassVar[int]
    AVAILABLE_MB_FIELD_NUMBER: _ClassVar[int]
    USED_MB_FIELD_NUMBER: _ClassVar[int]
    PERCENT_FIELD_NUMBER: _ClassVar[int]
    total_mb: int
    available_mb: int
    used_mb: int
    percent: float
    def __init__(self, total_mb: _Optional[int] = ..., available_mb: _Optional[int] = ..., used_mb: _Optional[int] = ..., percent: _Optional[float] = ...) -> None: ...

class EnvironmentDetails(_message.Message):
    __slots__ = ("pwd", "lang", "timezone", "term", "is_container", "container_runtime", "container_signals", "init_system")
    PWD_FIELD_NUMBER: _ClassVar[int]
    LANG_FIELD_NUMBER: _ClassVar[int]
    TIMEZONE_FIELD_NUMBER: _ClassVar[int]
    TERM_FIELD_NUMBER: _ClassVar[int]
    IS_CONTAINER_FIELD_NUMBER: _ClassVar[int]
    CONTAINER_RUNTIME_FIELD_NUMBER: _ClassVar[int]
    CONTAINER_SIGNALS_FIELD_NUMBER: _ClassVar[int]
    INIT_SYSTEM_FIELD_NUMBER: _ClassVar[int]
    pwd: str
    lang: str
    timezone: str
    term: str
    is_container: bool
    container_runtime: str
    container_signals: _containers.RepeatedScalarFieldContainer[str]
    init_system: str
    def __init__(self, pwd: _Optional[str] = ..., lang: _Optional[str] = ..., timezone: _Optional[str] = ..., term: _Optional[str] = ..., is_container: _Optional[bool] = ..., container_runtime: _Optional[str] = ..., container_signals: _Optional[_Iterable[str]] = ..., init_system: _Optional[str] = ...) -> None: ...

class FingerprintDetails(_message.Message):
    __slots__ = ("os", "architecture", "cpu_count", "machine_id")
    OS_FIELD_NUMBER: _ClassVar[int]
    ARCHITECTURE_FIELD_NUMBER: _ClassVar[int]
    CPU_COUNT_FIELD_NUMBER: _ClassVar[int]
    MACHINE_ID_FIELD_NUMBER: _ClassVar[int]
    os: str
    architecture: str
    cpu_count: int
    machine_id: str
    def __init__(self, os: _Optional[str] = ..., architecture: _Optional[str] = ..., cpu_count: _Optional[int] = ..., machine_id: _Optional[str] = ...) -> None: ...

class PasskeyCredential(_message.Message):
    __slots__ = ("id", "public_key", "counter", "transports", "created_at_unix_ms", "last_used_at_unix_ms")
    ID_FIELD_NUMBER: _ClassVar[int]
    PUBLIC_KEY_FIELD_NUMBER: _ClassVar[int]
    COUNTER_FIELD_NUMBER: _ClassVar[int]
    TRANSPORTS_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    LAST_USED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    id: str
    public_key: str
    counter: int
    transports: _containers.RepeatedScalarFieldContainer[str]
    created_at_unix_ms: int
    last_used_at_unix_ms: int
    def __init__(self, id: _Optional[str] = ..., public_key: _Optional[str] = ..., counter: _Optional[int] = ..., transports: _Optional[_Iterable[str]] = ..., created_at_unix_ms: _Optional[int] = ..., last_used_at_unix_ms: _Optional[int] = ...) -> None: ...

class PasskeyRegisterChallengeRequested(_message.Message):
    __slots__ = ("user_id", "user_name")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    USER_NAME_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    user_name: str
    def __init__(self, user_id: _Optional[str] = ..., user_name: _Optional[str] = ...) -> None: ...

class PasskeyRegisterChallengeResult(_message.Message):
    __slots__ = ("success", "error", "challenge", "rp", "user", "pub_key_cred_params", "timeout", "attestation", "authenticator_selection", "exclude_credentials")
    class RelyingParty(_message.Message):
        __slots__ = ("name", "id")
        NAME_FIELD_NUMBER: _ClassVar[int]
        ID_FIELD_NUMBER: _ClassVar[int]
        name: str
        id: str
        def __init__(self, name: _Optional[str] = ..., id: _Optional[str] = ...) -> None: ...
    class UserInfo(_message.Message):
        __slots__ = ("id", "name", "display_name")
        ID_FIELD_NUMBER: _ClassVar[int]
        NAME_FIELD_NUMBER: _ClassVar[int]
        DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
        id: str
        name: str
        display_name: str
        def __init__(self, id: _Optional[str] = ..., name: _Optional[str] = ..., display_name: _Optional[str] = ...) -> None: ...
    class PublicKeyCredentialParameters(_message.Message):
        __slots__ = ("type", "alg")
        TYPE_FIELD_NUMBER: _ClassVar[int]
        ALG_FIELD_NUMBER: _ClassVar[int]
        type: str
        alg: int
        def __init__(self, type: _Optional[str] = ..., alg: _Optional[int] = ...) -> None: ...
    class AuthenticatorSelection(_message.Message):
        __slots__ = ("resident_key", "user_verification")
        RESIDENT_KEY_FIELD_NUMBER: _ClassVar[int]
        USER_VERIFICATION_FIELD_NUMBER: _ClassVar[int]
        resident_key: str
        user_verification: str
        def __init__(self, resident_key: _Optional[str] = ..., user_verification: _Optional[str] = ...) -> None: ...
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    CHALLENGE_FIELD_NUMBER: _ClassVar[int]
    RP_FIELD_NUMBER: _ClassVar[int]
    USER_FIELD_NUMBER: _ClassVar[int]
    PUB_KEY_CRED_PARAMS_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_FIELD_NUMBER: _ClassVar[int]
    ATTESTATION_FIELD_NUMBER: _ClassVar[int]
    AUTHENTICATOR_SELECTION_FIELD_NUMBER: _ClassVar[int]
    EXCLUDE_CREDENTIALS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    challenge: str
    rp: PasskeyRegisterChallengeResult.RelyingParty
    user: PasskeyRegisterChallengeResult.UserInfo
    pub_key_cred_params: _containers.RepeatedCompositeFieldContainer[PasskeyRegisterChallengeResult.PublicKeyCredentialParameters]
    timeout: int
    attestation: str
    authenticator_selection: PasskeyRegisterChallengeResult.AuthenticatorSelection
    exclude_credentials: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, success: _Optional[bool] = ..., error: _Optional[str] = ..., challenge: _Optional[str] = ..., rp: _Optional[_Union[PasskeyRegisterChallengeResult.RelyingParty, _Mapping]] = ..., user: _Optional[_Union[PasskeyRegisterChallengeResult.UserInfo, _Mapping]] = ..., pub_key_cred_params: _Optional[_Iterable[_Union[PasskeyRegisterChallengeResult.PublicKeyCredentialParameters, _Mapping]]] = ..., timeout: _Optional[int] = ..., attestation: _Optional[str] = ..., authenticator_selection: _Optional[_Union[PasskeyRegisterChallengeResult.AuthenticatorSelection, _Mapping]] = ..., exclude_credentials: _Optional[_Iterable[str]] = ...) -> None: ...

class AttestationResponse(_message.Message):
    __slots__ = ("id", "raw_id", "client_data_json", "attestation_object", "transports")
    ID_FIELD_NUMBER: _ClassVar[int]
    RAW_ID_FIELD_NUMBER: _ClassVar[int]
    CLIENT_DATA_JSON_FIELD_NUMBER: _ClassVar[int]
    ATTESTATION_OBJECT_FIELD_NUMBER: _ClassVar[int]
    TRANSPORTS_FIELD_NUMBER: _ClassVar[int]
    id: str
    raw_id: str
    client_data_json: str
    attestation_object: str
    transports: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, id: _Optional[str] = ..., raw_id: _Optional[str] = ..., client_data_json: _Optional[str] = ..., attestation_object: _Optional[str] = ..., transports: _Optional[_Iterable[str]] = ...) -> None: ...

class PasskeyRegisterVerifyRequested(_message.Message):
    __slots__ = ("user_id", "attestation_response")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ATTESTATION_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    attestation_response: AttestationResponse
    def __init__(self, user_id: _Optional[str] = ..., attestation_response: _Optional[_Union[AttestationResponse, _Mapping]] = ...) -> None: ...

class PasskeyRegisterVerifyResult(_message.Message):
    __slots__ = ("success", "error", "credential")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    credential: PasskeyCredential
    def __init__(self, success: _Optional[bool] = ..., error: _Optional[str] = ..., credential: _Optional[_Union[PasskeyCredential, _Mapping]] = ...) -> None: ...

class PasskeyAuthChallengeRequested(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class PasskeyAuthChallengeResult(_message.Message):
    __slots__ = ("success", "error", "needs_setup", "challenge", "timeout", "rp_id", "allow_credentials", "user_verification")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    NEEDS_SETUP_FIELD_NUMBER: _ClassVar[int]
    CHALLENGE_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_FIELD_NUMBER: _ClassVar[int]
    RP_ID_FIELD_NUMBER: _ClassVar[int]
    ALLOW_CREDENTIALS_FIELD_NUMBER: _ClassVar[int]
    USER_VERIFICATION_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    needs_setup: bool
    challenge: str
    timeout: int
    rp_id: str
    allow_credentials: _containers.RepeatedScalarFieldContainer[str]
    user_verification: str
    def __init__(self, success: _Optional[bool] = ..., error: _Optional[str] = ..., needs_setup: _Optional[bool] = ..., challenge: _Optional[str] = ..., timeout: _Optional[int] = ..., rp_id: _Optional[str] = ..., allow_credentials: _Optional[_Iterable[str]] = ..., user_verification: _Optional[str] = ...) -> None: ...

class AssertionResponse(_message.Message):
    __slots__ = ("id", "raw_id", "client_data_json", "authenticator_data", "signature", "user_handle")
    ID_FIELD_NUMBER: _ClassVar[int]
    RAW_ID_FIELD_NUMBER: _ClassVar[int]
    CLIENT_DATA_JSON_FIELD_NUMBER: _ClassVar[int]
    AUTHENTICATOR_DATA_FIELD_NUMBER: _ClassVar[int]
    SIGNATURE_FIELD_NUMBER: _ClassVar[int]
    USER_HANDLE_FIELD_NUMBER: _ClassVar[int]
    id: str
    raw_id: str
    client_data_json: str
    authenticator_data: str
    signature: str
    user_handle: str
    def __init__(self, id: _Optional[str] = ..., raw_id: _Optional[str] = ..., client_data_json: _Optional[str] = ..., authenticator_data: _Optional[str] = ..., signature: _Optional[str] = ..., user_handle: _Optional[str] = ...) -> None: ...

class PasskeyAuthVerifyRequested(_message.Message):
    __slots__ = ("user_id", "assertion_response")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    ASSERTION_RESPONSE_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    assertion_response: AssertionResponse
    def __init__(self, user_id: _Optional[str] = ..., assertion_response: _Optional[_Union[AssertionResponse, _Mapping]] = ...) -> None: ...

class PasskeyAuthVerifyResult(_message.Message):
    __slots__ = ("success", "error", "user_id", "web_session_id", "session_expires_at_unix_ms")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    WEB_SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    SESSION_EXPIRES_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    user_id: str
    web_session_id: str
    session_expires_at_unix_ms: int
    def __init__(self, success: _Optional[bool] = ..., error: _Optional[str] = ..., user_id: _Optional[str] = ..., web_session_id: _Optional[str] = ..., session_expires_at_unix_ms: _Optional[int] = ...) -> None: ...

class ListPasskeyCredentialsRequested(_message.Message):
    __slots__ = ("user_id",)
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    def __init__(self, user_id: _Optional[str] = ...) -> None: ...

class ListPasskeyCredentialsResult(_message.Message):
    __slots__ = ("success", "error", "credentials")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    CREDENTIALS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    credentials: _containers.RepeatedCompositeFieldContainer[PasskeyCredential]
    def __init__(self, success: _Optional[bool] = ..., error: _Optional[str] = ..., credentials: _Optional[_Iterable[_Union[PasskeyCredential, _Mapping]]] = ...) -> None: ...

class RevokePasskeyCredentialRequested(_message.Message):
    __slots__ = ("user_id", "credential_id")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_ID_FIELD_NUMBER: _ClassVar[int]
    user_id: str
    credential_id: str
    def __init__(self, user_id: _Optional[str] = ..., credential_id: _Optional[str] = ...) -> None: ...

class RevokePasskeyCredentialResult(_message.Message):
    __slots__ = ("success", "error", "found", "remaining")
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    FOUND_FIELD_NUMBER: _ClassVar[int]
    REMAINING_FIELD_NUMBER: _ClassVar[int]
    success: bool
    error: str
    found: bool
    remaining: int
    def __init__(self, success: _Optional[bool] = ..., error: _Optional[str] = ..., found: _Optional[bool] = ..., remaining: _Optional[int] = ...) -> None: ...
