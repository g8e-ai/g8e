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
    tribunal_id: str = ""
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

    # Protocol
    transaction_hash: str | None = None
    protocol_version: str = "1.0"


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
