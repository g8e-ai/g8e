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

The dispatch endpoint returns the complete ``InferenceResult`` after the Inference Node finishes generation. Ordered conversation turns, tool declarations, tool calls, and tool results cross the governed path without prompt flattening. Inline-data parts remain unsupported and fail closed with a typed ``ModelCapabilityError``.
"""

from __future__ import annotations

import json
import logging
from collections.abc import AsyncGenerator
from contextvars import ContextVar
from uuid import uuid4

from app.constants import LLM_OLLAMA_DEFAULT_NUM_CTX, ThinkingLevel
from app.llm.llm_dataclasses import (
    Candidate,
    Content,
    GenerateContentResponse,
    Part,
    ResponseFormat,
    StreamChunkFromModel,
    ToolCall,
    ToolConfig,
    ToolGroup,
    UsageMetadata,
)
from app.llm.llm_types import (
    AssistantLLMSettings,
    LiteLLMSettings,
    PrimaryLLMSettings,
    ThinkingConfig,
)
from app.llm.provider import LLMProvider
from app.llm.thinking import translate_for_ollama
from app.llm.utils import schema_to_dict
from app.models.model_configs import get_model_config
from app.models.http_context import G8eHttpContext
from app.models.internal_api import (
    InferenceDispatchRequest,
    InferenceDispatchResponse,
)
from app.models.model_telemetry import GovernedDispatchEvidence
from g8e.constants import PLATFORM
from g8e.operator.v1.operator_pb2 import (
    EXECUTION_STATUS_COMPLETED,
    INFERENCE_MESSAGE_ROLE_ASSISTANT,
    INFERENCE_MESSAGE_ROLE_SYSTEM,
    INFERENCE_MESSAGE_ROLE_TOOL,
    INFERENCE_MESSAGE_ROLE_USER,
    INFERENCE_TOOL_CHOICE_MODE_AUTO,
    INFERENCE_TOOL_CHOICE_MODE_NONE,
    INFERENCE_TOOL_CHOICE_MODE_REQUIRED,
    MODEL_ROLE_ASSISTANT,
    MODEL_ROLE_LITE,
    MODEL_ROLE_PRIMARY,
    ExecutionStatus,
    InferenceMessage,
    InferenceMessagePart,
    InferenceModelVariant,
    InferenceResponseFormat,
    InferenceThinkingControl,
    InferenceToolCall,
    InferenceToolChoice,
    InferenceToolDeclaration,
    InferenceToolResult,
)

logger = logging.getLogger(__name__)

_ROLE_PRIMARY = MODEL_ROLE_PRIMARY
_ROLE_ASSISTANT = MODEL_ROLE_ASSISTANT
_ROLE_LITE = MODEL_ROLE_LITE
_REQUEST_SCHEMA_VERSION = PLATFORM["platform"]["InferenceRequestSchemaVersion"]["value"]


def _canonical_json(value: object) -> str:
    from app.errors import ValidationError

    try:
        return json.dumps(
            value,
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
    except (TypeError, ValueError) as exc:
        raise ValidationError("Governed inference content is not canonical JSON") from exc


def _contents_to_messages(
    contents: list[Content],
    system_instructions: str | None,
    model: str = "",
) -> list[InferenceMessage]:
    from app.errors import ModelCapabilityError, ValidationError

    messages: list[InferenceMessage] = []
    if system_instructions is not None:
        messages.append(
            InferenceMessage(
                role=INFERENCE_MESSAGE_ROLE_SYSTEM,
                parts=[InferenceMessagePart(text=system_instructions)],
            )
        )
    roles = {
        "system": INFERENCE_MESSAGE_ROLE_SYSTEM,
        "user": INFERENCE_MESSAGE_ROLE_USER,
        "model": INFERENCE_MESSAGE_ROLE_ASSISTANT,
        "assistant": INFERENCE_MESSAGE_ROLE_ASSISTANT,
        "tool": INFERENCE_MESSAGE_ROLE_TOOL,
    }
    for content in contents:
        if content.role not in roles:
            raise ValidationError(f"Unsupported governed inference message role: {content.role}")
        parts: list[InferenceMessagePart] = []
        for part in content.parts:
            kinds = sum(
                value is not None
                for value in (part.text, part.tool_call, part.tool_response, part.inline_data)
            )
            if kinds != 1:
                raise ValidationError(
                    "Governed inference message parts must contain exactly one value"
                )
            if part.inline_data is not None:
                raise ModelCapabilityError(
                    "G8E governed dispatch does not support inline data content parts",
                    model=model,
                    capability="multimodal",
                    service_name="g8e",
                )
            if part.tool_call is not None:
                if content.role not in ("model", "assistant") or not part.tool_call.name:
                    raise ValidationError(
                        "Governed inference tool calls require an assistant role and name"
                    )
                parts.append(
                    InferenceMessagePart(
                        tool_call=InferenceToolCall(
                            call_id=part.tool_call.id or "",
                            name=part.tool_call.name,
                            arguments_json=_canonical_json(part.tool_call.args),
                        )
                    )
                )
            elif part.tool_response is not None:
                if content.role != "tool" or not part.tool_response.name:
                    raise ValidationError(
                        "Governed inference tool results require a tool role and name"
                    )
                parts.append(
                    InferenceMessagePart(
                        tool_result=InferenceToolResult(
                            call_id=part.tool_response.id or "",
                            name=part.tool_response.name,
                            result_json=_canonical_json(part.tool_response.response),
                        )
                    )
                )
            elif part.text is not None:
                if content.role == "tool":
                    raise ValidationError("Governed inference tool messages require a tool result")
                parts.append(InferenceMessagePart(text=part.text))
        if not parts:
            raise ValidationError("Governed inference messages require at least one part")
        messages.append(InferenceMessage(role=roles[content.role], parts=parts))
    return messages


def _tools_to_declarations(tools: list[ToolGroup] | None) -> list[InferenceToolDeclaration]:
    declarations: list[InferenceToolDeclaration] = []
    for group in tools or []:
        for tool in group.tools:
            schema = schema_to_dict(tool.parameters) if tool.parameters else {}
            declarations.append(
                InferenceToolDeclaration(
                    name=tool.name,
                    description=tool.description,
                    json_schema=_canonical_json(schema),
                )
            )
    return declarations


def _response_format(format_value: ResponseFormat | None) -> InferenceResponseFormat | None:
    if format_value is None:
        return None
    from app.llm.llm_schema import inline_json_schema_refs

    return InferenceResponseFormat(
        media_type="application/json",
        json_schema=_canonical_json(
            inline_json_schema_refs(format_value.json_schema.json_schema_dict)
        ),
    )


def _tool_choice(config: ToolConfig | None) -> InferenceToolChoice | None:
    if config is None:
        return None
    from app.errors import ValidationError

    mode_name = config.tool_calling_config.mode.upper()
    modes = {
        "AUTO": INFERENCE_TOOL_CHOICE_MODE_AUTO,
        "NONE": INFERENCE_TOOL_CHOICE_MODE_NONE,
        "ANY": INFERENCE_TOOL_CHOICE_MODE_REQUIRED,
        "REQUIRED": INFERENCE_TOOL_CHOICE_MODE_REQUIRED,
    }
    if mode_name not in modes:
        raise ValidationError(f"Unsupported governed inference tool-choice mode: {mode_name}")
    return InferenceToolChoice(
        mode=modes[mode_name],
        allowed_tool_names=config.tool_calling_config.allowed_tool_names,
    )


def _thinking_control(model: str, config: ThinkingConfig | None) -> InferenceThinkingControl | None:
    level = config.thinking_level if config is not None else ThinkingLevel.OFF
    translation = translate_for_ollama(level, get_model_config(model))
    if translation.think is None:
        return None
    return InferenceThinkingControl(
        enabled=translation.think,
        include_thoughts=config.include_thoughts if config is not None else False,
    )


def _apply_evaluation_context(
    request: InferenceDispatchRequest, context: G8eHttpContext | None, model: str
) -> None:
    if context is None or context.evaluation_context is None:
        return
    from app.errors import ValidationError

    evaluation = context.evaluation_context
    matches = [variant for variant in evaluation.model_registry if variant.model == model]
    if len(matches) != 1:
        raise ValidationError(
            "Governed inference model is not uniquely bound in the evaluation registry"
        )
    request.model_digest = matches[0].digest
    request.campaign_id = evaluation.campaign_id
    request.run_id = evaluation.run_id
    request.assignment_id = evaluation.assignment_id
    request.evaluation_attempt_id = evaluation.evaluation_attempt_id
    request.scenario_id = evaluation.scenario_id
    request.model_registry_digest = evaluation.model_registry_digest
    request.target_operator_session_id = evaluation.target_operator_session_id
    request.model_registry.extend(
        InferenceModelVariant(model=variant.model, digest=variant.digest)
        for variant in evaluation.model_registry
    )


def _response_to_usage_metadata(result: InferenceDispatchResponse) -> UsageMetadata:
    """Build UsageMetadata from the dispatch response result."""
    if not result.HasField("result"):
        return UsageMetadata()
    inference_result = result.result
    return UsageMetadata(
        prompt_token_count=inference_result.prompt_tokens,
        candidates_token_count=inference_result.completion_tokens,
        total_token_count=inference_result.total_tokens,
        thinking_token_count=(
            inference_result.thinking_tokens if inference_result.HasField("thinking_tokens") else 0
        ),
        cache_token_count=(
            inference_result.cache_tokens if inference_result.HasField("cache_tokens") else 0
        ),
        usage_reported=inference_result.usage_reported,
        time_to_first_token_seconds=(
            inference_result.time_to_first_token_ns / 1_000_000_000
            if inference_result.HasField("time_to_first_token_ns")
            else None
        ),
        prompt_eval_duration_seconds=(
            inference_result.prompt_eval_duration_ns / 1_000_000_000
            if inference_result.HasField("prompt_eval_duration_ns")
            else None
        ),
        eval_duration_seconds=(
            inference_result.generation_duration_ns / 1_000_000_000
            if inference_result.HasField("generation_duration_ns")
            else None
        ),
        total_duration_seconds=(
            inference_result.total_duration_ns / 1_000_000_000
            if inference_result.HasField("total_duration_ns")
            else None
        ),
        load_duration_seconds=(
            inference_result.load_duration_ns / 1_000_000_000
            if inference_result.HasField("load_duration_ns")
            else None
        ),
    )


def _response_parts(result: InferenceDispatchResponse) -> list[Part]:
    from app.errors import ValidationError

    if not result.HasField("result"):
        raise ValidationError("Governed inference response is missing its result")
    parts: list[Part] = []
    for response_part in result.result.parts:
        kind = response_part.WhichOneof("part")
        if kind == "text":
            parts.append(Part(text=response_part.text))
        elif kind == "thinking":
            parts.append(Part(text=response_part.thinking, thought=True))
        elif kind == "tool_call":
            try:
                arguments = json.loads(response_part.tool_call.arguments_json)
            except json.JSONDecodeError as exc:
                raise ValidationError(
                    "Governed inference tool-call arguments are invalid JSON"
                ) from exc
            if (
                not isinstance(arguments, dict)
                or not response_part.tool_call.name
                or _canonical_json(arguments) != response_part.tool_call.arguments_json
            ):
                raise ValidationError("Governed inference tool-call arguments are invalid")
            parts.append(
                Part(
                    tool_call=ToolCall(
                        name=response_part.tool_call.name,
                        args=arguments,
                        id=response_part.tool_call.call_id or None,
                    )
                )
            )
        else:
            raise ValidationError("Governed inference response contains an unspecified part")
    if not parts:
        raise ValidationError("Governed inference response contains no parts")
    return parts


def _validate_response_identity(
    request: InferenceDispatchRequest, response: InferenceDispatchResponse
) -> None:
    from app.errors import ValidationError

    if not response.HasField("result"):
        raise ValidationError("Governed inference response is missing its result")
    result = response.result
    hashes = (result.normalized_request_hash, result.output_hash, result.result_digest)
    if (
        not response.HasField("receipt")
        or response.receipt.status != EXECUTION_STATUS_COMPLETED
        or response.transaction_id != response.receipt.transaction_id
        or response.receipt.result_summary != result.result_digest
        or result.provider_attempt_id != request.provider_attempt_id
        or (
            request.model
            and (result.requested_model != request.model or result.model != request.model)
        )
        or result.requested_model_digest != request.model_digest
        or result.served_model_digest != request.model_digest
        or result.campaign_id != request.campaign_id
        or result.run_id != request.run_id
        or result.assignment_id != request.assignment_id
        or result.evaluation_attempt_id != request.evaluation_attempt_id
        or result.scenario_id != request.scenario_id
        or result.model_registry_digest != request.model_registry_digest
        or result.retry_count != request.retry_count
        or any(len(value) != 64 or value.lower() != value for value in hashes)
        or any(any(char not in "0123456789abcdef" for char in value) for value in hashes)
        or (request.model_digest and result.served_model_digest != request.model_digest)
    ):
        raise ValidationError("Governed inference response identity binding is invalid")


def _response_to_generate_content(
    result: InferenceDispatchResponse,
) -> GenerateContentResponse:
    finish_reason = result.result.finish_reason if result.HasField("result") else "stop"
    return GenerateContentResponse(
        candidates=[
            Candidate(
                content=Content(role="model", parts=_response_parts(result)),
                finish_reason=finish_reason or "stop",
            )
        ],
        usage_metadata=_response_to_usage_metadata(result),
    )


def _part_to_stream_chunk(part: Part) -> StreamChunkFromModel:
    if part.tool_call is not None:
        return StreamChunkFromModel(tool_calls=[part.tool_call])
    return StreamChunkFromModel(text=part.text, thought=part.thought)


def _progress_parts_to_stream_chunks(progress) -> list[StreamChunkFromModel]:
    from app.errors import ValidationError

    chunks: list[StreamChunkFromModel] = []
    for response_part in progress.parts:
        kind = response_part.WhichOneof("part")
        if kind == "text":
            chunks.append(StreamChunkFromModel(text=response_part.text))
        elif kind == "thinking":
            chunks.append(StreamChunkFromModel(text=response_part.thinking, thought=True))
        elif kind == "tool_call":
            try:
                arguments = json.loads(response_part.tool_call.arguments_json)
            except json.JSONDecodeError as exc:
                raise ValidationError(
                    "Governed inference tool-call arguments are invalid JSON"
                ) from exc
            if (
                not isinstance(arguments, dict)
                or not response_part.tool_call.name
                or _canonical_json(arguments) != response_part.tool_call.arguments_json
            ):
                raise ValidationError("Governed inference tool-call arguments are invalid")
            chunks.append(
                StreamChunkFromModel(
                    tool_calls=[
                        ToolCall(
                            name=response_part.tool_call.name,
                            args=arguments,
                            id=response_part.tool_call.call_id or None,
                        )
                    ]
                )
            )
        else:
            raise ValidationError("Governed inference progress contains an unspecified part")
    return chunks


def _response_to_stream_chunks(
    result: InferenceDispatchResponse,
) -> list[StreamChunkFromModel]:
    chunks: list[StreamChunkFromModel] = []
    for part in _response_parts(result):
        chunks.append(_part_to_stream_chunk(part))
    chunks.append(
        StreamChunkFromModel(
            finish_reason=result.result.finish_reason if result.HasField("result") else "stop",
            usage_metadata=_response_to_usage_metadata(result),
        )
    )
    return chunks


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
        self._provider_retry_count: ContextVar[int] = ContextVar(
            f"{type(self).__name__}_provider_retry_count_{id(self)}", default=0
        )

    @property
    def governed_dispatch_evidence(self) -> GovernedDispatchEvidence | None:
        return self._governed_dispatch_evidence.get()

    def set_g8e_context(self, context: G8eHttpContext | None) -> None:
        """Store the turn's HTTP context so governed dispatch can propagate
        the case, investigation, task, and session identities onto the
        ``InferenceDispatchRequest``."""
        self._g8e_context.set(context)

    def set_provider_retry_count(self, retry_count: int) -> None:
        self._provider_retry_count.set(retry_count)

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
        top_p: float | None,
        top_k: int | None,
        random_seed: int | None,
        stop_sequences: list[str] | None,
        response_format: ResponseFormat | None,
        tool_config: ToolConfig | None,
        parallel_tool_calls: bool | None,
        thinking_config: ThinkingConfig | None,
        tools: list[ToolGroup] | None = None,
    ) -> InferenceDispatchResponse:
        """Dispatch a governed inference request and return the response."""
        self._governed_dispatch_evidence.set(None)
        context = self._g8e_context.get()
        retry_count = self._provider_retry_count.get()
        request = InferenceDispatchRequest(
            role=role,
            messages=_contents_to_messages(contents, system_instructions, model=model),
            tools=_tools_to_declarations(tools),
            model=model or "",
            max_tokens=max_output_tokens,
            stop_sequences=stop_sequences or [],
            request_schema_version=_REQUEST_SCHEMA_VERSION,
            context_limit=LLM_OLLAMA_DEFAULT_NUM_CTX,
            provider_attempt_id=str(uuid4()),
            retry_count=retry_count,
            case_id=(context.case_id or "") if context else "",
            investigation_id=(context.investigation_id or "") if context else "",
            task_id=(context.task_id or "") if context else "",
            web_session_id=(context.web_session_id or "") if context else "",
            cli_session_id=(context.cli_session_id or "") if context else "",
        )
        _apply_evaluation_context(request, context, model)
        if top_p is not None:
            request.top_p = top_p
        if top_k is not None:
            request.top_k = top_k
        if random_seed is not None:
            request.seed = random_seed
        if parallel_tool_calls is not None:
            request.parallel_tool_calls = parallel_tool_calls
        normalized_tool_choice = _tool_choice(tool_config)
        if normalized_tool_choice is not None:
            request.tool_choice.CopyFrom(normalized_tool_choice)
        normalized_thinking = _thinking_control(model, thinking_config)
        if normalized_thinking is not None:
            request.thinking.CopyFrom(normalized_thinking)
        normalized_response_format = _response_format(response_format)
        if normalized_response_format is not None:
            request.response_format.CopyFrom(normalized_response_format)
        self._record_model_boundary(request)
        response = await self._client.dispatch_inference(request)
        _validate_response_identity(request, response)
        _response_parts(response)
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
                provider_attempt_id=response.result.provider_attempt_id,
                requested_model=response.result.requested_model,
                served_model=response.result.model,
                model_digest=response.result.served_model_digest,
                normalized_request_hash=response.result.normalized_request_hash,
                output_hash=response.result.output_hash,
                campaign_id=response.result.campaign_id,
                run_id=response.result.run_id,
                assignment_id=response.result.assignment_id,
                evaluation_attempt_id=response.result.evaluation_attempt_id,
                scenario_id=response.result.scenario_id,
                model_registry_digest=response.result.model_registry_digest,
            )
        )
        return response

    async def _dispatch_stream(
        self,
        role: int,
        model: str,
        contents: list[Content],
        system_instructions: str | None,
        max_output_tokens: int,
        top_p: float | None,
        top_k: int | None,
        random_seed: int | None,
        stop_sequences: list[str] | None,
        response_format: ResponseFormat | None,
        tool_config: ToolConfig | None,
        parallel_tool_calls: bool | None,
        thinking_config: ThinkingConfig | None,
        tools: list[ToolGroup] | None = None,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        from app.errors import ValidationError

        self._governed_dispatch_evidence.set(None)
        context = self._g8e_context.get()
        retry_count = self._provider_retry_count.get()
        request = InferenceDispatchRequest(
            role=role,
            messages=_contents_to_messages(contents, system_instructions, model=model),
            tools=_tools_to_declarations(tools),
            model=model or "",
            max_tokens=max_output_tokens,
            stop_sequences=stop_sequences or [],
            request_schema_version=_REQUEST_SCHEMA_VERSION,
            context_limit=LLM_OLLAMA_DEFAULT_NUM_CTX,
            provider_attempt_id=str(uuid4()),
            retry_count=retry_count,
            stream=True,
            case_id=(context.case_id or "") if context else "",
            investigation_id=(context.investigation_id or "") if context else "",
            task_id=(context.task_id or "") if context else "",
            web_session_id=(context.web_session_id or "") if context else "",
            cli_session_id=(context.cli_session_id or "") if context else "",
        )
        _apply_evaluation_context(request, context, model)
        if top_p is not None:
            request.top_p = top_p
        if top_k is not None:
            request.top_k = top_k
        if random_seed is not None:
            request.seed = random_seed
        if parallel_tool_calls is not None:
            request.parallel_tool_calls = parallel_tool_calls
        normalized_tool_choice = _tool_choice(tool_config)
        if normalized_tool_choice is not None:
            request.tool_choice.CopyFrom(normalized_tool_choice)
        normalized_thinking = _thinking_control(model, thinking_config)
        if normalized_thinking is not None:
            request.thinking.CopyFrom(normalized_thinking)
        normalized_response_format = _response_format(response_format)
        if normalized_response_format is not None:
            request.response_format.CopyFrom(normalized_response_format)
        self._record_model_boundary(request)

        completion: InferenceDispatchResponse | None = None
        async for frame in self._client.dispatch_inference_stream(request):
            if frame.HasField("failure"):
                raise ValidationError(frame.failure.reason)
            if frame.HasField("progress"):
                for chunk in _progress_parts_to_stream_chunks(frame.progress):
                    yield chunk
            elif frame.HasField("completion"):
                completion = frame.completion

        if completion is None:
            raise ValidationError("Governed inference stream ended without a completion frame")
        _validate_response_identity(request, completion)
        _response_parts(completion)
        self._governed_dispatch_evidence.set(
            GovernedDispatchEvidence(
                transaction_id=completion.transaction_id,
                result_digest=(
                    completion.result.result_digest if completion.HasField("result") else ""
                ),
                receipt_status=(
                    ExecutionStatus.Name(completion.receipt.status)
                    if completion.HasField("receipt")
                    else ""
                ),
                provider_attempt_id=completion.result.provider_attempt_id,
                requested_model=completion.result.requested_model,
                served_model=completion.result.model,
                model_digest=completion.result.served_model_digest,
                normalized_request_hash=completion.result.normalized_request_hash,
                output_hash=completion.result.output_hash,
                campaign_id=completion.result.campaign_id,
                run_id=completion.result.run_id,
                assignment_id=completion.result.assignment_id,
                evaluation_attempt_id=completion.result.evaluation_attempt_id,
                scenario_id=completion.result.scenario_id,
                model_registry_digest=completion.result.model_registry_digest,
            )
        )
        yield StreamChunkFromModel(
            finish_reason=completion.result.finish_reason
            if completion.HasField("result")
            else "stop",
            usage_metadata=_response_to_usage_metadata(completion),
        )

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        async for chunk in self.generate_content_stream_scored_role(
            "primary",
            model,
            contents,
            primary_llm_settings,
        ):
            yield chunk

    async def generate_content_stream_scored_role(
        self,
        model_role: str,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        role_map = {
            "primary": _ROLE_PRIMARY,
            "assistant": _ROLE_ASSISTANT,
            "lite": _ROLE_LITE,
        }
        if model_role not in role_map:
            from app.errors import ValidationError

            raise ValidationError(f"Unsupported scored model role: {model_role}")
        async for chunk in self._dispatch_stream(
            role_map[model_role],
            model,
            contents,
            primary_llm_settings.system_instructions,
            primary_llm_settings.max_output_tokens,
            primary_llm_settings.top_p_nucleus_sampling,
            primary_llm_settings.top_k_filtering,
            primary_llm_settings.random_seed,
            primary_llm_settings.stop_sequences,
            None,
            primary_llm_settings.tool_config,
            primary_llm_settings.parallel_tool_calls,
            primary_llm_settings.thinking_config,
            primary_llm_settings.tools,
        ):
            yield chunk

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
            primary_llm_settings.top_p_nucleus_sampling,
            primary_llm_settings.top_k_filtering,
            primary_llm_settings.random_seed,
            primary_llm_settings.stop_sequences,
            None,
            primary_llm_settings.tool_config,
            primary_llm_settings.parallel_tool_calls,
            primary_llm_settings.thinking_config,
            primary_llm_settings.tools,
        )
        return _response_to_generate_content(result)

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        async for chunk in self._dispatch_stream(
            _ROLE_ASSISTANT,
            model,
            contents,
            assistant_llm_settings.system_instructions,
            assistant_llm_settings.max_output_tokens,
            assistant_llm_settings.top_p_nucleus_sampling,
            assistant_llm_settings.top_k_filtering,
            assistant_llm_settings.random_seed,
            assistant_llm_settings.stop_sequences,
            assistant_llm_settings.response_format,
            None,
            None,
            None,
        ):
            yield chunk

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
            assistant_llm_settings.top_p_nucleus_sampling,
            assistant_llm_settings.top_k_filtering,
            assistant_llm_settings.random_seed,
            assistant_llm_settings.stop_sequences,
            assistant_llm_settings.response_format,
            None,
            None,
            None,
        )
        return _response_to_generate_content(result)

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        async for chunk in self._dispatch_stream(
            _ROLE_LITE,
            model,
            contents,
            lite_llm_settings.system_instructions,
            lite_llm_settings.max_output_tokens,
            lite_llm_settings.top_p_nucleus_sampling,
            lite_llm_settings.top_k_filtering,
            lite_llm_settings.random_seed,
            lite_llm_settings.stop_sequences,
            lite_llm_settings.response_format,
            None,
            None,
            None,
        ):
            yield chunk

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
            lite_llm_settings.top_p_nucleus_sampling,
            lite_llm_settings.top_k_filtering,
            lite_llm_settings.random_seed,
            lite_llm_settings.stop_sequences,
            lite_llm_settings.response_format,
            None,
            None,
            None,
        )
        return _response_to_generate_content(result)
