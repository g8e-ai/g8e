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

"""Typed fake for ExecutionServiceProtocol."""

from typing import Any
from app.models.tool_results import CommandInternalResult
from app.models.operators import OperatorDocument, TargetSystem, DirectCommandResult
from app.models.internal_api import DirectCommandRequest
from app.models.http_context import G8eHttpContext
from app.models.pubsub_messages import G8eMessage, G8eoResultEnvelope
from app.services.protocols import ExecutionServiceProtocol
from app.utils.whitelist_validator import CommandWhitelistValidator
from app.utils.blacklist_validator import CommandBlacklistValidator
from app.constants.status import ExecutionStatus


# Create a default operator for the protocol instance
_default_operator = OperatorDocument(
    id="fake-operator",
    user_id="fake-user",
    operator_session_id="fake-session",
    name="Fake Operator",
)


class FakeExecutionService:
    """Typed fake implementing ExecutionServiceProtocol.

    Returns a successful result by default. Configurable via constructor.
    Records all calls for assertion in tests.
    """

    def __init__(
        self,
        *,
        exit_code: int = 0,
        output: str = "fake output",
        resolved_operator: OperatorDocument = _default_operator,
        resolve_error: Exception | None = None,
        g8ed_event_service: Any = None,
        ai_response_analyzer: Any = None,
        whitelist_validator: CommandWhitelistValidator | None = None,
        blacklist_validator: CommandBlacklistValidator | None = None,
        envelope: G8eoResultEnvelope | None = None,
        pubsub_service: Any = None,
    ) -> None:
        self._exit_code = exit_code
        self._output = output
        self._resolved_operator = resolved_operator
        self._resolve_error = resolve_error
        self.g8ed_event_service = g8ed_event_service
        self.ai_response_analyzer = ai_response_analyzer
        self.whitelist_validator = whitelist_validator
        self.blacklist_validator = blacklist_validator
        self.pubsub_service = pubsub_service
        self._envelope = envelope
        self.execute_calls: list[dict] = []
        self.resolve_calls: list[dict] = []
        self.send_command_calls: list[dict] = []

    async def execute(
        self,
        g8e_message: G8eMessage,
        g8e_context: G8eHttpContext,
        timeout_seconds: int = 60,
    ) -> tuple[CommandInternalResult, G8eoResultEnvelope | None]:
        self.execute_calls.append({
            "g8e_message": g8e_message,
            "g8e_context": g8e_context,
            "timeout_seconds": timeout_seconds,
        })
        if self.pubsub_service:
            await self.pubsub_service.publish_command(
                operator_id=g8e_message.operator_id,
                operator_session_id=g8e_message.operator_session_id,
                command_data=g8e_message,
            )
        return CommandInternalResult(
            exit_code=self._exit_code, 
            output=self._output, 
            status=ExecutionStatus.COMPLETED
        ), self._envelope

    async def execute_command_internal(self, **kwargs) -> CommandInternalResult:
        self.execute_calls.append(kwargs)
        return CommandInternalResult(exit_code=self._exit_code, output=self._output, status=ExecutionStatus.COMPLETED)

    def resolve_target_operator(
        self,
        operator_documents: list[OperatorDocument],
        target_operator: str | None,
        tool_name: str | None = None,
    ) -> OperatorDocument:
        self.resolve_calls.append({
            "operator_documents": operator_documents,
            "target_operator": target_operator,
            "tool_name": tool_name,
        })
        if self._resolve_error:
            raise self._resolve_error
        if target_operator and operator_documents:
            for op in operator_documents:
                if op.id == target_operator:
                    return op
        return operator_documents[0] if operator_documents else self._resolved_operator

    def resolve_multiple_operators(
        self,
        operator_documents: list[OperatorDocument],
        target_operators: list[str],
    ) -> list[OperatorDocument]:
        if not operator_documents:
            return [self._resolved_operator]
        if "all" in target_operators:
            return operator_documents
        resolved = []
        for target_id in target_operators:
            for op in operator_documents:
                if op.id == target_id:
                    resolved.append(op)
                    break
        return resolved if resolved else operator_documents

    def build_target_systems_list(self, operator_documents: list[OperatorDocument]) -> list[TargetSystem]:
        return [TargetSystem(
            operator_id=op.id,
            hostname=op.current_hostname or "fake-host",
            operator_type=op.operator_type,
        ) for op in operator_documents]

    async def send_command_to_operator(
        self,
        command_payload: DirectCommandRequest,
        g8e_context: G8eHttpContext,
    ) -> DirectCommandResult:
        self.send_command_calls.append({
            "command_payload": command_payload,
            "g8e_context": g8e_context,
        })
        return DirectCommandResult(
            execution_id=command_payload.execution_id,
        )


_: ExecutionServiceProtocol = FakeExecutionService(resolved_operator=_default_operator)
