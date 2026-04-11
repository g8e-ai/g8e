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
Grounding Services

Unified pipeline for all grounding context fed to the AI.

Grounding is any external context injected to anchor AI responses to reality:
  - Attachments: user-uploaded files (PDF, image, text) sent as LLM Parts
  - Web search: explicit search_web tool call results (provider-agnostic)
  - Provider-native: SDK-level grounding metadata (e.g. Gemini Search grounding)

Service hierarchy:
  GroundingService            — converts provider-native SDK grounding data
                                (SdkGroundingRawData) into canonical GroundingMetadata
  AttachmentGroundingProvider — formats attachment files as typed LLM Parts
  WebSearchProvider           — executes web search queries via Google Custom Search
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
