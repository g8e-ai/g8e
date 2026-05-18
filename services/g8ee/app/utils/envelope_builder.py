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
from datetime import datetime, timedelta, UTC

from typing import Any, Dict
from pydantic import ValidationError as PydanticValidationError

from app.constants import ExecutionStatus
from app.constants.proto_mappings import protobuf_execution_status_to_python
from app.constants.action_type_mappings import map_event_type_to_action_type
from app.errors import ValidationError
from app.models.pubsub_messages import (
    G8eMessage,
    G8eoResultEnvelope,
    G8eoResultPayload,
    G8eoResultPayloadAdapter,
    G8eoHeartbeatPayload,
)
from app.models.uap import UAPEnvelope
from app.proto import operator_pb2, common_pb2
from google.protobuf.json_format import MessageToDict, ParseDict

logger = logging.getLogger(__name__)


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

    # Serialize the protobuf payload to bytes for the envelope's payload field
    # and as the canonical material for L2 Tribunal signing.
    proto_payload = message.payload.to_protobuf()
    payload_bytes = proto_payload.SerializeToString()
    payload_dict = message.payload.model_dump(mode="json")

    action_type = map_event_type_to_action_type(message.event_type)

    now_utc = datetime.now(UTC)
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
        web_session_id=message.web_session_id or "",
        cli_session_id=message.cli_session_id or "",
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


def decode_uap_envelope(data: bytes | str) -> Dict[str, Any]:
    """Decode UAP JSON envelope from g8eo."""
    if isinstance(data, bytes):
        data = data.decode("utf-8")
    return json.loads(data)


def _convert_value(value: Any) -> Any:
    """Convert a value to a Pydantic-friendly format."""
    if isinstance(value, dict):
        return {k: _convert_value(v) for k, v in value.items()}
    if isinstance(value, list):
        return [_convert_value(v) for v in value]
    return value


def decode_g8eo_result_envelope(envelope_data: bytes | str | Dict[str, Any]) -> Dict[str, Any]:
    """Decode a g8eo result GovernanceEnvelope (JSON or bytes) and convert to a Pydantic-compatible dict.

    Strict protojson unmarshaling: the input must be a valid GovernanceEnvelope.
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
        "timestamp": envelope.timestamp.ToDatetime(tzinfo=UTC) if envelope.HasField("timestamp") else None,
        "event_type": envelope.event_type,
        "operator_id": envelope.operator_id,
        "operator_session_id": envelope.operator_session_id,
        "web_session_id": envelope.web_session_id,
        "cli_session_id": envelope.cli_session_id,
        "case_id": envelope.case_id,
        "investigation_id": envelope.investigation_id,
        "task_id": envelope.task_id,
    }

    # 3. Extract and normalize payload from intent_data
    payload_dict = MessageToDict(envelope.intent_data, preserving_proto_field_name=True)

    # 4. Handle numeric enum status from g8eo
    if "status" in payload_dict and isinstance(payload_dict["status"], (int, float)):
        payload_dict["status"] = protobuf_execution_status_to_python(int(payload_dict["status"])).value

    result["payload"] = _convert_value(payload_dict)
    return result


def parse_inbound_g8eo_payload(payload_raw: dict[str, object]) -> G8eoResultPayload:
    """Parse inbound g8eo payload using discriminator-based union parsing.

    The payload models use a 'payload_type' discriminator field that Pydantic uses
    to automatically determine the correct model class. This matches the wire
    deserialization pattern used for outbound payloads.

    Args:
        payload_raw: The raw payload dict from the pub/sub message

    Returns:
        A validated G8eoResultPayload instance

    Raises:
        ValidationError: If the payload_type is invalid or payload validation fails
    """
    try:
        return G8eoResultPayloadAdapter.validate_python(payload_raw)
    except PydanticValidationError as e:
        raise ValidationError(
            f"Invalid g8eo result payload: {e}",
            component="g8ee",
        ) from e


def decode_and_validate_uap_result(
    data: str | bytes | dict[str, object],
    operator_id: str,
    operator_session_id: str,
) -> G8eoResultEnvelope:
    """Decode and validate a UAP result envelope from g8eo.

    Args:
        data: Raw envelope data (JSON string, bytes, or dict)
        operator_id: Operator ID from channel routing
        operator_session_id: Operator session ID from channel routing

    Returns:
        A validated G8eoResultEnvelope instance

    Raises:
        ValidationError: If decoding or validation fails
    """
    try:
        raw = decode_g8eo_result_envelope(data)
    except (ValueError, TypeError) as e:
        raise ValidationError(f"Failed to decode UAP envelope: {e}", component="g8ee") from e

    event_type_raw = raw.get("event_type")
    if not event_type_raw:
        raise ValidationError("Received message without event_type", component="g8ee")

    _raw_payload = raw.get("payload")
    payload_raw: dict[str, object] = _raw_payload if isinstance(_raw_payload, dict) else {}

    # Propagate IDs from payload to envelope when the envelope omits them.
    for id_field in ("case_id", "investigation_id", "task_id"):
        if not raw.get(id_field) and payload_raw.get(id_field):
            raw[id_field] = payload_raw[id_field]

    payload = parse_inbound_g8eo_payload(payload_raw)
    
    try:
        return G8eoResultEnvelope.model_validate({
            **raw,
            "operator_id": operator_id,
            "operator_session_id": operator_session_id,
            "payload": payload,
        })
    except PydanticValidationError as e:
        raise ValidationError(f"Invalid G8eoResultEnvelope: {e}", component="g8ee") from e


def decode_and_validate_uap_heartbeat(
    data: str | bytes | dict[str, object],
    operator_id: str,
    operator_session_id: str,
) -> G8eoHeartbeatPayload:
    """Decode and validate a UAP heartbeat envelope from g8eo.

    Args:
        data: Raw envelope data (JSON string, bytes, or dict)
        operator_id: Operator ID from channel routing
        operator_session_id: Operator session ID from channel routing

    Returns:
        A validated G8eoHeartbeatPayload instance

    Raises:
        ValidationError: If decoding or validation fails
    """
    if not isinstance(data, (str, bytes, dict)):
        raise ValidationError("Heartbeat must be a UAP envelope (string, bytes, or dict)", component="g8ee")

    try:
        envelope_dict = decode_g8eo_result_envelope(data)
    except (ValueError, TypeError) as e:
        raise ValidationError(f"Failed to decode UAP heartbeat envelope: {e}", component="g8ee") from e

    raw = envelope_dict.get("payload", {})
    if not isinstance(raw, dict):
        raw = {}

    # Ensure identity fields from envelope are present in payload
    if not raw.get("operator_id"):
        raw["operator_id"] = envelope_dict.get("operator_id") or operator_id
    if not raw.get("operator_session_id"):
        raw["operator_session_id"] = envelope_dict.get("operator_session_id") or operator_session_id
    if not raw.get("event_type"):
        raw["event_type"] = str(envelope_dict.get("event_type", ""))
    if not raw.get("timestamp"):
        raw["timestamp"] = envelope_dict.get("timestamp")

    try:
        return G8eoHeartbeatPayload.model_validate(raw)
    except PydanticValidationError as e:
        raise ValidationError(f"Invalid G8eoHeartbeatPayload: {e}", component="g8ee") from e
