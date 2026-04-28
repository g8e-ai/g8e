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

"""Typed fake for DocumentServiceProtocol."""

from app.constants import EventType
from app.models.operators import CommandResultRecord
from app.services.protocols import DocumentServiceProtocol
from app.models.cache import CacheOperationResult


class FakeDBService:
    """Typed fake implementing DocumentServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self) -> None:
        self.operator_activities: list[dict] = []
        self.chat_messages: list[dict] = []
        self.command_results: list[dict] = []
        self.heartbeat_updates: list[dict] = []

    @property
    def kv(self):
        return None

    @property
    def db(self):
        return self

    async def create_document(self, collection: str, document_id: str, data, ttl: int | None = None) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def update_document(
        self, collection: str, document_id: str, data, merge: bool = True, ttl: int | None = None
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def get_document(self, collection: str, document_id: str):
        from app.models.internal_api import DocumentResult
        return DocumentResult(success=True)

    async def delete_document(self, collection: str, document_id: str) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def query_collection(
        self, collection: str, field_filters, order_by, limit: int, select_fields=None, ttl: int | None = 300
    ):
        from app.models.internal_api import QueryResult
        return QueryResult(success=True, documents=[])

    async def update_with_array_union(
        self, collection: str, document_id: str, array_field: str, items_to_add: list[object], additional_updates: dict[str, object]
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def batch_write(self, operations) -> CacheOperationResult:
        return CacheOperationResult(success=True)

    async def close(self) -> None:
        pass

    async def add_operator_activity(
        self,
        operator_id: str,
        sender: EventType,
        content: str,
        metadata: object,
        investigation_id: str,
        case_id: str,
    ) -> None:
        self.operator_activities.append({
            "operator_id": operator_id,
            "sender": sender,
            "content": content,
            "metadata": metadata,
            "investigation_id": investigation_id,
            "case_id": case_id,
        })

    async def add_chat_message(
        self,
        investigation_id: str | None,
        sender: EventType,
        content: str,
        metadata: object,
    ) -> None:
        self.chat_messages.append({
            "investigation_id": investigation_id,
            "sender": sender,
            "content": content,
            "metadata": metadata,
        })

    async def append_command_result(
        self,
        operator_id: str,
        command_result: CommandResultRecord,
    ) -> None:
        self.command_results.append({
            "operator_id": operator_id,
            "command_result": command_result,
        })

    async def update_operator_heartbeat(self, operator_id: str, **kwargs) -> bool:
        self.heartbeat_updates.append({"operator_id": operator_id, **kwargs})
        return True


_: DocumentServiceProtocol = FakeDBService()
