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
import logging
import secrets
from typing import TYPE_CHECKING, Any

from app.constants import DB_COLLECTION_API_KEYS, ApiKeyStatus
from app.models.api_keys import ApiKeyDocument
from app.services.cache.cache_aside import CacheAsideService
from app.utils.timestamp import now

if TYPE_CHECKING:
    from app.services.infra.settings_service import SettingsServiceProtocol

logger = logging.getLogger(__name__)

# Constants for hashing (aligned with client)
KEY_DERIVATION_ALGORITHM = "sha256"
API_KEY_HASH_LENGTH = 32

class ApiKeyService:
    """
    ApiKeyService for g8ee.
    Handles validation and lifecycle of API keys.
    """

    def __init__(self, cache_aside: CacheAsideService):
        self.cache = cache_aside
        self.collection = DB_COLLECTION_API_KEYS

    def make_doc_id(self, raw_material: str) -> str:
        """Generate a deterministic document ID from a raw API key."""
        return hashlib.sha256(raw_material.encode()).hexdigest()[:API_KEY_HASH_LENGTH]

    def generate_raw_key(self, prefix: str = "g8e_") -> str:
        """Generate a new raw API key.
        
        Default prefix is 'g8e_' matching canonical format from protocol/constants/api_key_patterns.json.
        For operator keys, use format: g8e_{8hex}_{64hex} (e.g., g8e_1a2b3c4d_...64hex...).
        For regular keys, use format: g8e_{64hex}.
        """
        return f"{prefix}{secrets.token_hex(32)}"

    async def validate_key(self, raw_key: str, system_fingerprint: str | None = None) -> tuple[bool, ApiKeyDocument | None, str | None]:
        """Validate a raw API key."""
        if not raw_key:
            return False, None, "API key is required"

        doc_id = self.make_doc_id(raw_key)
        data = await self.cache.get_document_with_cache(self.collection, doc_id)

        if not data:
            return False, None, "API key not found"

        doc = ApiKeyDocument.model_validate(data)

        if doc.status != ApiKeyStatus.ACTIVE:
            return False, doc, f"API key is {doc.status}"

        if doc.expires_at and doc.expires_at < now():
            return False, doc, "API key has expired"

        # Enforce fingerprint matching if established
        if doc.system_fingerprint and system_fingerprint and doc.system_fingerprint != system_fingerprint:
            logger.error(
                "[API-KEY-SERVICE] Fingerprint mismatch",
                extra={
                    "doc_id": doc_id[:8] + "...",
                    "expected": doc.system_fingerprint,
                    "received": system_fingerprint
                }
            )
            return False, doc, "Invalid system fingerprint"

        return True, doc, None

    async def issue_key(
        self,
        raw_key: str,
        user_id: str,
        organization_id: str | None = None,
        operator_id: str | None = None,
        client_name: str = "operator",
        permissions: list[str] | None = None,
        status: ApiKeyStatus = ApiKeyStatus.ACTIVE,
    ) -> bool:
        """Issue (create and store) a new API key."""
        try:
            doc_id = self.make_doc_id(raw_key)
            doc_data = {
                "user_id": user_id,
                "organization_id": organization_id,
                "operator_id": operator_id,
                "client_name": client_name,
                "permissions": permissions or [],
                "status": status,
                "created_at": now(),
                "last_used_at": None,
                "expires_at": None,
            }

            result = await self.cache.create_document(
                collection=self.collection,
                document_id=doc_id,
                data=doc_data,
            )

            if result.success:
                logger.info(
                    "[API-KEY-SERVICE] API key issued",
                    extra={"doc_id": doc_id[:8] + "...", "user_id": user_id}
                )
                return True
            logger.error("[API-KEY-SERVICE] Failed to issue API key: %s", result.error)
            return False

        except Exception as e:
            logger.error("[API-KEY-SERVICE] Failed to issue API key: %s", e)
            return False

    async def revoke_key(self, raw_key: str) -> bool:
        """Mark an API key as REVOKED in the api_keys collection.

        Returns True on success or if the key did not exist (idempotent).
        Returns False only on storage failure.
        """
        if not raw_key:
            return True
        doc_id = self.make_doc_id(raw_key)
        try:
            existing = await self.cache.get_document_with_cache(self.collection, doc_id)
            if not existing:
                return True
            await self.cache.update_document(
                collection=self.collection,
                document_id=doc_id,
                data={"status": ApiKeyStatus.REVOKED, "revoked_at": now()},
                merge=True,
            )
            logger.info(
                "[API-KEY-SERVICE] API key revoked",
                extra={"doc_id": doc_id[:8] + "..."},
            )
            return True
        except Exception as e:
            logger.error("[API-KEY-SERVICE] Failed to revoke API key: %s", e)
            return False

    async def issue_operator_key(
        self,
        raw_key: str,
        user_id: str,
        organization_id: str | None,
        operator_id: str,
        settings_service: SettingsServiceProtocol,
        client_name: str = "operator",
        permissions: list[str] | None = None,
    ) -> bool:
        """Issue an operator API key."""
        return await self.issue_key(
            raw_key=raw_key,
            user_id=user_id,
            organization_id=organization_id,
            operator_id=operator_id,
            client_name=client_name,
            permissions=permissions,
            status=ApiKeyStatus.ACTIVE,
        )

    async def rotate_operator_key(
        self,
        old_raw_key: str | None,
        new_raw_key: str,
        user_id: str,
        organization_id: str | None,
        operator_id: str,
        settings_service: SettingsServiceProtocol,
        client_name: str = "operator",
        permissions: list[str] | None = None,
    ) -> bool:
        """Rotate an operator API key.

        Issues the new key first so a failure leaves the OLD key authoritative.
        Only after the new key is fully in place is the old key revoked.
        """
        ok = await self.issue_operator_key(
            raw_key=new_raw_key,
            user_id=user_id,
            organization_id=organization_id,
            operator_id=operator_id,
            settings_service=settings_service,
            client_name=client_name,
            permissions=permissions,
        )
        if not ok:
            return False

        if old_raw_key and old_raw_key != new_raw_key:
            revoked = await self.revoke_key(old_raw_key)
            if not revoked:
                logger.warning(
                    "[API-KEY-SERVICE] Old operator API key revocation failed "
                    "after successful rotation; reconciliation will catch up",
                    extra={"operator_id": operator_id},
                )

        return True

    async def revoke_operator_key(
        self,
        raw_key: str,
        settings_service: SettingsServiceProtocol,
    ) -> bool:
        """Revoke an operator API key in the canonical store."""
        return await self.revoke_key(raw_key)

    async def record_usage(self, raw_key: str, system_fingerprint: str | None = None) -> None:
        """Update the last used timestamp of a key and establish fingerprint if missing."""
        try:
            doc_id = self.make_doc_id(raw_key)
            data = await self.cache.get_document_with_cache(self.collection, doc_id)
            if not data:
                return

            doc = ApiKeyDocument.model_validate(data)
            updates: dict[str, Any] = {"last_used_at": now()}

            # Establish fingerprint if not already set (immutable thereafter)
            if not doc.system_fingerprint and system_fingerprint:
                updates["system_fingerprint"] = system_fingerprint
                logger.info(
                    "[API-KEY-SERVICE] Established system fingerprint for API key",
                    extra={
                        "doc_id": doc_id[:8] + "...",
                        "system_fingerprint": system_fingerprint
                    }
                )

            await self.cache.update_document(
                collection=self.collection,
                document_id=doc_id,
                data=updates,
                merge=True
            )
        except Exception as e:
            logger.warning("[API-KEY-SERVICE] Failed to record usage: %s", e)
