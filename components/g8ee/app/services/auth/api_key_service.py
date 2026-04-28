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
from typing import TYPE_CHECKING

from app.constants import DB_COLLECTION_API_KEYS
from app.models.api_keys import ApiKeyDocument
from app.services.cache.cache_aside import CacheAsideService
from app.utils.timestamp import now

if TYPE_CHECKING:
    from app.services.infra.settings_service import SettingsServiceProtocol

logger = logging.getLogger(__name__)

# Constants for hashing (aligned with g8ed)
API_KEY_HASH_ALGORITHM = "sha256"
API_KEY_HASH_LENGTH = 32

API_KEY_STATUS_ACTIVE = "ACTIVE"
API_KEY_STATUS_REVOKED = "REVOKED"

class ApiKeyService:
    """
    ApiKeyService for g8ee.
    Handles validation and lifecycle of API keys.
    """

    def __init__(self, cache_aside: CacheAsideService):
        self.cache = cache_aside
        self.collection = DB_COLLECTION_API_KEYS

    def make_doc_id(self, api_key: str) -> str:
        """Generate a deterministic document ID from a raw API key."""
        return hashlib.sha256(api_key.encode()).hexdigest()[:API_KEY_HASH_LENGTH]

    def generate_raw_key(self, prefix: str = "g8e_") -> str:
        """Generate a new raw API key.
        
        Default prefix is 'g8e_' matching canonical format from shared/constants/api_key_patterns.json.
        For operator keys, use format: g8e_{8hex}_{64hex} (e.g., g8e_1a2b3c4d_...64hex...).
        For regular keys, use format: g8e_{64hex}.
        """
        import secrets
        return f"{prefix}{secrets.token_hex(32)}"

    async def validate_key(self, api_key: str) -> tuple[bool, ApiKeyDocument | None, str | None]:
        """Validate a raw API key."""
        if not api_key:
            return False, None, "API key is required"

        doc_id = self.make_doc_id(api_key)
        data = await self.cache.get_document_with_cache(self.collection, doc_id)

        if not data:
            return False, None, "API key not found"

        doc = ApiKeyDocument.model_validate(data)

        if doc.status != "ACTIVE":
            return False, doc, f"API key is {doc.status}"

        if doc.expires_at and doc.expires_at < now():
            return False, doc, "API key has expired"

        return True, doc, None

    async def issue_key(
        self,
        api_key: str,
        user_id: str,
        organization_id: str | None = None,
        operator_id: str | None = None,
        client_name: str = "operator",
        permissions: list[str] | None = None,
        status: str = "ACTIVE",
    ) -> bool:
        """Issue (create and store) a new API key."""
        try:
            doc_id = self.make_doc_id(api_key)
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
                    f"[API-KEY-SERVICE] API key issued",
                    extra={"doc_id": doc_id[:8] + "...", "user_id": user_id}
                )
                return True
            else:
                logger.error(f"[API-KEY-SERVICE] Failed to issue API key: {result.error}")
                return False

        except Exception as e:
            logger.error(f"[API-KEY-SERVICE] Failed to issue API key: {e}")
            return False

    async def revoke_key(self, api_key: str) -> bool:
        """Mark an API key as REVOKED in the api_keys collection.

        Returns True on success or if the key did not exist (idempotent).
        Returns False only on storage failure.
        """
        if not api_key:
            return True
        doc_id = self.make_doc_id(api_key)
        try:
            existing = await self.cache.get_document_with_cache(self.collection, doc_id)
            if not existing:
                return True
            await self.cache.update_document(
                collection=self.collection,
                document_id=doc_id,
                data={"status": API_KEY_STATUS_REVOKED, "revoked_at": now()},
                merge=True,
            )
            logger.info(
                "[API-KEY-SERVICE] API key revoked",
                extra={"doc_id": doc_id[:8] + "..."},
            )
            return True
        except Exception as e:
            logger.error(f"[API-KEY-SERVICE] Failed to revoke API key: {e}")
            return False

    async def issue_operator_key(
        self,
        api_key: str,
        user_id: str,
        organization_id: str | None,
        operator_id: str,
        is_g8ep: bool,
        settings_service: "SettingsServiceProtocol",
        client_name: str = "operator",
        permissions: list[str] | None = None,
    ) -> bool:
        """Issue an operator API key with canonical-first dual-write.

        1. Writes the key to the ``api_keys`` collection (canonical).
        2. If ``is_g8ep`` is True, mirrors the key to
           ``platform_settings.g8ep_operator_api_key`` so g8ep's
           ``fetch-key-and-run.sh`` can retrieve it on bootstrap.
        3. If step 2 fails, the canonical record is rolled back to REVOKED
           so we never leave a key authoritative-without-mirror.
        """
        issued = await self.issue_key(
            api_key=api_key,
            user_id=user_id,
            organization_id=organization_id,
            operator_id=operator_id,
            client_name=client_name,
            permissions=permissions,
            status=API_KEY_STATUS_ACTIVE,
        )
        if not issued:
            return False

        if not is_g8ep:
            return True

        try:
            await settings_service.update_g8ep_operator_api_key(api_key)
            return True
        except Exception as e:
            logger.error(
                "[API-KEY-SERVICE] Failed to mirror g8ep operator API key to "
                "platform_settings; rolling back api_keys entry",
                extra={"operator_id": operator_id, "error": str(e)},
            )
            await self.revoke_key(api_key)
            return False

    async def rotate_operator_key(
        self,
        old_api_key: str | None,
        new_api_key: str,
        user_id: str,
        organization_id: str | None,
        operator_id: str,
        is_g8ep: bool,
        settings_service: "SettingsServiceProtocol",
        client_name: str = "operator",
        permissions: list[str] | None = None,
    ) -> bool:
        """Rotate an operator API key.

        Issues + mirrors the new key first via ``issue_operator_key`` so a
        failure leaves the OLD key authoritative everywhere. Only after the
        new key is fully in place is the old key revoked in ``api_keys``.
        Old-key revocation is best-effort; failure is logged but not fatal.
        """
        ok = await self.issue_operator_key(
            api_key=new_api_key,
            user_id=user_id,
            organization_id=organization_id,
            operator_id=operator_id,
            is_g8ep=is_g8ep,
            settings_service=settings_service,
            client_name=client_name,
            permissions=permissions,
        )
        if not ok:
            return False

        if old_api_key and old_api_key != new_api_key:
            revoked = await self.revoke_key(old_api_key)
            if not revoked:
                logger.warning(
                    "[API-KEY-SERVICE] Old operator API key revocation failed "
                    "after successful rotation; reconciliation will catch up",
                    extra={"operator_id": operator_id},
                )

        return True

    async def revoke_operator_key(
        self,
        api_key: str,
        is_g8ep: bool,
        settings_service: "SettingsServiceProtocol",
    ) -> bool:
        """Revoke an operator API key in the canonical store, then clear the
        platform_settings mirror if this was a g8ep key.

        ``api_keys`` is intentionally revoked first: even if the mirror clear
        fails, the key cannot authenticate anymore, and the startup
        reconciler will clear the stale mirror on next boot.
        """
        ok = await self.revoke_key(api_key)
        if not ok:
            return False

        if is_g8ep:
            try:
                await settings_service.clear_g8ep_operator_api_key(expected=api_key)
            except Exception as e:
                logger.warning(
                    "[API-KEY-SERVICE] Failed to clear g8ep operator API key "
                    "from platform_settings after revoke; reconciler will "
                    "catch up",
                    extra={"error": str(e)},
                )
        return True

    async def record_usage(self, api_key: str) -> None:
        """Update the last used timestamp of a key."""
        try:
            doc_id = self.make_doc_id(api_key)
            await self.cache.update_document(
                collection=self.collection,
                document_id=doc_id,
                data={"last_used_at": now()},
                merge=True
            )
        except Exception as e:
            logger.warning(f"[API-KEY-SERVICE] Failed to record usage: {e}")
