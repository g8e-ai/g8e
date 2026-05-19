import hashlib
import base64
import binascii
from datetime import datetime, timezone, timedelta
from typing import Optional, Dict, Any
from google.protobuf import json_format
from google.protobuf.timestamp_pb2 import Timestamp
import nacl.signing

from g8e_evals.proto import common_pb2

def format_rfc3339_nano(dt: datetime) -> str:
    """Format datetime as RFC3339Nano (compatible with Go's time.RFC3339Nano)."""
    # Go's RFC3339Nano is 2006-01-02T15:04:05.999999999Z07:00
    # Python's isoformat is close, but we need to ensure 'Z' for UTC and nano precision if possible.
    s = dt.astimezone(timezone.utc).isoformat(timespec='nanoseconds')
    return s.replace('+00:00', 'Z')

def canonicalize_value(v: Any) -> str:
    if v is None:
        return ""
    if isinstance(v, str):
        return v
    if isinstance(v, (int, float, bool)):
        # Python bools are True/False, Go's are true/false
        if isinstance(v, bool):
            return "true" if v else "false"
        return str(v)
    if isinstance(v, list):
        return "[" + ",".join(canonicalize_value(x) for x in v) + "]"
    if isinstance(v, dict):
        return canonicalize_map(v)
    return str(v)

def canonicalize_map(m: Dict[str, Any]) -> str:
    if not m:
        return ""
    sorted_keys = sorted(m.keys())
    parts = []
    for k in sorted_keys:
        parts.append(f"{k}={canonicalize_value(m[k])}")
    return ",".join(parts)

def generate_message_id(env: common_pb2.GovernanceEnvelope) -> str:
    """
    Generate deterministic hash of critical envelope fields.
    Matches services/g8eo/pkg/uap/types.go:GenerateMessageID
    """
    parts = []
    
    # 1. action_type
    if env.action_type:
        parts.append(env.action_type)
    
    # 2. target_resource
    if env.target_resource:
        parts.append(env.target_resource)
        
    # 3. payload (bytes) - base64 encoded
    if env.payload:
        parts.append(base64.b64encode(env.payload).decode('utf-8'))
        
    # 4. state_merkle_root
    if env.state_merkle_root:
        parts.append(env.state_merkle_root)
        
    # 5. nonce
    if env.nonce:
        parts.append(env.nonce)
        
    # 6. expires_at (timestamp) - UTC RFC3339Nano format
    if env.HasField('expires_at'):
        dt = env.expires_at.ToDatetime().replace(tzinfo=timezone.utc)
        parts.append(format_rfc3339_nano(dt))
        
    # 7. intent_data (struct)
    if env.HasField('intent_data'):
        # Convert struct to dict
        intent_dict = json_format.MessageToDict(env.intent_data)
        parts.append(canonicalize_map(intent_dict))
        
    canonical_str = "|".join(parts)
    if parts:
        canonical_str += "|"
        
    return hashlib.sha256(canonical_str.encode('utf-8')).hexdigest()

def sign_l2(message_id: str, private_key_hex: str, decision: bool = True) -> str:
    """Sign for L2 consensus using ED25519."""
    signing_key = nacl.signing.SigningKey(binascii.unhexlify(private_key_hex))
    # Payload format from transaction_verifier.go:384
    # fmt.Sprintf("%s|%v", messageID, decision)
    # decision bool is true/false in Go
    decision_str = "true" if decision else "false"
    payload = f"{message_id}|{decision_str}"
    signed = signing_key.sign(payload.encode('utf-8'))
    return binascii.hexlify(signed.signature).decode('utf-8')

def build_envelope(
    action_type: str,
    payload: bytes,
    operator_id: str,
    operator_session_id: str,
    state_root: str,
    nonce: str,
    expires_in_seconds: int = 3600,
    target_resource: str = "localhost",
    l2_private_key: Optional[str] = None,
    l2_key_id: Optional[str] = None
) -> common_pb2.GovernanceEnvelope:
    env = common_pb2.GovernanceEnvelope()
    env.protocol_version = "1.0"
    env.timestamp.FromDatetime(datetime.now(timezone.utc))
    env.expires_at.FromDatetime(datetime.now(timezone.utc).replace(microsecond=0) + timedelta(seconds=expires_in_seconds))
    env.source_component = common_pb2.Component.COMPONENT_CLIENT
    env.operator_id = operator_id
    env.operator_session_id = operator_session_id
    env.action_type = action_type
    env.target_resource = target_resource
    env.payload = payload
    env.state_merkle_root = state_root
    env.nonce = nonce
    
    # Compute hash
    tx_hash = generate_message_id(env)
    env.id = tx_hash
    env.transaction_hash = tx_hash
    
    if l2_private_key and l2_key_id:
        sig = sign_l2(tx_hash, l2_private_key)
        env.governance.l2.key_id = l2_key_id
        env.governance.l2.tribunal_signature = sig
        env.governance.l2.agent_ids.append(l2_key_id)
        
    return env
