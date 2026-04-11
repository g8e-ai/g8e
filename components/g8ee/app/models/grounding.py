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


from pydantic import Field

from app.constants.settings import GroundingSource

from .base import G8eBaseModel


class GroundingChunk(G8eBaseModel):
    """A single grounding chunk referencing a web source."""
    uri: str = Field(default="", description="Source URI")
    title: str = Field(default="", description="Source title or domain")


class GroundingSegment(G8eBaseModel):
    """A text segment within the response that is grounded by a source."""
    start_index: int = Field(default=0, description="Start character index in the response text")
    end_index: int = Field(default=0, description="End character index in the response text")
    text: str = Field(default="", description="The grounded text segment")


class GroundingSupport(G8eBaseModel):
    """Maps a grounded text segment to one or more grounding chunk indices."""
    segment: GroundingSegment = Field(default_factory=GroundingSegment)
    grounding_chunk_indices: list[int] = Field(default_factory=list, description="Indices into grounding_chunks list")
    text: str = Field(default="", description="Convenience copy of segment.text")


class SearchEntryPoint(G8eBaseModel):
    """Provider-native search entry point widget rendered content (e.g. Gemini Search widget)."""
    rendered_content: str = Field(default="", description="HTML rendered search widget content")


class GroundingSourceInfo(G8eBaseModel):
    """Resolved display information for a single grounding source, with citation assignment."""
    uri: str = Field(default="", description="Source URI")
    domain: str = Field(default="", description="Cleaned domain name")
    display_name: str = Field(default="", description="Human-readable display name")
    full_title: str = Field(default="", description="Full display title")
    favicon_url: str | None = Field(default=None, description="Favicon URL for this domain")
    citation_num: int = Field(default=0, description="1-based citation number assigned during processing")
    segments: list[str] = Field(default_factory=list, description="Text segments supported by this source")


class GroundingMetadata(G8eBaseModel):
    """Typed grounding metadata for a single AI response turn.

    Produced by GroundingService.extract_provider_grounding() from SdkGroundingRawData
    (provider-native grounding) or constructed directly for web_search and attachment
    grounding sources.

    sources is populated in place by WebSearchProvider.add_inline_citations().
    Carried on the wire in StreamChunkData.grounding_metadata.
    """
    grounding_used: bool = Field(default=False, description="Whether any grounding context was applied")
    source: GroundingSource = Field(default=GroundingSource.PROVIDER_NATIVE, description="Origin of this grounding context")
    web_search_queries: list[str] = Field(default_factory=list, description="Search queries executed")
    search_queries_count: int = Field(default=0, description="Number of search queries executed")
    grounding_chunks: list[GroundingChunk] = Field(default_factory=list, description="Web sources referenced")
    sources_count: int = Field(default=0, description="Number of valid web sources")
    grounding_supports: list[GroundingSupport] = Field(default_factory=list, description="Text-to-source mappings")
    citations_count: int = Field(default=0, description="Number of citation supports found")
    search_entry_point: SearchEntryPoint | None = Field(default=None, description="Provider-native search entry point widget")
    sources: list[GroundingSourceInfo] = Field(default_factory=list, description="Resolved sources with citation numbers, populated by WebSearchProvider.add_inline_citations()")
    error: str | None = Field(default=None, description="Error message if extraction failed")
