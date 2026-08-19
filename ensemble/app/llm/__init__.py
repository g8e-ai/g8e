# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""
g8ee LLM Provider Abstraction

Model-agnostic LLM interface for provider-independent AI operations.

LLM modules:
- factory.py: Provider factory for creating LLM instances
- llm_dataclasses.py: LLM-specific dataclass definitions
- llm_schema.py: Schema definitions for structured outputs
- llm_types.py: Type definitions for LLM operations
- prompts.py: Prompt templates and generation logic
- provider.py: Base LLM provider interface
- providers/: Provider-specific implementations (Anthropic, OpenAI, Gemini, Ollama, etc.)
- structured.py: Structured output generation
- thinking.py: Thinking/reasoning mode support
- utils.py: LLM utility functions
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
from .factory import get_llm_provider, clear_provider_cache

__all__ = [
    "Candidate",
    "Content",
    "GenerateContentConfig",
    "GenerateContentResponse",
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
    "clear_provider_cache",
    "get_llm_provider",
]
