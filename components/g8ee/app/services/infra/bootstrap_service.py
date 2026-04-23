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

import hashlib
import json
import logging
from pathlib import Path
from typing import Any, Protocol, cast, runtime_checkable

# Filename of the tamper-evidence manifest written by g8eo SecretManager
# alongside bootstrap secrets on the SSL volume. Must stay in sync with
# components/g8eo/services/listen/secret_manager.go::BootstrapDigestManifestFile.
BOOTSTRAP_DIGEST_MANIFEST_FILE = "bootstrap_digest.json"


class BootstrapSecretTamperError(RuntimeError):
    """Raised when a bootstrap secret loaded from the volume does not match
    the SHA-256 digest recorded by g8eo SecretManager in the tamper-evidence
    manifest. Callers must treat this as a hard startup error; authenticating
    with a drifted secret would only surface as a confusing 401 later."""


@runtime_checkable
class BootstrapServiceProtocol(Protocol):
    """Protocol for bootstrap services that load g8es volume data."""

    def load_internal_auth_token(self) -> str | None:
        """Load internal auth token from g8es volume."""
        ...

    def load_session_encryption_key(self) -> str | None:
        """Load session encryption key from g8es volume."""
        ...

    def load_ca_cert_path(self) -> str | None:
        """Load CA certificate path from g8es volume."""
        ...

    def is_available(self) -> bool:
        """Check if bootstrap data is available."""
        ...

    def verify_against_manifest(self, secret_name: str, value: str | None) -> None:
        """Verify a loaded secret's SHA-256 matches the digest g8eo recorded.

        Raises BootstrapSecretTamperError on divergence. No-op when the
        manifest is absent or lacks an entry for the given secret.
        """
        ...


class BootstrapService:
    """Service responsible for loading bootstrap data from g8es volume.

    This service is ONLY responsible for loading values from the g8es data volume.
    It does not perform any settings management or configuration logic.
    """

    def __init__(self, volume_path: str = "/g8es") -> None:
        self._volume_path = Path(volume_path)
        self._logger = logging.getLogger(__name__)
        self._cached_token: str | None = None
        self._cached_key: str | None = None
        self._cached_ca_path: str | None = None

    def load_internal_auth_token(self) -> str | None:
        """Load internal auth token from g8es volume."""
        if self._cached_token is not None:
            return self._cached_token

        token_path = self._volume_path / "internal_auth_token"
        try:
            if token_path.exists():
                self._cached_token = token_path.read_text().strip()
                self._logger.info("Loaded internal auth token from g8es volume")
                return self._cached_token
            else:
                self._logger.info("Internal auth token not found in g8es volume")
                return None
        except Exception as e:
            self._logger.warning(f"Failed to read internal auth token: {e}")
            return None

    def load_session_encryption_key(self) -> str | None:
        """Load session encryption key from g8es volume."""
        if self._cached_key is not None:
            return self._cached_key

        key_path = self._volume_path / "session_encryption_key"
        try:
            if key_path.exists():
                self._cached_key = key_path.read_text().strip()
                self._logger.info("Loaded session encryption key from g8es volume")
                return self._cached_key
            else:
                self._logger.info("Session encryption key not found in g8es volume")
                return None
        except Exception as e:
            self._logger.warning(f"Failed to read session encryption key: {e}")
            return None

    def load_ca_cert_path(self) -> str | None:
        """Load CA certificate path from g8es volume."""
        if self._cached_ca_path is not None:
            return self._cached_ca_path

        # Check both possible locations
        ca_paths = [
            self._volume_path / "ca.crt",
            self._volume_path / "ca" / "ca.crt"
        ]

        for ca_path in ca_paths:
            try:
                if ca_path.exists():
                    self._cached_ca_path = str(ca_path)
                    self._logger.info(f"Loaded CA cert path from g8es volume: {self._cached_ca_path}")
                    return self._cached_ca_path
            except Exception as e:
                self._logger.warning(f"Failed to read CA cert at {ca_path}: {e}")

        self._logger.info("CA certificate not found in g8es volume")
        return None

    def is_available(self) -> bool:
        """Check if bootstrap data is available."""
        return (
            self._volume_path.exists() and
            (self.load_internal_auth_token() is not None or
             self.load_session_encryption_key() is not None or
             self.load_ca_cert_path() is not None)
        )

    def clear_cache(self) -> None:
        """Clear cached values - useful for testing or re-initialization."""
        self._cached_token = None
        self._cached_key = None
        self._cached_ca_path = None

    def verify_against_manifest(self, secret_name: str, value: str | None) -> None:
        """Verify a loaded secret's SHA-256 matches the digest g8eo recorded.

        Closes the silent bootstrap-secret coupling: without this, g8ee
        trusts whatever value the volume contains and any drift from the
        DB-authoritative value written by g8eo surfaces only as an opaque
        401 on the first downstream API call.

        Behaviour:
          * manifest missing -> log warning, return (transitional window
            before a g8eo with manifest support has booted);
          * manifest present, entry present, digest mismatch -> raise
            :class:`BootstrapSecretTamperError`;
          * manifest present, no entry for ``secret_name`` -> log warning,
            return.

        Args:
            secret_name: logical secret name
                (``internal_auth_token`` or ``session_encryption_key``).
            value: value loaded from the volume (already stripped).

        Raises:
            BootstrapSecretTamperError: when the manifest has an entry for
                ``secret_name`` but its digest does not match ``value``.
        """
        if not value:
            return

        manifest_path = Path(self._volume_path) / BOOTSTRAP_DIGEST_MANIFEST_FILE
        if not manifest_path.exists():
            self._logger.warning(
                "Bootstrap digest manifest missing; skipping verification for %s (path=%s)",
                secret_name,
                manifest_path,
            )
            return

        try:
            raw_manifest: object = json.loads(manifest_path.read_text())
        except (OSError, ValueError) as err:
            raise BootstrapSecretTamperError(
                f"Bootstrap digest manifest at {manifest_path} is unreadable or malformed: {err}. "
                f"Refusing to start with an unverified {secret_name}."
            ) from err

        manifest = cast(dict[str, Any], raw_manifest) if isinstance(raw_manifest, dict) else {}
        secrets_dict_raw = manifest.get("secrets")
        secrets_dict = cast(dict[str, Any], secrets_dict_raw) if isinstance(secrets_dict_raw, dict) else {}
        entry_raw = secrets_dict.get(secret_name)
        entry = cast(dict[str, Any], entry_raw) if isinstance(entry_raw, dict) else {}
        expected = entry.get("sha256") if isinstance(entry.get("sha256"), str) else None
        if not expected:
            self._logger.warning(
                "Bootstrap digest manifest has no entry for %s (manifest_version=%s)",
                secret_name,
                manifest.get("version"),
            )
            return

        actual = hashlib.sha256(value.encode("utf-8")).hexdigest()
        if actual != expected:
            raise BootstrapSecretTamperError(
                f"Bootstrap secret {secret_name} failed tamper-evidence check: "
                f"volume SHA-256 {actual} does not match manifest digest {expected}. "
                f"The on-disk secret has drifted from the DB-authoritative value. "
                f"Refusing to start to avoid authenticating with a divergent secret."
            )

        self._logger.info("Bootstrap secret %s verified against digest manifest", secret_name)
