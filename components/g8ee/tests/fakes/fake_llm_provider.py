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

from collections.abc import AsyncGenerator
from typing import Any

from app.llm.llm_types import (
    AssistantLLMSettings,
    Candidate,
    Content,
    GenerateContentConfig,
    GenerateContentResponse,
    LiteLLMSettings,
    Part,
    PrimaryLLMSettings,
    StreamChunkFromModel,
    ToolCall,
    ToolGroup,
)
from app.llm.provider import LLMProvider

class FakeLLMProvider(LLMProvider):
    """A fake LLM provider for testing that avoids AsyncMock/MagicMock.
    
    Adheres to testing.md principles: 'Never mock LLM clients'.
    This provides a deterministic fake implementation instead of a mock.
    """

    def __init__(self):
        self.responses: list[GenerateContentResponse] = []
        self.stream_chunks: list[list[StreamChunkFromModel]] = []
        self.call_log: list[dict[str, Any]] = []

    def add_response(self, text: str, finish_reason: str = "STOP"):
        """Queue a standard text response."""
        response = GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(role="model", parts=[Part(text=text)]),
                    finish_reason=finish_reason
                )
            ]
        )
        self.responses.append(response)

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        self.call_log.append({
            "method": "generate_content_stream_primary",
            "model": model,
            "contents": contents,
            "primary_llm_settings": primary_llm_settings,
        })
        if not self.stream_chunks:
            return
        chunks = self.stream_chunks.pop(0)
        for chunk in chunks:
            yield chunk

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        self.call_log.append({
            "method": "generate_content_primary",
            "model": model,
            "contents": contents,
            "primary_llm_settings": primary_llm_settings,
        })
        if not self.responses:
            return GenerateContentResponse(candidates=[])
        return self.responses.pop(0)

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        self.call_log.append({
            "method": "generate_content_stream_assistant",
            "model": model,
            "contents": contents,
            "assistant_llm_settings": assistant_llm_settings,
        })
        if not self.stream_chunks:
            return
        chunks = self.stream_chunks.pop(0)
        for chunk in chunks:
            yield chunk

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        self.call_log.append({
            "method": "generate_content_assistant",
            "model": model,
            "contents": contents,
            "assistant_llm_settings": assistant_llm_settings,
        })
        if not self.responses:
            return GenerateContentResponse(candidates=[])
        return self.responses.pop(0)

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        self.call_log.append({
            "method": "generate_content_stream_lite",
            "model": model,
            "contents": contents,
            "lite_llm_settings": lite_llm_settings,
        })
        if not self.stream_chunks:
            return
        chunks = self.stream_chunks.pop(0)
        for chunk in chunks:
            yield chunk

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        self.call_log.append({
            "method": "generate_content_lite",
            "model": model,
            "contents": contents,
            "lite_llm_settings": lite_llm_settings,
        })
        if not self.responses:
            return GenerateContentResponse(candidates=[])
        return self.responses.pop(0)

    async def generate_content(
        self,
        model: str,
        contents: list[Content],
        config: GenerateContentConfig,
        tools: list[ToolGroup] = None,
        system_instruction: str = None,
    ) -> GenerateContentResponse:
        self.call_log.append({
            "method": "generate_content",
            "model": model,
            "contents": contents,
            "config": config,
            "tools": tools,
            "system_instruction": system_instruction,
        })
        if not self.responses:
            # Default fallback if no response queued, though tests should queue them
            return GenerateContentResponse(candidates=[])
        return self.responses.pop(0)

    async def generate_content_stream(
        self,
        model: str,
        contents: list[Content],
        config: GenerateContentConfig,
        tools: list[ToolGroup] = None,
        system_instruction: str = None,
    ) -> AsyncGenerator[StreamChunkFromModel, None]:
        self.call_log.append({
            "method": "generate_content_stream",
            "model": model,
            "contents": contents,
            "config": config,
            "tools": tools,
            "system_instruction": system_instruction,
        })
        if not self.stream_chunks:
            return
        
        chunks = self.stream_chunks.pop(0)
        for chunk in chunks:
            yield chunk
