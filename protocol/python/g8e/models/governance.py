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
expires_at (UTC RFC3339Nano), intent_data (canonicalized map),
requestor_user_id, acting_app_id.

L3 proof is intentionally excluded from the hash so that L2 consensus can sign
before the human notary is asked.
"""

from __future__ import annotations

import hashlib
import json
from typing import Any

from .base import G8eBaseModel, Field, UTCDatetime


class GovernanceL1(G8eBaseModel):
    """L1 Technical Bedrock proof."""
    validated: bool = False
    violations: list[dict[str, Any]] = Field(default_factory=list)


class GovernanceL2Vote(G8eBaseModel):
    """A single L2 consensus vote."""
    voter_id: str
    decision: str
    timestamp: str | None = None
    signature: str | None = None


class GovernanceL2(G8eBaseModel):
    """L2 Consensus proof."""
    votes: list[GovernanceL2Vote] = Field(default_factory=list)
    consensus_reached: bool = False
    threshold: int | None = None


class GovernanceL3(G8eBaseModel):
    """L3 Notary proof."""
    notary_id: str | None = None
    signature: str | None = None
    timestamp: str | None = None
    certificate_chain: list[str] | None = None


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


def _canonicalize_intent_data(intent_data: dict[str, Any]) -> str:
    """Canonicalize intent_data to a deterministic JSON string.

    Keys are sorted recursively; values are serialized with compact separators.
    """
    return json.dumps(intent_data, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


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
    nonce, expires_at (UTC RFC3339Nano), intent_data (canonicalized map),
    requestor_user_id, acting_app_id.

    L3 proof is intentionally excluded so that L2 consensus can sign before the
    human notary is asked.

    Args:
        action_type: UAP-compatible action type (e.g. ``"EXECUTE_BASH"``).
        target_resource: Target resource path or identifier.
        payload: Base64-encoded protobuf payload bytes.
        state_merkle_root: Current state Merkle root from the Gateway.
        nonce: Unique nonce for replay defense.
        expires_at: Expiry timestamp in UTC RFC3339Nano format.
        intent_data: Structured JSON view of the intent.
        requestor_user_id: Human delegator user ID.
        acting_app_id: Delegate tool/app ID.

    Returns:
        Hex-encoded SHA-256 digest string.
    """
    canonical_intent = _canonicalize_intent_data(intent_data)
    parts = [
        action_type,
        target_resource,
        payload,
        state_merkle_root,
        nonce,
        expires_at,
        canonical_intent,
        requestor_user_id or "",
        acting_app_id or "",
    ]
    message = "|".join(parts)
    return hashlib.sha256(message.encode("utf-8")).hexdigest()
