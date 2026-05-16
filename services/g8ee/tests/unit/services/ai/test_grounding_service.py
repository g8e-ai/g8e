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
Unit tests for GroundingService.

Pure logic methods (no external dependencies):
- extract_provider_grounding: Converts SdkGroundingRawData into GroundingMetadata
"""

import pytest

from app.llm.llm_types import (
    SdkGroundingChunk,
    SdkGroundingRawData,
    SdkGroundingSegment,
    SdkGroundingSupport,
    SdkGroundingWebSource,
    SdkSearchEntryPoint,
)
from app.services.ai.grounding.grounding_service import GroundingService

pytestmark = [pytest.mark.unit]


@pytest.fixture
def service():
    return GroundingService()


def _make_sdk_chunk(uri: str, title: str) -> SdkGroundingChunk:
    return SdkGroundingChunk(web=SdkGroundingWebSource(uri=uri, title=title))


def _make_sdk_support(
    start: int, end: int, text: str, indices: list[int]
) -> SdkGroundingSupport:
    return SdkGroundingSupport(
        segment=SdkGroundingSegment(start_index=start, end_index=end, text=text),
        grounding_chunk_indices=indices,
    )


def _make_raw(
    queries: list[str],
    chunks: list[SdkGroundingChunk],
    supports: list[SdkGroundingSupport],
    entry_point: SdkSearchEntryPoint,
) -> SdkGroundingRawData:
    return SdkGroundingRawData(
        web_search_queries=queries or [],
        grounding_chunks=chunks or [],
        grounding_supports=supports or [],
        search_entry_point=entry_point,
    )


class TestExtractProviderGrounding:
    """extract_provider_grounding converts SdkGroundingRawData into GroundingMetadata."""

    def test_returns_grounding_used_true_with_queries(self, service):
        raw = _make_raw(
            queries=["latest AI news", "gemini api updates"],
            chunks=[],
            supports=[],
            entry_point=None,
        )
        result = service.extract_provider_grounding(raw)
        assert result.grounding_used is True
        assert result.web_search_queries == ["latest AI news", "gemini api updates"]
        assert result.search_queries_count == 2

    def test_returns_grounding_used_true_with_empty_queries(self, service):
        """Even with no queries, grounding_used is True when raw data is present."""
        raw = _make_raw(
            queries=[],
            chunks=[_make_sdk_chunk("https://example.com", "example.com")],
            supports=[],
            entry_point=None,
        )
        result = service.extract_provider_grounding(raw)
        assert result.grounding_used is True

    def test_source_is_provider_native(self, service):
        """Provider grounding always sets source=PROVIDER_NATIVE."""
        from app.constants.settings import GroundingSource
        raw = _make_raw(
            queries=["query"],
            chunks=[],
            supports=[],
            entry_point=None,
        )
        result = service.extract_provider_grounding(raw)
        assert result.source == GroundingSource.PROVIDER_NATIVE

    def test_extracts_web_chunks(self, service):
        raw = _make_raw(
            queries=["query"],
            chunks=[
                _make_sdk_chunk("https://reuters.com/news", "reuters.com"),
                _make_sdk_chunk("https://bbc.com/article", "bbc.com"),
            ],
            supports=[],
            entry_point=None,
        )
        result = service.extract_provider_grounding(raw)
        assert len(result.grounding_chunks) == 2
        assert result.sources_count == 2
        assert result.grounding_chunks[0].uri == "https://reuters.com/news"
        assert result.grounding_chunks[0].title == "reuters.com"
        assert result.grounding_chunks[1].uri == "https://bbc.com/article"

    def test_non_web_chunk_produces_empty_placeholder(self, service):
        """Chunks without web data get empty GroundingChunk to preserve index alignment."""
        raw = _make_raw(
            queries=[],
            chunks=[
                SdkGroundingChunk(web=None),
                _make_sdk_chunk("https://example.com", "example.com"),
            ],
            supports=[],
            entry_point=None,
        )
        result = service.extract_provider_grounding(raw)
        assert len(result.grounding_chunks) == 2
        assert result.grounding_chunks[0].uri == ""
        assert result.grounding_chunks[0].title == ""
        assert result.grounding_chunks[1].uri == "https://example.com"
        assert result.sources_count == 1

    def test_extracts_grounding_supports(self, service):
        raw = _make_raw(
            queries=[],
            chunks=[],
            supports=[
                _make_sdk_support(0, 24, "AI is advancing rapidly.", [0]),
                _make_sdk_support(25, 60, "New models released this year.", [0, 1]),
            ],
            entry_point=None,
        )
        result = service.extract_provider_grounding(raw)
        assert result.citations_count == 2
        assert len(result.grounding_supports) == 2
        assert result.grounding_supports[0].segment.text == "AI is advancing rapidly."
        assert result.grounding_supports[0].grounding_chunk_indices == [0]
        assert result.grounding_supports[0].text == "AI is advancing rapidly."
        assert result.grounding_supports[1].grounding_chunk_indices == [0, 1]

    def test_support_segment_fields_mapped_correctly(self, service):
        raw = _make_raw(
            queries=[],
            chunks=[],
            supports=[_make_sdk_support(10, 50, "grounded text", [2])],
            entry_point=None,
        )
        result = service.extract_provider_grounding(raw)
        seg = result.grounding_supports[0].segment
        assert seg.start_index == 10
        assert seg.end_index == 50
        assert seg.text == "grounded text"

    def test_extracts_search_entry_point(self, service):
        raw = _make_raw(
            queries=[],
            chunks=[],
            supports=[],
            entry_point=SdkSearchEntryPoint(rendered_content="<div>Search widget</div>")
        )
        result = service.extract_provider_grounding(raw)
        assert result.search_entry_point is not None
        assert result.search_entry_point.rendered_content == "<div>Search widget</div>"

    def test_no_entry_point_when_absent(self, service):
        raw = _make_raw(queries=[], chunks=[], supports=[], entry_point=None)
        result = service.extract_provider_grounding(raw)
        assert result.search_entry_point is None

    def test_empty_raw_data_still_returns_grounding_used_true(self, service):
        """An empty SdkGroundingRawData (no queries/chunks) succeeds without error."""
        raw = _make_raw(queries=[], chunks=[], supports=[], entry_point=None)
        result = service.extract_provider_grounding(raw)
        assert result.grounding_used is True
        assert result.grounding_chunks == []
        assert result.grounding_supports == []
        assert result.error is None

    def test_returns_error_on_exception(self, service):
        """If raw data raises during processing, returns grounding_used=False with error."""
        class BadRaw:
            @property
            def web_search_queries(self):
                raise RuntimeError("corrupted data")
            grounding_chunks = []
            grounding_supports = []
            search_entry_point = None

        result = service.extract_provider_grounding(BadRaw())
        assert result.grounding_used is False
        assert result.error is not None

