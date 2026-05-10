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

import asyncio
import logging
from typing import Any
from app.errors import OllamaEmptyResponseError
from app.models.base import G8eBaseModel
from app.models.agent import OperatorContext
from app.constants import (
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    EventType,
)
from app.llm.prompts import (
    build_tribunal_generator_prompt,
    build_tribunal_prompt_fields,
)
from app.llm.llm_types import Content, Part, Role, ResponseFormat
from app.llm.provider import LLMProvider
from app.models.agents.tribunal import (
    CandidateCommand,
    AuditorClusterInfo,
    TribunalSystemError,
    TribunalGenerationFailedError,
    TribunalPassCompletedPayload,
    TribunalSessionSystemErrorPayload,
    TribunalSessionGenerationFailedPayload,
)
from app.models.model_configs import get_model_config
from app.utils.agent_persona_loader import get_agent_persona
from app.utils.json_utils import extract_json_from_text
from app.utils.command import normalise_command
from app.utils.safety import validate_command_safety
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.utils import _is_system_error, _member_for_pass

logger = logging.getLogger(__name__)

class TribunalResponse(G8eBaseModel):
    """Structured response for Tribunal command generation."""
    command: str

async def _run_generation_pass(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    pass_index: int,
    emitter: TribunalEmitter,
    pass_errors: list[str],
    command_constraints_message: str,
    round_num: int = 1,
    r1_clusters: list[Any] | None = None,
) -> str | None:
    """Run a single Tribunal generation pass."""
    member = _member_for_pass(pass_index)
    member_persona = get_agent_persona(member.value)
    fields = build_tribunal_prompt_fields(
        operator_context,
        request=request,
        guidelines=guidelines,
        default_os=DEFAULT_OS_NAME,
        default_shell=DEFAULT_SHELL,
        default_working_directory=DEFAULT_WORKING_DIRECTORY,
    )

    cluster_context = None
    if round_num == 2 and r1_clusters:
        cluster_lines = []
        for c in r1_clusters:
            cluster_lines.append(f"[{c.cluster_id}] (support: {c.support_count})\n{c.command}")
        cluster_context = "\n".join(cluster_lines)

    prompt = build_tribunal_generator_prompt(
        request=request,
        guidelines=guidelines,
        forbidden_patterns_message=fields["forbidden_patterns_message"],
        command_constraints_message=command_constraints_message,
        os=fields["os"],
        shell=fields["shell"],
        user_context=fields["user_context"],
        working_directory=fields["working_directory"],
        operator_context_str=fields["operator_context"],
        round_num=round_num,
        cluster_context=cluster_context,
        member=member.value if round_num == 2 else None,
    )

    logger.info(
        "[TRIBUNAL-PASS] pass=%d member=%s model=%s request_len=%d",
        pass_index, member.value, model, len(request),
    )

    model_config = get_model_config(model)

    response_format = None
    if model_config.supports_structured_output:
        response_format = ResponseFormat.from_pydantic_schema(
            TribunalResponse.model_json_schema(),
            name="TribunalResponse"
        )

    settings = model_config.to_litellm_settings(
        system_instructions=member_persona.get_system_prompt() or "",
        response_format=response_format,
    )

    try:
        response = await provider.generate_content_lite(
            model=model,
            contents=[Content(role=Role.USER, parts=[Part.from_text(prompt)])],
            lite_llm_settings=settings,
        )

        if not response.text or not response.text.strip():
            error_msg = f"Pass {pass_index} ({member.value}): empty response"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s", error_msg)
            return None

        raw_command = response.text.strip()

        if model_config.supports_structured_output:
            parsed = extract_json_from_text(raw_command)
            if not (isinstance(parsed, dict) and isinstance(parsed.get("command"), str)):
                error_msg = f"Pass {pass_index} ({member.value}): structured output missing 'command' field"
                pass_errors.append(error_msg)
                logger.error("[TRIBUNAL-PASS] %s (raw=%r)", error_msg, raw_command[:100])
                return None
            raw_command = parsed["command"]

        normalised = normalise_command(raw_command)

        if not normalised:
            error_msg = f"Pass {pass_index} ({member.value}): normalisation failed"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s (raw=%r)", error_msg, raw_command[:100])
            return None

        safety_result = validate_command_safety(normalised, False, False, operator_context)
        if not safety_result.is_safe:
            error_msg = f"Pass {pass_index} ({member.value}): safety validation failed: {safety_result.error_message}"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s", error_msg)
            return None

        logger.info(
            "[TRIBUNAL-PASS] pass=%d member=%s success: cmd=%r",
            pass_index, member.value, normalised[:80],
        )

        await emitter.emit(
            EventType.TRIBUNAL_VOTING_PASS_COMPLETED,
            TribunalPassCompletedPayload(
                pass_index=pass_index,
                member=member,
                candidate=normalised,
                success=True,
            ),
        )

        return normalised

    except OllamaEmptyResponseError as exc:
        error_msg = f"Pass {pass_index} ({member.value}): {exc!s}"
        pass_errors.append(error_msg)
        logger.error("[TRIBUNAL-PASS] %s", error_msg)
        return None
    except Exception as exc:
        error_msg = f"Pass {pass_index} ({member.value}): {exc!s}"
        pass_errors.append(error_msg)
        logger.error("[TRIBUNAL-PASS] %s", error_msg, exc_info=True)
        return None

def _anonymize_clusters(candidates: list[CandidateCommand]) -> tuple[list[AuditorClusterInfo], dict[str, str], dict[str, list[str]]]:
    """Anonymize R1 candidates as cluster_a, cluster_b, etc.

    Reuses the auditor's cluster anonymization helper for Round 2 peer review.

    Returns (clusters, cluster_to_cmd, cluster_to_members).
    """
    # Group candidates by command
    candidates_by_command: dict[str, list[str]] = {}
    for c in candidates:
        if c.command not in candidates_by_command:
            candidates_by_command[c.command] = []
        candidates_by_command[c.command].append(c.member.value)

    # Anonymize as cluster_a, cluster_b, ...
    clusters: list[AuditorClusterInfo] = []
    cluster_to_cmd: dict[str, str] = {}
    cluster_to_members: dict[str, list[str]] = {}

    idx = 0
    for cmd, members in candidates_by_command.items():
        c_id = f"cluster_{chr(ord('a') + idx)}"
        cluster_to_cmd[c_id] = cmd
        cluster_to_members[c_id] = members
        clusters.append(AuditorClusterInfo(
            cluster_id=c_id,
            command=cmd,
            support_count=len(members)
        ))
        idx += 1

    return clusters, cluster_to_cmd, cluster_to_members

async def _run_generation_stage(
    provider: LLMProvider,
    model: str,
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    num_passes: int,
    emitter: TribunalEmitter,
    command_constraints_message: str,
    round_num: int = 1,
    r1_clusters: list[AuditorClusterInfo] | None = None,
) -> list[CandidateCommand]:
    """Run N parallel generation passes and return successful candidates.

    Args:
        round_num: Round number (1 for initial, 2 for peer review)
        r1_clusters: Anonymized R1 clusters for Round 2 peer review context
    """
    pass_errors: list[str] = []
    pass_tasks = [
        _run_generation_pass(
            provider=provider, model=model, request=request, guidelines=guidelines,
            operator_context=operator_context, pass_index=i, emitter=emitter, pass_errors=pass_errors,
            command_constraints_message=command_constraints_message,
            round_num=round_num,
            r1_clusters=r1_clusters,
        )
        for i in range(num_passes)
    ]
    raw_results = await asyncio.gather(*pass_tasks, return_exceptions=False)
    candidates = [
        CandidateCommand(command=res, pass_index=i, member=_member_for_pass(i))
        for i, res in enumerate(raw_results) if res
    ]

    if not candidates:
        if not pass_errors:
            raise AssertionError(
                "Tribunal invariant violated: all generation passes returned None but pass_errors is empty"
            )
        if all(_is_system_error(e) for e in pass_errors):
            logger.error(
                "[TRIBUNAL] All %d generation passes failed due to system errors: %s",
                num_passes, pass_errors,
            )
            await emitter.emit(
                EventType.TRIBUNAL_SESSION_SYSTEM_ERROR,
                TribunalSessionSystemErrorPayload(
                    request=request,
                    pass_errors=pass_errors,
                ),
            )
            raise TribunalSystemError(pass_errors=pass_errors, request=request)

        logger.error("[TRIBUNAL] All generation passes failed for non-system reasons; halting execution")
        await emitter.emit(
            EventType.TRIBUNAL_SESSION_GENERATION_FAILED,
            TribunalSessionGenerationFailedPayload(
                request=request,
                pass_errors=pass_errors,
            ),
        )
        raise TribunalGenerationFailedError(
            pass_errors=pass_errors,
            request=request,
        )

    return candidates

# Removed incorrect duplicate imports at the bottom
