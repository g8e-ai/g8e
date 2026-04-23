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
WebSearchProvider — web search execution and citation processing.

Executes web search queries and returns typed results. Also owns all
web-search-specific citation logic: source info extraction, inline citation
insertion, and citation number normalisation.

The underlying search backend is Vertex AI Search (Discovery Engine), querying
a pre-configured website data store. Auth uses an explicit API key passed at
construction; ADC is never used.

This is distinct from Gemini's native Search grounding (SdkGroundingRawData),
which is processed by GroundingService.extract_provider_grounding().
"""

import asyncio
import logging
import re
from typing import Any, Protocol, runtime_checkable

from google.api_core.client_options import ClientOptions
from google.api_core.exceptions import GoogleAPICallError, ResourceExhausted, ServiceUnavailable
from google.auth.api_key import Credentials as ApiKeyCredentials
from google.cloud import discoveryengine_v1 as discoveryengine

from app.constants import (
    WEB_SEARCH_CLIENT_MAX_RETRIES,
    WEB_SEARCH_CLIENT_RETRY_BACKOFF,
    WEB_SEARCH_CLIENT_TIMEOUT,
)
from app.errors import NetworkError
from app.models.grounding import GroundingMetadata, GroundingSourceInfo
from app.models.tool_results import SearchWebResult, WebSearchResultItem

logger = logging.getLogger(__name__)

_CITATION_STRIP_PATTERN = re.compile(r"\[\d+(?:,\s*\d+)*\]")
_CITATION_LINK_PATTERN = re.compile(r'<a href="([^"]+)"([^>]*?)>(\d+)</a>')


@runtime_checkable
class SearchClientProtocol(Protocol):
    """Protocol for Google Discovery Engine SearchServiceClient."""

    def search_lite(
        self,
        request: discoveryengine.SearchRequest,
        *,
        retry: Any = None,
        timeout: float | None = None,
        metadata: Any = None,
    ) -> Any:
        """Execute a search_lite request."""
        ...


class WebSearchProvider:
    """Executes web search queries and owns web-search citation processing.

    Constructed with GCP config at startup and injected where needed.
    The search_web LLM tool delegates here; no LLM-provider-specific logic is present.
    """

    def __init__(
        self,
        project_id: str | None,
        engine_id: str | None,
        api_key: str | None,
        location: str = "global",
    ) -> None:
        if not project_id or not engine_id or not api_key:
            logger.warning(
                "WebSearchProvider initialized with missing credentials "
                "(project=%s, engine=%s, api_key_set=%s). Search will fail.",
                project_id,
                engine_id,
                bool(api_key),
            )
            self._project_id = project_id or ""
            self._engine_id = engine_id or ""
            self._location = location
            self._client = None  # type: ignore
            return

        self._project_id = project_id
        self._engine_id = engine_id
        self._location = location
        client_options = (
            ClientOptions(api_endpoint=f"{location}-discoveryengine.googleapis.com")
            if location != "global"
            else None
        )
        self._client: SearchClientProtocol = discoveryengine.SearchServiceClient(
            credentials=ApiKeyCredentials(api_key),
            client_options=client_options,
        )
        logger.info("WebSearchProvider initialized (project=%s engine=%s)", project_id, engine_id)

    def extract_source_info(self, uri: str | None, title: str | None) -> GroundingSourceInfo:
        """Derive display metadata (domain, display name, favicon) from a grounding source."""
        try:
            domain = title.replace("www.", "") if title else ""
            if domain:
                display_name = domain.split(".")[0].replace("-", " ").replace("_", " ").title()
            else:
                display_name = "Web Source"
            favicon_url = (
                f"https://www.google.com/s2/favicons?domain={domain}&sz=32"
                if domain
                else None
            )
            logger.info("SOURCE INFO: domain=%s, display_name=%s", domain, display_name)
            return GroundingSourceInfo(
                uri=uri or "",
                domain=domain,
                display_name=display_name,
                full_title=display_name,
                favicon_url=favicon_url,
            )
        except Exception as e:
            logger.error(
                "Failed to extract source info from URI=%s, title=%s: %s", uri, title, e
            )
            return GroundingSourceInfo(
                uri=uri or "",
                domain=title if title else "",
                display_name=title[:30] if title else "Web Source",
                full_title=title if title else "Web Source",
            )

    def add_inline_citations(
        self, text: str, grounding_metadata: GroundingMetadata
    ) -> str:
        """Strip LLM auto-citations, then reinsert properly numbered citations.

        Mutates grounding_metadata.sources in place with the resolved source list.
        Returns the modified text. Returns original text on any error.
        """
        try:
            if not grounding_metadata.grounding_used:
                return text

            supports = grounding_metadata.grounding_supports
            chunks = grounding_metadata.grounding_chunks

            if not supports or not chunks:
                logger.info("No grounding supports or chunks to process for citations")
                return text

            original_length = len(text)
            text_clean = _CITATION_STRIP_PATTERN.sub("", text)
            stripped_count = original_length - len(text_clean)
            if stripped_count > 0:
                logger.info(
                    "Stripped %d characters of auto-citations from response", stripped_count
                )

            chunk_to_citation_num: dict[int, int] = {}
            chunk_to_source_info: dict[int, GroundingSourceInfo] = {}
            chunk_to_segments: dict[int, list[str]] = {}
            next_citation_num = 1

            for support in supports:
                chunk_indices = support.grounding_chunk_indices
                if not chunk_indices:
                    continue

                segment_text = support.text

                for chunk_idx in chunk_indices:
                    if chunk_idx < len(chunks):
                        if chunk_idx not in chunk_to_segments:
                            chunk_to_segments[chunk_idx] = []
                        if segment_text:
                            chunk_to_segments[chunk_idx].append(segment_text)
                        if chunk_idx not in chunk_to_citation_num:
                            uri = chunks[chunk_idx].uri
                            title = chunks[chunk_idx].title
                            if uri:
                                source_info = self.extract_source_info(uri, title)
                                source_info = GroundingSourceInfo(
                                    uri=source_info.uri,
                                    domain=source_info.domain,
                                    display_name=source_info.display_name,
                                    full_title=source_info.full_title,
                                    favicon_url=source_info.favicon_url,
                                    citation_num=next_citation_num,
                                )
                                chunk_to_citation_num[chunk_idx] = next_citation_num
                                chunk_to_source_info[chunk_idx] = source_info
                                next_citation_num += 1

            for chunk_idx, source_info in chunk_to_source_info.items():
                chunk_to_source_info[chunk_idx] = GroundingSourceInfo(
                    uri=source_info.uri,
                    domain=source_info.domain,
                    display_name=source_info.display_name,
                    full_title=source_info.full_title,
                    favicon_url=source_info.favicon_url,
                    citation_num=source_info.citation_num,
                    segments=chunk_to_segments.get(chunk_idx, []),
                )

            sources_list = [chunk_to_source_info[idx] for idx in sorted(chunk_to_source_info.keys())]
            grounding_metadata.sources = sources_list

            if not sources_list:
                logger.info("No valid sources found in grounding metadata")
                return text_clean

            logger.info("Built %d citation sources for response", len(sources_list))

            supports_with_segments = [
                s for s in supports
                if s.segment.end_index > 0 and s.grounding_chunk_indices
            ]

            if not supports_with_segments:
                logger.info("No supports with valid segment end_index found")
                return text_clean

            sorted_supports = sorted(
                supports_with_segments,
                key=lambda s: s.segment.end_index,
                reverse=True,
            )

            citations_inserted = 0
            for support in sorted_supports:
                end_index = support.segment.end_index
                chunk_indices = support.grounding_chunk_indices
                if end_index > len(text_clean):
                    logger.warning(
                        "Segment end_index %d exceeds text length %d, skipping citation",
                        end_index, len(text_clean),
                    )
                    continue

                if not chunk_indices:
                    continue

                citation_nums = [
                    str(chunk_to_citation_num[idx])
                    for idx in chunk_indices
                    if idx in chunk_to_citation_num
                ]

                if citation_nums:
                    citation_string = "[" + ",".join(citation_nums) + "]"
                    text_clean = text_clean[:end_index] + citation_string + text_clean[end_index:]
                    citations_inserted += 1

            logger.info("Inserted %d properly-placed citations into response", citations_inserted)
            return text_clean

        except Exception as e:
            logger.error("Failed to add inline citations: %s", e, exc_info=True)
            return text

    def normalize_citation_numbers(self, markdown_text: str | None) -> str | None:
        """Ensure citation numbers in HTML anchor tags are sequential by first URI appearance."""
        if not markdown_text:
            return markdown_text

        uri_to_number: dict[str, int] = {}
        next_number = 1

        def replace(match: re.Match[str]) -> str:
            nonlocal next_number
            uri = match.group(1).strip() if match.group(1) else ""
            other_attrs = match.group(2)

            if not uri:
                return match.group(0)

            if uri not in uri_to_number:
                uri_to_number[uri] = next_number
                next_number += 1

            return f'<a href="{uri}"{other_attrs}>{uri_to_number[uri]}</a>'

        return _CITATION_LINK_PATTERN.sub(replace, markdown_text)

    def build_g8e_web_search_grounding(self, result: SearchWebResult) -> GroundingMetadata:
        """Build GroundingMetadata directly from a SearchWebResult.

        Used for the search_web function tool path, which is independent of any
        provider-native grounding. Populates grounding_chunks and sources from
        the search result items. grounding_supports is intentionally left empty
        because the search_web tool does not produce segment-level text mappings.
        """
        from app.constants.settings import GroundingSource
        from app.models.grounding import GroundingChunk

        if not result.success or not result.results:
            return GroundingMetadata(
                grounding_used=False,
                source=GroundingSource.WEB_SEARCH,
            )

        chunks: list[GroundingChunk] = []
        sources: list[GroundingSourceInfo] = []

        for i, item in enumerate(result.results):
            uri = item.link or ""
            title = item.title or ""
            if not uri:
                continue
            chunks.append(GroundingChunk(uri=uri, title=title))
            source_info = self.extract_source_info(uri, title)
            sources.append(GroundingSourceInfo(
                uri=source_info.uri,
                domain=source_info.domain,
                display_name=source_info.display_name,
                full_title=title or source_info.full_title,
                favicon_url=source_info.favicon_url,
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

    async def search(self, query: str, num: int = 5) -> SearchWebResult:
        """Execute a web search query and return typed results.

        Retries up to WEB_SEARCH_CLIENT_MAX_RETRIES times on transient
        GoogleAPICallError failures with exponential backoff. Raises NetworkError
        on non-retryable failures so callers receive structured error context.

        Args:
            query: Search query string.
            num:   Number of results to return (1–25, capped at 25).

        Returns:
            SearchWebResult with success=True and populated results on success,
            or success=False with error set on non-retryable failure.
        """
        num = min(num, 25)
        logger.info("[WEB_SEARCH] Query: %s num=%d", query, num)

        serving_config = (
            f"projects/{self._project_id}/locations/{self._location}"
            f"/collections/default_collection/engines/{self._engine_id}"
            f"/servingConfigs/default_search"
        )
        request = discoveryengine.SearchRequest(
            serving_config=serving_config,
            query=query,
            page_size=num,
            query_expansion_spec=discoveryengine.SearchRequest.QueryExpansionSpec(
                condition=discoveryengine.SearchRequest.QueryExpansionSpec.Condition.AUTO,
            ),
            spell_correction_spec=discoveryengine.SearchRequest.SpellCorrectionSpec(
                mode=discoveryengine.SearchRequest.SpellCorrectionSpec.Mode.AUTO,
            ),
        )

        attempt = 0
        last_error: Exception = Exception("Unknown error")
        while attempt <= WEB_SEARCH_CLIENT_MAX_RETRIES:
            try:
                pager = await self._execute_search_lite(request)
                results: list[WebSearchResultItem] = []
                for result in pager:
                    derived = result.document.derived_struct_data
                    snippets = derived.get("snippets", [])
                    snippet_text = snippets[0].get("snippet", "") if snippets else ""
                    results.append(
                        WebSearchResultItem(
                            title=derived.get("title", ""),
                            link=derived.get("link", ""),
                            snippet=snippet_text,
                        )
                    )
                    if len(results) >= num:
                        break
                total_size = pager.total_size
                logger.info("[WEB_SEARCH] Returned %d results (total_size=%s)", len(results), total_size)
                return SearchWebResult(
                    success=True,
                    query=query,
                    results=results,
                    total_results=str(total_size),
                )
            except asyncio.TimeoutError as e:
                last_error = e
                logger.warning(
                    "[WEB_SEARCH] Timeout on attempt %d/%d",
                    attempt + 1, WEB_SEARCH_CLIENT_MAX_RETRIES + 1,
                )
            except GoogleAPICallError as e:
                last_error = e
                if not isinstance(e, (ServiceUnavailable, ResourceExhausted)):
                    logger.error("[WEB_SEARCH] Non-retryable API error: %s", e)
                    raise NetworkError(
                        message=f"Web search API call failed: {e}",
                        details={"query": query, "attempt": attempt + 1},
                        cause=e,
                    )
                logger.warning(
                    "[WEB_SEARCH] Retryable API error on attempt %d/%d: %s",
                    attempt + 1, WEB_SEARCH_CLIENT_MAX_RETRIES + 1, e,
                )
            except Exception as e:
                logger.error("[WEB_SEARCH] Unexpected error: %s", e, exc_info=True)
                return SearchWebResult(success=False, query=query, error=f"Web search failed: {e}. Retry or use alternative information source.")

            attempt += 1
            if attempt <= WEB_SEARCH_CLIENT_MAX_RETRIES:
                backoff = WEB_SEARCH_CLIENT_RETRY_BACKOFF * (2 ** (attempt - 1))
                await asyncio.sleep(backoff)

        logger.error(
            "[WEB_SEARCH] All %d attempts exhausted. Last error: %s",
            WEB_SEARCH_CLIENT_MAX_RETRIES + 1, last_error,
        )
        return SearchWebResult(success=False, query=query, error=str(last_error))

    async def _execute_search_lite(self, request: discoveryengine.SearchRequest) -> Any:
        """Helper to run the synchronous search_lite call in a thread with a timeout."""
        if not self._client:
            raise NetworkError(
                message="WebSearchProvider client not initialized due to missing credentials",
                details={"project_id": self._project_id, "engine_id": self._engine_id},
            )

        return await asyncio.wait_for(
            asyncio.to_thread(self._client.search_lite, request),
            timeout=WEB_SEARCH_CLIENT_TIMEOUT,
        )
