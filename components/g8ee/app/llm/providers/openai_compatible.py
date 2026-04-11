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

import json
import logging
from collections.abc import AsyncGenerator

import httpx
from openai import AsyncOpenAI

from app.llm.llm_types import (
    Candidate,
    Content,
    ToolCall,
    GenerateContentConfig,
    GenerateContentResponse,
    Part,
    StreamChunkFromModel,
    UsageMetadata,
    ToolGroup,
)

from ..provider import LLMProvider
from ..utils import is_internal_endpoint, is_ollama_endpoint, schema_to_dict

logger = logging.getLogger(__name__)


async def _ollama_direct_call(
    endpoint: str,
    model: str,
    messages: list[dict],
    tools: list[dict] | None,
    temperature: float,
    max_tokens: int | None,
    top_p: float | None,
    stop: list[str] | None,
    think: bool,
    stream: bool,
    http_client: httpx.AsyncClient,
) -> AsyncGenerator[StreamChunkFromModel, None]:
    """Make direct HTTP calls to Ollama API when thinking is needed."""
    
    # Convert endpoint to string and handle OpenAI-style endpoint to Ollama API endpoint
    endpoint_str = str(endpoint).rstrip('/')
    if "/v1" in endpoint_str:
        ollama_endpoint = endpoint_str.replace("/v1", "/api")
    else:
        ollama_endpoint = endpoint_str + "/api"
    
    payload = {
        "model": model,
        "messages": messages,
        "stream": stream,
        "temperature": temperature,
    }
    
    if max_tokens is not None:
        payload["num_predict"] = max_tokens
    if top_p is not None:
        payload["top_p"] = top_p
    if stop:
        payload["stop"] = stop
    if tools:
        payload["tools"] = tools
    if think:
        payload["think"] = True
    
    if stream:
        async with http_client.stream(
            "POST",
            f"{ollama_endpoint}/chat",
            json=payload,
        ) as response:
            response.raise_for_status()
            async for line in response.aiter_lines():
                if line.strip():
                    try:
                        chunk = json.loads(line)
                        if "message" in chunk:
                            message = chunk["message"]
                            # Handle thinking content
                            if "thinking" in message and message["thinking"]:
                                yield StreamChunkFromModel(text=message["thinking"], thought=True)
                            # Handle regular content
                            if "content" in message and message["content"]:
                                yield StreamChunkFromModel(text=message["content"])
                        if chunk.get("done"):
                            yield StreamChunkFromModel(finish_reason="stop")
                            if "prompt_eval_count" in chunk and "eval_count" in chunk:
                                usage = UsageMetadata(
                                    prompt_token_count=chunk.get("prompt_eval_count", 0),
                                    candidates_token_count=chunk.get("eval_count", 0),
                                    total_token_count=chunk.get("prompt_eval_count", 0) + chunk.get("eval_count", 0),
                                )
                                yield StreamChunkFromModel(usage_metadata=usage)
                    except json.JSONDecodeError:
                        continue
    else:
        response = await http_client.post(
            f"{ollama_endpoint}/chat",
            json=payload,
        )
        response.raise_for_status()
        data = response.json()
        
        if "message" in data:
            message = data["message"]
            # Handle thinking content
            if "thinking" in message and message["thinking"]:
                yield StreamChunkFromModel(text=message["thinking"], thought=True)
            # Handle regular content
            if "content" in message and message["content"]:
                yield StreamChunkFromModel(text=message["content"])
        
        yield StreamChunkFromModel(finish_reason="stop")
        if "prompt_eval_count" in data and "eval_count" in data:
            usage = UsageMetadata(
                prompt_token_count=data.get("prompt_eval_count", 0),
                candidates_token_count=data.get("eval_count", 0),
                total_token_count=data.get("prompt_eval_count", 0) + data.get("eval_count", 0),
            )
            yield StreamChunkFromModel(usage_metadata=usage)


def _contents_to_messages(
    contents: list[Content],
    system_instruction: str,
) -> list[dict]:
    messages = []

    if system_instruction:
        messages.append({"role": "system", "content": system_instruction})

    for content in contents:
        role = "assistant" if content.role == "model" else content.role

        for part in content.parts:
            if part.tool_call:
                messages.append({
                    "role": "assistant",
                    "content": None,
                    "tool_calls": [{
                        "id": f"call_{part.tool_call.name}",
                        "type": "tool",
                        "tool": {
                            "name": part.tool_call.name,
                            "arguments": json.dumps(part.tool_call.args),
                        },
                    }],
                })
            elif part.tool_response:
                messages.append({
                    "role": "tool",
                    "tool_call_id": f"call_{part.tool_response.name}",
                    "content": json.dumps(part.tool_response.response),
                })
            elif part.text:
                messages.append({"role": role, "content": part.text})

    return messages


def _tools_to_openai(tools: list[ToolGroup] | None) -> list[dict] | None:
    if not tools:
        return None

    openai_tools = []
    for tool in tools:
        for decl in tool.tools:
            params = _schema_to_dict(decl.parameters) if decl.parameters else None
            openai_tools.append({
                "type": "function",
                "function": {
                    "name": decl.name,
                    "description": decl.description,
                    "parameters": params,
                },
            })

    return openai_tools if openai_tools else None


class OpenAICompatibleProvider(LLMProvider):
    _TIMEOUT = httpx.Timeout(connect=10.0, read=300.0, write=30.0, pool=5.0)
    _LIMITS = httpx.Limits(
        max_connections=10,
        max_keepalive_connections=5,
        keepalive_expiry=30.0,
    )

    def __init__(self, endpoint: str, api_key: str, ca_cert_path: str | None = None):
        self._is_ollama = is_ollama_endpoint(endpoint)

        verify: str | bool
        if is_internal_endpoint(endpoint):
            if endpoint.startswith("http://"):
                verify = False
            elif ca_cert_path:
                verify = ca_cert_path
            else:
                verify = True
        else:
            verify = True

        # Create HTTP client for direct Ollama calls
        self._http_client = httpx.AsyncClient(
            timeout=self._TIMEOUT,
            limits=self._LIMITS,
            verify=verify,
        )
        
        # Create OpenAI client for non-Ollama endpoints
        self._client = AsyncOpenAI(
            base_url=endpoint,
            api_key=api_key or "not-needed",
            http_client=httpx.AsyncClient(
                timeout=self._TIMEOUT,
                limits=self._LIMITS,
                verify=verify,
            ),
            max_retries=0,
        )
        logger.info(f"OpenAI-compatible provider initialized: {endpoint} (Ollama: {self._is_ollama})")
    

    async def generate_content_stream(
        self,
        model: str,
        contents: list[Content],
        config: GenerateContentConfig,
        tools: list[ToolGroup] = None,
        system_instruction: str = None,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        sys_instr = system_instruction or (config.system_instruction if config else None)
        effective_tools = tools or (config.tools if config else None)
        messages = _contents_to_messages(contents, sys_instr)
        openai_tools = _tools_to_openai(effective_tools)

        # For Ollama with thinking, use direct HTTP calls
        if self._is_ollama:
            async for chunk in _ollama_direct_call(
                endpoint=self._client.base_url,
                model=model,
                messages=messages,
                tools=openai_tools,
                temperature=config.temperature,
                max_tokens=config.max_output_tokens,
                top_p=config.top_p_nucleus_sampling,
                stop=config.stop_sequences,
                think=True,  # Always enable thinking for Ollama
                stream=True,
                http_client=self._http_client,
            ):
                yield chunk
            return

        # For non-Ollama endpoints, use the OpenAI client
        kwargs = {
            "model": model,
            "messages": messages,
            "temperature": config.temperature,
        }
        if config.max_output_tokens is not None:
            kwargs["max_tokens"] = config.max_output_tokens
        if config.top_p_nucleus_sampling is not None:
            kwargs["top_p"] = config.top_p_nucleus_sampling
        if config.stop_sequences:
            kwargs["stop"] = config.stop_sequences
        if openai_tools:
            kwargs["tools"] = openai_tools
        if config.response_format is not None:
            rjs = config.response_format.json_schema
            kwargs["response_format"] = {
                "type": "json_schema",
                "json_schema": {
                    "name": rjs.name,
                    "schema": rjs.schema,
                    "strict": rjs.strict,
                },
            }

        if openai_tools:
            # Ollama (and some other endpoints) hang on streaming when tools are present.
            # Use non-streaming and yield the response as chunks.
            response = await self._client.chat.completions.create(**kwargs, stream=False)
            choice = response.choices[0] if response.choices else None
            finish_reason = choice.finish_reason if choice else None

            if choice and choice.message:
                # Check for thinking content (Ollama) or reasoning content (OpenAI)
                thinking = getattr(choice.message, "thinking", None)
                reasoning = getattr(choice.message, "reasoning_content", None)
                if thinking:
                    yield StreamChunkFromModel(text=thinking, thought=True)
                elif reasoning:
                    yield StreamChunkFromModel(text=reasoning, thought=True)
                
                if choice.message.content:
                    yield StreamChunkFromModel(text=choice.message.content)

                if choice.message.tool_calls:
                    calls = []
                    for tc in choice.message.tool_calls:
                        try:
                            args = json.loads(tc.function.arguments)
                        except json.JSONDecodeError:
                            args = {}
                        calls.append(ToolCall(name=tc.function.name, args=args, id=getattr(tc, "id", None)))
                    yield StreamChunkFromModel(tool_calls=calls)

            usage = None
            if response.usage:
                usage = UsageMetadata(
                    prompt_token_count=response.usage.prompt_tokens or 0,
                    candidates_token_count=response.usage.completion_tokens or 0,
                    total_token_count=response.usage.total_tokens or 0,
                )
            yield StreamChunkFromModel(finish_reason=finish_reason or "stop", usage_metadata=usage)
        else:
            stream = await self._client.chat.completions.create(**kwargs, stream=True)

            async for chunk in stream:
                delta = chunk.choices[0].delta if chunk.choices else None
                finish_reason = chunk.choices[0].finish_reason if chunk.choices else None

                if delta:
                    # Check for thinking content (Ollama) or reasoning content (OpenAI)
                    thinking = getattr(delta, "thinking", None)
                    reasoning = getattr(delta, "reasoning_content", None)
                    if thinking:
                        yield StreamChunkFromModel(text=thinking, thought=True)
                    elif reasoning:
                        yield StreamChunkFromModel(text=reasoning, thought=True)

                if delta and delta.content:
                    yield StreamChunkFromModel(text=delta.content)

                if finish_reason and finish_reason != "tool_calls":
                    yield StreamChunkFromModel(finish_reason=finish_reason)

    async def generate_content(
        self,
        model: str,
        contents: list[Content],
        config: GenerateContentConfig,
        tools: list[ToolGroup] = None,
        system_instruction: str = None,
    ) -> GenerateContentResponse:
        sys_instr = system_instruction or (config.system_instruction if config else None)
        effective_tools = tools or (config.tools if config else None)
        messages = _contents_to_messages(contents, sys_instr)
        openai_tools = _tools_to_openai(effective_tools)

        # For Ollama with thinking, use direct HTTP calls and convert to GenerateContentResponse
        if self._is_ollama:
            parts = []
            usage = None
            
            async for chunk in _ollama_direct_call(
                endpoint=self._client.base_url,
                model=model,
                messages=messages,
                tools=openai_tools,
                temperature=config.temperature,
                max_tokens=config.max_output_tokens,
                top_p=config.top_p_nucleus_sampling,
                stop=config.stop_sequences,
                think=True,  # Always enable thinking for Ollama
                stream=False,
                http_client=self._http_client,
            ):
                if chunk.text and chunk.thought:
                    parts.append(Part(text=chunk.text, thought=True))
                elif chunk.text:
                    parts.append(Part(text=chunk.text))
                if chunk.usage_metadata:
                    usage = chunk.usage_metadata
            
            return GenerateContentResponse(
                candidates=[Candidate(
                    content=Content(role="model", parts=parts),
                    finish_reason="stop",
                )],
                usage_metadata=usage,
            )

        # For non-Ollama endpoints, use the OpenAI client
        kwargs = {
            "model": model,
            "messages": messages,
            "temperature": config.temperature,
        }
        if config.max_output_tokens is not None:
            kwargs["max_tokens"] = config.max_output_tokens
        if config.top_p_nucleus_sampling is not None:
            kwargs["top_p"] = config.top_p_nucleus_sampling
        if config.stop_sequences:
            kwargs["stop"] = config.stop_sequences
        if openai_tools:
            kwargs["tools"] = openai_tools
        if config.response_format is not None:
            rjs = config.response_format.json_schema
            kwargs["response_format"] = {
                "type": "json_schema",
                "json_schema": {
                    "name": rjs.name,
                    "schema": rjs.schema,
                    "strict": rjs.strict,
                },
            }

        response = await self._client.chat.completions.create(**kwargs)

        parts = []
        choice = response.choices[0] if response.choices else None

        if choice and choice.message:
            # Check for thinking content (Ollama) or reasoning content (OpenAI)
            thinking = getattr(choice.message, "thinking", None)
            reasoning = getattr(choice.message, "reasoning_content", None)
            if thinking:
                parts.append(Part(text=thinking, thought=True))
            elif reasoning:
                parts.append(Part(text=reasoning, thought=True))
            if choice.message.content:
                parts.append(Part(text=choice.message.content))
            if choice.message.tool_calls:
                for tc in choice.message.tool_calls:
                    try:
                        args = json.loads(tc.function.arguments)
                    except json.JSONDecodeError:
                        args = {}
                    parts.append(Part(tool_call=ToolCall(
                        name=tc.function.name, args=args, id=getattr(tc, "id", None)
                    )))

        usage = None
        if response.usage:
            usage = UsageMetadata(
                prompt_token_count=response.usage.prompt_tokens or 0,
                candidates_token_count=response.usage.completion_tokens or 0,
                total_token_count=response.usage.total_tokens or 0,
            )

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=choice.finish_reason if choice else None,
            )],
            usage_metadata=usage,
        )
