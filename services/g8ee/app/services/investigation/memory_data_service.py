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
from app.models.http_context import RequestContext
from app.services.cache.cache_aside import CacheAsideService
from app.services.protocols import MemoryDataServiceProtocol
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from app.clients.governance_client import GovernanceClient

logger = logging.getLogger(__name__)


class MemoryDataService(MemoryDataServiceProtocol):
    """Cache-aside persistence layer for InvestigationMemory documents.

    All reads and writes route exclusively through CacheAsideService.
    """

    def __init__(self, cache_aside_service: CacheAsideService, governance_client: GovernanceClient) -> None:
        self._cache_aside = cache_aside_service
        self._governance_client = governance_client
        self.memories_collection = DB_COLLECTION_MEMORIES

    async def create_memory(self, investigation: InvestigationModel, context: RequestContext) -> InvestigationMemory:
        memory = InvestigationMemory(
            case_id=investigation.case_id,
            investigation_id=investigation.id,
            user_id=investigation.user_id,
            status=investigation.status,
            case_title=investigation.case_title,
        )

        from app.models.pubsub_messages import G8eMessage
        from app.models.command_request_payloads import DocumentUpdateRequestPayload
        from app.constants import EventType, ComponentName, AITaskId

        payload = DocumentUpdateRequestPayload(
            collection=self.memories_collection,
            document_id=investigation.id,
            updates=memory.model_dump(mode="json"),
            merge=False,
        )

        message = G8eMessage(
            id=investigation.id,
            source_component=ComponentName.G8EE,
            event_type=EventType.APP_MEMORY_CREATED,
            case_id=investigation.case_id,
            investigation_id=investigation.id,
            task_id=AITaskId.MEMORY,
            web_session_id=context.web_session_id,
            user_id=context.user_id,
            payload=payload,
        )
        await self._governance_client.submit_envelope(message)
        logger.info(
            "Created new memory for investigation %s via governance envelope",
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

    async def save_memory(self, memory: InvestigationMemory, is_new: bool, context: RequestContext) -> None:
        data = memory.model_dump(mode="json")

        from app.constants import EventType, ComponentName

        if is_new:
            from app.models.pubsub_messages import G8eMessage
            from app.models.command_request_payloads import DocumentUpdateRequestPayload
            from app.constants import AITaskId

            payload = DocumentUpdateRequestPayload(
                collection=self.memories_collection,
                document_id=memory.investigation_id,
                updates=data,
                merge=False,
            )

            message = G8eMessage(
                id=memory.investigation_id,
                source_component=ComponentName.G8EE,
                event_type=EventType.APP_MEMORY_CREATED,
                case_id=memory.case_id,
                investigation_id=memory.investigation_id,
                task_id=AITaskId.MEMORY,
                web_session_id=context.web_session_id,
                user_id=context.user_id,
                payload=payload,
            )
            await self._governance_client.submit_envelope(message)
        else:
            await self._governance_client.update_governed_doc(
                collection=self.memories_collection,
                document_id=memory.investigation_id,
                updates=data,
                event_type=EventType.APP_MEMORY_UPDATED,
                case_id=memory.case_id,
                investigation_id=memory.investigation_id,
                web_session_id=context.web_session_id,
                user_id=context.user_id,
                operator_id=context.operator_id,
                operator_session_id=context.operator_session_id,
                merge=True,
            )

    async def get_memory(self, investigation_id: str) -> InvestigationMemory | None:
        data = await self._cache_aside.get_document_with_cache(
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
