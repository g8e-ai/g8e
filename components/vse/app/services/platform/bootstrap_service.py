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

"""
Bootstrap Service for VSE

This service is ONLY responsible for loading values from the VSODB data volume.
It does not perform any settings management or configuration logic.
"""

import os
from pathlib import Path
from typing import Optional
import logging

logger = logging.getLogger(__name__)


class BootstrapService:
    """Bootstrap service for loading VSODB volume data."""
    
    def __init__(self, volume_path: str = "/vsodb"):
        """
        Initialize bootstrap service.
        
        Args:
            volume_path: Path to VSODB volume (default: /vsodb)
        """
        self.volume_path = volume_path
        self._cached_token: Optional[str] = None
        self._cached_key: Optional[str] = None
        self._cached_ca_path: Optional[str] = None
    
    def load_internal_auth_token(self) -> Optional[str]:
        """
        Load internal auth token from VSODB volume.
        
        Returns:
            The internal auth token or None if not found
        """
        if self._cached_token is not None:
            return self._cached_token
        
        token_path = Path(self.volume_path) / "internal_auth_token"
        try:
            if token_path.exists():
                self._cached_token = token_path.read_text().strip()
                logger.info("[BOOTSTRAP-SERVICE] Loaded internal auth token from VSODB volume")
                return self._cached_token
            else:
                logger.info("[BOOTSTRAP-SERVICE] Internal auth token not found in VSODB volume")
                return None
        except Exception as err:
            logger.warning("[BOOTSTRAP-SERVICE] Failed to read internal auth token", 
                           extra={"error": str(err)})
            return None
    
    def load_session_encryption_key(self) -> Optional[str]:
        """
        Load session encryption key from VSODB volume.
        
        Returns:
            The session encryption key or None if not found
        """
        if self._cached_key is not None:
            return self._cached_key
        
        key_path = Path(self.volume_path) / "session_encryption_key"
        try:
            if key_path.exists():
                self._cached_key = key_path.read_text().strip()
                logger.info("[BOOTSTRAP-SERVICE] Loaded session encryption key from VSODB volume")
                return self._cached_key
            else:
                logger.info("[BOOTSTRAP-SERVICE] Session encryption key not found in VSODB volume")
                return None
        except Exception as err:
            logger.warning("[BOOTSTRAP-SERVICE] Failed to read session encryption key", 
                           extra={"error": str(err)})
            return None
    
    def load_ca_cert_path(self) -> Optional[str]:
        """
        Load CA certificate path from VSODB volume.
        
        Returns:
            The CA certificate path or None if not found
        """
        if self._cached_ca_path is not None:
            return self._cached_ca_path
        
        # Check both possible locations
        ca_paths = [
            Path(self.volume_path) / "ca.crt",
            Path(self.volume_path) / "ca" / "ca.crt"
        ]
        
        for ca_path in ca_paths:
            try:
                if ca_path.exists():
                    self._cached_ca_path = str(ca_path)
                    logger.info("[BOOTSTRAP-SERVICE] Loaded CA cert path from VSODB volume", 
                               extra={"path": self._cached_ca_path})
                    return self._cached_ca_path
            except Exception as err:
                logger.warning("[BOOTSTRAP-SERVICE] Failed to read CA cert", 
                               extra={"path": str(ca_path), "error": str(err)})
        
        logger.info("[BOOTSTRAP-SERVICE] CA certificate not found in VSODB volume")
        return None
    
    def get_ssl_dir(self) -> str:
        """
        Get SSL directory path.
        
        Returns:
            The SSL directory path
        """
        return self.volume_path
    
    def is_available(self) -> bool:
        """
        Check if bootstrap data is available.
        
        Returns:
            True if any bootstrap data is available
        """
        volume_path = Path(self.volume_path)
        if not volume_path.exists():
            return False
        
        return (
            self.load_internal_auth_token() is not None or
            self.load_session_encryption_key() is not None or
            self.load_ca_cert_path() is not None
        )
