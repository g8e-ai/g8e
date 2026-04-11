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

from __future__ import annotations

import json
import logging
from collections.abc import AsyncGenerator

import anthropic
import httpx

from app.llm.llm_types import (
    AssistantLLMSettings,
    Candidate,
    Content,
    LiteLLMSettings,
    PrimaryLLMSettings,
    ToolCall,
    GenerateContentResponse,
    Part,
    StreamChunkFromModel,
    ToolGroup,
    UsageMetadata,
)

from ..provider import LLMProvider
from ..utils import is_internal_endpoint, schema_to_dict

logger = logging.getLogger(__name__)


def _contents_to_anthropic(contents: list[Content]) -> list[dict]:
    messages = []

    for content in contents:
        role = "assistant" if content.role == "model" else "user"
        blocks = []

        for part in content.parts:
            if part.text is not None and not part.thought:
                blocks.append({"type": "text", "text": part.text})
            elif part.thought and part.text is not None:
                blocks.append({
                    "type": "thinking",
                    "thinking": part.text,
                    "signature": part.thought_signature or "",
                })
            elif part.tool_call:
                blocks.append({
                    "type": "tool_use",
                    "id": part.tool_call.id or f"toolc_{part.tool_call.name}",
                    "name": part.tool_call.name,
                    "input": part.tool_call.args,
                })
            elif part.tool_response:
                blocks.append({
                    "type": "tool_result",
                    "tool_use_id": part.tool_response.id or f"toolc_{part.tool_response.name}",
                    "content": json.dumps(part.tool_response.response),
                })

        if blocks:
            messages.append({"role": role, "content": blocks})

    return messages


def _tools_to_anthropic(tools: list[ToolGroup] | None) -> list[dict] | None:
    if not tools:
        return None

    anthropic_tools = []
    for tool in tools:
        for decl in tool.tools:
            input_schema = schema_to_dict(decl.parameters) if decl.parameters else {
                "type": "object",
                "properties": {},
            }
            anthropic_tools.append({
                "name": decl.name,
                "description": decl.description,
                "input_schema": input_schema,
            })

    return anthropic_tools if anthropic_tools else None


def _parse_response_blocks(blocks: list) -> list[Part]:
    parts = []
    for block in blocks:
        block_type = getattr(block, "type", None)
        if block_type == "text":
            parts.append(Part(text=block.text))
        elif block_type == "thinking":
            parts.append(Part(
                text=block.thinking,
                thought=True,
                thought_signature=getattr(block, "signature", None),
            ))
        elif block_type == "tool_use":
            args = block.input if isinstance(block.input, dict) else {}
            parts.append(Part(tool_call=ToolCall(
                name=block.name,
                args=args,
                id=block.id,
            )))
    return parts


class AnthropicProvider(LLMProvider):
    _TIMEOUT = httpx.Timeout(connect=10.0, read=300.0, write=30.0, pool=5.0)
    _LIMITS = httpx.Limits(
        max_connections=20,
        max_keepalive_connections=10,
        keepalive_expiry=30.0,
    )

    @staticmethod
    def _is_internal_endpoint(endpoint: str | None) -> bool:
        return is_internal_endpoint(endpoint)

    def __init__(self, endpoint: str | None, api_key: str, ca_cert_path: str | None = None):
        verify: str | bool
        if self._is_internal_endpoint(endpoint):
            if endpoint and endpoint.startswith("http://"):
                verify = False
            elif ca_cert_path:
                verify = ca_cert_path
            else:
                verify = True
        else:
            verify = True

        http_client = httpx.AsyncClient(
            timeout=self._TIMEOUT,
            limits=self._LIMITS,
            verify=verify,
        )
        kwargs: dict = {"http_client": http_client, "max_retries": 0}
        if api_key:
            kwargs["api_key"] = api_key
        if endpoint:
            kwargs["base_url"] = endpoint
        self._client = anthropic.AsyncAnthropic(**kwargs)
        self._http_client = http_client
        logger.info("Anthropic provider initialized")

    async def close(self):
        """Clean up the httpx client to prevent resource leaks."""
        if self._http_client:
            await self._http_client.aclose()
            logger.info("Anthropic provider closed")

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_anthropic(contents)
        anthropic_tools = _tools_to_anthropic(primary_llm_settings.tools)

        kwargs = {
            "model": model,
            "messages": messages,
            "max_tokens": primary_llm_settings.max_output_tokens,
        }
        if primary_llm_settings.temperature is not None:
            kwargs["temperature"] = primary_llm_settings.temperature
        if primary_llm_settings.top_p_nucleus_sampling is not None:
            kwargs["top_p"] = primary_llm_settings.top_p_nucleus_sampling
        if primary_llm_settings.top_k_filtering is not None:
            kwargs["top_k"] = primary_llm_settings.top_k_filtering
        if primary_llm_settings.system_instruction:
            kwargs["system"] = primary_llm_settings.system_instruction
        if anthropic_tools:
            kwargs["tools"] = anthropic_tools

        thinking_enabled = (
            primary_llm_settings.thinking_config is not None
            and primary_llm_settings.thinking_config.thinking_level is not None
        )
        if thinking_enabled:
            kwargs["thinking"] = {
                "type": "enabled",
                "budget_tokens": primary_llm_settings.max_output_tokens // 2,
            }
            kwargs["temperature"] = 1.0
        accumulated_tool_name: dict[int, str] = {}
        accumulated_tool_id: dict[int, str] = {}
        accumulated_tool_input: dict[int, str] = {}
        block_types: dict[int, str] = {}
        accumulated_thinking_sig: dict[int, str] = {}

        async with self._client.messages.stream(**kwargs) as stream:
            async for event in stream:
                event_type = event.type

                if event_type == "content_block_start":
                    idx = event.index
                    block = event.content_block
                    block_types[idx] = block.type
                    if block.type == "tool_use":
                        accumulated_tool_name[idx] = block.name
                        accumulated_tool_id[idx] = block.id
                        accumulated_tool_input[idx] = ""
                    elif block.type == "thinking":
                        sig = getattr(block, "signature", None)
                        if sig:
                            accumulated_thinking_sig[idx] = sig

                elif event_type == "content_block_delta":
                    idx = event.index
                    delta = event.delta

                    if delta.type == "text_delta":
                        yield StreamChunkFromModel(text=delta.text or "")

                    elif delta.type == "thinking_delta":
                        yield StreamChunkFromModel(text=delta.thinking or "", thought=True)

                    elif delta.type == "signature_delta":
                        sig = getattr(delta, "signature", None)
                        if sig:
                            accumulated_thinking_sig[idx] = sig

                    elif delta.type == "input_json_delta":
                        accumulated_tool_input[idx] = accumulated_tool_input.get(idx, "") + (delta.partial_json or "")

                elif event_type == "content_block_stop":
                    idx = event.index
                    if block_types.get(idx) == "thinking":
                        sig = accumulated_thinking_sig.pop(idx, None)
                        if sig:
                            yield StreamChunkFromModel(thought=True, thought_signature=sig)
                    elif block_types.get(idx) == "tool_use":
                        raw_input = accumulated_tool_input.get(idx, "{}")
                        try:
                            args = json.loads(raw_input) if raw_input else {}
                        except json.JSONDecodeError:
                            args = {}
                        yield StreamChunkFromModel(tool_calls=[ToolCall(
                            name=accumulated_tool_name.get(idx, ""),
                            args=args,
                            id=accumulated_tool_id.get(idx),
                        )])

                elif event_type == "message_delta":
                    stop_reason = getattr(event.delta, "stop_reason", None)
                    usage = getattr(event, "usage", None)
                    um = None
                    if usage:
                        um = UsageMetadata(
                            candidates_token_count=getattr(usage, "output_tokens", 0) or 0,
                        )
                    if stop_reason:
                        yield StreamChunkFromModel(finish_reason=stop_reason, usage_metadata=um)

                elif event_type == "message_start":
                    msg = getattr(event, "message", None)
                    if msg:
                        usage = getattr(msg, "usage", None)
                        if usage:
                            yield StreamChunkFromModel(usage_metadata=UsageMetadata(
                                prompt_token_count=getattr(usage, "input_tokens", 0) or 0,
                            ))

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_anthropic(contents)
        anthropic_tools = _tools_to_anthropic(primary_llm_settings.tools)

        kwargs = {
            "model": model,
            "messages": messages,
            "max_tokens": primary_llm_settings.max_output_tokens,
        }
        if primary_llm_settings.temperature is not None:
            kwargs["temperature"] = primary_llm_settings.temperature
        if primary_llm_settings.top_p_nucleus_sampling is not None:
            kwargs["top_p"] = primary_llm_settings.top_p_nucleus_sampling
        if primary_llm_settings.top_k_filtering is not None:
            kwargs["top_k"] = primary_llm_settings.top_k_filtering
        if primary_llm_settings.system_instruction:
            kwargs["system"] = primary_llm_settings.system_instruction
        if anthropic_tools:
            kwargs["tools"] = anthropic_tools

        thinking_enabled = (
            primary_llm_settings.thinking_config is not None
            and primary_llm_settings.thinking_config.thinking_level is not None
        )
        if thinking_enabled:
            kwargs["thinking"] = {
                "type": "enabled",
                "budget_tokens": primary_llm_settings.max_output_tokens // 2,
            }
            kwargs["temperature"] = 1.0

        response = await self._client.messages.create(**kwargs)

        parts = _parse_response_blocks(response.content)

        usage = UsageMetadata()
        if response.usage:
            usage = UsageMetadata(
                prompt_token_count=getattr(response.usage, "input_tokens", 0) or 0,
                candidates_token_count=getattr(response.usage, "output_tokens", 0) or 0,
                total_token_count=(
                    (getattr(response.usage, "input_tokens", 0) or 0)
                    + (getattr(response.usage, "output_tokens", 0) or 0)
                ),
            )

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=response.stop_reason,
            )],
            usage_metadata=usage,
        )

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_anthropic(contents)

        kwargs = {
            "model": model,
            "messages": messages,
            "max_tokens": assistant_llm_settings.max_output_tokens,
        }
        if assistant_llm_settings.temperature is not None:
            kwargs["temperature"] = assistant_llm_settings.temperature
        if assistant_llm_settings.top_p_nucleus_sampling is not None:
            kwargs["top_p"] = assistant_llm_settings.top_p_nucleus_sampling
        if assistant_llm_settings.top_k_filtering is not None:
            kwargs["top_k"] = assistant_llm_settings.top_k_filtering
        if assistant_llm_settings.system_instruction:
            kwargs["system"] = assistant_llm_settings.system_instruction

        accumulated_text: list[str] = []

        async with self._client.messages.stream(**kwargs) as stream:
            async for event in stream:
                event_type = event.type

                if event_type == "content_block_delta":
                    delta = event.delta
                    if delta.type == "text_delta":
                        yield StreamChunkFromModel(text=delta.text or "")

                elif event_type == "message_delta":
                    stop_reason = getattr(event.delta, "stop_reason", None)
                    usage = getattr(event, "usage", None)
                    um = None
                    if usage:
                        um = UsageMetadata(
                            candidates_token_count=getattr(usage, "output_tokens", 0) or 0,
                        )
                    if stop_reason:
                        yield StreamChunkFromModel(finish_reason=stop_reason, usage_metadata=um)

                elif event_type == "message_start":
                    msg = getattr(event, "message", None)
                    if msg:
                        usage = getattr(msg, "usage", None)
                        if usage:
                            yield StreamChunkFromModel(usage_metadata=UsageMetadata(
                                prompt_token_count=getattr(usage, "input_tokens", 0) or 0,
                            ))

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_anthropic(contents)

        kwargs = {
            "model": model,
            "messages": messages,
            "max_tokens": assistant_llm_settings.max_output_tokens,
        }
        if assistant_llm_settings.temperature is not None:
            kwargs["temperature"] = assistant_llm_settings.temperature
        if assistant_llm_settings.top_p_nucleus_sampling is not None:
            kwargs["top_p"] = assistant_llm_settings.top_p_nucleus_sampling
        if assistant_llm_settings.top_k_filtering is not None:
            kwargs["top_k"] = assistant_llm_settings.top_k_filtering
        if assistant_llm_settings.system_instruction:
            kwargs["system"] = assistant_llm_settings.system_instruction

        response = await self._client.messages.create(**kwargs)

        parts = _parse_response_blocks(response.content)

        usage = UsageMetadata()
        if response.usage:
            usage = UsageMetadata(
                prompt_token_count=getattr(response.usage, "input_tokens", 0) or 0,
                candidates_token_count=getattr(response.usage, "output_tokens", 0) or 0,
                total_token_count=(
                    (getattr(response.usage, "input_tokens", 0) or 0)
                    + (getattr(response.usage, "output_tokens", 0) or 0)
                ),
            )

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=response.stop_reason,
            )],
            usage_metadata=usage,
        )

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_anthropic(contents)

        kwargs = {
            "model": model,
            "messages": messages,
            "max_tokens": lite_llm_settings.max_output_tokens,
        }
        if lite_llm_settings.temperature is not None:
            kwargs["temperature"] = lite_llm_settings.temperature
        if lite_llm_settings.top_p_nucleus_sampling is not None:
            kwargs["top_p"] = lite_llm_settings.top_p_nucleus_sampling
        if lite_llm_settings.top_k_filtering is not None:
            kwargs["top_k"] = lite_llm_settings.top_k_filtering
        if lite_llm_settings.system_instruction:
            kwargs["system"] = lite_llm_settings.system_instruction

        async with self._client.messages.stream(**kwargs) as stream:
            async for event in stream:
                event_type = event.type

                if event_type == "content_block_delta":
                    delta = event.delta
                    if delta.type == "text_delta":
                        yield StreamChunkFromModel(text=delta.text or "")

                elif event_type == "message_delta":
                    stop_reason = getattr(event.delta, "stop_reason", None)
                    usage = getattr(event, "usage", None)
                    um = None
                    if usage:
                        um = UsageMetadata(
                            candidates_token_count=getattr(usage, "output_tokens", 0) or 0,
                        )
                    if stop_reason:
                        yield StreamChunkFromModel(finish_reason=stop_reason, usage_metadata=um)

                elif event_type == "message_start":
                    msg = getattr(event, "message", None)
                    if msg:
                        usage = getattr(msg, "usage", None)
                        if usage:
                            yield StreamChunkFromModel(usage_metadata=UsageMetadata(
                                prompt_token_count=getattr(usage, "input_tokens", 0) or 0,
                            ))

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_anthropic(contents)

        kwargs = {
            "model": model,
            "messages": messages,
            "max_tokens": lite_llm_settings.max_output_tokens,
        }
        if lite_llm_settings.temperature is not None:
            kwargs["temperature"] = lite_llm_settings.temperature
        if lite_llm_settings.top_p_nucleus_sampling is not None:
            kwargs["top_p"] = lite_llm_settings.top_p_nucleus_sampling
        if lite_llm_settings.top_k_filtering is not None:
            kwargs["top_k"] = lite_llm_settings.top_k_filtering
        if lite_llm_settings.system_instruction:
            kwargs["system"] = lite_llm_settings.system_instruction

        response = await self._client.messages.create(**kwargs)

        parts = _parse_response_blocks(response.content)

        usage = UsageMetadata()
        if response.usage:
            usage = UsageMetadata(
                prompt_token_count=getattr(response.usage, "input_tokens", 0) or 0,
                candidates_token_count=getattr(response.usage, "output_tokens", 0) or 0,
                total_token_count=(
                    (getattr(response.usage, "input_tokens", 0) or 0)
                    + (getattr(response.usage, "output_tokens", 0) or 0)
                ),
            )

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=response.stop_reason,
            )],
            usage_metadata=usage,
        )
