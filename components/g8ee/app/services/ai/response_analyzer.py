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

import app.llm.llm_types as types
from app.models.settings import G8eeUserSettings
from app.constants import ErrorAnalysisCategory, FileOperation, RiskLevel
from app.llm import get_llm_provider, Role
from app.llm.structured import parse_structured_response
from app.models.base import G8eBaseModel
from app.models.tool_results import (
    CommandRiskAnalysis,
    CommandRiskContext,
    ErrorAnalysisContext,
    ErrorAnalysisResult,
    FileOperationRiskAnalysis,
    FileOperationRiskContext,
)
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder
from app.utils.agent_persona_loader import get_agent_persona

logger = logging.getLogger(__name__)


class AIResponseAnalyzer:
    """Analyzes AI responses and performs defensive operations.

    Responsibilities:
    - Defensive operations analysis (command risk, error analysis, file operations)
    """

    def __init__(self):
        logger.info("AIResponseAnalyzer initialized")

    async def _run_assistant_analysis(
        self,
        prompt: str,
        response_model: type[G8eBaseModel],
        assistant_model: str | None,
        settings: G8eeUserSettings,
        fallback_no_model,
        fallback_no_response,
        fallback_exception,
        log_context: str,
        post_process=None,
    ):
        if not assistant_model:
            logger.warning("%s: no assistant_model configured", log_context)
            return fallback_no_model()

        try:
            client = get_llm_provider(settings.llm, is_assistant=True)
            config = AIGenerationConfigBuilder.build_assistant_settings(
                model=assistant_model,
                temperature=settings.llm.llm_temperature,
                max_tokens=settings.llm.llm_max_tokens,
                system_instructions=prompt,
                response_format=types.ResponseFormat.from_pydantic_schema(response_model.model_json_schema()),
            )
            response = await client.generate_content_assistant(
                model=assistant_model,
                contents=[types.Content(role=Role.USER, parts=[types.Part(text=prompt)])],
                assistant_llm_settings=config,
            )

            response_text = response.text
            if response_text is None:
                logger.error("%s: LLM returned no text content", log_context)
                return fallback_no_response()
            analysis = parse_structured_response(response_text, response_model)

            if post_process:
                post_process(analysis)

            logger.info("%s completed", log_context)
            return analysis

        except Exception as e:
            logger.error("%s failed: %s", log_context, e, exc_info=True)
            return fallback_exception(e)

    async def analyze_command_risk(
        self,
        command: str,
        justification: str,
        context: CommandRiskContext,
        settings: G8eeUserSettings,
    ) -> CommandRiskAnalysis:
        context = context or CommandRiskContext()
        working_dir = context.working_directory
        resolved_settings = settings

        command_risk_persona = get_agent_persona("response_analyzer_command_risk")
        prompt = command_risk_persona.persona.format(
            command=command,
            justification=justification,
            working_dir=working_dir
        )

        assistant_model = resolved_settings.llm.resolved_assistant_model

        def log_result(analysis):
            logger.info("Command risk analysis completed: command=%s risk_level=%s", command[:60], analysis.risk_level)

        return await self._run_assistant_analysis(
            prompt=prompt,
            response_model=CommandRiskAnalysis,
            assistant_model=assistant_model,
            settings=resolved_settings,
            fallback_no_model=lambda: CommandRiskAnalysis(risk_level=RiskLevel.HIGH),
            fallback_no_response=lambda: CommandRiskAnalysis(risk_level=RiskLevel.HIGH),
            fallback_exception=lambda e: CommandRiskAnalysis(risk_level=RiskLevel.HIGH),
            log_context="Command risk analysis",
            post_process=log_result,
        )

    async def analyze_error_and_suggest_fix(
        self,
        command: str,
        exit_code: int | None,
        stdout: str,
        stderr: str,
        context: ErrorAnalysisContext,
        settings: G8eeUserSettings,
    ) -> ErrorAnalysisResult:
        context = context or ErrorAnalysisContext()
        retry_count = context.retry_count
        working_dir = context.working_directory
        resolved_settings = settings

        if retry_count >= 2:
            logger.info(
                "Error analysis short-circuited at retry limit: command=%s retry_count=%s",
                command[:60], retry_count,
            )
            return ErrorAnalysisResult(
                error_category=ErrorAnalysisCategory.UNKNOWN,
                root_cause=f"Command failed after {retry_count} retries",
                can_auto_fix=False,
                should_escalate=True,
                reasoning="Retry limit reached - escalating to prevent infinite loop",
                user_message=f"Command failed after {retry_count} retries. Manual intervention required.",
            )

        error_persona = get_agent_persona("response_analyzer_error")
        prompt = error_persona.persona.format(
            command=command,
            exit_code=exit_code,
            stdout=stdout[:1000],
            stderr=stderr[:1000],
            retry_count=retry_count,
            working_dir=working_dir
        )

        assistant_model = resolved_settings.llm.resolved_assistant_model

        def post_process(analysis):
            if retry_count >= 2:
                analysis.can_auto_fix = False
                analysis.should_escalate = True
                analysis.reasoning = (analysis.reasoning or "") + " (Retry limit reached - escalating to prevent infinite loop)"
            logger.info(
                "Error analysis completed: command=%s error_category=%s can_auto_fix=%s should_escalate=%s",
                command[:60], analysis.error_category, analysis.can_auto_fix, analysis.should_escalate,
            )

        return await self._run_assistant_analysis(
            prompt=prompt,
            response_model=ErrorAnalysisResult,
            assistant_model=assistant_model,
            settings=resolved_settings,
            fallback_no_model=lambda: ErrorAnalysisResult(
                error_category=ErrorAnalysisCategory.UNKNOWN,
                root_cause="No assistant model configured for error analysis",
                can_auto_fix=False,
                should_escalate=True,
                reasoning="No assistant_model configured in platform settings",
                user_message=f"Command failed with exit code {exit_code}. Error analysis unavailable - manual intervention required.",
            ),
            fallback_no_response=lambda: ErrorAnalysisResult(
                error_category=ErrorAnalysisCategory.UNKNOWN,
                root_cause="LLM returned no text content",
                can_auto_fix=False,
                should_escalate=True,
                reasoning="LLM response contained no text parts",
                user_message=f"Command failed with exit code {exit_code}. Error analysis unavailable - manual intervention required.",
            ),
            fallback_exception=lambda e: ErrorAnalysisResult(
                error_category=ErrorAnalysisCategory.UNKNOWN,
                root_cause="Error analysis failed",
                can_auto_fix=False,
                should_escalate=True,
                reasoning=f"Analysis error: {e!s}",
                user_message=f"Command failed with exit code {exit_code}. Error analysis unavailable - manual intervention required.",
            ),
            log_context="Error analysis",
            post_process=post_process,
        )

    async def analyze_file_operation_risk(
        self,
        operation: FileOperation,
        file_path: str,
        content: str | None,
        context: FileOperationRiskContext,
        settings: G8eeUserSettings,
    ) -> FileOperationRiskAnalysis:
        context = context or FileOperationRiskContext()
        git_status = context.git_status
        backup_available = context.backup_available
        resolved_settings = settings

        content_preview = content[:500] if content else "N/A"

        file_risk_persona = get_agent_persona("response_analyzer_file_risk")
        prompt = file_risk_persona.persona.format(
            operation=operation,
            file_path=file_path,
            content_preview=content_preview,
            git_status=git_status,
            backup_available=backup_available
        )

        assistant_model = resolved_settings.llm.resolved_assistant_model

        def post_process(analysis):
            system_prefixes = ("/etc/", "/usr/", "/sys/", "/proc/", "/bin/", "/sbin/", "/boot/", "/lib/")
            analysis.is_system_file = any(file_path.startswith(p) for p in system_prefixes)

            if analysis.risk_level == RiskLevel.HIGH and analysis.is_system_file:
                analysis.safe_to_proceed = False

            logger.info(
                "File operation risk analysis completed: operation=%s file_path=%s risk_level=%s is_system_file=%s safe_to_proceed=%s",
                operation, file_path, analysis.risk_level, analysis.is_system_file, analysis.safe_to_proceed,
            )

        return await self._run_assistant_analysis(
            prompt=prompt,
            response_model=FileOperationRiskAnalysis,
            assistant_model=assistant_model,
            settings=resolved_settings,
            fallback_no_model=lambda: FileOperationRiskAnalysis(risk_level=RiskLevel.HIGH, safe_to_proceed=False, is_system_file=True),
            fallback_no_response=lambda: FileOperationRiskAnalysis(
                risk_level=RiskLevel.HIGH,
                is_system_file=False,
                safe_to_proceed=False,
                blocking_issues=["Risk analysis failed - LLM returned no content"],
                approval_prompt=f"Risk analysis failed. File operation: {operation} on {file_path}\nProceed with extreme caution?",
            ),
            fallback_exception=lambda e: FileOperationRiskAnalysis(
                risk_level=RiskLevel.HIGH,
                is_system_file=False,
                safe_to_proceed=False,
                blocking_issues=["Risk analysis failed - manual review required"],
                approval_prompt=f"Risk analysis failed. File operation: {operation} on {file_path}\nProceed with extreme caution?",
            ),
            log_context="File operation risk analysis",
            post_process=post_process,
        )
