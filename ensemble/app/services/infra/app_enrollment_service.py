# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""AppEnrollmentService - Self-enrollment for the g8ee app identity.

The ensemble authenticates to the gateway exclusively via its mTLS app cert.
This service runs at startup, before the operator clients connect, and
establishes the ensemble's app identity (cert, key, trust bundle) stored in its
own runtime directory.

Per docs/g8e/guides/build_apps.md § Identity and Authentication, application
identity is established via an mTLS client certificate with SPIFFE-style URI
SANs. The gateway acts as the CA; there are no invite codes, pre-shared keys,
or manual approval steps. Starting the gateway is the platform owner's
authorization.

Two explicit operations, no hidden fallbacks:

- ``load_identity()`` — read path. Loads an existing cert/key pair from disk
  and validates that the cert is not expired. Raises ``ConfigurationError`` if
  the cert or key is missing, or if the cert is within the renewal threshold
  of expiry. Does not touch the network.

- ``enroll()`` — write path. Generates a CSR, enrolls with the gateway over
  HTTP, persists the cert/key/trust-bundle to disk, and returns an
  ``AppIdentity``. Always contacts the gateway; never reads cached state.

The caller (``app.main.lifespan``) decides which to call. The service does not
hide that decision behind an ``ensure*`` method.
"""

from __future__ import annotations

import logging
import os
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone, UTC
from pathlib import Path

import httpx
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import NameOID

from app.constants.env_vars import EnvVar
from app.constants.paths import PATHS, get_app_cert_paths
from app.errors import ConfigurationError

logger = logging.getLogger(__name__)

# Renew when the cert is within this many days of expiry.
_RENEWAL_THRESHOLD_DAYS = 7
# App identity metadata.
_APP_NAME = "g8ee"
_APP_TYPE = "custom"
# HTTP timeouts for the bootstrap surface (plain HTTP, no TLS).
_HTTP_TIMEOUT_SECONDS = 10.0
# Well-known paths on the gateway's public HTTP bootstrap surface.
_CA_BUNDLE_PATH = "/.well-known/g8e/pki/ca-bundle"
_ENROLL_PATH = "/api/v1/pki/apps/enroll"


@dataclass(frozen=True)
class AppIdentity:
    """Resolved app identity for mTLS client configuration."""

    app_id: str
    cert_path: str
    key_path: str
    ca_cert_path: str


class AppEnrollmentService:
    """Self-enrolls the g8ee app with the gateway at startup.

    The service reads `G8E_GATEWAY_HTTP_URL` to find the gateway's plain-HTTP
    bootstrap surface. When unset, it derives from `G8E_OPERATOR_URL` (the
    HTTPS surface the operator clients use) by replacing `https` with `http`
    and `8443` with `8080`. It does not read `G8E_GATEWAY_URL` — that env var
    is set in the compose but unused by any ensemble code.
    """

    def __init__(self, app_name: str = _APP_NAME, app_type: str = _APP_TYPE) -> None:
        self._app_name = app_name
        self._app_type = app_type

    def _resolve_gateway_http_url(self) -> str:
        """Resolve the gateway's plain-HTTP bootstrap surface URL."""
        explicit = os.environ.get(EnvVar.GATEWAY_HTTP_URL)
        if explicit:
            return explicit.rstrip("/")

        operator_url = os.environ.get(EnvVar.OPERATOR_URL)
        if not operator_url:
            raise ConfigurationError(
                "AppEnrollmentService cannot resolve gateway HTTP URL: "
                f"neither {EnvVar.GATEWAY_HTTP_URL} nor {EnvVar.OPERATOR_URL} is set"
            )
        derived = operator_url.replace("https://", "http://", 1).replace(":8443", ":8080", 1)
        if not derived.startswith("http://"):
            raise ConfigurationError(
                f"AppEnrollmentService cannot derive gateway HTTP URL from "
                f"{EnvVar.OPERATOR_URL}={operator_url!r}: expected an https://...:8443 URL"
            )
        return derived.rstrip("/")

    # ------------------------------------------------------------------
    # Read path: load an existing identity from disk
    # ------------------------------------------------------------------

    def load_identity(self) -> AppIdentity:
        """Load an existing app identity from disk and validate it.

        Reads the cert and key from the ensemble's PKI tree, parses the cert
        to check expiry, and extracts the SPIFFE app_id from the URI SAN.
        Raises ``ConfigurationError`` if the cert or key file is missing, if
        the cert cannot be parsed, if the cert is within the renewal
        threshold of expiry, or if the cert has no SPIFFE URI SAN.

        Does not touch the network.
        """
        cert_path, key_path = get_app_cert_paths(self._app_name)

        if not Path(cert_path).exists():
            raise ConfigurationError(
                f"AppEnrollmentService: app cert not found at {cert_path}"
            )
        if not Path(key_path).exists():
            raise ConfigurationError(
                f"AppEnrollmentService: app key not found at {key_path}"
            )

        cert = self._load_cert(cert_path)
        expiry = cert.not_valid_after_utc
        remaining = expiry - datetime.now(UTC)
        if remaining <= timedelta(days=_RENEWAL_THRESHOLD_DAYS):
            raise ConfigurationError(
                f"AppEnrollmentService: app cert at {cert_path} is within "
                f"{_RENEWAL_THRESHOLD_DAYS} days of expiry "
                f"(expires {expiry.isoformat()}, {remaining.days} days remaining)"
            )

        app_id = self._extract_app_id(cert)
        ca_cert_path = PATHS["infra"]["ca_cert_path"]

        logger.info(
            "AppEnrollmentService: loaded existing app cert (cert=%s, app_id=%s, expires=%s)",
            cert_path,
            app_id,
            expiry.isoformat(),
        )
        return AppIdentity(
            app_id=app_id,
            cert_path=cert_path,
            key_path=key_path,
            ca_cert_path=ca_cert_path,
        )

    @staticmethod
    def _load_cert(cert_path: str) -> x509.Certificate:
        """Load and parse a PEM cert from disk. Raises on any failure."""
        with open(cert_path, "rb") as fh:
            return x509.load_pem_x509_certificate(fh.read())

    @staticmethod
    def _extract_app_id(cert: x509.Certificate) -> str:
        """Extract the SPIFFE app_id from the cert's URI SAN.

        Raises ``ConfigurationError`` if the cert has no URI SAN.
        """
        try:
            san = cert.extensions.get_extension_for_class(x509.SubjectAlternativeName).value
        except x509.ExtensionNotFound as exc:
            raise ConfigurationError(
                "AppEnrollmentService: app cert has no SubjectAlternativeName extension"
            ) from exc
        uris = san.get_values_for_type(x509.UniformResourceIdentifier)
        if not uris:
            raise ConfigurationError(
                "AppEnrollmentService: app cert has no URI SAN (SPIFFE app_id)"
            )
        return str(uris[0])

    # ------------------------------------------------------------------
    # Write path: enroll with the gateway
    # ------------------------------------------------------------------

    def _generate_csr(self) -> tuple[str, str]:
        """Generate an ECDSA P-256 private key and CSR. Returns (csr_pem, key_pem)."""
        private_key = ec.generate_private_key(ec.SECP256R1())
        subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, self._app_name)])
        csr = (
            x509.CertificateSigningRequestBuilder()
            .subject_name(subject)
            .sign(private_key, hashes.SHA256())
        )
        csr_pem = csr.public_bytes(serialization.Encoding.PEM).decode("utf-8")
        key_pem = private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        ).decode("utf-8")
        return csr_pem, key_pem

    def _write_credentials(
        self, cert_pem: str, cert_chain_pem: str, key_pem: str, trust_bundle: str
    ) -> tuple[str, str, str]:
        """Write cert, key, and trust bundle to the ensemble's own runtime tree.

        Returns (cert_path, key_path, ca_cert_path). The cert file contains the
        app cert followed by the chain so the mTLS handshake presents the full
        chain. The trust bundle is written to the path `PATHS["infra"]["ca_cert_path"]`
        resolves to (default `runtime_dir/pki/trust/hub-bundle.pem`), which is
        the filename the ensemble's `SettingsService.ca_cert_path` property and
        `PATHS` resolution expect — not the gateway's `g8eg-ca-bundle.pem`.
        """
        cert_path, key_path = get_app_cert_paths(self._app_name)
        ca_cert_path = PATHS["infra"]["ca_cert_path"]

        cert_dir = Path(cert_path).parent
        cert_dir.mkdir(parents=True, exist_ok=True)
        ca_dir = Path(ca_cert_path).parent
        ca_dir.mkdir(parents=True, exist_ok=True)

        # cert file = app cert + chain
        combined = cert_pem
        if cert_chain_pem and cert_chain_pem not in combined:
            combined = combined.rstrip() + "\n" + cert_chain_pem.lstrip()
        Path(cert_path).write_text(combined, encoding="utf-8")
        os.chmod(cert_path, 0o600)

        Path(key_path).write_text(key_pem, encoding="utf-8")
        os.chmod(key_path, 0o600)

        if trust_bundle:
            Path(ca_cert_path).write_text(trust_bundle, encoding="utf-8")
            os.chmod(ca_cert_path, 0o644)

        logger.info(
            "AppEnrollmentService: app cert saved (cert=%s, key=%s, ca=%s)",
            cert_path,
            key_path,
            ca_cert_path,
        )
        return cert_path, key_path, ca_cert_path

    async def _fetch_ca_bundle(self, client: httpx.AsyncClient, base_url: str) -> str:
        """Fetch the gateway CA bundle from the public well-known endpoint."""
        url = base_url + _CA_BUNDLE_PATH
        logger.info("AppEnrollmentService: fetching CA bundle from %s", url)
        resp = await client.get(url)
        resp.raise_for_status()
        return resp.text

    async def _submit_enrollment(
        self, client: httpx.AsyncClient, base_url: str, csr_pem: str
    ) -> dict:
        """Submit the CSR to the gateway's public app enrollment endpoint."""
        url = base_url + _ENROLL_PATH
        payload = {
            "csr_pem": csr_pem,
            "app_name": self._app_name,
            "app_type": self._app_type,
        }
        logger.info("AppEnrollmentService: submitting enrollment for app=%s", self._app_name)
        resp = await client.post(url, json=payload)
        resp.raise_for_status()
        data = resp.json()
        if not data.get("success"):
            raise ConfigurationError(
                f"AppEnrollmentService: enrollment rejected by gateway: {data.get('error', 'unknown error')}"
            )
        return data

    async def enroll(self) -> AppIdentity:
        """Enroll with the gateway and persist the resulting credentials.

        Generates a CSR, fetches the CA bundle, POSTs the enrollment request,
        writes the cert/key/trust-bundle to disk, and returns an
        ``AppIdentity``. Always contacts the gateway; never reads cached state.
        Raises ``ConfigurationError`` on any failure.
        """
        base_url = self._resolve_gateway_http_url()
        csr_pem, key_pem = self._generate_csr()

        try:
            async with httpx.AsyncClient(timeout=_HTTP_TIMEOUT_SECONDS) as client:
                try:
                    trust_bundle = await self._fetch_ca_bundle(client, base_url)
                except Exception as exc:
                    raise ConfigurationError(
                        f"AppEnrollmentService: failed to fetch CA bundle from {base_url}: {exc}",
                        cause=exc,
                    ) from exc

                try:
                    enrollment = await self._submit_enrollment(client, base_url, csr_pem)
                except ConfigurationError:
                    raise
                except Exception as exc:
                    raise ConfigurationError(
                        f"AppEnrollmentService: enrollment POST to {base_url} failed: {exc}",
                        cause=exc,
                    ) from exc
        except ConfigurationError:
            raise

        cert_pem = enrollment.get("app_cert", "")
        cert_chain_pem = enrollment.get("cert_chain", "")
        # The gateway's response trust_bundle may be empty (non-fatal on the
        # gateway side); prefer the freshly-fetched well-known bundle when the
        # response omits it.
        response_bundle = enrollment.get("trust_bundle", "")
        trust_bundle = response_bundle or trust_bundle
        app_id = enrollment.get("app_id", "")

        if not cert_pem:
            raise ConfigurationError(
                "AppEnrollmentService: enrollment response missing app_cert field"
            )

        cert_path, key_path, ca_cert_path = self._write_credentials(
            cert_pem, cert_chain_pem, key_pem, trust_bundle
        )

        logger.info(
            "AppEnrollmentService: enrolled successfully (app_id=%s, app_name=%s)",
            app_id,
            self._app_name,
        )
        return AppIdentity(
            app_id=app_id,
            cert_path=cert_path,
            key_path=key_path,
            ca_cert_path=ca_cert_path,
        )
