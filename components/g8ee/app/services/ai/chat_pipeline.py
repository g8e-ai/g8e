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
ChatPipelineService — internal chat pipeline coordinator.

Owns:
- Pre-flight context assembly (_prepare_chat_context)
- Post-stream AI response persistence and LFAA audit
- The single public chat entry-point: run_chat (SSE via EventService)

Does NOT own:
- Task lifecycle / cancellation  →  ChatTaskManager
- LLM streaming / ReAct loop    →  g8eEngine
- Investigation context queries →  InvestigationContextManager
"""

import asyncio
import logging
import time

import app.llm.llm_types as types
from app.models.settings import G8eeUserSettings
from app.errors import BusinessLogicError, ConfigurationError
from app.constants import (
    AgentName,
    AITaskId,
    NEW_CASE_ID,
    EventType,
    LLMProvider,
    TriageComplexityClassification,
    TriageConfidence,
    AgentMode,
    OperatorStatus,
)
from app.constants.message_sender import MessageSender
from app.llm import get_llm_provider
from app.models.agent import AgentInputs, AgentStreamState
from app.models.attachments import AttachmentMetadata, ProcessedAttachment
from app.models.http_context import G8eHttpContext
from app.models.investigations import (
    ConversationMessageMetadata,
    ConversationHistoryMessage,
    EnrichedInvestigationContext,
)
from app.models.agent_activity import AgentActivityMetadata
from app.llm.prompts import build_modular_system_prompt
from app.llm.utils import resolve_model, ModelOverrideResolver

from ..infra.g8ed_event_service import EventService
from .agent import g8eEngine
from ..investigation.investigation_service import extract_all_operators_context, InvestigationService
from ..investigation.memory_data_service import MemoryDataService
from .memory_generation_service import MemoryGenerationService
from .chat_task_manager import BackgroundTaskManager as BackgroundTaskManager
from .request_builder import AIRequestBuilder
from .triage import TriageAgent
from ..data.agent_activity_data_service import AgentActivityDataService
from app.models.agents.triage import TriageRequest
from app.models.g8ed_client import (
    ChatErrorPayload,
    ChatProcessingStartedPayload,
    ChatResponseChunkPayload,
    ChatResponseCompletePayload,
)

logger = logging.getLogger(__name__)


class ChatPipelineService:
    """Coordinates the full internal chat pipeline for browser sessions.

    All dependencies are injected — nothing is constructed internally.
    """

    def __init__(
        self,
        g8ed_event_service: EventService,
        investigation_service: InvestigationService,
        request_builder: AIRequestBuilder,
        g8e_agent: g8eEngine,
        memory_service: MemoryDataService,
        memory_generation_service: MemoryGenerationService,
        agent_activity_data_service: AgentActivityDataService,
    ) -> None:
        self.g8ed_event_service = g8ed_event_service
        self.investigation_service = investigation_service
        self.request_builder = request_builder
        self.g8e_agent = g8e_agent
        self.memory_service = memory_service
        self.memory_generation_service = memory_generation_service
        self.agent_activity_data_service = agent_activity_data_service
        self.triage_agent = TriageAgent()

        logger.info("ChatPipelineService initialized")

    async def _prepare_chat_context(
        self,
        message: str,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        attachments: list[AttachmentMetadata],
        sentinel_mode: bool,
        model_overrides: ModelOverrideResolver,
    ) -> AgentInputs:
        """Assemble all immutable, request-scoped inputs for a chat turn.

        Sequence:
        1. Fetch and enrich investigation context (operators, memory)
        2. Sync sentinel_mode to DB if changed
        3. Determine workflow type (OPERATOR_BOUND vs NOT_BOUND)
        4. Fetch prior conversation history for triage
        5. Triage message (main model vs lite model)
        6. Mark pending approvals as feedback
        7. Persist user message to DB
        8. Re-fetch conversation history (now includes user message)
        9. Dispatch LFAA user-message audit per bound operator
        10. Retrieve user and case memories
        11. Build system prompt
        12. Build generation config
        13. Format attachment parts
        14. Build contents from history
        15. Assemble AgentInputs

        Returns an ``AgentInputs`` instance. The caller is responsible for
        pairing it with a fresh ``AgentStreamState`` when running the agent;
        the stream sinks (response_text, token_usage, finish_reason,
        grounding_metadata) are intentionally absent from the inputs to
        keep request inputs immutable.
        """
        case_id = g8e_context.case_id
        investigation_id = g8e_context.investigation_id
        user_id = g8e_context.user_id
        
        logger.info(
            "[SSE-CHAT] _prepare_chat_context started: investigation_id=%s case_id=%s",
            investigation_id, case_id
        )
        
        logger.info(
            "[SSE-CHAT] Extracted context: case_id=%s investigation_id=%s web_session_id=%s user_id=%s",
            case_id, investigation_id, g8e_context.web_session_id, user_id
        )

        if not investigation_id or investigation_id == NEW_CASE_ID:
            raise BusinessLogicError(
                "_prepare_chat_context requires real investigation_id, not NEW_CASE_ID",
                details={"investigation_id": investigation_id}
            )

        investigation = await self.investigation_service.get_investigation_context(
            investigation_id=investigation_id, user_id=user_id
        )
        investigation = await self.investigation_service.get_enriched_investigation_context(
            investigation=investigation, user_id=user_id, g8e_context=g8e_context
        )

        current_sentinel_mode = investigation.sentinel_mode
        if current_sentinel_mode != sentinel_mode:
            await self.investigation_service.update_investigation_raw(
                investigation_id=investigation_id,
                updates={"sentinel_mode": sentinel_mode},
            )
            investigation.sentinel_mode = sentinel_mode

        operator_bound = any(op.status == OperatorStatus.BOUND for op in g8e_context.bound_operators)
        agent_mode = AgentMode.OPERATOR_BOUND if operator_bound else AgentMode.OPERATOR_NOT_BOUND

        prior_history = await self.investigation_service.get_chat_messages(
            investigation_id=investigation_id
        )

        triage_request = TriageRequest(
            message=message,
            agent_mode=agent_mode,
            conversation_history=prior_history,
            attachments=attachments,
            settings=request_settings,
            model_override=model_overrides.for_triage(),
        )
        triage_result = await self.triage_agent.triage(triage_request)

        needs_main_model = triage_result.complexity == TriageComplexityClassification.COMPLEX

        model_to_use = resolve_model(
            tier="primary" if needs_main_model else "assistant",
            primary_override=model_overrides.for_main_generation(needs_primary=True),
            assistant_override=model_overrides.for_main_generation(needs_primary=False),
            settings_primary_model=request_settings.llm.primary_model,
            settings_assistant_model=request_settings.llm.resolved_assistant_model,
        )
        
        if not model_to_use:
            raise ConfigurationError(
                "No LLM model configured. Set a primary_model and/or assistant_model in platform settings."
            )
            
        max_tokens = request_settings.llm.llm_max_tokens

        logger.info(
            "[CHAT] Triage: complexity=%s (conf=%s) intent=%s (conf=%s) posture=%s (conf=%s) model=%s",
            triage_result.complexity,
            triage_result.complexity_confidence,
            triage_result.intent_summary,
            triage_result.intent_confidence,
            triage_result.request_posture,
            triage_result.posture_confidence,
            model_to_use,
        )

        attachment_filenames = [att.filename for att in attachments] if attachments else []

        if investigation_id:
            await self.investigation_service.add_chat_message(
                investigation_id=investigation_id,
                sender=MessageSender.USER_CHAT,
                content=message,
                metadata=ConversationMessageMetadata(attachment_filenames=attachment_filenames),
            )

        conversation_history = await self.investigation_service.get_chat_messages(
            investigation_id=investigation_id
        )

        user_memories = []
        case_memories = []
        try:
            if investigation.user_id:
                user_memories = await self.memory_service.get_user_memories(
                    user_id=investigation.user_id
                )
            if case_id and case_id != NEW_CASE_ID and investigation.user_id:
                case_memories = await self.memory_service.get_case_memories(
                    case_id=case_id, 
                    user_id=investigation.user_id
                )
        except Exception as e:
            logger.warning("Failed to retrieve memories for chat context: %s", e, exc_info=True)

        all_operator_contexts = extract_all_operators_context(investigation)
        active_agent = AgentName.SAGE if needs_main_model else AgentName.DASH
        system_instructions = build_modular_system_prompt(
            operator_bound=operator_bound,
            system_context=all_operator_contexts,
            user_memories=user_memories,
            case_memories=case_memories,
            investigation=investigation,
            g8e_web_search_available=self.g8e_agent.g8e_web_search_available,
            triage_result=triage_result,
            agent_name=active_agent,
        )

        generation_config = self.request_builder.get_generation_config(
            system_instructions=system_instructions,
            settings=request_settings,
            agent_mode=agent_mode,
            max_tokens=max_tokens,
            model_override=model_to_use,
        )

        attachment_parts: list[types.Part] = []
        if attachments:
            processed: list[ProcessedAttachment] = [
                ProcessedAttachment(
                    filename=a.filename,
                    content_type=a.content_type,
                )
                for a in attachments
            ]
            attachment_parts = self.request_builder.format_attachment_parts(processed)

        investigation_sentinel_mode = investigation.sentinel_mode if investigation else True
        contents = self.request_builder.build_contents_from_history(
            conversation_history=conversation_history,
            attachments=attachment_parts if attachment_parts else [],
            sentinel_mode=investigation_sentinel_mode
        )

        return AgentInputs(
            case_id=g8e_context.case_id,
            investigation_id=g8e_context.investigation_id,
            investigation=investigation,
            user_id=g8e_context.user_id,
            g8e_context=g8e_context,
            web_session_id=g8e_context.web_session_id,
            task_id=AITaskId.CHAT.value,
            agent_mode=agent_mode,
            request_settings=request_settings,
            operator_bound=operator_bound,
            model_to_use=model_to_use,
            max_tokens=max_tokens,
            conversation_history=conversation_history,
            system_instructions=system_instructions,
            contents=contents,
            generation_config=generation_config,
            user_memories=user_memories,
            case_memories=case_memories,
            triage_result=triage_result,
        )

    async def _persist_ai_response(
        self,
        g8e_context: G8eHttpContext,
        inputs: AgentInputs,
        state: AgentStreamState,
        user_settings: G8eeUserSettings,
        task_manager: BackgroundTaskManager | None = None,
    ) -> None:
        """Persist the final AI response and schedule memory update off the response path.

        Memory generation is dispatched as a tracked background task so it cannot
        block SSE completion or swallow errors silently. If ``task_manager`` is
        provided, the task is registered (and thus awaitable during cleanup);
        otherwise it is fired as a detached asyncio task with a done-callback
        that surfaces failures at WARNING level.
        """
        logger.info(
            "[SSE-CHAT] _persist_ai_response started: investigation_id=%s response_len=%d",
            getattr(g8e_context, 'investigation_id', 'unknown') if g8e_context else 'None',
            len(state.response_text),
        )

        persisted = await self.investigation_service.persist_ai_message(
            investigation_id=g8e_context.investigation_id,
            text=state.response_text,
            grounding_metadata=state.grounding_metadata,
            token_usage=state.token_usage,
        )
        if persisted:
            logger.info("[SSE-CHAT] Final AI response persisted to database")
        else:
            logger.info("[SSE-CHAT] Skipping final AI response persistence (empty response_text)")

        if inputs.investigation and g8e_context.investigation_id:
            self._dispatch_memory_update(
                investigation=inputs.investigation,
                conversation_history=inputs.conversation_history,
                user_settings=user_settings,
                task_manager=task_manager,
            )

    async def _record_agent_activity_metadata(
        self,
        inputs: AgentInputs,
        state: AgentStreamState,
        start_time: float,
        attachments: list[AttachmentMetadata],
        error: str | None = None,
    ) -> None:
        """Record agent activity metadata for data science analysis.
        
        This is called after each chat turn completes to capture comprehensive
        telemetry including model usage, token consumption, tool execution,
        triage classification, and performance metrics.
        """
        try:
            duration_seconds = time.time() - start_time
            
            metadata = AgentActivityMetadata(
                user_id=inputs.user_id,
                user_email=inputs.investigation.user_email if inputs.investigation else None,
                investigation_id=inputs.investigation_id,
                case_id=inputs.case_id,
                web_session_id=inputs.web_session_id,
                agent_mode=inputs.agent_mode,
                model_name=inputs.model_to_use,
                provider=inputs.request_settings.llm.primary_provider.value if inputs.request_settings.llm.primary_provider else None,
                token_usage=state.token_usage,
                finish_reason=state.finish_reason,
                triage_complexity=inputs.triage_result.complexity if inputs.triage_result else None,
                triage_complexity_confidence=inputs.triage_result.complexity_confidence if inputs.triage_result else None,
                triage_intent_summary=inputs.triage_result.intent_summary if inputs.triage_result else None,
                triage_intent_confidence=inputs.triage_result.intent_confidence if inputs.triage_result else None,
                triage_request_posture=inputs.triage_result.request_posture if inputs.triage_result else None,
                triage_posture_confidence=inputs.triage_result.posture_confidence if inputs.triage_result else None,
                tool_call_count=state.tool_call_count,
                tool_types_used=state.tool_types_used,
                duration_seconds=duration_seconds,
                has_attachments=len(attachments) > 0,
                attachment_count=len(attachments),
                grounding_used=state.grounding_metadata is not None,
                citation_count=len(state.grounding_metadata.citations) if state.grounding_metadata and state.grounding_metadata.citations else 0,
                operator_bound=inputs.operator_bound,
                bound_operator_count=len(inputs.g8e_context.bound_operators) if inputs.g8e_context.bound_operators else 0,
                response_length=len(state.response_text),
                error=error,
            )

            await self.agent_activity_data_service.record_activity(metadata)
            logger.info(
                "Agent activity metadata recorded",
                extra={
                    "activity_id": metadata.id,
                    "investigation_id": metadata.investigation_id,
                    "model": metadata.model_name,
                    "duration_seconds": metadata.duration_seconds,
                }
            )
        except Exception as e:
            logger.warning(f"Failed to record agent activity metadata: {e}", exc_info=True)

    def _dispatch_memory_update(
        self,
        investigation: EnrichedInvestigationContext,
        conversation_history: list[ConversationHistoryMessage],
        user_settings: G8eeUserSettings,
        task_manager: BackgroundTaskManager | None,
    ) -> None:
        """Schedule memory generation as a background task so it never blocks persistence.

        Note: ``conversation_history`` is a snapshot from ``_prepare_chat_context``
        and does NOT include the final AI response row written above — any memory
        generation that needs the very latest turn must re-read from the DB. The
        staleness is intentional: we trade a one-turn lag for keeping memory
        generation off the response path. See ``docs/architecture/ai_agents.md``.
        """
        investigation_id = investigation.id

        async def _run_memory_update() -> None:
            try:
                await self.memory_generation_service.update_memory_from_conversation(
                    conversation_history=conversation_history,
                    investigation=investigation,
                    settings=user_settings,
                )
                logger.info(
                    "Background memory update completed for investigation %s",
                    investigation_id,
                    extra={
                        "investigation_id": investigation_id,
                        "operation": "memory_update_background_complete",
                    },
                )
            except Exception as e:
                logger.warning(
                    "Background memory update failed for investigation %s: %s",
                    investigation_id, e,
                    exc_info=True,
                    extra={
                        "investigation_id": investigation_id,
                        "error": str(e),
                        "operation": "memory_update_background_failed",
                    },
                )

        task = asyncio.create_task(_run_memory_update())
        if task_manager is not None:
            task_manager.track_detached(f"memory:{investigation_id}", task)
        else:
            def _log_uncaught(t: asyncio.Task[None]) -> None:
                if t.cancelled():
                    return
                exc = t.exception()
                if exc is not None:
                    logger.warning(
                        "Detached memory update task crashed for investigation %s: %s",
                        investigation_id, exc,
                        exc_info=exc,
                    )

            task.add_done_callback(_log_uncaught)

    async def run_chat(
        self,
        message: str,
        g8e_context: G8eHttpContext,
        attachments: list[AttachmentMetadata],
        sentinel_mode: bool,
        llm_primary_provider: str | None,
        llm_assistant_provider: str | None,
        llm_primary_model: str,
        llm_assistant_model: str,
        _task_manager: BackgroundTaskManager,
        user_settings: G8eeUserSettings,
        _track_task: bool = True,
    ) -> None:
        """Non-streaming chat path — AI response delivered via SSE through g8ed.

        Optionally registers the current asyncio task with ChatTaskManager so it
        can be cancelled via the stop endpoint.
        """
        logger.info(
            "[SSE-CHAT] run_chat started: new_case=%s case_id=%s investigation_id=%s web_session_id=%s",
            getattr(g8e_context, 'new_case', 'unknown') if g8e_context else 'None',
            getattr(g8e_context, 'case_id', 'unknown') if g8e_context else 'None',
            getattr(g8e_context, 'investigation_id', 'unknown') if g8e_context else 'None',
            getattr(g8e_context, 'web_session_id', 'unknown') if g8e_context else 'None',
        )
        
        investigation_id = g8e_context.investigation_id if g8e_context else ""
        logger.info("[SSE-CHAT] Extracted investigation_id: %s", investigation_id)
        
        task = None
        if _track_task and investigation_id:
            task = asyncio.current_task()
            if task:
                await _task_manager.track(investigation_id, task)

        try:
            logger.info(
                "[SSE-CHAT] About to call _run_chat_impl: message_len=%d sentinel_mode=%s primary_provider=%s assistant_provider=%s primary=%s assistant=%s",
                len(message), sentinel_mode, llm_primary_provider, llm_assistant_provider, llm_primary_model, llm_assistant_model
            )
            await self._run_chat_impl(
                message=message,
                g8e_context=g8e_context,
                attachments=attachments,
                sentinel_mode=sentinel_mode,
                llm_primary_provider=llm_primary_provider,
                llm_assistant_provider=llm_assistant_provider,
                llm_primary_model=llm_primary_model,
                llm_assistant_model=llm_assistant_model,
                user_settings=user_settings,
                task_manager=_task_manager,
            )
            logger.info("[SSE-CHAT] _run_chat_impl completed successfully")
        except asyncio.CancelledError:
            logger.info(
                "[SSE-CHAT] Task cancelled for investigation %s", investigation_id
            )
            raise
        except Exception as e:
            logger.error(
                "[SSE-CHAT] Background task crashed for investigation %s: %s",
                investigation_id, e, exc_info=True
            )
            try:
                await self.g8ed_event_service.publish_investigation_event(
                    investigation_id=investigation_id,
                    event_type=EventType.LLM_CHAT_ITERATION_FAILED,
                    payload=ChatErrorPayload(error=str(e)),
                    web_session_id=g8e_context.web_session_id,
                    case_id=g8e_context.case_id,
                    user_id=g8e_context.user_id,
                )
            except Exception as notify_err:
                logger.error(
                    "[SSE-CHAT] Failed to send error event to frontend: %s", notify_err
                )
        finally:
            if task and investigation_id:
                await _task_manager.untrack(investigation_id)

    async def _run_chat_impl(
        self,
        message: str,
        g8e_context: G8eHttpContext,
        attachments: list[AttachmentMetadata],
        sentinel_mode: bool,
        llm_primary_provider: str | None,
        llm_assistant_provider: str | None,
        llm_primary_model: str,
        llm_assistant_model: str,
        user_settings: G8eeUserSettings,
        task_manager: BackgroundTaskManager | None = None,
    ) -> None:
        """Run the chat implementation - main logic flow."""
        start_time = time.time()
        logger.info(
            "[SSE-CHAT] _run_chat_impl started: g8e_context=%s attachments_count=%d",
            f"case_id={getattr(g8e_context, 'case_id', 'missing')}",
            len(attachments) if attachments else 0
        )

        # Coerce the raw HTTP string to LLMProvider explicitly —
        # model_copy(update=...) skips validation and would leave a bare
        # str in the enum-typed field.
        resolved_settings = user_settings
        if llm_primary_provider:
            logger.info("[SSE-CHAT] Applying primary_provider override: %s", llm_primary_provider)
            resolved_settings = resolved_settings.model_copy(update={"llm": resolved_settings.llm.model_copy(update={"primary_provider": LLMProvider(llm_primary_provider)})})
        if llm_assistant_provider:
            logger.info("[SSE-CHAT] Applying assistant_provider override: %s", llm_assistant_provider)
            resolved_settings = resolved_settings.model_copy(update={"llm": resolved_settings.llm.model_copy(update={"assistant_provider": LLMProvider(llm_assistant_provider)})})

        logger.info("[SSE-CHAT] About to call _prepare_chat_context")
        model_overrides = ModelOverrideResolver(
            primary_model=llm_primary_model,
            assistant_model=llm_assistant_model,
        )
        inputs = await self._prepare_chat_context(
            message=message,
            g8e_context=g8e_context,
            request_settings=resolved_settings,
            attachments=attachments,
            sentinel_mode=sentinel_mode,
            model_overrides=model_overrides,
        )
        logger.info("[SSE-CHAT] _prepare_chat_context completed successfully")

        state = AgentStreamState()

        is_assistant_provider = (
            inputs.triage_result.complexity != TriageComplexityClassification.COMPLEX
            if inputs.triage_result
            else False
        )
        llm_provider = get_llm_provider(resolved_settings.llm, is_assistant=is_assistant_provider)
        logger.info("[SSE-CHAT] LLM provider resolved: %s (is_assistant=%s)", type(llm_provider).__name__, is_assistant_provider)

        logger.info(
            "[SSE-CHAT] Starting LLM call: model=%s workflow=%s contents=%d max_tokens=%s",
            inputs.model_to_use, inputs.agent_mode, len(inputs.contents), inputs.max_tokens
        )

        if inputs.triage_result and inputs.triage_result.follow_up_question and (
            inputs.triage_result.complexity_confidence == TriageConfidence.LOW or
            inputs.triage_result.intent_confidence == TriageConfidence.LOW
        ):
            follow_up = inputs.triage_result.follow_up_question
            logger.info("[SSE-CHAT] Triage short-circuit: delivering follow-up question")

            await self.g8ed_event_service.publish_investigation_event(
                investigation_id=g8e_context.investigation_id,
                event_type=EventType.LLM_CHAT_ITERATION_STARTED,
                payload=ChatProcessingStartedPayload(agent_mode=inputs.agent_mode),
                web_session_id=g8e_context.web_session_id,
                case_id=g8e_context.case_id,
                user_id=g8e_context.user_id,
            )

            await self.g8ed_event_service.publish_investigation_event(
                investigation_id=g8e_context.investigation_id,
                event_type=EventType.LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED,
                payload=ChatResponseChunkPayload(content=follow_up),
                web_session_id=g8e_context.web_session_id,
                case_id=g8e_context.case_id,
                user_id=g8e_context.user_id,
            )

            await self.g8ed_event_service.publish_investigation_event(
                investigation_id=g8e_context.investigation_id,
                event_type=EventType.LLM_CHAT_ITERATION_TEXT_COMPLETED,
                payload=ChatResponseCompletePayload(
                    content=follow_up,
                    finish_reason="stop",
                ),
                web_session_id=g8e_context.web_session_id,
                case_id=g8e_context.case_id,
                user_id=g8e_context.user_id,
            )
            state.response_text = follow_up
        elif inputs.model_to_use and inputs.generation_config:
            logger.info("[SSE-CHAT] Running full agent execution")

            async def _persist_iteration_text(text: str) -> None:
                await self.investigation_service.persist_ai_message(
                    investigation_id=inputs.investigation_id,
                    text=text,
                )

            await self.g8e_agent.run_with_sse(
                inputs=inputs,
                state=state,
                g8ed_event_service=self.g8ed_event_service,
                llm_provider=llm_provider,
                on_iteration_text=_persist_iteration_text,
            )
            logger.info("[SSE-CHAT] Agent execution completed")

        if state.token_usage:
            logger.info(
                "[TOKEN_USAGE] SSE-CHAT final: %s", state.token_usage
            )

        await self._persist_ai_response(
            g8e_context=g8e_context,
            inputs=inputs,
            state=state,
            user_settings=user_settings,
            task_manager=task_manager,
        )

        await self._record_agent_activity_metadata(
            inputs=inputs,
            state=state,
            start_time=start_time,
            attachments=attachments,
        )

        logger.info(
            "[SSE-CHAT] Completed: %d chars",
            len(state.response_text)
        )
