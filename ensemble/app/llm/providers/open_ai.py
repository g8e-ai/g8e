# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
import logging
from collections.abc import AsyncGenerator

from openai import AsyncOpenAI

from app.errors import ValidationError
from app.llm.llm_types import (
    AssistantLLMSettings,
    Candidate,
    Content,
    GenerateContentResponse,
    LiteLLMSettings,
    Part,
    PrimaryLLMSettings,
    StreamChunkFromModel,
    ToolCall,
    ToolGroup,
    UsageMetadata,
)
from app.llm.provider import LLMProvider
from app.llm.providers._capability import translate_capability_error
from app.llm.thinking import translate_for_openai
from app.models.model_configs import get_model_config

logger = logging.getLogger(__name__)


def _contents_to_messages(
    contents: list[Content],
    system_instructions: str | None,
) -> list[dict]:
    messages = []

    if system_instructions:
        messages.append({"role": "system", "content": system_instructions})

    for content in contents:
        role = "assistant" if content.role == "model" else content.role
        calls = [part.tool_call for part in content.parts if part.tool_call]
        if calls:
            messages.append(
                {
                    "role": "assistant",
                    "content": "".join(part.text for part in content.parts if part.text) or None,
                    "tool_calls": [
                        {
                            "id": call.id or f"call_{call.name}",
                            "type": "function",
                            "function": {
                                "name": call.name,
                                "arguments": json.dumps(call.args),
                            },
                        }
                        for call in calls
                    ],
                }
            )

        for part in content.parts:
            if part.tool_call:
                continue
            if part.tool_response:
                messages.append(
                    {
                        "role": "tool",
                        "tool_call_id": part.tool_response.id or f"call_{part.tool_response.name}",
                        "content": json.dumps(part.tool_response.response),
                    }
                )
            elif part.text and not calls:
                messages.append({"role": role, "content": part.text})

    return messages


def _token_count(value) -> int:
    return value if isinstance(value, int) and not isinstance(value, bool) else 0


def _optional_token_count(value) -> int | None:
    return value if isinstance(value, int) and not isinstance(value, bool) else None


def _usage_from_sdk(response_usage) -> UsageMetadata:
    if not response_usage:
        return UsageMetadata()
    details = getattr(response_usage, "prompt_tokens_details", None)
    cache_token_count = None
    if details is not None and hasattr(details, "cached_tokens"):
        cache_token_count = _optional_token_count(getattr(details, "cached_tokens", None))
    return UsageMetadata(
        prompt_token_count=_token_count(getattr(response_usage, "prompt_tokens", 0)),
        candidates_token_count=_token_count(getattr(response_usage, "completion_tokens", 0)),
        total_token_count=_token_count(getattr(response_usage, "total_tokens", 0)),
        cache_token_count=cache_token_count,
        usage_reported=True,
    )


def _tools_to_openai(tools: list[ToolGroup] | None) -> list[dict] | None:
    if not tools:
        return None

    openai_tools = []
    openai_tools.extend(
        {
            "type": "function",
            "function": {
                "name": decl.name,
                "description": decl.description,
                "parameters": decl.parameters.to_json_schema(),
            },
        }
        for tool in tools
        for decl in tool.tools
    )

    return openai_tools if openai_tools else None


class OpenAIProvider(LLMProvider):
    def __init__(self, endpoint: str, api_key: str):
        super().__init__()

        # Ensure endpoint has /v1 suffix for OpenAI API
        base_url = endpoint
        if not base_url.endswith("/v1"):
            base_url = base_url + "/v1"

        self._client = AsyncOpenAI(
            base_url=base_url,
            api_key=api_key or "not-needed",
            max_retries=0,
        )
        # CodeQL: Don't log full URLs as they may contain sensitive info in some setups
        logger.info("OpenAI provider initialized")

    @property
    def service_name(self) -> str:
        """Return the service name for error reporting."""
        return "openai"

    @staticmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        """Validate OpenAI provider configuration.

        OpenAI requires both an API key and an endpoint URL.

        Args:
            api_key: The API key for the provider
            endpoint: The endpoint URL for the provider

        Returns:
            List of validation error messages. Empty if configuration is valid.
        """
        errors = []
        if not api_key:
            errors.append("Provider 'openai' requires an API key.")
        if not endpoint:
            errors.append("Provider 'openai' requires an endpoint URL.")
        return errors

    async def _close_resources(self):
        """Clean up provider resources."""
        if hasattr(self._client, "close"):
            await self._client.close()
        logger.info("OpenAI provider closed")

    @staticmethod
    def _build_openai_kwargs(
        model: str,
        messages: list[dict],
        max_tokens: int | None,
        top_p: float | None,
        stop: list[str] | None,
        tools: list[dict] | None = None,
        response_format: dict | None = None,
        stream: bool = False,
        thinking_config=None,
    ) -> dict:
        """Build OpenAI API kwargs, omitting None values.

        When a ThinkingConfig is supplied we translate its ThinkingLevel into
        the reasoning.effort field that GPT-5/o-series reasoning models accept.
        Models that do not support reasoning effort (per LLModelConfig)
        receive no reasoning key.
        """
        kwargs = {
            "model": model,
            "messages": messages,
            "stream": stream,
        }
        if max_tokens is not None:
            kwargs["max_tokens"] = max_tokens
        if stream:
            kwargs["stream_options"] = {"include_usage": True}
        if top_p is not None:
            kwargs["top_p"] = top_p
        if stop:
            kwargs["stop"] = stop
        if tools:
            kwargs["tools"] = tools
        if response_format:
            kwargs["response_format"] = response_format
        if thinking_config is not None:
            translation = translate_for_openai(
                thinking_config.thinking_level,
                get_model_config(model),
            )
            if translation.enabled and translation.reasoning_effort:
                kwargs["reasoning"] = {"effort": translation.reasoning_effort}
        return kwargs

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        try:
            async for chunk in self._generate_content_stream_primary_impl(
                model, contents, primary_llm_settings
            ):
                yield chunk
        except Exception as e:
            translate_capability_error(
                e,
                service_name=self.service_name,
                model=model,
                thinking_requested=bool(
                    primary_llm_settings.thinking_config
                    and primary_llm_settings.thinking_config.enabled
                ),
                tools_requested=bool(primary_llm_settings.tools),
            )
            raise

    async def _generate_content_stream_primary_impl(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_messages(contents, primary_llm_settings.system_instructions)
        openai_tools = _tools_to_openai(primary_llm_settings.tools)
        self._record_declared_tools(tool["function"]["name"] for tool in (openai_tools or []))

        effective_max_tokens = primary_llm_settings.max_output_tokens

        if openai_tools:
            # Some endpoints hang on streaming when tools are present.
            # Use non-streaming and yield the response as chunks.
            kwargs = self._build_openai_kwargs(
                model=model,
                messages=messages,
                max_tokens=effective_max_tokens,
                top_p=primary_llm_settings.top_p_nucleus_sampling,
                stop=primary_llm_settings.stop_sequences,
                tools=openai_tools,
                stream=False,
                thinking_config=primary_llm_settings.thinking_config,
            )
            self._record_model_boundary(kwargs)
            response = await self._client.chat.completions.create(**kwargs)
            self._record_response(response, complete=True)
            choice = response.choices[0] if response.choices else None
            finish_reason = choice.finish_reason if choice else None

            if choice and choice.message:
                # Check for reasoning content (OpenAI)
                reasoning = getattr(choice.message, "reasoning_content", None)
                if reasoning:
                    yield StreamChunkFromModel(text=reasoning, thought=True)

                if choice.message.content:
                    yield StreamChunkFromModel(text=choice.message.content)

                if choice.message.tool_calls:
                    calls = []
                    for tc in choice.message.tool_calls:
                        try:
                            args = json.loads(tc.function.arguments)
                        except json.JSONDecodeError as exc:
                            raise ValidationError(
                                "Provider returned invalid tool arguments JSON"
                            ) from exc
                        if not isinstance(args, dict):
                            raise ValidationError("Provider tool arguments must be a JSON object")
                        calls.append(
                            ToolCall(name=tc.function.name, args=args, id=getattr(tc, "id", None))
                        )
                    yield StreamChunkFromModel(tool_calls=calls)

            usage = _usage_from_sdk(response.usage)
            yield StreamChunkFromModel(finish_reason=finish_reason or "stop", usage_metadata=usage)
        else:
            kwargs = self._build_openai_kwargs(
                model=model,
                messages=messages,
                max_tokens=effective_max_tokens,
                top_p=primary_llm_settings.top_p_nucleus_sampling,
                stop=primary_llm_settings.stop_sequences,
                tools=openai_tools,
                stream=True,
                thinking_config=primary_llm_settings.thinking_config,
            )
            self._record_model_boundary(kwargs)
            stream = await self._client.chat.completions.create(**kwargs)

            for chunk in await self._receive_stream(stream):
                delta = chunk.choices[0].delta if chunk.choices else None
                finish_reason = chunk.choices[0].finish_reason if chunk.choices else None
                usage = _usage_from_sdk(getattr(chunk, "usage", None))
                if usage.usage_reported:
                    yield StreamChunkFromModel(usage_metadata=usage)

                if delta:
                    # Check for reasoning content (OpenAI)
                    reasoning = getattr(delta, "reasoning_content", None)
                    if reasoning:
                        yield StreamChunkFromModel(text=reasoning, thought=True)

                if delta and delta.content:
                    yield StreamChunkFromModel(text=delta.content)

                if finish_reason and finish_reason != "tool_calls":
                    yield StreamChunkFromModel(finish_reason=finish_reason)

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_messages(contents, primary_llm_settings.system_instructions)
        openai_tools = _tools_to_openai(primary_llm_settings.tools)

        effective_max_tokens = primary_llm_settings.max_output_tokens

        kwargs = self._build_openai_kwargs(
            model=model,
            messages=messages,
            max_tokens=effective_max_tokens,
            top_p=primary_llm_settings.top_p_nucleus_sampling,
            stop=primary_llm_settings.stop_sequences,
            tools=openai_tools,
            stream=False,
            thinking_config=primary_llm_settings.thinking_config,
        )
        try:
            self._record_model_boundary(kwargs)
            response = await self._client.chat.completions.create(**kwargs)
            self._record_response(response, complete=True)
        except Exception as e:
            translate_capability_error(
                e,
                service_name=self.service_name,
                model=model,
                thinking_requested=bool(
                    primary_llm_settings.thinking_config
                    and primary_llm_settings.thinking_config.enabled
                ),
                tools_requested=bool(primary_llm_settings.tools),
            )
            raise

        parts = []
        choice = response.choices[0] if response.choices else None

        if choice and choice.message:
            # Check for reasoning content (OpenAI)
            reasoning = getattr(choice.message, "reasoning_content", None)
            if reasoning:
                parts.append(Part(text=reasoning, thought=True))
            if choice.message.content:
                parts.append(Part(text=choice.message.content))
            if choice.message.tool_calls:
                for tc in choice.message.tool_calls:
                    try:
                        args = json.loads(tc.function.arguments)
                    except json.JSONDecodeError as exc:
                        raise ValidationError(
                            "Provider returned invalid tool arguments JSON"
                        ) from exc
                    if not isinstance(args, dict):
                        raise ValidationError("Provider tool arguments must be a JSON object")
                    parts.append(
                        Part(
                            tool_call=ToolCall(
                                name=tc.function.name, args=args, id=getattr(tc, "id", None)
                            )
                        )
                    )

        usage = _usage_from_sdk(response.usage)

        return GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(role="model", parts=parts),
                    finish_reason=(choice.finish_reason if choice else None) or "stop",
                )
            ],
            usage_metadata=usage,
        )

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        try:
            async for chunk in self._generate_content_stream_assistant_impl(
                model, contents, assistant_llm_settings
            ):
                yield chunk
        except Exception as e:
            translate_capability_error(
                e,
                service_name=self.service_name,
                model=model,
                thinking_requested=False,  # Thinking not supported for assistant yet
                tools_requested=False,  # Tools not supported for assistant yet
            )
            raise

    async def _generate_content_stream_assistant_impl(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_messages(contents, assistant_llm_settings.system_instructions)

        effective_max_tokens = assistant_llm_settings.max_output_tokens

        response_format = (
            assistant_llm_settings.response_format.flatten_for_openai()
            if assistant_llm_settings.response_format
            else None
        )

        kwargs = self._build_openai_kwargs(
            model=model,
            messages=messages,
            max_tokens=effective_max_tokens,
            top_p=assistant_llm_settings.top_p_nucleus_sampling,
            stop=assistant_llm_settings.stop_sequences,
            response_format=response_format,
            stream=True,
        )
        self._record_model_boundary(kwargs)
        stream = await self._client.chat.completions.create(**kwargs)

        for chunk in await self._receive_stream(stream):
            delta = chunk.choices[0].delta if chunk.choices else None
            finish_reason = chunk.choices[0].finish_reason if chunk.choices else None

            if delta and delta.content:
                yield StreamChunkFromModel(text=delta.content)

            if finish_reason:
                yield StreamChunkFromModel(finish_reason=finish_reason)

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        try:
            return await self._generate_content_assistant_impl(
                model, contents, assistant_llm_settings
            )
        except Exception as e:
            translate_capability_error(
                e,
                service_name=self.service_name,
                model=model,
                thinking_requested=False,
                tools_requested=False,
            )
            raise

    async def _generate_content_assistant_impl(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_messages(contents, assistant_llm_settings.system_instructions)

        effective_max_tokens = assistant_llm_settings.max_output_tokens

        response_format = (
            assistant_llm_settings.response_format.flatten_for_openai()
            if assistant_llm_settings.response_format
            else None
        )

        kwargs = self._build_openai_kwargs(
            model=model,
            messages=messages,
            max_tokens=effective_max_tokens,
            top_p=assistant_llm_settings.top_p_nucleus_sampling,
            stop=assistant_llm_settings.stop_sequences,
            response_format=response_format,
            stream=False,
        )
        self._record_model_boundary(kwargs)
        response = await self._client.chat.completions.create(**kwargs)
        self._record_response(response, complete=True)

        parts = []
        choice = response.choices[0] if response.choices else None

        if choice and choice.message and choice.message.content:
            parts.append(Part(text=choice.message.content))

        usage = _usage_from_sdk(response.usage)

        return GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(role="model", parts=parts),
                    finish_reason=(choice.finish_reason if choice else None) or "stop",
                )
            ],
            usage_metadata=usage,
        )

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        try:
            async for chunk in self._generate_content_stream_lite_impl(
                model, contents, lite_llm_settings
            ):
                yield chunk
        except Exception as e:
            translate_capability_error(
                e,
                service_name=self.service_name,
                model=model,
                thinking_requested=False,
                tools_requested=False,
            )
            raise

    async def _generate_content_stream_lite_impl(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_messages(contents, lite_llm_settings.system_instructions)

        effective_max_tokens = lite_llm_settings.max_output_tokens

        response_format = (
            lite_llm_settings.response_format.flatten_for_openai()
            if lite_llm_settings.response_format
            else None
        )

        kwargs = self._build_openai_kwargs(
            model=model,
            messages=messages,
            max_tokens=effective_max_tokens,
            top_p=lite_llm_settings.top_p_nucleus_sampling,
            stop=lite_llm_settings.stop_sequences,
            response_format=response_format,
            stream=True,
        )
        self._record_model_boundary(kwargs)
        stream = await self._client.chat.completions.create(**kwargs)

        for chunk in await self._receive_stream(stream):
            delta = chunk.choices[0].delta if chunk.choices else None
            finish_reason = chunk.choices[0].finish_reason if chunk.choices else None

            if delta and delta.content:
                yield StreamChunkFromModel(text=delta.content)

            if finish_reason:
                yield StreamChunkFromModel(finish_reason=finish_reason)

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        try:
            return await self._generate_content_lite_impl(model, contents, lite_llm_settings)
        except Exception as e:
            translate_capability_error(
                e,
                service_name=self.service_name,
                model=model,
                thinking_requested=False,
                tools_requested=False,
            )
            raise

    async def _generate_content_lite_impl(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_messages(contents, lite_llm_settings.system_instructions)

        effective_max_tokens = lite_llm_settings.max_output_tokens

        response_format = (
            lite_llm_settings.response_format.flatten_for_openai()
            if lite_llm_settings.response_format
            else None
        )

        kwargs = self._build_openai_kwargs(
            model=model,
            messages=messages,
            max_tokens=effective_max_tokens,
            top_p=lite_llm_settings.top_p_nucleus_sampling,
            stop=lite_llm_settings.stop_sequences,
            response_format=response_format,
            stream=False,
        )
        self._record_model_boundary(kwargs)
        response = await self._client.chat.completions.create(**kwargs)
        self._record_response(response, complete=True)

        parts = []
        choice = response.choices[0] if response.choices else None

        if choice and choice.message and choice.message.content:
            parts.append(Part(text=choice.message.content))

        usage = _usage_from_sdk(response.usage)

        return GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(role="model", parts=parts),
                    finish_reason=(choice.finish_reason if choice else None) or "stop",
                )
            ],
            usage_metadata=usage,
        )
