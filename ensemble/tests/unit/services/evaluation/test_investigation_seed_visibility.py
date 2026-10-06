# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""A seeded fact is reachable the way a model reaches it.

The real investigation services write the seed into an in-memory document
store, and the real ``query_investigation_context`` handler reads it back. This
is the in-process half of the seeded-fact check; the Tier 2 twin in
``tests/integration`` runs the same assertions against the platform's database.

Per the seeded investigation layout: conversation turns are part of the chat
contents, while history-trail events are reachable only through
``query_investigation_context``.
"""

import json
from types import SimpleNamespace
from typing import Any
from unittest.mock import AsyncMock, MagicMock

import pytest
from g8e.models.internal_api import (
    EvaluationInvestigationSeed,
    EvaluationSeedHistoryEvent,
    EvaluationSeedTurn,
)

from app.clients.governance_client import GovernanceClient
from app.models.http_context import G8eHttpContext
from app.models.settings import G8eeUserSettings
from app.models.tool_results import InvestigationContextResult
from app.services.ai.tool_service import AIToolService
from app.services.ai.tools import query_investigation_context
from app.services.cache.cache_aside import CacheAsideService
from app.services.evaluation.investigation_seed import InvestigationSeedService
from app.services.investigation.investigation_data_service import InvestigationDataService
from app.services.investigation.investigation_service import InvestigationService
from app.utils.hashing.ledger_hash import verify_chain
from tests.fakes.factories import build_enriched_context, create_investigation_data

pytestmark = pytest.mark.unit

FACT = "PAYMENT_TIMEOUT seen at 08:02"
LOOKED_UP_COMMAND = (
    f'Executed: tail -n 50 /var/log/payments-gateway.log — 12 lines matched "{FACT}"'
)
FAILED_GREP = "recursive_grep_search failed for pattern AUTH_FAILURE"


class _DocumentStore:
    """In-memory document map shared by the cache-aside read and the governed write stand-ins."""

    def __init__(self) -> None:
        self.documents: dict[tuple[str, str], dict[str, Any]] = {}


class _StoreCache(CacheAsideService):
    """Cache-aside read served from the in-memory document store."""

    def __init__(self, store: _DocumentStore) -> None:
        self._store = store

    async def get_document_with_cache(
        self, collection: str, document_id: str
    ) -> dict[str, Any] | None:
        document = self._store.documents.get((collection, document_id))
        return json.loads(json.dumps(document)) if document is not None else None


class _StoreGovernance(GovernanceClient):
    """Governed write boundary that applies updates to the in-memory document store."""

    def __init__(self, store: _DocumentStore) -> None:
        self._store = store

    async def update_governed_doc(
        self,
        collection: str,
        document_id: str,
        updates: dict[str, Any],
        event_type: str,
        *,
        case_id: str | None = None,
        investigation_id: str | None = None,
        task_id: str | None = None,
        web_session_id: str | None = None,
        user_id: str | None = None,
        operator_id: str | None = None,
        operator_session_id: str | None = None,
        merge: bool = True,
    ) -> dict[str, Any]:
        del event_type, case_id, investigation_id, task_id, web_session_id
        del user_id, operator_id, operator_session_id
        key = (collection, document_id)
        documents = self._store.documents
        documents[key] = {**documents.get(key, {}), **updates} if merge else updates
        return {"status": "accepted"}


@pytest.fixture
async def seeded():
    store = _DocumentStore()
    investigation = create_investigation_data(
        investigation_id="inv-1", case_id="case-1", user_id="user-1"
    )
    store.documents[("investigations", "inv-1")] = investigation.model_dump(mode="json")

    data_service = InvestigationDataService(_StoreCache(store), _StoreGovernance(store))
    investigation_service = InvestigationService(
        investigation_data_service=data_service,
        operator_data_service=AsyncMock(),
        memory_data_service=AsyncMock(),
        event_service=AsyncMock(),
    )
    seed_service = InvestigationSeedService(
        investigation_service=investigation_service,
        case_service=AsyncMock(),
        memory_service=AsyncMock(),
    )
    g8e_context = G8eHttpContext(
        user_id="user-1", case_id="case-1", investigation_id="inv-1", web_session_id="web-1"
    )
    await seed_service.apply(
        EvaluationInvestigationSeed(
            case_title="Checkout failures during deploy",
            turns=[
                EvaluationSeedTurn(sender="user", content="Auth failures spiked after rotation."),
                EvaluationSeedTurn(sender="primary", content="Starting at 08:02, after rotation."),
            ],
            history_events=[
                EvaluationSeedHistoryEvent(
                    event_type="g8e.v1.operator.command.execution.started",
                    actor="g8eo",
                    summary=LOOKED_UP_COMMAND,
                    command="tail -n 50 /var/log/payments-gateway.log",
                ),
                EvaluationSeedHistoryEvent(
                    event_type="g8e.v1.operator.filesystem.grep.failed",
                    actor="system",
                    summary=FAILED_GREP,
                    tool_name="recursive_grep_search",
                    error="path is required",
                ),
            ],
        ),
        g8e_context,
    )
    return SimpleNamespace(
        store=store,
        investigation_service=investigation_service,
        g8e_context=g8e_context,
        # The chain is anchored on the investigation's creation time as it was
        # when the first message was appended.
        created_at=investigation.created_at.isoformat(),
    )


async def _query(seeded, data_type: str) -> InvestigationContextResult:
    svc = MagicMock(spec=AIToolService)
    svc.investigation_service = seeded.investigation_service
    result = await query_investigation_context.handle(
        svc,
        {"data_type": data_type},
        build_enriched_context(investigation_id="inv-1", case_id="case-1"),
        seeded.g8e_context,
        G8eeUserSettings(),
        "exec-query",
    )
    assert isinstance(result, InvestigationContextResult)
    assert result.success, result.error
    return result


async def test_a_seeded_fact_is_returned_by_the_history_trail_query(seeded):
    result = await _query(seeded, "history_trail")

    assert FACT in json.dumps(result.data, default=str)
    assert FAILED_GREP in json.dumps(result.data, default=str)


async def test_a_seeded_operator_command_is_returned_by_the_operator_actions_query(seeded):
    result = await _query(seeded, "operator_actions")

    assert isinstance(result.data, str)
    assert FACT in result.data
    assert FAILED_GREP not in result.data, "a failed grep is a history-trail fact, not an action"


async def test_seeded_turns_are_conversation_history_the_model_is_shown_inline(seeded):
    result = await _query(seeded, "conversation_history")

    assert isinstance(result.data, list)
    assert [message["content"] for message in result.data] == [
        "Auth failures spiked after rotation.",
        "Starting at 08:02, after rotation.",
    ]


async def test_the_seeded_conversation_keeps_a_valid_hash_chain(seeded):
    document = seeded.store.documents[("investigations", "inv-1")]

    valid, bad_index = verify_chain(
        entries=document["conversation_history"],
        investigation_id="inv-1",
        created_at=seeded.created_at,
    )

    assert valid is True, f"chain broken at entry {bad_index}"


async def test_the_seeded_history_trail_validates_when_read_back_as_an_investigation(seeded):
    investigation = await seeded.investigation_service.get_investigation("inv-1")

    assert investigation is not None
    summaries = [entry.summary for entry in investigation.history_trail]
    assert LOOKED_UP_COMMAND in summaries
    assert any(summary.startswith(FAILED_GREP) for summary in summaries)
    assert investigation.case_title == "Checkout failures during deploy"
