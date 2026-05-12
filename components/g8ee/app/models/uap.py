from datetime import datetime, timezone
from typing import Any, Optional, Dict, List, Union
from pydantic import BaseModel, Field

class L1Metadata(BaseModel):
    validated: bool = True
    violations: List[str] = Field(default_factory=list)

class L2Metadata(BaseModel):
    tribunal_signature: str = ""
    agent_ids: List[str] = Field(default_factory=list)
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
    timestamp: datetime = Field(default_factory=lambda: datetime.now(timezone.utc))
    signature: str = ""

class Intent(BaseModel):
    action_type: str
    target_resource: str = "localhost"

class Context(BaseModel):
    data_format: str = "json"
    intent_data: Dict[str, Any] = Field(default_factory=dict)
    data_blob: str = ""

class UAPEnvelope(BaseModel):
    protocol_version: str = "1.0"
    id: str = "" # Canonical UniversalEnvelope ID
    timestamp: Union[datetime, str] = Field(default_factory=lambda: datetime.now(timezone.utc))
    expires_at: Union[datetime, str] = Field(default_factory=lambda: datetime.now(timezone.utc))
    
    source_component: str = "COMPONENT_UNSPECIFIED"
    operator_id: Optional[str] = None
    operator_session_id: Optional[str] = None
    web_session_id: Optional[str] = None

    event_type: Optional[str] = None
    payload: Optional[bytes] = None
    intent_data: Dict[str, Any] = Field(default_factory=dict)
    action_type: str = ""
    target_resource: str = "localhost"
    
    state_merkle_root: str = ""
    nonce: str = ""
    transaction_hash: str = ""
    
    governance: GovernanceMetadata = Field(default_factory=GovernanceMetadata)
    
    case_id: Optional[str] = None
    investigation_id: Optional[str] = None
    task_id: Optional[str] = None
    system_fingerprint: str = ""
