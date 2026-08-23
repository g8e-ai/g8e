# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
GovernanceEnvelope construction and L2 Tribunal signing for outbound
g8ee -> g8eo command messages.

This is the canonical UAP (Universal Action Protocol) construction point
that carries governance metadata (L1/L2/L3) that downstream
components verify before execution.
"""

from __future__ import annotations

import hashlib
import logging
import json
import secrets
from datetime import datetime, timedelta, UTC
from pathlib import Path

from typing import Any
from app.models.base import ValidationError as PydanticValidationError

from app.constants.proto_mappings import protobuf_execution_status_to_python
from app.constants.action_type_mappings import map_event_type_to_action_type
from app.constants.config import G8EE_COMPONENT
from app.errors import ValidationError
from app.models.pubsub_messages import (
    G8eMessage,
    G8eoResultEnvelope,
    G8eoResultPayload,
    G8eoResultPayloadAdapter,
    G8eoHeartbeatPayload,
)
from app.models.uap import UAPEnvelope
from app.proto import common_pb2
from g8e.models.governance import compute_transaction_hash as compute_canonical_transaction_hash
from google.protobuf.json_format import MessageToDict, ParseDict

logger = logging.getLogger(__name__)


# L2 Tribunal signing removed - Engine must not hold L2 keys per cryptographic key segregation.
# L2 signatures are handled by the Gateway (g8eg) using Ed25519 trusted signers.
# See: .local.dev/docs/plans/engine_gateway_secure_link.md §3


# Mapping from internal g8ee payload types to canonical g8e protocol payload types
PAYLOAD_TYPE_MAPPING = {
    "command": "CommandRequested",
    "command_cancel": "CommandCancelRequested",
    "file_edit": "FileEditRequested",
    "fs_list": "FsListRequested",
    "fs_grep": "FsGrepRequested",
    "fs_read": "FsReadRequested",
    "fetch_logs": "FetchLogsRequested",
    "fetch_history": "FetchHistoryRequested",
    "fetch_file_history": "FetchFileHistoryRequested",
    "fetch_file_diff": "FetchFileDiffRequested",
    "restore_file": "RestoreFileRequested",
    "check_port": "CheckPortRequested",
    "heartbeat": "HeartbeatRequested",
    "document_update": "DocumentUpdateRequested",
    "document_delete": "DocumentDeleteRequested",
    "direct_command_audit": "DirectCommandAuditRequested",
}


# Mapping from internal source_component strings to proto Component enum value
# names. The Go gateway decodes GovernanceEnvelope.source_component as the
# g8e.common.v1.Component proto enum via protojson, which expects enum value
# names (e.g. "COMPONENT_AGENT"), not the internal lowercase strings the
# ensemble uses for routing and validation (e.g. "g8ee", "client").
#
# The gateway component ("g8eo-gateway") is not a valid source for governed
# outbound mutations from the ensemble and is intentionally absent. Unknown
# or empty values raise a typed ValidationError rather than silently
# defaulting to COMPONENT_AGENT — a misclassified identity could attribute a
# governed action to the wrong component and bypass identity binding.
_COMPONENT_TO_PROTO_ENUM = {
    "g8ee": "COMPONENT_AGENT",
    "client": "COMPONENT_CLIENT",
    "g8eo": "COMPONENT_G8EO",
}


def _source_component_to_proto_enum(internal: str) -> str:
    """Translate an internal source_component string to a proto Component enum value name.

    Fail-closed: raises ValidationError for unknown or empty values. A
    misclassified source component could attribute a governed action to the
    wrong component and silently bypass transport-to-envelope identity binding.
    """
    if not internal:
        raise ValidationError(
            "source_component is required for governance envelope construction",
            component="g8ee",
        )
    try:
        return _COMPONENT_TO_PROTO_ENUM[internal]
    except KeyError as exc:
        raise ValidationError(
            f"unknown source_component {internal!r}: cannot map to proto Component enum",
            component="g8ee",
        ) from exc


def map_to_canonical_payload_type(internal_payload_type: str) -> str:
    """Map internal g8ee payload type to canonical g8e protocol payload type.

    Per g8e protocol specification, applications must use canonical typed payload
    identifiers for envelope construction. This function translates internal
    payload naming to the canonical protocol schema.

    Args:
        internal_payload_type: Internal payload type (e.g., "command", "file_edit")

    Returns:
        Canonical g8e protocol payload type (e.g., "CommandRequested")
    """
    return PAYLOAD_TYPE_MAPPING.get(internal_payload_type, internal_payload_type)


def generate_nonce() -> str:
    """Generate a cryptographically secure random nonce for replay defense.

    Returns:
        Hexadecimal string (32 bytes = 64 hex characters)
    """
    return secrets.token_hex(32)


def get_certificate_fingerprint(cert_path: str | None) -> str:
    """Compute SHA256 fingerprint of mTLS client certificate.

    Args:
        cert_path: Path to client certificate file

    Returns:
        Hexadecimal SHA256 fingerprint string, or empty string if cert not found
    """
    if not cert_path or not Path(cert_path).exists():
        return ""

    try:
        cert_bytes = Path(cert_path).read_bytes()
        return hashlib.sha256(cert_bytes).hexdigest()
    except Exception as e:
        logger.warning("Failed to compute certificate fingerprint: %s", e)
        return ""


def build_uap_envelope(
    message: G8eMessage,
    *,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
    client_cert_path: str | None = None,
) -> UAPEnvelope:
    """Build a g8e-compliant UAP JSON envelope with structured intent data.

    This function constructs a GovernanceEnvelope per the g8e protocol specification:
    - Uses canonical JSON wire format
    - Generates deterministic transaction hash (SHA256 of canonical fields)
    - Includes nonce for replay defense
    - Includes L3 notary proof (mTLS certificate fingerprint)
    - Uses typed payload identifiers per protocol schema

    Args:
        message: The G8eMessage to wrap in a governance envelope
        agent_ids: Optional list of Tribunal agent IDs for L2 metadata
        state_merkle_root: Current state Merkle root for replay protection
        client_cert_path: Path to mTLS client certificate for L3 proof

    Returns:
        A g8e-compliant UAPEnvelope with transaction hash and governance metadata
    """
    if message.payload is None:
        raise ValueError("G8eMessage.payload is required to build UAPEnvelope")

    # Serialize the protobuf payload to bytes for the envelope's payload field
    proto_payload = message.payload.to_protobuf()
    payload_bytes = proto_payload.SerializeToString()
    payload_dict = message.payload.model_dump(mode="json")

    action_type = map_event_type_to_action_type(message.event_type)

    now_utc = datetime.now(UTC)
    expires_at = now_utc + timedelta(minutes=5)

    # Generate nonce for replay defense
    nonce = generate_nonce()

    # Compute L3 notary proof (mTLS certificate fingerprint)
    cert_fingerprint = get_certificate_fingerprint(client_cert_path)

    # Compute deterministic transaction hash using the canonical g8e algorithm
    # (matches Go's GenerateMessageID exactly). The hash is computed over
    # action_type, target_resource, payload (base64), state_merkle_root, nonce,
    # expires_at, intent_data, requestor_user_id, acting_app_id in proto field
    # order. See g8e.models.governance.compute_transaction_hash.
    import base64

    # Bind identity: the human user who authorized the action (requestor) and
    # the app acting on their behalf (acting_app). Both are included in the
    # canonical transaction hash so they are cryptographically tamper-evident
    # and verified by the gateway's identity binding check. The acting app is
    # always the ensemble (g8ee) for envelopes built here.
    requestor_user_id = message.user_id or ""
    acting_app_id = G8EE_COMPONENT

    payload_b64 = base64.b64encode(payload_bytes).decode("ascii") if payload_bytes else ""
    transaction_hash = compute_canonical_transaction_hash(
        action_type=action_type,
        target_resource="localhost",
        payload=payload_b64,
        state_merkle_root=state_merkle_root,
        nonce=nonce,
        expires_at=expires_at.isoformat(),
        intent_data=payload_dict,
        requestor_user_id=requestor_user_id,
        acting_app_id=acting_app_id,
    )

    envelope = UAPEnvelope(
        protocol_version="1.0",
        id=transaction_hash,
        timestamp=message.timestamp or now_utc,
        expires_at=expires_at,
        source_component=_source_component_to_proto_enum(message.source_component),
        action_type=action_type,
        target_resource="localhost",
        operator_id=message.operator_id or "",
        operator_session_id=message.operator_session_id or "",
        web_session_id=message.web_session_id or "",
        cli_session_id=message.cli_session_id or "",
        state_merkle_root=state_merkle_root,
        nonce=nonce,
        transaction_hash=transaction_hash,
        intent_data=payload_dict,
        case_id=message.case_id,
        investigation_id=message.investigation_id,
        task_id=message.task_id,
        payload=payload_bytes,
        requestor_user_id=requestor_user_id or None,
        acting_app_id=acting_app_id,
    )

    # L2 Metadata - tribunal_id only; votes/signatures handled by Gateway
    if agent_ids:
        envelope.governance.l2.tribunal_id = agent_ids[0] if agent_ids else None

    # L3 Metadata - notary proof (mTLS certificate fingerprint)
    if cert_fingerprint:
        envelope.governance.l3.proof.mtls_cert_fingerprint = cert_fingerprint

    return envelope


def build_uap_envelope_json(
    message: G8eMessage,
    *,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
    client_cert_path: str | None = None,
) -> str:
    """Build a g8e-compliant UAP JSON envelope and return it as a JSON string.

    Args:
        message: The G8eMessage to wrap in a governance envelope
        agent_ids: Optional list of Tribunal agent IDs for L2 metadata
        state_merkle_root: Current state Merkle root for replay protection
        client_cert_path: Path to mTLS client certificate for L3 proof

    Returns:
        Canonical JSON string representation of the envelope
    """
    envelope = build_uap_envelope(
        message,
        agent_ids=agent_ids,
        state_merkle_root=state_merkle_root,
        client_cert_path=client_cert_path,
    )
    return envelope.model_dump_json(exclude_none=True)


def decode_uap_envelope(data: bytes | str) -> dict[str, Any]:
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
