import json
import logging
from collections.abc import AsyncGenerator

from ollama import AsyncClient, Message as OllamaMessage

from app.constants import (
    LLM_DEFAULT_TEMPERATURE,
    LLM_DEFAULT_MAX_OUTPUT_TOKENS,
    LLM_OLLAMA_DEFAULT_NUM_CTX,
    ThinkingLevel,
)
from app.llm.thinking import translate_for_ollama
from app.models.model_configs import get_model_config
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
from ..utils import schema_to_dict
from ._capability import translate_capability_error

logger = logging.getLogger(__name__)

def _contents_to_messages(
    contents: list[Content],
    system_instructions: str,
) -> list[OllamaMessage]:
    messages = []

    if system_instructions:
        messages.append(OllamaMessage(role="system", content=system_instructions))

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


def _warn_on_empty_content(
    response,
    *,
    model: str,
    channel: str,
    num_ctx: int,
    num_predict: int,
) -> None:
    """Emit a diagnostic WARNING when Ollama returns HTTP 200 with no content.

    Context-window overflow, load failures, and thinking-only output all surface
    as ``message.content == ""`` with a 200 response. Surfacing the done_reason,
    token counts, and configured num_ctx/num_predict makes the root cause
    obvious instead of leaving callers with a generic "empty response" error.
    """
    message = getattr(response, "message", None)
    content = getattr(message, "content", None) if message else None
    if content:
        return

    done_reason = getattr(response, "done_reason", None)
    prompt_eval_count = getattr(response, "prompt_eval_count", None)
    eval_count = getattr(response, "eval_count", None)
    thinking = getattr(message, "thinking", None) if message else None
    thinking_len = len(thinking) if thinking else 0
    tool_calls = getattr(message, "tool_calls", None) if message else None
    tool_calls_count = len(tool_calls) if tool_calls else 0

    ctx_overflow_suspected = (
        prompt_eval_count is not None
        and num_ctx is not None
        and prompt_eval_count >= num_ctx
    )

    logger.warning(
        "[OLLAMA] Empty message.content on 200 OK: channel=%s model=%s "
        "done_reason=%s prompt_eval_count=%s eval_count=%s num_ctx=%s "
        "num_predict=%s thinking_chars=%d tool_calls=%d ctx_overflow_suspected=%s",
        channel, model, done_reason, prompt_eval_count, eval_count,
        num_ctx, num_predict, thinking_len, tool_calls_count,
        ctx_overflow_suspected,
    )


def _normalize_ollama_host(endpoint: str) -> str:
    """Normalize a user-supplied Ollama host into a base URL.

    Accepts any of:
      - "host:port"              -> "http://host:port"
      - "http://host:port"       -> "http://host:port"
      - "http://host:port/v1"    -> "http://host:port"   (legacy)
      - "http://host:port/v1/v1" -> "http://host:port"   (legacy double-append)

    The Ollama native API lives at /api/chat, not /v1; any /v1 path suffix is
    stripped so httpx base_url joining produces the correct URL.
    """
    cleaned = (endpoint or "").strip().rstrip('/')
    while cleaned.endswith('/v1'):
        cleaned = cleaned[:-3].rstrip('/')
    if cleaned and not cleaned.startswith(('http://', 'https://')):
        cleaned = 'http://' + cleaned
    return cleaned


class OllamaProvider(LLMProvider):
    def __init__(self, endpoint: str, api_key: str):
        super().__init__()

        host = _normalize_ollama_host(endpoint)
        self._client = AsyncClient(host=host)
        logger.info("Ollama provider initialized: %s", host)

    async def _close_resources(self):
        """Clean up provider resources."""
        if hasattr(self._client, 'close'):
            await self._client.close()

    @staticmethod
    def _build_primary_chat_kwargs(
        *,
        model: str,
        messages: list,
        effective_temperature: float,
        effective_max_tokens: int,
        top_p: float | None,
        stop: list[str] | None,
        thinking_config,
        ollama_tools: list[dict] | None,
        stream: bool,
    ) -> dict:
        """Assemble chat() kwargs for primary calls.

        Thinking support varies by model family; translate_for_ollama consults
        LLMModelConfig.thinking_dialect to decide whether to send the ``think``
        kwarg at all, and if so whether to set it True or False based on the
        requested ThinkingLevel. Models with dialect=NONE never see ``think``.
        """
        kwargs: dict = {
            "model": model,
            "messages": messages,
            "options": {
                "temperature": effective_temperature,
                "num_predict": effective_max_tokens,
                "num_ctx": LLM_OLLAMA_DEFAULT_NUM_CTX,
                "top_p": top_p,
                "stop": stop,
            },
            "stream": stream,
            "tools": ollama_tools or None,
        }

        OllamaProvider._apply_think_kwarg(kwargs, model, thinking_config)
        return kwargs

    @staticmethod
    def _apply_think_kwarg(kwargs: dict, model: str, thinking_config) -> None:
        """Apply the Ollama ``think`` kwarg according to the model's dialect.

        Used by both primary (opt-in thinking) and assistant/lite (always-off
        thinking) paths. Assistant and lite calls pass thinking_config=None;
        we synthesize an OFF translation so dialect=NONE models still omit
        the kwarg entirely while NATIVE_TOGGLE models receive ``think=False``
        — matching the contract enforced on the primary path.
        """
        if thinking_config is not None:
            level = thinking_config.thinking_level
        else:
            level = ThinkingLevel.OFF
        translation = translate_for_ollama(level, get_model_config(model))
        if translation.think is not None:
            kwargs["think"] = translation.think

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        messages = _contents_to_messages(contents, primary_llm_settings.system_instructions)
        ollama_tools = _tools_to_ollama(primary_llm_settings.tools)

        effective_temperature = primary_llm_settings.temperature if primary_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = primary_llm_settings.max_output_tokens if primary_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS

        chat_kwargs = self._build_primary_chat_kwargs(
            model=model,
            messages=messages,
            effective_temperature=effective_temperature,
            effective_max_tokens=effective_max_tokens,
            top_p=primary_llm_settings.top_p_nucleus_sampling,
            stop=primary_llm_settings.stop_sequences,
            thinking_config=primary_llm_settings.thinking_config,
            ollama_tools=ollama_tools,
            stream=True,
        )
        try:
            stream = await self._client.chat(**chat_kwargs)
        except Exception as e:
            translate_capability_error(
                e,
                service_name="ollama",
                model=model,
                thinking_requested=bool(
                    primary_llm_settings.thinking_config
                    and primary_llm_settings.thinking_config.enabled
                ),
                tools_requested=bool(primary_llm_settings.tools),
            )
            raise

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
        messages = _contents_to_messages(contents, primary_llm_settings.system_instructions)
        ollama_tools = _tools_to_ollama(primary_llm_settings.tools)

        effective_temperature = primary_llm_settings.temperature if primary_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = primary_llm_settings.max_output_tokens if primary_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS

        chat_kwargs = self._build_primary_chat_kwargs(
            model=model,
            messages=messages,
            effective_temperature=effective_temperature,
            effective_max_tokens=effective_max_tokens,
            top_p=primary_llm_settings.top_p_nucleus_sampling,
            stop=primary_llm_settings.stop_sequences,
            thinking_config=primary_llm_settings.thinking_config,
            ollama_tools=ollama_tools,
            stream=False,
        )
        try:
            response = await self._client.chat(**chat_kwargs)
        except Exception as e:
            translate_capability_error(
                e,
                service_name="ollama",
                model=model,
                thinking_requested=bool(
                    primary_llm_settings.thinking_config
                    and primary_llm_settings.thinking_config.enabled
                ),
                tools_requested=bool(primary_llm_settings.tools),
            )
            raise

        _warn_on_empty_content(
            response,
            model=model,
            channel="primary",
            num_ctx=LLM_OLLAMA_DEFAULT_NUM_CTX,
            num_predict=effective_max_tokens,
        )

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
        messages = _contents_to_messages(contents, assistant_llm_settings.system_instructions)

        effective_temperature = assistant_llm_settings.temperature if assistant_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = assistant_llm_settings.max_output_tokens if assistant_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS

        chat_kwargs: dict = {
            "model": model,
            "messages": messages,
            "options": {
                "temperature": effective_temperature,
                "num_predict": effective_max_tokens,
                "num_ctx": LLM_OLLAMA_DEFAULT_NUM_CTX,
                "top_p": assistant_llm_settings.top_p_nucleus_sampling,
                "stop": assistant_llm_settings.stop_sequences,
            },
            "stream": True,
            "format": assistant_llm_settings.response_format.flatten_for_ollama() if assistant_llm_settings.response_format else None,
        }
        self._apply_think_kwarg(chat_kwargs, model, None)
        stream = await self._client.chat(**chat_kwargs)
        
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
        messages = _contents_to_messages(contents, assistant_llm_settings.system_instructions)

        effective_temperature = assistant_llm_settings.temperature if assistant_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = assistant_llm_settings.max_output_tokens if assistant_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS

        chat_kwargs: dict = {
            "model": model,
            "messages": messages,
            "options": {
                "temperature": effective_temperature,
                "num_predict": effective_max_tokens,
                "num_ctx": LLM_OLLAMA_DEFAULT_NUM_CTX,
                "top_p": assistant_llm_settings.top_p_nucleus_sampling,
                "stop": assistant_llm_settings.stop_sequences,
            },
            "stream": False,
            "format": assistant_llm_settings.response_format.flatten_for_ollama() if assistant_llm_settings.response_format else None,
        }
        self._apply_think_kwarg(chat_kwargs, model, None)
        response = await self._client.chat(**chat_kwargs)

        _warn_on_empty_content(
            response,
            model=model,
            channel="assistant",
            num_ctx=LLM_OLLAMA_DEFAULT_NUM_CTX,
            num_predict=effective_max_tokens,
        )

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
        messages = _contents_to_messages(contents, lite_llm_settings.system_instructions)

        effective_temperature = lite_llm_settings.temperature if lite_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = lite_llm_settings.max_output_tokens if lite_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS

        chat_kwargs: dict = {
            "model": model,
            "messages": messages,
            "options": {
                "temperature": effective_temperature,
                "num_predict": effective_max_tokens,
                "num_ctx": LLM_OLLAMA_DEFAULT_NUM_CTX,
                "top_p": lite_llm_settings.top_p_nucleus_sampling,
                "stop": lite_llm_settings.stop_sequences,
            },
            "stream": True,
            "format": lite_llm_settings.response_format.flatten_for_ollama() if lite_llm_settings.response_format else None,
        }
        self._apply_think_kwarg(chat_kwargs, model, None)
        stream = await self._client.chat(**chat_kwargs)
        
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
        messages = _contents_to_messages(contents, lite_llm_settings.system_instructions)

        effective_temperature = lite_llm_settings.temperature if lite_llm_settings.temperature is not None else LLM_DEFAULT_TEMPERATURE
        effective_max_tokens = lite_llm_settings.max_output_tokens if lite_llm_settings.max_output_tokens is not None else LLM_DEFAULT_MAX_OUTPUT_TOKENS

        chat_kwargs: dict = {
            "model": model,
            "messages": messages,
            "options": {
                "temperature": effective_temperature,
                "num_predict": effective_max_tokens,
                "num_ctx": LLM_OLLAMA_DEFAULT_NUM_CTX,
                "top_p": lite_llm_settings.top_p_nucleus_sampling,
                "stop": lite_llm_settings.stop_sequences,
            },
            "stream": False,
            "format": lite_llm_settings.response_format.flatten_for_ollama() if lite_llm_settings.response_format else None,
        }
        self._apply_think_kwarg(chat_kwargs, model, None)
        response = await self._client.chat(**chat_kwargs)

        _warn_on_empty_content(
            response,
            model=model,
            channel="lite",
            num_ctx=LLM_OLLAMA_DEFAULT_NUM_CTX,
            num_predict=effective_max_tokens,
        )

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
