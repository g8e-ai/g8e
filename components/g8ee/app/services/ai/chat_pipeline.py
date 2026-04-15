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

import app.llm.llm_types as types
from app.models.settings import G8eeUserSettings
from app.errors import BusinessLogicError, ConfigurationError
from app.constants import (
    NEW_CASE_ID,
    EventType,
    TriageComplexityClassification,
    TriageConfidence,
    AgentMode,
    OperatorStatus,
)
from app.constants.message_sender import MessageSender
from app.llm import get_llm_provider
from app.models.agent import AgentStreamContext
from app.models.attachments import AttachmentMetadata, ProcessedAttachment
from app.models.http_context import G8eHttpContext
from app.models.investigations import (
    AIResponseMetadata,
    ConversationMessageMetadata,
)
from app.llm.prompts import build_modular_system_prompt
from app.llm.utils import resolve_model

from ..infra.g8ed_event_service import EventService
from .agent import g8eEngine
from ..investigation.investigation_service import extract_all_operators_context, InvestigationService
from ..investigation.memory_data_service import MemoryDataService
from .memory_generation_service import MemoryGenerationService
from .chat_task_manager import BackgroundTaskManager as BackgroundTaskManager
from .request_builder import AIRequestBuilder
from .triage import TriageAgent
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
    ) -> None:
        self.g8ed_event_service = g8ed_event_service
        self.investigation_service = investigation_service
        self.request_builder = request_builder
        self.g8e_agent = g8e_agent
        self.memory_service = memory_service
        self.memory_generation_service = memory_generation_service
        self.triage_agent = TriageAgent()

        logger.info("ChatPipelineService initialized")

    async def _prepare_chat_context(
        self,
        message: str,
        g8e_context: G8eHttpContext,
        request_settings: G8eeUserSettings,
        attachments: list[AttachmentMetadata],
        sentinel_mode: bool,
        llm_primary_model: str,
        llm_assistant_model: str,
    ) -> AgentStreamContext:
        """Assemble all inputs required for a chat turn.

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
        15. Assemble AgentStreamContext
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
            model_override=llm_primary_model,
        )
        triage_result = await self.triage_agent.triage(triage_request)

        needs_main_model = triage_result.complexity == TriageComplexityClassification.COMPLEX

        model_to_use = resolve_model(
            tier="primary" if needs_main_model else "assistant",
            primary_override=llm_primary_model,
            assistant_override=llm_assistant_model,
            settings_primary_model=request_settings.llm.primary_model,
            settings_assistant_model=request_settings.llm.resolved_assistant_model,
        )
        
        if not model_to_use:
            raise ConfigurationError(
                "No LLM model configured. Set a primary_model and/or assistant_model in platform settings."
            )
            
        max_tokens = request_settings.llm.llm_max_tokens

        logger.info(
            "[CHAT] Triage: complexity=%s (conf=%s) intent=%s (conf=%s) model=%s",
            triage_result.complexity,
            triage_result.complexity_confidence,
            triage_result.intent_summary,
            triage_result.intent_confidence,
            model_to_use
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
        system_instructions = build_modular_system_prompt(
            operator_bound=operator_bound,
            system_context=all_operator_contexts,
            user_memories=user_memories,
            case_memories=case_memories,
            investigation=investigation,
            g8e_web_search_available=self.g8e_agent.g8e_web_search_available,
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

        return AgentStreamContext(
            case_id=g8e_context.case_id,
            investigation_id=g8e_context.investigation_id,
            investigation=investigation,
            user_id=g8e_context.user_id,
            g8e_context=g8e_context,
            web_session_id=g8e_context.web_session_id,
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
        ctx: AgentStreamContext,
        user_settings: G8eeUserSettings,
    ) -> None:
        """Persist AI response to database and trigger memory updates."""
        logger.info(
            "[SSE-CHAT] _persist_ai_response started: investigation_id=%s response_len=%d",
            getattr(g8e_context, 'investigation_id', 'unknown') if g8e_context else 'None',
            len(ctx.response_text) if ctx else 0
        )
        
        response_text = ctx.response_text

        await self.investigation_service.add_chat_message(
            investigation_id=g8e_context.investigation_id or "",
            content=response_text,
            sender=MessageSender.AI_PRIMARY,
            metadata=AIResponseMetadata(
                source=EventType.EVENT_SOURCE_AI_PRIMARY,
                grounding_metadata=ctx.grounding_metadata,
                token_usage=ctx.token_usage,
            ),
        )
        logger.info("[SSE-CHAT] AI response persisted to database")

        if ctx.investigation and g8e_context.investigation_id:
            investigation = ctx.investigation
            conversation_history = ctx.conversation_history
            try:
                await self.memory_generation_service.update_memory_from_conversation(
                    conversation_history=conversation_history,
                    investigation=investigation,
                    settings=user_settings,
                )
                logger.info(
                    "Background memory update completed for investigation %s",
                    investigation.id,
                    extra={"investigation_id": investigation.id, "operation": "memory_update_background_complete"},
                )
            except Exception as e:
                logger.info(
                    "Background memory update failed for investigation %s: %s",
                    investigation.id, e,
                    extra={"investigation_id": investigation.id, "error": str(e), "operation": "memory_update_background_failed"},
                )

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
    ) -> None:
        """Run the chat implementation - main logic flow."""
        logger.info(
            "[SSE-CHAT] _run_chat_impl started: g8e_context=%s attachments_count=%d",
            f"case_id={getattr(g8e_context, 'case_id', 'missing')}",
            len(attachments) if attachments else 0
        )

        # Apply provider overrides if provided
        resolved_settings = user_settings
        if llm_primary_provider:
            logger.info("[SSE-CHAT] Applying primary_provider override: %s", llm_primary_provider)
            resolved_settings = resolved_settings.model_copy(update={"llm": resolved_settings.llm.model_copy(update={"primary_provider": llm_primary_provider})})
        if llm_assistant_provider:
            logger.info("[SSE-CHAT] Applying assistant_provider override: %s", llm_assistant_provider)
            resolved_settings = resolved_settings.model_copy(update={"llm": resolved_settings.llm.model_copy(update={"assistant_provider": llm_assistant_provider})})

        async with get_llm_provider(resolved_settings.llm) as llm_provider:
            logger.info("[SSE-CHAT] LLM provider resolved: %s", type(llm_provider).__name__)

            logger.info("[SSE-CHAT] About to call _prepare_chat_context")
            ctx = await self._prepare_chat_context(
                message=message,
                g8e_context=g8e_context,
                request_settings=resolved_settings,
                attachments=attachments,
                sentinel_mode=sentinel_mode,
                llm_primary_model=llm_primary_model,
                llm_assistant_model=llm_assistant_model,
            )
            logger.info("[SSE-CHAT] _prepare_chat_context completed successfully")

            logger.info(
                "[SSE-CHAT] Starting LLM call: model=%s workflow=%s contents=%d max_tokens=%s",
                ctx.model_to_use, ctx.agent_mode, len(ctx.contents), ctx.max_tokens
            )

            if ctx.triage_result and ctx.triage_result.follow_up_question and (
                ctx.triage_result.complexity_confidence == TriageConfidence.LOW or
                ctx.triage_result.intent_confidence == TriageConfidence.LOW
            ):
                follow_up = ctx.triage_result.follow_up_question
                logger.info("[SSE-CHAT] Triage short-circuit: delivering follow-up question")

                await self.g8ed_event_service.publish_investigation_event(
                    investigation_id=g8e_context.investigation_id,
                    event_type=EventType.LLM_CHAT_ITERATION_STARTED,
                    payload=ChatProcessingStartedPayload(agent_mode=ctx.agent_mode),
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
                ctx.response_text = follow_up
            elif ctx.model_to_use and ctx.generation_config:
                logger.info("[SSE-CHAT] Running full agent execution")
                await self.g8e_agent.run_with_sse(
                    contents=ctx.contents,
                    generation_config=ctx.generation_config,
                    model_name=ctx.model_to_use,
                    agent_streaming_context=ctx,
                    context=ctx,
                    g8ed_event_service=self.g8ed_event_service,
                    llm_provider=llm_provider,
                )
                logger.info("[SSE-CHAT] Agent execution completed")

            if ctx.token_usage:
                logger.info(
                    "[TOKEN_USAGE] SSE-CHAT final: %s", ctx.token_usage
                )

            await self._persist_ai_response(
                g8e_context=g8e_context,
                ctx=ctx,
                user_settings=user_settings,
            )

            logger.info(
                "[SSE-CHAT] Completed: %d chars",
                len(ctx.response_text)
            )
