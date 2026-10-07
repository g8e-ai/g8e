# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import asyncio
import logging
import time
from typing import Any

from app.constants import (
    DEFAULT_OS_NAME,
    DEFAULT_SHELL,
    DEFAULT_WORKING_DIRECTORY,
    EventType,
)
from app.errors import ContextWindowExceededError, OllamaEmptyResponseError
from app.llm.llm_types import Content, GenerateContentResponse, Part, ResponseFormat, Role
from app.llm.model_call_attribution import build_model_call_telemetry, prepare_provider_call
from app.llm.model_evidence import model_boundary_hash
from app.llm.prompts import (
    build_tribunal_generator_prompt,
    build_tribunal_prompt_fields,
)
from app.llm.provider import LLMProvider
from app.models.agent import OperatorContext
from app.models.agents.tribunal import (
    AuditorClusterInfo,
    CandidateCommand,
    TribunalGenerationFailedError,
    TribunalPassCompletedPayload,
    TribunalSessionGenerationFailedPayload,
    TribunalSessionSystemErrorPayload,
    TribunalSystemError,
)
from app.models.base import G8eBaseModel
from app.models.model_configs import get_model_config
from app.models.model_telemetry import ModelCallTelemetry
from app.services.ai.generation_config_builder import AIGenerationConfigBuilder
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.ai.tribunal.utils import is_system_error, member_for_pass
from app.utils.agent_persona_loader import get_agent_persona
from app.utils.command import normalise_command
from app.utils.json_utils import extract_json_from_text
from app.utils.validation.safety import validate_command_safety

logger = logging.getLogger(__name__)


class TribunalResponse(G8eBaseModel):
    """Structured response for Tribunal command generation."""

    command: str


def _pass_model_call(
    provider: LLMProvider,
    model: str,
    response: GenerateContentResponse | None,
    monotonic_start: float,
    input_artifact_hash: str,
    error: str | None = None,
    error_type: str | None = None,
) -> ModelCallTelemetry:
    usage = response.usage_metadata if response else None
    candidates = response.candidates if response else []
    finish_reason = candidates[0].finish_reason if candidates else None
    response_text = response.text if response else ""
    return build_model_call_telemetry(
        provider=provider,
        agent_role="tribunal",
        model_role="lite",
        model=model,
        monotonic_start=monotonic_start,
        monotonic_end=time.monotonic(),
        input_artifact_hash=input_artifact_hash,
        input_tokens=usage.prompt_token_count if usage else 0,
        output_tokens=usage.candidates_token_count if usage else 0,
        thinking_tokens=usage.thinking_token_count if usage else None,
        cache_tokens=usage.cache_token_count if usage else None,
        total_tokens=usage.total_token_count if usage else 0,
        usage_reported=usage.usage_reported if usage else False,
        finish_reason=finish_reason if isinstance(finish_reason, str) else None,
        generation_duration_seconds=usage.eval_duration_seconds if usage else None,
        prompt_eval_duration_seconds=usage.prompt_eval_duration_seconds if usage else None,
        total_duration_seconds=usage.total_duration_seconds if usage else None,
        load_duration_seconds=usage.load_duration_seconds if usage else None,
        succeeded=error is None,
        error_type=error_type,
        output_artifact_hash=model_boundary_hash(response_text or ""),
    )


async def _emit_pass_observation(
    emitter: TribunalEmitter,
    pass_index: int,
    member: Any,
    provider: LLMProvider,
    model: str,
    response: GenerateContentResponse | None,
    monotonic_start: float,
    input_artifact_hash: str,
    candidate: str | None,
    error: str | None = None,
    error_type: str | None = None,
) -> None:
    model_call = _pass_model_call(
        provider,
        model,
        response,
        monotonic_start,
        input_artifact_hash,
        error=error,
        error_type=error_type,
    )
    await emitter.emit(
        EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED,
        TribunalPassCompletedPayload(
            pass_index=pass_index,
            member=member,
            candidate=candidate,
            success=error is None,
            error=error,
            provider=type(provider).__name__,
            model=model,
            input_tokens=model_call.input_tokens,
            output_tokens=model_call.output_tokens,
            thinking_tokens=model_call.thinking_tokens,
            cache_tokens=model_call.cache_tokens,
            usage_reported=model_call.usage_reported,
            finish_reason=model_call.finish_reason,
            monotonic_start=monotonic_start,
            monotonic_end=model_call.monotonic_end,
            input_artifact_hash=input_artifact_hash,
            output_artifact_hash=model_call.output_artifact_hash,
            model_boundary_privacy=model_call.model_boundary_privacy,
            succeeded=error is None,
            error_type=error_type,
            model_calls=[model_call],
        ),
    )


def _generation_prompt(
    request: str,
    guidelines: str,
    operator_context: OperatorContext | None,
    pass_index: int,
    command_constraints_message: str,
    round_num: int,
    r1_clusters: list[Any] | None,
) -> tuple[Any, Any, str]:
    """Build a member-specific prompt for one generation pass."""
    member = member_for_pass(pass_index)
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
        cluster_lines = [
            f"[{c.cluster_id}] (support: {c.support_count})\n{c.command}" for c in r1_clusters
        ]
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

    return member, member_persona, prompt


def _prepare_generation_call(
    provider: LLMProvider,
    model: str,
    request: str,
    member_persona: Any,
    prompt: str,
    emitter: TribunalEmitter,
    pass_index: int,
    member: Any,
) -> tuple[Any, list[Content], Any, str]:
    """Build and fingerprint the provider request for one generation pass."""
    logger.info(
        "[TRIBUNAL-PASS] pass=%d member=%s model=%s request_len=%d",
        pass_index,
        member.value,
        model,
        len(request),
    )

    model_config = get_model_config(model)

    response_format = None
    if model_config.supports_structured_output:
        response_format = ResponseFormat.from_pydantic_schema(
            TribunalResponse.model_json_schema(), name="TribunalResponse"
        )

    settings = AIGenerationConfigBuilder.build_lite_settings(
        model=model,
        max_tokens=None,
        system_instructions=member_persona.get_system_prompt() or "",
        response_format=response_format,
    )

    contents = [Content(role=Role.USER, parts=[Part.from_text(prompt)])]
    prepare_provider_call(provider, g8e_context=emitter.g8e_context)
    input_artifact_hash = model_boundary_hash(
        {
            "model": model,
            "contents": contents,
            "settings": settings,
        }
    )
    return model_config, contents, settings, input_artifact_hash


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
    member, member_persona, prompt = _generation_prompt(
        request,
        guidelines,
        operator_context,
        pass_index,
        command_constraints_message,
        round_num,
        r1_clusters,
    )

    model_config, contents, settings, input_artifact_hash = _prepare_generation_call(
        provider, model, request, member_persona, prompt, emitter, pass_index, member
    )
    monotonic_start = time.monotonic()
    response = None
    try:
        response = await provider.generate_content_lite(
            model=model,
            contents=contents,
            lite_llm_settings=settings,
        )
        if not response.text or not response.text.strip():
            error_msg = f"Pass {pass_index} ({member.value}): empty response"
            pass_errors.append(error_msg)
            logger.exception("[TRIBUNAL-PASS] %s", error_msg)
            await _emit_pass_observation(
                emitter,
                pass_index,
                member,
                provider,
                model,
                response,
                monotonic_start,
                input_artifact_hash,
                None,
                error_msg,
                "EmptyResponseError",
            )
            return None

        raw_command = response.text.strip()

        if model_config.supports_structured_output:
            parsed = extract_json_from_text(raw_command)
            parsed_command = parsed.get("command") if parsed is not None else None
            if not isinstance(parsed_command, str):
                error_msg = (
                    f"Pass {pass_index} ({member.value}): structured output missing 'command' field"
                )
                pass_errors.append(error_msg)
                logger.error("[TRIBUNAL-PASS] %s (raw=%r)", error_msg, raw_command[:100])
                await _emit_pass_observation(
                    emitter,
                    pass_index,
                    member,
                    provider,
                    model,
                    response,
                    monotonic_start,
                    input_artifact_hash,
                    None,
                    error_msg,
                    "StructuredOutputError",
                )
                return None
            raw_command = parsed_command

        normalised = normalise_command(raw_command)

        if not normalised:
            error_msg = f"Pass {pass_index} ({member.value}): normalisation failed"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s (raw=%r)", error_msg, raw_command[:100])
            await _emit_pass_observation(
                emitter,
                pass_index,
                member,
                provider,
                model,
                response,
                monotonic_start,
                input_artifact_hash,
                None,
                error_msg,
                "NormalizationError",
            )
            return None

        safety_result = validate_command_safety(normalised, False, False, operator_context)
        if not safety_result.is_safe:
            error_msg = f"Pass {pass_index} ({member.value}): safety validation failed: {safety_result.error_message}"
            pass_errors.append(error_msg)
            logger.error("[TRIBUNAL-PASS] %s", error_msg)
            await _emit_pass_observation(
                emitter,
                pass_index,
                member,
                provider,
                model,
                response,
                monotonic_start,
                input_artifact_hash,
                None,
                error_msg,
                "SafetyValidationError",
            )
            return None

        _log_pass_success(pass_index, member.value, normalised)

        await _emit_pass_observation(
            emitter,
            pass_index,
            member,
            provider,
            model,
            response,
            monotonic_start,
            input_artifact_hash,
            normalised,
        )

        return normalised

    except ContextWindowExceededError as exc:
        # Fixed text, not str(exc): token counts in the exception could contain
        # digits that is_system_error() would misread as an HTTP status.
        error_msg = (
            f"Pass {pass_index} ({member.value}): "
            "the conversation exceeded the model's context window"
        )
        await _record_generation_failure(
            emitter,
            pass_index,
            member,
            provider,
            model,
            response,
            monotonic_start,
            input_artifact_hash,
            pass_errors,
            error_msg,
            type(exc).__name__,
            exc,
        )

    except OllamaEmptyResponseError as exc:
        error_msg = f"Pass {pass_index} ({member.value}): {exc!s}"
        await _record_generation_failure(
            emitter,
            pass_index,
            member,
            provider,
            model,
            response,
            monotonic_start,
            input_artifact_hash,
            pass_errors,
            error_msg,
            type(exc).__name__,
        )
    except Exception as exc:
        error_msg = f"Pass {pass_index} ({member.value}): {exc!s}"
        await _record_generation_failure(
            emitter,
            pass_index,
            member,
            provider,
            model,
            response,
            monotonic_start,
            input_artifact_hash,
            pass_errors,
            error_msg,
            type(exc).__name__,
        )

    return None


def _log_pass_success(pass_index: int, member: str, command: str) -> None:
    logger.info(
        "[TRIBUNAL-PASS] pass=%d member=%s success: cmd=%r",
        pass_index,
        member,
        command[:80],
    )


async def _record_generation_failure(
    emitter: TribunalEmitter,
    pass_index: int,
    member: Any,
    provider: LLMProvider,
    model: str,
    response: GenerateContentResponse | None,
    monotonic_start: float,
    input_artifact_hash: str,
    pass_errors: list[str],
    error_msg: str,
    error_type: str,
    context_error: Exception | None = None,
) -> None:
    pass_errors.append(error_msg)
    if context_error:
        logger.error("[TRIBUNAL-PASS] %s, not retrying: %s", error_msg, context_error)
    else:
        logger.error("[TRIBUNAL-PASS] %s", error_msg)
    await _emit_pass_observation(
        emitter,
        pass_index,
        member,
        provider,
        model,
        response,
        monotonic_start,
        input_artifact_hash,
        None,
        error_msg,
        error_type,
    )


def anonymize_clusters(
    candidates: list[CandidateCommand],
) -> tuple[list[AuditorClusterInfo], dict[str, str], dict[str, list[str]]]:
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

    for idx, (cmd, members) in enumerate(candidates_by_command.items()):
        c_id = f"cluster_{chr(ord('a') + idx)}"
        cluster_to_cmd[c_id] = cmd
        cluster_to_members[c_id] = members
        clusters.append(
            AuditorClusterInfo(cluster_id=c_id, command=cmd, support_count=len(members))
        )

    return clusters, cluster_to_cmd, cluster_to_members


async def run_generation_stage(
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
            provider=provider,
            model=model,
            request=request,
            guidelines=guidelines,
            operator_context=operator_context,
            pass_index=i,
            emitter=emitter,
            pass_errors=pass_errors,
            command_constraints_message=command_constraints_message,
            round_num=round_num,
            r1_clusters=r1_clusters,
        )
        for i in range(num_passes)
    ]
    raw_results = await asyncio.gather(*pass_tasks, return_exceptions=False)
    candidates = [
        CandidateCommand(command=res, pass_index=i, member=member_for_pass(i))
        for i, res in enumerate(raw_results)
        if res
    ]

    if not candidates:
        if not pass_errors:
            raise AssertionError(
                "Tribunal invariant violated: all generation passes returned None but pass_errors is empty"
            )
        if all(is_system_error(e) for e in pass_errors):
            logger.error(
                "[TRIBUNAL] All %d generation passes failed due to system errors: %s",
                num_passes,
                pass_errors,
            )
            await emitter.emit(
                EventType.AI_CONSENSUS_SESSION_SYSTEM_ERROR,
                TribunalSessionSystemErrorPayload(
                    request=request,
                    pass_errors=pass_errors,
                ),
            )
            raise TribunalSystemError(pass_errors=pass_errors, request=request)

        logger.error(
            "[TRIBUNAL] All generation passes failed for non-system reasons; halting execution"
        )
        await emitter.emit(
            EventType.AI_CONSENSUS_SESSION_GENERATION_FAILED,
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
