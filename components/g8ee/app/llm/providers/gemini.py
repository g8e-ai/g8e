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
"""

import asyncio
import logging
from collections.abc import AsyncGenerator

import httpx
from google.genai import types as genai_types
from tenacity import (
    AsyncRetrying,
    retry_if_exception,
    stop_after_attempt,
    wait_exponential,
)

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
    UsageMetadata,
)
from app.models.base import G8eBaseModel, Field
from app.models.model_configs import get_model_config
from app.llm.thinking import translate_for_gemini

from ..provider import LLMProvider
from ._capability import translate_capability_error

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


def _sig_from_sdk(raw) -> ThoughtSignature | None:
    """Normalise an inbound SDK thought_signature to a canonical ThoughtSignature.

    Delegates to ThoughtSignature.from_sdk() which owns the normalisation
    contract. Returns None when raw is None/falsy.
    """
    return ThoughtSignature.from_sdk(raw)


def _content_to_genai(content: Content) -> dict:
    """Convert canonical Content to a google.genai-compatible dict.

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

    return {"role": role, "parts": [p.model_dump(by_alias=True, exclude_none=True) for p in parts]}


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
        super().__init__()
        from google import genai

        http_options = genai_types.HttpOptions(
            timeout=300_000,
        )
        self._client = genai.Client(
            api_key=api_key,
            http_options=http_options,
        )
        logger.info("Gemini provider initialized")

    async def _close_resources(self):
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
    def _build_thinking_config_gemini3(tc, model: str, genai_types):
        """Build google.genai ThinkingConfig from canonical ThinkingConfig.

        Delegates the ThinkingLevel -> wire value mapping to
        translate_for_gemini so it stays consistent across providers.
        Returns a tuple ``(sdk_thinking_config, translation)`` so the caller
        can log the clamped (post-translation) values rather than the raw
        requested level — which would be misleading when the model's
        supported_thinking_levels force a clamp (e.g. HIGH -> MEDIUM).
        ``sdk_thinking_config`` is None when thinking is OFF and no thoughts
        are requested, which signals the caller to omit the key entirely.
        """
        if not tc:
            return None, None
        cfg = get_model_config(model)
        translation = translate_for_gemini(
            tc.thinking_level,
            cfg,
            include_thoughts=tc.include_thoughts,
        )
        if not translation.enabled and not translation.include_thoughts:
            return None, translation
        return genai_types.ThinkingConfig(
            thinking_level=translation.thinking_level,
            include_thoughts=translation.include_thoughts,
        ), translation

    @staticmethod
    def _build_genai_config(
        settings: PrimaryLLMSettings | AssistantLLMSettings | LiteLLMSettings,
        genai_tools: list | None,
        model: str,
    ):
        """Build a genai_types.GenerateContentConfig from LLM settings."""
        if isinstance(settings, PrimaryLLMSettings):
            thinking_config, thinking_translation = GeminiProvider._build_thinking_config_gemini3(
                settings.thinking_config, model, genai_types
            )

            tool_config = None
            if settings.tool_config and settings.tool_config.tool_calling_config:
                fc_cfg = settings.tool_config.tool_calling_config
                tool_config = genai_types.ToolConfig(
                    function_calling_config=genai_types.FunctionCallingConfig(
                        mode=fc_cfg.mode,
                        allowed_function_names=fc_cfg.allowed_tool_names,
                    )
                )

            # Log the clamped (post-translation) thinking level — which is what
            # actually goes out on the wire — rather than the raw requested
            # value, so debug traces match what Gemini sees.
            logged_thinking_level = (
                thinking_translation.thinking_level if thinking_translation and thinking_translation.enabled else None
            )
            logged_include_thoughts = bool(thinking_translation and thinking_translation.include_thoughts)
            log_parts = [
                f"[GEMINI] Building config: model={model}",
                f"max_output_tokens={settings.max_output_tokens}",
            ]
            if settings.top_p_nucleus_sampling is not None:
                log_parts.append(f"top_p={settings.top_p_nucleus_sampling}")
            if settings.top_k_filtering is not None:
                log_parts.append(f"top_k={settings.top_k_filtering}")
            log_parts.extend([
                f"system_instructions_len={len(settings.system_instructions)}",
                f"tools_count={len(genai_tools) if genai_tools else 0}",
                f"thinking_level={logged_thinking_level}",
                f"include_thoughts={logged_include_thoughts}",
                f"tool_calling_mode={fc_cfg.mode if settings.tool_config and settings.tool_config.tool_calling_config else None}",
                f"allowed_tools={len(fc_cfg.allowed_tool_names) if fc_cfg and fc_cfg.allowed_tool_names else 0}",
            ])
            logger.debug(" ".join(log_parts))
            
            config_kwargs = {
                "max_output_tokens": settings.max_output_tokens,
                "system_instruction": settings.system_instructions,
                "thinking_config": thinking_config,
                "tools": genai_tools,
                "tool_config": tool_config,
            }
            if settings.top_p_nucleus_sampling is not None:
                config_kwargs["top_p"] = settings.top_p_nucleus_sampling
            if settings.top_k_filtering is not None:
                config_kwargs["top_k"] = settings.top_k_filtering
            
            return genai_types.GenerateContentConfig(**config_kwargs)
        else:
            log_parts = [
                f"[GEMINI] Building config: model={model}",
                f"max_output_tokens={settings.max_output_tokens}",
            ]
            if settings.top_p_nucleus_sampling is not None:
                log_parts.append(f"top_p={settings.top_p_nucleus_sampling}")
            if settings.top_k_filtering is not None:
                log_parts.append(f"top_k={settings.top_k_filtering}")
            log_parts.append(f"response_format={settings.response_format is not None}")
            logger.info(" ".join(log_parts))
            
            config_kwargs = {
                "max_output_tokens": settings.max_output_tokens,
                "response_mime_type": "application/json" if settings.response_format else None,
                "response_json_schema": settings.response_format.flatten_for_gemini() if settings.response_format else None,
            }
            if settings.top_p_nucleus_sampling is not None:
                config_kwargs["top_p"] = settings.top_p_nucleus_sampling
            if settings.top_k_filtering is not None:
                config_kwargs["top_k"] = settings.top_k_filtering
            
            return genai_types.GenerateContentConfig(**config_kwargs)

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

        logger.debug(
            "[GEMINI] Parsed response: parts=%d finish_reason=%s has_usage=%s has_grounding=%s",
            len(parts),
            finish_reason,
            usage is not None,
            grounding_raw is not None,
        )

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=finish_reason,
            )],
            usage_metadata=usage,
            grounding_raw=grounding_raw,
        )

    def _sync_generate(self, model: str, contents: list[dict], config):
        """Blocking generate_content via the sync SDK (uses httpx, not aiohttp)."""
        return self._client.models.generate_content(model=model, contents=contents, config=config)

    def _sync_stream(self, model: str, contents: list[dict], config):
        """Blocking generate_content_stream via the sync SDK (uses httpx, not aiohttp)."""
        return self._client.models.generate_content_stream(model=model, contents=contents, config=config)

    async def _generate_with_retry(self, model: str, contents: list[dict], config) -> GenerateContentResponse:
        """Run a non-streaming generate call with tenacity retry."""
        response = None
        async for attempt in AsyncRetrying(
            retry=retry_if_exception(self._is_retryable),
            stop=stop_after_attempt(self._RETRY_ATTEMPTS),
            wait=wait_exponential(multiplier=1, min=2, max=30),
            reraise=True,
        ):
            with attempt:
                response = await asyncio.to_thread(self._sync_generate, model, contents, config)
        assert response is not None
        return self._parse_response(response)

    async def _stream_with_retry(
        self, model: str, contents: list[dict], config,
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
                stream_iter = await asyncio.to_thread(self._sync_stream, model, contents, config)
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
        logger.info(
            "[GEMINI] generate_content_stream_primary: model=%s contents=%d "
            "max_output_tokens=%d top_k=%s top_p=%s "
            "system_instructions_len=%d tools_count=%d",
            model,
            len(contents),
            primary_llm_settings.max_output_tokens,
            primary_llm_settings.top_k_filtering if primary_llm_settings.top_k_filtering is not None else "None",
            primary_llm_settings.top_p_nucleus_sampling if primary_llm_settings.top_p_nucleus_sampling is not None else "None",
            len(primary_llm_settings.system_instructions),
            len(primary_llm_settings.tools) if primary_llm_settings.tools else 0,
        )

        genai_contents = [_content_to_genai(c) for c in contents]
        genai_tools = []
        if primary_llm_settings.tools:
            for tool_group in primary_llm_settings.tools:
                genai_tools.extend(tool_group.to_genai_tools())
        genai_tools = genai_tools or None
        gen_config = self._build_genai_config(primary_llm_settings, genai_tools, model)
        try:
            async for chunk in self._stream_with_retry(model, genai_contents, gen_config):
                yield chunk
        except Exception as e:
            translate_capability_error(
                e,
                service_name="gemini",
                model=model,
                thinking_requested=bool(
                    primary_llm_settings.thinking_config
                    and primary_llm_settings.thinking_config.enabled
                ),
                tools_requested=bool(primary_llm_settings.tools),
            )
            raise

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        genai_contents = [_content_to_genai(c) for c in contents]
        genai_tools = []
        if primary_llm_settings.tools:
            for tool_group in primary_llm_settings.tools:
                genai_tools.extend(tool_group.to_genai_tools())
        genai_tools = genai_tools or None
        gen_config = self._build_genai_config(primary_llm_settings, genai_tools, model)
        try:
            return await self._generate_with_retry(model, genai_contents, gen_config)
        except Exception as e:
            translate_capability_error(
                e,
                service_name="gemini",
                model=model,
                thinking_requested=bool(
                    primary_llm_settings.thinking_config
                    and primary_llm_settings.thinking_config.enabled
                ),
                tools_requested=bool(primary_llm_settings.tools),
            )
            raise

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        logger.info(
            "[GEMINI] generate_content_stream_assistant: model=%s contents=%d "
            "max_output_tokens=%d top_k=%s top_p=%s "
            "response_format=%s",
            model,
            len(contents),
            assistant_llm_settings.max_output_tokens,
            assistant_llm_settings.top_k_filtering if assistant_llm_settings.top_k_filtering is not None else "None",
            assistant_llm_settings.top_p_nucleus_sampling if assistant_llm_settings.top_p_nucleus_sampling is not None else "None",
            assistant_llm_settings.response_format is not None,
        )

        genai_contents = [_content_to_genai(c) for c in contents]
        gen_config = self._build_genai_config(assistant_llm_settings, None, model)
        async for chunk in self._stream_with_retry(model, genai_contents, gen_config):
            yield chunk

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        genai_contents = [_content_to_genai(c) for c in contents]
        gen_config = self._build_genai_config(assistant_llm_settings, None, model)
        return await self._generate_with_retry(model, genai_contents, gen_config)

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        logger.info(
            "[GEMINI] generate_content_stream_lite: model=%s contents=%d "
            "max_output_tokens=%d top_k=%s top_p=%s "
            "response_format=%s",
            model,
            len(contents),
            lite_llm_settings.max_output_tokens,
            lite_llm_settings.top_k_filtering if lite_llm_settings.top_k_filtering is not None else "None",
            lite_llm_settings.top_p_nucleus_sampling if lite_llm_settings.top_p_nucleus_sampling is not None else "None",
            lite_llm_settings.response_format is not None,
        )

        genai_contents = [_content_to_genai(c) for c in contents]
        gen_config = self._build_genai_config(lite_llm_settings, None, model)
        async for chunk in self._stream_with_retry(model, genai_contents, gen_config):
            yield chunk

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        genai_contents = [_content_to_genai(c) for c in contents]
        gen_config = self._build_genai_config(lite_llm_settings, None, model)
        return await self._generate_with_retry(model, genai_contents, gen_config)
