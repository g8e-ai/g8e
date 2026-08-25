# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
Grounding Services

Unified pipeline for all grounding context fed to the AI.

Grounding is any external context injected to anchor AI responses to reality:
  - Attachments: user-uploaded files (PDF, image, text) sent as LLM Parts
  - Web search: explicit search_web tool call results (provider-agnostic)
  - Provider-native: SDK-level grounding metadata (e.g. Gemini Search grounding)

Service hierarchy:
  GroundingService            - converts provider-native SDK grounding data
                                (SdkGroundingRawData) into canonical GroundingMetadata
  AttachmentGroundingProvider - formats attachment files as typed LLM Parts
  WebSearchProvider           - executes web search queries via Google Custom Search
                                and owns citation processing (source info extraction,
                                inline citation insertion, citation number normalisation)
"""

from .attachment_provider import AttachmentGroundingProvider
from .grounding_service import GroundingService
from .web_search_provider import WebSearchProvider

__all__ = [
    "AttachmentGroundingProvider",
    "GroundingService",
    "WebSearchProvider",
]
