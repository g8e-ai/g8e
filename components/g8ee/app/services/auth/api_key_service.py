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

import hashlib
import logging
from typing import Optional

from app.constants import DB_COLLECTION_API_KEYS
from app.models.api_keys import ApiKeyDocument
from app.services.cache.cache_aside import CacheAsideService
from app.utils.timestamp import now

logger = logging.getLogger(__name__)

# Constants for hashing (aligned with g8ed)
API_KEY_HASH_ALGORITHM = "sha256"
API_KEY_HASH_LENGTH = 32

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

    async def validate_key(self, api_key: str) -> tuple[bool, Optional[ApiKeyDocument], Optional[str]]:
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
