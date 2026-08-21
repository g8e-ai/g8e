# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for AppEnrollmentService (owner-approved platform enrollment).

Covers the two explicit operations:

- ``load_identity()`` — read path: loads an existing cert/key pair from disk,
  validates expiry, extracts the SPIFFE app_id from the URI SAN. Raises
  ``ConfigurationError`` if missing, expired, near-expiry, or malformed.

- ``enroll()`` — write path: implements the nine-step platform enrollment
  sequence — generate keys, submit request, persist pending state, poll until
  approved, sign the completion transcript, submit completion, validate the
  response, write credentials atomically, remove pending state. Resumes from
  a persisted pending attempt without generating new keys.

HTTP traffic is intercepted at the httpx transport layer via
``httpx.MockTransport`` and filesystem state is isolated to ``tmp_path`` via
``G8E_PKI_DIR`` + ``reload_paths()``.
"""

from __future__ import annotations

import asyncio
import base64
import datetime as _dt
import json
import os
import stat
from collections.abc import Callable
from pathlib import Path

import httpx
import pytest

from app.constants import paths as paths_module
from app.constants.env_vars import EnvVar
from app.errors import ConfigurationError
from app.services.infra.app_enrollment_service import (
    AppEnrollmentService,
    AppIdentity,
)
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

pytestmark = pytest.mark.unit


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _isolate_pki_dir(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> Path:
    """Point PATHS at a tmp_path-rooted PKI tree and reload the paths cache.

    Returns the resolved pki_dir. The ensemble's PATHS resolution derives
    pki_dir / app_cert_dir / ca_cert_path / pending_enrollment_dir from
    G8E_PKI_DIR (or G8E_RUNTIME_DIR when PKI_DIR is unset); setting PKI_DIR
    directly keeps the test independent of the runtime-dir default and of any
    host-side .g8e state.
    """
    pki_dir = tmp_path / "pki"
    pki_dir.mkdir(parents=True, exist_ok=True)
    monkeypatch.setenv(EnvVar.PKI_DIR, str(pki_dir))
    monkeypatch.delenv(EnvVar.RUNTIME_DIR, raising=False)
    monkeypatch.delenv(EnvVar.CA_CERT_PATH, raising=False)
    paths_module.reload_paths()
    return pki_dir


def _self_signed_cert(
    not_after: _dt.datetime,
    app_name: str = "g8ee",
    include_san: bool = True,
) -> tuple[str, str]:
    """Generate a self-signed ECDSA P-256 cert + key PEM pair.

    The cert is not a real gateway-signed app cert, but AppEnrollmentService
    only parses the not-after timestamp and the SPIFFE URI SAN — it does not
    verify the issuer chain on the reuse path. not_after must be
    timezone-aware (UTC). When include_san is False, the cert is generated
    without a SubjectAlternativeName extension.
    """
    key = ec.generate_private_key(ec.SECP256R1())
    subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, app_name)])
    not_before = not_after - _dt.timedelta(days=30)
    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(not_before)
        .not_valid_after(not_after)
    )
    if include_san:
        san = x509.SubjectAlternativeName(
            [x509.UniformResourceIdentifier(f"spiffe://g8e.local/app/{app_name}")]
        )
        builder = builder.add_extension(san, critical=False)
    cert = builder.sign(key, hashes.SHA256())
    cert_pem = cert.public_bytes(serialization.Encoding.PEM).decode("utf-8")
    key_pem = key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    ).decode("utf-8")
    return cert_pem, key_pem


def _write_existing_identity(
    pki_dir: Path, cert_pem: str, key_pem: str, app_name: str = "g8ee"
) -> tuple[str, str]:
    """Write a pre-existing app cert/key pair into the isolated PKI tree."""
    app_cert_dir = pki_dir / "issued" / "apps"
    app_cert_dir.mkdir(parents=True, exist_ok=True)
    cert_path = app_cert_dir / f"{app_name}.crt"
    key_path = app_cert_dir / f"{app_name}.key"
    cert_path.write_text(cert_pem, encoding="utf-8")
    key_path.write_text(key_pem, encoding="utf-8")
    return str(cert_path), str(key_path)


def _patch_httpx_with_mock_transport(
    monkeypatch: pytest.MonkeyPatch, handler: Callable[[httpx.Request], httpx.Response]
) -> None:
    """Replace httpx.AsyncClient in the enrollment service module with a
    subclass that injects a MockTransport.

    AppEnrollmentService constructs its own ``httpx.AsyncClient(timeout=...)``
    internally, so the test cannot pass a transport through the public API.
    Patching the AsyncClient symbol used inside the service module is the
    narrowest interception point — it leaves the rest of the ensemble's httpx
    usage untouched.
    """
    mock_transport = httpx.MockTransport(handler)
    real_async_client = httpx.AsyncClient

    class _MockAsyncClient(real_async_client):  # type: ignore[misc]
        def __init__(self, *args, **kwargs):
            kwargs["transport"] = mock_transport
            super().__init__(*args, **kwargs)

    monkeypatch.setattr(
        "app.services.infra.app_enrollment_service.httpx.AsyncClient", _MockAsyncClient
    )


def _mock_platform_enrollment_handler(
    *,
    request_id: str = "test-req-123",
    token: str = "test-token-abc",
    app_cert: str = "FAKE-APP-CERT-PEM",
    cert_chain: str = "",
    trust_bundle: str = "CA-BUNDLE-PEM",
    state: str = "approved",
    deny: bool = False,
    expire: bool = False,
) -> tuple[Callable[[httpx.Request], httpx.Response], dict]:
    """Build a mock httpx handler for the platform enrollment flow.

    Returns (handler, captured) where ``captured`` is a dict tracking the
    requests made. The handler responds to the request, status, and completion
    endpoints. When ``deny`` or ``expire`` is True, the status endpoint
    returns a terminal state instead of ``approved``.
    """
    captured: dict = {"requests": [], "poll_count": 0, "request_submitted": False}
    expires_at = (_dt.datetime.now(_dt.UTC) + _dt.timedelta(minutes=30)).isoformat()

    def handler(request: httpx.Request) -> httpx.Response:
        captured["requests"].append(request)
        path = request.url.path

        if path == "/.well-known/g8e/pki/ca-bundle":
            return httpx.Response(200, text="CA-BUNDLE-PEM")

        if path == "/api/v1/auth/platform-enrollments/request":
            captured["request_submitted"] = True
            return httpx.Response(
                201,
                json={
                    "request_id": request_id,
                    "token": token,
                    "component_kind": "ensemble",
                    "component_name": "g8ee",
                    "fingerprints": {"app": "test-fp"},
                    "approval_url": f"https://gateway.local/console#platform-enrollment={request_id}",
                    "expires_at": expires_at,
                },
            )

        if path == "/api/v1/auth/platform-enrollments/status":
            captured["poll_count"] += 1
            if deny:
                return httpx.Response(
                    200,
                    json={
                        "request_id": request_id,
                        "component_kind": "ensemble",
                        "state": "denied",
                        "expires_at": expires_at,
                    },
                )
            if expire:
                return httpx.Response(
                    200,
                    json={
                        "request_id": request_id,
                        "component_kind": "ensemble",
                        "state": "expired",
                        "expires_at": expires_at,
                    },
                )
            # First poll: pending; subsequent: approved (or the specified state).
            if captured["poll_count"] < 2:
                return httpx.Response(
                    200,
                    json={
                        "request_id": request_id,
                        "component_kind": "ensemble",
                        "state": "pending",
                        "expires_at": expires_at,
                    },
                )
            return httpx.Response(
                200,
                json={
                    "request_id": request_id,
                    "component_kind": "ensemble",
                    "state": state,
                    "expires_at": expires_at,
                },
            )

        if path == "/api/v1/auth/platform-enrollments/complete":
            return httpx.Response(
                200,
                json={
                    "request_id": request_id,
                    "component_kind": "ensemble",
                    "app": {
                        "app_id": "spiffe://g8e.local/app/g8ee",
                        "app_cert": app_cert,
                        "cert_chain": cert_chain,
                        "trust_bundle": trust_bundle,
                        "expires_at": (
                            _dt.datetime.now(_dt.UTC) + _dt.timedelta(days=365)
                        ).isoformat(),
                        "policy_id": "test-policy-id",
                    },
                },
            )

        return httpx.Response(404)

    return handler, captured


def _file_mode(path: str) -> int:
    """Return the permission bits (0o777 mask) of a file."""
    return stat.S_IMODE(os.stat(path).st_mode)


# ---------------------------------------------------------------------------
# Tests: load_identity (read path)
# ---------------------------------------------------------------------------


class TestLoadIdentityValidCert:
    """load_identity returns an AppIdentity when a valid cert exists on disk."""

    def test_loads_existing_cert_without_http_calls(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)

        not_after = _dt.datetime.now(_dt.UTC) + _dt.timedelta(days=90)
        cert_pem, key_pem = _self_signed_cert(not_after)
        cert_path, key_path = _write_existing_identity(pki_dir, cert_pem, key_pem)

        service = AppEnrollmentService()
        identity = service.load_identity()

        assert identity.cert_path == cert_path
        assert identity.key_path == key_path
        assert identity.ca_cert_path == str(pki_dir / "trust" / "hub-bundle.pem")
        assert identity.app_id == "spiffe://g8e.local/app/g8ee"


class TestLoadIdentityMissingCert:
    """load_identity raises ConfigurationError when cert or key is missing."""

    def test_raises_when_cert_missing(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        _isolate_pki_dir(monkeypatch, tmp_path)

        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="app cert not found"):
            service.load_identity()

    def test_raises_when_key_missing(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)

        not_after = _dt.datetime.now(_dt.UTC) + _dt.timedelta(days=90)
        cert_pem, _key_pem = _self_signed_cert(not_after)
        cert_path, _ = _write_existing_identity(pki_dir, cert_pem, "dummy")
        Path(cert_path).with_suffix(".key").unlink()

        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="app key not found"):
            service.load_identity()


class TestLoadIdentityNearExpiry:
    """load_identity raises ConfigurationError when the cert is near expiry."""

    def test_raises_when_cert_near_expiry(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)

        not_after = _dt.datetime.now(_dt.UTC) + _dt.timedelta(days=3)
        cert_pem, key_pem = _self_signed_cert(not_after)
        _write_existing_identity(pki_dir, cert_pem, key_pem)

        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="within 7 days of expiry"):
            service.load_identity()


class TestLoadIdentityMalformedCert:
    """load_identity raises when the cert file cannot be parsed."""

    def test_raises_on_unparseable_cert(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)

        _write_existing_identity(pki_dir, "not a cert", "not a key")

        service = AppEnrollmentService()
        with pytest.raises(Exception):
            service.load_identity()


class TestLoadIdentityNoSan:
    """load_identity raises when the cert has no SPIFFE URI SAN."""

    def test_raises_when_cert_has_no_san(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)

        not_after = _dt.datetime.now(_dt.UTC) + _dt.timedelta(days=90)
        cert_pem, key_pem = _self_signed_cert(not_after, include_san=False)
        _write_existing_identity(pki_dir, cert_pem, key_pem)

        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="no SubjectAlternativeName"):
            service.load_identity()


# ---------------------------------------------------------------------------
# Tests: enroll (write path)
# ---------------------------------------------------------------------------


class TestEnrollPlatformEnrollment:
    """enroll drives the full platform enrollment protocol."""

    pytestmark = pytest.mark.asyncio

    async def test_enrolls_and_writes_credentials(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        cert_pem, _ = _self_signed_cert(_dt.datetime.now(_dt.UTC) + _dt.timedelta(days=365))
        handler, captured = _mock_platform_enrollment_handler(app_cert=cert_pem)
        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService(instance_id="ensemble-test-1", hostname="test.local")
        identity = await service.enroll()

        assert isinstance(identity, AppIdentity)
        assert identity.app_id == "spiffe://g8e.local/app/g8ee"
        assert identity.cert_path == str(pki_dir / "issued" / "apps" / "g8ee.crt")
        assert identity.key_path == str(pki_dir / "issued" / "apps" / "g8ee.key")
        assert identity.ca_cert_path == str(pki_dir / "trust" / "hub-bundle.pem")

        # All three platform enrollment endpoints were called.
        paths_hit = [r.url.path for r in captured["requests"]]
        assert "/api/v1/auth/platform-enrollments/request" in paths_hit
        assert "/api/v1/auth/platform-enrollments/status" in paths_hit
        assert "/api/v1/auth/platform-enrollments/complete" in paths_hit

        # Credentials were written to disk.
        cert_on_disk = Path(identity.cert_path).read_text(encoding="utf-8")
        assert "BEGIN CERTIFICATE" in cert_on_disk
        key_on_disk = Path(identity.key_path).read_text(encoding="utf-8")
        assert "BEGIN PRIVATE KEY" in key_on_disk
        ca_on_disk = Path(identity.ca_cert_path).read_text(encoding="utf-8")
        assert ca_on_disk == "CA-BUNDLE-PEM"

        # Pending state was removed after successful enrollment.
        pending_path = str(pki_dir / "pending-enrollment" / "g8ee.json")
        assert not Path(pending_path).exists()

    async def test_persists_credentials_with_0600_permissions(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        cert_pem, _ = _self_signed_cert(_dt.datetime.now(_dt.UTC) + _dt.timedelta(days=365))
        handler, _ = _mock_platform_enrollment_handler(app_cert=cert_pem)
        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService(instance_id="ensemble-test-2", hostname="test.local")
        await service.enroll()

        cert_path = str(pki_dir / "issued" / "apps" / "g8ee.crt")
        key_path = str(pki_dir / "issued" / "apps" / "g8ee.key")
        assert _file_mode(cert_path) == 0o600
        assert _file_mode(key_path) == 0o644 or _file_mode(key_path) == 0o600
        # The key must be 0600 (private).
        assert _file_mode(key_path) == 0o600

    async def test_persists_pending_state_with_0600_during_enrollment(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        """Verify the pending state file is created with 0600 permissions.

        We intercept the flow after the request is submitted but before
        polling completes by making the status endpoint hang on the first
        poll. This lets us inspect the pending state file on disk.
        """
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        cert_pem, _ = _self_signed_cert(_dt.datetime.now(_dt.UTC) + _dt.timedelta(days=365))
        pending_path_str = str(pki_dir / "pending-enrollment" / "g8ee.json")
        poll_event = asyncio.Event()

        def handler(request: httpx.Request) -> httpx.Response:
            path = request.url.path
            if path == "/.well-known/g8e/pki/ca-bundle":
                return httpx.Response(200, text="CA-BUNDLE-PEM")
            if path == "/api/v1/auth/platform-enrollments/request":
                return httpx.Response(
                    201,
                    json={
                        "request_id": "test-req-pending",
                        "token": "test-token-pending",
                        "component_kind": "ensemble",
                        "component_name": "g8ee",
                        "fingerprints": {"app": "test-fp"},
                        "approval_url": "https://gateway.local/console#platform-enrollment=test-req-pending",
                        "expires_at": (_dt.datetime.now(_dt.UTC) + _dt.timedelta(minutes=30)).isoformat(),
                    },
                )
            if path == "/api/v1/auth/platform-enrollments/status":
                # Signal that the pending state should now exist on disk.
                if not poll_event.is_set():
                    poll_event.set()
                    # Return pending so the flow continues.
                    return httpx.Response(
                        200,
                        json={
                            "request_id": "test-req-pending",
                            "component_kind": "ensemble",
                            "state": "pending",
                            "expires_at": (_dt.datetime.now(_dt.UTC) + _dt.timedelta(minutes=30)).isoformat(),
                        },
                    )
                return httpx.Response(
                    200,
                    json={
                        "request_id": "test-req-pending",
                        "component_kind": "ensemble",
                        "state": "approved",
                        "expires_at": (_dt.datetime.now(_dt.UTC) + _dt.timedelta(minutes=30)).isoformat(),
                    },
                )
            if path == "/api/v1/auth/platform-enrollments/complete":
                return httpx.Response(
                    200,
                    json={
                        "request_id": "test-req-pending",
                        "component_kind": "ensemble",
                        "app": {
                            "app_id": "spiffe://g8e.local/app/g8ee",
                            "app_cert": cert_pem,
                            "cert_chain": "",
                            "trust_bundle": "CA-BUNDLE-PEM",
                            "expires_at": (_dt.datetime.now(_dt.UTC) + _dt.timedelta(days=365)).isoformat(),
                            "policy_id": "test-policy-id",
                        },
                    },
                )
            return httpx.Response(404)

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService(instance_id="ensemble-pending-test", hostname="test.local")

        # Start enrollment in a task so we can inspect the pending file
        # after the first poll but before completion.
        enroll_task = asyncio.create_task(service.enroll())
        # Wait for the first poll to fire (pending state should be on disk).
        await asyncio.wait_for(poll_event.wait(), timeout=5.0)
        # Give the filesystem a moment to settle.
        await asyncio.sleep(0.05)

        assert Path(pending_path_str).exists(), "pending state file should exist during enrollment"
        assert _file_mode(pending_path_str) == 0o600

        # Let enrollment complete.
        identity = await asyncio.wait_for(enroll_task, timeout=10.0)
        assert isinstance(identity, AppIdentity)
        # Pending state removed after completion.
        assert not Path(pending_path_str).exists()

    async def test_resumes_from_persisted_pending_state_without_generating_new_keys(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        cert_pem, _ = _self_signed_cert(_dt.datetime.now(_dt.UTC) + _dt.timedelta(days=365))
        # Generate a real key pair for the pending state.
        _real_key = ec.generate_private_key(ec.SECP256R1())
        real_key_pem = _real_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        ).decode("utf-8")

        # Write a pending state file that simulates a prior request.
        pending_dir = pki_dir / "pending-enrollment"
        pending_dir.mkdir(parents=True, exist_ok=True)
        pending_path = pending_dir / "g8ee.json"
        pending_state = {
            "request_id": "resume-req-789",
            "token": "resume-token-ghi",
            "fingerprint": "resume-fingerprint",
            "key_pem": real_key_pem,
            "expires_at": (_dt.datetime.now(_dt.UTC) + _dt.timedelta(minutes=30)).isoformat(),
            "instance_id": "ensemble-resume-1",
        }
        pending_path.write_text(json.dumps(pending_state), encoding="utf-8")
        os.chmod(pending_path, 0o600)

        request_submitted = False

        def handler(request: httpx.Request) -> httpx.Response:
            nonlocal request_submitted
            path = request.url.path
            if path == "/.well-known/g8e/pki/ca-bundle":
                return httpx.Response(200, text="CA-BUNDLE")
            if path == "/api/v1/auth/platform-enrollments/request":
                request_submitted = True
                return httpx.Response(201, json={})
            if path == "/api/v1/auth/platform-enrollments/status":
                return httpx.Response(
                    200,
                    json={
                        "request_id": "resume-req-789",
                        "component_kind": "ensemble",
                        "state": "approved",
                        "expires_at": (_dt.datetime.now(_dt.UTC) + _dt.timedelta(minutes=30)).isoformat(),
                    },
                )
            if path == "/api/v1/auth/platform-enrollments/complete":
                return httpx.Response(
                    200,
                    json={
                        "request_id": "resume-req-789",
                        "component_kind": "ensemble",
                        "app": {
                            "app_id": "spiffe://g8e.local/app/g8ee",
                            "app_cert": cert_pem,
                            "cert_chain": "",
                            "trust_bundle": "CA-BUNDLE",
                            "expires_at": (_dt.datetime.now(_dt.UTC) + _dt.timedelta(days=365)).isoformat(),
                            "policy_id": "test-policy-id",
                        },
                    },
                )
            return httpx.Response(404)

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService(instance_id="ensemble-resume-1", hostname="test.local")
        identity = await service.enroll()

        assert isinstance(identity, AppIdentity)
        assert identity.app_id == "spiffe://g8e.local/app/g8ee"
        # No new request was submitted — the flow resumed from pending state.
        assert not request_submitted
        # Pending state removed after completion.
        assert not pending_path.exists()

    async def test_denial_raises_configuration_error(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        handler, _ = _mock_platform_enrollment_handler(deny=True)
        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService(instance_id="ensemble-deny-test", hostname="test.local")
        with pytest.raises(ConfigurationError, match="denied by the owner"):
            await service.enroll()

    async def test_expiry_raises_configuration_error(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        handler, _ = _mock_platform_enrollment_handler(expire=True)
        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService(instance_id="ensemble-expire-test", hostname="test.local")
        with pytest.raises(ConfigurationError, match="expired"):
            await service.enroll()

    async def test_request_rejection_raises_configuration_error(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        def handler(request: httpx.Request) -> httpx.Response:
            path = request.url.path
            if path == "/.well-known/g8e/pki/ca-bundle":
                return httpx.Response(200, text="CA-BUNDLE-PEM")
            if path == "/api/v1/auth/platform-enrollments/request":
                return httpx.Response(403, json={"error": "activation required"})
            return httpx.Response(404)

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService(instance_id="ensemble-reject-test", hostname="test.local")
        with pytest.raises(ConfigurationError, match="enrollment request rejected"):
            await service.enroll()

    async def test_completion_transcript_is_byte_identical_to_gateway_construction(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        """Verify the manually constructed transcript matches the gateway's
        deterministic protobuf encoding.

        The gateway uses ``proto.MarshalOptions{Deterministic: true}`` on a
        ``PlatformEnrollmentCompletionTranscript`` message. The Python client
        constructs the same bytes manually. This test verifies the encoding
        against known-good bytes computed from the proto field definitions.
        """
        service = AppEnrollmentService()

        transcript = service._build_completion_transcript(
            request_id="req-abc",
            token_hash="deadbeef",
            instance_id="ensemble-test",
            fingerprint="fp123",
        )

        # Manually decode the transcript to verify field order and values.
        # The expected encoding (deterministic, field-number order):
        #   field 1 (string): protocol_version = "1"
        #   field 2 (string): request_id = "req-abc"
        #   field 3 (string): token_hash = "deadbeef"
        #   field 4 (varint): component_kind = 2 (ENSEMBLE)
        #   field 5 (string): instance_id = "ensemble-test"
        #   field 6 (message): fingerprints { app = "fp123" }
        expected = (
            b"\x0a\x01\x31"  # field 1: "1"
            b"\x12\x07req-abc"  # field 2: "req-abc"
            b"\x1a\x08deadbeef"  # field 3: "deadbeef"
            b"\x20\x02"  # field 4: 2 (ENSEMBLE)
            b"\x2a\x0densemble-test"  # field 5: "ensemble-test"
            b"\x32\x07\x0a\x05fp123"  # field 6: fingerprints { app: "fp123" }
        )
        assert transcript == expected

    async def test_completion_proof_is_valid_ecdsa_der_signature(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        """Verify the proof signature is valid ASN.1 DER ECDSA and verifies
        against the transcript digest."""
        from cryptography.hazmat.primitives.asymmetric import utils

        service = AppEnrollmentService()
        private_key = ec.generate_private_key(ec.SECP256R1())

        transcript = service._build_completion_transcript(
            request_id="req-sig",
            token_hash="hash-sig",
            instance_id="ensemble-sig",
            fingerprint="fp-sig",
        )
        proof = service._sign_transcript(private_key, transcript)

        # Decode the base64url proof and verify it as an ASN.1 DER ECDSA signature.
        signature = base64.urlsafe_b64decode(proof)
        digest = hashes.Hash(hashes.SHA256())
        digest.update(transcript)
        digest_bytes = digest.finalize()

        # Verify the signature using the public key.
        public_key = private_key.public_key()
        public_key.verify(signature, digest_bytes, ec.ECDSA(utils.Prehashed(hashes.SHA256())))


# ---------------------------------------------------------------------------
# Tests: gateway HTTP URL resolution
# ---------------------------------------------------------------------------


class TestResolveGatewayHttpUrl:
    """G8E_GATEWAY_HTTP_URL is preferred; otherwise derive from G8E_OPERATOR_URL."""

    def test_uses_explicit_gateway_http_url(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080/")
        service = AppEnrollmentService()
        assert service._resolve_gateway_http_url() == "http://g8e.local:8080"

    def test_derives_from_operator_url_when_unset(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.delenv(EnvVar.GATEWAY_HTTP_URL, raising=False)
        monkeypatch.setenv(EnvVar.OPERATOR_URL, "https://g8e.local:8443")
        service = AppEnrollmentService()
        assert service._resolve_gateway_http_url() == "http://g8e.local:8080"

    def test_raises_when_neither_set(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.delenv(EnvVar.GATEWAY_HTTP_URL, raising=False)
        monkeypatch.delenv(EnvVar.OPERATOR_URL, raising=False)
        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="cannot resolve gateway HTTP URL"):
            service._resolve_gateway_http_url()

    def test_raises_when_operator_url_not_https_8443(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """An operator URL that is not https://...:8443 cannot be derived to a
        valid HTTP bootstrap URL — the derivation produces a non-http:// result
        and the service raises."""
        monkeypatch.delenv(EnvVar.GATEWAY_HTTP_URL, raising=False)
        monkeypatch.setenv(EnvVar.OPERATOR_URL, "ftp://g8e.local:21")
        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="cannot derive gateway HTTP URL"):
            service._resolve_gateway_http_url()
