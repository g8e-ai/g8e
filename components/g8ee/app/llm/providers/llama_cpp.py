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

"""llama.cpp LLM provider implementation.

llama.cpp server provides an OpenAI-compatible HTTP API, so this implementation
reuses the OpenAI provider with a custom endpoint configuration.
"""

import logging
from collections.abc import AsyncGenerator

import httpx

from app.constants import (
    LLM_DEFAULT_MAX_OUTPUT_TOKENS,
    ThinkingLevel,
)
from app.llm.llm_types import (
    AssistantLLMSettings,
    Candidate,
    Content,
    LiteLLMSettings,
    PrimaryLLMSettings,
    ToolCall,
    GenerateContentResponse,
    Part,
    StreamChunkFromModel,
    UsageMetadata,
    ToolGroup,
)
from app.llm.providers.open_ai import OpenAIProvider
from ..provider import LLMProvider
from ..utils import schema_to_dict
from ._capability import translate_capability_error

logger = logging.getLogger(__name__)


class LlamaCppProvider(LLMProvider):
    """llama.cpp provider using OpenAI-compatible API.

    llama.cpp server provides an OpenAI-compatible HTTP API, so we extend
    OpenAIProvider with llama.cpp-specific endpoint handling.
    """

    def __init__(self, endpoint: str, api_key: str):
        super().__init__()
        self._endpoint = endpoint
        self._api_key = api_key
        self._openai_provider = OpenAIProvider(endpoint=endpoint, api_key=api_key)
        logger.info("llama.cpp provider initialized: %s", endpoint)

    async def _close_resources(self):
        """Clean up provider resources."""
        await self._openai_provider._close_resources()

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from llama.cpp (primary LLM)."""
        async for chunk in self._openai_provider.generate_content_stream_primary(
            model=model,
            contents=contents,
            primary_llm_settings=primary_llm_settings,
        ):
            yield chunk

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from llama.cpp (primary LLM)."""
        return await self._openai_provider.generate_content_primary(
            model=model,
            contents=contents,
            primary_llm_settings=primary_llm_settings,
        )

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from llama.cpp (assistant LLM)."""
        async for chunk in self._openai_provider.generate_content_stream_assistant(
            model=model,
            contents=contents,
            assistant_llm_settings=assistant_llm_settings,
        ):
            yield chunk

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from llama.cpp (assistant LLM)."""
        return await self._openai_provider.generate_content_assistant(
            model=model,
            contents=contents,
            assistant_llm_settings=assistant_llm_settings,
        )

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        """Stream a response from llama.cpp (lite LLM)."""
        async for chunk in self._openai_provider.generate_content_stream_lite(
            model=model,
            contents=contents,
            lite_llm_settings=lite_llm_settings,
        ):
            yield chunk

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        """Generate a complete response from llama.cpp (lite LLM)."""
        return await self._openai_provider.generate_content_lite(
            model=model,
            contents=contents,
            lite_llm_settings=lite_llm_settings,
        )
