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
from typing import Dict, Any, TYPE_CHECKING

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
    """

    def __init__(self, ssl_dir: str = DEFAULT_SSL_DIR, data_service: CertificateDataService | None = None):
        self.ssl_dir = ssl_dir
        self.data_service = data_service
        self.ca_cert: x509.Certificate | None = None
        self.ca_key: ec.EllipticCurvePrivateKey | None = None
        self.initialized = False
        self._revoked_serials: set[str] = set()

    async def initialize(self) -> None:
        """Load CA certificate and key from disk and load CRL from DB."""
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
                logger.info(f"[CERT-SERVICE] Loaded {len(self._revoked_serials)} revocations from persistence")
            except Exception as e:
                logger.error(f"[CERT-SERVICE] Failed to load revocations from persistence: {e}")

        # 2. Load CA
        paths = [
            (os.path.join(self.ssl_dir, "ca.crt"), os.path.join(self.ssl_dir, "ca.key")),
            (os.path.join(self.ssl_dir, "ca", "ca.crt"), os.path.join(self.ssl_dir, "ca", "ca.key"))
        ]

        found_cert_path = None
        found_key_path = None

        for cert_path, key_path in paths:
            if os.path.exists(cert_path) and os.path.exists(key_path):
                found_cert_path = cert_path
                found_key_path = key_path
                break

        if found_cert_path and found_key_path:
            try:
                with open(found_cert_path, "rb") as f:
                    self.ca_cert = x509.load_pem_x509_certificate(f.read())
                with open(found_key_path, "rb") as f:
                    self.ca_key = serialization.load_pem_private_key(f.read(), password=None)
                
                if not isinstance(self.ca_key, ec.EllipticCurvePrivateKey):
                    raise ValueError("CA key is not an EC key")

                logger.info(f"[CERT-SERVICE] CA certificate loaded from {found_cert_path}")
                self.initialized = True
            except Exception as e:
                logger.error(f"[CERT-SERVICE] Failed to load CA certificate or key: {str(e)}")
                raise RuntimeError(f"Failed to load CA: {str(e)}")
        else:
            logger.error(f"[CERT-SERVICE] CA certificate or key not found in {self.ssl_dir}")
            # In a production environment, this should probably be a hard error.
            # For now we log and let it fail on first use if not found.

    async def generate_operator_certificate(
        self, 
        operator_id: str, 
        user_id: str, 
        organization_id: str
    ) -> Dict[str, str]:
        """Generate and sign a new per-operator client certificate."""
        if not self.initialized:
            await self.initialize()

        if not self.ca_cert or not self.ca_key:
            raise RuntimeError("CertificateService not initialized with CA")

        logger.info(f"[CERT-SERVICE] Generating certificate for operator {operator_id}")

        # Generate private key
        private_key = ec.generate_private_key(ec.SECP384R1())
        public_key = private_key.public_key()

        # Build certificate
        builder = x509.CertificateBuilder()
        builder = builder.subject_name(x509.Name([
            x509.NameAttribute(NameOID.COMMON_NAME, operator_id),
            x509.NameAttribute(NameOID.ORGANIZATIONAL_UNIT_NAME, user_id),
            x509.NameAttribute(NameOID.ORGANIZATION_NAME, CERT_SUBJECT_ORG),
            x509.NameAttribute(NameOID.COUNTRY_NAME, CERT_SUBJECT_COUNTRY),
        ]))
        builder = builder.issuer_name(self.ca_cert.subject)
        builder = builder.not_valid_before(datetime.datetime.now(datetime.timezone.utc))
        builder = builder.not_valid_after(
            datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=CLIENT_CERT_VALIDITY_DAYS)
        )
        builder = builder.serial_number(x509.random_serial_number())
        builder = builder.public_key(public_key)
        
        # Extensions
        builder = builder.add_extension(
            x509.BasicConstraints(ca=False, path_length=None), critical=True
        )
        builder = builder.add_extension(
            x509.KeyUsage(
                digital_signature=True,
                content_commitment=False,
                key_encipherment=True,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=False,
                crl_sign=False,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        builder = builder.add_extension(
            x509.ExtendedKeyUsage([x509.oid.ExtendedKeyUsageOID.CLIENT_AUTH]),
            critical=False,
        )

        # Sign
        certificate = builder.sign(
            private_key=self.ca_key,
            algorithm=hashes.SHA384(),
        )

        # Serialize
        cert_pem = certificate.public_bytes(serialization.Encoding.PEM).decode("utf-8")
        key_pem = private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        ).decode("utf-8")

        return {
            "cert": cert_pem,
            "key": key_pem,
            "serial": hex(certificate.serial_number)[2:].upper(),
        }

    async def revoke_certificate(self, serial: str, reason: str, operator_id: str | None = None) -> bool:
        """Revoke a certificate by serial number."""
        serial_upper = serial.upper()
        logger.info(f"[CERT-SERVICE] Revoking certificate {serial_upper} (reason: {reason}, operator_id: {operator_id})")
        
        # 1. Persist to DB
        if self.data_service:
            await self.data_service.revoke_certificate(serial_upper, reason, operator_id)

        # 2. Update memory
        self._revoked_serials.add(serial_upper)
        return True

    def is_revoked(self, serial: str) -> bool:
        """Check if a certificate serial is revoked."""
        return serial.upper() in self._revoked_serials

    def get_crl(self) -> Dict[str, Any]:
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
