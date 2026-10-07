# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Apply an evaluation investigation seed through g8ee's own write paths.

A scored turn runs inside an investigation that already has a realistic
history: a titled case, prior conversation turns, history-trail events, and
case memory. The seed is written through the same investigation, case, and
memory services the live chat path writes through; nothing here adds a new
write path. Only the scored turn itself is a real chat request.
"""

from __future__ import annotations

import logging
from dataclasses import dataclass

from g8e.models.internal_api import (
    EvaluationInvestigationSeed,
    EvaluationSeedHistoryEvent,
    EvaluationSeedTurn,
)

from app.constants import EventType, HistoryActor, InvestigationStatus, MessageSender
from app.errors import BusinessLogicError, ValidationError
from app.models.cases import CaseUpdateRequest
from app.models.evaluation_trace import EvaluationSeedApplication
from app.models.http_context import G8eHttpContext, RequestContext
from app.models.investigations import (
    ConversationMessageMetadata,
    InvestigationUpdateRequest,
    UserChatMetadata,
)
from app.models.memory import InvestigationMemory
from app.services.data.case_data_service import CaseDataService
from app.services.investigation.investigation_service import InvestigationService
from app.services.investigation.memory_data_service import MemoryDataService

logger = logging.getLogger(__name__)

# The operator events a seeded history trail may carry. Each is a prior tool
# call or its outcome as g8eo would have recorded it; anything else (approvals,
# status changes, AI events) is not a fact a scenario needs to plant and is
# refused rather than written.
SEEDABLE_HISTORY_EVENTS: frozenset[EventType] = frozenset(
    {
        EventType.OPERATOR_COMMAND_EXECUTION,
        EventType.OPERATOR_COMMAND_FAILED,
        EventType.OPERATOR_COMMAND_APPROVAL_REJECTED,
        EventType.OPERATOR_FILESYSTEM_GREP_COMPLETED,
        EventType.OPERATOR_FILESYSTEM_GREP_FAILED,
        EventType.OPERATOR_FILESYSTEM_READ_COMPLETED,
        EventType.OPERATOR_FILESYSTEM_READ_FAILED,
        EventType.OPERATOR_FILE_EDIT_FAILED,
    }
)

_HISTORY_ACTORS: dict[str, HistoryActor] = {
    "g8eo": HistoryActor.G8EO,
    "system": HistoryActor.SYSTEM,
    "user": HistoryActor.USER,
}

_SEED_FIELD = "evaluation_context.seed"


@dataclass(frozen=True)
class _ResolvedHistoryEvent:
    """A seed history event whose type and actor were validated before any write."""

    event_type: EventType
    actor: HistoryActor
    summary: str
    details: ConversationMessageMetadata


def _history_summary(event: EvaluationSeedHistoryEvent) -> str:
    """The summary line a model reads in the history trail.

    History metadata has no field for the tool name or its arguments, so a
    prior tool call carries them on the summary line.
    """
    qualifiers: list[str] = []
    if event.tool_name:
        qualifiers.append(f"tool: {event.tool_name}")
    if event.arguments_json:
        qualifiers.append(f"arguments: {event.arguments_json}")
    if not qualifiers:
        return event.summary
    return f"{event.summary} ({'; '.join(qualifiers)})"


def _resolve_history_event(index: int, event: EvaluationSeedHistoryEvent) -> _ResolvedHistoryEvent:
    field = f"{_SEED_FIELD}.history_events[{index}]"
    try:
        event_type = EventType(event.event_type)
    except ValueError as exc:
        raise ValidationError(
            f"unknown seed history event type: {event.event_type}",
            field=f"{field}.event_type",
            constraint="known_event_type",
        ) from exc
    if event_type not in SEEDABLE_HISTORY_EVENTS:
        raise ValidationError(
            f"seed history event type is not seedable: {event.event_type}",
            field=f"{field}.event_type",
            constraint="seedable_event_type",
        )
    return _ResolvedHistoryEvent(
        event_type=event_type,
        actor=_HISTORY_ACTORS[event.actor],
        summary=_history_summary(event),
        details=ConversationMessageMetadata(
            event_type=event_type,
            execution_id=event.execution_id,
            command=event.command,
            error=event.error,
            error_type=event.error_type,
        ),
    )


class InvestigationSeedService:
    """Write an investigation seed through the investigation, case, and memory services."""

    def __init__(
        self,
        *,
        investigation_service: InvestigationService,
        case_service: CaseDataService,
        memory_service: MemoryDataService,
    ) -> None:
        self._investigation_service = investigation_service
        self._case_service = case_service
        self._memory_service = memory_service

    async def apply(
        self, seed: EvaluationInvestigationSeed, g8e_context: G8eHttpContext
    ) -> EvaluationSeedApplication:
        """Write the seed into the investigation ``g8e_context`` names.

        Order: case title and description, conversation turns, history events,
        then case memory. Every history event is validated before the first
        write, so a refused seed leaves nothing behind.
        """
        case_id = g8e_context.case_id
        investigation_id = g8e_context.investigation_id
        user_id = g8e_context.user_id
        if not case_id or not investigation_id or not user_id:
            raise BusinessLogicError(
                "an investigation seed needs a created case, investigation, and user",
                details={"case_id": case_id, "investigation_id": investigation_id},
            )

        history_events = [
            _resolve_history_event(index, event) for index, event in enumerate(seed.history_events)
        ]
        context = RequestContext.from_app_context(g8e_context)

        await self._apply_case_identity(seed, case_id, investigation_id, context)
        for turn in seed.turns:
            await self._write_turn(investigation_id, turn, context)
        data_service = self._investigation_service.investigation_data_service
        for event in history_events:
            await data_service.add_history_entry(
                investigation_id=investigation_id,
                event_type=event.event_type,
                actor=event.actor,
                summary=event.summary,
                details=event.details,
                context=context,
            )
        if seed.case_memory is not None:
            await self._memory_service.save_memory(
                InvestigationMemory(
                    case_id=case_id,
                    investigation_id=investigation_id,
                    user_id=user_id,
                    status=InvestigationStatus.OPEN,
                    case_title=seed.case_title,
                    **seed.case_memory.model_dump(),
                ),
                is_new=True,
                context=context,
            )

        application = EvaluationSeedApplication(
            turns=len(seed.turns),
            history_events=len(history_events),
            case_memory=seed.case_memory is not None,
        )
        logger.info(
            "Applied evaluation investigation seed",
            extra={
                "investigation_id": investigation_id,
                "turns": application.turns,
                "history_events": application.history_events,
                "case_memory": application.case_memory,
            },
        )
        return application

    async def _apply_case_identity(
        self,
        seed: EvaluationInvestigationSeed,
        case_id: str,
        investigation_id: str,
        context: RequestContext,
    ) -> None:
        """Replace the inline-created case's derived title with the seed's.

        The same two calls the background title generator makes, so the seeded
        case reads like any other titled case. The description is only sent when
        the seed has one, because an unset field is what leaves it untouched.
        """
        update = CaseUpdateRequest(context=context, title=seed.case_title)
        if seed.case_description:
            update.description = seed.case_description
        await self._case_service.update_case(case_id, update)
        await self._investigation_service.update_investigation(
            investigation_id,
            InvestigationUpdateRequest(context=context, case_title=seed.case_title),
        )

    async def _write_turn(
        self, investigation_id: str, turn: EvaluationSeedTurn, context: RequestContext
    ) -> None:
        if turn.sender == "user":
            written = await self._investigation_service.investigation_data_service.add_chat_message(
                investigation_id=investigation_id,
                sender=MessageSender.USER_CHAT,
                content=turn.content,
                metadata=UserChatMetadata(),
                context=context,
            )
        else:
            sender = (
                MessageSender.AI_PRIMARY if turn.sender == "primary" else MessageSender.AI_ASSISTANT
            )
            written = await self._investigation_service.persist_ai_message(
                investigation_id=investigation_id,
                text=turn.content,
                context=context,
                sender=sender,
            )
        if not written:
            raise ValidationError(
                f"seed {turn.sender} turn was not written",
                field=f"{_SEED_FIELD}.turns",
                constraint="non_blank_content",
            )


__all__ = ["SEEDABLE_HISTORY_EVENTS", "InvestigationSeedService"]
