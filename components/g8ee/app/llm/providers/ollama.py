import json
import logging
from collections.abc import AsyncGenerator

import httpx
from ollama import AsyncClient, Message as OllamaMessage
from ollama._client import BaseClient

from app.constants import LLM_DEFAULT_TEMPERATURE, LLM_DEFAULT_MAX_OUTPUT_TOKENS
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
    UsageMetadata,
    ToolGroup,
)

from ..provider import LLMProvider
from ..utils import is_internal_endpoint, schema_to_dict

logger = logging.getLogger(__name__)

class _InjectedAsyncClient(AsyncClient):
    """
    Subclass that overrides AsyncClient to properly inject an existing httpx client instance
    rather than letting the SDK construct its own internal instance.
    """
    def __init__(self, httpx_client: httpx.AsyncClient, host: str | None = None, **kwargs):
        # Pass a factory function that ignores kwargs and returns our instance
        BaseClient.__init__(self, lambda **k: httpx_client, host, **kwargs)

def _contents_to_messages(
    contents: list[Content],
    system_instruction: str,
) -> list[OllamaMessage]:
    messages = []

    if system_instruction:
        messages.append(OllamaMessage(role="system", content=system_instruction))

    for content in contents:
        role = "assistant" if content.role == "model" else content.role

        for part in content.parts:
            if part.tool_call:
                messages.append({
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [{
                        "type": "function",
                        "function": {
                            "name": part.tool_call.name,
                            "arguments": part.tool_call.args,
                        }
                    }],
                })
            elif part.tool_response:
                messages.append({
                    "role": "tool",
                    "content": json.dumps(part.tool_response.response),
                })
            elif part.text:
                messages.append({"role": role, "content": part.text})

    return messages


def _tools_to_ollama(tools: list[ToolGroup] | None) -> list[dict] | None:
    if not tools:
        return None

    ollama_tools = []
    for tool in tools:
        for decl in tool.tools:
            params = schema_to_dict(decl.parameters) if decl.parameters else None
            ollama_tools.append({
                "type": "function",
                "function": {
                    "name": decl.name,
                    "description": decl.description,
                    "parameters": params,
                },
            })

    return ollama_tools if ollama_tools else None


class OllamaProvider(LLMProvider):
    _TIMEOUT = httpx.Timeout(connect=10.0, read=300.0, write=30.0, pool=5.0)

    def __init__(self, endpoint: str, api_key: str, ca_cert_path: str | None = None):
        # Strip /v1 suffix if present - ollama SDK handles API paths internally
        cleaned_endpoint = endpoint.rstrip('/')
        if cleaned_endpoint.endswith('/v1'):
            cleaned_endpoint = cleaned_endpoint[:-3]
        
        # Ensure protocol prefix exists - Ollama SDK requires http:// or https://
        if not cleaned_endpoint.startswith('http://') and not cleaned_endpoint.startswith('https://'):
            cleaned_endpoint = 'http://' + cleaned_endpoint
        
        self._original_endpoint = cleaned_endpoint

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

        self._httpx_client = httpx.AsyncClient(
            timeout=self._TIMEOUT,
            verify=verify,
        )
        self._client = _InjectedAsyncClient(
            httpx_client=self._httpx_client,
            host=self._original_endpoint,
        )
        logger.info(f"Ollama provider initialized: {self._original_endpoint}")

    async def close(self):
        """Clean up the underlying httpx client."""
        await self._httpx_client.aclose()
        
    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_messages(contents, primary_llm_settings.system_instruction)
        ollama_tools = _tools_to_ollama(primary_llm_settings.tools)

        effective_temperature = primary_llm_settings.temperature if primary_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = primary_llm_settings.max_output_tokens if primary_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        options = {
            "temperature": effective_temperature,
            "num_predict": effective_max_tokens,
        }
        if primary_llm_settings.top_p_nucleus_sampling is not None:
            options["top_p"] = primary_llm_settings.top_p_nucleus_sampling
        if primary_llm_settings.stop_sequences:
            options["stop"] = primary_llm_settings.stop_sequences

        kwargs = {
            "model": model,
            "messages": messages,
            "options": options,
            "stream": True,
            "think": True, # enable thinking for primary
        }
        if ollama_tools:
            kwargs["tools"] = ollama_tools

        stream = await self._client.chat(**kwargs)
        
        async for chunk in stream:
            msg = chunk.message
            if getattr(msg, "thinking", None):
                yield StreamChunkFromModel(text=msg.thinking, thought=True)
            if getattr(msg, "content", None):
                yield StreamChunkFromModel(text=msg.content)
            
            if getattr(msg, "tool_calls", None):
                calls = []
                for tc in msg.tool_calls:
                    args = tc.function.arguments
                    calls.append(ToolCall(name=tc.function.name, args=args, id=None))
                yield StreamChunkFromModel(tool_calls=calls)
                
            if chunk.done:
                usage = None
                if getattr(chunk, "prompt_eval_count", None) is not None and getattr(chunk, "eval_count", None) is not None:
                    usage = UsageMetadata(
                        prompt_token_count=chunk.prompt_eval_count,
                        candidates_token_count=chunk.eval_count,
                        total_token_count=chunk.prompt_eval_count + chunk.eval_count,
                    )
                yield StreamChunkFromModel(finish_reason=chunk.done_reason or "stop", usage_metadata=usage)

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_messages(contents, primary_llm_settings.system_instruction)
        ollama_tools = _tools_to_ollama(primary_llm_settings.tools)

        effective_temperature = primary_llm_settings.temperature if primary_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = primary_llm_settings.max_output_tokens if primary_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        options = {
            "temperature": effective_temperature,
            "num_predict": effective_max_tokens,
        }
        if primary_llm_settings.top_p_nucleus_sampling is not None:
            options["top_p"] = primary_llm_settings.top_p_nucleus_sampling
        if primary_llm_settings.stop_sequences:
            options["stop"] = primary_llm_settings.stop_sequences

        kwargs = {
            "model": model,
            "messages": messages,
            "options": options,
            "stream": False,
            "think": True,
        }
        if ollama_tools:
            kwargs["tools"] = ollama_tools

        response = await self._client.chat(**kwargs)
        
        parts = []
        if getattr(response.message, "thinking", None):
            parts.append(Part(text=response.message.thinking, thought=True))
        if getattr(response.message, "content", None):
            parts.append(Part(text=response.message.content))
        if getattr(response.message, "tool_calls", None):
            for tc in response.message.tool_calls:
                parts.append(Part(tool_call=ToolCall(
                    name=tc.function.name, args=tc.function.arguments, id=None
                )))
                
        usage = None
        if getattr(response, "prompt_eval_count", None) is not None and getattr(response, "eval_count", None) is not None:
            usage = UsageMetadata(
                prompt_token_count=response.prompt_eval_count,
                candidates_token_count=response.eval_count,
                total_token_count=response.prompt_eval_count + response.eval_count,
            )

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=response.done_reason or "stop",
            )],
            usage_metadata=usage,
        )

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_messages(contents, assistant_llm_settings.system_instruction)

        effective_temperature = assistant_llm_settings.temperature if assistant_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = assistant_llm_settings.max_output_tokens if assistant_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        options = {
            "temperature": effective_temperature,
            "num_predict": effective_max_tokens,
        }
        if assistant_llm_settings.top_p_nucleus_sampling is not None:
            options["top_p"] = assistant_llm_settings.top_p_nucleus_sampling
        if assistant_llm_settings.stop_sequences:
            options["stop"] = assistant_llm_settings.stop_sequences
            
        kwargs = {
            "model": model,
            "messages": messages,
            "options": options,
            "stream": True,
            "think": False,
        }
        
        if assistant_llm_settings.response_format is not None:
            rjs = assistant_llm_settings.response_format.json_schema
            kwargs["format"] = rjs.schema

        stream = await self._client.chat(**kwargs)
        
        async for chunk in stream:
            msg = chunk.message
            if getattr(msg, "content", None):
                yield StreamChunkFromModel(text=msg.content)
            
            if chunk.done:
                usage = None
                if getattr(chunk, "prompt_eval_count", None) is not None and getattr(chunk, "eval_count", None) is not None:
                    usage = UsageMetadata(
                        prompt_token_count=chunk.prompt_eval_count,
                        candidates_token_count=chunk.eval_count,
                        total_token_count=chunk.prompt_eval_count + chunk.eval_count,
                    )
                yield StreamChunkFromModel(finish_reason=chunk.done_reason or "stop", usage_metadata=usage)

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_messages(contents, assistant_llm_settings.system_instruction)

        effective_temperature = assistant_llm_settings.temperature if assistant_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = assistant_llm_settings.max_output_tokens if assistant_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        options = {
            "temperature": effective_temperature,
            "num_predict": effective_max_tokens,
        }
        if assistant_llm_settings.top_p_nucleus_sampling is not None:
            options["top_p"] = assistant_llm_settings.top_p_nucleus_sampling
        if assistant_llm_settings.stop_sequences:
            options["stop"] = assistant_llm_settings.stop_sequences
            
        kwargs = {
            "model": model,
            "messages": messages,
            "options": options,
            "stream": False,
            "think": False,
        }
        
        if assistant_llm_settings.response_format is not None:
            rjs = assistant_llm_settings.response_format.json_schema
            kwargs["format"] = rjs.schema

        response = await self._client.chat(**kwargs)
        
        parts = []
        if getattr(response.message, "content", None):
            parts.append(Part(text=response.message.content))
                
        usage = None
        if getattr(response, "prompt_eval_count", None) is not None and getattr(response, "eval_count", None) is not None:
            usage = UsageMetadata(
                prompt_token_count=response.prompt_eval_count,
                candidates_token_count=response.eval_count,
                total_token_count=response.prompt_eval_count + response.eval_count,
            )

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=response.done_reason or "stop",
            )],
            usage_metadata=usage,
        )

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_messages(contents, lite_llm_settings.system_instruction)

        effective_temperature = lite_llm_settings.temperature if lite_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = lite_llm_settings.max_output_tokens if lite_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        options = {
            "temperature": effective_temperature,
            "num_predict": effective_max_tokens,
        }
        if lite_llm_settings.top_p_nucleus_sampling is not None:
            options["top_p"] = lite_llm_settings.top_p_nucleus_sampling
        if lite_llm_settings.stop_sequences:
            options["stop"] = lite_llm_settings.stop_sequences
            
        kwargs = {
            "model": model,
            "messages": messages,
            "options": options,
            "stream": True,
            "think": False,
        }
        
        if lite_llm_settings.response_format is not None:
            rjs = lite_llm_settings.response_format.json_schema
            kwargs["format"] = rjs.schema

        stream = await self._client.chat(**kwargs)
        
        async for chunk in stream:
            msg = chunk.message
            if getattr(msg, "content", None):
                yield StreamChunkFromModel(text=msg.content)
            
            if chunk.done:
                usage = None
                if getattr(chunk, "prompt_eval_count", None) is not None and getattr(chunk, "eval_count", None) is not None:
                    usage = UsageMetadata(
                        prompt_token_count=chunk.prompt_eval_count,
                        candidates_token_count=chunk.eval_count,
                        total_token_count=chunk.prompt_eval_count + chunk.eval_count,
                    )
                yield StreamChunkFromModel(finish_reason=chunk.done_reason or "stop", usage_metadata=usage)

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        messages = _contents_to_messages(contents, lite_llm_settings.system_instruction)

        effective_temperature = lite_llm_settings.temperature if lite_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = lite_llm_settings.max_output_tokens if lite_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS
        options = {
            "temperature": effective_temperature,
            "num_predict": effective_max_tokens,
        }
        if lite_llm_settings.top_p_nucleus_sampling is not None:
            options["top_p"] = lite_llm_settings.top_p_nucleus_sampling
        if lite_llm_settings.stop_sequences:
            options["stop"] = lite_llm_settings.stop_sequences
            
        kwargs = {
            "model": model,
            "messages": messages,
            "options": options,
            "stream": False,
            "think": False,
        }
        
        if lite_llm_settings.response_format is not None:
            rjs = lite_llm_settings.response_format.json_schema
            kwargs["format"] = rjs.schema

        response = await self._client.chat(**kwargs)
        
        parts = []
        if getattr(response.message, "content", None):
            parts.append(Part(text=response.message.content))
                
        usage = None
        if getattr(response, "prompt_eval_count", None) is not None and getattr(response, "eval_count", None) is not None:
            usage = UsageMetadata(
                prompt_token_count=response.prompt_eval_count,
                candidates_token_count=response.eval_count,
                total_token_count=response.prompt_eval_count + response.eval_count,
            )

        return GenerateContentResponse(
            candidates=[Candidate(
                content=Content(role="model", parts=parts),
                finish_reason=response.done_reason or "stop",
            )],
            usage_metadata=usage,
        )
