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
from pathlib import Path
from typing import Protocol, runtime_checkable


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
