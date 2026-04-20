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
GroundingService — top-level grounding coordinator.

Converts provider-native SDK grounding data (SdkGroundingRawData) into canonical
GroundingMetadata. Acts as the pipeline entry point for all grounding sources:

  PROVIDER_NATIVE — SDK-level grounding (e.g. Gemini Search grounding)
  WEB_SEARCH      — explicit search_web tool call (see WebSearchProvider)
  ATTACHMENT      — user-uploaded file (see AttachmentGroundingProvider)

Web-search-specific logic (source info extraction, inline citation insertion,
citation number normalisation) lives in WebSearchProvider.
"""

import logging

from app.constants.settings import GroundingSource
from app.llm.llm_types import SdkGroundingRawData
from app.models.grounding import (
    GroundingChunk,
    GroundingMetadata,
    GroundingSegment,
    GroundingSupport,
    SearchEntryPoint,
)

logger = logging.getLogger(__name__)


class GroundingService:
    """Converts raw SDK provider grounding data into canonical GroundingMetadata.

    Stateless — all methods are pure functions operating on their arguments.
    Instantiated once at startup and injected where needed.
    """

    def extract_provider_grounding(self, grounding_raw: SdkGroundingRawData) -> GroundingMetadata:
        """Convert SdkGroundingRawData into canonical GroundingMetadata.

        Returns GroundingMetadata(grounding_used=True, source=PROVIDER_NATIVE) on success,
        GroundingMetadata(grounding_used=False, error=...) on failure.
        """
        try:
            web_search_queries = list(grounding_raw.web_search_queries)
            if web_search_queries:
                logger.info(
                    "GROUNDING: %d search queries executed: %s",
                    len(web_search_queries), web_search_queries,
                )
            else:
                logger.warning("GROUNDING: No web search queries found in grounding metadata")

            chunks: list[GroundingChunk] = []
            valid_sources = 0
            for idx, sdk_chunk in enumerate(grounding_raw.grounding_chunks):
                if sdk_chunk.web is not None:
                    uri = sdk_chunk.web.uri
                    title = sdk_chunk.web.title
                    chunks.append(GroundingChunk(uri=uri, title=title))
                    logger.info(
                        "GROUNDING CHUNK %d: URI=%s, TITLE=%s",
                        idx, uri[:60], title[:60] if title else "(empty)",
                    )
                    valid_sources += 1
                else:
                    chunks.append(GroundingChunk(uri="", title=""))
                    logger.warning("GROUNDING CHUNK %d: No web source data found", idx)
            logger.info(
                "GROUNDING: %d sources found from %d total chunks",
                valid_sources, len(chunks),
            )

            supports: list[GroundingSupport] = []
            for sdk_support in grounding_raw.grounding_supports:
                segment = GroundingSegment(
                    start_index=sdk_support.segment.start_index,
                    end_index=sdk_support.segment.end_index,
                    text=sdk_support.segment.text,
                )
                supports.append(GroundingSupport(
                    segment=segment,
                    grounding_chunk_indices=list(sdk_support.grounding_chunk_indices),
                    text=segment.text,
                ))
            logger.info("GROUNDING: %d citation supports found", len(supports))

            search_entry_point: SearchEntryPoint | None = None
            if grounding_raw.search_entry_point is not None:
                search_entry_point = SearchEntryPoint(
                    rendered_content=grounding_raw.search_entry_point.rendered_content,
                )

            return GroundingMetadata(
                grounding_used=True,
                source=GroundingSource.PROVIDER_NATIVE,
                web_search_queries=web_search_queries,
                search_queries_count=len(web_search_queries),
                grounding_chunks=chunks,
                sources_count=valid_sources,
                grounding_supports=supports,
                citations_count=len(supports),
                search_entry_point=search_entry_point,
            )

        except Exception as e:
            logger.error("GROUNDING: Failed to extract grounding metadata: %s", e, exc_info=True)
            return GroundingMetadata(grounding_used=False, error=f"Grounding metadata extraction failed: {e}. Proceed without grounding.")

