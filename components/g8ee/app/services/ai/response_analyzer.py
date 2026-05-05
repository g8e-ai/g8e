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

import json
import logging
import time
from collections.abc import Callable
from pathlib import Path
from typing import TypeVar

import app.llm.llm_types as types
from app.models.settings import G8eeUserSettings
from app.errors import OllamaEmptyResponseError
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
from app.utils.agent_persona_loader import get_agent_persona, AgentPersona
from app.constants.paths import PATHS

logger = logging.getLogger(__name__)

T = TypeVar("T", bound=G8eBaseModel)


def _load_security_constraints() -> dict:
    """Load security constraints from shared model."""
    shared_models_dir = PATHS["infra"]["shared_models_dir"]
    shared_models_path = Path(shared_models_dir) / "security_constraints.json"
    try:
        with open(shared_models_path, "r") as f:
            constraints = json.load(f)
        logger.info("Loaded security constraints from %s", shared_models_path)
        return constraints
    except Exception as e:
        logger.error("Failed to load security constraints from %s: %s", shared_models_path, e)
        return {}


_SECURITY_CONSTRAINTS = _load_security_constraints()
SYSTEM_PATH_PREFIXES = tuple(_SECURITY_CONSTRAINTS.get("system_path_prefixes", {}).get("prefixes", ["/etc/", "/usr/", "/sys/", "/proc/", "/bin/", "/sbin/", "/boot/", "/lib/"]))
HIGH_RISK_SYSTEM_FILES = _SECURITY_CONSTRAINTS.get("high_risk_system_files", {})


def _build_warden_command_risk_template(
    command: str,
    justification: str,
    working_dir: str,
    investigation_context: str = "",
) -> str:
    """Build the Warden command risk analysis template using centralized XML formatting.

    Uses AgentPersona.format_xml_tag to guarantee hard structural boundaries.
    """
    parts = []

    parts.append(AgentPersona.format_xml_tag("command", command))
    parts.append(AgentPersona.format_xml_tag("justification", justification))
    parts.append(AgentPersona.format_xml_tag("working_directory", working_dir))
    if investigation_context:
        parts.append(AgentPersona.format_xml_tag("investigation_context", investigation_context))

    output_format = """Respond with ONLY a JSON object matching this exact schema, with no prose, no markdown fences, and no additional fields:
{{"risk_level": "LOW"}}  OR  {{"risk_level": "MEDIUM"}}  OR  {{"risk_level": "HIGH"}}"""
    parts.append(AgentPersona.format_xml_tag("output_format", output_format))

    return "\n\n".join(parts)


def _build_warden_error_template(
    command: str,
    exit_code: int | None,
    stdout: str,
    stderr: str,
    retry_count: int,
    working_dir: str,
) -> str:
    """Build the Warden error analysis template using centralized XML formatting.

    Uses AgentPersona.format_xml_tag to guarantee hard structural boundaries.
    """
    parts = []

    parts.append(AgentPersona.format_xml_tag("failed_command", command))
    parts.append(AgentPersona.format_xml_tag("exit_code", str(exit_code)))
    parts.append(AgentPersona.format_xml_tag("stdout", stdout))
    parts.append(AgentPersona.format_xml_tag("stderr", stderr))

    context = f"""Retry Count: {retry_count}
Working Directory: {working_dir}"""
    parts.append(AgentPersona.format_xml_tag("context", context))

    auto_fixable_errors = """- Missing dependencies or commands: suggested install command
- Syntax errors in commands (wrong flags, typos): corrected command
- Missing directories: suggested mkdir command
- Port conflicts: suggest killing process or using different port"""
    parts.append(AgentPersona.format_xml_tag("auto_fixable_errors", auto_fixable_errors))

    escalate_to_human = """- Authentication or permission failures requiring manual intervention
- Invalid API keys or credentials
- Critical system or hardware failures
- Data corruption or ambiguous errors
- Retry limit exceeded (retry_count >= 2)
- Configuration issues requiring human access"""
    parts.append(AgentPersona.format_xml_tag("escalate_to_human", escalate_to_human))

    parts.append("Based on the information above, analyze the failure and fill in ALL response fields.")

    return "\n\n".join(parts)


def _build_warden_file_risk_template(
    operation: str,
    file_path: str,
    content_preview: str,
    git_status: str,
    backup_available: bool,
) -> str:
    """Build the Warden file operation risk template using centralized XML formatting.

    Uses AgentPersona.format_xml_tag to guarantee hard structural boundaries.
    """
    parts = []

    parts.append(AgentPersona.format_xml_tag("operation", operation))
    parts.append(AgentPersona.format_xml_tag("file_path", file_path))
    parts.append(AgentPersona.format_xml_tag("content_preview", content_preview))

    context = f"""Git Status: {git_status}
Backup Available: {backup_available}"""
    parts.append(AgentPersona.format_xml_tag("context", context))

    high_risk_paths = "As defined in system security policy"
    high_risk_files = "As defined in system security policy"
    high_risk_patterns = "As defined in system security policy"
    system_file_patterns = f"""HIGH risk paths: {high_risk_paths}
HIGH risk files: {high_risk_files}, {high_risk_patterns}"""
    parts.append(AgentPersona.format_xml_tag("system_file_patterns", system_file_patterns))

    risk_levels = """LOW: Temporary files or artifacts
MEDIUM: Project source files or local configuration
HIGH: Global configuration or irreversible system changes"""
    parts.append(AgentPersona.format_xml_tag("risk_levels", risk_levels))

    blocking_conditions = """- Unauthorized system file access
- Destructive operation with no backup available
- Operation violating system integrity policy"""
    parts.append(AgentPersona.format_xml_tag("blocking_conditions", blocking_conditions))

    parts.append("Based on the information above, assess the risk and fill in ALL response fields. You MUST set is_system_file to true or false (never omit it). You MUST set safe_to_proceed to false for any HIGH risk system file operation.")

    return "\n\n".join(parts)


class AIResponseAnalyzer:
    """Analyzes AI responses and performs defensive operations.

    Responsibilities:
    - Defensive operations analysis (command risk, error analysis, file operations)
    """

    def __init__(self):
        logger.info("AIResponseAnalyzer initialized")

    async def _run_lite_analysis(
        self,
        prompt: str,
        response_model: type[T],
        lite_model: str | None,
        settings: G8eeUserSettings,
        fallback_no_model: Callable[[], T],
        fallback_no_response: Callable[[], T],
        fallback_exception: Callable[[Exception], T],
        log_context: str,
        post_process: Callable[[T], None] | None = None,
    ) -> T:
        if not lite_model:
            logger.warning("%s: no lite_model configured", log_context)
            return fallback_no_model()

        try:
            client = get_llm_provider(settings.llm, is_lite=True)
            config = AIGenerationConfigBuilder.build_lite_settings(
                model=lite_model,
                max_tokens=settings.llm.llm_max_tokens,
                system_instructions=prompt,
                response_format=types.ResponseFormat.from_pydantic_schema(response_model.model_json_schema()),
            )
            llm_call_start = time.time()
            response = await client.generate_content_lite(
                model=lite_model,
                contents=[types.Content(role=Role.USER, parts=[types.Part(text=prompt)])],
                lite_llm_settings=config,
            )
            llm_call_duration_ms = (time.time() - llm_call_start) * 1000
            logger.info("[WARDEN-LLM] %s LLM call duration_ms=%.2f", log_context, llm_call_duration_ms)

            response_text = response.text
            analysis = parse_structured_response(response_text, response_model)

            if post_process:
                post_process(analysis)

            logger.info("%s completed", log_context)
            return analysis
        except Exception as e:
            if isinstance(e, OllamaEmptyResponseError):
                logger.error("%s: LLM returned no text content: %s", log_context, e)
                return fallback_no_response()
            logger.error("%s failed: %s", log_context, e, exc_info=True)
            return fallback_exception(e)

    async def analyze_command_risk(
        self,
        command: str,
        justification: str,
        context: CommandRiskContext,
        settings: G8eeUserSettings,
    ) -> CommandRiskAnalysis:
        analysis_start_time = time.time()
        context = context or CommandRiskContext()
        working_dir = context.working_directory
        investigation_context = context.investigation_context
        resolved_settings = settings

        prompt_build_start = time.time()
        command_risk_persona = get_agent_persona("warden_command_risk")
        template = _build_warden_command_risk_template(
            command=command,
            justification=justification,
            working_dir=working_dir,
            investigation_context=investigation_context,
        )
        prompt = f"{command_risk_persona.get_system_prompt()}\n\n{template}"
        prompt_build_duration_ms = (time.time() - prompt_build_start) * 1000
        logger.info("[WARDEN-COMMAND-RISK] command=%r prompt_build_duration_ms=%.2f", command[:60], prompt_build_duration_ms)

        lite_model = resolved_settings.llm.resolved_lite_model

        def log_result(analysis: CommandRiskAnalysis) -> None:
            total_duration_ms = (time.time() - analysis_start_time) * 1000
            logger.info("[WARDEN-COMMAND-RISK] Completed command=%r risk_level=%s total_duration_ms=%.2f", command[:60], analysis.risk_level, total_duration_ms)

        return await self._run_lite_analysis(
            prompt=prompt,
            response_model=CommandRiskAnalysis,
            lite_model=lite_model,
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
        analysis_start_time = time.time()
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

        prompt_build_start = time.time()
        error_persona = get_agent_persona("warden_error")
        template = _build_warden_error_template(
            command=command,
            exit_code=exit_code,
            stdout=stdout[:1000],
            stderr=stderr[:1000],
            retry_count=retry_count,
            working_dir=working_dir
        )
        prompt = f"{error_persona.get_system_prompt()}\n\n{template}"
        prompt_build_duration_ms = (time.time() - prompt_build_start) * 1000
        logger.info("[WARDEN-ERROR] command=%r retry_count=%d prompt_build_duration_ms=%.2f", command[:60], retry_count, prompt_build_duration_ms)

        lite_model = resolved_settings.llm.resolved_lite_model

        def post_process(analysis: ErrorAnalysisResult) -> None:
            if retry_count >= 2:
                analysis.can_auto_fix = False
                analysis.should_escalate = True
                analysis.reasoning = (analysis.reasoning or "") + " (Retry limit reached - escalating to prevent infinite loop)"
            total_duration_ms = (time.time() - analysis_start_time) * 1000
            logger.info(
                "[WARDEN-ERROR] Completed command=%r error_category=%s can_auto_fix=%s should_escalate=%s total_duration_ms=%.2f",
                command[:60], analysis.error_category, analysis.can_auto_fix, analysis.should_escalate, total_duration_ms,
            )

        return await self._run_lite_analysis(
            prompt=prompt,
            response_model=ErrorAnalysisResult,
            lite_model=lite_model,
            settings=resolved_settings,
            fallback_no_model=lambda: ErrorAnalysisResult(
                error_category=ErrorAnalysisCategory.UNKNOWN,
                root_cause="No lite model configured for error analysis",
                can_auto_fix=False,
                should_escalate=True,
                reasoning="No lite_model configured in platform settings",
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
        analysis_start_time = time.time()
        context = context or FileOperationRiskContext()
        git_status = context.git_status
        backup_available = context.backup_available
        resolved_settings = settings

        content_preview = content[:500] if content else "N/A"

        prompt_build_start = time.time()
        file_risk_persona = get_agent_persona("warden_file_risk")
        template = _build_warden_file_risk_template(
            operation=operation,
            file_path=file_path,
            content_preview=content_preview,
            git_status=git_status,
            backup_available=backup_available
        )
        prompt = f"{file_risk_persona.get_system_prompt()}\n\n{template}"
        prompt_build_duration_ms = (time.time() - prompt_build_start) * 1000
        logger.info("[WARDEN-FILE-RISK] operation=%s file_path=%r prompt_build_duration_ms=%.2f", operation, file_path[:60], prompt_build_duration_ms)

        lite_model = resolved_settings.llm.resolved_lite_model

        def post_process(analysis: FileOperationRiskAnalysis) -> None:
            analysis.is_system_file = any(file_path.startswith(p) for p in SYSTEM_PATH_PREFIXES)

            # System files are blocked if HIGH risk, UNLESS a backup is available.
            # This follows Warden's discipline: "A sed -i on a config file is MEDIUM if a .bak was just created."
            if analysis.risk_level == RiskLevel.HIGH and analysis.is_system_file and not backup_available:
                analysis.safe_to_proceed = False

            total_duration_ms = (time.time() - analysis_start_time) * 1000
            logger.info(
                "[WARDEN-FILE-RISK] Completed operation=%s file_path=%r risk_level=%s is_system_file=%s safe_to_proceed=%s total_duration_ms=%.2f",
                operation, file_path, analysis.risk_level, analysis.is_system_file, analysis.safe_to_proceed, total_duration_ms,
            )

        return await self._run_lite_analysis(
            prompt=prompt,
            response_model=FileOperationRiskAnalysis,
            lite_model=lite_model,
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
