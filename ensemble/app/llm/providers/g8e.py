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
finish chunk; the token stream is a rendering optimization, not a governed
artifact. This matches the plan's streaming-through-governance contract:
the governed envelope carries the request and final result for the
receipt and audit chain.
"""

from __future__ import annotations

import logging
from collections.abc import AsyncGenerator

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
from app.models.internal_api import (
    InferenceDispatchRequest,
    InferenceDispatchResponse,
)

logger = logging.getLogger(__name__)

# ModelRole enum values from protocol/proto/g8e/operator/v1/operator.proto.
_ROLE_PRIMARY = 1
_ROLE_ASSISTANT = 2
_ROLE_LITE = 3


def _contents_to_prompt(
    contents: list[Content],
    system_instructions: str | None,
) -> str:
    """Flatten the conversation contents into a single prompt string.

    The dispatch endpoint carries a single prompt string (not a list of
    messages). The Inference Node's handler sends this prompt as a single
    user message to Ollama's ``/api/chat`` endpoint. The flattening
    preserves the conversation structure by formatting each turn with its
    role label so the model sees the full chat context.
    """
    lines: list[str] = []
    if system_instructions:
        lines.append(f"System: {system_instructions}")
    for content in contents:
        role = "Assistant" if content.role == "model" else content.role.capitalize()
        parts = [p.text for p in content.parts if p.text]
        if parts:
            lines.append(f"{role}: {' '.join(parts)}")
    return "\n\n".join(lines)


def _response_to_usage_metadata(result: InferenceDispatchResponse) -> UsageMetadata:
    """Build UsageMetadata from the dispatch response result."""
    if result.result is None:
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
    text = result.result.text if result.result else ""
    finish_reason = result.result.finish_reason if result.result else "stop"
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

    async def _close_resources(self):
        """Clean up provider resources. The HTTP client is owned by the
        application lifecycle, not by this provider, so close is a no-op."""
        pass

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
        prompt = _contents_to_prompt(contents, system_instructions)
        request = InferenceDispatchRequest(
            role=role,
            prompt=prompt,
            model=model or None,
            max_tokens=max_output_tokens,
        )
        self._record_model_boundary(request)
        return await self._client.dispatch_inference(request)

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
        text = result.result.text if result.result else ""
        if text:
            yield StreamChunkFromModel(text=text)
        yield StreamChunkFromModel(
            finish_reason=result.result.finish_reason if result.result else "stop",
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
        text = result.result.text if result.result else ""
        if text:
            yield StreamChunkFromModel(text=text)
        yield StreamChunkFromModel(
            finish_reason=result.result.finish_reason if result.result else "stop",
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
        text = result.result.text if result.result else ""
        if text:
            yield StreamChunkFromModel(text=text)
        yield StreamChunkFromModel(
            finish_reason=result.result.finish_reason if result.result else "stop",
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
