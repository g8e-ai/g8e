# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""
GovernanceEnvelope construction and L2 Tribunal signing for outbound
g8ee -> g8eo command messages.

This is the single Protobuf-first construction point that replaces
the legacy JSON `G8eMessage.model_dump_json()` wire format. The
envelope carries governance metadata (L1/L2/L3) that downstream
components verify before execution.
"""

from __future__ import annotations

import hashlib
import hmac
import logging

from google.protobuf.timestamp_pb2 import Timestamp
from google.protobuf.message import DecodeError

from app.constants import ComponentName, ExecutionStatus, EventType
from app.models.pubsub_messages import G8eMessage
from app.proto import common_pb2, operator_pb2

logger = logging.getLogger(__name__)


_COMPONENT_MAP: dict[ComponentName, int] = {
    ComponentName.G8EE: common_pb2.COMPONENT_G8EE,
    ComponentName.G8EO: common_pb2.COMPONENT_G8EO,
    ComponentName.G8ED: common_pb2.COMPONENT_G8ED,
}


_PROTOBUF_EXECUTION_STATUS_TO_PYTHON: dict[int, ExecutionStatus] = {
    operator_pb2.EXECUTION_STATUS_UNSPECIFIED: ExecutionStatus.PENDING,
    operator_pb2.EXECUTION_STATUS_EXECUTING: ExecutionStatus.EXECUTING,
    operator_pb2.EXECUTION_STATUS_COMPLETED: ExecutionStatus.COMPLETED,
    operator_pb2.EXECUTION_STATUS_FAILED: ExecutionStatus.FAILED,
    operator_pb2.EXECUTION_STATUS_CANCELLED: ExecutionStatus.CANCELLED,
    operator_pb2.EXECUTION_STATUS_TIMEOUT: ExecutionStatus.TIMEOUT,
}


def sign_l2_tribunal(event_type: str, payload_bytes: bytes, hmac_key: str) -> str:
    """Compute the L2 Tribunal signature over the canonical envelope material.

    The signature binds the event_type to its payload bytes using
    HMAC-SHA256 with the shared auditor key. The canonical material is
    ``event_type + "\\n" + payload_bytes`` — both sides MUST compute
    the signature over the identical byte sequence.

    Returns the hex-encoded digest. Never returns an empty string on
    success; callers treat an empty signature as "unsigned".
    """
    if not hmac_key:
        raise ValueError("auditor_hmac_key is required for L2 Tribunal signing")
    mac = hmac.new(
        hmac_key.encode("utf-8"),
        event_type.encode("utf-8") + b"\n" + payload_bytes,
        hashlib.sha256,
    )
    return mac.hexdigest()


def build_universal_envelope_bytes(
    message: G8eMessage,
    *,
    auditor_hmac_key: str,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
    system_fingerprint: str = "",
) -> bytes:
    """Build a Protobuf ``GovernanceEnvelope`` with structured intent data.

    The payload is serialized as a google.protobuf.Struct in ``intent_data``
    for JSON-first protocol compatibility. The legacy ``payload`` bytes
    field is also populated for backward compatibility during the pivot.
    """
    if message.payload is None:
        raise ValueError("G8eMessage.payload is required to build GovernanceEnvelope")

    # Serialize to bytes for legacy payload and L2 signature
    proto_payload = message.payload.to_protobuf()
    payload_bytes = proto_payload.SerializeToString()

    envelope = common_pb2.GovernanceEnvelope()
    envelope.id = message.id
    ts = Timestamp()
    ts.FromDatetime(message.timestamp)
    envelope.timestamp.CopyFrom(ts)
    envelope.source_component = _COMPONENT_MAP.get(
        message.source_component, common_pb2.COMPONENT_UNSPECIFIED
    )
    envelope.event_type = message.event_type
    envelope.operator_id = message.operator_id or ""
    envelope.operator_session_id = message.operator_session_id or ""
    envelope.case_id = message.case_id
    envelope.investigation_id = message.investigation_id
    envelope.task_id = message.task_id or ""
    envelope.web_session_id = message.web_session_id
    envelope.system_fingerprint = system_fingerprint
    envelope.state_merkle_root = state_merkle_root
    envelope.payload = payload_bytes

    # Populate structured intent_data (JSON-first)
    payload_dict = message.payload.model_dump(mode="json")
    envelope.intent_data.update(payload_dict)

    # L1 Metadata — technical bedrock validation is enforced downstream
    # via protobuf reflection. This flag is an honest self-report.
    envelope.governance.l1.validated = True

    # L2 Metadata — Tribunal signature binding this envelope to the
    # auditor key. Required for acceptance by g8eo.
    envelope.governance.l2.tribunal_signature = sign_l2_tribunal(
        message.event_type, payload_bytes, auditor_hmac_key
    )
    for agent_id in agent_ids or []:
        envelope.governance.l2.agent_ids.append(agent_id)

    # L3 Metadata — Human approval signature is populated upstream by
    # the approval service when a Passkey assertion is captured. For
    # auto-approved commands the ``auto_approved`` flag is set.
    # This builder leaves L3 empty; callers populate it via the
    # returned envelope before signing upgrades (Phase 4).

    return envelope.SerializeToString()


def protobuf_execution_status_to_python(status_int: int) -> ExecutionStatus:
    """Convert protobuf ExecutionStatus enum value to Python ExecutionStatus string.

    Args:
        status_int: Protobuf enum numeric value (e.g., 2 for EXECUTION_STATUS_COMPLETED)

    Returns:
        Python ExecutionStatus enum string value

    Raises:
        ValueError: If the status_int is not a known protobuf enum value
    """
    if status_int not in _PROTOBUF_EXECUTION_STATUS_TO_PYTHON:
        raise ValueError(
            f"Unknown protobuf ExecutionStatus value: {status_int}. "
            f"Valid values: {list(_PROTOBUF_EXECUTION_STATUS_TO_PYTHON.keys())}"
        )
    return _PROTOBUF_EXECUTION_STATUS_TO_PYTHON[status_int]


def decode_universal_envelope(envelope_bytes: bytes) -> common_pb2.GovernanceEnvelope:
    """Decode protobuf GovernanceEnvelope bytes from g8eo.

    Args:
        envelope_bytes: Raw protobuf GovernanceEnvelope bytes from operator pub/sub

    Returns:
        Decoded GovernanceEnvelope protobuf message

    Raises:
        ValueError: If the bytes cannot be decoded as a valid GovernanceEnvelope
    """
    try:
        envelope = common_pb2.GovernanceEnvelope()
        envelope.ParseFromString(envelope_bytes)
        if not envelope.id:
            raise ValueError("Invalid GovernanceEnvelope: missing id field")
        return envelope
    except DecodeError as e:
        raise ValueError(f"Failed to decode GovernanceEnvelope: {e}") from e


def _protobuf_message_to_dict(message) -> dict[str, object]:
    """Convert a protobuf message to a dict, converting enum fields to strings.

    This handles ExecutionStatus enum fields by converting numeric enum values
    to Python ExecutionStatus string values. Includes all fields with their
    default values, not just explicitly set fields.

    Args:
        message: A protobuf message instance

    Returns:
        Dict representation with enum values converted to strings
    """
    result = {}
    descriptor = message.DESCRIPTOR

    for field_descriptor in descriptor.fields:
        field_name = field_descriptor.name
        value = getattr(message, field_name)

        # In proto3, HasField() only returns True for message fields and singular scalar fields
        # that are explicitly set. For repeated fields, check if non-empty.
        # For scalar fields with default values in proto3, we always include them.
        is_set = False
        if field_descriptor.is_repeated:
            is_set = len(value) > 0
        elif field_descriptor.type == field_descriptor.TYPE_MESSAGE:
            is_set = message.HasField(field_name)
        else:
            # For scalar fields in proto3, always include them (they have default values)
            is_set = True

        if not is_set:
            continue

        # Handle enum fields (ExecutionStatus)
        if field_descriptor.type == field_descriptor.TYPE_ENUM:
            if field_name == "status":
                # Convert ExecutionStatus enum to Python string
                result[field_name] = protobuf_execution_status_to_python(value).value
            else:
                # For other enum fields, use the enum name
                result[field_name] = field_descriptor.enum_type.values_by_number[value].name
        # Handle repeated message fields
        elif field_descriptor.type == field_descriptor.TYPE_MESSAGE and field_descriptor.is_repeated:
            result[field_name] = [_protobuf_message_to_dict(item) for item in value]
        # Handle nested message fields
        elif field_descriptor.type == field_descriptor.TYPE_MESSAGE:
            result[field_name] = _protobuf_message_to_dict(value)
        # Handle scalar fields
        else:
            # Map 'output' to 'stdout' for CommandResult/ExecutionResultsPayload consistency
            if field_name == "output":
                result["stdout"] = value
            elif field_name == "exit_code":
                result["return_code"] = value
            else:
                result[field_name] = value

    return result


def decode_g8eo_result_envelope(envelope_bytes: bytes) -> dict[str, object]:
    """Decode a g8eo result GovernanceEnvelope and convert to Pydantic-compatible dict.

    This function:
    1. Decodes the protobuf GovernanceEnvelope
    2. Extracts the inner protobuf payload based on event_type
    3. Converts protobuf enum fields to Python strings
    4. Returns a dict compatible with G8eoResultEnvelope Pydantic model

    Args:
        envelope_bytes: Raw protobuf GovernanceEnvelope bytes from operator pub/sub

    Returns:
        Dict with envelope metadata and converted payload dict

    Raises:
        ValueError: If decoding fails or event_type is unknown
    """
    envelope = decode_universal_envelope(envelope_bytes)

    # Extract envelope metadata
    try:
        event_type_enum = EventType(envelope.event_type) if envelope.event_type else EventType.OPERATOR_COMMAND_RESULT
    except ValueError:
        # Fallback for unknown event types in tests or future-proofing
        # Pydantic validation will fail if this is used to create a model, 
        # but decode_g8eo_result_envelope should be resilient.
        event_type_enum = envelope.event_type  # type: ignore

    result = {
        "id": envelope.id,
        "timestamp": envelope.timestamp.ToDatetime().isoformat() if envelope.HasField("timestamp") else None,
        "event_type": event_type_enum,
        "operator_id": envelope.operator_id,
        "operator_session_id": envelope.operator_session_id,
        "case_id": envelope.case_id,
        "investigation_id": envelope.investigation_id,
        "task_id": envelope.task_id,
    }

    # Decode inner payload based on event_type
    payload_bytes = envelope.payload
    event_type = envelope.event_type

    # Map event types to protobuf message types
    payload_message = None
    if event_type in (
        "g8e.v1.operator.command.completed",
        "g8e.v1.operator.command.failed",
    ):
        payload_message = operator_pb2.CommandResult()
    elif event_type in (
        EventType.OPERATOR_COMMAND_STATUS_UPDATED,
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_EXECUTING,
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_CANCELLED,
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_QUEUED,
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_RUNNING,
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_COMPLETED,
        EventType.OPERATOR_COMMAND_STATUS_UPDATED_FAILED,
        "g8e.v1.operator.command.status.updated",
        "g8e.v1.operator.command.status.updated.executing",
        "g8e.v1.operator.command.status.updated.cancelled",
        "g8e.v1.operator.command.status.updated.queued",
        "g8e.v1.operator.command.status.updated.running",
        "g8e.v1.operator.command.status.updated.completed",
        "g8e.v1.operator.command.status.updated.failed",
    ):
        payload_message = operator_pb2.ExecutionStatusUpdate()
    elif event_type in (
        "g8e.v1.operator.file.edit.completed",
        "g8e.v1.operator.file.edit.failed",
    ):
        payload_message = operator_pb2.FileEditResult()
    elif event_type in (
        EventType.OPERATOR_FS_LIST_COMPLETED,
        EventType.OPERATOR_FS_LIST_FAILED,
        "g8e.v1.operator.fs.list.completed",
        "g8e.v1.operator.fs.list.failed",
    ):
        payload_message = operator_pb2.FsListResult()
    elif event_type in (
        EventType.OPERATOR_FS_GREP_COMPLETED,
        EventType.OPERATOR_FS_GREP_FAILED,
        "g8e.v1.operator.fs.grep.completed",
        "g8e.v1.operator.fs.grep.failed",
    ):
        payload_message = operator_pb2.FsGrepResult()
    elif event_type in (
        EventType.OPERATOR_FS_READ_COMPLETED,
        EventType.OPERATOR_FS_READ_FAILED,
        "g8e.v1.operator.fs.read.completed",
        "g8e.v1.operator.fs.read.failed",
    ):
        payload_message = operator_pb2.FsReadResult()
    elif event_type in (
        EventType.OPERATOR_PORT_CHECK_COMPLETED,
        EventType.OPERATOR_PORT_CHECK_FAILED,
        "g8e.v1.operator.port.check.completed",
        "g8e.v1.operator.port.check.failed",
    ):
        payload_message = operator_pb2.PortCheckResult()
    elif event_type in ("g8e.v1.operator.heartbeat", "g8e.v1.operator.heartbeat.sent"):
        # Heartbeat uses string status, not enum - handle separately
        payload_message = operator_pb2.HeartbeatResult()
    elif event_type == "g8e.v1.operator.command.cancelled":
        payload_message = operator_pb2.CommandResult()
    elif event_type in (
        "g8e.v1.operator.logs.fetch.completed",
        "g8e.v1.operator.logs.fetch.failed",
    ):
        payload_message = operator_pb2.FetchLogsResult()
    elif event_type in (
        "g8e.v1.operator.history.fetch.completed",
        "g8e.v1.operator.history.fetch.failed",
    ):
        payload_message = operator_pb2.FetchHistoryResult()
    elif event_type in (
        "g8e.v1.operator.file.history.fetch.completed",
        "g8e.v1.operator.file.history.fetch.failed",
    ):
        payload_message = operator_pb2.FetchFileHistoryResult()
    elif event_type in (
        "g8e.v1.operator.file.diff.fetch.completed",
        "g8e.v1.operator.file.diff.fetch.failed",
    ):
        payload_message = operator_pb2.FetchFileDiffResult()
    elif event_type in (
        "g8e.v1.operator.file.restore.completed",
        "g8e.v1.operator.file.restore.failed",
    ):
        payload_message = operator_pb2.RestoreFileResult()
    elif event_type.startswith("g8e.v1.operator.fetch."):
        # Fetch results don't have ExecutionStatus enum fields
        # Parse as generic message to dict
        payload_message = None
        # For fetch results, we need to handle them differently
        # For now, skip payload conversion for these
        result["payload"] = {"payload_type": "unknown"}
        return result
    else:
        logger.warning("[ENVELOPE] Unknown event type: %s", event_type)
        result["payload"] = {"payload_type": "unknown"}
        return result

    if payload_message is not None:
        try:
            payload_message.ParseFromString(payload_bytes)
            payload_dict = _protobuf_message_to_dict(payload_message)

            # Add payload_type discriminator based on event_type
            if "command" in event_type and "status" not in event_type:
                if "completed" in event_type or "failed" in event_type:
                    payload_dict["payload_type"] = "execution_result"
                elif "cancelled" in event_type:
                    payload_dict["payload_type"] = "cancellation_result"
            elif "status" in event_type:
                payload_dict["payload_type"] = "execution_status"
            elif "file.edit" in event_type:
                payload_dict["payload_type"] = "file_edit_result"
            elif "fs.list" in event_type:
                payload_dict["payload_type"] = "fs_list_result"
            elif "fs.grep" in event_type:
                payload_dict["payload_type"] = "fs_grep_result"
            elif "fs.read" in event_type:
                payload_dict["payload_type"] = "fs_read_result"
            elif "port.check" in event_type:
                payload_dict["payload_type"] = "port_check_result"
            elif "logs.fetch" in event_type:
                payload_dict["payload_type"] = "fetch_logs_result"
            elif "history.fetch" in event_type:
                payload_dict["payload_type"] = "fetch_history_result"
            elif "file.history.fetch" in event_type:
                payload_dict["payload_type"] = "fetch_file_history_result"
            elif "file.diff.fetch" in event_type:
                payload_dict["payload_type"] = "fetch_file_diff_result"
            elif "file.restore" in event_type:
                payload_dict["payload_type"] = "restore_file_result"
            elif "heartbeat" in event_type:
                # Heartbeat is handled separately
                payload_dict["payload_type"] = "heartbeat"
            else:
                payload_dict["payload_type"] = "unknown"

            result["payload"] = payload_dict
        except DecodeError as e:
            logger.error("[ENVELOPE] Failed to decode payload for event_type %s: %s", event_type, e)
            raise ValueError(f"Failed to decode payload: {e}") from e

    return result
