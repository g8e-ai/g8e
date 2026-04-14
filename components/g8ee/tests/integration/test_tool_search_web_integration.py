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
Integration tests: search_web tool — request, handling, and response.

These tests exercise the full path from AIToolService.execute_tool_call with
OperatorToolName.G8E_SEARCH_WEB through MockWebSearchProvider.search() to a
typed SearchWebResult, and through build_g8e_web_search_grounding to GroundingMetadata.

    Segment 1 — tool registration
      G8E_SEARCH_WEB is registered when a web_search_provider is present.
      It is NOT registered when web_search_provider is None.

    Segment 2 — execute_tool_call routing
      G8E_SEARCH_WEB routes to the provider's search() method with the correct query.
      Result is a typed SearchWebResult.

    Segment 3 — successful search response shape
      SearchWebResult carries success=True, query, and typed WebSearchResultItem list.
      Each item has title, link, snippet fields.

    Segment 4 — failed / empty search response
      Provider returning success=False is passed through as SearchWebResult(success=False).
      Empty results list is valid (no error).

    Segment 5 — grounding metadata construction
      build_g8e_web_search_grounding produces GroundingMetadata with grounding_used=True
      when results are present.
      grounding_used=False when no results or success=False.

    Segment 6 — search_web SSE events through deliver_via_sse
      A TOOL_CALL chunk for G8E_SEARCH_WEB produces LLM_TOOL_SEARCH_WEB_REQUESTED.
      A TOOL_RESULT chunk for G8E_SEARCH_WEB with grounding produces CITATIONS event.

    Segment 7 — _search_calls records all invocations (observability)
      MockWebSearchProvider._search_calls captures every search call for assertion.

Real code under test:
    AIToolService.execute_tool_call (app/services/ai/tool_service.py)
    MockWebSearchProvider.build_g8e_web_search_grounding (real grounding logic)
    deliver_via_sse (app/services/ai/agent_sse.py) — SSE event translation

Only the WebSearchProvider network boundary is replaced:
    MockWebSearchProvider — no GCP credentials required
"""

import pytest

from app.constants import EventType, OperatorToolName, StreamChunkFromModelType, AgentMode
from app.models.agent import StreamChunkData, StreamChunkFromModel
from app.models.grounding import GroundingMetadata
from app.models.settings import LLMSettings, G8eeUserSettings
from app.models.tool_results import SearchWebResult, WebSearchResultItem
from app.services.ai.agent_sse import deliver_via_sse
from tests.fakes.agent_helpers import make_streaming_context, make_g8ed_event_service
from tests.fakes.factories import build_enriched_context, build_g8e_http_context
from tests.fakes.builder import create_mock_tool_executor
from tests.fakes.fake_web_search_provider import FakeWebSearchProvider
from tests.fakes.tool_helpers import create_tool_service_fake

pytestmark = [pytest.mark.integration]


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_search_result(
    items: list[WebSearchResultItem] | None = None,
    query: str = "test query",
) -> SearchWebResult:
    return SearchWebResult(
        success=True,
        query=query,
        results=items or [
            WebSearchResultItem(
                title="Site Reliability Engineering",
                link="https://sre.google/",
                snippet="SRE principles and practices.",
            )
        ],
    )


def _make_tool_service_with_search(search_result: SearchWebResult):
    provider = FakeWebSearchProvider(search_result=search_result or _make_search_result())
    return create_tool_service_fake(web_search_provider=provider), provider


def _investigation():
    return build_enriched_context()


def _g8e_context():
    return build_g8e_http_context()


async def _collect_sse_events(chunks, ctx=None):
    streaming_ctx = ctx or make_streaming_context()
    event_svc = make_g8ed_event_service()

    async def _gen():
        for c in chunks:
            yield c

    await deliver_via_sse(stream=_gen(), agent_streaming_context=streaming_ctx, g8ed_event_service=event_svc)
    # Check both publish and publish_investigation_event calls for compatibility
    events = []
    for call in event_svc.publish.call_args_list:
        events.append(call.args[0])
    for call in event_svc.publish_investigation_event.call_args_list:
        # Calls are made with keyword arguments
        if call.kwargs:
            from app.models.events import SessionEvent
            event = SessionEvent(
                event_type=call.kwargs.get('event_type'),
                payload=call.kwargs.get('payload'),
                web_session_id=call.kwargs.get('web_session_id'),
                user_id=call.kwargs.get('user_id'),
                case_id=call.kwargs.get('case_id'),
                investigation_id=call.kwargs.get('investigation_id'),
            )
            events.append(event)
    return events


# ---------------------------------------------------------------------------
# Segment 1 — tool registration
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
@pytest.mark.requires_web_search
class TestSearchWebToolRegistration:
    """G8E_SEARCH_WEB is registered only when a provider is injected."""

    async def test_search_web_registered_when_provider_present(self):
        svc, _ = _make_tool_service_with_search(search_result=_make_search_result())
        assert OperatorToolName.G8E_SEARCH_WEB in svc._tool_declarations

    async def test_search_web_not_registered_without_provider(self):
        svc = create_tool_service_fake(web_search_provider=None, with_run_commands_result=None)
        assert OperatorToolName.G8E_SEARCH_WEB not in svc._tool_declarations

    async def test_search_web_has_tool_declaration(self):
        svc, _ = _make_tool_service_with_search(search_result=_make_search_result())
        decl = svc._tool_declarations[OperatorToolName.G8E_SEARCH_WEB]
        assert decl is not None
        assert decl.name == OperatorToolName.G8E_SEARCH_WEB


# ---------------------------------------------------------------------------
# Segment 2 — execute_tool_call routing
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
@pytest.mark.requires_web_search
class TestSearchWebRouting:
    """G8E_SEARCH_WEB routes to provider.search() with the correct query."""

    async def test_search_called_with_query_arg(self):
        svc, provider = _make_tool_service_with_search(search_result=_make_search_result())

        await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "kubernetes node pressure"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert len(provider._search_calls) == 1
        assert provider._search_calls[0]["query"] == "kubernetes node pressure"

    async def test_result_is_search_web_result_type(self):
        svc, _ = _make_tool_service_with_search(search_result=_make_search_result())

        result = await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "linux disk usage"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert isinstance(result, SearchWebResult)

    async def test_result_carries_query(self):
        svc, _ = _make_tool_service_with_search(
            search_result=_make_search_result(query="docker container logs")
        )

        result = await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "docker container logs"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert result.query == "docker container logs"

    async def test_result_success_true_on_happy_path(self):
        svc, _ = _make_tool_service_with_search(search_result=_make_search_result())

        result = await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "nginx access log format"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert result.success is True


# ---------------------------------------------------------------------------
# Segment 3 — successful search response shape
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
@pytest.mark.requires_web_search
class TestSearchWebResponseShape:
    """SearchWebResult carries typed WebSearchResultItem list with correct fields."""

    async def test_results_list_populated(self):
        items = [
            WebSearchResultItem(title="A", link="https://a.com/", snippet="snippet A"),
            WebSearchResultItem(title="B", link="https://b.com/", snippet="snippet B"),
        ]
        svc, _ = _make_tool_service_with_search(
            search_result=_make_search_result(query="multi result", items=items)
        )

        result = await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "multi result"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert len(result.results) == 2

    async def test_result_items_are_web_search_result_item_type(self):
        items = [WebSearchResultItem(title="Go doc", link="https://go.dev/", snippet="Go docs")]
        svc, _ = _make_tool_service_with_search(
            search_result=_make_search_result(items=items)
        )

        result = await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "go documentation"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert isinstance(result.results[0], WebSearchResultItem)

    async def test_result_item_fields_preserved(self):
        items = [
            WebSearchResultItem(
                title="Prometheus docs",
                link="https://prometheus.io/docs/",
                snippet="Monitoring system & time series database.",
            )
        ]
        svc, _ = _make_tool_service_with_search(
            search_result=_make_search_result(items=items)
        )

        result = await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "prometheus monitoring"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        item = result.results[0]
        assert item.title == "Prometheus docs"
        assert item.link == "https://prometheus.io/docs/"
        assert item.snippet == "Monitoring system & time series database."


# ---------------------------------------------------------------------------
# Segment 4 — failed / empty search response
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
@pytest.mark.requires_web_search
class TestSearchWebFailureHandling:
    """Provider failure and empty results are handled as typed results."""

    async def test_provider_failure_produces_success_false(self):
        provider = FakeWebSearchProvider(
            search_result=SearchWebResult(success=False, query="bad query", error="upstream unavailable")
        )
        svc = create_tool_service_fake(web_search_provider=provider, with_run_commands_result=None)

        result = await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "bad query"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert isinstance(result, SearchWebResult)
        assert result.success is False
        assert result.error == "upstream unavailable"

    async def test_empty_results_list_is_valid(self):
        provider = FakeWebSearchProvider(
            search_result=SearchWebResult(success=True, query="no hits", results=[])
        )
        svc = create_tool_service_fake(web_search_provider=provider, with_run_commands_result=None)

        result = await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "no hits"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert isinstance(result, SearchWebResult)
        assert result.success is True
        assert result.results == []


# ---------------------------------------------------------------------------
# Segment 5 — grounding metadata construction
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
@pytest.mark.requires_web_search
class TestSearchWebGrounding:
    """build_g8e_web_search_grounding produces GroundingMetadata with correct grounding_used."""

    async def test_grounding_used_true_when_results_present(self):
        items = [WebSearchResultItem(title="Grafana", link="https://grafana.com/", snippet="Observability")]
        provider = FakeWebSearchProvider(
            search_result=_make_search_result(query="grafana dashboards", items=items)
        )
        result = SearchWebResult(success=True, query="grafana dashboards", results=items)
        grounding = provider.build_g8e_web_search_grounding(result)

        assert isinstance(grounding, GroundingMetadata)
        assert grounding.grounding_used is True

    async def test_grounding_used_false_when_no_results(self):
        provider = FakeWebSearchProvider()
        result = SearchWebResult(success=True, query="empty", results=[])
        grounding = provider.build_g8e_web_search_grounding(result)

        assert grounding.grounding_used is False

    async def test_grounding_used_false_when_success_false(self):
        provider = FakeWebSearchProvider()
        result = SearchWebResult(success=False, query="fail", error="error")
        grounding = provider.build_g8e_web_search_grounding(result)

        assert grounding.grounding_used is False

    async def test_grounding_chunks_populated_from_results(self):
        items = [
            WebSearchResultItem(title="Title A", link="https://a.example.com/", snippet="S"),
            WebSearchResultItem(title="Title B", link="https://b.example.com/", snippet="S"),
        ]
        provider = FakeWebSearchProvider(
            search_result=_make_search_result(items=items)
        )
        result = _make_search_result(items=items)
        grounding = provider.build_g8e_web_search_grounding(result)

        assert len(grounding.grounding_chunks) == 2
        assert grounding.grounding_chunks[0].uri == "https://a.example.com/"

    async def test_grounding_query_list_populated(self):
        items = [WebSearchResultItem(title="T", link="https://t.com/", snippet="s")]
        provider = FakeWebSearchProvider()
        result = SearchWebResult(success=True, query="site reliability", results=items)
        grounding = provider.build_g8e_web_search_grounding(result)

        assert "site reliability" in grounding.web_search_queries

    async def test_sources_count_matches_results(self):
        items = [
            WebSearchResultItem(title="X", link="https://x.com/", snippet="sx"),
            WebSearchResultItem(title="Y", link="https://y.com/", snippet="sy"),
            WebSearchResultItem(title="Z", link="https://z.com/", snippet="sz"),
        ]
        provider = FakeWebSearchProvider()
        result = SearchWebResult(success=True, query="test", results=items)
        grounding = provider.build_g8e_web_search_grounding(result)

        assert grounding.sources_count == 3


# ---------------------------------------------------------------------------
# Segment 6 — search_web SSE events through deliver_via_sse
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
@pytest.mark.requires_web_search
class TestSearchWebSSEEvents:
    """TOOL_CALL and TOOL_RESULT for G8E_SEARCH_WEB produce correct SSE events."""

    async def test_search_web_tool_call_fires_search_requested(self):
        chunks = [
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_CALL,
                data=StreamChunkData(
                    tool_name=OperatorToolName.G8E_SEARCH_WEB,
                    execution_id="exe_sw_001",
                    display_detail="prometheus alerting rules",
                    is_operator_tool=True,
                ),
            ),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            ),
        ]
        events = await _collect_sse_events(chunks)
        search_events = [e for e in events if e.event_type == EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED]
        assert len(search_events) == 1

    async def test_search_web_requested_event_carriesexecution_id(self):
        chunks = [
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_CALL,
                data=StreamChunkData(
                    tool_name=OperatorToolName.G8E_SEARCH_WEB,
                    execution_id="exe_sw_002",
                    display_detail="k8s pod restart loop",
                    is_operator_tool=True,
                ),
            ),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            ),
        ]
        events = await _collect_sse_events(chunks)
        search_event = next(e for e in events if e.event_type == EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED)
        assert search_event.payload.execution_id == "exe_sw_002"

    async def test_search_web_requested_event_carries_query(self):
        chunks = [
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_CALL,
                data=StreamChunkData(
                    tool_name=OperatorToolName.G8E_SEARCH_WEB,
                    execution_id="exe_sw_003",
                    display_detail="nginx upstream timeout",
                    is_operator_tool=True,
                ),
            ),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            ),
        ]
        events = await _collect_sse_events(chunks)
        search_event = next(e for e in events if e.event_type == EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED)
        assert search_event.payload.query == "nginx upstream timeout"

    async def test_citations_chunk_fires_citations_ready_event(self):
        from app.constants.settings import GroundingSource
        from app.models.grounding import GroundingChunk, GroundingMetadata
        grounding = GroundingMetadata(
            grounding_used=True,
            source=GroundingSource.WEB_SEARCH,
            web_search_queries=["grafana"],
            search_queries_count=1,
            grounding_chunks=[GroundingChunk(uri="https://grafana.com/", title="Grafana")],
            sources_count=1,
            sources=[],
        )
        chunks = [
            StreamChunkFromModel(
                type=StreamChunkFromModelType.CITATIONS,
                data=StreamChunkData(grounding_metadata=grounding, has_citations=True),
            ),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            ),
        ]
        events = await _collect_sse_events(chunks)
        citation_events = [e for e in events if e.event_type == EventType.LLM_CHAT_ITERATION_CITATIONS_RECEIVED]
        assert len(citation_events) == 1

    async def test_multiple_search_calls_each_fire_separate_events(self):
        chunks = [
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_CALL,
                data=StreamChunkData(
                    tool_name=OperatorToolName.G8E_SEARCH_WEB,
                    execution_id="exe_sw_004",
                    display_detail="query one",
                    is_operator_tool=True,
                ),
            ),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.TOOL_CALL,
                data=StreamChunkData(
                    tool_name=OperatorToolName.G8E_SEARCH_WEB,
                    execution_id="exe_sw_005",
                    display_detail="query two",
                    is_operator_tool=True,
                ),
            ),
            StreamChunkFromModel(
                type=StreamChunkFromModelType.COMPLETE,
                data=StreamChunkData(finish_reason="STOP"),
            ),
        ]
        events = await _collect_sse_events(chunks)
        search_events = [e for e in events if e.event_type == EventType.LLM_TOOL_G8E_WEB_SEARCH_REQUESTED]
        assert len(search_events) == 2


# ---------------------------------------------------------------------------
# Segment 7 — _search_calls observability
# ---------------------------------------------------------------------------


@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.integration
@pytest.mark.requires_web_search
class TestSearchWebObservability:
    """FakeWebSearchProvider._search_calls records every invocation."""

    async def test_single_search_recorded(self):
        provider = FakeWebSearchProvider(search_result=_make_search_result())
        svc = create_tool_service_fake(web_search_provider=provider, with_run_commands_result=None)

        await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "recorded query"},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert len(provider._search_calls) == 1
        assert provider._search_calls[0]["query"] == "recorded query"

    async def test_multiple_searches_all_recorded(self):
        provider = FakeWebSearchProvider(search_result=_make_search_result())
        svc = create_tool_service_fake(web_search_provider=provider, with_run_commands_result=None)

        for query in ["first", "second", "third"]:
            await svc.execute_tool_call(
                OperatorToolName.G8E_SEARCH_WEB,
                {"query": query},
                _investigation(),
                _g8e_context(),
                G8eeUserSettings(llm=LLMSettings()),
            )

        assert len(provider._search_calls) == 3
        assert [c["query"] for c in provider._search_calls] == ["first", "second", "third"]

    async def test_num_param_forwarded_to_provider(self):
        provider = FakeWebSearchProvider(search_result=_make_search_result())
        svc = create_tool_service_fake(web_search_provider=provider, with_run_commands_result=None)

        await svc.execute_tool_call(
            OperatorToolName.G8E_SEARCH_WEB,
            {"query": "num test", "num": 10},
            _investigation(),
            _g8e_context(),
            G8eeUserSettings(llm=LLMSettings()),
        )

        assert provider._search_calls[0]["num"] == 10
