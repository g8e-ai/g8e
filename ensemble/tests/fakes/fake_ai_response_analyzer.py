# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed fake for AIResponseAnalyzerProtocol."""

from __future__ import annotations

from app.constants import ErrorAnalysisCategory, FileOperation, RiskLevel
from app.models.settings import G8eeUserSettings
from app.models.tool_results import (
    CommandRiskAnalysis,
    CommandRiskContext,
    ErrorAnalysisContext,
    ErrorAnalysisResult,
    FileOperationRiskAnalysis,
    FileOperationRiskContext,
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
        settings: G8eeUserSettings | None = None,
    ) -> CommandRiskAnalysis:
        self.risk_analyses.append(
            {
                "command": command,
                "justification": justification,
                "context": context,
                "settings": settings,
            }
        )
        return CommandRiskAnalysis(risk_level=RiskLevel.LOW)

    async def analyze_error_and_suggest_fix(
        self,
        command: str,
        exit_code: int | None,
        stdout: str,
        stderr: str,
        context: ErrorAnalysisContext,
        settings: G8eeUserSettings | None = None,
    ) -> ErrorAnalysisResult:
        self.error_analyses.append(
            {
                "command": command,
                "exit_code": exit_code,
                "stdout": stdout,
                "stderr": stderr,
                "context": context,
                "settings": settings,
            }
        )
        return ErrorAnalysisResult(
            error_category=ErrorAnalysisCategory.UNKNOWN,
            root_cause="fake: no analysis",
            can_auto_fix=False,
            should_escalate=True,
            reasoning="fake analyzer returns a safe default",
            user_message="fake: no analysis available",
        )

    async def analyze_file_operation_risk(
        self,
        operation: FileOperation,
        file_path: str,
        content: str | None,
        context: FileOperationRiskContext,
        settings: G8eeUserSettings | None = None,
    ) -> FileOperationRiskAnalysis:
        self.file_risk_analyses.append(
            {
                "operation": operation,
                "file_path": file_path,
                "content": content,
                "context": context,
                "settings": settings,
            }
        )
        return FileOperationRiskAnalysis(
            risk_level=RiskLevel.LOW,
            is_system_file=False,
            safe_to_proceed=True,
        )


_: AIResponseAnalyzerProtocol = FakeAIResponseAnalyzer()
