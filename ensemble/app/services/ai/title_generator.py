# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Title Generator Utility

Generates concise, meaningful case titles from descriptions using the configured LLM provider.
Uses a lightweight model optimized for quick text generation tasks.
"""

import logging
import time

from app.constants import LLM_DEFAULT_MAX_OUTPUT_TOKENS
from app.errors import OllamaEmptyResponseError
from app.llm import get_generative_lite_provider, Role
from app.llm.llm_types import Content, Part, LiteLLMSettings
from app.llm.model_call_attribution import build_model_call_telemetry, prepare_provider_call
from app.llm.model_evidence import model_boundary_hash
from app.models.agents.title_generator import CaseTitleResult
from app.models.http_context import G8eHttpContext
from app.models.model_configs import get_model_config
from app.models.settings import G8eeUserSettings
from app.utils.agent_persona_loader import get_agent_persona

logger = logging.getLogger(__name__)


async def generate_case_title(
    description: str,
    *,
    max_length: int = 80,
    settings: G8eeUserSettings,
    g8e_context: G8eHttpContext | None = None,
) -> CaseTitleResult:
    """
    Generate a concise case title from a description using the configured LLM.

    Args:
        description: The case description or initial message
        max_length: Maximum title length in characters (default: 80)
        settings: Optional Settings object
        g8e_context: Request-scoped evaluation and governance correlation

    Returns:
        CaseTitleResult containing generated title and fallback flag
    """
    if not description or not description.strip():
        return CaseTitleResult(generated_title="New Technical Support Case", fallback=True)

    if not settings:
        return CaseTitleResult(
            generated_title=_create_fallback_title(description, max_length), fallback=True
        )

    try:
        provider = get_generative_lite_provider(settings.llm)
        model = settings.llm.resolved_generative_lite_model
        if not model:
            logger.warning(
                "[TITLE-GEN] No lite_model or assistant_model configured, using fallback title"
            )
            return CaseTitleResult(
                generated_title=_create_fallback_title(description, max_length), fallback=True
            )

        persona = get_agent_persona("scribe")
        prompt = f"{persona.get_system_prompt()}\n\n<message>\n{description}\n</message>\n\nTitle:"

        logger.info(
            "[TITLE-GEN] Generating case title, description_length=%d, description=%s",
            len(description),
            description,
        )

        model_config = get_model_config(model)
        max_output_tokens = (
            model_config.max_output_tokens
            if model_config and model_config.max_output_tokens is not None
            else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        )
        lite_llm_settings = LiteLLMSettings(
            max_output_tokens=max_output_tokens,
            top_p_nucleus_sampling=model_config.top_p,
            top_k_filtering=model_config.top_k,
            stop_sequences=model_config.stop_sequences,
            system_instructions="",
            response_format=None,
        )
        prepare_provider_call(provider, g8e_context=g8e_context)
        contents = [Content(role=Role.USER, parts=[Part.from_text(prompt)])]
        input_artifact_hash = model_boundary_hash({
            "model": model,
            "contents": contents,
            "settings": lite_llm_settings,
        })
        monotonic_start = time.monotonic()
        try:
            response = await provider.generate_content_lite(
                model=model,
                contents=contents,
                lite_llm_settings=lite_llm_settings,
            )
            monotonic_end = time.monotonic()
            usage = response.usage_metadata
            finish_reason = response.candidates[0].finish_reason if response.candidates else None
            model_call = build_model_call_telemetry(
                provider=provider,
                agent_role="scribe",
                model_role="lite",
                model=model,
                monotonic_start=monotonic_start,
                monotonic_end=monotonic_end,
                input_artifact_hash=input_artifact_hash,
                input_tokens=usage.prompt_token_count,
                output_tokens=usage.candidates_token_count,
                thinking_tokens=usage.thinking_token_count,
                cache_tokens=usage.cache_token_count,
                total_tokens=usage.total_token_count,
                usage_reported=usage.usage_reported,
                finish_reason=finish_reason,
                generation_duration_seconds=usage.eval_duration_seconds,
                prompt_eval_duration_seconds=usage.prompt_eval_duration_seconds,
                total_duration_seconds=usage.total_duration_seconds,
                load_duration_seconds=usage.load_duration_seconds,
                output_artifact_hash=model_boundary_hash(response.text or ""),
            )
            if response.text is None:
                raise OllamaEmptyResponseError(
                    "LLM returned empty response",
                    model=model,
                    channel="lite",
                    done_reason="stop",
                    prompt_eval_count=None,
                    eval_count=None,
                    num_ctx=0,
                    num_predict=0,
                    thinking_len=0,
                    tool_calls_count=0,
                    ctx_overflow_suspected=False,
                )
            generated_title = response.text.strip()
        except OllamaEmptyResponseError as exc:
            logger.warning("[TITLE-GEN] No response from LLM, using fallback title: %s", exc)
            failed_call = build_model_call_telemetry(
                provider=provider,
                agent_role="scribe",
                model_role="lite",
                model=model,
                monotonic_start=monotonic_start,
                input_artifact_hash=input_artifact_hash,
                succeeded=False,
                error_type=type(exc).__name__,
            )
            return CaseTitleResult(
                generated_title=_create_fallback_title(description, max_length),
                fallback=True,
                model_call=failed_call,
            )

        if generated_title.startswith('"') and generated_title.endswith('"'):
            generated_title = generated_title[1:-1]
        if generated_title.startswith("'") and generated_title.endswith("'"):
            generated_title = generated_title[1:-1]

        if len(generated_title) > max_length:
            generated_title = generated_title[: max_length - 3] + "..."

        if not generated_title or len(generated_title.strip()) < 5:
            return CaseTitleResult(
                generated_title=_create_fallback_title(description, max_length),
                fallback=True,
                model_call=model_call,
            )

        logger.info("[TITLE-GEN] Title generated: %s", generated_title)

        return CaseTitleResult(
            generated_title=generated_title,
            fallback=False,
            model_call=model_call,
        )

    except Exception as e:
        logger.error("[TITLE-GEN] Failed to generate title: %s", e)
        return CaseTitleResult(
            generated_title=_create_fallback_title(description, max_length), fallback=True
        )


def _create_fallback_title(description: str, max_length: int) -> str:
    """
    Create a fallback title from the description by extracting the first line.

    Args:
        description: The case description
        max_length: Maximum title length

    Returns:
        Fallback title string
    """
    if not description or not description.strip():
        return "New Technical Support Case"

    first_line = description.split("\n", maxsplit=1)[0].strip()
    if not first_line:
        first_line = description.strip()

    prefixes_to_remove = ["hi", "hello", "hey", "i need help with", "can you help me with"]

    changed = True
    while changed:
        changed = False
        lower_line = first_line.lower()
        for prefix in prefixes_to_remove:
            if lower_line.startswith(prefix):
                first_line = first_line[len(prefix) :].strip().lstrip(",:;-").strip()
                changed = True
                break

    if first_line:
        first_line = first_line[0].upper() + first_line[1:]

    if len(first_line) > max_length:
        first_line = first_line[: max_length - 3] + "..."

    return first_line if first_line else "New Technical Support Case"
