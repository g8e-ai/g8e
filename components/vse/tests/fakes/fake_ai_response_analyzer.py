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

"""Typed fake for AIResponseAnalyzerProtocol."""

from __future__ import annotations
from app.constants import FileOperation, RiskLevel
from app.models.settings import VSEUserSettings
from app.models.tool_results import (
    CommandRiskAnalysis,
    CommandRiskContext,
    ErrorAnalysisContext,
    FileOperationRiskAnalysis,
)
from app.services.protocols import AIResponseAnalyzerProtocol


class FakeAIResponseAnalyzer:
    """Typed fake implementing AIResponseAnalyzerProtocol.

    Returns safe defaults. Records calls for assertion in tests.
    """

    def __init__(self) -> None:
        self.risk_analyses: list[dict] = []
        self.error_analyses: list[dict] = []
        self.file_risk_analyses: list[dict] = []

    async def analyze_command_risk(
        self,
        command: str,
        justification: str,
        context: CommandRiskContext,
        settings: VSEUserSettings | None = None,
    ) -> CommandRiskAnalysis:
        self.risk_analyses.append({
            "command": command,
            "justification": justification,
            "context": context,
            "settings": settings,
        })
        return CommandRiskAnalysis(
            risk_level=RiskLevel.LOW,
            explanation="fake: low risk",
            requires_approval=False,
        )

    async def analyze_error_and_suggest_fix(
        self,
        command: str,
        exit_code: int | None,
        stdout: str,
        stderr: str,
        context: ErrorAnalysisContext,
    ) -> object:
        self.error_analyses.append({
            "command": command,
            "exit_code": exit_code,
            "stdout": stdout,
            "stderr": stderr,
            "context": context,
        })
        return None

    async def analyze_file_operation_risk(
        self,
        operation: FileOperation,
        file_path: str,
        content: str | None,
    ) -> FileOperationRiskAnalysis:
        self.file_risk_analyses.append({
            "operation": operation,
            "file_path": file_path,
            "content": content,
        })
        return FileOperationRiskAnalysis(
            risk_level=RiskLevel.LOW,
            is_system_file=False,
            safe_to_proceed=True,
        )


_: AIResponseAnalyzerProtocol = FakeAIResponseAnalyzer()
