# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""G8E governed-dispatch LLM provider.

This provider routes inference through the gateway's platform-internal
``/api/v1/inference/dispatch`` endpoint instead of calling an LLM backend
directly. The gateway constructs a governed envelope, resolves the Inference
Node's operator session, dispatches the request through the full L1–L5
gauntlet on the Inference Node, and returns the signed receipt and
``InferenceResult``. This is the transport layer underneath the ensemble
chat pipeline's tiered routing logic; the governed dispatch wraps the
request and result, and the receipt proves what was requested and returned.

The provider is wired into the LLM provider factory as ``LLMProvider.G8E``
and selected via the per-role provider settings (``primary_provider=g8e``,
``assistant_provider=g8e``, ``lite_provider=g8e``). It uses the
``InternalHttpClient`` (injected via the factory singleton) for mTLS
authentication to the gateway; no endpoint or API key is configured directly.

The dispatch endpoint is non-streaming: it returns the complete
``InferenceResult`` after the Inference Node finishes generation. The
stream methods yield the complete text as a single chunk followed by a
finish chunk; no token stream traverses the governed path in this
release. The provider is text-only: tool call/response and inline-data
parts in the conversation contents fail closed with a typed
``ModelCapabilityError`` rather than being silently dropped.
"""

from __future__ import annotations

import logging
from collections.abc import AsyncGenerator
from contextvars import ContextVar

from app.llm.llm_dataclasses import (
    Candidate,
    Content,
    GenerateContentResponse,
    Part,
    StreamChunkFromModel,
    UsageMetadata,
)
from app.llm.llm_types import (
    AssistantLLMSettings,
    LiteLLMSettings,
    PrimaryLLMSettings,
)
from app.llm.provider import LLMProvider
from app.models.http_context import G8eHttpContext
from app.models.internal_api import (
    InferenceDispatchRequest,
    InferenceDispatchResponse,
)
from app.models.model_telemetry import GovernedDispatchEvidence
from g8e.operator.v1.operator_pb2 import (
    MODEL_ROLE_ASSISTANT,
    MODEL_ROLE_LITE,
    MODEL_ROLE_PRIMARY,
    ExecutionStatus,
)

logger = logging.getLogger(__name__)

_ROLE_PRIMARY = MODEL_ROLE_PRIMARY
_ROLE_ASSISTANT = MODEL_ROLE_ASSISTANT
_ROLE_LITE = MODEL_ROLE_LITE


def _contents_to_prompt(
    contents: list[Content],
    system_instructions: str | None,
    model: str = "",
) -> str:
    """Flatten the conversation contents into a single prompt string.

    The dispatch endpoint carries a single prompt string (not a list of
    messages). The Inference Node's handler sends this prompt as a single
    user message to Ollama's ``/api/chat`` endpoint. The flattening
    preserves the conversation structure by formatting each turn with its
    role label so the model sees the full chat context.

    The provider is text-only this release: non-text parts fail closed
    with a typed capability error instead of being silently dropped.
    """
    # Lazy import to avoid circular dependency: app.errors -> app.models ->
    # app.llm -> providers -> this module (same convention as _capability.py).
    from app.errors import ModelCapabilityError, ToolsNotSupportedError

    lines: list[str] = []
    if system_instructions:
        lines.append(f"System: {system_instructions}")
    for content in contents:
        role = "Assistant" if content.role == "model" else content.role.capitalize()
        texts: list[str] = []
        for part in content.parts:
            if part.tool_call is not None or part.tool_response is not None:
                raise ToolsNotSupportedError(
                    "G8E governed dispatch does not support tool call/response content parts",
                    model=model,
                    service_name="g8e",
                )
            if part.inline_data is not None:
                raise ModelCapabilityError(
                    "G8E governed dispatch does not support inline data content parts",
                    model=model,
                    capability="multimodal",
                    service_name="g8e",
                )
            if part.text:
                texts.append(part.text)
        if texts:
            lines.append(f"{role}: {' '.join(texts)}")
    return "\n\n".join(lines)


def _response_to_usage_metadata(result: InferenceDispatchResponse) -> UsageMetadata:
    """Build UsageMetadata from the dispatch response result."""
    if not result.HasField("result"):
        return UsageMetadata()
    return UsageMetadata(
        prompt_token_count=result.result.prompt_tokens,
        candidates_token_count=result.result.completion_tokens,
        total_token_count=result.result.total_tokens,
        usage_reported=result.result.total_tokens > 0,
    )


def _response_to_generate_content(
    result: InferenceDispatchResponse,
) -> GenerateContentResponse:
    """Build a GenerateContentResponse from the dispatch response."""
    text = result.result.text if result.HasField("result") else ""
    finish_reason = result.result.finish_reason if result.HasField("result") else "stop"
    return GenerateContentResponse(
        candidates=[
            Candidate(
                content=Content(role="model", parts=[Part(text=text)]),
                finish_reason=finish_reason or "stop",
            )
        ],
        usage_metadata=_response_to_usage_metadata(result),
    )


class G8EProvider(LLMProvider):
    """Governed-dispatch LLM provider that routes inference through the gateway.

    Uses the ``InternalHttpClient`` to call the gateway's
    ``/api/v1/inference/dispatch`` endpoint. The gateway resolves the
    Inference Node, constructs the governed envelope, dispatches through
    the L1–L5 gauntlet, and returns the signed receipt and InferenceResult.
    """

    def __init__(self, internal_http_client):
        super().__init__()
        self._client = internal_http_client
        self._g8e_context: ContextVar[G8eHttpContext | None] = ContextVar(
            f"{type(self).__name__}_g8e_context_{id(self)}", default=None
        )
        self._governed_dispatch_evidence: ContextVar[GovernedDispatchEvidence | None] = ContextVar(
            f"{type(self).__name__}_governed_dispatch_evidence_{id(self)}", default=None
        )

    @property
    def governed_dispatch_evidence(self) -> GovernedDispatchEvidence | None:
        return self._governed_dispatch_evidence.get()

    def set_g8e_context(self, context: G8eHttpContext | None) -> None:
        """Store the turn's HTTP context so governed dispatch can propagate
        the case, investigation, task, and session identities onto the
        ``InferenceDispatchRequest``."""
        self._g8e_context.set(context)

    async def _close_resources(self):
        """Clean up provider resources. The HTTP client is owned by the
        application lifecycle, not by this provider, so close is a no-op."""

    @staticmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        """Validate G8E provider configuration.

        The G8E provider uses the InternalHttpClient for transport (mTLS
        to the gateway), so no endpoint or API key is configured directly.
        """
        return []

    async def _dispatch(
        self,
        role: int,
        model: str,
        contents: list[Content],
        system_instructions: str | None,
        max_output_tokens: int,
    ) -> InferenceDispatchResponse:
        """Dispatch a governed inference request and return the response."""
        prompt = _contents_to_prompt(contents, system_instructions, model=model)
        context = self._g8e_context.get()
        request = InferenceDispatchRequest(
            role=role,
            prompt=prompt,
            model=model or "",
            max_tokens=max_output_tokens,
            case_id=(context.case_id or "") if context else "",
            investigation_id=(context.investigation_id or "") if context else "",
            task_id=(context.task_id or "") if context else "",
            web_session_id=(context.web_session_id or "") if context else "",
            cli_session_id=(context.cli_session_id or "") if context else "",
        )
        self._record_model_boundary(request)
        self._governed_dispatch_evidence.set(None)
        response = await self._client.dispatch_inference(request)
        self._governed_dispatch_evidence.set(
            GovernedDispatchEvidence(
                transaction_id=response.transaction_id,
                result_digest=(
                    response.result.result_digest if response.HasField("result") else ""
                ),
                receipt_status=(
                    ExecutionStatus.Name(response.receipt.status)
                    if response.HasField("receipt")
                    else ""
                ),
            )
        )
        return response

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        result = await self._dispatch(
            _ROLE_PRIMARY,
            model,
            contents,
            primary_llm_settings.system_instructions,
            primary_llm_settings.max_output_tokens,
        )
        text = result.result.text if result.HasField("result") else ""
        if text:
            yield StreamChunkFromModel(text=text)
        yield StreamChunkFromModel(
            finish_reason=result.result.finish_reason if result.HasField("result") else "stop",
            usage_metadata=_response_to_usage_metadata(result),
        )

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        result = await self._dispatch(
            _ROLE_PRIMARY,
            model,
            contents,
            primary_llm_settings.system_instructions,
            primary_llm_settings.max_output_tokens,
        )
        return _response_to_generate_content(result)

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        result = await self._dispatch(
            _ROLE_ASSISTANT,
            model,
            contents,
            assistant_llm_settings.system_instructions,
            assistant_llm_settings.max_output_tokens,
        )
        text = result.result.text if result.HasField("result") else ""
        if text:
            yield StreamChunkFromModel(text=text)
        yield StreamChunkFromModel(
            finish_reason=result.result.finish_reason if result.HasField("result") else "stop",
            usage_metadata=_response_to_usage_metadata(result),
        )

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        result = await self._dispatch(
            _ROLE_ASSISTANT,
            model,
            contents,
            assistant_llm_settings.system_instructions,
            assistant_llm_settings.max_output_tokens,
        )
        return _response_to_generate_content(result)

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        result = await self._dispatch(
            _ROLE_LITE,
            model,
            contents,
            lite_llm_settings.system_instructions,
            lite_llm_settings.max_output_tokens,
        )
        text = result.result.text if result.HasField("result") else ""
        if text:
            yield StreamChunkFromModel(text=text)
        yield StreamChunkFromModel(
            finish_reason=result.result.finish_reason if result.HasField("result") else "stop",
            usage_metadata=_response_to_usage_metadata(result),
        )

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        result = await self._dispatch(
            _ROLE_LITE,
            model,
            contents,
            lite_llm_settings.system_instructions,
            lite_llm_settings.max_output_tokens,
        )
        return _response_to_generate_content(result)
