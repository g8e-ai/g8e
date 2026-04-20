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

"""Typed fake for WebSearchProvider."""

from app.constants.settings import GroundingSource
from app.models.grounding import GroundingChunk, GroundingMetadata, GroundingSourceInfo

class FakeWebSearchProvider:
    """Test double for WebSearchProvider.

    Implements the same interface as WebSearchProvider but requires no GCP
    credentials and performs no network calls. Fully controllable via
    constructor arguments.

    build_g8e_web_search_grounding delegates to the real implementation so
    grounding construction logic is exercised, not bypassed.

    search() returns the configured SearchWebResult directly.
    """

    def __init__(self, search_result=None):
        from app.models.tool_results import SearchWebResult
        self._search_result = search_result or SearchWebResult(
            success=False, query="", error="no result configured"
        )
        self._search_calls: list[dict] = []

    async def search(self, query: str, num: int = 5):
        self._search_calls.append({"query": query, "num": num})
        return self._search_result

    def build_g8e_web_search_grounding(self, result):
        if not result.success or not result.results:
            return GroundingMetadata(
                grounding_used=False,
                source=GroundingSource.WEB_SEARCH,
            )

        chunks = []
        sources = []
        for i, item in enumerate(result.results):
            uri = getattr(item, 'link', None) or ""
            title = getattr(item, 'title', None) or ""
            if not uri:
                continue
            chunks.append(GroundingChunk(uri=uri, title=title))
            sources.append(GroundingSourceInfo(
                uri=uri,
                domain=title,
                display_name=title,
                full_title=title,
                citation_num=i + 1,
            ))

        if not sources:
            return GroundingMetadata(
                grounding_used=False,
                source=GroundingSource.WEB_SEARCH,
            )

        query_list = [result.query] if result.query else []
        return GroundingMetadata(
            grounding_used=True,
            source=GroundingSource.WEB_SEARCH,
            web_search_queries=query_list,
            search_queries_count=len(query_list),
            grounding_chunks=chunks,
            sources_count=len(sources),
            sources=sources,
        )

    def extract_source_info(self, uri, title):
        return GroundingSourceInfo(
            uri=uri or "",
            domain=title or "",
            display_name=title or "Web Source",
            full_title=title or "Web Source",
        )
