# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import base64
import binascii
import hashlib
from datetime import UTC, datetime, timedelta
from typing import Any

import nacl.signing
from google.protobuf import json_format

from g8e.common.v1 import common_pb2

def format_rfc3339_nano(dt: datetime) -> str:
    """Format datetime as RFC3339Nano (compatible with Go's time.RFC3339Nano)."""
    # Go's RFC3339Nano is 2006-01-02T15:04:05.999999999Z07:00
    # Python datetimes provide microsecond precision; trailing zeros are trimmed to Go's format.
    utc = dt.astimezone(UTC)
    formatted = utc.strftime("%Y-%m-%dT%H:%M:%S")
    if utc.microsecond:
        formatted += f".{utc.microsecond:06d}".rstrip("0")
    return f"{formatted}Z"

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

def canonicalize_map(m: dict[str, Any]) -> str:
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
        parts.append(base64.b64encode(env.payload).decode("utf-8"))

    # 4. state_merkle_root
    if env.state_merkle_root:
        parts.append(env.state_merkle_root)

    # 5. nonce
    if env.nonce:
        parts.append(env.nonce)

    # 6. expires_at (timestamp) - UTC RFC3339Nano format
    if env.HasField("expires_at"):
        dt = env.expires_at.ToDatetime().replace(tzinfo=UTC)
        parts.append(format_rfc3339_nano(dt))

    # 7. intent_data (struct)
    if env.HasField("intent_data"):
        # Convert struct to dict
        intent_dict = json_format.MessageToDict(env.intent_data)
        parts.append(canonicalize_map(intent_dict))

    canonical_str = "|".join(parts)
    if parts:
        canonical_str += "|"

    return hashlib.sha256(canonical_str.encode("utf-8")).hexdigest()

def sign_l2(message_id: str, private_key_hex: str, decision: bool = True) -> str:
    """Sign for L2 consensus using ED25519."""
    signing_key = nacl.signing.SigningKey(binascii.unhexlify(private_key_hex))
    # Payload format from transaction_verifier.go:384
    # fmt.Sprintf("%s|%v", messageID, decision)
    # decision bool is true/false in Go
    decision_str = "true" if decision else "false"
    payload = f"{message_id}|{decision_str}"
    signed = signing_key.sign(payload.encode("utf-8"))
    return binascii.hexlify(signed.signature).decode("utf-8")

def build_envelope(
    action_type: str,
    payload: bytes,
    operator_id: str,
    operator_session_id: str,
    state_root: str,
    nonce: str,
    expires_in_seconds: int = 3600,
    target_resource: str = "localhost",
    l2_private_key: str | None = None,
    l2_key_id: str | None = None
) -> common_pb2.GovernanceEnvelope:
    env = common_pb2.GovernanceEnvelope()
    env.protocol_version = "1.0"
    env.timestamp.FromDatetime(datetime.now(UTC))
    env.expires_at.FromDatetime(datetime.now(UTC).replace(microsecond=0) + timedelta(seconds=expires_in_seconds))
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
        env.governance.l2.consensus_set_id = l2_key_id
        vote = env.governance.l2.votes.add()
        vote.signer_key_id = l2_key_id
        vote.consensus_signature = sig
        vote.decision = True

    return env
