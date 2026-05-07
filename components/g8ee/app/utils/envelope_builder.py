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

"""
UniversalEnvelope construction and L2 Tribunal signing for outbound
g8ee -> g8eo command messages.

This is the single Protobuf-first construction point that replaces
the legacy JSON `G8eMessage.model_dump_json()` wire format. The
envelope carries governance metadata (L1/L2/L3) that downstream
components verify before execution.
"""

from __future__ import annotations

import hashlib
import hmac
import logging

from google.protobuf.timestamp_pb2 import Timestamp

from app.constants import ComponentName
from app.models.pubsub_messages import G8eMessage
from app.proto import common_pb2

logger = logging.getLogger(__name__)


_COMPONENT_MAP: dict[ComponentName, int] = {
    ComponentName.G8EE: common_pb2.COMPONENT_G8EE,
    ComponentName.G8EO: common_pb2.COMPONENT_G8EO,
    ComponentName.G8ED: common_pb2.COMPONENT_G8ED,
}


def sign_l2_tribunal(event_type: str, payload_bytes: bytes, hmac_key: str) -> str:
    """Compute the L2 Tribunal signature over the canonical envelope material.

    The signature binds the event_type to its payload bytes using
    HMAC-SHA256 with the shared auditor key. The canonical material is
    ``event_type + "\\n" + payload_bytes`` — both sides MUST compute
    the signature over the identical byte sequence.

    Returns the hex-encoded digest. Never returns an empty string on
    success; callers treat an empty signature as "unsigned".
    """
    if not hmac_key:
        raise ValueError("auditor_hmac_key is required for L2 Tribunal signing")
    mac = hmac.new(
        hmac_key.encode("utf-8"),
        event_type.encode("utf-8") + b"\n" + payload_bytes,
        hashlib.sha256,
    )
    return mac.hexdigest()


def build_universal_envelope_bytes(
    message: G8eMessage,
    *,
    auditor_hmac_key: str,
    agent_ids: list[str] | None = None,
    state_merkle_root: str = "",
    system_fingerprint: str = "",
) -> bytes:
    """Build a Protobuf ``UniversalEnvelope`` with signed L2 metadata.

    The inner payload MUST implement ``to_protobuf()`` returning a
    Protobuf message. The resulting bytes are published directly to
    the g8es pub/sub channel for consumption by g8eo.

    The L2 Tribunal signature is always populated. L1 technical
    validation is performed downstream by g8eo via protobuf reflection
    on ``forbidden_patterns`` custom options; we set ``l1.validated``
    here only as a hint and do not short-circuit any enforcement.
    """
    if message.payload is None:
        raise ValueError("G8eMessage.payload is required to build UniversalEnvelope")

    proto_payload = message.payload.to_protobuf()
    payload_bytes = proto_payload.SerializeToString()

    envelope = common_pb2.UniversalEnvelope()
    envelope.id = message.id
    ts = Timestamp()
    ts.FromDatetime(message.timestamp)
    envelope.timestamp.CopyFrom(ts)
    envelope.source_component = _COMPONENT_MAP.get(
        message.source_component, common_pb2.COMPONENT_UNSPECIFIED
    )
    envelope.event_type = message.event_type
    envelope.operator_id = message.operator_id or ""
    envelope.operator_session_id = message.operator_session_id or ""
    envelope.case_id = message.case_id
    envelope.investigation_id = message.investigation_id
    envelope.task_id = message.task_id or ""
    envelope.web_session_id = message.web_session_id
    envelope.system_fingerprint = system_fingerprint
    envelope.state_merkle_root = state_merkle_root
    envelope.payload = payload_bytes

    # L1 Metadata — technical bedrock validation is enforced downstream
    # via protobuf reflection. This flag is an honest self-report.
    envelope.governance.l1.validated = True

    # L2 Metadata — Tribunal signature binding this envelope to the
    # auditor key. Required for acceptance by g8eo.
    envelope.governance.l2.tribunal_signature = sign_l2_tribunal(
        message.event_type, payload_bytes, auditor_hmac_key
    )
    for agent_id in agent_ids or []:
        envelope.governance.l2.agent_ids.append(agent_id)

    # L3 Metadata — Human approval signature is populated upstream by
    # the approval service when a Passkey assertion is captured. For
    # auto-approved commands the ``auto_approved`` flag is set.
    # This builder leaves L3 empty; callers populate it via the
    # returned envelope before signing upgrades (Phase 4).

    return envelope.SerializeToString()
