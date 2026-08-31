# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""GovernanceEnvelope model and transaction hash utility.

Provides pydantic models matching the GovernanceEnvelope wire format and a
``compute_transaction_hash()`` function implementing the SHA-256 algorithm
described in the protocol specification.

The hash is a SHA-256 digest over canonicalized fields in protocol field order:
action_type, target_resource, payload (base64), state_merkle_root, nonce,
expires_at (normalized to fixed microsecond UTC), intent_data (canonicalized
``key=value`` map), requestor_user_id, acting_app_id.  Empty fields are
omitted; a trailing ``|`` follows each present field.  This matches Go's
``GenerateMessageID`` canonicalization exactly.

L3 proof is intentionally excluded from the hash so that L2 consensus can sign
before the human notary is asked.
"""

from __future__ import annotations

import base64
import hashlib
from datetime import datetime, timezone
from typing import Any

from .base import G8eBaseModel, Field, UTCDatetime

try:
    from datetime import UTC
except ImportError:
    UTC = timezone.utc


class GovernanceL1(G8eBaseModel):
    """L1 Technical Bedrock proof."""
    validated: bool = False
    violations: list[str] = Field(default_factory=list)


class GovernanceL2Vote(G8eBaseModel):
    """A single L2 consensus vote."""
    signer_key_id: str
    consensus_signature: str
    decision: bool


class GovernanceL2(G8eBaseModel):
    """L2 Consensus proof."""
    consensus_set_id: str = ""
    votes: list[GovernanceL2Vote] = Field(default_factory=list)


class GovernanceL3Proof(G8eBaseModel):
    """L3 Notary proof (union: WebAuthn or CLI/mTLS)."""
    client_data_json: str | None = None
    authenticator_data: str | None = None
    signature: str | None = None
    credential_id: str | None = None
    mtls_cert_fingerprint: str | None = None
    cli_signature: str | None = None


class GovernanceL3(G8eBaseModel):
    """L3 Notary proof."""
    proof: GovernanceL3Proof | None = None


class GovernanceMetadata(G8eBaseModel):
    """Governance metadata attached to an envelope (L1/L2/L3 proofs)."""
    l1: GovernanceL1 = Field(default_factory=GovernanceL1)
    l2: GovernanceL2 = Field(default_factory=GovernanceL2)
    l3: GovernanceL3 = Field(default_factory=GovernanceL3)


class GovernanceEnvelope(G8eBaseModel):
    """Canonical wire format for all mutations in the g8e platform.

    Fields match the protocol specification.  The ``id`` field must be set to
    the deterministic transaction hash computed from the envelope's critical
    fields via :func:`compute_transaction_hash`.
    """

    # Identity and routing
    id: str
    timestamp: UTCDatetime
    expires_at: UTCDatetime
    source_component: str
    operator_id: str | None = None
    operator_session_id: str | None = None
    web_session_id: str | None = None
    cli_session_id: str | None = None
    event_type: str

    # Payload
    payload: str
    intent_data: dict[str, Any] = Field(default_factory=dict)

    # Action classification
    action_type: str
    target_resource: str

    # State binding
    state_merkle_root: str
    nonce: str

    # Governance metadata
    governance: GovernanceMetadata = Field(default_factory=GovernanceMetadata)

    # Delegation
    requestor_user_id: str | None = None
    acting_app_id: str | None = None

    # Optional context
    case_id: str | None = None
    investigation_id: str | None = None
    task_id: str | None = None
    system_fingerprint: str | None = None
    tenant_id: str | None = None
    binding_persona: str | None = None

    # Gateway governance posture at envelope construction time (doctrine,
    # consensus, ratify, notary). Set by the gateway; the operator reads it here at
    # L4 verification time instead of from out-of-band config. Not included
    # in the transaction hash — it is policy metadata, not intent.
    posture: str | None = None

    # Protocol
    transaction_hash: str | None = None
    protocol_version: str = "1.0"


class CommandIntent(G8eBaseModel):
    """Pre-governance command intent published by an app workload to a cmd: channel.

    The ensemble constructs this from a ``G8eMessage`` at the publish boundary,
    serializes the payload protobuf to bytes and base64-encodes it, and
    publishes the protojson representation to ``cmd:<operator_id>:<operator_session_id>``.
    The gateway decodes it via protojson, validates the target operator session,
    fetches the state Merkle root, and constructs the governed
    ``GovernanceEnvelope``. The ensemble does not build governance envelopes
    for operator command dispatch.

    The ``payload`` field carries base64-encoded serialized operator protobuf
    bytes (e.g., ``CommandRequested``, ``FileEditRequested``), matching the
    protojson encoding of the proto ``bytes`` field.
    """

    # Identity & routing
    operator_id: str
    operator_session_id: str
    requestor_user_id: str | None = None

    # Intent classification
    event_type: str | None = None
    action_type: str
    target_resource: str | None = None

    # Payload: base64-encoded serialized operator protobuf bytes
    payload: str

    # Application context
    case_id: str | None = None
    investigation_id: str | None = None
    task_id: str | None = None
    web_session_id: str | None = None
    cli_session_id: str | None = None

    @classmethod
    def from_payload_bytes(
        cls,
        *,
        operator_id: str,
        operator_session_id: str,
        action_type: str,
        payload_bytes: bytes,
        requestor_user_id: str | None = None,
        event_type: str | None = None,
        target_resource: str | None = None,
        case_id: str | None = None,
        investigation_id: str | None = None,
        task_id: str | None = None,
        web_session_id: str | None = None,
        cli_session_id: str | None = None,
    ) -> "CommandIntent":
        """Build a CommandIntent from raw protobuf payload bytes.

        Encodes ``payload_bytes`` as base64 ASCII for the protojson wire format.
        """
        return cls(
            operator_id=operator_id,
            operator_session_id=operator_session_id,
            requestor_user_id=requestor_user_id,
            event_type=event_type,
            action_type=action_type,
            target_resource=target_resource,
            payload=base64.b64encode(payload_bytes).decode("ascii"),
            case_id=case_id,
            investigation_id=investigation_id,
            task_id=task_id,
            web_session_id=web_session_id,
            cli_session_id=cli_session_id,
        )

    @property
    def payload_bytes(self) -> bytes:
        """Decode the base64 payload back to raw protobuf bytes."""
        return base64.b64decode(self.payload) if self.payload else b""


def _normalize_timestamp(ts: str) -> str:
    """Normalize a timestamp string to fixed 6-digit microsecond UTC format.

    Matches Go's ``timesvc.FormatTimestamp`` which produces
    ``2026-01-01T00:00:00.000000Z``.  Accepts any RFC3339Nano input.
    """
    s = ts.strip()
    if s.endswith("Z"):
        s = s[:-1] + "+00:00"
    dt = datetime.fromisoformat(s)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=UTC)
    dt = dt.astimezone(UTC)
    return dt.strftime("%Y-%m-%dT%H:%M:%S.") + f"{dt.microsecond:06d}Z"


def _canonicalize_map(m: dict[str, Any]) -> str:
    """Recursively convert a map to Go-compatible ``key=value,key=value`` format.

    Keys are sorted alphabetically.  Matches Go's ``canonicalizeMap`` exactly.
    """
    if len(m) == 0:
        return ""
    keys = sorted(m.keys())
    parts = []
    for k in keys:
        parts.append(f"{k}={_canonicalize_value(m[k])}")
    return ",".join(parts)


def _canonicalize_value(v: Any) -> str:
    """Convert a value to its canonical string representation.

    Matches Go's ``canonicalizeValue`` type switch exactly.
    """
    if v is None:
        return ""
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, str):
        return v
    if isinstance(v, (int, float)):
        return f"{float(v):f}"
    if isinstance(v, list):
        return "[" + ",".join(_canonicalize_value(item) for item in v) + "]"
    if isinstance(v, dict):
        return _canonicalize_map(v)
    raise TypeError(f"unsupported type {type(v).__name__} in intent_data canonicalization")


def _canonicalize_intent_data(intent_data: dict[str, Any]) -> str:
    """Canonicalize intent_data to Go-compatible ``key=value,key=value`` format.

    Keys are sorted alphabetically; nested maps are recursed.  This matches
    Go's ``canonicalizeMap``/``canonicalizeValue`` exactly.
    """
    if not intent_data:
        return ""
    return _canonicalize_map(intent_data)


def compute_transaction_hash(
    *,
    action_type: str,
    target_resource: str,
    payload: str,
    state_merkle_root: str,
    nonce: str,
    expires_at: str,
    intent_data: dict[str, Any],
    requestor_user_id: str | None = None,
    acting_app_id: str | None = None,
) -> str:
    """Compute the deterministic SHA-256 transaction hash for a GovernanceEnvelope.

    The hash is computed over the following fields in protocol field order:
    action_type, target_resource, payload (base64-encoded), state_merkle_root,
    nonce, expires_at (normalized to fixed microsecond UTC), intent_data
    (canonicalized ``key=value`` map), requestor_user_id, acting_app_id.

    Empty/None fields are omitted entirely (no value, no separator).  A
    trailing ``|`` is appended after each present field, matching Go's
    ``GenerateMessageID`` canonicalization exactly.

    L3 proof is intentionally excluded so that L2 consensus can sign before the
    human notary is asked.

    Args:
        action_type: UAP-compatible action type (e.g. ``"EXECUTE_BASH"``).
        target_resource: Target resource path or identifier.
        payload: Base64-encoded payload bytes (standard encoding, as produced
            by ``base64.b64encode``).  Empty string is omitted.
        state_merkle_root: Current state Merkle root from the Gateway.
        nonce: Unique nonce for replay defense.
        expires_at: Expiry timestamp in RFC3339Nano format.  Normalized to
            fixed 6-digit microsecond UTC (e.g. ``2026-01-01T00:00:00.000000Z``).
        intent_data: Structured JSON view of the intent.  Empty dict is omitted.
        requestor_user_id: Human delegator user ID.  None/empty omitted.
        acting_app_id: Delegate tool/app ID.  None/empty omitted.

    Returns:
        Hex-encoded SHA-256 digest string.
    """
    parts: list[str] = []

    if action_type:
        parts.append(action_type + "|")
    if target_resource:
        parts.append(target_resource + "|")
    if payload:
        parts.append(payload + "|")
    if state_merkle_root:
        parts.append(state_merkle_root + "|")
    if nonce:
        parts.append(nonce + "|")
    if expires_at:
        parts.append(_normalize_timestamp(expires_at) + "|")
    canonical_intent = _canonicalize_intent_data(intent_data)
    if canonical_intent:
        parts.append(canonical_intent + "|")
    if requestor_user_id:
        parts.append(requestor_user_id + "|")
    if acting_app_id:
        parts.append(acting_app_id + "|")

    message = "".join(parts)
    return hashlib.sha256(message.encode("utf-8")).hexdigest()
