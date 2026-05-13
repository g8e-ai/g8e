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

This is the canonical UAP (Universal Action Protocol) construction point
that carries governance metadata (L1/L2/L3) that downstream
components verify before execution.
"""

from __future__ import annotations

import hashlib
import hmac
import logging
import json
from datetime import datetime, timezone, timedelta

from app.constants import ComponentName, ExecutionStatus, EventType
from app.constants.action_type_mappings import map_event_type_to_action_type, map_action_type_to_event_type
from app.models.pubsub_messages import G8eMessage
from app.models.uap import UAPEnvelope, Metadata, Intent, Context, GovernanceMetadata
from app.proto import operator_pb2, common_pb2
from google.protobuf.json_format import MessageToDict, ParseDict

logger = logging.getLogger(__name__)


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


def build_uap_envelope(
    message: G8eMessage,
    *,
    auditor_hmac_key: str,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
) -> UAPEnvelope:
    """Build a UAP JSON envelope with structured intent data."""
    if message.payload is None:
        raise ValueError("G8eMessage.payload is required to build UAPEnvelope")

    # Serialize to bytes for legacy payload and L2 signature
    proto_payload = message.payload.to_protobuf()
    payload_bytes = proto_payload.SerializeToString()
    payload_dict = message.payload.model_dump(mode="json")

    action_type = map_event_type_to_action_type(message.event_type)
    
    now_utc = datetime.now(timezone.utc)
    expires_at = now_utc + timedelta(minutes=5)

    envelope = UAPEnvelope(
        protocol_version="1.0",
        id=message.id,
        timestamp=message.timestamp or now_utc,
        expires_at=expires_at,
        source_component=message.source_component.value,
        action_type=action_type,
        target_resource="localhost",
        operator_id=message.operator_id or "",
        operator_session_id=message.operator_session_id or "",
        state_merkle_root=state_merkle_root,
        intent_data=payload_dict,
        case_id=message.case_id,
        investigation_id=message.investigation_id,
        task_id=message.task_id,
        payload=payload_bytes,
    )

    # L2 Metadata
    if auditor_hmac_key:
        envelope.governance.l2.tribunal_signature = sign_l2_tribunal(
            message.event_type, payload_bytes, auditor_hmac_key
        )
    if agent_ids:
        envelope.governance.l2.agent_ids = agent_ids

    return envelope


def build_uap_envelope_json(
    message: G8eMessage,
    *,
    auditor_hmac_key: str,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
) -> str:
    """Build a UAP JSON envelope and return it as a JSON string."""
    envelope = build_uap_envelope(
        message,
        auditor_hmac_key=auditor_hmac_key,
        agent_ids=agent_ids,
        state_merkle_root=state_merkle_root,
    )
    return envelope.model_dump_json(exclude_none=True)


def build_universal_envelope_bytes(
    message: G8eMessage,
    *,
    auditor_hmac_key: str,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
    system_fingerprint: str = "",
) -> bytes:
    """DEPRECATED: Use build_uap_envelope_json. Returns UAP JSON as bytes."""
    return build_uap_envelope_json(
        message,
        auditor_hmac_key=auditor_hmac_key,
        agent_ids=agent_ids,
        state_merkle_root=state_merkle_root,
    ).encode("utf-8")


def protobuf_execution_status_to_python(status_int: int) -> ExecutionStatus:
    """Convert protobuf ExecutionStatus enum value to Python ExecutionStatus string."""
    if status_int not in _PROTOBUF_EXECUTION_STATUS_TO_PYTHON:
        raise ValueError(
            f"Unknown protobuf ExecutionStatus value: {status_int}. "
            f"Valid values: {list(_PROTOBUF_EXECUTION_STATUS_TO_PYTHON.keys())}"
        )
    return _PROTOBUF_EXECUTION_STATUS_TO_PYTHON[status_int]


def decode_uap_envelope(data: bytes | str) -> Dict[str, Any]:
    """Decode UAP JSON envelope from g8eo."""
    if isinstance(data, bytes):
        data = data.decode("utf-8")
    return json.loads(data)


def decode_universal_envelope(envelope_bytes: bytes) -> Dict[str, Any]:
    """DEPRECATED: Use decode_uap_envelope. Decodes UAP JSON from bytes."""
    return decode_uap_envelope(envelope_bytes)


def _convert_value(value: Any) -> Any:
    """Convert a value to a Pydantic-friendly format."""
    if isinstance(value, dict):
        return {k: _convert_value(v) for k, v in value.items()}
    if isinstance(value, list):
        return [_convert_value(v) for v in value]
    return value


def decode_g8eo_result_envelope(envelope_data: bytes | str | Dict[str, Any]) -> Dict[str, Any]:
    """Decode a g8eo result GovernanceEnvelope (JSON or bytes) and convert to a Pydantic-compatible dict.
    
    This is the strict protojson unmarshaling path that replaces legacy dictionary fallback logic.
    """
    if isinstance(envelope_data, (bytes, str)):
        raw_json = envelope_data.decode("utf-8") if isinstance(envelope_data, bytes) else envelope_data
    else:
        raw_json = json.dumps(envelope_data)

    # 1. Unmarshal into GovernanceEnvelope protobuf model (strict)
    envelope = common_pb2.GovernanceEnvelope()
    ParseDict(json.loads(raw_json), envelope, ignore_unknown_fields=True)

    # 2. Extract metadata
    result = {
        "id": envelope.id,
        "timestamp": envelope.timestamp.ToDatetime(tzinfo=timezone.utc) if envelope.HasField("timestamp") else None,
        "event_type": envelope.event_type,
        "operator_id": envelope.operator_id,
        "operator_session_id": envelope.operator_session_id,
        "case_id": envelope.case_id,
        "investigation_id": envelope.investigation_id,
        "task_id": envelope.task_id,
    }

    # 3. Extract and normalize payload from intent_data
    payload_dict = MessageToDict(envelope.intent_data, preserving_proto_field_name=True)
    
    # 4. Canonical payload_type check
    # We prefer the 'payload_type' field injected by g8eo for strict discriminator-based parsing.
    # Fallback to manual mapping only if missing (legacy or non-g8eo sources).
    if "payload_type" not in payload_dict:
        action_type = envelope.action_type or envelope.event_type or ""
        
        if "EXECUTE_BASH_RESULT" in action_type or EventType.OPERATOR_COMMAND_RESULT in action_type:
            payload_dict["payload_type"] = "execution_result"
        elif "EXECUTE_BASH_CANCELLED" in action_type or EventType.OPERATOR_COMMAND_CANCELLED in action_type:
            payload_dict["payload_type"] = "cancellation_result"
        elif "EXECUTE_STATUS_UPDATE" in action_type or EventType.OPERATOR_COMMAND_STATUS_UPDATED in action_type:
            payload_dict["payload_type"] = "execution_status"
        elif "FILE_EDIT_RESULT" in action_type or EventType.OPERATOR_FILE_EDIT_COMPLETED in action_type:
            payload_dict["payload_type"] = "file_edit_result"
        elif "FS_LIST_RESULT" in action_type or EventType.OPERATOR_FS_LIST_COMPLETED in action_type:
            payload_dict["payload_type"] = "fs_list_result"
        elif "FS_GREP_RESULT" in action_type or EventType.OPERATOR_FS_GREP_COMPLETED in action_type:
            payload_dict["payload_type"] = "fs_grep_result"
        elif "FS_READ_RESULT" in action_type or EventType.OPERATOR_FS_READ_COMPLETED in action_type:
            payload_dict["payload_type"] = "fs_read_result"
        elif "PORT_CHECK_RESULT" in action_type or EventType.OPERATOR_PORT_CHECK_COMPLETED in action_type:
            payload_dict["payload_type"] = "port_check_result"
        elif "HEARTBEAT" in action_type or EventType.OPERATOR_HEARTBEAT_RECEIVED in action_type:
            payload_dict["payload_type"] = "heartbeat"
        else:
            payload_dict["payload_type"] = "unknown"

    # 5. Handle numeric enum status from g8eo
    if "status" in payload_dict and isinstance(payload_dict["status"], (int, float)):
        payload_dict["status"] = protobuf_execution_status_to_python(int(payload_dict["status"])).value

    result["payload"] = _convert_value(payload_dict)
    return result
