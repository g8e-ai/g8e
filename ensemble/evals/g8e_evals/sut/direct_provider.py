# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""DirectProviderSUT - calls the model directly through the shared provider
abstraction without g8ee prompts, agent orchestration, the gateway, or the
operator.

This arm isolates raw model behaviour from all g8e orchestration and
governance. It uses the same ``app.llm`` provider classes that g8ee uses
internally, but constructs a single-turn request with the raw user prompt,
no system instructions, no tools, no thinking configuration, and no agent
loop. The response is the model's direct output.

Tests prove the bypass: no g8ee HTTP call is made, no SSE trail is
produced, no receipt is collected, and no governance event fires.
"""

from __future__ import annotations

import logging
import time
from typing import Any

from pydantic import BaseModel, computed_field

from app.constants import LLMProvider
from app.llm.factory import get_llm_provider
from app.llm.model_evidence import (
    model_boundary_hash,
    recorded_model_boundary_hash,
    recorded_model_boundary_privacy,
)
from app.llm.llm_types import (
    Content,
    GenerateContentResponse,
    Part,
    PrimaryLLMSettings,
    ThinkingConfig,
)
from app.models.model_telemetry import ModelBoundaryPrivacyAttestation
from app.models.settings import LLMSettings

from g8e_evals.arms import Arm
from g8e_evals.harness import BindingType, Response, SUTConfig, Task

logger = logging.getLogger(__name__)

__all__ = ["DirectCallEvidence", "DirectProviderSUT"]


class DirectCallEvidence(BaseModel):
    """Evidence record for a direct provider call.

    This is not a ``ChatEvaluationReceipt`` (no SSE trail, no g8ee case or
    investigation) and not an ``ActionReceipt`` (no governance mutation).
    It captures the raw provider response metadata needed for stage
    instrumentation and cost analysis.
    """

    binding: str = "direct_provider"
    provider: str
    model: str
    finish_reason: str | None = None
    prompt_token_count: int = 0
    candidates_token_count: int = 0
    total_token_count: int = 0
    thinking_token_count: int = 0
    cache_token_count: int = 0
    usage_reported: bool = False
    monotonic_start: float = 0.0
    monotonic_end: float = 0.0
    input_artifact_hash: str = ""
    output_artifact_hash: str = ""
    model_boundary_privacy: ModelBoundaryPrivacyAttestation | None = None
    error: str | None = None

    @computed_field
    @property
    def elapsed_s(self) -> float:
        return self.monotonic_end - self.monotonic_start


class DirectProviderSUT:
    """Direct provider SUT. One model call per task, no g8ee or governance."""

    def __init__(self, config: SUTConfig):
        self.config = config
        primary = config.primary
        if not primary.provider or not primary.model:
            raise ValueError(
                "DirectProviderSUT requires a primary provider and model. "
                "Provide them via --provider/--model or G8E_TEST_LLM_PRIMARY_*."
            )
        self._provider_str = primary.provider
        self._model = primary.model
        self._settings = _build_llm_settings(primary)
        self._provider = get_llm_provider(self._settings)
        if primary.provider and primary.model:
            self.model_provider = f"{primary.provider}:{primary.model}"
        else:
            self.model_provider = primary.model or "direct:unknown"

    async def check_settings(self) -> None:
        """No remote settings to fetch for the direct arm.

        The direct arm bypasses g8ee entirely, so there is no settings
        endpoint to query. Provider construction in ``__init__`` is the
        only preflight.
        """
        return

    async def get_answer(self, task: Task) -> Response:
        """Call the model directly with the raw user prompt.

        No g8ee chat request, no SSE drain, no receipt collection, no
        governance events. The prompt goes to the provider as a single
        user turn with no system instructions and no tools.
        """
        contents = [Content(role="user", parts=[Part.from_text(task.prompt)])]
        primary_settings = PrimaryLLMSettings(
            system_instructions=None,
            tools=[],
            thinking_config=ThinkingConfig(),
        )

        self._provider.clear_input_artifact_hash()
        input_artifact_hash = model_boundary_hash({
            "model": self._model,
            "contents": contents,
            "settings": primary_settings,
        })
        start = time.monotonic()
        try:
            response: GenerateContentResponse = await self._provider.generate_content_primary(
                model=self._model,
                contents=contents,
                primary_llm_settings=primary_settings,
            )
        except Exception as e:
            input_artifact_hash = recorded_model_boundary_hash(self._provider, input_artifact_hash)
            end = time.monotonic()
            logger.warning("Direct provider call failed for task %s: %s", task.id, e)
            evidence = DirectCallEvidence(
                provider=self._provider_str,
                model=self._model,
                monotonic_start=start,
                monotonic_end=end,
                input_artifact_hash=input_artifact_hash,
                model_boundary_privacy=recorded_model_boundary_privacy(self._provider),
                error=str(e),
            )
            return Response(
                answer="",
                model=self.model_provider,
                arm=Arm.DIRECT,
                binding=BindingType.UNBOUND,
                unbound_reason=f"direct provider call failed: {e}",
                chat_evidence=_DirectEvidenceWrapper(evidence),
            )

        input_artifact_hash = recorded_model_boundary_hash(self._provider, input_artifact_hash)
        end = time.monotonic()
        answer_text = response.text or ""
        usage = response.usage_metadata

        evidence = DirectCallEvidence(
            provider=self._provider_str,
            model=self._model,
            finish_reason=response.candidates[0].finish_reason if response.candidates else None,
            prompt_token_count=usage.prompt_token_count,
            candidates_token_count=usage.candidates_token_count,
            total_token_count=usage.total_token_count,
            thinking_token_count=usage.thinking_token_count,
            cache_token_count=usage.cache_token_count,
            usage_reported=usage.usage_reported,
            monotonic_start=start,
            monotonic_end=end,
            input_artifact_hash=input_artifact_hash,
            output_artifact_hash=model_boundary_hash(answer_text),
            model_boundary_privacy=recorded_model_boundary_privacy(self._provider),
        )

        return Response(
            answer=answer_text,
            model=self.model_provider,
            arm=Arm.DIRECT,
            binding=BindingType.UNBOUND,
            unbound_reason="direct arm (no governance, no receipt binding)",
            chat_evidence=_DirectEvidenceWrapper(evidence),
        )

    async def close(self) -> None:
        """Close the underlying provider client."""
        await self._provider.force_close()


class _DirectEvidenceWrapper:
    """Adapter so ``Response.chat_evidence`` can carry a ``DirectCallEvidence``.

    The ``ChatEvaluationReceipt`` type used by ``G8eeChatSUT`` is a Pydantic
    model with SSE-trail-specific fields. The direct arm has no SSE trail,
    so it uses ``DirectCallEvidence`` instead. This wrapper exposes the
    ``model_dump`` and ``terminal_event`` attributes that the CLI and
    report writer expect, without forcing the direct arm into the SSE
    evidence schema.
    """

    def __init__(self, evidence: DirectCallEvidence):
        self._evidence = evidence

    def model_dump(self) -> dict[str, Any]:
        return self._evidence.model_dump()

    @property
    def terminal_event(self) -> str | None:
        if self._evidence.error:
            return "direct.provider.failed"
        return "direct.provider.completed"

    @property
    def event_count(self) -> int:
        return 0

    @property
    def answer_chars(self) -> int:
        return 0


def _build_llm_settings(primary: Any) -> LLMSettings:
    """Construct a minimal ``LLMSettings`` with only the primary role set."""
    provider_enum = LLMProvider(primary.provider)
    return LLMSettings(
        llm_primary_provider=provider_enum,
        llm_model=primary.model,
        primary_api_key=primary.api_key,
        primary_endpoint=primary.endpoint,
    )
