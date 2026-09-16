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

from app.llm.llm_dataclasses import (
    Candidate,
    Content,
    GenerateContentResponse,
    Part,
    StreamChunkFromModel,
    ToolCall,
    ToolGroup,
    UsageMetadata,
)
from app.llm.llm_types import (
    AssistantLLMSettings,
    LiteLLMSettings,
    PrimaryLLMSettings,
)
from app.llm.provider import LLMProvider
from app.llm.utils import schema_to_dict
from app.models.http_context import G8eHttpContext
from app.models.internal_api import (
    InferenceDispatchRequest,
    InferenceDispatchResponse,
)
from app.models.model_telemetry import GovernedDispatchEvidence
from g8e.operator.v1.operator_pb2 import (
    INFERENCE_MESSAGE_ROLE_ASSISTANT,
    INFERENCE_MESSAGE_ROLE_SYSTEM,
    INFERENCE_MESSAGE_ROLE_TOOL,
    INFERENCE_MESSAGE_ROLE_USER,
    MODEL_ROLE_ASSISTANT,
    MODEL_ROLE_LITE,
    MODEL_ROLE_PRIMARY,
    ExecutionStatus,
    InferenceMessage,
    InferenceMessagePart,
    InferenceToolCall,
    InferenceToolDeclaration,
    InferenceToolResult,
)

logger = logging.getLogger(__name__)

_ROLE_PRIMARY = MODEL_ROLE_PRIMARY
_ROLE_ASSISTANT = MODEL_ROLE_ASSISTANT
_ROLE_LITE = MODEL_ROLE_LITE


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


def _response_to_usage_metadata(result: InferenceDispatchResponse) -> UsageMetadata:
    """Build UsageMetadata from the dispatch response result."""
    if not result.HasField("result"):
        return UsageMetadata()
    return UsageMetadata(
        prompt_token_count=result.result.prompt_tokens,
        candidates_token_count=result.result.completion_tokens,
        total_token_count=result.result.total_tokens,
        usage_reported=result.result.total_tokens > 0,
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


def _response_to_stream_chunks(
    result: InferenceDispatchResponse,
) -> list[StreamChunkFromModel]:
    chunks: list[StreamChunkFromModel] = []
    for part in _response_parts(result):
        if part.text is not None:
            chunks.append(StreamChunkFromModel(text=part.text))
        elif part.tool_call is not None:
            chunks.append(StreamChunkFromModel(tool_calls=[part.tool_call]))
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

    @property
    def governed_dispatch_evidence(self) -> GovernedDispatchEvidence | None:
        return self._governed_dispatch_evidence.get()

    def set_g8e_context(self, context: G8eHttpContext | None) -> None:
        """Store the turn's HTTP context so governed dispatch can propagate
        the case, investigation, task, and session identities onto the
        ``InferenceDispatchRequest``."""
        self._g8e_context.set(context)

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
        tools: list[ToolGroup] | None = None,
    ) -> InferenceDispatchResponse:
        """Dispatch a governed inference request and return the response."""
        self._governed_dispatch_evidence.set(None)
        context = self._g8e_context.get()
        request = InferenceDispatchRequest(
            role=role,
            messages=_contents_to_messages(contents, system_instructions, model=model),
            tools=_tools_to_declarations(tools),
            model=model or "",
            max_tokens=max_output_tokens,
            case_id=(context.case_id or "") if context else "",
            investigation_id=(context.investigation_id or "") if context else "",
            task_id=(context.task_id or "") if context else "",
            web_session_id=(context.web_session_id or "") if context else "",
            cli_session_id=(context.cli_session_id or "") if context else "",
        )
        self._record_model_boundary(request)
        response = await self._client.dispatch_inference(request)
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
            )
        )
        return response

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
            primary_llm_settings.tools,
        )
        for chunk in _response_to_stream_chunks(result):
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
            primary_llm_settings.tools,
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
        for chunk in _response_to_stream_chunks(result):
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
        for chunk in _response_to_stream_chunks(result):
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
        )
        return _response_to_generate_content(result)
