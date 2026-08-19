# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from datetime import datetime, UTC
from typing import Any
from app.models.base import BaseModel, Field


class L1Metadata(BaseModel):
    """Doctrine (L1Doctrine) Metadata: Technical Bedrock (Hard Gates)"""

    validated: bool = True
    violations: list[str] = Field(default_factory=list)


class L2Vote(BaseModel):
    """A single Tribunal member's consensus vote (L2)."""

    signer_key_id: str | None = None
    consensus_signature: str | None = None
    decision: bool = False


class L2Metadata(BaseModel):
    """Quorum (L2Consensus) Metadata: Consensus (Tribunal)"""

    tribunal_id: str | None = None
    votes: list[L2Vote] = Field(default_factory=list)


class L3Proof(BaseModel):
    """Notary (L3Notary) proof: WebAuthn or CLI/mTLS."""

    client_data_json: str | None = None
    authenticator_data: str | None = None
    signature: str | None = None
    credential_id: str | None = None
    mtls_cert_fingerprint: str | None = None
    cli_signature: str | None = None


class L3Metadata(BaseModel):
    """Notary (L3Notary) Metadata: Authorization (Human-in-the-loop)"""

    proof: L3Proof = Field(default_factory=L3Proof)


class GovernanceMetadata(BaseModel):
    l1: L1Metadata = Field(default_factory=L1Metadata)
    l2: L2Metadata = Field(default_factory=L2Metadata)
    l3: L3Metadata = Field(default_factory=L3Metadata)


class Metadata(BaseModel):
    sender_id: str
    timestamp: datetime = Field(default_factory=lambda: datetime.now(UTC))
    signature: str | None = None


class Intent(BaseModel):
    action_type: str
    target_resource: str = "localhost"


class Context(BaseModel):
    data_format: str = "json"
    intent_data: dict[str, Any] = Field(default_factory=dict)
    data_blob: str | None = None


class UAPEnvelope(BaseModel):
    protocol_version: str = "1.0"
    id: str | None = None  # Canonical GovernanceEnvelope ID
    timestamp: datetime | str = Field(default_factory=lambda: datetime.now(UTC))
    expires_at: datetime | str = Field(default_factory=lambda: datetime.now(UTC))

    source_component: str = "COMPONENT_UNSPECIFIED"
    operator_id: str | None = None
    operator_session_id: str | None = None
    web_session_id: str | None = None
    cli_session_id: str | None = None

    event_type: str | None = None
    payload: bytes | None = None
    intent_data: dict[str, Any] = Field(default_factory=dict)
    action_type: str | None = None
    target_resource: str = "localhost"

    state_merkle_root: str | None = None
    nonce: str | None = None
    transaction_hash: str | None = None

    governance: GovernanceMetadata = Field(default_factory=GovernanceMetadata)

    case_id: str | None = None
    investigation_id: str | None = None
    task_id: str | None = None
    system_fingerprint: str | None = None
    requestor_user_id: str | None = None
    acting_app_id: str | None = None
    tenant_id: str | None = None
    binding_persona: str | None = None
