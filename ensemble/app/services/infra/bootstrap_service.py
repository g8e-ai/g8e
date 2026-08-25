# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import json
import logging
from pathlib import Path
from typing import Any, Protocol, cast, runtime_checkable

from app.constants.paths import PATHS
from app.utils.security import validate_safe_path

# Filename of the tamper-evidence manifest written by g8eo SecretManager
# alongside bootstrap secrets on the host bootstrap directory (.g8e/secrets). Must stay in sync with
# services/g8eo/services/listen/secret_manager.go::BootstrapDigestManifestFile.
BOOTSTRAP_DIGEST_MANIFEST_FILE = "bootstrap_digest.json"


class BootstrapSecretTamperError(RuntimeError):
    """Raised when a bootstrap secret loaded from the volume does not match
    the SHA-256 digest recorded by g8eo SecretManager in the tamper-evidence
    manifest. Callers must treat this as a hard startup error; authenticating
    with a drifted secret would only surface as a confusing 401 later."""


@runtime_checkable
class BootstrapServiceProtocol(Protocol):
    """Protocol for bootstrap services that load host bootstrap data."""

    def load_session_encryption_key(self) -> str | None:
        """Load session encryption key from host bootstrap directory."""
        ...

    def load_auditor_hmac_key(self) -> str | None:
        """Load Tribunal auditor HMAC-SHA256 signing key from host bootstrap directory."""
        ...

    def load_ca_cert_path(self) -> str | None:
        """Load CA certificate path from host bootstrap directory."""
        ...

    def is_available(self) -> bool:
        """Check if bootstrap data is available."""
        ...

    def verify_against_manifest(self, resource_name: str, value: str | None = None) -> None:
        """Verify a loaded secret's SHA-256 matches the digest g8eo recorded.

        Raises BootstrapSecretTamperError on divergence. No-op when the
        manifest is absent or lacks an entry for the given secret.
        """
        ...


class BootstrapService:
    """Service responsible for loading bootstrap data from host volumes.

    This service is ONLY responsible for loading values from host dot directories.
    It does not perform any settings management or configuration logic.
    """

    def __init__(self, secrets_dir: str | None = None, pki_dir: str | None = None) -> None:
        if secrets_dir is None:
            secrets_dir = PATHS["infra"]["secrets_dir"]
        self._secrets_dir = Path(secrets_dir)

        if pki_dir is None:
            pki_dir = PATHS["infra"]["pki_dir"]
        self._pki_dir = Path(pki_dir)

        self._logger = logging.getLogger(__name__)
        self._cached_key: str | None = None
        self._cached_auditor_hmac_key: str | None = None
        self._cached_ca_path: str | None = None

    def load_session_encryption_key(self) -> str | None:
        """Load session encryption key from host secrets directory."""
        if self._cached_key is not None:
            return self._cached_key

        try:
            key_path = validate_safe_path("session_encryption_key", self._secrets_dir)
            if key_path.exists():
                self._cached_key = key_path.read_text().strip()
                self._logger.info("Loaded session encryption key from host secrets directory")
                return self._cached_key
            self._logger.info("Session encryption key not found in host secrets directory")
            return None
        except Exception as e:
            self._logger.warning("Failed to read session encryption key: %s", e)
            return None

    def load_auditor_hmac_key(self) -> str | None:
        """Load Tribunal auditor HMAC-SHA256 signing key from host secrets directory.

        Paired with ``session_encryption_key``:
        the same SecretManager pattern on the g8eo side generates and
        persists this key, and the same bootstrap_digest.json entry is
        used for tamper verification by the caller.
        """
        if self._cached_auditor_hmac_key is not None:
            return self._cached_auditor_hmac_key

        try:
            key_path = validate_safe_path("auditor_hmac_key", self._secrets_dir)
            if key_path.exists():
                self._cached_auditor_hmac_key = key_path.read_text().strip()
                self._logger.info("Loaded auditor HMAC key from host secrets directory")
                return self._cached_auditor_hmac_key
            self._logger.info("Auditor HMAC key not found in host secrets directory")
            return None
        except Exception as e:
            self._logger.warning("Failed to read auditor HMAC key: %s", e)
            return None

    def load_ca_cert_path(self) -> str | None:
        """Load CA certificate path from host PKI directory."""
        if self._cached_ca_path is not None:
            return self._cached_ca_path

        # Check the canonical location in the PKI directory
        try:
            ca_path = validate_safe_path("trust/hub-bundle.pem", self._pki_dir)
            if ca_path.exists():
                self._cached_ca_path = str(ca_path)
                self._logger.info(
                    "Loaded CA cert path from host PKI directory: %s", self._cached_ca_path
                )
                return self._cached_ca_path
        except Exception as e:
            self._logger.warning("Failed to read CA cert: %s", e)

        self._logger.info("CA certificate not found in host PKI directory")
        return None

    def is_available(self) -> bool:
        """Check if bootstrap data is available."""
        return self._secrets_dir.exists() and (
            self.load_session_encryption_key() is not None
            or self.load_auditor_hmac_key() is not None
            or self.load_ca_cert_path() is not None
        )

    def clear_cache(self) -> None:
        """Clear cached values - useful for testing or re-initialization."""
        self._cached_key = None
        self._cached_auditor_hmac_key = None
        self._cached_ca_path = None

    def verify_against_manifest(self, resource_name: str, value: str | None = None) -> None:
        """Verify the encrypted secret file's SHA-256 matches the digest g8eo recorded.

        g8eo encrypts secrets with AES-256-GCM before writing to disk. The manifest
        contains SHA-256 digests of the encrypted file content, not the plaintext
        secrets. This verification ensures the encrypted file on disk has not been
        tampered with since g8eo wrote it.

        g8ee cannot decrypt secrets (no access to OS key store), so it reads the
        encrypted file content directly and hashes it for comparison against the manifest.

        Behaviour:
          * manifest missing -> log warning, return (transitional window
            before a g8eo with manifest support has booted);
          * manifest present, entry present, digest mismatch -> raise
            :class:`BootstrapSecretTamperError`;
          * manifest present, no entry for ``resource_name`` -> log warning,
            return.

        Args:
            resource_name: logical identifier of the bootstrap artifact (e.g. "session_encryption_key").
            value: ignored - we read the encrypted file directly for verification.

        Raises:
            BootstrapSecretTamperError: when the manifest has an entry for
                ``resource_name`` but the encrypted file digest does not match.
        """
        manifest_path = self._secrets_dir / BOOTSTRAP_DIGEST_MANIFEST_FILE
        if not manifest_path.exists():
            self._logger.warning(
                "Bootstrap digest manifest missing; skipping verification for %s (path=%s)",
                resource_name,
                manifest_path,
            )
            return

        try:
            raw_manifest: object = json.loads(manifest_path.read_text())
        except (OSError, ValueError) as err:
            raise BootstrapSecretTamperError(
                f"Bootstrap digest manifest at {manifest_path} is unreadable or malformed: {err}. "
                f"Refusing to start with an unverified {resource_name}."
            ) from err

        manifest = cast(dict[str, Any], raw_manifest) if isinstance(raw_manifest, dict) else {}
        entries_dict_raw = manifest.get("secrets")
        entries_dict = (
            cast(dict[str, Any], entries_dict_raw) if isinstance(entries_dict_raw, dict) else {}
        )
        entry_raw = entries_dict.get(resource_name)
        entry = cast(dict[str, Any], entry_raw) if isinstance(entry_raw, dict) else {}
        expected = entry.get("sha256") if isinstance(entry.get("sha256"), str) else None
        if not expected:
            self._logger.warning(
                "Bootstrap digest manifest has no entry for %s (manifest_version=%s)",
                resource_name,
                manifest.get("version"),
            )
            return

        # Read the encrypted file content and hash it (not the plaintext value)
        try:
            key_path = validate_safe_path(resource_name, self._secrets_dir)
            if not key_path.exists():
                raise BootstrapSecretTamperError(
                    f"Bootstrap secret file {resource_name} does not exist at {key_path}. "
                    f"Refusing to start with missing secret."
                )
            encrypted_content = key_path.read_bytes()
            actual = hashlib.sha256(encrypted_content).hexdigest()
        except Exception as err:
            raise BootstrapSecretTamperError(
                f"Failed to read encrypted secret file {resource_name} for verification: {err}. "
                f"Refusing to start with an unreadable secret."
            ) from err

        if actual != expected:
            raise BootstrapSecretTamperError(
                f"Bootstrap secret {resource_name} failed tamper-evidence check: "
                f"encrypted file SHA-256 {actual} does not match manifest digest {expected}. "
                f"The on-disk encrypted file has been tampered with or corrupted. "
                f"Refusing to start to avoid using a compromised secret."
            )

        self._logger.info(
            "Bootstrap secret %s verified against digest manifest (encrypted content)",
            resource_name,
        )
