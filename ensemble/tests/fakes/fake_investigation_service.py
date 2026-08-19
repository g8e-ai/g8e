# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed fake for InvestigationServiceProtocol."""

from unittest.mock import MagicMock

from app.constants import EventType, HistoryActor
from app.models.http_context import RequestContext
from app.models.investigations import (
    ConversationMessageMetadata,
    EnrichedInvestigationContext,
    InvestigationModel,
)
from app.models.operators import FileOperation
from app.models.tool_results import CommandInternalResult, FileEditResult
from app.services.protocols import InvestigationDataServiceProtocol, InvestigationServiceProtocol


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

    async def get_investigation(self, investigation_id: str):
        return None

    async def get_chat_messages(self, investigation_id: str):
        return []

    async def get_enriched_investigation_context(self, investigation, user_id, g8e_context):
        return investigation

    async def update_investigation(self, investigation_id, request, actor=None):
        return None

    async def add_history_entry(
        self,
        investigation_id: str,
        event_type: EventType,
        actor: HistoryActor,
        summary: str,
        details: ConversationMessageMetadata,
        context: RequestContext,
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
        context: RequestContext,
        actor: HistoryActor = HistoryActor.G8EO,
    ) -> InvestigationModel:
        self.command_results.append(
            {
                "investigation_id": investigation_id,
                "execution_id": execution_id,
                "command": command,
                "result": result,
                "operator_id": operator_id,
                "operator_session_id": operator_session_id,
                "actor": actor,
            }
        )
        return MagicMock(spec=InvestigationModel)

    async def add_chat_message(
        self,
        investigation_id: str | None,
        sender: EventType,
        content: str,
        metadata: ConversationMessageMetadata,
    ) -> bool:
        self.messages.append(
            {
                "investigation_id": investigation_id,
                "sender": sender,
                "content": content,
                "metadata": metadata,
            }
        )
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
        context: RequestContext,
        operator_session_id: str,
    ) -> InvestigationModel:
        return MagicMock(spec=InvestigationModel)

    async def add_approval_record(
        self,
        investigation_id: str,
        event_type: EventType,
        metadata: ConversationMessageMetadata,
        context: RequestContext,
        actor: HistoryActor = HistoryActor.SYSTEM,
    ) -> InvestigationModel:
        return MagicMock(spec=InvestigationModel)


_: InvestigationServiceProtocol = FakeInvestigationService()
