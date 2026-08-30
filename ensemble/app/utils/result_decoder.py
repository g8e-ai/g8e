# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Inbound result envelope decoding for g8eo -> g8ee pub/sub messages.

Decodes GovernanceEnvelope protojson payloads published by the g8eo
operator on the ``results:`` and ``heartbeat:`` channels into typed
Pydantic models (``G8eoResultEnvelope``, ``G8eoHeartbeatPayload``).

These functions were previously in ``app.utils.envelope_builder``, which
has been deleted. Envelope construction for the HTTP governance path now
lives in ``app.clients.governance_client`` and uses
``g8e.models.governance`` directly; this module handles only the inbound
decoding direction.
"""

from __future__ import annotations

import json
from datetime import UTC
from typing import Any

from app.constants.proto_mappings import protobuf_execution_status_to_python
from app.errors import ValidationError
from app.models.base import ValidationError as PydanticValidationError
from app.models.pubsub_messages import (
    G8eoResultEnvelope,
    G8eoResultPayload,
    G8eoResultPayloadAdapter,
    G8eoHeartbeatPayload,
)
from g8e.common.v1 import common_pb2
from google.protobuf.json_format import MessageToDict, ParseDict


def decode_uap_envelope(data: bytes | str) -> dict[str, Any]:
    """Decode a UAP JSON envelope from g8eo into a raw dict."""
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


def decode_g8eo_result_envelope(envelope_data: bytes | str | dict[str, Any]) -> dict[str, Any]:
    """Decode a g8eo result GovernanceEnvelope (JSON or bytes) and convert to a Pydantic-compatible dict.

    Strict protojson unmarshaling: the input must be a valid GovernanceEnvelope.
    """
    if isinstance(envelope_data, (bytes, str)):
        raw_json = (
            envelope_data.decode("utf-8") if isinstance(envelope_data, bytes) else envelope_data
        )
    else:
        raw_json = json.dumps(envelope_data)

    # 1. Unmarshal into GovernanceEnvelope protobuf model (strict)
    envelope = common_pb2.GovernanceEnvelope()
    ParseDict(json.loads(raw_json), envelope, ignore_unknown_fields=True)

    # 2. Extract metadata
    result = {
        "id": envelope.id,
        "timestamp": envelope.timestamp.ToDatetime(tzinfo=UTC)
        if envelope.HasField("timestamp")
        else None,
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
        payload_dict["status"] = protobuf_execution_status_to_python(
            int(payload_dict["status"])
        ).value

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
        return G8eoResultEnvelope.model_validate(
            {
                **raw,
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "payload": payload,
            }
        )
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
        raise ValidationError(
            "Heartbeat must be a UAP envelope (string, bytes, or dict)", component="g8ee"
        )

    try:
        envelope_dict = decode_g8eo_result_envelope(data)
    except (ValueError, TypeError) as e:
        raise ValidationError(
            f"Failed to decode UAP heartbeat envelope: {e}", component="g8ee"
        ) from e

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
