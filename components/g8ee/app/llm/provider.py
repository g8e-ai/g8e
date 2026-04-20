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
Abstract LLM Provider Interface

All provider implementations must implement this interface.
"""

from abc import ABC, abstractmethod
from collections.abc import AsyncGenerator

from app.llm.llm_types import (
    AssistantLLMSettings,
    Content,
    GenerateContentResponse,
    LiteLLMSettings,
    PrimaryLLMSettings,
    StreamChunkFromModel,
)


class LLMProvider(ABC):
    """Abstract base class for LLM providers."""

    def __init__(self):
        self._is_cached_singleton = False

    @abstractmethod
    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from the primary LLM (agent main loop)."""
        yield

    @abstractmethod
    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from the primary LLM."""
        ...

    @abstractmethod
    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from the assistant LLM (analysis, memory, title)."""
        yield

    @abstractmethod
    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from the assistant LLM."""
        ...

    @abstractmethod
    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from the lite LLM (triage, eval)."""
        yield

    @abstractmethod
    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from the lite LLM."""
        ...

    async def close(self):
        """Clean up provider resources (e.g., close HTTP clients).

        For cached singleton providers, this is a no-op to allow reuse.
        Use force_close() to actually close a cached provider.
        """
        if not self._is_cached_singleton:
            await self._close_resources()

    async def _close_resources(self):
        """Internal method to actually close resources. Override in subclasses."""
        pass

    async def force_close(self):
        """Force close provider resources even if it's a cached singleton."""
        await self._close_resources()

    async def __aenter__(self):
        """Async context manager entry."""
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Async context manager exit - ensures cleanup for non-cached providers."""
        await self.close()
        return False
