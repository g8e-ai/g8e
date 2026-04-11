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
VSE LLM Provider Abstraction

Model-agnostic LLM interface for provider-independent AI operations.
"""

from .llm_types import (
    Candidate,
    Content,
    GenerateContentConfig,
    GenerateContentResponse,
    InlineData,
    Part,
    Role,
    Schema,
    StreamChunkFromModel,
    ThinkingConfig,
    ToolCall,
    ToolDeclaration,
    ToolGroup,
    ToolResponse,
    Type,
    UsageMetadata,
)

from .provider import LLMProvider
from .factory import get_llm_provider

__all__ = [
    "Candidate",
    "Content",
    "GenerateContentConfig",
    "GenerateContentResponse",
    "get_llm_provider",
    "InlineData",
    "LLMProvider",
    "Part",
    "Role",
    "Schema",
    "StreamChunkFromModel",
    "ThinkingConfig",
    "ToolCall",
    "ToolDeclaration",
    "ToolGroup",
    "ToolResponse",
    "Type",
    "UsageMetadata",
]
