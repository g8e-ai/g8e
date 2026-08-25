# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for source component fail-closed mapping (Bug 8) and envelope
identity binding (Bug 11) in ``governance_client.build_governance_envelope``.

Bug 8: ``_source_component_to_proto_enum`` must reject unknown and empty
values with a typed ValidationError rather than silently defaulting to
COMPONENT_AGENT. A misclassified source component could attribute a governed
action to the wrong component and bypass transport-to-envelope identity
binding.

Bug 11: ``build_governance_envelope`` must populate ``requestor_user_id``
and ``acting_app_id`` and include them in the canonical transaction hash so
they are cryptographically tamper-evident and verified by the gateway.
"""

import json
from pathlib import Path

import pytest

from app.constants import EventType, G8EE_COMPONENT
from app.errors import ValidationError
from app.models.command_request_payloads import CommandRequestPayload
from app.models.pubsub_messages import G8eMessage
from app.clients.governance_client import (
    _source_component_to_proto_enum,
    build_governance_envelope,
    build_governance_envelope_json,
)

pytestmark = [pytest.mark.unit]


def _make_message(
    *,
    source_component: str = G8EE_COMPONENT,
    user_id: str | None = "user-123",
    operator_id: str | None = None,
    operator_session_id: str | None = None,
) -> G8eMessage:
    payload = CommandRequestPayload(command="ls -la", execution_id="exec-1")
    return G8eMessage(
        id="msg-1",
        source_component=source_component,
        event_type=EventType.OPERATOR_COMMAND_REQUESTED,
        user_id=user_id,
        operator_id=operator_id,
        operator_session_id=operator_session_id,
        payload=payload,
    )


class TestSourceComponentToProtoEnum:
    """Bug 8: fail-closed mapping for source_component strings."""

    @pytest.mark.parametrize(
        "internal,expected",
        [
            ("g8ee", "COMPONENT_AGENT"),
            ("client", "COMPONENT_CLIENT"),
            ("g8eo", "COMPONENT_G8EO"),
        ],
    )
    def test_known_values_map_to_correct_enum(self, internal: str, expected: str):
        assert _source_component_to_proto_enum(internal) == expected

    def test_empty_string_raises_validation_error(self):
        with pytest.raises(ValidationError, match="source_component is required"):
            _source_component_to_proto_enum("")

    def test_unknown_value_raises_validation_error(self):
        with pytest.raises(ValidationError, match="unknown source_component"):
            _source_component_to_proto_enum("g8eo-gateway")

    def test_arbitrary_unknown_value_raises_validation_error(self):
        with pytest.raises(ValidationError, match="unknown source_component"):
            _source_component_to_proto_enum("rogue-component")

    def test_build_governance_envelope_rejects_empty_source_component(self):
        message = _make_message(source_component="")
        with pytest.raises(ValidationError, match="source_component is required"):
            build_governance_envelope(message, state_merkle_root="root")

    def test_build_governance_envelope_rejects_unknown_source_component(self):
        message = _make_message(source_component="rogue")
        with pytest.raises(ValidationError, match="unknown source_component"):
            build_governance_envelope(message, state_merkle_root="root")


class TestEnvelopeIdentityBinding:
    """Bug 11: requestor_user_id and acting_app_id are populated and hashed."""

    def test_envelope_populates_requestor_user_id_from_message(self):
        message = _make_message(user_id="user-abc")
        envelope = build_governance_envelope(message, state_merkle_root="root")
        assert envelope.requestor_user_id == "user-abc"

    def test_envelope_populates_acting_app_id_as_g8ee(self):
        message = _make_message()
        envelope = build_governance_envelope(message, state_merkle_root="root")
        assert envelope.acting_app_id == G8EE_COMPONENT

    def test_envelope_json_includes_identity_fields(self):
        message = _make_message(user_id="user-xyz")
        envelope_json = build_governance_envelope_json(message, state_merkle_root="root")
        parsed = json.loads(envelope_json)
        assert parsed["requestor_user_id"] == "user-xyz"
        assert parsed["acting_app_id"] == G8EE_COMPONENT

    def test_hash_changes_when_requestor_user_id_changes(self):
        msg_a = _make_message(user_id="user-a")
        msg_b = _make_message(user_id="user-b")
        env_a = build_governance_envelope(msg_a, state_merkle_root="root")
        env_b = build_governance_envelope(msg_b, state_merkle_root="root")
        assert env_a.transaction_hash != env_b.transaction_hash

    def test_hash_includes_acting_app_id(self):
        """The acting_app_id is always g8ee, but the hash must include it.

        We verify this by checking that the hash differs from a hash computed
        without acting_app_id (using the canonical function directly).
        """
        from g8e.models.governance import compute_transaction_hash

        message = _make_message(user_id="user-1")
        envelope = build_governance_envelope(message, state_merkle_root="root")

        # Recompute without acting_app_id to prove it was included
        import base64

        payload_bytes = message.payload.to_protobuf().SerializeToString()
        payload_b64 = base64.b64encode(payload_bytes).decode("ascii")
        hash_without_app = compute_transaction_hash(
            action_type=envelope.action_type,
            target_resource=envelope.target_resource,
            payload=payload_b64,
            state_merkle_root="root",
            nonce=envelope.nonce,
            expires_at=envelope.expires_at.isoformat()
            if hasattr(envelope.expires_at, "isoformat")
            else str(envelope.expires_at),
            intent_data=envelope.intent_data,
            requestor_user_id="user-1",
            acting_app_id=None,
        )
        assert envelope.transaction_hash != hash_without_app

    def test_requestor_user_id_none_when_message_has_no_user(self):
        message = _make_message(user_id=None)
        envelope = build_governance_envelope(message, state_merkle_root="root")
        assert envelope.requestor_user_id is None

    def test_acting_app_id_always_set_even_without_user(self):
        message = _make_message(user_id=None)
        envelope = build_governance_envelope(message, state_merkle_root="root")
        assert envelope.acting_app_id == G8EE_COMPONENT


class TestHashParityVectors:
    """Bug 9: builder-level Go/Python hash parity using shared vectors.

    Reads ``protocol/conformance/hash_vectors.json`` and verifies the
    canonical ``compute_transaction_hash`` produces the expected hash for
    every vector, including vectors with non-empty requestor_user_id and
    acting_app_id.
    """

    @staticmethod
    def _load_vectors() -> list[dict]:
        # Resolve from the ensemble directory to the repo-root protocol dir.
        # ensemble/tests/unit/utils/ -> ../../../../protocol/conformance/
        path = Path(__file__).resolve().parents[4] / "protocol" / "conformance" / "hash_vectors.json"
        data = json.loads(path.read_text())
        assert data["vectors"], "hash_vectors.json contains no vectors"
        return data["vectors"]

    def test_vector_count(self):
        vectors = self._load_vectors()
        assert len(vectors) >= 6

    def test_all_vectors_match_go_expected_hashes(self):
        from g8e.models.governance import compute_transaction_hash

        vectors = self._load_vectors()
        for v in vectors:
            requestor = v.get("requestor_user_id")
            acting = v.get("acting_app_id")
            result = compute_transaction_hash(
                action_type=v["action_type"],
                target_resource=v["target_resource"],
                payload=v["payload_b64"],
                state_merkle_root=v["state_merkle_root"],
                nonce=v["nonce"],
                expires_at=v["expires_at"],
                intent_data=v["intent_data"],
                requestor_user_id=requestor if requestor else None,
                acting_app_id=acting if acting else None,
            )
            assert result == v["expected_hash"], (
                f"hash mismatch for vector {v['name']!r}: "
                f"expected {v['expected_hash']}, got {result}"
            )
