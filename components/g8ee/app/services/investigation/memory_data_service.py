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

from app.constants import DB_COLLECTION_MEMORIES
from app.errors import DatabaseError
from app.models.cache import FieldFilter
from app.models.investigations import InvestigationModel
from app.models.memory import InvestigationMemory
from app.services.cache.cache_aside import CacheAsideService
from app.services.protocols import MemoryDataServiceProtocol

logger = logging.getLogger(__name__)


class MemoryDataService(MemoryDataServiceProtocol):
    """Cache-aside persistence layer for InvestigationMemory documents.

    All reads and writes route exclusively through CacheAsideService.
    """

    def __init__(self, cache_aside_service: CacheAsideService) -> None:
        self._cache_aside = cache_aside_service
        self.memories_collection = DB_COLLECTION_MEMORIES

    async def create_memory(self, investigation: InvestigationModel) -> InvestigationMemory:
        memory = InvestigationMemory(
            case_id=investigation.case_id,
            investigation_id=investigation.id,
            user_id=investigation.user_id,
            status=investigation.status,
            case_title=investigation.case_title,
        )
        await self._cache_aside.create_document(
            collection=self.memories_collection,
            document_id=investigation.id,
            data=memory.model_dump(mode="json"),
        )
        logger.info(
            "Created new memory for investigation %s",
            investigation.id,
            extra={
                "investigation_id": investigation.id,
                "case_id": memory.case_id,
                "user_id": memory.user_id,
                "case_title": memory.case_title,
                "operation": "memory_created",
            },
        )
        return memory

    async def save_memory(self, memory: InvestigationMemory, is_new: bool) -> None:
        data = memory.model_dump(mode="json")
        if is_new:
            await self._cache_aside.create_document(
                collection=self.memories_collection,
                document_id=memory.investigation_id,
                data=data,
            )
        else:
            await self._cache_aside.update_document(
                collection=self.memories_collection,
                document_id=memory.investigation_id,
                data=data,
                merge=True,
            )

    async def get_memory(self, investigation_id: str) -> InvestigationMemory | None:
        data = await self._cache_aside.get_document(
            collection=self.memories_collection,
            document_id=investigation_id,
        )
        if data is not None:
            return InvestigationMemory.model_validate(data)
        return None

    async def get_user_memories(self, user_id: str) -> list[InvestigationMemory]:
        try:
            docs = await self._cache_aside.query_documents(
                collection=self.memories_collection,
                field_filters=[FieldFilter(field="user_id", op="==", value=user_id).model_dump(mode="json")],
                order_by={"created_at": "desc"},
            )
        except Exception as exc:
            raise DatabaseError("Failed to query memories for user", cause=exc, component="g8ee") from exc
        memories = [InvestigationMemory.model_validate(d) for d in docs]
        logger.info(
            "Retrieved %d memories for user %s",
            len(memories),
            user_id,
            extra={"user_id": user_id, "memory_count": len(memories), "operation": "get_user_memories"},
        )
        return memories

    async def get_case_memories(self, case_id: str, user_id: str) -> list[InvestigationMemory]:
        try:
            docs = await self._cache_aside.query_documents(
                collection=self.memories_collection,
                field_filters=[
                    FieldFilter(field="user_id", op="==", value=user_id).model_dump(mode="json"),
                    FieldFilter(field="case_id", op="==", value=case_id).model_dump(mode="json"),
                ],
                order_by={"created_at": "desc"},
            )
        except Exception as exc:
            raise DatabaseError("Failed to query memories for case", cause=exc, component="g8ee") from exc
        memories = [InvestigationMemory.model_validate(d) for d in docs]
        logger.info(
            "Retrieved %d memories for case %s",
            len(memories),
            case_id,
            extra={"case_id": case_id, "memory_count": len(memories), "operation": "get_case_memories_complete"},
        )
        return memories
