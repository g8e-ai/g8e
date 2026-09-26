# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

"""
Typed client publish events.

All events published to client are represented as one of two typed envelopes:

- SessionEvent: routes to a specific connected browser session. web_session_id,
  case_id, and investigation_id are all required - the caller must have all
  three or the event cannot be constructed.

- BackgroundEvent: system-initiated, no browser session. client fans the event
  out to every active SSE session owned by user_id. investigation_id and
  case_id are optional correlation hints carried inside the payload.

The EventService.publish() method accepts the union type and dispatches
accordingly. publish_event_to_client() no longer exists.

Wire models (SessionEventWire, BackgroundEventWire, _SSEEventBody) and all SSE
payload classes are sourced from the g8e protocol package. g8ee subclasses the
wire models to add from_session_event/from_background_event classmethods that
accept g8ee's internal SessionEvent/BackgroundEvent routing wrappers.
"""

from typing import Any

from app.constants import EventType
from app.models.agents.tribunal import (
    TribunalAuditorCompletedPayload,
    TribunalAuditorFailedPayload,
    TribunalAuditorStartedPayload,
    TribunalConsensusFailedPayload,
    TribunalDissentRecordedPayload,
    TribunalMarshalBlockedPayload,
    TribunalPassCompletedPayload,
    TribunalSessionCompletedPayload,
    TribunalSessionDisabledPayload,
    TribunalSessionGenerationFailedPayload,
    TribunalSessionModelNotConfiguredPayload,
    TribunalSessionProviderUnavailablePayload,
    TribunalSessionStartedPayload,
    TribunalSessionSystemErrorPayload,
    TribunalVotingCompletedPayload,
)
from app.models.base import G8eBaseModel, Field, model_validator
from app.models.cases import CaseCreatedPayload, CaseEventPayload
from app.models.operators import (
    AgentContinueApprovalEvent,
    CommandApprovalEvent,
    FileEditApprovalEvent,
    IntentApprovalEvent,
    OperatorStatusUpdatedPayload,
    StreamApprovalEvent,
)
from app.models.reputation import (
    ReputationCommitmentCreatedPayload,
    ReputationCommitmentFailedPayload,
    StakeResolutionPayload,
)

from g8e.models.events import (
    _SSEEventBody,
    AiProcessingStoppedPayload,
    AIToolLifecyclePayload,
    ChatCitationsReadyPayload,
    ChatErrorPayload,
    ChatProcessingStartedPayload,
    ChatResponseChunkPayload,
    ChatResponseCompletePayload,
    ChatRetryPayload,
    ChatThinkingPayload,
    ChatTurnCompletePayload,
    TriageClarificationQuestionsPayload,
)
from g8e.models.events import (
    SessionEventWire as _G8eSessionEventWire,
    ScrubbingTelemetry,
)
from g8e.models.events import (
    BackgroundEventWire as _G8eBackgroundEventWire,
)

# Registry-backed SSE payload types for every ensemble-produced event on the
# sse transport. SessionEvent and BackgroundEvent reject unknown pairs.
SSE_PAYLOADS: dict[EventType, type[G8eBaseModel]] = {
    EventType.AI_AGENT_CONFLICT_DETECTED: TribunalMarshalBlockedPayload,
    EventType.AI_AGENT_CONTINUE_APPROVAL_REQUESTED: AgentContinueApprovalEvent,
    EventType.AI_CONSENSUS_SESSION_AUDITOR_FAILED: TribunalAuditorFailedPayload,
    EventType.AI_CONSENSUS_SESSION_COMPLETED: TribunalSessionCompletedPayload,
    EventType.AI_CONSENSUS_SESSION_DISABLED: TribunalSessionDisabledPayload,
    EventType.AI_CONSENSUS_SESSION_GENERATION_FAILED: TribunalSessionGenerationFailedPayload,
    EventType.AI_CONSENSUS_SESSION_MARSHAL_BLOCKED: TribunalMarshalBlockedPayload,
    EventType.AI_CONSENSUS_SESSION_MODEL_NOT_CONFIGURED: TribunalSessionModelNotConfiguredPayload,
    EventType.AI_CONSENSUS_SESSION_PROVIDER_UNAVAILABLE: TribunalSessionProviderUnavailablePayload,
    EventType.AI_CONSENSUS_SESSION_STARTED: TribunalSessionStartedPayload,
    EventType.AI_CONSENSUS_SESSION_SYSTEM_ERROR: TribunalSessionSystemErrorPayload,
    EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED: TribunalAuditorCompletedPayload,
    EventType.AI_CONSENSUS_VOTING_AUDIT_STARTED: TribunalAuditorStartedPayload,
    EventType.AI_CONSENSUS_VOTING_CONSENSUS_FAILED: TribunalConsensusFailedPayload,
    EventType.AI_CONSENSUS_VOTING_CONSENSUS_NOT_REACHED: TribunalConsensusFailedPayload,
    EventType.AI_CONSENSUS_VOTING_CONSENSUS_REACHED: TribunalVotingCompletedPayload,
    EventType.AI_CONSENSUS_VOTING_DISSENT_RECORDED: TribunalDissentRecordedPayload,
    EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED: TribunalPassCompletedPayload,
    EventType.AI_CONSENSUS_VOTING_ROUND_2_CONSENSUS_FAILED: TribunalConsensusFailedPayload,
    EventType.AI_CONSENSUS_VOTING_ROUND_2_CONSENSUS_REACHED: TribunalVotingCompletedPayload,
    EventType.AI_CONSENSUS_VOTING_ROUND_2_STARTED: TribunalSessionStartedPayload,
    EventType.AI_CONSENSUS_VOTING_ROUND_COMPLETED: TribunalSessionCompletedPayload,
    EventType.AI_CONSENSUS_VOTING_ROUND_STARTED: TribunalSessionStartedPayload,
    EventType.AI_LLM_CHAT_ITERATION_CITATIONS_RECEIVED: ChatCitationsReadyPayload,
    EventType.AI_LLM_CHAT_ITERATION_COMPLETED: ChatTurnCompletePayload,
    EventType.AI_LLM_CHAT_ITERATION_FAILED: ChatErrorPayload,
    EventType.AI_LLM_CHAT_ITERATION_RETRY: ChatRetryPayload,
    EventType.AI_LLM_CHAT_ITERATION_STARTED: ChatProcessingStartedPayload,
    EventType.AI_LLM_CHAT_ITERATION_STOPPED: AiProcessingStoppedPayload,
    EventType.AI_LLM_CHAT_ITERATION_TEXT_CHUNK_RECEIVED: ChatResponseChunkPayload,
    EventType.AI_LLM_CHAT_ITERATION_TEXT_COMPLETED: ChatResponseCompletePayload,
    EventType.AI_LLM_CHAT_ITERATION_THINKING_STARTED: ChatThinkingPayload,
    EventType.AI_LLM_TOOL_G8E_COMMAND_CONSTRAINTS_COMPLETED: AIToolLifecyclePayload,
    EventType.AI_LLM_TOOL_G8E_COMMAND_CONSTRAINTS_REQUESTED: AIToolLifecyclePayload,
    EventType.AI_LLM_TOOL_G8E_INVESTIGATION_QUERY_COMPLETED: AIToolLifecyclePayload,
    EventType.AI_LLM_TOOL_G8E_INVESTIGATION_QUERY_REQUESTED: AIToolLifecyclePayload,
    EventType.AI_LLM_TOOL_G8E_WEB_SEARCH_COMPLETED: AIToolLifecyclePayload,
    EventType.AI_LLM_TOOL_G8E_WEB_SEARCH_REQUESTED: AIToolLifecyclePayload,
    EventType.AI_TRIAGE_CLARIFICATION_QUESTIONS: TriageClarificationQuestionsPayload,
    EventType.APP_CASE_CREATED: CaseCreatedPayload,
    EventType.APP_CASE_UPDATED: CaseEventPayload,
    EventType.OPERATOR_COMMAND_APPROVAL_REQUESTED: CommandApprovalEvent,
    EventType.OPERATOR_FILE_EDIT_APPROVAL_REQUESTED: FileEditApprovalEvent,
    EventType.OPERATOR_INTENT_APPROVAL_REQUESTED: IntentApprovalEvent,
    EventType.OPERATOR_REPUTATION_COMMITMENT_CREATED: ReputationCommitmentCreatedPayload,
    EventType.OPERATOR_REPUTATION_COMMITMENT_FAILED: ReputationCommitmentFailedPayload,
    EventType.OPERATOR_REPUTATION_SLASH_TIER_1: StakeResolutionPayload,
    EventType.OPERATOR_REPUTATION_SLASH_TIER_2: StakeResolutionPayload,
    EventType.OPERATOR_REPUTATION_SLASH_TIER_3: StakeResolutionPayload,
    EventType.OPERATOR_REPUTATION_STATE_UPDATED: StakeResolutionPayload,
    EventType.OPERATOR_STATUS_UPDATED_ACTIVE: OperatorStatusUpdatedPayload,
    EventType.OPERATOR_STATUS_UPDATED_BOUND: OperatorStatusUpdatedPayload,
    EventType.OPERATOR_STREAM_APPROVAL_REQUESTED: StreamApprovalEvent,
}


def _validate_sse_payload(event_type: EventType, payload: G8eBaseModel) -> None:
    expected = SSE_PAYLOADS.get(event_type)
    if expected is None:
        raise ValueError(f"event_type {event_type!r} is not registered for ensemble SSE push")
    if type(payload) is not expected:
        raise ValueError(
            f"payload for {event_type!r} must be {expected.__name__}, got {type(payload).__name__}"
        )


class SessionEvent(G8eBaseModel):
    """Event that must reach a specific connected client session.

    Use this when the triggering request arrived on a known client session -
    either a browser (web_session_id) or a BYO CLI client (cli_session_id).
    Both are first-class session types and the Gateway keeps their routing
    namespaces strictly disjoint, so producers MUST set exactly one of the two
    session id fields. Setting neither (anonymous) or both (ambiguous) is a
    construction error caught here, before the wire layer would otherwise
    collapse the event into the wrong namespace.
    """

    event_type: EventType = Field(description="client event type")
    payload: G8eBaseModel = Field(description="Typed event-specific payload")
    web_session_id: str | None = Field(default=None, description="Browser session to deliver to")
    cli_session_id: str | None = Field(default=None, description="CLI/BYO session to deliver to")
    user_id: str = Field(description="User ID associated with the session")
    case_id: str | None = Field(default=None, description="Case correlation ID")
    investigation_id: str | None = Field(default=None, description="Investigation correlation ID")
    task_id: str | None = Field(default=None, description="AI task ID for routing")

    @classmethod
    def from_context(
        cls,
        context: Any,
        event_type: EventType,
        payload: G8eBaseModel,
        task_id: str | None = None,
    ) -> SessionEvent:
        """Create a SessionEvent safely extracting IDs from a g8e_context.

        Args:
            context: G8eHttpContext or RequestContext object.
            event_type: The EventType.
            payload: The event payload.
            task_id: Optional override for task_id. If provided, takes precedence over context.task_id.
        """
        user_id = getattr(context, "user_id", None)
        if not user_id:
            # SessionEvent requires a user_id for routing fallback
            user_id = "unknown"

        return cls(
            event_type=event_type,
            payload=payload,
            web_session_id=getattr(context, "web_session_id", None),
            cli_session_id=getattr(context, "cli_session_id", None),
            user_id=user_id,
            case_id=getattr(context, "case_id", None),
            investigation_id=getattr(context, "investigation_id", None),
            task_id=task_id if task_id is not None else getattr(context, "task_id", None),
        )

    @model_validator(mode="after")
    def _validate_session_event(self) -> SessionEvent:
        if self.web_session_id and self.cli_session_id:
            raise ValueError(
                "SessionEvent cannot set both web_session_id and cli_session_id; "
                "web and CLI are disjoint first-class session types"
            )
        if not self.web_session_id and not self.cli_session_id:
            raise ValueError(
                "SessionEvent requires exactly one of web_session_id or cli_session_id; "
                "use BackgroundEvent for user-fanout events"
            )
        _validate_sse_payload(self.event_type, self.payload)
        return self


class BackgroundEvent(G8eBaseModel):
    """System-initiated event with no connected browser session.

    client fans the event out to every active SSE session owned by user_id.
    investigation_id and case_id are optional correlation hints carried inside
    the payload - they do not drive routing.
    """

    event_type: EventType = Field(description="client event type")
    payload: G8eBaseModel = Field(description="Typed event-specific payload")
    user_id: str = Field(description="User ID to fan out to")
    investigation_id: str | None = Field(default=None, description="Investigation correlation ID")
    case_id: str | None = Field(default=None, description="Case correlation ID")
    task_id: str | None = Field(default=None, description="AI task ID for routing")

    @model_validator(mode="after")
    def _validate_background_event(self) -> BackgroundEvent:
        _validate_sse_payload(self.event_type, self.payload)
        return self


class SessionEventWire(_G8eSessionEventWire):
    """g8ee wire model for session-scoped SSE events.

    Subclasses g8e's SessionEventWire to add from_session_event() that accepts
    g8ee's internal SessionEvent routing wrapper and injects routing metadata
    into the event data dict for the client SSE consumer.
    """

    @classmethod
    def from_session_event(cls, se: SessionEvent) -> SessionEventWire:
        data = se.payload.model_dump(mode="json")
        if se.web_session_id:
            data["web_session_id"] = se.web_session_id
        if se.cli_session_id:
            data["cli_session_id"] = se.cli_session_id
        data["user_id"] = se.user_id
        if se.case_id is not None:
            data["case_id"] = se.case_id
        if se.investigation_id is not None:
            data["investigation_id"] = se.investigation_id
        if se.task_id is not None:
            data["task_id"] = se.task_id
        return cls(
            web_session_id=se.web_session_id,
            cli_session_id=se.cli_session_id,
            user_id=se.user_id,
            event=_SSEEventBody(type=se.event_type, data=data),
        )


class BackgroundEventWire(_G8eBackgroundEventWire):
    """g8ee wire model for background (user-fanout) SSE events.

    Subclasses g8e's BackgroundEventWire to add from_background_event() that
    accepts g8ee's internal BackgroundEvent routing wrapper and injects routing
    metadata into the event data dict for the client SSE consumer.
    """

    @classmethod
    def from_background_event(cls, be: BackgroundEvent) -> BackgroundEventWire:
        data = be.payload.model_dump(mode="json")
        data["user_id"] = be.user_id
        if be.investigation_id is not None:
            data["investigation_id"] = be.investigation_id
        if be.case_id is not None:
            data["case_id"] = be.case_id
        if be.task_id is not None:
            data["task_id"] = be.task_id
        return cls(
            user_id=be.user_id,
            event=_SSEEventBody(type=be.event_type, data=data),
        )
