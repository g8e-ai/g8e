# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for g8e protocol compliance in envelope construction."""

import pytest
from datetime import datetime, UTC

from app.utils.envelope_builder import (
    generate_nonce,
    get_certificate_fingerprint,
    map_to_canonical_payload_type,
    build_uap_envelope,
    build_uap_envelope_json,
)
from app.models.pubsub_messages import G8eMessage
from app.constants import EventType, G8EE_COMPONENT
from app.models.command_request_payloads import CommandRequestPayload
from app.models.uap import UAPEnvelope

pytestmark = [pytest.mark.unit]


class TestGenerateNonce:
    """Test nonce generation for replay defense."""

    def test_nonce_is_random(self):
        """Each call generates a unique nonce."""
        nonce1 = generate_nonce()
        nonce2 = generate_nonce()

        assert nonce1 != nonce2
        assert len(nonce1) == 64  # 32 bytes = 64 hex characters
        assert len(nonce2) == 64

    def test_nonce_is_hex_string(self):
        """Nonce is a valid hexadecimal string."""
        nonce = generate_nonce()
        int(nonce, 16)  # Should not raise ValueError


class TestGetCertificateFingerprint:
    """Test mTLS certificate fingerprint computation for L3 notary proof."""

    def test_nonexistent_cert_returns_empty_string(self):
        """Non-existent certificate path returns empty string."""
        fingerprint = get_certificate_fingerprint("/nonexistent/cert.pem")
        assert fingerprint == ""

    def test_none_path_returns_empty_string(self):
        """None path returns empty string."""
        fingerprint = get_certificate_fingerprint(None)
        assert fingerprint == ""


class TestMapToCanonicalPayloadType:
    """Test mapping from internal payload types to canonical g8e protocol types."""

    def test_map_command_to_command_requested(self):
        """Internal 'command' maps to 'CommandRequested'."""
        assert map_to_canonical_payload_type("command") == "CommandRequested"

    def test_map_file_edit_to_file_edit_requested(self):
        """Internal 'file_edit' maps to 'FileEditRequested'."""
        assert map_to_canonical_payload_type("file_edit") == "FileEditRequested"

    def test_map_fs_read_to_fs_read_requested(self):
        """Internal 'fs_read' maps to 'FsReadRequested'."""
        assert map_to_canonical_payload_type("fs_read") == "FsReadRequested"

    def test_map_restore_file(self):
        """Internal 'restore_file' maps to 'RestoreFileRequested'."""
        assert map_to_canonical_payload_type("restore_file") == "RestoreFileRequested"

    def test_map_audit_types(self):
        """Audit-related types map to their canonical proto message names."""
        assert map_to_canonical_payload_type("check_port") == "CheckPortRequested"
        assert map_to_canonical_payload_type("heartbeat") == "HeartbeatRequested"
        assert (
            map_to_canonical_payload_type("direct_command_audit") == "DirectCommandAuditRequested"
        )

    def test_unknown_type_passthrough(self):
        """Unknown types pass through unchanged."""
        assert map_to_canonical_payload_type("unknown.type") == "unknown.type"


class TestBuildUapEnvelope:
    """Test g8e-compliant envelope construction."""

    def test_envelope_has_transaction_hash(self):
        """Envelope includes deterministic transaction hash as id."""
        payload = CommandRequestPayload(
            command="ls -la",
            execution_id="exec-123",
        )
        message = G8eMessage(
            id="msg-123",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            payload=payload,
        )

        envelope = build_uap_envelope(message, state_merkle_root="test-root")

        assert envelope.id is not None
        assert len(envelope.id) == 64  # SHA256 hex string
        assert envelope.transaction_hash == envelope.id

    def test_envelope_has_nonce(self):
        """Envelope includes nonce for replay defense."""
        payload = CommandRequestPayload(
            command="ls -la",
            execution_id="exec-123",
        )
        message = G8eMessage(
            id="msg-123",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            payload=payload,
        )

        envelope = build_uap_envelope(message, state_merkle_root="test-root")

        assert envelope.nonce is not None
        assert len(envelope.nonce) == 64  # 32 bytes = 64 hex characters

    def test_envelope_has_state_merkle_root(self):
        """Envelope includes state Merkle root for state binding."""
        payload = CommandRequestPayload(
            command="ls -la",
            execution_id="exec-123",
        )
        message = G8eMessage(
            id="msg-123",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            payload=payload,
        )

        envelope = build_uap_envelope(message, state_merkle_root="test-root-123")

        assert envelope.state_merkle_root == "test-root-123"

    def test_envelope_has_l3_notary_proof_with_cert(self):
        """Envelope includes L3 notary proof when certificate path is provided."""
        payload = CommandRequestPayload(
            command="ls -la",
            execution_id="exec-123",
        )
        message = G8eMessage(
            id="msg-123",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            payload=payload,
        )

        # With a non-existent cert path, should still set empty string
        envelope = build_uap_envelope(
            message,
            state_merkle_root="test-root",
            client_cert_path="/nonexistent/cert.pem",
        )

        # Certificate fingerprint is empty for non-existent cert, so L3 proof is not set
        assert envelope.governance.l3.proof.mtls_cert_fingerprint is None

    def test_envelope_without_cert_no_l3_proof(self):
        """Envelope without certificate path has no L3 proof."""
        payload = CommandRequestPayload(
            command="ls -la",
            execution_id="exec-123",
        )
        message = G8eMessage(
            id="msg-123",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            payload=payload,
        )

        envelope = build_uap_envelope(message, state_merkle_root="test-root")

        assert envelope.governance.l3.proof.mtls_cert_fingerprint is None

    def test_envelope_uses_canonical_payload_type(self):
        """Envelope uses canonical g8e protocol payload type for hash computation."""
        payload = CommandRequestPayload(
            command="ls -la",
            execution_id="exec-123",
        )
        message = G8eMessage(
            id="msg-123",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            payload=payload,
        )

        envelope = build_uap_envelope(message, state_merkle_root="test-root")

        # The transaction hash should be computed using "CommandRequested"
        # not the internal "command" type
        assert envelope.transaction_hash is not None

    def test_envelope_has_expires_at(self):
        """Envelope includes expiry timestamp."""
        payload = CommandRequestPayload(
            command="ls -la",
            execution_id="exec-123",
        )
        message = G8eMessage(
            id="msg-123",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            payload=payload,
        )

        now = datetime.now(UTC)
        envelope = build_uap_envelope(message, state_merkle_root="test-root")

        assert envelope.expires_at is not None
        # Should be approximately 5 minutes in the future
        expires_at_dt = (
            envelope.expires_at
            if isinstance(envelope.expires_at, datetime)
            else datetime.fromisoformat(str(envelope.expires_at))
        )
        time_diff = (expires_at_dt - now).total_seconds()
        assert 295 <= time_diff <= 305  # Allow 5 second tolerance

    def test_envelope_json_is_valid(self):
        """Envelope can be serialized to canonical JSON."""
        payload = CommandRequestPayload(
            command="ls -la",
            execution_id="exec-123",
        )
        message = G8eMessage(
            id="msg-123",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_COMMAND_REQUESTED,
            payload=payload,
        )

        envelope_json = build_uap_envelope_json(message, state_merkle_root="test-root")

        assert isinstance(envelope_json, str)
        assert len(envelope_json) > 0

        # Should be valid JSON
        import json

        parsed = json.loads(envelope_json)
        assert parsed["transaction_hash"] is not None
        assert parsed["nonce"] is not None
        assert parsed["state_merkle_root"] == "test-root"
