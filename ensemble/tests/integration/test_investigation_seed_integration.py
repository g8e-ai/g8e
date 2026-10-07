# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Integration test: a seeded fact is reachable through the real tool handler.

The seed is written by the real ``InvestigationSeedService`` from the full
``ServiceFactory`` graph (real investigation, case, and memory services over a
write-through document store), and the fact is read back by the real
``query_investigation_context`` handler, the way a scored model reaches it.

    Segment 1 - History-trail and operator-actions queries return the fact
    Segment 2 - The case carries the scenario title and case memory is stored
    Segment 3 - A refused seed writes nothing

Real code under test:
    InvestigationSeedService (app/services/evaluation/investigation_seed.py)
    InvestigationService / InvestigationDataService / CaseDataService / MemoryDataService
    query_investigation_context (app/services/ai/tools/query_investigation_context.py)
"""

import json
from unittest.mock import MagicMock

import pytest
import pytest_asyncio
from g8e.models.internal_api import (
    EvaluationInvestigationSeed,
    EvaluationSeedHistoryEvent,
    EvaluationSeedMemory,
    EvaluationSeedTurn,
)

from app.errors import ValidationError
from app.models.cases import CaseCreateRequest
from app.models.http_context import G8eHttpContext
from app.models.investigations import InvestigationCreateRequest
from app.models.settings import G8eeUserSettings
from app.models.tool_results import InvestigationContextResult
from app.services.ai.tools import query_investigation_context
from app.services.service_factory import ServiceFactory
from tests.fakes.factories import build_enriched_context
from tests.integration.cleanup import IntegrationCleanupTracker
from tests.integration.conftest import make_write_through_governance_client

pytestmark = [pytest.mark.integration]

FACT = "PAYMENT_TIMEOUT seen at 08:02"


@pytest.fixture
def cache_aside_service(fake_cache_aside_service):
    return fake_cache_aside_service


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def all_services(cache_aside_service, test_settings):

    services = ServiceFactory.create_all_services(
        test_settings,
        cache_aside_service,
        db_service=MagicMock(),
        kv_service=MagicMock(),
        blob_service=MagicMock(),
        governance_client=make_write_through_governance_client(cache_aside_service),
    )
    yield services
    await ServiceFactory.stop_services(services)


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def cleanup(cache_aside_service):

    tracker = IntegrationCleanupTracker(cache_aside_service)
    yield tracker
    await tracker.cleanup()


def _seed() -> EvaluationInvestigationSeed:
    return EvaluationInvestigationSeed(
        case_title="Checkout failures during deploy",
        case_description="Card payments time out right after each deploy.",
        turns=[EvaluationSeedTurn(sender="user", content="Checkout is failing after deploys.")],
        history_events=[
            EvaluationSeedHistoryEvent(
                event_type="g8e.v1.operator.command.execution.started",
                actor="g8eo",
                summary=(
                    "Executed: tail -n 50 /var/log/payments-gateway.log"
                    f' — 12 lines matched "{FACT}"'
                ),
                command="tail -n 50 /var/log/payments-gateway.log",
            )
        ],
        case_memory=EvaluationSeedMemory(
            investigation_summary="Customers report checkout timeouts after deploys."
        ),
    )


@pytest_asyncio.fixture(scope="function", loop_scope="session")
async def seeded_investigation(all_services, cleanup):
    """A case and investigation as the router creates them inline, then seeded."""
    user_id = "seed-user-1"
    case = await all_services.case_data_service.create_case(
        CaseCreateRequest(initial_message="Search for AUTH_FAILURE.", user_id=user_id),
        generated_title=None,
    )
    cleanup.track("cases", case.id)
    investigation = await all_services.investigation_data_service.create_investigation(
        InvestigationCreateRequest(
            case_id=case.id,
            case_title=case.title,
            case_description=case.description,
            user_id=user_id,
            created_with_case=True,
        )
    )
    cleanup.track_investigation(investigation.id)
    cleanup.track_memory(investigation.id)
    g8e_context = G8eHttpContext(
        user_id=user_id,
        case_id=case.id,
        investigation_id=investigation.id,
        web_session_id="seed-web-session",
    )

    application = await all_services.investigation_seed_service.apply(_seed(), g8e_context)

    return case, investigation, g8e_context, application


async def _query(all_services, investigation, g8e_context, data_type: str):
    result = await query_investigation_context.handle(
        all_services.tool_service,
        {"data_type": data_type},
        build_enriched_context(investigation_id=investigation.id, case_id=g8e_context.case_id),
        g8e_context,
        G8eeUserSettings(),
        "exec-seed-query",
    )
    assert isinstance(result, InvestigationContextResult)
    assert result.success, result.error
    return result


@pytest.mark.asyncio(loop_scope="session")
class TestSeededFactsAreReachableThroughTheRealHandler:
    async def test_history_trail_query_returns_the_seeded_fact(
        self, all_services, seeded_investigation
    ):
        _, investigation, g8e_context, application = seeded_investigation

        result = await _query(all_services, investigation, g8e_context, "history_trail")

        assert FACT in json.dumps(result.data, default=str)
        assert (application.turns, application.history_events, application.case_memory) == (
            1,
            1,
            True,
        )

    async def test_operator_actions_query_returns_the_seeded_fact(
        self, all_services, seeded_investigation
    ):
        _, investigation, g8e_context, _ = seeded_investigation

        result = await _query(all_services, investigation, g8e_context, "operator_actions")

        assert isinstance(result.data, str)
        assert FACT in result.data

    async def test_the_seeded_turn_is_part_of_the_conversation(
        self, all_services, seeded_investigation
    ):
        _, investigation, g8e_context, _ = seeded_investigation

        result = await _query(all_services, investigation, g8e_context, "conversation_history")

        assert isinstance(result.data, list)
        assert [m["content"] for m in result.data] == ["Checkout is failing after deploys."]


@pytest.mark.asyncio(loop_scope="session")
class TestSeedIdentityAndMemory:
    async def test_the_case_and_investigation_carry_the_scenario_title(
        self, all_services, seeded_investigation
    ):
        case, investigation, _, _ = seeded_investigation

        stored_case = await all_services.case_data_service.get_case(case.id)
        stored_investigation = await all_services.investigation_service.get_investigation(
            investigation.id
        )

        assert stored_case.title == "Checkout failures during deploy"
        assert stored_case.description == "Card payments time out right after each deploy."
        assert stored_investigation is not None
        assert stored_investigation.case_title == "Checkout failures during deploy"

    async def test_case_memory_is_stored_for_the_seeded_case_only(
        self, all_services, seeded_investigation
    ):
        case, _, g8e_context, _ = seeded_investigation

        memories = await all_services.memory_data_service.get_case_memories(
            case_id=case.id, user_id=g8e_context.user_id
        )

        assert [m.investigation_summary for m in memories] == [
            "Customers report checkout timeouts after deploys."
        ]


@pytest.mark.asyncio(loop_scope="session")
class TestARefusedSeedWritesNothing:
    async def test_an_unseedable_history_event_leaves_the_investigation_untouched(
        self, all_services, cleanup
    ):
        user_id = "seed-user-2"
        case = await all_services.case_data_service.create_case(
            CaseCreateRequest(initial_message="hello", user_id=user_id), generated_title=None
        )
        cleanup.track("cases", case.id)
        investigation = await all_services.investigation_data_service.create_investigation(
            InvestigationCreateRequest(
                case_id=case.id,
                case_title=case.title,
                case_description=case.description,
                user_id=user_id,
                created_with_case=True,
            )
        )
        cleanup.track_investigation(investigation.id)
        g8e_context = G8eHttpContext(
            user_id=user_id, case_id=case.id, investigation_id=investigation.id
        )
        seed = EvaluationInvestigationSeed(
            case_title="Should never be applied",
            turns=[EvaluationSeedTurn(sender="user", content="hello")],
            history_events=[
                EvaluationSeedHistoryEvent(
                    event_type="g8e.v1.operator.command.approval.requested",
                    actor="system",
                    summary="not seedable",
                )
            ],
        )

        with pytest.raises(ValidationError):
            await all_services.investigation_seed_service.apply(seed, g8e_context)

        stored = await all_services.investigation_service.get_investigation(investigation.id)
        assert stored is not None
        assert stored.case_title == case.title
        assert stored.conversation_history == []
