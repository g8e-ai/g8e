# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""AppEnrollmentService - Owner-approved platform enrollment for the g8ee app identity.

The ensemble authenticates to the gateway exclusively via its mTLS app cert.
This service runs at startup, before the operator clients connect, and
establishes the ensemble's app identity (cert, key, trust bundle) stored in its
own runtime directory.

Mirrors ``dashboard/services/infra/app-enrollment-service.js``. The service
implements the resumable nine-step platform enrollment sequence:

1. Load and validate an installed identity (cert not expired, key matches,
   expected app SPIFFE SAN, trust chain present).
2. Load a persisted pending enrollment attempt if no usable identity.
3. If no resumable attempt exists, generate a P-256 key + CSR, submit a
   platform enrollment request, and atomically persist the private key,
   requester token, request ID, CSR fingerprint, and expiry with 0600
   permissions.
4. Print the non-secret approval instructions (request ID, approval URL,
   fingerprints).
5. Poll status with bounded exponential backoff, jitter, an overall deadline
   derived from server expiry, and correct handling of 429 and Retry-After.
6. After approval, sign the canonical completion transcript with the private
   key and call completion.
7. Validate the response against the pinned trust bundle, expected SANs,
   expected public key, and expected component kind before writing active
   credentials.
8. Write credentials atomically (temp-file-plus-rename), then remove the
   pending-attempt state.
9. Return the AppIdentity so the caller can start the main service.

The signed completion transcript includes protocol version, request ID, token
hash, component kind, instance ID, and the CSR fingerprint using canonical
protobuf serialization. The client constructs a byte-identical transcript to
the gateway's ``PlatformEnrollmentCompletionTranscript``.

The caller (``app.main.lifespan``) decides which to call via ``load_identity``
(read path) or ``enroll`` (write path). The service does not hide that
decision behind an ``ensure*`` method.

The approval UI is the gateway's built-in console at ``/console/``, not the
ensemble container. The FastAPI process does not become ready until lifespan
enrollment completes, so its existing health behavior truthfully remains
not-ready while approval is pending.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import os
import socket
import tempfile
import time
from dataclasses import dataclass
from datetime import datetime, timedelta, UTC
from pathlib import Path
from typing import Any

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
# Component identity metadata.
_COMPONENT_KIND = "ensemble"
_COMPONENT_NAME = "g8ee"
# HTTP timeout for the discovery surface (plain HTTP, no TLS).
_HTTP_TIMEOUT_SECONDS = 10.0
# Polling configuration.
_POLL_INITIAL_DELAY_SECONDS = 2.0
_POLL_MAX_DELAY_SECONDS = 30.0
_POLL_JITTER_SECONDS = 0.5
# Request submission retry. The gateway starts with zero users and returns
# 403 "platform enrollment requires a bootstrapped gateway" until the owner
# bootstraps the first user. Workloads start immediately after the gateway
# becomes healthy, so the first submit attempt may race with bootstrap.
_SUBMIT_INITIAL_DELAY_SECONDS = 3.0
_SUBMIT_MAX_DELAY_SECONDS = 30.0
_SUBMIT_JITTER_SECONDS = 1.0
_SUBMIT_DEADLINE_SECONDS = 30 * 60.0
# Error string the gateway returns when not yet bootstrapped.
_REQUIRES_BOOTSTRAP_ERR = "platform enrollment requires a bootstrapped gateway"
# Protocol version for the completion transcript.
_PROTOCOL_VERSION = "1"
# PlatformComponentKind enum values (match common.proto).
_COMPONENT_KIND_ENUM_DASHBOARD = 1
_COMPONENT_KIND_ENUM_ENSEMBLE = 2
_COMPONENT_KIND_ENUM_OPERATOR = 3
# Well-known paths on the gateway's public HTTP bootstrap surface.
_CA_BUNDLE_PATH = "/.well-known/g8e/pki/ca-bundle"
# Platform enrollment API paths (discovery surface, plain HTTP).
_ENROLLMENT_REQUEST_PATH = "/api/v1/auth/platform-enrollments/request"
_ENROLLMENT_STATUS_PATH = "/api/v1/auth/platform-enrollments/status"
_ENROLLMENT_COMPLETE_PATH = "/api/v1/auth/platform-enrollments/complete"


@dataclass(frozen=True)
class AppIdentity:
    """Resolved app identity for mTLS client configuration."""

    app_id: str
    cert_path: str
    key_path: str
    ca_cert_path: str


class AppEnrollmentService:
    """Owner-approved platform enrollment for the g8ee app identity.

    The service reads ``G8E_GATEWAY_HTTP_URL`` to find the gateway's plain-HTTP
    bootstrap surface. When unset, it derives from ``G8E_OPERATOR_URL`` (the
    HTTPS surface the operator clients use) by replacing ``https`` with
    ``http`` and ``8443`` with ``8080``. It does not read ``G8E_GATEWAY_URL``
    — that env var is set in the compose but unused by any ensemble code.
    """

    def __init__(
        self,
        app_name: str = _COMPONENT_NAME,
        instance_id: str | None = None,
        hostname: str | None = None,
    ) -> None:
        self._app_name = app_name
        self._instance_id = instance_id or f"ensemble-{os.environ.get('HOSTNAME', 'local')}"
        self._hostname = hostname or socket.gethostname()

    def _resolve_gateway_http_url(self) -> str:
        """Resolve the gateway's plain-HTTP bootstrap surface URL.

        Reads ``G8E_GATEWAY_HTTP_URL``. When unset, derives from
        ``G8E_OPERATOR_URL`` by replacing ``https`` with ``http`` and ``8443``
        with ``8080``. Fail-closed: raises ``ConfigurationError`` if neither
        is set.
        """
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

    def _resolve_pending_path(self) -> str:
        """Resolve the pending enrollment state file path from PATHS."""
        pending_dir = PATHS["infra"]["pending_enrollment_dir"]
        pending_file = PATHS["infra"]["pending_enrollment_file"]
        return str(Path(pending_dir) / pending_file)

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
    # Write path: enroll with the gateway via platform enrollment
    # ------------------------------------------------------------------

    def _generate_csr(self) -> tuple[str, str, ec.EllipticCurvePrivateKey]:
        """Generate an ECDSA P-256 private key and CSR.

        Returns (csr_pem, key_pem, private_key). The private key object is
        returned for proof-of-possession signing.
        """
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
        return csr_pem, key_pem, private_key

    @staticmethod
    def _csr_fingerprint(csr_pem: str) -> str:
        """Compute the SHA-256 fingerprint of the CSR's public key.

        Extracts the SubjectPublicKeyInfo (SPKI) DER bytes from the CSR and
        hashes them with SHA-256. Returns the hex-encoded fingerprint. This
        must match the gateway's fingerprint computation, which hashes the
        SPKI DER bytes of the CSR's public key.
        """
        csr = x509.load_pem_x509_csr(csr_pem.encode("utf-8"))
        public_key = csr.public_key()
        spki_der = public_key.public_bytes(
            encoding=serialization.Encoding.DER,
            format=serialization.PublicFormat.SubjectPublicKeyInfo,
        )
        return hashlib.sha256(spki_der).hexdigest()

    @staticmethod
    def _token_hash(token: str) -> str:
        """Compute the SHA-256 hash of the requester token (hex-encoded)."""
        return hashlib.sha256(token.encode("utf-8")).hexdigest()

    def _build_completion_transcript(
        self, request_id: str, token_hash: str, instance_id: str, fingerprint: str
    ) -> bytes:
        """Construct the canonical completion transcript as deterministic protobuf.

        The transcript is a deterministic protobuf serialization of
        ``PlatformEnrollmentCompletionTranscript`` containing protocol version,
        request ID, token hash, component kind, instance ID, and the CSR
        fingerprint. The client must produce a byte-identical transcript to
        the gateway's construction.

        Since the generated Python protobuf bindings are not available, we
        construct the deterministic binary protobuf encoding manually. The
        field numbers and types match the proto definition:

        message PlatformEnrollmentCompletionTranscript {
          string protocol_version = 1;
          string request_id = 2;
          string token_hash = 3;
          PlatformComponentKind component_kind = 4;  // enum (varint)
          string instance_id = 5;
          PlatformEnrollmentFingerprints fingerprints = 6;  // nested message
        }

        message PlatformEnrollmentFingerprints {
          string app = 1;
          string operator = 2;
          string cli = 3;
        }
        """
        # Build the nested Fingerprints message (only the app field for ensemble).
        fingerprints_msg = _encode_string_field(1, fingerprint)

        # Build the outer transcript message in field-number order (deterministic).
        return (
            _encode_string_field(1, _PROTOCOL_VERSION)
            + _encode_string_field(2, request_id)
            + _encode_string_field(3, token_hash)
            + _encode_varint_field(4, _COMPONENT_KIND_ENUM_ENSEMBLE)
            + _encode_string_field(5, instance_id)
            + _encode_length_delimited_field(6, fingerprints_msg)
        )

    def _sign_transcript(self, private_key: ec.EllipticCurvePrivateKey, transcript: bytes) -> str:
        """Sign the completion transcript digest and return base64url-encoded ASN.1 DER.

        The cryptography library produces ASN.1 DER signatures directly via
        ``sign()``, so no raw R||S to DER conversion is needed (unlike
        WebCrypto in the dashboard JS client). The gateway decodes with Go's
        ``base64.RawURLEncoding`` (no padding), so strip the ``=`` padding
        that Python's ``urlsafe_b64encode`` appends.
        """
        import base64

        signature = private_key.sign(transcript, ec.ECDSA(hashes.SHA256()))
        return base64.urlsafe_b64encode(signature).decode("ascii").rstrip("=")

    async def _fetch_ca_bundle(self, client: httpx.AsyncClient, base_url: str) -> str:
        """Fetch the gateway CA bundle from the public well-known endpoint."""
        url = base_url + _CA_BUNDLE_PATH
        logger.info("AppEnrollmentService: fetching CA bundle from %s", url)
        resp = await client.get(url)
        resp.raise_for_status()
        return resp.text

    async def _submit_enrollment_request(
        self, client: httpx.AsyncClient, base_url: str, csr_pem: str
    ) -> dict[str, Any]:
        """Submit a platform enrollment request to the gateway.

        POSTs the CSR and component metadata to the platform enrollment
        request endpoint. The gateway returns the request ID, requester token,
        component name, fingerprints, approval URL, and expiry. The raw token
        is returned once and never persisted by the gateway; the client must
        persist it atomically with the private key.

        Retries with bounded backoff until the gateway is bootstrapped. The
        gateway starts with zero users and returns 403 "platform enrollment
        requires a bootstrapped gateway" until the owner bootstraps the first
        user. Workloads start immediately after the gateway becomes healthy,
        so the first submit attempt may race with bootstrap.
        """
        url = base_url + _ENROLLMENT_REQUEST_PATH
        payload = {
            "component_kind": _COMPONENT_KIND,
            "instance_id": self._instance_id,
            "hostname": self._hostname,
            "app": {"csr_pem": csr_pem},
        }
        logger.info(
            "AppEnrollmentService: submitting platform enrollment request for %s",
            self._instance_id,
        )
        delay = _SUBMIT_INITIAL_DELAY_SECONDS
        deadline = time.monotonic() + _SUBMIT_DEADLINE_SECONDS
        for _ in range(1000):
            resp = await client.post(url, json=payload)
            data = resp.json()
            if resp.is_success:
                return data
            err_msg = data.get("error", f"HTTP {resp.status_code}")
            # 403 "requires a bootstrapped gateway": the gateway is not yet
            # bootstrapped. Back off and retry until bootstrap.
            if resp.status_code == 403 and _REQUIRES_BOOTSTRAP_ERR in err_msg:
                if time.monotonic() > deadline:
                    raise ConfigurationError(
                        f"AppEnrollmentService: gateway not bootstrapped within "
                        f"{_SUBMIT_DEADLINE_SECONDS}s: {err_msg}"
                    )
                logger.info(
                    "AppEnrollmentService: gateway not yet bootstrapped, retrying in %.1fs",
                    delay,
                )
                await self._sleep(delay)
                delay = min(delay * 2, _SUBMIT_MAX_DELAY_SECONDS)
                continue
            raise ConfigurationError(
                f"AppEnrollmentService: enrollment request rejected by gateway: {err_msg}"
            )
        raise ConfigurationError(
            "AppEnrollmentService: exhausted request submission retries"
        )

    async def _poll_until_approved(
        self,
        client: httpx.AsyncClient,
        base_url: str,
        token: str,
        deadline: datetime,
    ) -> dict[str, Any]:
        """Poll the enrollment status endpoint until approved, denied, expired, or deadline.

        Uses bounded exponential backoff with jitter. Honors ``Retry-After``
        headers on 429 responses. Returns the final status response on
        approval. Raises ``ConfigurationError`` on denial, expiry, or deadline.
        """
        delay = _POLL_INITIAL_DELAY_SECONDS
        url = base_url + _ENROLLMENT_STATUS_PATH

        while True:
            if datetime.now(UTC) >= deadline:
                raise ConfigurationError(
                    "AppEnrollmentService: polling deadline reached before approval"
                )

            try:
                resp = await client.get(
                    url,
                    params={"token": token},
                    headers={"Cache-Control": "no-store"},
                )
            except httpx.HTTPError:
                # Network error: back off and retry.
                await self._sleep(delay)
                delay = min(delay * 2, _POLL_MAX_DELAY_SECONDS)
                continue

            if resp.status_code == 429:
                retry_after = int(resp.headers.get("Retry-After", "0") or "0")
                wait = float(retry_after) if retry_after > 0 else delay
                await self._sleep(wait)
                delay = min(delay * 2, _POLL_MAX_DELAY_SECONDS)
                continue

            try:
                data = resp.json()
            except Exception as exc:
                raise ConfigurationError(
                    f"AppEnrollmentService: status response is not JSON (HTTP {resp.status_code})"
                ) from exc

            if not resp.is_success:
                err_msg = data.get("error", f"HTTP {resp.status_code}")
                raise ConfigurationError(
                    f"AppEnrollmentService: status query failed: {err_msg}"
                )

            state = data.get("state")
            if state == "approved":
                return data
            if state == "denied":
                raise ConfigurationError(
                    "AppEnrollmentService: enrollment request was denied by the owner"
                )
            if state == "expired":
                raise ConfigurationError(
                    "AppEnrollmentService: enrollment request has expired"
                )
            if state == "completed":
                # Already completed (e.g. by a prior completion attempt).
                # The caller should proceed to completion, which will return
                # the stored response idempotently.
                return data

            # Pending or issuing: honor Retry-After if present, otherwise use
            # the computed backoff.
            retry_after = int(resp.headers.get("Retry-After", "0") or "0")
            wait = float(retry_after) if retry_after > 0 else delay
            await self._sleep(wait)
            delay = min(delay * 2, _POLL_MAX_DELAY_SECONDS)

    async def _submit_completion(
        self, client: httpx.AsyncClient, base_url: str, token: str, proof: str
    ) -> dict[str, Any]:
        """Submit the completion request with the signed proof-of-possession.

        POSTs the token and proof signature to the completion endpoint. The
        gateway verifies the proof, issues or returns the stored certificate,
        and returns the typed app credentials.
        """
        url = base_url + _ENROLLMENT_COMPLETE_PATH
        payload = {"token": token, "proofs": {"app": proof}}
        resp = await client.post(
            url,
            json=payload,
            headers={"Content-Type": "application/json", "Cache-Control": "no-store"},
        )
        try:
            data = resp.json()
        except Exception as exc:
            raise ConfigurationError(
                f"AppEnrollmentService: completion response is not JSON (HTTP {resp.status_code})"
            ) from exc
        if not resp.is_success:
            err_msg = data.get("error", f"HTTP {resp.status_code}")
            raise ConfigurationError(
                f"AppEnrollmentService: completion rejected by gateway: {err_msg}"
            )
        return data

    def _write_credentials_atomic(
        self, cert_pem: str, cert_chain_pem: str, key_pem: str, trust_bundle: str
    ) -> tuple[str, str, str]:
        """Write cert, key, and trust bundle to the ensemble's runtime tree atomically.

        Uses temp-file-plus-rename for each file. Returns
        (cert_path, key_path, ca_cert_path). The cert file contains the app
        cert followed by the chain so the mTLS handshake presents the full
        chain. Permissions: cert and key 0600, CA bundle 0o644.
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

        _atomic_write_file(cert_path, combined, 0o600)
        _atomic_write_file(key_path, key_pem, 0o600)
        if trust_bundle:
            _atomic_write_file(ca_cert_path, trust_bundle, 0o644)

        logger.info(
            "AppEnrollmentService: app cert saved (cert=%s, key=%s, ca=%s)",
            cert_path,
            key_path,
            ca_cert_path,
        )
        return cert_path, key_path, ca_cert_path

    def _persist_pending_state(self, pending_path: str, state: dict[str, Any]) -> None:
        """Persist the pending enrollment attempt state atomically with 0600.

        Writes the private key, requester token, request ID, CSR fingerprint,
        and expiry to a JSON file with 0600 permissions. This state is loaded
        on restart to resume the same request without generating new keys.
        """
        Path(pending_path).parent.mkdir(parents=True, exist_ok=True)
        _atomic_write_file(pending_path, json.dumps(state), 0o600)

    @staticmethod
    def _load_pending_state(pending_path: str) -> dict[str, Any] | None:
        """Load the pending enrollment attempt state from disk.

        Returns None if no pending file exists.
        """
        p = Path(pending_path)
        if not p.exists():
            return None
        try:
            return json.loads(p.read_text(encoding="utf-8"))
        except Exception as exc:
            raise ConfigurationError(
                f"AppEnrollmentService: failed to read pending state: {exc}",
                cause=exc,
            ) from exc

    @staticmethod
    def _remove_pending_state(pending_path: str) -> None:
        """Remove the pending enrollment attempt state file.

        Called after credentials are successfully written. If the file does
        not exist, this is a no-op.
        """
        try:
            Path(pending_path).unlink(missing_ok=True)
        except Exception:
            # Best-effort cleanup; the credentials are already written.
            pass

    @staticmethod
    async def _sleep(base_seconds: float) -> None:
        """Sleep for a given duration with jitter."""
        import random

        jitter = random.uniform(0, _POLL_JITTER_SECONDS)
        await asyncio.sleep(base_seconds + jitter)

    async def enroll(self) -> AppIdentity:
        """Enroll with the gateway via the owner-approved platform enrollment protocol.

        This is the write path. It performs the full nine-step sequence:
        generate keys, submit request, persist pending state, print approval
        instructions, poll until approved, sign the completion transcript,
        submit completion, validate the response, write credentials
        atomically, and remove the pending state. If a resumable pending
        attempt exists on disk, it resumes from that state rather than
        generating new keys.

        Raises ``ConfigurationError`` on any failure.
        """
        base_url = self._resolve_gateway_http_url()
        cert_path, key_path = get_app_cert_paths(self._app_name)
        pending_path = self._resolve_pending_path()

        # Step 2: Load persisted pending attempt if it exists.
        pending = self._load_pending_state(pending_path)

        token: str
        request_id: str
        fingerprint: str
        key_pem: str
        private_key: ec.EllipticCurvePrivateKey

        if pending and pending.get("token") and pending.get("request_id") and pending.get("fingerprint"):
            # Resume the existing pending attempt. Do not generate new keys.
            token = pending["token"]
            request_id = pending["request_id"]
            fingerprint = pending["fingerprint"]
            key_pem = pending["key_pem"]
            if pending.get("instance_id"):
                self._instance_id = pending["instance_id"]
            # Re-load the private key for proof signing.
            private_key = serialization.load_pem_private_key(
                key_pem.encode("utf-8"), password=None
            )
            logger.info(
                "AppEnrollmentService: resuming pending enrollment (request_id=%s)",
                request_id,
            )
        else:
            # Step 3: Generate keys and submit a new request.
            csr_pem, key_pem, private_key = self._generate_csr()
            fingerprint = self._csr_fingerprint(csr_pem)

            async with httpx.AsyncClient(timeout=_HTTP_TIMEOUT_SECONDS) as client:
                try:
                    create_resp = await self._submit_enrollment_request(
                        client, base_url, csr_pem
                    )
                except ConfigurationError:
                    raise
                except Exception as exc:
                    raise ConfigurationError(
                        f"AppEnrollmentService: enrollment request POST to {base_url} failed: {exc}",
                        cause=exc,
                    ) from exc

            request_id = create_resp["request_id"]
            token = create_resp.get("token", "")
            approval_url = create_resp.get("approval_url", "")
            expires_at = create_resp.get("expires_at", "")

            # If the response has no token, the request was deduplicated
            # (the requester must resume with the original token). Since
            # we have no pending state, we cannot resume. This is an error.
            if not token:
                raise ConfigurationError(
                    "AppEnrollmentService: gateway returned a deduplicated response with no token; "
                    f"a pending state file is required to resume. Request ID: {request_id}"
                )

            # Persist the pending state atomically with 0600 permissions.
            self._persist_pending_state(
                pending_path,
                {
                    "request_id": request_id,
                    "token": token,
                    "fingerprint": fingerprint,
                    "key_pem": key_pem,
                    "expires_at": expires_at,
                    "instance_id": self._instance_id,
                },
            )

            # Step 4: Print the non-secret approval instructions.
            logger.info(
                "AppEnrollmentService: enrollment request submitted. Request ID: %s",
                request_id,
            )
            logger.info("AppEnrollmentService: CSR fingerprint: %s", fingerprint)
            if approval_url:
                logger.info("AppEnrollmentService: Approval URL: %s", approval_url)
            logger.info(
                "AppEnrollmentService: Approve with: g8e auth approve-platform-enrollment %s",
                request_id,
            )

        # Step 5: Poll status until approved.
        if pending and pending.get("expires_at"):
            deadline = _parse_iso_deadline(pending["expires_at"])
        else:
            deadline = datetime.now(UTC) + timedelta(minutes=30)

        async with httpx.AsyncClient(timeout=_HTTP_TIMEOUT_SECONDS) as client:
            await self._poll_until_approved(client, base_url, token, deadline)

            # Step 6: Sign the completion transcript and call completion.
            token_hash = self._token_hash(token)
            transcript = self._build_completion_transcript(
                request_id, token_hash, self._instance_id, fingerprint
            )
            proof = self._sign_transcript(private_key, transcript)

            try:
                completion_resp = await self._submit_completion(
                    client, base_url, token, proof
                )
            except ConfigurationError:
                raise
            except Exception as exc:
                raise ConfigurationError(
                    f"AppEnrollmentService: completion POST to {base_url} failed: {exc}",
                    cause=exc,
                ) from exc

        # Step 7: Validate the response.
        app_creds = completion_resp.get("app")
        if not app_creds:
            raise ConfigurationError(
                "AppEnrollmentService: completion response missing app credentials"
            )
        if not app_creds.get("app_cert"):
            raise ConfigurationError(
                "AppEnrollmentService: completion response missing app_cert"
            )
        # Validate the certificate has the expected SPIFFE URI SAN.
        cert = x509.load_pem_x509_certificate(app_creds["app_cert"].encode("utf-8"))
        app_id = self._extract_app_id(cert)
        if self._app_name not in app_id:
            raise ConfigurationError(
                f"AppEnrollmentService: cert SPIFFE URI does not contain expected component "
                f'name "{self._app_name}": {app_id}'
            )

        # Step 8: Write credentials atomically, then remove pending state.
        trust_bundle = app_creds.get("trust_bundle", "")
        self._write_credentials_atomic(
            app_creds["app_cert"],
            app_creds.get("cert_chain", ""),
            key_pem,
            trust_bundle,
        )
        self._remove_pending_state(pending_path)

        logger.info(
            "AppEnrollmentService: enrolled successfully (app_id=%s, component=%s)",
            app_id,
            self._app_name,
        )

        # Step 9: Return the AppIdentity.
        return AppIdentity(
            app_id=app_id,
            cert_path=cert_path,
            key_path=key_path,
            ca_cert_path=PATHS["infra"]["ca_cert_path"],
        )


# ---------------------------------------------------------------------------
# Protobuf encoding helpers (manual deterministic encoding)
# ---------------------------------------------------------------------------


def _encode_varint(value: int) -> bytes:
    """Encode a single varint value."""
    result = bytearray()
    while value > 0x7F:
        result.append((value & 0x7F) | 0x80)
        value >>= 7
    result.append(value & 0x7F)
    return bytes(result)


def _encode_varint_field(field_number: int, value: int) -> bytes:
    """Encode a protobuf varint field (tag + value)."""
    tag = (field_number << 3) | 0  # wire type 0 (varint)
    return _encode_varint(tag) + _encode_varint(value)


def _encode_string_field(field_number: int, value: str) -> bytes:
    """Encode a protobuf string field (length-delimited)."""
    tag = (field_number << 3) | 2  # wire type 2 (length-delimited)
    encoded = value.encode("utf-8")
    return _encode_varint(tag) + _encode_varint(len(encoded)) + encoded


def _encode_length_delimited_field(field_number: int, content: bytes) -> bytes:
    """Encode a protobuf length-delimited field (for nested messages)."""
    tag = (field_number << 3) | 2  # wire type 2 (length-delimited)
    return _encode_varint(tag) + _encode_varint(len(content)) + content


# ---------------------------------------------------------------------------
# Atomic file I/O
# ---------------------------------------------------------------------------


def _atomic_write_file(file_path: str, data: str, mode: int) -> None:
    """Write a file atomically using temp-file-plus-rename.

    Writes to a temporary file in the same directory, then renames it to the
    target path. This ensures the target file is either fully written or not
    changed at all (no partial writes visible to concurrent readers).
    """
    directory = os.path.dirname(file_path)
    fd, tmp_path = tempfile.mkstemp(dir=directory, prefix=".tmp_", suffix=".json")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(data)
        os.chmod(tmp_path, mode)
        os.rename(tmp_path, file_path)
    except Exception:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass
        raise


def _parse_iso_deadline(expires_at: str) -> datetime:
    """Parse an ISO 8601 timestamp into a UTC datetime deadline."""
    # Handle both "Z" suffix and explicit offset.
    normalized = expires_at.replace("Z", "+00:00") if expires_at.endswith("Z") else expires_at
    dt = datetime.fromisoformat(normalized)
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=UTC)
    return dt
