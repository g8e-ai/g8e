# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
g8e Agent - orchestrates the ReAct streaming loop.

Concerns handled here:
  - Retry loop with backoff around _stream_with_tool_loop
  - ReAct loop: provider turn -> tool calls -> next turn contents -> repeat
  - SSE delivery via run_with_sse (delegates to agent_sse)

All other concerns live in dedicated modules:
  agent_turn.py          - thinking state machine, stream parsing, parts consolidation,
                           interrogation gate, finish reason normalization, retry classification
  agent_tool_loop.py - tool call dispatch, sequential execution,
                           tool display metadata, grounding merge
  agent_sse.py           - SSE translation and client event delivery
  investigation_service.py - operator context extraction
"""

import asyncio
import logging
import time
from collections.abc import AsyncGenerator, Awaitable, Callable
from dataclasses import dataclass
from typing import Literal, Self

import app.llm.llm_types as types
from app.constants import (
    AGENT_CONTINUE_APPROVAL_TIMEOUT_SECONDS,
    AGENT_MAX_RETRIES,
    AGENT_MAX_TOOL_TURNS,
    AGENT_RETRY_BACKOFF_MULTIPLIER,
    AGENT_RETRY_DELAY_SECONDS,
    DEFAULT_FINISH_REASON,
    AITaskId,
)
from app.errors import ToolsNotSupportedError, ValidationError
from app.llm.model_call_attribution import build_model_call_telemetry, prepare_provider_call
from app.llm.model_evidence import model_boundary_hash, model_boundary_json
from app.llm.provider import LLMProvider
from app.llm.providers.g8e import G8EProvider
from app.models.agent import (
    AgentInputs,
    AgentStreamState,
    StreamChunkData,
    StreamChunkFromModel,
    StreamChunkFromModelType,
    TokenUsage,
    ToolCallResponse,
    TurnResult,
)
from app.models.grounding import GroundingMetadata
from app.models.model_telemetry import ModelCallTelemetry
from app.models.operators import AgentContinueApprovalRequest
from app.services.ai.agent_sse import deliver_via_sse
from app.services.ai.agent_tool_loop import (
    execute_turn_tool_calls,
    merge_grounding,
)
from app.services.ai.agent_turn import (
    GatedTurnResult,
    consolidate_model_parts,
    process_turn_with_gate,
    should_retry_error,
)
from app.services.ai.grounding.grounding_service import GroundingService
from app.services.ai.tool_service import AIToolService
from app.services.evaluation.role_control import resolve_scored_model_role
from app.services.protocols import ApprovalServiceProtocol, EventServiceProtocol
from app.utils.time_ids.ids import generate_command_execution_id

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class _ModelTurnContext:
    inputs: AgentInputs
    provider: LLMProvider
    contents: list[types.Content]
    model_calls: list[ModelCallTelemetry]
    retry_count: int
    model_name: str
    generation_config: types.PrimaryLLMSettings


@dataclass
class _TokenTotals:
    """Running token totals across every model call in one tool loop."""

    input_tokens: int = 0
    output_tokens: int = 0
    total_tokens: int = 0

    @classmethod
    def from_calls(cls, calls: list[ModelCallTelemetry]) -> Self:
        return cls(
            input_tokens=sum(call.input_tokens for call in calls),
            output_tokens=sum(call.output_tokens for call in calls),
            total_tokens=sum(call.total_tokens for call in calls),
        )

    def add(self, turn_result: TurnResult) -> None:
        self.input_tokens += turn_result.input_tokens
        self.output_tokens += turn_result.output_tokens
        self.total_tokens += turn_result.total_tokens


@dataclass
class _TurnResponseContext:
    contents: list[types.Content]
    turn_result: TurnResult
    responses: list[ToolCallResponse]
    grounding_metadata: GroundingMetadata | None
    response_sizes: list[int]


@dataclass
class _ToolTurnContext:
    turn_result: TurnResult
    inputs: AgentInputs
    event_service: EventServiceProtocol
    loop_turn: int


def _sum_optional_usage_counts(values: list[int | None]) -> int | None:
    if not values:
        return None
    if any(value is None for value in values):
        return None
    return sum(value for value in values if value is not None)


def _resolve_agent_model_role(inputs: AgentInputs) -> Literal["primary", "assistant", "lite"]:
    return resolve_scored_model_role(
        designated_model_role=inputs.designated_model_role,
        active_agent=inputs.active_agent,
    )


def _agent_role_for_telemetry(inputs: AgentInputs) -> str:
    """Return the persona a model call is attributed to.

    The chat pipeline always assigns an active agent. A request without one
    would be attributed to a role with no call classification, so it fails
    loudly instead of reporting ``unknown``.
    """
    if inputs.active_agent:
        return inputs.active_agent.value
    raise ValidationError(
        "active_agent is required for every agent model call",
        field="active_agent",
        component="g8ee",
    )


def _agent_generation_stream(
    llm_provider: LLMProvider,
    *,
    inputs: AgentInputs,
    model_name: str,
    contents: list[types.Content],
    generation_config: types.PrimaryLLMSettings,
):
    model_role = _resolve_agent_model_role(inputs)
    if isinstance(llm_provider, G8EProvider):
        return llm_provider.generate_content_stream_scored_role(
            model_role,
            model_name,
            contents,
            generation_config,
        )
    if generation_config.tools:
        # Assistant/lite settings cannot carry tools. Keep the selected model
        # and telemetry role, but use the tool-capable provider entry point.
        return llm_provider.generate_content_stream_primary(
            model=model_name,
            contents=contents,
            primary_llm_settings=generation_config,
        )
    if model_role == "assistant":
        assistant_settings = types.AssistantLLMSettings(
            max_output_tokens=generation_config.max_output_tokens,
            top_p_nucleus_sampling=generation_config.top_p_nucleus_sampling,
            top_k_filtering=generation_config.top_k_filtering,
            stop_sequences=generation_config.stop_sequences,
            system_instructions=generation_config.system_instructions,
        )
        return llm_provider.generate_content_stream_assistant(
            model=model_name,
            contents=contents,
            assistant_llm_settings=assistant_settings,
        )
    if model_role == "lite":
        lite_settings = types.LiteLLMSettings(
            max_output_tokens=generation_config.max_output_tokens,
            top_p_nucleus_sampling=generation_config.top_p_nucleus_sampling,
            top_k_filtering=generation_config.top_k_filtering,
            stop_sequences=generation_config.stop_sequences,
            system_instructions=generation_config.system_instructions,
        )
        return llm_provider.generate_content_stream_lite(
            model=model_name,
            contents=contents,
            lite_llm_settings=lite_settings,
        )
    return llm_provider.generate_content_stream_primary(
        model=model_name,
        contents=contents,
        primary_llm_settings=generation_config,
    )


class G8eEnsemble:
    """
    Unified g8e AI Agent - orchestrates the ReAct streaming loop.

    Usage:
        async for chunk in agent.stream_response(contents, config, model, inputs, ...):
            yield chunk

        await agent.run_with_sse(inputs, state, event_service, llm_provider, ...)
    """

    def __init__(
        self,
        tool_executor: AIToolService,
        grounding_service: GroundingService | None = None,
        approval_service: ApprovalServiceProtocol | None = None,
    ):
        self._tool_executor = tool_executor
        self._grounding_service = grounding_service or GroundingService()
        self._approval_service = approval_service
        logger.info("G8eEnsemble initialized")

    @property
    def tool_executor(self) -> AIToolService:
        return self._tool_executor

    @property
    def g8e_web_search_available(self) -> bool:
        return self._tool_executor.g8e_web_search_available

    async def stream_response(
        self,
        inputs: AgentInputs,
        event_service: EventServiceProtocol,
        llm_provider: LLMProvider,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """
        Stream AI response with full tool support.

        Yields StreamChunkFromModel objects. Callers deliver them via HTTP SSE
        (through run_with_sse) or consume them directly for pub/sub delivery.
        """
        case_id = inputs.case_id
        investigation_id = inputs.investigation_id
        user_id = inputs.user_id
        g8e_context = inputs.g8e_context
        agent_mode = inputs.agent_mode
        contents = inputs.contents
        model_name = inputs.model_to_use

        _bound_count = len(g8e_context.bound_operators) if g8e_context else 0
        logger.info(
            "[AGENT] stream_response start: model=%s investigation_id=%s case_id=%s "
            "workflow=%s bound_operators=%d contents=%d user_id=%s",
            model_name,
            investigation_id,
            case_id,
            agent_mode,
            _bound_count,
            len(contents),
            (user_id[:8] + "...") if user_id else None,
        )

        max_attempts = AGENT_MAX_RETRIES + 1
        backoff_seconds = AGENT_RETRY_DELAY_SECONDS
        attempt = 1
        streaming_started = False
        logger.info(
            "[AGENT] Retry config: max_attempts=%d initial_delay=%.1fs",
            max_attempts,
            backoff_seconds,
        )

        triage_call = inputs.triage_result.model_call if inputs.triage_result else None
        model_calls = [triage_call] if triage_call else []
        while attempt <= max_attempts:
            try:
                # Emit RETRY chunk before retrying (not on first attempt)
                if attempt > 1:
                    logger.info("Retry attempt %d/%d", attempt, max_attempts)
                    yield StreamChunkFromModel(
                        type=StreamChunkFromModelType.RETRY,
                        data=StreamChunkData(attempt=attempt, max_attempts=max_attempts),
                    )

                async for chunk in self._stream_with_tool_loop(
                    inputs=inputs,
                    llm_provider=llm_provider,
                    event_service=event_service,
                    model_calls=model_calls,
                    retry_count=attempt - 1,
                ):
                    # Mark streaming as started as soon as we receive any chunk
                    # This prevents retries after streaming has begun
                    streaming_started = True
                    yield chunk

                return

            except Exception as e:
                can_retry = (
                    attempt < max_attempts and not streaming_started and should_retry_error(e)
                )
                if not can_retry:
                    if streaming_started:
                        logger.error("[AGENT] Fatal error after streaming started: %s", e)
                    yield StreamChunkFromModel(
                        type=StreamChunkFromModelType.ERROR,
                        data=StreamChunkData(
                            error=str(e),
                            model_calls=model_calls,
                            provider_tool_rejection=isinstance(e, ToolsNotSupportedError),
                        ),
                    )
                    return

                logger.warning("[AGENT] Attempt %d failed, retrying: %s", attempt, e)
                await asyncio.sleep(backoff_seconds)
                backoff_seconds *= AGENT_RETRY_BACKOFF_MULTIPLIER
                attempt += 1

    async def run_with_sse(
        self,
        inputs: AgentInputs,
        state: AgentStreamState,
        event_service: EventServiceProtocol,
        llm_provider: LLMProvider,
        on_iteration_text: Callable[[str], Awaitable[None]] | None = None,
    ) -> None:
        """
        SSE chat path - runs stream_response and delivers events to the browser.

        All request-scoped data (contents, generation_config, model_to_use) is
        read from ``inputs`` - it is redundant to pass them separately, and
        doing so creates drift risk where the caller's ``inputs.contents`` and
        the top-level ``contents`` argument could disagree.

        Owns the invocation context lifecycle: starts it before iterating
        stream_response and resets it in finally. This must live here (a normal
        coroutine) rather than inside stream_response (an async generator), because
        Python dispatches async-generator cleanup in a new asyncio Context, which
        makes ContextVar.reset() raise ValueError if the token was created in the
        original request Context.

        Delegates all SSE translation to agent_sse.deliver_via_sse.
        """
        if not inputs.g8e_context:
            raise ValidationError(
                "G8eHttpContext is required for run_with_sse",
                field="g8e_context",
                constraint="required",
            )
        if not inputs.model_to_use:
            raise ValidationError(
                "inputs.model_to_use is required for run_with_sse",
                field="model_to_use",
                constraint="required",
            )
        if inputs.generation_config is None:
            raise ValidationError(
                "inputs.generation_config is required for run_with_sse",
                field="generation_config",
                constraint="required",
            )

        logger.info(
            "[AGENT] run_with_sse: investigation_id=%s case_id=%s model=%s "
            "workflow=%s sentinel_mode=%s contents=%d",
            inputs.investigation_id,
            inputs.case_id,
            inputs.model_to_use,
            inputs.agent_mode,
            inputs.sentinel_mode,
            len(inputs.contents),
        )
        await deliver_via_sse(
            stream=self.stream_response(
                inputs=inputs,
                llm_provider=llm_provider,
                event_service=event_service,
            ),
            inputs=inputs,
            state=state,
            event_service=event_service,
            on_iteration_text=on_iteration_text,
        )

    async def _stream_with_tool_loop(
        self,
        inputs: AgentInputs,
        llm_provider: LLMProvider,
        event_service: EventServiceProtocol,
        model_calls: list[ModelCallTelemetry] | None = None,
        retry_count: int = 0,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """
        ReAct function-calling loop.

        Each iteration:
          1. Calls process_provider_turn - consumes one provider stream,
             drives the thinking state machine, yields chunks, writes TurnResult.
          2. If the turn produced tool calls, calls execute_turn_tool_calls
             - executes them sequentially, yields TOOL_CALL/TOOL_RESULT chunks.
          3. Appends model response + tool responses to contents and loops.
          4. Breaks when the turn produces no tool calls.

        Yields CITATIONS and COMPLETE at the end.
        """
        # Copy contents to avoid mutating inputs.contents (AgentInputs must be immutable)
        contents = list(inputs.contents)
        generation_config = inputs.generation_config
        model_name = inputs.model_to_use

        # These should be validated by run_with_sse, but add assertions for type safety
        assert generation_config is not None, "generation_config must not be None"
        assert model_name is not None, "model_name must not be None"

        if model_calls is None:
            triage_call = inputs.triage_result.model_call if inputs.triage_result else None
            model_calls = [triage_call] if triage_call else []
        totals = _TokenTotals.from_calls(model_calls)
        grounding_metadata: GroundingMetadata | None = None
        final_finish_reason: str = DEFAULT_FINISH_REASON
        tool_response_sizes: list[int] = []

        loop_turn = 0
        malformed_call_retries = 0
        tool_turn_limit_reached = False
        try:
            while True:
                loop_turn += 1
                if loop_turn > AGENT_MAX_TOOL_TURNS:
                    tool_turn_limit_reached = True
                    loop_turn, final_finish_reason, should_stop = await self._resolve_turn_limit(
                        inputs, loop_turn, final_finish_reason
                    )
                    if should_stop:
                        break
                logger.info(
                    "[AGENT] Tool loop turn %d: contents=%d case_id=%s investigation_id=%s",
                    loop_turn,
                    len(contents),
                    inputs.case_id,
                    inputs.investigation_id,
                )

                gated_result_out: list[GatedTurnResult] = []
                turn_context = _ModelTurnContext(
                    inputs=inputs,
                    provider=llm_provider,
                    contents=contents,
                    model_calls=model_calls,
                    retry_count=retry_count,
                    model_name=model_name,
                    generation_config=generation_config,
                )
                async for chunk in self._process_model_turn(turn_context, gated_result_out):
                    yield chunk
                gated = gated_result_out[0]
                turn_result = gated.turn_result

                totals.add(turn_result)
                final_finish_reason = turn_result.finish_reason or final_finish_reason

                malformed_call_retries = self._retry_malformed_call(
                    turn_result.finish_reason,
                    contents,
                    loop_turn,
                    malformed_call_retries,
                )
                if malformed_call_retries is not None:
                    continue
                malformed_call_retries = 0

                if gated.interrogation_detected:
                    logger.info(
                        "[AGENT] Interrogation gate fired at turn=%d, suppressing tool execution",
                        loop_turn,
                    )

                if not turn_result.pending_tool_calls:
                    logger.info(
                        "[AGENT] Tool loop breaking: turn=%d finish_reason=%s "
                        "input_tokens=%d output_tokens=%d total_tokens=%d",
                        loop_turn,
                        turn_result.finish_reason,
                        totals.input_tokens,
                        totals.output_tokens,
                        totals.total_tokens,
                    )
                    break

                fc_responses_out: list[list[ToolCallResponse]] = []
                async for chunk in self._execute_tool_turn(
                    _ToolTurnContext(turn_result, inputs, event_service, loop_turn),
                    fc_responses_out,
                ):
                    yield chunk
                fc_responses = fc_responses_out[0]

                grounding_metadata = self._append_turn_responses(
                    _TurnResponseContext(
                        contents=contents,
                        turn_result=turn_result,
                        responses=fc_responses,
                        grounding_metadata=grounding_metadata,
                        response_sizes=tool_response_sizes,
                    )
                )
        except asyncio.CancelledError:
            logger.info(
                "[AGENT] Tool loop cancelled at turn %d for investigation %s",
                loop_turn,
                inputs.investigation_id,
            )
            raise

        for chunk in self._closing_chunks(
            grounding_metadata,
            final_finish_reason,
            totals,
            model_calls,
            tool_response_sizes,
            tool_turn_limit_reached,
        ):
            yield chunk

    async def _resolve_turn_limit(
        self,
        inputs: AgentInputs,
        loop_turn: int,
        final_finish_reason: str,
    ) -> tuple[int, str, bool]:
        """Decide whether the tool loop may continue after reaching the max turn count.

        Returns ``(loop_turn, final_finish_reason, should_stop)``. A scored run or a
        missing approval service stops immediately; otherwise the operator is asked to
        approve another batch of turns, and approval resets the turn counter.
        """
        if inputs.g8e_context.evaluation_context is not None:
            # A scored run has no human to answer the continue-approval,
            # so the request would stall until it times out. Deny it
            # immediately; the trace records tool_turn_limit_reached.
            logger.warning(
                "[AGENT] Scored run reached max tool turns (%d); stopping without approval",
                AGENT_MAX_TOOL_TURNS,
            )
            return loop_turn, "tool_turn_limit", True
        if self._approval_service is None:
            logger.error(
                "[AGENT] Tool loop exceeded max turns (%d) with no approval service available; aborting",
                AGENT_MAX_TOOL_TURNS,
            )
            return loop_turn, final_finish_reason, True

        logger.warning(
            "[AGENT] Tool loop reached max turns (%d); requesting operator approval to continue",
            AGENT_MAX_TOOL_TURNS,
        )
        justification = (
            f"The AI agent has executed {AGENT_MAX_TOOL_TURNS} tool-use turns "
            f"without completing its response. Approve to reset the turn counter "
            f"and allow the agent to continue; deny to stop the agent now."
        )
        approval_result = await self._approval_service.request_agent_continue_approval(
            AgentContinueApprovalRequest(
                g8e_context=inputs.g8e_context,
                timeout_seconds=AGENT_CONTINUE_APPROVAL_TIMEOUT_SECONDS,
                justification=justification,
                execution_id=generate_command_execution_id(),
                turn_limit=AGENT_MAX_TOOL_TURNS,
                turns_completed=loop_turn - 1,
                task_id=AITaskId.AGENT_CONTINUE.value,
            )
        )
        if not approval_result.approved:
            logger.info(
                "[AGENT] Continuation denied (reason=%s); stopping tool loop",
                approval_result.reason,
            )
            return loop_turn, "stopped_by_operator", True
        logger.info("[AGENT] Continuation approved; resetting turn counter")
        return 1, final_finish_reason, False

    async def _process_model_turn(
        self,
        context: _ModelTurnContext,
        result_out: list[GatedTurnResult],
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream one provider turn and attach its success or failure telemetry."""
        inputs = context.inputs
        provider = context.provider
        model_role = _resolve_agent_model_role(inputs)
        agent_role = _agent_role_for_telemetry(inputs)
        prepare_provider_call(
            provider,
            g8e_context=inputs.g8e_context,
            retry_count=context.retry_count,
        )

        input_artifact_hash = model_boundary_hash(
            {
                "model": context.model_name,
                "contents": context.contents,
                "settings": context.generation_config,
            }
        )
        monotonic_start = time.monotonic()
        try:
            response = _agent_generation_stream(
                provider,
                inputs=inputs,
                model_name=context.model_name,
                contents=context.contents,
                generation_config=context.generation_config,
            )
            async for chunk in process_turn_with_gate(response, result_out):
                yield chunk
        except Exception as exc:
            if hasattr(provider, "record_processing_error"):
                provider.record_processing_error(exc)
            context.model_calls.append(
                build_model_call_telemetry(
                    provider=provider,
                    agent_role=agent_role,
                    model_role=model_role,
                    model=context.model_name,
                    monotonic_start=monotonic_start,
                    input_artifact_hash=input_artifact_hash,
                    retry_count=context.retry_count,
                    succeeded=False,
                    error_type=type(exc).__name__,
                )
            )
            raise

        turn_result = result_out[0].turn_result
        if hasattr(provider, "record_processed_response"):
            provider.record_processed_response(
                model_boundary_json(turn_result.model_response_parts)
            )
        context.model_calls.append(
            build_model_call_telemetry(
                provider=provider,
                agent_role=agent_role,
                model_role=model_role,
                model=context.model_name,
                monotonic_start=monotonic_start,
                monotonic_end=time.monotonic(),
                input_artifact_hash=input_artifact_hash,
                input_tokens=turn_result.input_tokens,
                output_tokens=turn_result.output_tokens,
                thinking_tokens=turn_result.thinking_tokens,
                cache_tokens=turn_result.cache_tokens,
                total_tokens=turn_result.total_tokens,
                usage_reported=turn_result.usage_reported,
                finish_reason=turn_result.finish_reason,
                time_to_first_token_seconds=turn_result.time_to_first_token_seconds,
                generation_duration_seconds=turn_result.eval_duration_seconds,
                prompt_eval_duration_seconds=turn_result.prompt_eval_duration_seconds,
                total_duration_seconds=turn_result.total_duration_seconds,
                load_duration_seconds=turn_result.load_duration_seconds,
                retry_count=context.retry_count,
                output_artifact_hash=model_boundary_hash(turn_result.model_response_parts),
                succeeded=turn_result.finish_reason != "MALFORMED_FUNCTION_CALL",
                error_type=(
                    "MalformedFunctionCall"
                    if turn_result.finish_reason == "MALFORMED_FUNCTION_CALL"
                    else None
                ),
            )
        )

    async def _execute_tool_turn(
        self,
        context: _ToolTurnContext,
        result_out: list[list[ToolCallResponse]],
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Execute the pending calls and attach the current ReAct turn to each event."""
        async for chunk in execute_turn_tool_calls(
            pending_tool_calls=context.turn_result.pending_tool_calls,
            tool_executor=self._tool_executor,
            investigation=context.inputs.investigation,
            g8e_context=context.inputs.g8e_context,
            result_out=result_out,
            request_settings=context.inputs.request_settings,
            event_service=context.event_service,
        ):
            yield chunk.model_copy(
                update={"data": chunk.data.model_copy(update={"loop_turn": context.loop_turn})}
            )

    @staticmethod
    def _retry_malformed_call(
        finish_reason: str | None,
        contents: list[types.Content],
        loop_turn: int,
        retry_count: int,
    ) -> int | None:
        """Append the repair instruction when the provider returns a malformed tool call."""
        if finish_reason != "MALFORMED_FUNCTION_CALL":
            return None
        if retry_count >= AGENT_MAX_RETRIES:
            raise RuntimeError(
                "The AI could not generate a valid operator tool call after retries. "
                "The requested check did not complete. Please retry the request."
            )
        retry_count += 1
        logger.warning(
            "[AGENT] Malformed function call at turn %d; retrying model turn (%d/%d)",
            loop_turn,
            retry_count,
            AGENT_MAX_RETRIES,
        )
        contents.append(
            types.Content(
                role=types.Role.USER,
                parts=[
                    types.Part.from_text(
                        "Your previous tool call was malformed and was not executed. "
                        "Continue the request using the declared tools and arguments "
                        "that match their JSON schemas. Use {} for a tool with no arguments."
                    )
                ],
            )
        )
        return retry_count

    @staticmethod
    def _append_turn_responses(context: _TurnResponseContext) -> GroundingMetadata | None:
        for response in context.responses:
            if response.grounding is not None:
                context.grounding_metadata = merge_grounding(
                    context.grounding_metadata, response.grounding
                )
        if context.turn_result.model_response_parts:
            consolidated = consolidate_model_parts(context.turn_result.model_response_parts)
            context.contents.append(types.Content(role=types.Role.MODEL, parts=consolidated))
            logger.info("[AGENT] Added model response: %d parts", len(consolidated))
        tool_parts = [
            types.Part.from_tool_response(
                name=response.tool_name,
                response=response.flattened_response,
                call_id=response.tool_call_id,
            )
            for response in context.responses
        ]
        context.response_sizes.extend(
            len(str(response.flattened_response)) for response in context.responses
        )
        context.contents.append(types.Content(role=types.Role.TOOL, parts=tool_parts))
        logger.info("[AGENT] Added %d tool responses, looping...", len(tool_parts))
        return context.grounding_metadata

    @staticmethod
    def _citation_chunk(
        grounding_metadata: GroundingMetadata | None,
    ) -> StreamChunkFromModel | None:
        if not (grounding_metadata and grounding_metadata.grounding_used):
            logger.info("[AGENT] No grounding metadata to emit for this turn")
            return None
        return StreamChunkFromModel(
            type=StreamChunkFromModelType.CITATIONS,
            data=StreamChunkData(grounding_metadata=grounding_metadata),
        )

    @classmethod
    def _closing_chunks(
        cls,
        grounding_metadata: GroundingMetadata | None,
        finish_reason: str,
        totals: _TokenTotals,
        model_calls: list[ModelCallTelemetry],
        tool_response_sizes: list[int],
        turn_limit_reached: bool,
    ) -> list[StreamChunkFromModel]:
        """Build the optional CITATIONS chunk followed by the terminal COMPLETE chunk."""
        chunks: list[StreamChunkFromModel] = []
        citation_chunk = cls._citation_chunk(grounding_metadata)
        if citation_chunk is not None:
            chunks.append(citation_chunk)
        chunks.append(
            cls._completion_chunk(
                finish_reason, totals, model_calls, tool_response_sizes, turn_limit_reached
            )
        )
        return chunks

    @staticmethod
    def _completion_chunk(
        finish_reason: str,
        totals: _TokenTotals,
        model_calls: list[ModelCallTelemetry],
        tool_response_sizes: list[int],
        turn_limit_reached: bool,
    ) -> StreamChunkFromModel:
        token_usage = None
        if model_calls:
            token_usage = TokenUsage(
                input_tokens=totals.input_tokens,
                output_tokens=totals.output_tokens,
                total_tokens=totals.total_tokens,
                thinking_tokens=_sum_optional_usage_counts(
                    [call.thinking_tokens for call in model_calls]
                ),
                cache_tokens=_sum_optional_usage_counts(
                    [call.cache_tokens for call in model_calls]
                ),
                usage_reported=all(call.usage_reported for call in model_calls),
            )
        logger.info(
            "[AGENT] Yielding COMPLETE chunk: finish_reason=%s input_tokens=%d output_tokens=%d total_tokens=%d",
            finish_reason or DEFAULT_FINISH_REASON,
            totals.input_tokens,
            totals.output_tokens,
            totals.total_tokens,
        )
        return StreamChunkFromModel(
            type=StreamChunkFromModelType.COMPLETE,
            data=StreamChunkData(
                finish_reason=finish_reason or DEFAULT_FINISH_REASON,
                token_usage=token_usage,
                model_calls=model_calls,
                tool_response_sizes=tool_response_sizes if tool_response_sizes else None,
                tool_turn_limit_reached=turn_limit_reached or None,
            ),
        )
