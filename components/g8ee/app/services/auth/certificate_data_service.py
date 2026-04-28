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

import logging
from typing import TYPE_CHECKING

from app.constants.collections import DB_COLLECTION_REVOKED_CERTS
from app.errors import ExternalServiceError
from app.utils.timestamp import now

if TYPE_CHECKING:
    from app.services.cache.cache_aside import CacheAsideService

logger = logging.getLogger(__name__)

class CertificateDataService:
    """Service for persisting certificate-related data, such as revocations."""

    def __init__(self, cache: "CacheAsideService"):
        self.cache = cache
        self.collection = DB_COLLECTION_REVOKED_CERTS

    async def get_all_revocations(self) -> list[dict]:
        """Get all revoked certificate records."""
        try:
            # query_documents handles cache-aside internally
            rows = await self.cache.query_documents(
                collection=self.collection,
                field_filters=[],
                limit=10000,
            )
            return rows
        except Exception as e:
            logger.error(f"[CERT-DATA] Failed to query revocations: {e}")
            return []

    async def revoke_certificate(self, serial: str, reason: str, operator_id: str | None = None) -> bool:
        """Persist a certificate revocation."""
        doc_id = serial.upper()
        data = {
            "serial": doc_id,
            "reason": reason,
            "operator_id": operator_id,
            "revoked_at": now().isoformat(),
        }

        result = await self.cache.create_document(
            collection=self.collection,
            document_id=doc_id,
            data=data
        )

        if not result.success:
            raise ExternalServiceError(f"Failed to persist revocation for {serial}: {result.error}", service_name="certificate_service")

        return True
