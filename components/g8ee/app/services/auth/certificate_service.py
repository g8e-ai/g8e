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

from __future__ import annotations

import logging
import os
import datetime
from typing import Any, TYPE_CHECKING

if TYPE_CHECKING:
    from app.services.auth.certificate_data_service import CertificateDataService

from cryptography import x509
from cryptography.x509.oid import NameOID
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec

from app.constants.settings import (
    CLIENT_CERT_VALIDITY_DAYS,
    DEFAULT_SSL_DIR,
    CERT_SUBJECT_ORG,
    CERT_SUBJECT_COUNTRY,
    CRL_ISSUER
)

logger = logging.getLogger(__name__)

class CertificateService:
    """Certificate Service - Per-Operator mTLS Certificate Management.

    Authority: g8ee.
    Signs operator client certificates using the platform CA.

    TODO: Move CA private key operations behind a single host-side authority.
    App services should call a signing endpoint or consume pre-issued certificates
    rather than reading ca/ca.key directly. See g8es-host-transition-finalization.md.
    """

    def __init__(self, ssl_dir: str = DEFAULT_SSL_DIR, data_service: CertificateDataService | None = None):
        self.ssl_dir = ssl_dir
        self.data_service = data_service
        self.ca_cert: x509.Certificate | None = None
        self.ca_key: ec.EllipticCurvePrivateKey | None = None
        self.initialized = False
        self._revoked_serials: set[str] = set()

    async def initialize(self) -> None:
        """Load CA certificate from disk and load CRL from DB."""
        if self.initialized:
            return

        # 1. Load CRL from DB if data service is available
        if self.data_service:
            try:
                revocations = await self.data_service.get_all_revocations()
                for rev in revocations:
                    serial = rev.get("serial")
                    if serial:
                        self._revoked_serials.add(serial.upper())
                logger.info("[CERT-SERVICE] Loaded %s revocations from persistence", len(self._revoked_serials))
            except Exception as e:
                logger.error("[CERT-SERVICE] Failed to load revocations from persistence: %s", e)

        # 2. Load CA Certificate for local reference
        # Authority: g8es (Operator --listen mode)
        # We no longer read ca.key directly. Key operations are behind the /ssl/sign-certificate API.
        paths = [
            os.path.join(self.ssl_dir, "ca.crt"),
            os.path.join(self.ssl_dir, "ca", "ca.crt")
        ]

        found_cert_path = None
        for cert_path in paths:
            if os.path.exists(cert_path):
                found_cert_path = cert_path
                break

        if found_cert_path:
            try:
                with open(found_cert_path, "rb") as f:
                    self.ca_cert = x509.load_pem_x509_certificate(f.read())
                logger.info("[CERT-SERVICE] CA certificate loaded from %s", found_cert_path)
                self.initialized = True
            except Exception as e:
                logger.error("[CERT-SERVICE] Failed to load CA certificate: %s", e)
                raise RuntimeError(f"Failed to load CA cert: {e!s}") from e
        else:
            logger.error("[CERT-SERVICE] CA certificate not found in %s", self.ssl_dir)
            # We let it proceed but some operations might fail if they expect a CA cert local copy

    async def generate_operator_certificate(
        self,
        operator_id: str,
        user_id: str,
        organization_id: str
    ) -> dict[str, str]:
        """Request a new per-operator client certificate from g8es signing API."""
        if not self.initialized:
            await self.initialize()

        logger.info("[CERT-SERVICE] Requesting certificate for operator %s via g8es", operator_id)

        # Generate local private key
        private_key = ec.generate_private_key(ec.SECP384R1())
        public_key = private_key.public_key()
        public_key_pem = public_key.public_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PublicFormat.SubjectPublicKeyInfo
        ).decode("utf-8")

        # Authority: g8eo (/ssl/sign-certificate)
        # We use the DBClient's underlying session to reach the Operator's listen API
        from app.db.db_service import DBService
        from app.services.service_factory import AllServices

        # This is a bit of a shortcut, but service_factory provides access to everything.
        # In a cleaner world we'd inject a G8esClient, but DBClient already has the connection info.
        # We'll use the _request_json internal of db_client for this transition phase.
        
        db_client = self.data_service.cache.db.client # type: ignore
        
        payload = {
            "public_key_pem": public_key_pem,
            "common_name": operator_id,
            "organizational_unit": user_id,
            "validity_days": CLIENT_CERT_VALIDITY_DAYS
        }

        try:
            response = await db_client._request_json("POST", "/ssl/sign-certificate", json=payload)
            if not response or not response.get("success"):
                error_msg = response.get("error") if response else "Unknown error"
                raise RuntimeError(f"Failed to sign certificate via g8es: {error_msg}")

            cert_pem = str(response.get("certificate_pem"))
            serial = str(response.get("serial"))

            key_pem = private_key.private_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PrivateFormat.PKCS8,
                encryption_algorithm=serialization.NoEncryption(),
            ).decode("utf-8")

            return {
                "cert": cert_pem,
                "key": key_pem,
                "serial": serial,
            }
        except Exception as e:
            logger.error("[CERT-SERVICE] Certificate signing request failed: %s", e)
            raise RuntimeError(f"Certificate signing failed: {e!s}") from e

    async def revoke_certificate(self, serial: str, reason: str, operator_id: str | None = None) -> bool:
        """Revoke a certificate by serial number."""
        serial_upper = serial.upper()
        logger.info("[CERT-SERVICE] Revoking certificate %s (reason: %s, operator_id: %s)", serial_upper, reason, operator_id)

        # 1. Persist to DB
        if self.data_service:
            await self.data_service.revoke_certificate(serial_upper, reason, operator_id)

        # 2. Update memory
        self._revoked_serials.add(serial_upper)
        return True

    def is_revoked(self, serial: str) -> bool:
        """Check if a certificate serial is revoked."""
        return serial.upper() in self._revoked_serials

    def get_crl(self) -> dict[str, Any]:
        """Generate a basic CRL structure."""
        return {
            "version": 1,
            "issuer": CRL_ISSUER,
            "revoked_certificates": [{"serial": s} for s in self._revoked_serials],
        }

    async def cleanup(self) -> None:
        """Clear sensitive key material from memory."""
        self.ca_cert = None
        self.ca_key = None
        self._revoked_serials.clear()
        self.initialized = False
        logger.info("[CERT-SERVICE] Cleared sensitive key material from memory")
