from datetime import datetime, UTC
from typing import Any
from pydantic import BaseModel, Field

class L1Metadata(BaseModel):
    validated: bool = True
    violations: list[str] = Field(default_factory=list)

class L2Metadata(BaseModel):
    tribunal_signature: str = ""
    agent_ids: list[str] = Field(default_factory=list)
    key_id: str = ""

class L3Metadata(BaseModel):
    human_signature: str = ""
    public_key: str = ""
    auto_approved: bool = False

class GovernanceMetadata(BaseModel):
    l1: L1Metadata = Field(default_factory=L1Metadata)
    l2: L2Metadata = Field(default_factory=L2Metadata)
    l3: L3Metadata = Field(default_factory=L3Metadata)

class Metadata(BaseModel):
    sender_id: str
    timestamp: datetime = Field(default_factory=lambda: datetime.now(UTC))
    signature: str = ""

class Intent(BaseModel):
    action_type: str
    target_resource: str = "localhost"

class Context(BaseModel):
    data_format: str = "json"
    intent_data: dict[str, Any] = Field(default_factory=dict)
    data_blob: str = ""

class UAPEnvelope(BaseModel):
    protocol_version: str = "1.0"
    id: str = "" # Canonical GovernanceEnvelope ID
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
    action_type: str = ""
    target_resource: str = "localhost"

    state_merkle_root: str = ""
    nonce: str = ""
    transaction_hash: str = ""

    governance: GovernanceMetadata = Field(default_factory=GovernanceMetadata)

    case_id: str | None = None
    investigation_id: str | None = None
    task_id: str | None = None
    system_fingerprint: str = ""
