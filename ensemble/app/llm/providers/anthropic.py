# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json
import logging
from collections.abc import AsyncGenerator

import anthropic
from anthropic.types import (
    ContentBlockDeltaEvent,
    ContentBlockStartEvent,
    ContentBlockStopEvent,
    MessageDeltaEvent,
    MessageStartEvent,
)

from app.errors import ConfigurationError, ValidationError
from app.llm.llm_types import (
    AssistantLLMSettings,
    Candidate,
    Content,
    GenerateContentResponse,
    LiteLLMSettings,
    Part,
    PrimaryLLMSettings,
    StreamChunkFromModel,
    ThinkingConfig,
    ThoughtSignature,
    ToolCall,
    ToolGroup,
    UsageMetadata,
)
from app.llm.provider import LLMProvider
from app.llm.thinking import translate_for_anthropic
from app.models.base import G8eBaseModel
from app.models.model_configs import get_model_config

from ._capability import translate_capability_error

logger = logging.getLogger(__name__)


# =============================================================================
# LLM-boundary models for the Anthropic SDK
#
# These G8eBaseModel subclasses represent the known shapes of dicts passed to
# the Anthropic SDK. flatten_for_llm() is called at the LLM boundary to produce
# the plain dicts the SDK expects.
# =============================================================================


class AnthropicMessageRequest(G8eBaseModel):
    model: str
    messages: list[dict]
    max_tokens: int
    system: str | None = None
    tools: list[dict] | None = None
    top_k: int | None = None
    thinking: dict | None = None


def _contents_to_anthropic(contents: list[Content]) -> list[dict]:
    """Convert canonical Content objects to Anthropic message format.

    Anthropic API constraints enforced here:
    - Messages must strictly alternate between 'user' and 'assistant' roles.
      Consecutive same-role Content objects are merged into a single message.
    - Every tool_result block must reference a tool_use_id from the immediately
      preceding assistant message.
    - Empty text blocks are dropped (Anthropic rejects zero-length text).
    - tool_use blocks go in assistant messages; tool_result blocks go in user messages.
    """
    messages: list[dict] = []

    for content in contents:
        role = "assistant" if content.role == "model" else "user"
        blocks: list[dict] = []

        for part in content.parts:
            if part.text is not None and not part.thought:
                if part.text:
                    blocks.append({"type": "text", "text": part.text})
            elif part.thought and part.text is not None:
                blocks.append(
                    {
                        "type": "thinking",
                        "thinking": part.text,
                        "signature": str(part.thought_signature) if part.thought_signature else "",
                    }
                )
            elif part.tool_call:
                tool_use_id = part.tool_call.id
                if not tool_use_id:
                    logger.warning(
                        "tool_use block missing ID for tool '%s', using fallback",
                        part.tool_call.name,
                    )
                    tool_use_id = f"toolc_{part.tool_call.name}"
                blocks.append(
                    {
                        "type": "tool_use",
                        "id": tool_use_id,
                        "name": part.tool_call.name,
                        "input": part.tool_call.args,
                    }
                )
            elif part.tool_response:
                tool_result_id = part.tool_response.id
                if not tool_result_id:
                    logger.warning(
                        "tool_result block missing ID for tool '%s', using fallback",
                        part.tool_response.name,
                    )
                    tool_result_id = f"toolc_{part.tool_response.name}"
                blocks.append(
                    {
                        "type": "tool_result",
                        "tool_use_id": tool_result_id,
                        "content": json.dumps(part.tool_response.response),
                    }
                )

        if not blocks:
            continue

        if messages and messages[-1]["role"] == role:
            messages[-1]["content"].extend(blocks)
        else:
            messages.append({"role": role, "content": blocks})

    return messages


def _tools_to_anthropic(tools: list[ToolGroup] | None) -> list[dict] | None:
    if not tools:
        return None

    anthropic_tools = []
    anthropic_tools.extend(
        {
            "name": decl.name,
            "description": decl.description,
            "input_schema": decl.parameters.to_json_schema(),
        }
        for tool in tools
        for decl in tool.tools
    )

    return anthropic_tools if anthropic_tools else None


def _parse_response_blocks(blocks: list) -> list[Part]:
    parts = []
    for block in blocks:
        block_type = getattr(block, "type", None)
        if block_type == "text":
            parts.append(Part(text=block.text))
        elif block_type == "thinking":
            parts.append(
                Part(
                    text=block.thinking,
                    thought=True,
                    thought_signature=getattr(block, "signature", None),
                )
            )
        elif block_type == "tool_use":
            args = block.input
            if not isinstance(args, dict):
                raise ValidationError("Provider tool arguments must be a JSON object")
            parts.append(
                Part(
                    tool_call=ToolCall(
                        name=block.name,
                        args=args,
                        id=block.id,
                    )
                )
            )
    return parts


def _cache_token_count(response_usage) -> int | None:
    if not hasattr(response_usage, "cache_read_input_tokens"):
        return None
    cache_tokens = getattr(response_usage, "cache_read_input_tokens", None)
    return (
        cache_tokens
        if isinstance(cache_tokens, int) and not isinstance(cache_tokens, bool)
        else None
    )


def _build_usage(response_usage) -> UsageMetadata:
    """Build UsageMetadata from an Anthropic response usage object."""
    if not response_usage:
        return UsageMetadata()
    input_tokens = getattr(response_usage, "input_tokens", 0) or 0
    output_tokens = getattr(response_usage, "output_tokens", 0) or 0
    return UsageMetadata(
        prompt_token_count=input_tokens,
        candidates_token_count=output_tokens,
        total_token_count=input_tokens + output_tokens,
        cache_token_count=_cache_token_count(response_usage),
        usage_reported=True,
    )


class AnthropicProvider(LLMProvider):
    def __init__(self, endpoint: str | None, api_key: str):
        super().__init__()
        kwargs: dict = {"max_retries": 0}
        if api_key:
            kwargs["api_key"] = api_key
        if endpoint:
            kwargs["base_url"] = endpoint
        self._client = anthropic.AsyncAnthropic(**kwargs)
        logger.info("Anthropic provider initialized")

    async def _close_resources(self):
        """Clean up provider resources."""
        if hasattr(self._client, "close"):
            await self._client.close()
        logger.info("Anthropic provider closed")

    @staticmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        """Validate Anthropic provider configuration.

        Anthropic requires both an API key and an endpoint URL.

        Args:
            api_key: The API key for the provider
            endpoint: The endpoint URL for the provider

        Returns:
            List of validation error messages. Empty if configuration is valid.
        """
        errors = []
        if not api_key:
            errors.append("Provider 'anthropic' requires an API key.")
        if not endpoint:
            errors.append("Provider 'anthropic' requires an endpoint URL.")
        return errors

    def _build_kwargs(
        self,
        *,
        model: str,
        messages: list[dict],
        max_tokens: int | None,
        top_k: int | None,
        system_instructions: str | None,
        anthropic_tools: list[dict] | None = None,
        thinking_config: ThinkingConfig | None = None,
    ) -> AnthropicMessageRequest:
        """Build Anthropic API request with proper parameter constraints.

        Anthropic constraints enforced here:
        - Extended thinking requires no other sampling params
        """
        model_config = get_model_config(model)

        # The Messages API requires max_tokens. Use the caller's value, else the
        # model's documented ceiling; with neither there is nothing honest to send.
        effective_max_tokens = (
            max_tokens
            if max_tokens is not None
            else (model_config.max_output_tokens if model_config else None)
        )
        if effective_max_tokens is None:
            raise ConfigurationError(
                f"Anthropic requires max_tokens and model '{model}' declares no output ceiling",
                details={"model": model},
            )

        # Translate the canonical ThinkingConfig to Anthropic's extended-thinking
        # wire shape. The translator enforces per-level budgets (from the model
        # config's thinking_budgets map or the default table) and signals whether
        # the call must switch into thinking mode (temp=1, no top_k).
        if thinking_config is not None:
            translation = translate_for_anthropic(
                thinking_config.thinking_level,
                model_config,
            )
        else:
            translation = None

        thinking_enabled = translation is not None and translation.enabled

        if thinking_enabled and translation is not None:
            # Anthropic's contract: max_tokens is the TOTAL output budget and
            # must strictly exceed thinking.budget_tokens so the model has
            # headroom for a visible response. When the caller-provided
            # max_tokens is too small for the requested thinking budget, uplift
            # it rather than silently truncating the budget - clamping the
            # budget down to a tiny value (as the previous min() guard did)
            # leaves the model with effectively zero tokens for a real reply.
            # An explicit reserve is cheaper than a surprising empty response.
            effective_max_tokens = max(
                effective_max_tokens,
                translation.budget_tokens + model_config.thinking_output_reserve,
            )
            thinking_config_dict = {
                "type": "enabled",
                "budget_tokens": translation.budget_tokens,
            }
            effective_top_k = None
        else:
            thinking_config_dict = None
            effective_top_k = top_k

        request = AnthropicMessageRequest(
            model=model,
            messages=messages,
            max_tokens=effective_max_tokens,
            system=system_instructions if system_instructions else None,
            tools=anthropic_tools,
            top_k=effective_top_k,
            thinking=thinking_config_dict,
        )

        logger.info(
            "[ANTHROPIC] Building API request: model=%s max_tokens=%d "
            "top_k=%s system_instructions_len=%d tools_count=%d thinking_enabled=%s",
            model,
            request.max_tokens,
            request.top_k,
            len(system_instructions or ""),
            len(anthropic_tools) if anthropic_tools else 0,
            thinking_enabled,
        )

        return request

    def _build_response(self, response) -> GenerateContentResponse:
        """Parse a non-streaming Anthropic response into canonical form."""
        return GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(role="model", parts=_parse_response_blocks(response.content)),
                    finish_reason=response.stop_reason,
                )
            ],
            usage_metadata=_build_usage(response.usage),
        )

    async def _stream_text_only(self, kwargs: dict) -> AsyncGenerator[StreamChunkFromModel]:
        """Shared streaming handler for assistant/lite calls (text output only)."""
        finish_reason_received = False
        stream_exhausted = False
        self._record_model_boundary(kwargs)
        async with self._client.messages.stream(**kwargs) as stream:
            try:
                for event in await self._receive_stream(stream):
                    if isinstance(event, ContentBlockDeltaEvent):
                        delta = event.delta
                        if delta.type == "text_delta":
                            yield StreamChunkFromModel(text=delta.text or "")

                    elif isinstance(event, MessageDeltaEvent):
                        stop_reason = event.delta.stop_reason
                        usage = event.usage
                        um = None
                        if usage:
                            um = UsageMetadata(
                                candidates_token_count=getattr(usage, "output_tokens", 0) or 0,
                                usage_reported=True,
                            )
                        if stop_reason:
                            finish_reason_received = True
                            yield StreamChunkFromModel(
                                finish_reason=stop_reason,
                                usage_metadata=um or UsageMetadata(),
                            )

                    elif isinstance(event, MessageStartEvent):
                        usage = event.message.usage
                        yield StreamChunkFromModel(
                            usage_metadata=UsageMetadata(
                                prompt_token_count=usage.input_tokens or 0,
                                cache_token_count=_cache_token_count(usage),
                                usage_reported=True,
                            )
                        )
                stream_exhausted = True
            except Exception as e:
                logger.exception("[ANTHROPIC] Exception during streaming: %s", e)
                raise

        # Fallback: if stream ended without message_delta with stop_reason, yield completion
        # Only trigger fallback if stream was fully exhausted (no early termination)
        if stream_exhausted and not finish_reason_received:
            logger.warning(
                "[ANTHROPIC] Stream ended without message_delta with stop_reason, using fallback completion"
            )
            yield StreamChunkFromModel(finish_reason="stop")

    async def _handle_primary_stream_event(self, event, state: dict):
        if isinstance(event, ContentBlockStartEvent):
            self._handle_primary_block_start(event, state)
        elif isinstance(event, ContentBlockDeltaEvent):
            async for chunk in self._handle_primary_block_delta(event, state):
                yield chunk
        elif isinstance(event, ContentBlockStopEvent):
            async for chunk in self._handle_primary_block_stop(event, state):
                yield chunk
        elif isinstance(event, MessageDeltaEvent):
            async for chunk in self._handle_primary_message_delta(event, state):
                yield chunk
        elif isinstance(event, MessageStartEvent):
            usage = event.message.usage
            yield StreamChunkFromModel(
                usage_metadata=UsageMetadata(
                    prompt_token_count=usage.input_tokens or 0,
                    cache_token_count=_cache_token_count(usage),
                    usage_reported=True,
                )
            )

    def _handle_primary_block_start(self, event, state: dict) -> None:
        idx, block = event.index, event.content_block
        state["block_types"][idx] = block.type
        if block.type == "tool_use":
            state["tool_name"][idx] = block.name
            state["tool_id"][idx] = block.id
            state["tool_input"][idx] = ""
        elif block.type == "thinking":
            signature = getattr(block, "signature", None)
            if signature:
                state["thinking_sig"][idx] = signature

    async def _handle_primary_block_delta(self, event, state: dict):
        idx, delta = event.index, event.delta
        if delta.type == "text_delta":
            yield StreamChunkFromModel(text=delta.text or "")
        elif delta.type == "thinking_delta":
            yield StreamChunkFromModel(text=delta.thinking or "", thought=True)
        elif delta.type == "signature_delta":
            signature = getattr(delta, "signature", None)
            if signature:
                state["thinking_sig"][idx] = signature
        elif delta.type == "input_json_delta":
            state["tool_input"][idx] = state["tool_input"].get(idx, "") + (delta.partial_json or "")

    async def _handle_primary_block_stop(self, event, state: dict):
        idx = event.index
        if state["block_types"].get(idx) == "thinking":
            signature = state["thinking_sig"].pop(idx, None)
            if signature:
                yield StreamChunkFromModel(
                    thought=True, thought_signature=ThoughtSignature.from_sdk(signature)
                )
        elif state["block_types"].get(idx) == "tool_use":
            raw_input = state["tool_input"].get(idx, "{}")
            try:
                args = json.loads(raw_input)
            except json.JSONDecodeError as exc:
                raise ValidationError("Provider returned invalid tool arguments JSON") from exc
            if not isinstance(args, dict):
                raise ValidationError("Provider tool arguments must be a JSON object")
            yield StreamChunkFromModel(
                tool_calls=[
                    ToolCall(
                        name=state["tool_name"].get(idx, ""),
                        args=args,
                        id=state["tool_id"].get(idx),
                    )
                ]
            )

    async def _handle_primary_message_delta(self, event, state: dict):
        stop_reason, usage = event.delta.stop_reason, event.usage
        metadata = (
            UsageMetadata(
                candidates_token_count=getattr(usage, "output_tokens", 0) or 0,
                usage_reported=True,
            )
            if usage
            else UsageMetadata()
        )
        if stop_reason:
            state["finish_reason_received"] = True
            yield StreamChunkFromModel(finish_reason=stop_reason, usage_metadata=metadata)

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        logger.info(
            "[ANTHROPIC] generate_content_stream_primary: model=%s contents=%d "
            "max_output_tokens=%s top_k=%s top_p=%s "
            "system_instructions_len=%d tools_count=%d",
            model,
            len(contents),
            primary_llm_settings.max_output_tokens,
            primary_llm_settings.top_k_filtering
            if primary_llm_settings.top_k_filtering is not None
            else "None",
            primary_llm_settings.top_p_nucleus_sampling
            if primary_llm_settings.top_p_nucleus_sampling is not None
            else "None",
            len(primary_llm_settings.system_instructions or ""),
            len(primary_llm_settings.tools) if primary_llm_settings.tools else 0,
        )

        request = self._build_kwargs(
            model=model,
            messages=_contents_to_anthropic(contents),
            max_tokens=primary_llm_settings.max_output_tokens,
            top_k=primary_llm_settings.top_k_filtering,
            system_instructions=primary_llm_settings.system_instructions,
            anthropic_tools=_tools_to_anthropic(primary_llm_settings.tools),
            thinking_config=primary_llm_settings.thinking_config,
        )

        state = {
            "tool_name": {},
            "tool_id": {},
            "tool_input": {},
            "block_types": {},
            "thinking_sig": {},
            "finish_reason_received": False,
        }
        stream_exhausted = False

        payload = request.model_dump(mode="json", exclude_none=True)
        self._record_model_boundary(payload)
        async with self._client.messages.stream(**payload) as stream:
            try:
                for event in await self._receive_stream(stream):
                    async for chunk in self._handle_primary_stream_event(event, state):
                        yield chunk
                stream_exhausted = True
            except Exception as e:
                logger.exception("[ANTHROPIC] Exception during primary streaming: %s", e)
                translate_capability_error(
                    e,
                    service_name="anthropic",
                    model=model,
                    thinking_requested=bool(
                        primary_llm_settings.thinking_config
                        and primary_llm_settings.thinking_config.enabled
                    ),
                    tools_requested=bool(primary_llm_settings.tools),
                )
                raise

        # Fallback: if stream ended without message_delta with stop_reason, yield completion
        # Only trigger fallback if stream was fully exhausted (no early termination)
        if stream_exhausted and not state["finish_reason_received"]:
            logger.warning(
                "[ANTHROPIC] Primary stream ended without message_delta with stop_reason, using fallback completion"
            )
            yield StreamChunkFromModel(finish_reason="stop")

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        request = self._build_kwargs(
            model=model,
            messages=_contents_to_anthropic(contents),
            max_tokens=primary_llm_settings.max_output_tokens,
            top_k=primary_llm_settings.top_k_filtering,
            system_instructions=primary_llm_settings.system_instructions,
            anthropic_tools=_tools_to_anthropic(primary_llm_settings.tools),
            thinking_config=primary_llm_settings.thinking_config,
        )

        payload = request.model_dump(mode="json", exclude_none=True)
        try:
            self._record_model_boundary(payload)
            response = await self._client.messages.create(**payload)
            self._record_response(response, complete=True)
        except Exception as e:
            translate_capability_error(
                e,
                service_name="anthropic",
                model=model,
                thinking_requested=bool(
                    primary_llm_settings.thinking_config
                    and primary_llm_settings.thinking_config.enabled
                ),
                tools_requested=bool(primary_llm_settings.tools),
            )
            raise
        return self._build_response(response)

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        logger.info(
            "[ANTHROPIC] generate_content_stream_assistant: model=%s contents=%d "
            "max_output_tokens=%s top_k=%s top_p=%s "
            "system_instructions_len=%d response_format=%s",
            model,
            len(contents),
            assistant_llm_settings.max_output_tokens,
            assistant_llm_settings.top_k_filtering
            if assistant_llm_settings.top_k_filtering is not None
            else "None",
            assistant_llm_settings.top_p_nucleus_sampling
            if assistant_llm_settings.top_p_nucleus_sampling is not None
            else "None",
            len(assistant_llm_settings.system_instructions or ""),
            assistant_llm_settings.response_format is not None,
        )

        request = self._build_kwargs(
            model=model,
            messages=_contents_to_anthropic(contents),
            max_tokens=assistant_llm_settings.max_output_tokens,
            top_k=assistant_llm_settings.top_k_filtering,
            system_instructions=assistant_llm_settings.system_instructions,
            anthropic_tools=None,
            thinking_config=None,
        )

        async for chunk in self._stream_text_only(
            request.model_dump(mode="json", exclude_none=True)
        ):
            yield chunk

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        logger.info(
            "[ANTHROPIC] generate_content_assistant: model=%s contents=%d "
            "max_output_tokens=%s top_k=%s top_p=%s "
            "system_instructions_len=%d response_format=%s",
            model,
            len(contents),
            assistant_llm_settings.max_output_tokens,
            assistant_llm_settings.top_k_filtering
            if assistant_llm_settings.top_k_filtering is not None
            else "None",
            assistant_llm_settings.top_p_nucleus_sampling
            if assistant_llm_settings.top_p_nucleus_sampling is not None
            else "None",
            len(assistant_llm_settings.system_instructions or ""),
            assistant_llm_settings.response_format is not None,
        )

        request = self._build_kwargs(
            model=model,
            messages=_contents_to_anthropic(contents),
            max_tokens=assistant_llm_settings.max_output_tokens,
            top_k=assistant_llm_settings.top_k_filtering,
            system_instructions=assistant_llm_settings.system_instructions,
            anthropic_tools=None,
            thinking_config=None,
        )

        payload = request.model_dump(mode="json", exclude_none=True)
        self._record_model_boundary(payload)
        response = await self._client.messages.create(**payload)
        self._record_response(response, complete=True)
        return self._build_response(response)

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        logger.info(
            "[ANTHROPIC] generate_content_stream_lite: model=%s contents=%d "
            "max_output_tokens=%s top_k=%s top_p=%s "
            "system_instructions_len=%d response_format=%s",
            model,
            len(contents),
            lite_llm_settings.max_output_tokens,
            lite_llm_settings.top_k_filtering
            if lite_llm_settings.top_k_filtering is not None
            else "None",
            lite_llm_settings.top_p_nucleus_sampling
            if lite_llm_settings.top_p_nucleus_sampling is not None
            else "None",
            len(lite_llm_settings.system_instructions or ""),
            lite_llm_settings.response_format is not None,
        )

        request = self._build_kwargs(
            model=model,
            messages=_contents_to_anthropic(contents),
            max_tokens=lite_llm_settings.max_output_tokens,
            top_k=lite_llm_settings.top_k_filtering,
            system_instructions=lite_llm_settings.system_instructions,
            anthropic_tools=None,
            thinking_config=None,
        )

        async for chunk in self._stream_text_only(
            request.model_dump(mode="json", exclude_none=True)
        ):
            yield chunk

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        logger.info(
            "[ANTHROPIC] generate_content_lite: model=%s contents=%d "
            "max_output_tokens=%s top_k=%s top_p=%s "
            "system_instructions_len=%d response_format=%s",
            model,
            len(contents),
            lite_llm_settings.max_output_tokens,
            lite_llm_settings.top_k_filtering
            if lite_llm_settings.top_k_filtering is not None
            else "None",
            lite_llm_settings.top_p_nucleus_sampling
            if lite_llm_settings.top_p_nucleus_sampling is not None
            else "None",
            len(lite_llm_settings.system_instructions or ""),
            lite_llm_settings.response_format is not None,
        )

        request = self._build_kwargs(
            model=model,
            messages=_contents_to_anthropic(contents),
            max_tokens=lite_llm_settings.max_output_tokens,
            top_k=lite_llm_settings.top_k_filtering,
            system_instructions=lite_llm_settings.system_instructions,
            anthropic_tools=None,
            thinking_config=None,
        )

        payload = request.model_dump(mode="json", exclude_none=True)
        self._record_model_boundary(payload)
        response = await self._client.messages.create(**payload)
        self._record_response(response, complete=True)
        return self._build_response(response)
