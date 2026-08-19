# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for AppEnrollmentService.

Covers WS6 of the v2.0.0 ensemble app enrollment plan. The tests exercise the
6-step enrollment flow (check existing -> fetch CA bundle -> generate CSR ->
POST enroll -> write credentials -> return AppIdentity) and the reuse /
re-enrollment / failure paths, with HTTP traffic intercepted at the httpx
transport layer via `httpx.MockTransport` and filesystem state isolated to
`tmp_path` via `G8E_PKI_DIR` + `reload_paths()`.

The `g8e` protocol package must be installed (`pip install -e protocol/python`
from the repo root) for the conftest import to succeed — see
docs/ensemble/tests.md.
"""

from __future__ import annotations

import datetime as _dt
from pathlib import Path
from collections.abc import Callable

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
    pki_dir / app_cert_dir / ca_cert_path from G8E_PKI_DIR (or G8E_RUNTIME_DIR
    when PKI_DIR is unset); setting PKI_DIR directly keeps the test independent
    of the runtime-dir default and of any host-side .g8e state.
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
) -> tuple[str, str]:
    """Generate a self-signed ECDSA P-256 cert + key PEM pair.

    The cert is not a real gateway-signed app cert, but AppEnrollmentService
    only parses the not-after timestamp and the SPIFFE URI SAN (best-effort) —
    it does not verify the issuer chain on the reuse path. not_after must be
    timezone-aware (UTC).
    """
    key = ec.generate_private_key(ec.SECP256R1())
    subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, app_name)])
    san = x509.SubjectAlternativeName(
        [x509.UniformResourceIdentifier(f"spiffe://g8e.local/app/{app_name}")]
    )
    not_before = not_after - _dt.timedelta(days=30)
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(not_before)
        .not_valid_after(not_after)
        .add_extension(san, critical=False)
        .sign(key, hashes.SHA256())
    )
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

    AppEnrollmentService constructs its own `httpx.AsyncClient(timeout=...)`
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


def _enrollment_response(
    app_id: str = "spiffe://g8e.local/app/g8ee",
    cert_pem: str | None = None,
    cert_chain_pem: str = "",
    trust_bundle: str = "FAKE-CA-BUNDLE-PEM",
) -> dict:
    return {
        "success": True,
        "app_id": app_id,
        "app_cert": cert_pem or "FAKE-APP-CERT-PEM",
        "cert_chain": cert_chain_pem,
        "trust_bundle": trust_bundle,
        "expiry": "2099-01-01T00:00:00Z",
    }


# ---------------------------------------------------------------------------
# Tests: enroll_if_needed
# ---------------------------------------------------------------------------


class TestEnrollIfNeededNoExistingCert:
    """When no existing cert/key pair is present, the service enrolls."""

    pytestmark = pytest.mark.asyncio

    async def test_enrolls_and_writes_credentials(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        captured: list[httpx.Request] = []

        def handler(request: httpx.Request) -> httpx.Response:
            captured.append(request)
            if request.url.path == "/.well-known/g8e/pki/ca-bundle":
                return httpx.Response(200, text="CA-BUNDLE-PEM")
            if request.url.path == "/api/v1/pki/apps/enroll":
                return httpx.Response(200, json=_enrollment_response())
            return httpx.Response(404)

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService()
        identity = await service.enroll_if_needed()

        # AppIdentity points at the ensemble's own PKI tree.
        assert isinstance(identity, AppIdentity)
        assert identity.app_id == "spiffe://g8e.local/app/g8ee"
        assert identity.cert_path == str(pki_dir / "issued" / "apps" / "g8ee.crt")
        assert identity.key_path == str(pki_dir / "issued" / "apps" / "g8ee.key")
        assert identity.ca_cert_path == str(pki_dir / "trust" / "hub-bundle.pem")

        # Both HTTP calls fired: CA bundle fetch + enrollment POST.
        paths_hit = [r.url.path for r in captured]
        assert "/.well-known/g8e/pki/ca-bundle" in paths_hit
        assert "/api/v1/pki/apps/enroll" in paths_hit

        # The enrollment POST carried the app_name and a CSR.
        enroll_req = next(r for r in captured if r.url.path == "/api/v1/pki/apps/enroll")
        payload = enroll_req.read().decode("utf-8")
        assert "g8ee" in payload
        assert "BEGIN CERTIFICATE REQUEST" in payload

        # Credentials were written to disk with the expected content.
        cert_on_disk = Path(identity.cert_path).read_text(encoding="utf-8")
        assert "FAKE-APP-CERT-PEM" in cert_on_disk
        key_on_disk = Path(identity.key_path).read_text(encoding="utf-8")
        assert "BEGIN PRIVATE KEY" in key_on_disk
        ca_on_disk = Path(identity.ca_cert_path).read_text(encoding="utf-8")
        # The enrollment response's trust_bundle field takes precedence over
        # the well-known-fetched bundle (service: `response_bundle or trust_bundle`).
        assert ca_on_disk == "FAKE-CA-BUNDLE-PEM"


class TestEnrollIfNeededValidExistingCert:
    """When a valid, non-near-expiry cert already exists, the service reuses it."""

    pytestmark = pytest.mark.asyncio

    async def test_reuses_existing_cert_without_http_calls(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        # Existing cert expires well beyond the 7-day renewal threshold.
        not_after = _dt.datetime.now(_dt.UTC) + _dt.timedelta(days=90)
        cert_pem, key_pem = _self_signed_cert(not_after)
        cert_path, key_path = _write_existing_identity(pki_dir, cert_pem, key_pem)

        # If any HTTP call fires, the handler fails the test.
        def handler(_request: httpx.Request) -> httpx.Response:
            raise AssertionError("reuse path must not make HTTP calls")

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService()
        identity = await service.enroll_if_needed()

        assert identity.cert_path == cert_path
        assert identity.key_path == key_path
        assert identity.ca_cert_path == str(pki_dir / "trust" / "hub-bundle.pem")
        # app_id is best-effort extracted from the SPIFFE URI SAN.
        assert identity.app_id == "spiffe://g8e.local/app/g8ee"


class TestEnrollIfNeededNearExpiry:
    """When an existing cert is within the renewal threshold, the service re-enrolls."""

    pytestmark = pytest.mark.asyncio

    async def test_re_enrolls_when_cert_near_expiry(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        pki_dir = _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        # Existing cert expires in 3 days — inside the 7-day threshold.
        not_after = _dt.datetime.now(_dt.UTC) + _dt.timedelta(days=3)
        cert_pem, key_pem = _self_signed_cert(not_after)
        _write_existing_identity(pki_dir, cert_pem, key_pem)

        enrolled: list[bool] = []

        def handler(request: httpx.Request) -> httpx.Response:
            if request.url.path == "/.well-known/g8e/pki/ca-bundle":
                return httpx.Response(200, text="CA-BUNDLE-PEM")
            if request.url.path == "/api/v1/pki/apps/enroll":
                enrolled.append(True)
                return httpx.Response(200, json=_enrollment_response())
            return httpx.Response(404)

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService()
        await service.enroll_if_needed()

        assert enrolled, "expected re-enrollment to fire for a near-expiry cert"


class TestEnrollIfNeededFailures:
    """Failure paths raise ConfigurationError with a clear message."""

    pytestmark = pytest.mark.asyncio

    async def test_enrollment_rejected_raises_configuration_error(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        def handler(request: httpx.Request) -> httpx.Response:
            if request.url.path == "/.well-known/g8e/pki/ca-bundle":
                return httpx.Response(200, text="CA-BUNDLE-PEM")
            if request.url.path == "/api/v1/pki/apps/enroll":
                return httpx.Response(200, json={"success": False, "error": "bad csr"})
            return httpx.Response(404)

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="enrollment rejected"):
            await service.enroll_if_needed()

    async def test_ca_bundle_fetch_failure_raises_configuration_error(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        def handler(_request: httpx.Request) -> httpx.Response:
            # CA bundle endpoint fails; enrollment must not proceed.
            return httpx.Response(503, text="unavailable")

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="failed to fetch CA bundle"):
            await service.enroll_if_needed()

    async def test_enrollment_post_network_error_raises_configuration_error(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
    ) -> None:
        _isolate_pki_dir(monkeypatch, tmp_path)
        monkeypatch.setenv(EnvVar.GATEWAY_HTTP_URL, "http://g8e.local:8080")

        def handler(request: httpx.Request) -> httpx.Response:
            if request.url.path == "/.well-known/g8e/pki/ca-bundle":
                return httpx.Response(200, text="CA-BUNDLE-PEM")
            # Enrollment POST raises a transport error.
            raise httpx.ConnectError("connection refused")

        _patch_httpx_with_mock_transport(monkeypatch, handler)

        service = AppEnrollmentService()
        with pytest.raises(ConfigurationError, match="enrollment POST"):
            await service.enroll_if_needed()


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
