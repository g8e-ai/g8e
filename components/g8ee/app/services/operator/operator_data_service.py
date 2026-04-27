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

if TYPE_CHECKING:
    from app.clients.http_client import HTTPClient
    from app.services.cache.cache_aside import CacheAsideService
from app.constants.collections import (
    DB_COLLECTION_OPERATORS,
)
from app.constants.headers import (
    INTERNAL_AUTH_HEADER,
    WEB_SESSION_ID_HEADER,
)
from app.constants.settings import (
    MAX_COMMAND_RESULTS_HISTORY,
    MAX_HEARTBEAT_HISTORY,
)
from app.constants.events import (
    EventType,
)
from app.constants.status import (
    OperatorHistoryEventType,
    OperatorStatus,
    OperatorType,
)
from app.constants.status import ComponentName
from app.errors import ExternalServiceError, NetworkError, ValidationError
from app.models.http_context import G8eHttpContext
from app.models.investigations import ConversationHistoryMessage, ConversationMessageMetadata
from app.models.operators import (
    CommandResultRecord,
    OperatorDocument,
    OperatorHistoryEntry,
    OperatorHeartbeat,
    OperatorSystemInfo,
)
from app.models import BindOperatorsRequest, BindOperatorsResponse
from app.models.cache import ArrayUnion
from app.services.cache.cache_aside import CacheAsideService
from app.services.protocols import OperatorDataServiceProtocol
from app.utils.keyed_lock import KeyedAsyncLock
from app.utils.timestamp import now

from app.clients.http_client import HTTPClient

logger = logging.getLogger(__name__)

class OperatorDataService(OperatorDataServiceProtocol):
    """Domain service for Operator data management using CacheAsideService."""

    def __init__(self, cache: "CacheAsideService", internal_http_client: "HTTPClient"):
        self.cache = cache
        self.internal_http_client = internal_http_client
        self.collection = DB_COLLECTION_OPERATORS
        self._history_lock = KeyedAsyncLock()

    async def get_operator(self, operator_id: str) -> OperatorDocument | None:
        """Get Operator document using cache-aside pattern."""
        if not operator_id:
            raise ValidationError("operator_id is required")

        data = await self.cache.get_document_with_cache(self.collection, operator_id)
        if not data:
            return None

        return OperatorDocument.model_validate(data)

    async def query_operators(
        self,
        field_filters: list[dict[str, object]] | None = None,
        limit: int = 1000,
        bypass_cache: bool = False,
    ) -> list[OperatorDocument]:
        """Query Operator documents.

        ``bypass_cache=True`` mirrors g8ed's ``queryOperatorsFresh`` and is used
        by reconcilers (e.g. HeartbeatStaleMonitorService) where stale query
        cache results would produce false STALE/OFFLINE transitions.
        """
        rows = await self.cache.query_documents(
            collection=self.collection,
            field_filters=field_filters or [],
            limit=limit,
            bypass_cache=bypass_cache,
        )
        return [OperatorDocument.model_validate(row) for row in rows]

    async def create_operator(self, operator: OperatorDocument) -> bool:
        """Create a new Operator document in the database."""
        if not operator.id:
            raise ValidationError("id is required")
        
        # Convert to dict for storage
        operator_data = operator.model_dump(mode="json")
        
        result = await self.cache.create_document(
            collection=self.collection,
            document_id=operator.id,
            data=operator_data
        )
        
        if not result.success:
            raise ExternalServiceError(f"Failed to create Operator {operator.id}: {result.error}", service_name="operator_service")
        
        return True

    async def update_operator(self, operator: OperatorDocument) -> bool:
        """Update an existing Operator document in the database."""
        if not operator.id:
            raise ValidationError("id is required")
        
        # Convert to dict for storage
        operator_data = operator.model_dump(mode="json")
        
        result = await self.cache.update_document(
            collection=self.collection,
            document_id=operator.id,
            data=operator_data,
            merge=True
        )
        
        if not result.success:
            raise ExternalServiceError(f"Failed to update Operator {operator.id}: {result.error}", service_name="operator_service")
        
        return True

    async def add_history_entry(
        self,
        operator_id: str,
        event_type: OperatorHistoryEventType,
        actor: ComponentName,
        summary: str,
        details: dict[str, object] | None = None,
        additional_updates: dict[str, object] | None = None,
    ) -> OperatorDocument:
        """Atomic status + history update under a keyed lock.
        
        This is the single authoritative path for state transitions that require
        an audit trail entry.
        """
        async with self._history_lock.acquire(operator_id):
            operator = await self.get_operator(operator_id)
            if not operator:
                raise ValidationError(f"Operator {operator_id} not found")

            # Determine prev_hash
            prev_hash = "0" * 64
            if operator.history_trail:
                last_entry = operator.history_trail[-1]
                if last_entry.entry_hash:
                    prev_hash = last_entry.entry_hash

            # Create entry
            entry = OperatorHistoryEntry(
                event_type=event_type,
                actor=actor,
                summary=summary,
                prev_hash=prev_hash,
                details=details or {},
            )

            # Build full update payload
            updates: dict[str, object] = {
                "history_trail": ArrayUnion([entry.model_dump(mode="json")]),
                "updated_at": now(),
            }
            if additional_updates:
                updates.update(additional_updates)

            result = await self.cache.update_document(
                collection=self.collection,
                document_id=operator_id,
                data=updates,
                merge=True,
            )

            if not result.success:
                raise ExternalServiceError(
                    f"Failed to append history to operator {operator_id}: {result.error}",
                    service_name="operator_service"
                )

            # Return refreshed doc
            refreshed = await self.get_operator(operator_id)
            if not refreshed:
                raise ExternalServiceError(f"Operator {operator_id} vanished after update")
            return refreshed

    async def update_operator_heartbeat(
        self,
        operator_id: str,
        heartbeat: OperatorHeartbeat,
        investigation_id: str | None,
        case_id: str | None,
    ) -> bool:
        """Update Operator heartbeat and system info.

        investigation_id and case_id are only written when present; absent values
        are not coerced to sentinel strings so existing values on the document
        are preserved via merge=True.
        """
        now_timestamp = now()
        heartbeat_record = heartbeat.model_dump(mode="json")

        system_info = OperatorSystemInfo(
            hostname=heartbeat.system_identity.hostname,
            os=heartbeat.system_identity.os,
            architecture=heartbeat.system_identity.architecture,
            cpu_count=heartbeat.system_identity.cpu_count,
            memory_mb=heartbeat.system_identity.memory_mb,
            public_ip=heartbeat.network.public_ip,
            internal_ip=heartbeat.network.internal_ip,
            interfaces=heartbeat.network.interfaces or [],
            current_user=heartbeat.system_identity.current_user,
            system_fingerprint=heartbeat.system_fingerprint,
            fingerprint_details=heartbeat.fingerprint_details,
            os_details=heartbeat.os_details,
            user_details=heartbeat.user_details,
            disk_details=heartbeat.disk_details,
            memory_details=heartbeat.memory_details,
            environment=heartbeat.environment,
            is_cloud_operator=heartbeat.is_cloud_operator,
            cloud_provider=heartbeat.cloud_provider,
            local_storage_enabled=heartbeat.local_storage_enabled,
        )

        update_data: dict[str, object] = {
            "last_heartbeat": now_timestamp,
            "updated_at": now_timestamp,
            "system_info": system_info.model_dump(mode="json"),
            "latest_heartbeat_snapshot": heartbeat_record,
            "heartbeat_history": ArrayUnion([heartbeat_record], max_length=MAX_HEARTBEAT_HISTORY),
        }
        if investigation_id is not None:
            update_data["investigation_id"] = investigation_id
        if case_id is not None:
            update_data["case_id"] = case_id

        result = await self.cache.update_document(
            collection=self.collection,
            document_id=operator_id,
            data=update_data,
            merge=True,
        )

        if result.success:
            logger.info(f"Updated Operator {operator_id} heartbeat")
            return True

        raise ExternalServiceError(f"Failed to update Operator {operator_id} heartbeat: {result.error}", service_name="operator_service")

    async def append_command_result(
        self,
        operator_id: str,
        command_result: CommandResultRecord
    ) -> bool:
        """Append command execution result to operator history."""
        now_timestamp = now()
        result_record = command_result.model_dump(mode="json")

        update_data: dict[str, object] = {
            "updated_at": now_timestamp,
            "command_results_history": ArrayUnion([result_record], max_length=MAX_COMMAND_RESULTS_HISTORY),
        }

        result = await self.cache.update_document(
            collection=self.collection,
            document_id=operator_id,
            data=update_data,
            merge=True
        )

        if result.success:
            logger.info(f"Appended command result to Operator {operator_id}")
            return True

        raise ExternalServiceError(f"Failed to append command result to Operator {operator_id}: {result.error}", service_name="operator_service")

    async def add_operator_activity(
        self,
        operator_id: str,
        sender: str,
        content: str,
        metadata: ConversationMessageMetadata,
    ) -> bool:
        """Add activity entry to operator log."""

        from app.utils.ledger_hash import genesis_hash

        activity_entry = ConversationHistoryMessage(
            sender=sender,
            content=content,
            metadata=metadata or ConversationMessageMetadata(),
            prev_hash="0" * 64,
            entry_hash=genesis_hash(operator_id, now().isoformat()),
        )

        result = await self.cache.append_to_array(
            collection=self.collection,
            document_id=operator_id,
            array_field="activity_log",
            items_to_add=[activity_entry.model_dump(mode="json")],
            additional_updates={"updated_at": now()},
        )

        if result.success:
            logger.info(f"Added activity to Operator {operator_id}")
            return True

        raise ExternalServiceError(f"Failed to add activity to Operator {operator_id}: {result.error}", service_name="operator_service")

    async def add_operator_approval(
        self,
        operator_id: str,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
    ) -> bool:
        """Record an approval lifecycle event in the operator activity log."""
        return await self.add_operator_activity(
            operator_id=operator_id,
            sender=EventType.EVENT_SOURCE_SYSTEM,
            content=f"{event_type.value} ({metadata.approval_id})",
            metadata=metadata,
        )

