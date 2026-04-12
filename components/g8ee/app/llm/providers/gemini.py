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

"""
Google Gemini LLM Provider

Wraps the google-genai SDK for SaaS mode. Only loaded when LLM_PROVIDER=gemini.
Translates between canonical types and google.genai.types.

Thought signatures (Gemini 3+ strict):
  - MUST be returned on every tool_call Part (400 error if omitted).
  - SHOULD be returned on all other Part types for best reasoning quality.
  - NEVER merge a Part that has a signature with one that does not.
  - NEVER merge two Parts that both have signatures.
  - Signature-only Parts (no text, no FC): emit as empty-text Parts — valid
    per the streaming spec ("may arrive in a final chunk with an empty text
    part").
  - Canonical Part.thought_signature is stored as a base64 string; pass it
    through as-is when building outbound dicts (the SDK handles wire encoding).
  - Inbound SDK thought_signature is bytes; encode to base64 str for canonical.

Thinking API split:
  - Gemini 3:   uses thinking_level (high/medium/low/minimal) + include_thoughts.
  - Gemini 3.1: uses thinking_level (high/medium/low/minimal) + include_thoughts.

Temperature (Gemini 3):
  - Default is 1.0. The docs strongly recommend keeping it at 1.0.
  - Setting it below 1.0 may cause looping or degraded performance on complex
    reasoning tasks. Pass whatever the caller configures; callers should not
    set it below 1.0 for Gemini 3 models.
"""

import asyncio
import base64
import logging
from collections.abc import AsyncGenerator
from typing import Any

import httpx
from tenacity import (
    AsyncRetrying,
    retry_if_exception,
    stop_after_attempt,
    wait_exponential,
)

from app.constants import GeminiRole
from app.llm.llm_types import (
    AssistantLLMSettings,
    Candidate,
    Content,
    LiteLLMSettings,
    PrimaryLLMSettings,
    ToolCall,
    GenerateContentResponse,
    Part,
    Role,
    SdkGroundingChunk,
    SdkGroundingRawData,
    SdkGroundingSegment,
    SdkGroundingSupport,
    SdkGroundingWebSource,
    SdkSearchEntryPoint,
    StreamChunkFromModel,
    ThoughtSignature,
    ToolGroup,
    UsageMetadata,
)
from app.models.base import G8eBaseModel, Field, field_serializer, field_validator
from app.models.model_configs import get_model_config

from ..provider import LLMProvider

logger = logging.getLogger(__name__)

_STREAM_END = object()


# =============================================================================
# LLM-boundary models for the google.genai SDK
#
# These G8eBaseModel subclasses represent the known shapes of dicts passed to
# the genai SDK. flatten_for_llm() is called at the LLM boundary to produce
# the plain dicts the SDK expects.
# =============================================================================


class GenaiToolCallDict(G8eBaseModel):
    id: str | None = None
    name: str
    args: dict[str, object]


class GenaiToolResponseDict(G8eBaseModel):
    id: str | None = None
    name: str
    response: dict[str, object]


class GenaiPartDict(G8eBaseModel):
    text: str | None = None
    function_call: GenaiToolCallDict | None = Field(default=None, alias="function_call")
    function_response: GenaiToolResponseDict | None = Field(default=None, alias="function_response")
    thought: bool | None = None
    thought_signature: str | None = None


class GenaiContentDict(G8eBaseModel):
    role: str
    parts: list[GenaiPartDict]


class GenaiRequestKwargs(G8eBaseModel):
    model: str
    contents: list[dict[str, object]]
    config: Any


def _sig_from_sdk(raw) -> ThoughtSignature | None:
    """Normalise an inbound SDK thought_signature to a canonical ThoughtSignature.

    Delegates to ThoughtSignature.from_sdk() which owns the normalisation
    contract. Returns None when raw is None/falsy.
    """
    return ThoughtSignature.from_sdk(raw)


def _content_to_genai(content: Content) -> GenaiContentDict:
    """Convert canonical Content to a google.genai-compatible typed model.

    Rules enforced here per Gemini 3 thought-signature spec:
    - Each canonical Part maps to exactly one outbound GenaiPartDict.
    - thought_signature.value is passed through unchanged (base64 str).
    - Thought parts (thought=True) are emitted with thought=True.
    - Signature-only Parts are emitted as empty-text Parts so the API
      receives the signature context without a spurious text payload.
    - Parts with no payload and no signature are silently dropped.
    """
    parts: list[GenaiPartDict] = []
    for p in content.parts:
        part = GenaiPartDict()

        if p.tool_call:
            part.function_call = GenaiToolCallDict(
                id=p.tool_call.id,
                name=p.tool_call.name,
                args=p.tool_call.args,
            )
        elif p.tool_response:
            part.function_response = GenaiToolResponseDict(
                id=p.tool_response.id,
                name=p.tool_response.name,
                response=p.tool_response.response,
            )
        elif p.thought and p.text is not None:
            part.text = p.text
            part.thought = True
        elif p.text is not None:
            part.text = p.text
        elif p.thought_signature:
            part.text = ""
        else:
            continue

        if p.thought and part.thought is not True:
            part.thought = True
        if p.thought_signature:
            part.thought_signature = p.thought_signature.value

        parts.append(part)

    # Gemini 3 models use the user role for tool/function responses in multi-turn.
    # The Tool role is supported in some versions but User is more reliable for 3.
    role = content.role
    if role == Role.TOOL:
        role = Role.USER

    return GenaiContentDict(role=role, parts=parts)


def _tools_to_genai(tools: list[ToolGroup] | None) -> list | None:
    """Convert canonical Tool list to google.genai Tool format."""
    if not tools:
        return None
    from google.genai import types as genai_types
    genai_tools = []
    for tool in tools:
        # Each Tool in google.genai is a container for function_declarations, google_search, etc.
        # We map our canonical ToolGroup which contains tool declarations to a google.genai.types.Tool.
        funcs = []
        for d in (tool.tools or []):
            funcs.append({
                "name": d.name,
                "description": d.description,
                "parameters": d.parameters,
            })
        
        if funcs:
            genai_tools.append(genai_types.Tool(function_declarations=funcs))
        
        if getattr(tool, "google_search", False):
            genai_tools.append(genai_types.Tool(google_search=genai_types.GoogleSearch()))
            
    return genai_tools or None


def _usage_from_sdk(um) -> UsageMetadata:
    """Build canonical UsageMetadata from an SDK usage_metadata object."""
    return UsageMetadata(
        prompt_token_count=getattr(um, "prompt_token_count", 0) or 0,
        candidates_token_count=getattr(um, "candidates_token_count", 0) or 0,
        total_token_count=getattr(um, "total_token_count", 0) or 0,
        thinking_token_count=getattr(um, "thoughts_token_count", 0) or 0,
    )


def _finish_reason_from_candidate(candidate) -> str | None:
    """Extract a string finish_reason from an SDK Candidate."""
    fr = getattr(candidate, "finish_reason", None)
    if not fr:
        return None
    return getattr(fr, "name", None) or str(fr)


def _grounding_from_sdk_candidate(candidate) -> SdkGroundingRawData | None:
    """Extract typed SdkGroundingRawData from a Gemini SDK Candidate, or None if absent."""
    gm = getattr(candidate, "grounding_metadata", None)
    if not gm:
        return None

    web_search_queries: list[str] = []
    raw_queries = getattr(gm, "web_search_queries", None)
    if raw_queries:
        web_search_queries = list(raw_queries)

    grounding_chunks: list[SdkGroundingChunk] = []
    for raw_chunk in getattr(gm, "grounding_chunks", None) or []:
        raw_web = getattr(raw_chunk, "web", None)
        if raw_web is not None:
            grounding_chunks.append(SdkGroundingChunk(
                web=SdkGroundingWebSource(
                    uri=getattr(raw_web, "uri", "") or "",
                    title=getattr(raw_web, "title", "") or "",
                )
            ))
        else:
            grounding_chunks.append(SdkGroundingChunk(web=None))

    grounding_supports: list[SdkGroundingSupport] = []
    for raw_support in getattr(gm, "grounding_supports", None) or []:
        raw_seg = getattr(raw_support, "segment", None)
        if raw_seg is None:
            continue
        raw_indices = getattr(raw_support, "grounding_chunk_indices", None)
        try:
            chunk_indices = list(raw_indices) if raw_indices is not None else []
        except TypeError:
            chunk_indices = []
        grounding_supports.append(SdkGroundingSupport(
            segment=SdkGroundingSegment(
                start_index=getattr(raw_seg, "start_index", 0) or 0,
                end_index=getattr(raw_seg, "end_index", 0) or 0,
                text=getattr(raw_seg, "text", "") or "",
            ),
            grounding_chunk_indices=chunk_indices,
        ))

    search_entry_point: SdkSearchEntryPoint
    raw_sep = getattr(gm, "search_entry_point", None)
    if raw_sep is not None:
        search_entry_point = SdkSearchEntryPoint(
            rendered_content=getattr(raw_sep, "rendered_content", "") or "",
        )

    return SdkGroundingRawData(
        web_search_queries=web_search_queries,
        grounding_chunks=grounding_chunks,
        grounding_supports=grounding_supports,
        search_entry_point=search_entry_point,
    )


def _parts_from_sdk_candidate(candidate) -> list[Part]:
    """Convert SDK Candidate content parts to canonical Part objects.

    Ordering per Gemini 3 spec (thought signatures guide):
    1. thought parts (thought=True) — always carry a signature if thinking is on
    2. tool_call parts — signature is REQUIRED; carry it through
    3. text parts — may carry a signature; carry it through
    4. signature-only parts — no text, no FC, but signature present
    """
    parts: list[Part] = []
    if not (candidate.content and candidate.content.parts):
        return parts

    for sdk_part in candidate.content.parts:
        sig = _sig_from_sdk(getattr(sdk_part, "thought_signature", None))
        thought_flag = bool(getattr(sdk_part, "thought", False))
        fc = getattr(sdk_part, "function_call", None)
        text = getattr(sdk_part, "text", None)

        # Normalize literal '\n\n' strings that Gemini occasionally emits
        # into actual newline characters.
        if text and "\\n" in text:
            text = text.replace("\\n", "\n")

        if thought_flag and text:
            parts.append(Part(text=text, thought=True, thought_signature=sig))
        elif fc:
            parts.append(Part(
                tool_call=ToolCall(
                    name=fc.name,
                    args=dict(fc.args) if fc.args else {},
                    id=getattr(fc, "id", None),
                ),
                thought_signature=sig,
            ))
        elif text:
            parts.append(Part(text=text, thought_signature=sig))
        elif sig:
            parts.append(Part(thought_signature=sig))

    return parts


class GeminiProvider(LLMProvider):
    """Provider wrapping Google's genai SDK for Gemini models."""

    _TIMEOUT = httpx.Timeout(connect=10.0, read=300.0, write=30.0, pool=5.0)
    _RETRY_STATUS_CODES = frozenset({429, 503})
    _RETRY_ATTEMPTS = 4

    @classmethod
    def _is_retryable(cls, exc: BaseException) -> bool:
        """Return True for transient API errors that warrant a retry."""
        if isinstance(exc, (httpx.TimeoutException, TimeoutError)):
            return True
        code = getattr(exc, "code", None) or getattr(exc, "status_code", None)
        if code in cls._RETRY_STATUS_CODES:
            return True
        msg = str(exc).lower()
        return (
            "503" in msg
            or "429" in msg
            or "service unavailable" in msg
            or "too many requests" in msg
            or "deadline exceeded" in msg
            or "timed out" in msg
        )

    def __init__(self, api_key: str):
        from google import genai
        from google.genai import types as genai_types

        http_options = genai_types.HttpOptions(
            timeout=300_000,
        )
        self._client = genai.Client(
            api_key=api_key,
            http_options=http_options,
        )
        logger.info("Gemini provider initialized")

    async def close(self):
        """Clean up SDK-internal httpx clients."""
        try:
            if hasattr(self._client, '_api_client'):
                api = self._client._api_client
                if hasattr(api, '_httpx_client') and api._httpx_client:
                    api._httpx_client.close()
                if hasattr(api, '_async_httpx_client') and api._async_httpx_client:
                    await api._async_httpx_client.aclose()
        except Exception as exc:
            logger.debug("Gemini cleanup error (non-fatal): %s", exc)
        logger.info("Gemini provider closed")

    @staticmethod
    def _build_thinking_config_gemini3(tc, genai_types):
        """Build ThinkingConfig for Gemini 3 models.

        Uses thinking_level (high/medium/low/minimal) and include_thoughts.

        Returns None when the config carries no Gemini-3-relevant fields,
        meaning no thinking_config key will be sent in the request.
        """
        if not tc:
            return None
        if tc.thinking_level is None and not tc.include_thoughts:
            return None
        kwargs: dict[str, object] = {}
        if tc.thinking_level is not None:
            kwargs["thinking_level"] = tc.thinking_level
        if tc.include_thoughts:
            kwargs["include_thoughts"] = True
        return genai_types.ThinkingConfig(**kwargs)

    @staticmethod
    def _build_genai_config_primary(
        primary_llm_settings: PrimaryLLMSettings,
        genai_tools: list | None,
        model: str,
    ):
        """Build a genai_types.GenerateContentConfig from PrimaryLLMSettings."""
        from google.genai import types as genai_types

        gen_config_kwargs: dict[str, object] = {
            "temperature": primary_llm_settings.temperature,
            "top_p": primary_llm_settings.top_p_nucleus_sampling,
            "top_k": primary_llm_settings.top_k_filtering,
            "system_instruction": primary_llm_settings.system_instruction,
        }

        if primary_llm_settings.max_output_tokens is not None:
            gen_config_kwargs["max_output_tokens"] = primary_llm_settings.max_output_tokens

        thinking_config = GeminiProvider._build_thinking_config_gemini3(primary_llm_settings.thinking_config, genai_types)
        
        if thinking_config is not None:
            gen_config_kwargs["thinking_config"] = thinking_config

        if genai_tools:
            gen_config_kwargs["tools"] = genai_tools

        if primary_llm_settings.tool_config and primary_llm_settings.tool_config.tool_calling_config:
            fc_cfg = primary_llm_settings.tool_config.tool_calling_config
            fc_kwargs: dict[str, object] = {}
            if fc_cfg.mode is not None:
                fc_kwargs["mode"] = fc_cfg.mode
            if fc_cfg.allowed_tool_names is not None:
                fc_kwargs["allowed_function_names"] = fc_cfg.allowed_tool_names
            gen_config_kwargs["tool_config"] = genai_types.ToolConfig(
                function_calling_config=genai_types.FunctionCallingConfig(**fc_kwargs)
            )

        return genai_types.GenerateContentConfig(**gen_config_kwargs)

    @staticmethod
    def _build_genai_config_assistant(
        assistant_llm_settings: AssistantLLMSettings,
        model: str,
    ):
        """Build a genai_types.GenerateContentConfig from AssistantLLMSettings."""
        from google.genai import types as genai_types

        gen_config_kwargs: dict[str, object] = {
            "temperature": assistant_llm_settings.temperature,
            "top_p": assistant_llm_settings.top_p_nucleus_sampling,
            "top_k": assistant_llm_settings.top_k_filtering,
            "system_instruction": assistant_llm_settings.system_instruction,
        }

        if assistant_llm_settings.max_output_tokens is not None:
            gen_config_kwargs["max_output_tokens"] = assistant_llm_settings.max_output_tokens

        if assistant_llm_settings.response_format is not None:
            gen_config_kwargs["response_mime_type"] = "application/json"
            gen_config_kwargs["response_json_schema"] = assistant_llm_settings.response_format.json_schema.schema

        return genai_types.GenerateContentConfig(**gen_config_kwargs)

    @staticmethod
    def _build_genai_config_lite(
        lite_llm_settings: LiteLLMSettings,
        model: str,
    ):
        """Build a genai_types.GenerateContentConfig from LiteLLMSettings."""
        from google.genai import types as genai_types

        gen_config_kwargs: dict[str, object] = {
            "temperature": lite_llm_settings.temperature,
            "top_p": lite_llm_settings.top_p_nucleus_sampling,
            "top_k": lite_llm_settings.top_k_filtering,
            "system_instruction": lite_llm_settings.system_instruction,
        }

        if lite_llm_settings.max_output_tokens is not None:
            gen_config_kwargs["max_output_tokens"] = lite_llm_settings.max_output_tokens

        if lite_llm_settings.response_format is not None:
            gen_config_kwargs["response_mime_type"] = "application/json"
            gen_config_kwargs["response_json_schema"] = lite_llm_settings.response_format.json_schema.schema

        return genai_types.GenerateContentConfig(**gen_config_kwargs)

    @staticmethod
    def _sdk_chunk_to_stream_from_model_chunks(chunk) -> list[StreamChunkFromModel]:
        """Convert one SDK GenerateContentResponse chunk to canonical StreamFromModelChunks.

        Processes parts in the order the SDK returns them. Per Gemini 3 streaming
        spec, a signature-only chunk (empty text + thought_signature) may arrive
        as the final chunk of a turn — it is captured and forwarded.
        """
        result: list[StreamChunkFromModel] = []

        if not chunk.candidates:
            if chunk.usage_metadata:
                result.append(StreamChunkFromModel(usage_metadata=_usage_from_sdk(chunk.usage_metadata)))
            return result

        candidate = chunk.candidates[0]
        finish_reason = _finish_reason_from_candidate(candidate)

        if candidate.content and candidate.content.parts:
            for sdk_part in candidate.content.parts:
                sig = _sig_from_sdk(getattr(sdk_part, "thought_signature", None))
                thought_flag = bool(getattr(sdk_part, "thought", False))
                fc = getattr(sdk_part, "function_call", None)
                text = getattr(sdk_part, "text", None)

                # Normalize literal '\n\n' strings that Gemini occasionally emits
                # into actual newline characters.
                if text and "\\n" in text:
                    text = text.replace("\\n", "\n")

                if thought_flag and text:
                    result.append(StreamChunkFromModel(text=text, thought=True, thought_signature=sig))
                elif fc:
                    result.append(StreamChunkFromModel(
                        tool_calls=[ToolCall(
                            name=fc.name,
                            args=dict(fc.args) if fc.args else {},
                            id=getattr(fc, "id", None),
                        )],
                        thought_signature=sig,
                    ))
                elif text:
                    result.append(StreamChunkFromModel(text=text, thought_signature=sig))
                elif sig:
                    result.append(StreamChunkFromModel(thought_signature=sig))

        if chunk.usage_metadata:
            result.append(StreamChunkFromModel(
                usage_metadata=_usage_from_sdk(chunk.usage_metadata),
                finish_reason=finish_reason,
            ))
        elif finish_reason:
            result.append(StreamChunkFromModel(finish_reason=finish_reason))

        return result

    def _build_request_kwargs_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenaiRequestKwargs:
        """Build the typed kwargs model passed to the SDK for primary LLM calls."""
        genai_contents = [_content_to_genai(c).flatten_for_llm() for c in contents]
        genai_tools = _tools_to_genai(primary_llm_settings.tools)
        gen_config = self._build_genai_config_primary(primary_llm_settings, genai_tools, model)
        return GenaiRequestKwargs(model=model, contents=genai_contents, config=gen_config)

    def _build_request_kwargs_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenaiRequestKwargs:
        """Build the typed kwargs model passed to the SDK for assistant LLM calls."""
        genai_contents = [_content_to_genai(c).flatten_for_llm() for c in contents]
        gen_config = self._build_genai_config_assistant(assistant_llm_settings, model)
        return GenaiRequestKwargs(model=model, contents=genai_contents, config=gen_config)

    def _build_request_kwargs_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenaiRequestKwargs:
        """Build the typed kwargs model passed to the SDK for lite LLM calls."""
        genai_contents = [_content_to_genai(c).flatten_for_llm() for c in contents]
        gen_config = self._build_genai_config_lite(lite_llm_settings, model)
        return GenaiRequestKwargs(model=model, contents=genai_contents, config=gen_config)

    def _parse_response(self, response) -> GenerateContentResponse:
        """Convert an SDK GenerateContentResponse to canonical form."""
        parts: list[Part] = []
        finish_reason: str | None = None
        if response.candidates:
            candidate = response.candidates[0]
            parts = _parts_from_sdk_candidate(candidate)
            finish_reason = _finish_reason_from_candidate(candidate)

        usage: UsageMetadata | None = None
        if response.usage_metadata:
            usage = _usage_from_sdk(response.usage_metadata)

        grounding_raw: SdkGroundingRawData | None = None
        if response.candidates:
            grounding_raw = _grounding_from_sdk_candidate(response.candidates[0])

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=finish_reason,
            )],
            usage_metadata=usage,
            grounding_raw=grounding_raw,
        )

    def _sync_generate(self, kwargs: GenaiRequestKwargs):
        """Blocking generate_content via the sync SDK (uses httpx, not aiohttp)."""
        return self._client.models.generate_content(**kwargs.flatten_for_llm())

    def _sync_stream(self, kwargs: GenaiRequestKwargs):
        """Blocking generate_content_stream via the sync SDK (uses httpx, not aiohttp)."""
        return self._client.models.generate_content_stream(**kwargs.flatten_for_llm())

    async def _generate_with_retry(self, kwargs: GenaiRequestKwargs) -> GenerateContentResponse:
        """Run a non-streaming generate call with tenacity retry."""
        response = None
        async for attempt in AsyncRetrying(
            retry=retry_if_exception(self._is_retryable),
            stop=stop_after_attempt(self._RETRY_ATTEMPTS),
            wait=wait_exponential(multiplier=1, min=2, max=30),
            reraise=True,
        ):
            with attempt:
                response = await asyncio.to_thread(self._sync_generate, kwargs)
        assert response is not None
        return self._parse_response(response)

    async def _stream_with_retry(
        self, kwargs: GenaiRequestKwargs,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        """Run a streaming generate call with tenacity retry on initial connection."""
        stream_iter = None
        async for attempt in AsyncRetrying(
            retry=retry_if_exception(self._is_retryable),
            stop=stop_after_attempt(self._RETRY_ATTEMPTS),
            wait=wait_exponential(multiplier=1, min=2, max=30),
            reraise=True,
        ):
            with attempt:
                stream_iter = await asyncio.to_thread(self._sync_stream, kwargs)
        assert stream_iter is not None
        while True:
            sdk_chunk = await asyncio.to_thread(next, stream_iter, _STREAM_END)
            if sdk_chunk is _STREAM_END:
                break
            for chunk in self._sdk_chunk_to_stream_from_model_chunks(sdk_chunk):
                yield chunk

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        kwargs = self._build_request_kwargs_primary(model, contents, primary_llm_settings)
        async for chunk in self._stream_with_retry(kwargs):
            yield chunk

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        kwargs = self._build_request_kwargs_primary(model, contents, primary_llm_settings)
        return await self._generate_with_retry(kwargs)

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        kwargs = self._build_request_kwargs_assistant(model, contents, assistant_llm_settings)
        async for chunk in self._stream_with_retry(kwargs):
            yield chunk

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        kwargs = self._build_request_kwargs_assistant(model, contents, assistant_llm_settings)
        return await self._generate_with_retry(kwargs)

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        kwargs = self._build_request_kwargs_lite(model, contents, lite_llm_settings)
        async for chunk in self._stream_with_retry(kwargs):
            yield chunk

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        kwargs = self._build_request_kwargs_lite(model, contents, lite_llm_settings)
        return await self._generate_with_retry(kwargs)
