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

"""Typed fake for InvestigationServiceProtocol."""

from unittest.mock import MagicMock
from app.models.investigations import (
    EnrichedInvestigationContext,
    InvestigationModel,
    ConversationHistoryMessage,
    ConversationMessageMetadata,
)
from app.models.tool_results import CommandInternalResult, FileEditResult
from app.models.operators import FileOperation
from app.constants import ComponentName, EventType
from app.services.protocols import InvestigationServiceProtocol, InvestigationDataServiceProtocol


class FakeInvestigationService:
    """Typed fake implementing InvestigationServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self) -> None:
        self.operator_actions: list[dict] = []
        self.command_results: list[dict] = []
        self.messages: list[dict] = []
        self._investigation_data_service = MagicMock(spec=InvestigationDataServiceProtocol)

    @property
    def investigation_data_service(self) -> InvestigationDataServiceProtocol:
        return self._investigation_data_service

    async def get_investigation_context(self, *args, **kwargs) -> EnrichedInvestigationContext:
        return MagicMock(spec=EnrichedInvestigationContext)

    async def add_history_entry(
        self,
        investigation_id: str,
        event_type: EventType,
        actor: ComponentName,
        summary: str,
        details: ConversationMessageMetadata,
    ) -> InvestigationModel:
        entry = {
            "investigation_id": investigation_id,
            "event_type": event_type,
            "actor": actor,
            "summary": summary,
            "details": details,
        }
        self.operator_actions.append(entry)
        return MagicMock(spec=InvestigationModel)

    async def add_command_execution_result(
        self,
        investigation_id: str,
        execution_id: str,
        command: str,
        result: CommandInternalResult,
        operator_id: str,
        operator_session_id: str,
        actor: ComponentName = ComponentName.G8EO,
    ) -> InvestigationModel:
        self.command_results.append({
            "investigation_id": investigation_id,
            "execution_id": execution_id,
            "command": command,
            "result": result,
            "operator_id": operator_id,
            "operator_session_id": operator_session_id,
            "actor": actor,
        })
        return MagicMock(spec=InvestigationModel)

    async def add_chat_message(
        self,
        investigation_id: str | None,
        sender: EventType,
        content: str,
        metadata: ConversationMessageMetadata,
    ) -> bool:
        self.messages.append({
            "investigation_id": investigation_id,
            "sender": sender,
            "content": content,
            "metadata": metadata,
        })
        return True

    async def add_file_operation_result(
        self,
        investigation_id: str,
        execution_id: str,
        operator_id: str,
        event_type: EventType,
        file_path: str,
        result: FileEditResult,
        operation: FileOperation,
        operator_session_id: str
    ) -> InvestigationModel:
        return MagicMock(spec=InvestigationModel)

    async def add_approval_record(
        self,
        investigation_id: str,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
        actor: ComponentName = ComponentName.G8EE,
    ) -> InvestigationModel:
        return MagicMock(spec=InvestigationModel)


_: InvestigationServiceProtocol = FakeInvestigationService()
