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
from typing import Any

from app.constants import (
    DB_COLLECTION_INVESTIGATIONS,
    ComponentName,
    ComponentStatus,
    EscalationRisk,
    EventType,
    FileOperation,
    ExecutionStatus,
)
from app.constants.message_sender import MessageSender
from app.errors import ResourceNotFoundError, DatabaseError, ErrorCode
from app.models.cache import FieldFilter
from app.models.investigations import (
    ConversationHistoryMessage,
    ConversationMessageMetadata,
    InvestigationCreateRequest,
    InvestigationCurrentState,
    InvestigationHistoryEntry,
    InvestigationModel,
    InvestigationQueryRequest,
)

from app.models.operators import CommandInternalResult
from app.models.tool_results import FileEditResult
from app.services.cache.cache_aside import CacheAsideService
from app.services.protocols import InvestigationDataServiceProtocol
from app.utils.timestamp import now


logger = logging.getLogger(__name__)


class InvestigationDataService(InvestigationDataServiceProtocol):

    def __init__(self, cache: CacheAsideService):
        self.cache = cache
        self.collection = DB_COLLECTION_INVESTIGATIONS

    async def create_investigation(self, request: InvestigationCreateRequest) -> InvestigationModel:
        """Low-level persistence for a new investigation document."""
        investigation = InvestigationModel(
            case_id=request.case_id,
            case_title=request.case_title,
            case_description=request.case_description,
            web_session_id=request.web_session_id,
            priority=request.priority,
            user_email=request.user_email,
            user_id=request.user_id,
            created_with_case=request.created_with_case,
            case_source=request.case_source,
            sentinel_mode=request.sentinel_mode,
        )

        investigation.current_state = InvestigationCurrentState(
            active_attempt=1,
            escalation_risk=EscalationRisk.LOW,
            collaboration_status={ComponentName.G8EE: ComponentStatus.ACTIVE},
        )

        # Initial creation record is data-layer appropriate
        investigation.add_history_entry(
            event_type=EventType.INVESTIGATION_CREATED,
            actor=ComponentName.G8EE,
            summary=f"Investigation created for case {request.case_id}",
        )

        await self.cache.create_document(
            collection=self.collection,
            document_id=investigation.id,
            data=investigation.model_dump(),
        )
        logger.info(f"Created investigation {investigation.id} for case {request.case_id}")
        return investigation

    async def get_investigation(self, investigation_id: str) -> InvestigationModel | None:
        """Fetch a single investigation document by ID."""
        doc_data = await self.cache.get_document(
            collection=self.collection,
            document_id=investigation_id,
        )
        if doc_data is None:
            return None
        doc_data["id"] = investigation_id
        return InvestigationModel.model_validate(doc_data)

    async def update_investigation_raw(
        self,
        investigation_id: str,
        updates: dict[str, object],
        merge: bool = True,
    ):
        """Authoritative low-level update for the investigations collection."""
        result = await self.cache.update_document(
            collection=self.collection,
            document_id=investigation_id,
            data=updates,
            merge=merge,
        )
        if not result.success:
            raise DatabaseError(
                f"Failed to update investigation: {result.error}",
                code=ErrorCode.DB_WRITE_ERROR,
                component="g8ee"
            )

    async def query_investigations(self, request: InvestigationQueryRequest) -> list[InvestigationModel]:
        """Execute a filtered query against the investigations collection."""
        filter_map = {
            "user_id": request.user_id,
            "case_id": request.case_id,
            "task_id": request.task_id,
            "web_session_id": request.web_session_id,
            "status": request.status,
            "priority": request.priority,
        }
        filters = [
            FieldFilter(field=field, op="==", value=value).model_dump(mode="json")
            for field, value in filter_map.items()
            if value is not None
        ]

        raw_order_by = {request.order_by: request.order_direction}

        results = await self.cache.query_documents(
            collection=self.collection,
            field_filters=filters,
            order_by=raw_order_by,
            limit=request.limit,
        )

        # Data is already validated at write time - no need to re-validate on read
        return [InvestigationModel.model_validate(data) for data in results]

    async def get_case_investigations(
        self,
        case_id: str,
        user_id: str | None,
    ) -> list[InvestigationModel]:
        """Convenience query for all investigations associated with a case."""
        if not user_id:
            # Allow case-based queries without user_id for backward compatibility
            # but this should be logged as a security concern at the service layer
            request = InvestigationQueryRequest(case_id=case_id, user_id=None, limit=100)
        else:
            request = InvestigationQueryRequest(case_id=case_id, user_id=user_id, limit=100)
        return await self.query_investigations(request)

    async def delete_investigation(self, investigation_id: str) -> None:
        """Hard-delete an investigation document."""
        await self.cache.delete_document(
            collection=self.collection,
            document_id=investigation_id,
        )
        logger.info(f"Deleted investigation {investigation_id}")

    async def add_chat_message(
        self,
        investigation_id: str | None,
        sender: str,
        content: str,
        metadata: ConversationMessageMetadata,
    ) -> bool:
        """Persist a chat message to the investigation's conversation history."""
        if not investigation_id:
            return True

        message = ConversationHistoryMessage(
            sender=sender,
            content=content,
            metadata=metadata or ConversationMessageMetadata(),
        )

        await self.cache.append_to_array(
            collection=self.collection,
            document_id=investigation_id,
            array_field="conversation_history",
            items_to_add=[message.model_dump(mode="json")],
            additional_updates={"created_at": now()},
        )

        return True

    async def add_history_entry(
        self,
        investigation_id: str,
        event_type: EventType,
        actor: ComponentName,
        summary: str,
        details: ConversationMessageMetadata,
    ) -> InvestigationModel:
        """Record an event in the investigation history trail."""
        investigation = await self.get_investigation(investigation_id)
        if not investigation:
            raise ResourceNotFoundError(
                f"Investigation {investigation_id} not found",
                resource_type="investigation",
                resource_id=investigation_id,
            )

        investigation.add_history_entry(
            event_type=event_type,
            actor=actor,
            summary=summary,
            details=details or ConversationMessageMetadata(),
        )

        await self.update_investigation_raw(
            investigation_id=investigation_id,
            updates={"history_trail": [entry.model_dump(mode="json") for entry in investigation.history_trail]},
        )

        return investigation

    async def add_approval_record(
        self,
        investigation_id: str,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
        actor: ComponentName = ComponentName.G8EE,
    ) -> InvestigationModel:
        """Record an approval lifecycle event in both conversation_history and history_trail."""
        summary = f"{event_type.value} ({metadata.approval_id})"
        metadata.event_type = event_type

        await self.add_chat_message(
            investigation_id=investigation_id,
            sender=MessageSender.SYSTEM,
            content=summary,
            metadata=metadata,
        )

        return await self.add_history_entry(
            investigation_id=investigation_id,
            event_type=event_type,
            actor=actor,
            summary=summary,
            details=metadata,
        )

    async def add_command_execution_result(
        self,
        investigation_id: str,
        execution_id: str,
        command: str,
        result: CommandInternalResult,
        operator_id: str,
        operator_session_id: str,
    ) -> InvestigationModel:
        """Helper to record a command execution result."""
        details = ConversationMessageMetadata(
            execution_id=execution_id,
            command=command,
            status=result.status,
            exit_code=result.exit_code,
            error=result.error,
            execution_time_seconds=result.execution_time_seconds,
            hostname=operator_id, # Using operator_id as hostname for now as per previous pattern
        )
        summary = f"Executed: {command[:50]}... ({result.status})"
        return await self.add_history_entry(
            investigation_id=investigation_id,
            event_type=EventType.OPERATOR_COMMAND_EXECUTION,
            actor=ComponentName.G8EO,
            summary=summary,
            details=details,
        )

    async def add_file_operation_result(
        self,
        investigation_id: str,
        execution_id: str,
        operator_id: str,
        event_type: EventType,
        file_path: str,
        result: FileEditResult,
        operation: FileOperation,
    ) -> InvestigationModel:
        """Helper to record a file operation result."""
        details = ConversationMessageMetadata(
            execution_id=execution_id,
            file_path=file_path,
            operation=operation,
            status=ExecutionStatus.COMPLETED if result.success else ExecutionStatus.FAILED,
            error=result.error,
        )
        summary = f"{event_type.value}: {file_path} ({'success' if result.success else 'failed'})"
        return await self.add_history_entry(
            investigation_id=investigation_id,
            event_type=event_type,
            actor=ComponentName.G8EO,
            summary=summary,
            details=details,
        )

    async def get_command_execution_history(self, investigation_id: str) -> list[InvestigationHistoryEntry]:
        """Retrieve all command execution entries from investigation history."""
        investigation = await self.get_investigation(investigation_id)
        if not investigation:
            return []

        return [
            entry for entry in investigation.history_trail
            if entry.event_type == EventType.OPERATOR_COMMAND_EXECUTION
        ]

    async def get_operator_actions_for_ai_context(self, investigation_id: str) -> str:
        """Format operator actions for inclusion in AI system prompt."""
        investigation = await self.get_investigation(investigation_id)
        if not investigation or not investigation.history_trail:
            return "No Operator actions recorded yet."

        history = [
            e for e in investigation.history_trail
            if e.event_type in (
                EventType.OPERATOR_COMMAND_EXECUTION,
                EventType.OPERATOR_FILE_EDIT_COMPLETED,
                EventType.OPERATOR_FILESYSTEM_LIST_COMPLETED,
                EventType.OPERATOR_FILESYSTEM_READ_COMPLETED,
            )
        ]

        if not history:
            return "No Operator actions recorded yet."

        lines = [f"Recorded Operator Actions ({len(history)}):"]
        for entry in history:
            lines.append(f"- [{entry.timestamp}] {entry.summary}")
            if entry.details:
                if entry.event_type == EventType.OPERATOR_COMMAND_EXECUTION:
                    status = str(entry.details.status) if entry.details.status else "unknown"
                else:
                    status = "success" if entry.details.approved else "failed"
                lines.append(f"  Result: {status}")
        
        return "\n".join(lines)

    async def get_chat_messages(self, investigation_id: str) -> list[ConversationHistoryMessage]:
        """Retrieve full conversation history for an investigation."""
        data = await self.cache.get_document(
            collection=self.collection,
            document_id=investigation_id,
        )

        if not data:
            return []

        raw_history = data.get("conversation_history", [])
        if not isinstance(raw_history, list):
            return []
        raw_history_typed: list[Any] = raw_history
        messages = [ConversationHistoryMessage.model_validate(m) for m in raw_history_typed]
        messages.sort(key=lambda m: m.timestamp)

        return messages
