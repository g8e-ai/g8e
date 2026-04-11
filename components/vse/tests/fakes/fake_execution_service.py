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
from app.models.operators import OperatorDocument
from app.services.protocols import ExecutionServiceProtocol


# Create a default operator for the protocol instance
_default_operator = OperatorDocument(
    operator_id="fake-operator",
    name="Fake Operator",
    session_id="fake-session",
    component="vse"
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
        vsod_event_service: Any = None,
    ) -> None:
        self._exit_code = exit_code
        self._output = output
        self._resolved_operator = resolved_operator
        self._resolve_error = resolve_error
        self.vsod_event_service = vsod_event_service
        self.execute_calls: list[dict] = []
        self.resolve_calls: list[dict] = []

    async def execute_command_internal(self, **kwargs) -> CommandInternalResult:
        self.execute_calls.append(kwargs)
        return CommandInternalResult(exit_code=self._exit_code, output=self._output)

    def resolve_target_operator(
        self,
        *,
        operator_documents: list[OperatorDocument],
        target_operator: str | None,
    ) -> OperatorDocument:
        self.resolve_calls.append({
            "operator_documents": operator_documents,
            "target_operator": target_operator,
        })
        if self._resolve_error:
            raise self._resolve_error
        return self._resolved_operator


_: ExecutionServiceProtocol = FakeExecutionService(resolved_operator=_default_operator)
