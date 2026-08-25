# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tests for GovernanceClient operator transport identity binding.

The gateway's ``verifyEnvelopeIdentityBinding`` rejects mutation actions
(DOCUMENT_UPDATE, DOCUMENT_DELETE, FILE_EDIT) when the envelope lacks
both ``operator_id`` and ``operator_session_id``. The GovernanceClient
must resolve these from the operator mTLS certificate's SPIFFE URI SAN
and stamp them into every submitted envelope so the gateway can match
them against the transport identity.

See: .local.dev/docs/plans/in-progress/2026-08-23-ollama-ensemble-e2e-remaining-work.md
     BLOCKER: Ensemble governance envelope lacks operator transport identity
"""

import datetime
import json
import os
import tempfile
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.constants import EventType, G8EE_COMPONENT
from app.constants.config import G8EE_COMPONENT as G8EE_COMPONENT_STR
from app.models.command_request_payloads import DocumentUpdateRequestPayload
from app.models.pubsub_messages import G8eMessage

pytestmark = pytest.mark.unit


def _generate_operator_cert_with_spiffe(
    cert_path: str, key_path: str, spiffe_uri: str
) -> None:
    """Generate a self-signed ECDSA cert with a SPIFFE URI SAN and write PEM files."""
    from cryptography import x509
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import ec
    from cryptography.x509.oid import NameOID

    private_key = ec.generate_private_key(ec.SECP256R1())
    subject = issuer = x509.Name(
        [x509.NameAttribute(NameOID.COMMON_NAME, "test-operator")]
    )
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(private_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(minutes=1))
        .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1))
        .add_extension(
            x509.SubjectAlternativeName([x509.UniformResourceIdentifier(spiffe_uri)]),
            critical=False,
        )
        .sign(private_key, hashes.SHA256())
    )
    with open(cert_path, "wb") as f:
        f.write(cert.public_bytes(serialization.Encoding.PEM))
    with open(key_path, "wb") as f:
        f.write(
            private_key.private_bytes(
                serialization.Encoding.PEM,
                serialization.PrivateFormat.TraditionalOpenSSL,
                serialization.NoEncryption(),
            )
        )


@pytest.fixture
def operator_cert_tmpdir(tmp_path):
    """Create temp cert/key files with an operator SPIFFE URI SAN."""
    cert_path = str(tmp_path / "operator.crt")
    key_path = str(tmp_path / "operator.key")
    spiffe_uri = "spiffe://g8e.local/operator/test-org/test-op-id/test-op-session"
    _generate_operator_cert_with_spiffe(cert_path, key_path, spiffe_uri)
    return cert_path, key_path, spiffe_uri


@pytest.fixture
def operator_cert_no_spiffe(tmp_path):
    """Create temp cert/key files WITHOUT a SPIFFE URI SAN."""
    cert_path = str(tmp_path / "nosan.crt")
    key_path = str(tmp_path / "nosan.key")
    from cryptography import x509
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import ec
    from cryptography.x509.oid import NameOID

    private_key = ec.generate_private_key(ec.SECP256R1())
    subject = issuer = x509.Name(
        [x509.NameAttribute(NameOID.COMMON_NAME, "test-no-san")]
    )
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(private_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(minutes=1))
        .not_valid_after(datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1))
        .sign(private_key, hashes.SHA256())
    )
    with open(cert_path, "wb") as f:
        f.write(cert.public_bytes(serialization.Encoding.PEM))
    with open(key_path, "wb") as f:
        f.write(
            private_key.private_bytes(
                serialization.Encoding.PEM,
                serialization.PrivateFormat.TraditionalOpenSSL,
                serialization.NoEncryption(),
            )
        )
    return cert_path, key_path


class TestGovernanceClientOperatorBinding:
    """Verify GovernanceClient stamps operator_id + operator_session_id from the cert."""

    def test_resolve_operator_identity_from_cert(self, operator_cert_tmpdir):
        """_resolve_operator_identity_from_cert extracts operator_id and operator_session_id from SPIFFE URI SAN."""
        from app.clients.governance_client import GovernanceClient
        from app.models.settings import TLSConfig

        cert_path, key_path, spiffe_uri = operator_cert_tmpdir
        client = GovernanceClient(
            tls_config=TLSConfig(
                ca_cert_path=cert_path,
                client_cert_path=cert_path,
                client_key_path=key_path,
            ),
        )
        op_id, op_session = client._resolve_operator_identity_from_cert()
        assert op_id == "test-op-id"
        assert op_session == "test-op-session"

    def test_resolve_operator_identity_returns_none_when_no_cert(self):
        """_resolve_operator_identity_from_cert returns (None, None) when no client cert is configured."""
        from app.clients.governance_client import GovernanceClient

        client = GovernanceClient(tls_config=None)
        op_id, op_session = client._resolve_operator_identity_from_cert()
        assert op_id is None
        assert op_session is None

    def test_resolve_operator_identity_returns_none_when_no_spiffe_san(
        self, operator_cert_no_spiffe
    ):
        """_resolve_operator_identity_from_cert returns (None, None) when the cert has no URI SAN."""
        from app.clients.governance_client import GovernanceClient
        from app.models.settings import TLSConfig

        cert_path, key_path = operator_cert_no_spiffe
        client = GovernanceClient(
            tls_config=TLSConfig(
                ca_cert_path=cert_path,
                client_cert_path=cert_path,
                client_key_path=key_path,
            ),
        )
        op_id, op_session = client._resolve_operator_identity_from_cert()
        assert op_id is None
        assert op_session is None

    def test_resolve_operator_identity_caches_result(self, operator_cert_tmpdir):
        """_resolve_operator_identity_from_cert caches the parsed identity after the first call."""
        from app.clients.governance_client import GovernanceClient
        from app.models.settings import TLSConfig

        cert_path, key_path, _ = operator_cert_tmpdir
        client = GovernanceClient(
            tls_config=TLSConfig(
                ca_cert_path=cert_path,
                client_cert_path=cert_path,
                client_key_path=key_path,
            ),
        )
        # First call parses the cert
        op_id_1, op_session_1 = client._resolve_operator_identity_from_cert()
        assert op_id_1 == "test-op-id"
        # Second call returns cached values without re-reading the cert
        with patch("app.clients.governance_client.GovernanceClient._read_cert_spiffe_uri") as mock_read:
            op_id_2, op_session_2 = client._resolve_operator_identity_from_cert()
            mock_read.assert_not_called()
        assert op_id_2 == "test-op-id"
        assert op_session_2 == "test-op-session"

    @pytest.mark.asyncio
    async def test_submit_envelope_stamps_operator_identity_from_cert(
        self, operator_cert_tmpdir
    ):
        """submit_envelope stamps operator_id + operator_session_id from the cert when the message omits them.

        This reproduces and verifies the fix for the 403 blocker: without
        stamping, the gateway rejects mutation actions with
        ErrIdentityBindingFailed.
        """
        from app.clients.governance_client import GovernanceClient
        from app.models.settings import TLSConfig

        cert_path, key_path, spiffe_uri = operator_cert_tmpdir
        client = GovernanceClient(
            tls_config=TLSConfig(
                ca_cert_path=cert_path,
                client_cert_path=cert_path,
                client_key_path=key_path,
            ),
        )

        # Build a mutation message without operator_id/operator_session_id
        message = G8eMessage(
            id="test-doc-id",
            source_component=G8EE_COMPONENT_STR,
            event_type=EventType.APP_CASE_CREATED,
            case_id="test-case-id",
            user_id="test-user-id",
            payload=DocumentUpdateRequestPayload(
                collection="cases",
                document_id="test-doc-id",
                updates={"field": "value"},
                merge=False,
            ),
        )
        assert message.operator_id is None
        assert message.operator_session_id is None

        # Capture the envelope JSON submitted to the gateway
        captured_envelope: dict | None = None

        class FakeResponse:
            def __init__(self, status=200, text='{"status": "COMPLETED"}'):
                self.status = status
                self._text = text

            async def text(self):
                return self._text

            async def __aenter__(self):
                return self

            async def __aexit__(self, *args):
                pass

        class FakeSession:
            def post(self, url, data=None):
                nonlocal captured_envelope
                captured_envelope = json.loads(data)
                return FakeResponse()

        with patch.object(
            client, "_get_http_session", new=AsyncMock(return_value=FakeSession())
        ):
            with patch.object(
                client, "fetch_state_root", new=AsyncMock(return_value="test-root")
            ):
                await client.submit_envelope(message)

        assert captured_envelope is not None, "envelope was not submitted"
        assert captured_envelope.get("operator_id") == "test-op-id", (
            "envelope must carry operator_id from the cert SPIFFE URI SAN; "
            "without it the gateway rejects mutations with ErrIdentityBindingFailed"
        )
        assert captured_envelope.get("operator_session_id") == "test-op-session", (
            "envelope must carry operator_session_id from the cert SPIFFE URI SAN; "
            "without it the gateway rejects mutations with ErrIdentityBindingFailed"
        )

    @pytest.mark.asyncio
    async def test_submit_envelope_preserves_explicit_operator_identity(
        self, operator_cert_tmpdir
    ):
        """submit_envelope does not overwrite operator_id/operator_session_id when the message already carries them."""
        from app.clients.governance_client import GovernanceClient
        from app.models.settings import TLSConfig

        cert_path, key_path, _ = operator_cert_tmpdir
        client = GovernanceClient(
            tls_config=TLSConfig(
                ca_cert_path=cert_path,
                client_cert_path=cert_path,
                client_key_path=key_path,
            ),
        )

        message = G8eMessage(
            id="test-doc-id",
            source_component=G8EE_COMPONENT_STR,
            event_type=EventType.APP_CASE_CREATED,
            case_id="test-case-id",
            user_id="test-user-id",
            operator_id="explicit-op-id",
            operator_session_id="explicit-op-session",
            payload=DocumentUpdateRequestPayload(
                collection="cases",
                document_id="test-doc-id",
                updates={"field": "value"},
                merge=False,
            ),
        )

        captured_envelope: dict | None = None

        class FakeResponse:
            def __init__(self):
                self.status = 200

            async def text(self):
                return '{"status": "COMPLETED"}'

            async def __aenter__(self):
                return self

            async def __aexit__(self, *args):
                pass

        class FakeSession:
            def post(self, url, data=None):
                nonlocal captured_envelope
                captured_envelope = json.loads(data)
                return FakeResponse()

        with patch.object(
            client, "_get_http_session", new=AsyncMock(return_value=FakeSession())
        ):
            with patch.object(
                client, "fetch_state_root", new=AsyncMock(return_value="test-root")
            ):
                await client.submit_envelope(message)

        assert captured_envelope is not None
        assert captured_envelope.get("operator_id") == "explicit-op-id"
        assert captured_envelope.get("operator_session_id") == "explicit-op-session"
