# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for InvestigationSeedService.

The seed is written through the investigation, case, and memory services the
live chat path uses, in a documented order, and refused as a whole when a
history event is not seedable.
"""

from unittest.mock import AsyncMock, MagicMock

import pytest
from g8e.models.internal_api import (
    EvaluationInvestigationSeed,
    EvaluationSeedHistoryEvent,
    EvaluationSeedMemory,
    EvaluationSeedTurn,
)
from pydantic import ValidationError as PydanticValidationError

from app.constants import EventType, HistoryActor, InvestigationStatus, MessageSender
from app.errors import BusinessLogicError, ValidationError
from app.models.evaluation_trace import EvaluationSeedApplication
from app.models.http_context import G8eHttpContext
from app.models.investigations import (
    ConversationMessageMetadata,
    UserChatMetadata,
)
from app.services.evaluation.investigation_seed import (
    SEEDABLE_HISTORY_EVENTS,
    InvestigationSeedService,
)

pytestmark = pytest.mark.unit


class _Harness:
    """The seed service with every collaborator recording into one ordered log."""

    def __init__(self) -> None:
        self.log: list[str] = []
        self.data_service = MagicMock()
        self.data_service.add_chat_message = AsyncMock(
            side_effect=lambda **kw: self._record("chat:" + kw["sender"].name) or True
        )
        self.data_service.add_history_entry = AsyncMock(
            side_effect=lambda **kw: self._record("history:" + kw["event_type"].name)
        )
        self.investigation_service = MagicMock()
        self.investigation_service.investigation_data_service = self.data_service
        self.investigation_service.update_investigation = AsyncMock(
            side_effect=lambda *_a, **_kw: self._record("investigation_title")
        )
        self.investigation_service.persist_ai_message = AsyncMock(
            side_effect=lambda **kw: self._record("ai:" + kw["sender"].name) or True
        )
        self.case_service = MagicMock()
        self.case_service.update_case = AsyncMock(
            side_effect=lambda *_a, **_kw: self._record("case_title")
        )
        self.memory_service = MagicMock()
        self.memory_service.save_memory = AsyncMock(
            side_effect=lambda *_a, **_kw: self._record("case_memory")
        )
        self.service = InvestigationSeedService(
            investigation_service=self.investigation_service,
            case_service=self.case_service,
            memory_service=self.memory_service,
        )

    def _record(self, entry: str) -> None:
        self.log.append(entry)


def _context(**overrides: str | None) -> G8eHttpContext:
    fields: dict[str, str | None] = {
        "user_id": "user-1",
        "case_id": "case-1",
        "investigation_id": "inv-1",
        "web_session_id": "web-1",
    }
    fields.update(overrides)
    return G8eHttpContext.model_validate(fields)


def _grep_failure(**overrides: str) -> EvaluationSeedHistoryEvent:
    fields = {
        "event_type": "g8e.v1.operator.filesystem.grep.failed",
        "actor": "system",
        "summary": "recursive_grep_search failed for pattern AUTH_FAILURE",
        "tool_name": "recursive_grep_search",
        "execution_id": "exec-seed-1",
        "arguments_json": '{"pattern":"AUTH_FAILURE"}',
        "error": "path is required",
        "error_type": "validation_error",
    }
    fields.update(overrides)
    return EvaluationSeedHistoryEvent.model_validate(fields)


async def test_apply_writes_title_then_turns_then_history_then_memory_in_that_order():
    harness = _Harness()
    seed = EvaluationInvestigationSeed(
        case_title="Auth failure sweep (retry)",
        turns=[
            EvaluationSeedTurn(sender="user", content="Search the deploy workspace."),
            EvaluationSeedTurn(sender="primary", content="I ran a grep but it failed."),
            EvaluationSeedTurn(sender="assistant", content="Noted."),
        ],
        history_events=[_grep_failure()],
        case_memory=EvaluationSeedMemory(investigation_summary="Auth failures in deploys."),
    )

    application = await harness.service.apply(seed, _context())

    assert harness.log == [
        "case_title",
        "investigation_title",
        "chat:USER_CHAT",
        "ai:AI_PRIMARY",
        "ai:AI_ASSISTANT",
        "history:OPERATOR_FILESYSTEM_GREP_FAILED",
        "case_memory",
    ]
    assert application == EvaluationSeedApplication(turns=3, history_events=1, case_memory=True)


async def test_apply_for_a_title_only_seed_writes_only_the_title():
    harness = _Harness()

    application = await harness.service.apply(
        EvaluationInvestigationSeed(case_title="Release readiness check"), _context()
    )

    assert harness.log == ["case_title", "investigation_title"]
    assert application == EvaluationSeedApplication(turns=0, history_events=0, case_memory=False)


async def test_apply_sets_the_seed_title_on_the_case_and_the_investigation():
    harness = _Harness()

    await harness.service.apply(
        EvaluationInvestigationSeed(case_title="Checkout payment timeouts"), _context()
    )

    case_id, case_update = harness.case_service.update_case.await_args.args
    assert case_id == "case-1"
    assert case_update.title == "Checkout payment timeouts"
    investigation_id, investigation_update = (
        harness.investigation_service.update_investigation.await_args.args
    )
    assert investigation_id == "inv-1"
    assert investigation_update.case_title == "Checkout payment timeouts"


@pytest.mark.parametrize(
    ("description", "expected_fields"),
    [
        ("Checkout is failing for card payments.", {"title", "description"}),
        ("", {"title"}),
    ],
)
async def test_apply_only_overwrites_the_case_description_when_the_seed_has_one(
    description, expected_fields
):
    harness = _Harness()

    await harness.service.apply(
        EvaluationInvestigationSeed(case_title="T", case_description=description), _context()
    )

    _, case_update = harness.case_service.update_case.await_args.args
    assert case_update.model_fields_set - {"context"} == expected_fields


async def test_apply_writes_user_turns_as_user_chat_and_ai_turns_through_persist_ai_message():
    harness = _Harness()
    seed = EvaluationInvestigationSeed(
        case_title="T",
        turns=[
            EvaluationSeedTurn(sender="user", content="Question."),
            EvaluationSeedTurn(sender="primary", content="Primary answer."),
            EvaluationSeedTurn(sender="assistant", content="Assistant answer."),
        ],
    )

    await harness.service.apply(seed, _context())

    user_call = harness.data_service.add_chat_message.await_args.kwargs
    assert user_call["investigation_id"] == "inv-1"
    assert user_call["sender"] is MessageSender.USER_CHAT
    assert user_call["content"] == "Question."
    assert isinstance(user_call["metadata"], UserChatMetadata)
    ai_calls = [c.kwargs for c in harness.investigation_service.persist_ai_message.await_args_list]
    assert [(c["sender"], c["text"]) for c in ai_calls] == [
        (MessageSender.AI_PRIMARY, "Primary answer."),
        (MessageSender.AI_ASSISTANT, "Assistant answer."),
    ]
    assert {c["investigation_id"] for c in ai_calls} == {"inv-1"}


async def test_apply_records_a_prior_tool_call_with_its_real_error_in_the_history_trail():
    harness = _Harness()

    await harness.service.apply(
        EvaluationInvestigationSeed(case_title="T", history_events=[_grep_failure()]), _context()
    )

    entry = harness.data_service.add_history_entry.await_args.kwargs
    assert entry["investigation_id"] == "inv-1"
    assert entry["event_type"] is EventType.OPERATOR_FILESYSTEM_GREP_FAILED
    assert entry["actor"] is HistoryActor.SYSTEM
    # History metadata has no tool-name or arguments field, so the summary line
    # carries them; the metadata carries the rest.
    assert entry["summary"] == (
        "recursive_grep_search failed for pattern AUTH_FAILURE "
        '(tool: recursive_grep_search; arguments: {"pattern":"AUTH_FAILURE"})'
    )
    details = entry["details"]
    assert isinstance(details, ConversationMessageMetadata)
    assert details.event_type is EventType.OPERATOR_FILESYSTEM_GREP_FAILED
    assert details.execution_id == "exec-seed-1"
    assert details.error == "path is required"
    assert details.error_type == "validation_error"


async def test_apply_keeps_a_plain_summary_when_the_event_names_no_tool():
    harness = _Harness()
    event = EvaluationSeedHistoryEvent(
        event_type="g8e.v1.operator.command.execution.started",
        actor="g8eo",
        summary='Executed: tail -n 50 /var/log/payments-gateway.log — 12 lines matched "timeout"',
        command="tail -n 50 /var/log/payments-gateway.log",
    )

    await harness.service.apply(
        EvaluationInvestigationSeed(case_title="T", history_events=[event]), _context()
    )

    entry = harness.data_service.add_history_entry.await_args.kwargs
    assert entry["summary"] == event.summary
    assert entry["actor"] is HistoryActor.G8EO
    assert entry["details"].command == "tail -n 50 /var/log/payments-gateway.log"


async def test_apply_saves_case_memory_for_the_seeded_investigation_only():
    harness = _Harness()
    seed = EvaluationInvestigationSeed(
        case_title="Checkout payment timeouts",
        case_memory=EvaluationSeedMemory(
            investigation_summary="Customers see timeouts at checkout.",
            response_style="Terse.",
        ),
    )

    await harness.service.apply(seed, _context())

    (memory,), kwargs = harness.memory_service.save_memory.await_args
    assert kwargs["is_new"] is True
    assert memory.case_id == "case-1"
    assert memory.investigation_id == "inv-1"
    assert memory.user_id == "user-1"
    assert memory.status is InvestigationStatus.OPEN
    assert memory.case_title == "Checkout payment timeouts"
    assert memory.investigation_summary == "Customers see timeouts at checkout."
    assert memory.response_style == "Terse."
    harness.memory_service.save_memory.assert_awaited_once()


async def test_apply_writes_no_memory_when_the_seed_has_none():
    harness = _Harness()

    await harness.service.apply(EvaluationInvestigationSeed(case_title="T"), _context())

    harness.memory_service.save_memory.assert_not_awaited()


@pytest.mark.parametrize(
    "event_type",
    [
        "g8e.v1.operator.command.approval.requested",
        "g8e.v1.operator.file.edit.completed",
        "g8e.v1.operator.filesystem.list.completed",
    ],
)
async def test_apply_refuses_a_history_event_outside_the_allowlist_before_any_write(event_type):
    harness = _Harness()
    seed = EvaluationInvestigationSeed(
        case_title="T",
        turns=[EvaluationSeedTurn(sender="user", content="hello")],
        history_events=[
            _grep_failure(),
            EvaluationSeedHistoryEvent(event_type=event_type, actor="system", summary="x"),
        ],
    )

    with pytest.raises(ValidationError) as caught:
        await harness.service.apply(seed, _context())

    assert caught.value.error_detail.details["field"] == (
        "evaluation_context.seed.history_events[1].event_type"
    )
    assert harness.log == [], "a refused seed leaves nothing behind"


async def test_apply_refuses_an_unknown_history_event_type_before_any_write():
    harness = _Harness()
    seed = EvaluationInvestigationSeed(
        case_title="T",
        history_events=[
            EvaluationSeedHistoryEvent(
                event_type="g8e.v1.operator.does.not.exist", actor="g8eo", summary="x"
            )
        ],
    )

    with pytest.raises(ValidationError, match="unknown seed history event type"):
        await harness.service.apply(seed, _context())

    assert harness.log == []


@pytest.mark.parametrize(
    "event_type",
    ["g8e.v1.app.investigation.created", "g8e.v1.source.user.chat", "not-an-event"],
)
def test_the_protocol_model_already_refuses_events_that_are_not_operator_events(event_type):
    with pytest.raises(PydanticValidationError, match="event_type"):
        EvaluationSeedHistoryEvent(event_type=event_type, actor="system", summary="x")


def test_the_seedable_history_events_are_exactly_the_operator_tool_outcomes():
    assert {event.name for event in SEEDABLE_HISTORY_EVENTS} == {
        "OPERATOR_COMMAND_EXECUTION",
        "OPERATOR_COMMAND_FAILED",
        "OPERATOR_COMMAND_APPROVAL_REJECTED",
        "OPERATOR_FILESYSTEM_GREP_COMPLETED",
        "OPERATOR_FILESYSTEM_GREP_FAILED",
        "OPERATOR_FILESYSTEM_READ_COMPLETED",
        "OPERATOR_FILESYSTEM_READ_FAILED",
        "OPERATOR_FILE_EDIT_FAILED",
    }


@pytest.mark.parametrize(
    "context",
    [
        _context(case_id=None),
        _context(investigation_id=None),
        _context(user_id=None),
    ],
)
async def test_apply_requires_a_created_case_investigation_and_user(context):
    harness = _Harness()

    with pytest.raises(BusinessLogicError):
        await harness.service.apply(EvaluationInvestigationSeed(case_title="T"), context)

    assert harness.log == []


async def test_apply_rejects_a_blank_ai_turn_instead_of_silently_dropping_it():
    harness = _Harness()
    harness.investigation_service.persist_ai_message = AsyncMock(return_value=False)
    seed = EvaluationInvestigationSeed(
        case_title="T", turns=[EvaluationSeedTurn(sender="primary", content="   ")]
    )

    with pytest.raises(ValidationError, match="primary turn was not written"):
        await harness.service.apply(seed, _context())


async def test_apply_surfaces_a_write_failure_rather_than_continuing_with_a_partial_seed():
    harness = _Harness()
    harness.data_service.add_history_entry = AsyncMock(side_effect=RuntimeError("write failed"))
    seed = EvaluationInvestigationSeed(
        case_title="T",
        history_events=[_grep_failure()],
        case_memory=EvaluationSeedMemory(investigation_summary="s"),
    )

    with pytest.raises(RuntimeError, match="write failed"):
        await harness.service.apply(seed, _context())

    harness.memory_service.save_memory.assert_not_awaited()
