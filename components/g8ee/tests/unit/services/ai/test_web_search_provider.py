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
Unit tests for WebSearchProvider.

- search: happy path, caps num, empty results, unexpected exception
- search: retries on timeout, retries on retryable GoogleAPICallError, raises NetworkError on non-retryable, returns error after exhausted retries
- extract_source_info: derives domain, display name, and favicon from URI/title
- add_inline_citations: strips auto-citations, assigns numbers, reinserts at segment boundaries
- normalize_citation_numbers: renumbers HTML anchor citation tags sequentially
"""

import asyncio
import unittest.mock as mock

import pytest
from google.api_core.exceptions import ServiceUnavailable, InvalidArgument

from app.errors import NetworkError
from app.models.grounding import (
    GroundingChunk,
    GroundingMetadata,
    GroundingSegment,
    GroundingSupport,
)
from app.services.ai.grounding.web_search_provider import WebSearchProvider

pytestmark = [pytest.mark.unit]

_PATCH_CLIENT = "app.services.ai.grounding.web_search_provider.discoveryengine.SearchServiceClient"
_PATCH_CREDENTIALS = "app.services.ai.grounding.web_search_provider.ApiKeyCredentials"
_PATCH_TO_THREAD = "app.services.ai.grounding.web_search_provider.asyncio.to_thread"
_PATCH_WAIT_FOR = "app.services.ai.grounding.web_search_provider.asyncio.wait_for"
_PATCH_SLEEP = "app.services.ai.grounding.web_search_provider.asyncio.sleep"


def _make_provider() -> WebSearchProvider:
    with mock.patch(_PATCH_CREDENTIALS), mock.patch(_PATCH_CLIENT):
        return WebSearchProvider(project_id="test-project", engine_id="test-engine", api_key="test-key")


@pytest.fixture
def provider() -> WebSearchProvider:
    with mock.patch(_PATCH_CREDENTIALS), mock.patch(_PATCH_CLIENT):
        return WebSearchProvider(project_id="test-project", engine_id="test-engine", api_key="test-key")


def _make_sdk_result(title: str, link: str, snippet: str) -> mock.MagicMock:
    result = mock.MagicMock()
    result.document.derived_struct_data = {
        "title": title,
        "link": link,
        "snippets": [{"snippet": snippet}],
    }
    return result


def _make_pager(results: list, total_size: int = 42) -> mock.MagicMock:
    pager = mock.MagicMock()
    pager.__iter__ = mock.Mock(return_value=iter(results))
    pager.total_size = total_size
    return pager


class TestWebSearchProviderSearch:
    """WebSearchProvider.search happy path and error handling."""

    @pytest.mark.asyncio
    async def test_returns_results_on_success(self):
        sdk_results = [
            _make_sdk_result("Result One", "https://a.com/1", "First result"),
            _make_sdk_result("Result Two", "https://b.com/2", "Second result"),
        ]
        pager = _make_pager(sdk_results, total_size=42)
        provider = _make_provider()

        with mock.patch.object(provider, "_execute_search_lite", new_callable=mock.AsyncMock) as mock_exec:
            mock_exec.return_value = pager
            result = await provider.search("test query", num=2)

        assert result.success is True
        assert result.query == "test query"
        assert len(result.results) == 2
        assert result.results[0].title == "Result One"
        assert result.results[0].link == "https://a.com/1"
        assert result.results[0].snippet == "First result"
        assert result.total_results == "42"
        assert result.error is None

    @pytest.mark.asyncio
    async def test_caps_num_at_25(self):
        pager = _make_pager([], total_size=0)
        provider = _make_provider()

        with mock.patch.object(provider, "_execute_search_lite", new_callable=mock.AsyncMock) as mock_exec:
            mock_exec.return_value = pager
            await provider.search("query", num=99)
            # Verify the request passed to search_lite has page_size capped
            args, _ = mock_exec.call_args
            request = args[0]
            assert request.page_size == 25

    @pytest.mark.asyncio
    async def test_empty_results_returns_empty_list(self):
        pager = _make_pager([], total_size=0)
        provider = _make_provider()

        with mock.patch.object(provider, "_execute_search_lite", new_callable=mock.AsyncMock) as mock_exec:
            mock_exec.return_value = pager
            result = await provider.search("query")

        assert result.success is True
        assert result.results == []

    @pytest.mark.asyncio
    async def test_unexpected_exception_returns_error_result(self):
        provider = _make_provider()

        with mock.patch.object(provider, "_execute_search_lite", new_callable=mock.AsyncMock) as mock_exec:
            mock_exec.side_effect = RuntimeError("unexpected")
            result = await provider.search("query")

        assert result.success is False
        assert result.error is not None
        assert "unexpected" in result.error

    @pytest.mark.asyncio
    async def test_total_results_uses_pager_total_size(self):
        sdk_results = [_make_sdk_result("T", "https://x.com", "s")]
        pager = _make_pager(sdk_results, total_size=999)
        provider = _make_provider()

        with mock.patch.object(provider, "_execute_search_lite", new_callable=mock.AsyncMock) as mock_exec:
            mock_exec.return_value = pager
            result = await provider.search("query", num=1)

        assert result.total_results == "999"

    @pytest.mark.asyncio
    async def test_result_count_capped_at_num(self):
        sdk_results = [
            _make_sdk_result(f"R{i}", f"https://x.com/{i}", "s") for i in range(10)
        ]
        pager = _make_pager(sdk_results, total_size=100)
        provider = _make_provider()

        with mock.patch.object(provider, "_execute_search_lite", new_callable=mock.AsyncMock) as mock_exec:
            mock_exec.return_value = pager
            result = await provider.search("query", num=3)

        assert len(result.results) == 3

    @pytest.mark.asyncio
    async def test_constructs_correct_serving_config(self):
        pager = _make_pager([], total_size=0)

        with mock.patch(_PATCH_CREDENTIALS), mock.patch(_PATCH_CLIENT):
            p = WebSearchProvider(project_id="my-proj", engine_id="my-engine", api_key="test-key", location="global")

        with mock.patch.object(p, "_execute_search_lite", new_callable=mock.AsyncMock) as mock_exec:
            mock_exec.return_value = pager
            await p.search("query")
            args, _ = mock_exec.call_args
            request = args[0]
            assert "my-proj" in request.serving_config
            assert "my-engine" in request.serving_config


class TestWebSearchProviderRetry:
    """WebSearchProvider.search retry and error handling."""

    @pytest.mark.asyncio
    async def test_retries_on_timeout_and_succeeds(self):
        pager = _make_pager([_make_sdk_result("T", "https://x.com", "s")], total_size=1)
        provider = _make_provider()
        call_count = 0

        async def exec_side_effect(request):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                raise asyncio.TimeoutError()
            return pager

        with mock.patch.object(provider, "_execute_search_lite", side_effect=exec_side_effect):
            with mock.patch(_PATCH_SLEEP, new=mock.AsyncMock()):
                result = await provider.search("query")

        assert result.success is True
        assert call_count == 2

    @pytest.mark.asyncio
    async def test_retries_on_retryable_api_error_and_succeeds(self):
        pager = _make_pager([_make_sdk_result("T", "https://x.com", "s")], total_size=1)
        provider = _make_provider()
        call_count = 0

        retryable_error = ServiceUnavailable("service unavailable")

        async def exec_side_effect(request):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                raise retryable_error
            return pager

        with mock.patch.object(provider, "_execute_search_lite", side_effect=exec_side_effect):
            with mock.patch(_PATCH_SLEEP, new=mock.AsyncMock()):
                result = await provider.search("query")

        assert result.success is True
        assert call_count == 2

    @pytest.mark.asyncio
    async def test_raises_network_error_on_non_retryable_api_error(self):
        provider = _make_provider()
        non_retryable = InvalidArgument("bad request")

        with mock.patch.object(provider, "_execute_search_lite", side_effect=non_retryable):
            with pytest.raises(NetworkError):
                await provider.search("query")

    @pytest.mark.asyncio
    async def test_returns_error_result_after_all_retries_exhausted(self):
        provider = _make_provider()

        with mock.patch.object(provider, "_execute_search_lite", side_effect=asyncio.TimeoutError()):
            with mock.patch(_PATCH_SLEEP, new=mock.AsyncMock()):
                result = await provider.search("query")

        assert result.success is False
        assert result.error is not None

    @pytest.mark.asyncio
    async def test_sleeps_between_retries(self):
        pager = _make_pager([], total_size=0)
        provider = _make_provider()
        call_count = 0

        async def exec_side_effect(request):
            nonlocal call_count
            call_count += 1
            if call_count < 3:
                raise asyncio.TimeoutError()
            return pager

        with mock.patch.object(provider, "_execute_search_lite", side_effect=exec_side_effect):
            with mock.patch(_PATCH_SLEEP, new=mock.AsyncMock()) as mock_sleep:
                await provider.search("query")

        assert mock_sleep.call_count == 2


class TestExtractSourceInfo:
    """extract_source_info derives display metadata from URI and title."""

    def test_standard_domain_extraction(self, provider):
        result = provider.extract_source_info("https://reuters.com/article", "reuters.com")
        assert result.domain == "reuters.com"
        assert result.display_name == "Reuters"
        assert result.favicon_url is not None
        assert "google.com/s2/favicons" in result.favicon_url

    def test_strips_www_prefix(self, provider):
        result = provider.extract_source_info("https://proxy.url", "www.bbc.com")
        assert result.domain == "bbc.com"
        assert result.display_name == "Bbc"

    def test_hyphenated_domain_titlecased(self, provider):
        result = provider.extract_source_info("https://proxy.url", "my-news-site.org")
        assert result.domain == "my-news-site.org"
        assert result.display_name == "My News Site"

    def test_empty_title_falls_back_to_empty_string(self, provider):
        result = provider.extract_source_info("https://proxy.url", "")
        assert result.domain == ""
        assert result.display_name == "Web Source"
        assert result.favicon_url is None

    def test_uri_stored_on_result(self, provider):
        result = provider.extract_source_info("https://reuters.com/news", "reuters.com")
        assert result.uri == "https://reuters.com/news"

    def test_citation_num_defaults_to_zero(self, provider):
        result = provider.extract_source_info("https://x.com", "x.com")
        assert result.citation_num == 0

    def test_handles_exception_gracefully(self, provider):
        """Malformed inputs do not raise."""
        result = provider.extract_source_info(None, None)
        assert result.display_name in ("Web Source", "unknown", "")


class TestAddInlineCitations:
    """add_inline_citations strips auto-citations then reinserts at segment boundaries."""

    def _make_metadata(
        self,
        chunks: list[GroundingChunk],
        supports: list[GroundingSupport],
    ) -> GroundingMetadata:
        return GroundingMetadata(
            grounding_used=True,
            grounding_chunks=chunks,
            grounding_supports=supports,
        )

    def test_returns_text_unchanged_when_grounding_not_used(self, provider):
        text = "Plain response."
        metadata = GroundingMetadata(grounding_used=False)
        assert provider.add_inline_citations(text, metadata) == text

    def test_returns_text_when_no_supports(self, provider):
        text = "Response text."
        metadata = GroundingMetadata(
            grounding_used=True,
            grounding_chunks=[GroundingChunk(uri="https://x.com", title="x.com")],
            grounding_supports=[],
        )
        assert provider.add_inline_citations(text, metadata) == text

    def test_returns_text_when_no_chunks(self, provider):
        text = "Response text."
        metadata = GroundingMetadata(
            grounding_used=True,
            grounding_chunks=[],
            grounding_supports=[
                GroundingSupport(
                    segment=GroundingSegment(start_index=0, end_index=14, text="Response text."),
                    grounding_chunk_indices=[0],
                    text="Response text.",
                )
            ],
        )
        assert provider.add_inline_citations(text, metadata) == text

    def test_strips_llm_auto_citations(self, provider):
        """Auto-inserted citations [1] and [2,3] are stripped before reinsertion."""
        text = "AI is growing[1] rapidly[2,3] in 2026."
        metadata = self._make_metadata(
            chunks=[GroundingChunk(uri="https://example.com", title="example.com")],
            supports=[
                GroundingSupport(
                    segment=GroundingSegment(start_index=0, end_index=14, text="AI is growing"),
                    grounding_chunk_indices=[0],
                    text="AI is growing",
                )
            ],
        )
        result = provider.add_inline_citations(text, metadata)
        assert "[2,3]" not in result

    def test_inserts_citation_at_end_index(self, provider):
        text = "AI is advancing. More details here."
        metadata = self._make_metadata(
            chunks=[GroundingChunk(uri="https://example.com", title="example.com")],
            supports=[
                GroundingSupport(
                    segment=GroundingSegment(start_index=0, end_index=16, text="AI is advancing."),
                    grounding_chunk_indices=[0],
                    text="AI is advancing.",
                )
            ],
        )
        result = provider.add_inline_citations(text, metadata)
        assert result == "AI is advancing.[1] More details here."

    def test_multiple_sources_produce_grouped_citation(self, provider):
        text = "Both sources agree on this point."
        metadata = self._make_metadata(
            chunks=[
                GroundingChunk(uri="https://a.com", title="a.com"),
                GroundingChunk(uri="https://b.com", title="b.com"),
            ],
            supports=[
                GroundingSupport(
                    segment=GroundingSegment(start_index=0, end_index=32, text="Both sources agree on this point."),
                    grounding_chunk_indices=[0, 1],
                    text="Both sources agree on this point.",
                )
            ],
        )
        result = provider.add_inline_citations(text, metadata)
        assert "[1,2]" in result

    def test_populates_sources_on_metadata(self, provider):
        """add_inline_citations mutates grounding_metadata.sources in place."""
        text = "Test response text here."
        metadata = self._make_metadata(
            chunks=[GroundingChunk(uri="https://reuters.com/news", title="reuters.com")],
            supports=[
                GroundingSupport(
                    segment=GroundingSegment(start_index=0, end_index=24, text="Test response text here."),
                    grounding_chunk_indices=[0],
                    text="Test response text here.",
                )
            ],
        )
        provider.add_inline_citations(text, metadata)
        assert len(metadata.sources) == 1
        assert metadata.sources[0].domain == "reuters.com"
        assert metadata.sources[0].citation_num == 1
        assert metadata.sources[0].uri == "https://reuters.com/news"

    def test_skips_citation_when_end_index_exceeds_text_length(self, provider):
        text = "Short."
        metadata = self._make_metadata(
            chunks=[GroundingChunk(uri="https://example.com", title="example.com")],
            supports=[
                GroundingSupport(
                    segment=GroundingSegment(start_index=0, end_index=9999, text="Short."),
                    grounding_chunk_indices=[0],
                    text="Short.",
                )
            ],
        )
        result = provider.add_inline_citations(text, metadata)
        assert "Short." in result

    def test_skips_support_with_empty_chunk_indices(self, provider):
        text = "Some response."
        metadata = self._make_metadata(
            chunks=[GroundingChunk(uri="https://example.com", title="example.com")],
            supports=[
                GroundingSupport(
                    segment=GroundingSegment(start_index=0, end_index=14, text="Some response."),
                    grounding_chunk_indices=[],
                    text="Some response.",
                )
            ],
        )
        result = provider.add_inline_citations(text, metadata)
        assert "[" not in result

    def test_multiple_supports_inserted_in_correct_order(self, provider):
        """Supports are applied back-to-front so earlier insertions don't shift end indices."""
        text = "First claim. Second claim."
        metadata = self._make_metadata(
            chunks=[
                GroundingChunk(uri="https://a.com", title="a.com"),
                GroundingChunk(uri="https://b.com", title="b.com"),
            ],
            supports=[
                GroundingSupport(
                    segment=GroundingSegment(start_index=0, end_index=12, text="First claim."),
                    grounding_chunk_indices=[0],
                    text="First claim.",
                ),
                GroundingSupport(
                    segment=GroundingSegment(start_index=13, end_index=26, text="Second claim."),
                    grounding_chunk_indices=[1],
                    text="Second claim.",
                ),
            ],
        )
        result = provider.add_inline_citations(text, metadata)
        assert "[1]" in result
        assert "[2]" in result

    def test_returns_original_text_on_exception(self, provider):
        """On any error, returns the original text — never raises."""
        original = "Safe fallback text."

        class BadMetadata:
            grounding_used = True
            @property
            def grounding_supports(self):
                raise RuntimeError("corrupted")
            grounding_chunks = []

        result = provider.add_inline_citations(original, BadMetadata())
        assert result == original


class TestBuildG8eWebSearchGrounding:
    """build_g8e_web_search_grounding constructs GroundingMetadata from SearchWebResult."""

    def test_returns_grounding_used_true_with_results(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="test query",
            results=[
                WebSearchResultItem(title="reuters.com", link="https://reuters.com/news", snippet="s1"),
                WebSearchResultItem(title="bbc.com", link="https://bbc.com/article", snippet="s2"),
            ],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert gm.grounding_used is True

    def test_source_is_web_search(self, provider):
        from app.constants.settings import GroundingSource
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="q",
            results=[WebSearchResultItem(title="x.com", link="https://x.com", snippet="s")],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert gm.source == GroundingSource.WEB_SEARCH

    def test_sources_populated_from_results(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="q",
            results=[
                WebSearchResultItem(title="a.com", link="https://a.com/1", snippet="s1"),
                WebSearchResultItem(title="b.com", link="https://b.com/2", snippet="s2"),
            ],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert len(gm.sources) == 2
        assert gm.sources[0].uri == "https://a.com/1"
        assert gm.sources[1].uri == "https://b.com/2"

    def test_citation_numbers_are_sequential_from_one(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="q",
            results=[
                WebSearchResultItem(title="a.com", link="https://a.com", snippet="s"),
                WebSearchResultItem(title="b.com", link="https://b.com", snippet="s"),
                WebSearchResultItem(title="c.com", link="https://c.com", snippet="s"),
            ],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert [s.citation_num for s in gm.sources] == [1, 2, 3]

    def test_grounding_chunks_match_sources(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="q",
            results=[
                WebSearchResultItem(title="x.com", link="https://x.com/page", snippet="s"),
            ],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert len(gm.grounding_chunks) == 1
        assert gm.grounding_chunks[0].uri == "https://x.com/page"
        assert gm.grounding_chunks[0].title == "x.com"

    def test_web_search_query_populated(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="my search query",
            results=[WebSearchResultItem(title="x.com", link="https://x.com", snippet="s")],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert gm.web_search_queries == ["my search query"]
        assert gm.search_queries_count == 1

    def test_sources_count_matches(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="q",
            results=[
                WebSearchResultItem(title="a.com", link="https://a.com", snippet="s"),
                WebSearchResultItem(title="b.com", link="https://b.com", snippet="s"),
            ],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert gm.sources_count == 2

    def test_grounding_supports_is_empty(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="q",
            results=[WebSearchResultItem(title="x.com", link="https://x.com", snippet="s")],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert gm.grounding_supports == []

    def test_failed_result_returns_grounding_not_used(self, provider):
        from app.models.tool_results import SearchWebResult
        result = SearchWebResult(success=False, query="q", error="timeout")
        gm = provider.build_g8e_web_search_grounding(result)
        assert gm.grounding_used is False

    def test_empty_results_returns_grounding_not_used(self, provider):
        from app.models.tool_results import SearchWebResult
        result = SearchWebResult(success=True, query="q", results=[])
        gm = provider.build_g8e_web_search_grounding(result)
        assert gm.grounding_used is False

    def test_items_with_no_link_are_skipped(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="q",
            results=[
                WebSearchResultItem(title="no-link", link="", snippet="s"),
                WebSearchResultItem(title="has-link.com", link="https://has-link.com", snippet="s"),
            ],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert len(gm.sources) == 1
        assert gm.sources[0].uri == "https://has-link.com"

    def test_all_items_no_link_returns_grounding_not_used(self, provider):
        from app.models.tool_results import SearchWebResult, WebSearchResultItem
        result = SearchWebResult(
            success=True,
            query="q",
            results=[
                WebSearchResultItem(title="a", link="", snippet="s"),
                WebSearchResultItem(title="b", link="", snippet="s"),
            ],
        )
        gm = provider.build_g8e_web_search_grounding(result)
        assert gm.grounding_used is False


class TestNormalizeCitationNumbers:
    """normalize_citation_numbers renumbers HTML anchor tags sequentially by URI."""

    def test_renumbers_sequentially_by_first_occurrence(self, provider):
        html = (
            'Source <a href="https://b.com" title="B">5</a> and '
            '<a href="https://a.com" title="A">3</a> and '
            '<a href="https://b.com" title="B">5</a>'
        )
        result = provider.normalize_citation_numbers(html)
        assert ">1</a>" in result
        assert ">2</a>" in result
        assert ">5</a>" not in result
        assert result.count(">1</a>") == 2
        assert result.count(">2</a>") == 1

    def test_preserves_title_and_other_attributes(self, provider):
        html = '<a href="https://x.com" title="X Source" class="cite">7</a>'
        result = provider.normalize_citation_numbers(html)
        assert 'title="X Source"' in result
        assert 'class="cite"' in result
        assert ">1</a>" in result

    def test_returns_empty_string_unchanged(self, provider):
        assert provider.normalize_citation_numbers("") == ""

    def test_returns_none_unchanged(self, provider):
        assert provider.normalize_citation_numbers(None) is None

    def test_no_citations_returns_text_unchanged(self, provider):
        text = "No citations in this text."
        assert provider.normalize_citation_numbers(text) == text

    def test_single_citation_becomes_one(self, provider):
        html = '<a href="https://only.com" title="Only">42</a>'
        result = provider.normalize_citation_numbers(html)
        assert ">1</a>" in result
        assert ">42</a>" not in result

    def test_three_distinct_uris_numbered_in_appearance_order(self, provider):
        html = (
            '<a href="https://c.com" title="C">9</a> '
            '<a href="https://a.com" title="A">3</a> '
            '<a href="https://b.com" title="B">5</a>'
        )
        result = provider.normalize_citation_numbers(html)
        assert ">1</a>" in result
        assert ">2</a>" in result
        assert ">3</a>" in result
        assert ">9</a>" not in result
