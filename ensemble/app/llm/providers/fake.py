# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Fake LLM provider for CI and deterministic scenario testing.

This is a production-grade fake provider (not a test mock) that returns
deterministic tool-call responses without an external LLM. It is wired into
the LLM provider factory as ``LLMProvider.FAKE`` and selected via the
per-request override ``llm_primary_provider=fake`` or the env var
``G8E_LLM_PRIMARY_PROVIDER=fake``.

The provider inspects the last user message in the contents list and
pattern-matches the instruction to produce the appropriate tool call:

  - "create a new file" / "create a file" -> file_create_on_operator
  - "write" / "file_write"               -> file_write_on_operator
  - "delete" / "document delete"         -> (no tool call; text response)
  - "investigation note" / "document"    -> (no tool call; text response)

The file path and content are extracted from the message when present
(falls back to /tmp/g8e-fake-provider.txt). The target_operators field
defaults to ["all"].

This provider never makes a network call. It is safe for CI, airgapped
builds, and deterministic e2e tests.
"""

from __future__ import annotations

import logging
import re
from collections.abc import AsyncGenerator
from typing import Any

from app.constants.config import LLM_DEFAULT_MAX_OUTPUT_TOKENS
from app.llm.llm_dataclasses import (
    Candidate,
    Content,
    GenerateContentResponse,
    Part,
    StreamChunkFromModel,
    ToolCall,
    UsageMetadata,
)
from app.llm.llm_types import (
    AssistantLLMSettings,
    LiteLLMSettings,
    PrimaryLLMSettings,
    ToolGroup,
)
from app.llm.provider import LLMProvider

logger = logging.getLogger(__name__)

# Default file path and content for the fake provider when the message does
# not specify them explicitly.
_FAKE_DEFAULT_FILE_PATH = "/tmp/g8e-fake-provider.txt"
_FAKE_DEFAULT_CONTENT = "g8e fake provider deterministic output"

# Tool name constants (matching OperatorToolName values from the protocol).
_TOOL_FILE_CREATE = "file_create_on_operator"
_TOOL_FILE_WRITE = "file_write_on_operator"

# Regex patterns for extracting file path and content from the user message.
_FILE_PATH_RE = re.compile(r"(?:file\s+at\s+|path\s*[:=]\s*|create.*?at\s+)([^\s,]+)", re.IGNORECASE)
_CONTENT_RE = re.compile(
    r"(?:content\s*[:=]\s*|with\s+the\s+content\s*:?\s*|following\s+content\s+to\s+the\s+file\s+at\s+\S+\s*:\s*)(.+?)(?:$|\n)",
    re.IGNORECASE | re.DOTALL,
)


class FakeProvider(LLMProvider):
    """Deterministic LLM provider for CI and scenario testing.

    Returns tool-call responses based on pattern-matching the user message.
    No network calls, no external dependencies. The provider inspects the
    last user message in the contents list and produces the appropriate
    tool call (file_create, file_write) or a text response.
    """

    def __init__(self, endpoint: str | None = None, api_key: str | None = None):
        super().__init__()
        self._endpoint = endpoint
        self._api_key = api_key
        self.call_log: list[dict[str, Any]] = []

    @staticmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        """Fake provider validation - always valid (no external dependencies)."""
        return []

    def _extract_last_user_message(self, contents: list[Content]) -> str:
        """Extract the text of the last user message from the contents list."""
        for content in reversed(contents):
            if content.role == "user":
                parts = [p.text for p in content.parts if p.text]
                if parts:
                    return " ".join(parts)
        return ""

    def _extract_file_path(self, message: str) -> str:
        """Extract a file path from the user message, falling back to default."""
        match = _FILE_PATH_RE.search(message)
        if match:
            return match.group(1).strip().rstrip(".,:;")
        return _FAKE_DEFAULT_FILE_PATH

    def _extract_content(self, message: str) -> str:
        """Extract file content from the user message, falling back to default."""
        match = _CONTENT_RE.search(message)
        if match:
            return match.group(1).strip().rstrip(".,:;")
        return _FAKE_DEFAULT_CONTENT

    def _build_tool_call_response(self, message: str) -> GenerateContentResponse:
        """Build a GenerateContentResponse with the appropriate tool call."""
        msg_lower = message.lower()
        file_path = self._extract_file_path(message)
        content = self._extract_content(message)

        if "create" in msg_lower and "file" in msg_lower:
            tool_name = _TOOL_FILE_CREATE
        elif "write" in msg_lower and "file" in msg_lower:
            tool_name = _TOOL_FILE_WRITE
        else:
            # No tool call — return a text response.
            return GenerateContentResponse(
                candidates=[
                    Candidate(
                        content=Content(
                            role="model",
                            parts=[Part(text="No tool call needed for this request.")],
                        ),
                        finish_reason="STOP",
                    )
                ]
            )

        tool_call = ToolCall(
            name=tool_name,
            args={
                "file_path": file_path,
                "content": content,
                "justification": "Fake provider deterministic tool call for CI scenario",
                "target_operators": ["all"],
            },
            id="fake-tool-call-1",
        )

        return GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(
                        role="model",
                        parts=[Part(tool_call=tool_call)],
                    ),
                    finish_reason="STOP",
                )
            ],
            usage_metadata=UsageMetadata(
                prompt_token_count=100,
                candidates_token_count=50,
                total_token_count=150,
            ),
        )

    def _build_stream_chunks(self, message: str) -> list[StreamChunkFromModel]:
        """Build a list of StreamChunkFromModel for the streaming primary path."""
        msg_lower = message.lower()

        if "create" in msg_lower and "file" in msg_lower:
            tool_name = _TOOL_FILE_CREATE
        elif "write" in msg_lower and "file" in msg_lower:
            tool_name = _TOOL_FILE_WRITE
        else:
            return [
                StreamChunkFromModel(
                    text="No tool call needed for this request.",
                    finish_reason="STOP",
                )
            ]

        file_path = self._extract_file_path(message)
        content = self._extract_content(message)

        tool_call = ToolCall(
            name=tool_name,
            args={
                "file_path": file_path,
                "content": content,
                "justification": "Fake provider deterministic tool call for CI scenario",
                "target_operators": ["all"],
            },
            id="fake-tool-call-1",
        )

        return [
            StreamChunkFromModel(
                tool_calls=[tool_call],
                finish_reason="STOP",
                usage_metadata=UsageMetadata(
                    prompt_token_count=100,
                    candidates_token_count=50,
                    total_token_count=150,
                ),
            ),
        ]

    async def generate_content_stream_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        self.call_log.append(
            {
                "method": "generate_content_stream_primary",
                "model": model,
                "contents": contents,
                "primary_llm_settings": primary_llm_settings,
            }
        )
        message = self._extract_last_user_message(contents)
        logger.info("[FAKE-PROVIDER] stream_primary: message=%s", message[:100])
        for chunk in self._build_stream_chunks(message):
            yield chunk

    async def generate_content_primary(
        self,
        model: str,
        contents: list[Content],
        primary_llm_settings: PrimaryLLMSettings,
    ) -> GenerateContentResponse:
        self.call_log.append(
            {
                "method": "generate_content_primary",
                "model": model,
                "contents": contents,
                "primary_llm_settings": primary_llm_settings,
            }
        )
        message = self._extract_last_user_message(contents)
        logger.info("[FAKE-PROVIDER] primary: message=%s", message[:100])
        return self._build_tool_call_response(message)

    async def generate_content_stream_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        self.call_log.append(
            {
                "method": "generate_content_stream_assistant",
                "model": model,
                "contents": contents,
                "assistant_llm_settings": assistant_llm_settings,
            }
        )
        yield StreamChunkFromModel(text="Fake assistant response.", finish_reason="STOP")

    async def generate_content_assistant(
        self,
        model: str,
        contents: list[Content],
        assistant_llm_settings: AssistantLLMSettings,
    ) -> GenerateContentResponse:
        self.call_log.append(
            {
                "method": "generate_content_assistant",
                "model": model,
                "contents": contents,
                "assistant_llm_settings": assistant_llm_settings,
            }
        )
        return GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(role="model", parts=[Part(text="Fake assistant response.")]),
                    finish_reason="STOP",
                )
            ]
        )

    async def generate_content_stream_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        self.call_log.append(
            {
                "method": "generate_content_stream_lite",
                "model": model,
                "contents": contents,
                "lite_llm_settings": lite_llm_settings,
            }
        )
        yield StreamChunkFromModel(text="Fake lite response.", finish_reason="STOP")

    async def generate_content_lite(
        self,
        model: str,
        contents: list[Content],
        lite_llm_settings: LiteLLMSettings,
    ) -> GenerateContentResponse:
        self.call_log.append(
            {
                "method": "generate_content_lite",
                "model": model,
                "contents": contents,
                "lite_llm_settings": lite_llm_settings,
            }
        )
        return GenerateContentResponse(
            candidates=[
                Candidate(
                    content=Content(role="model", parts=[Part(text="Fake lite response.")]),
                    finish_reason="STOP",
                )
            ]
        )

    async def generate_content(
        self,
        model: str,
        contents: list[Content],
        config: Any,
        tools: list[ToolGroup] | None = None,
        system_instructions: str | None = None,
    ) -> GenerateContentResponse:
        self.call_log.append(
            {
                "method": "generate_content",
                "model": model,
                "contents": contents,
                "config": config,
                "tools": tools,
                "system_instructions": system_instructions,
            }
        )
        message = self._extract_last_user_message(contents)
        return self._build_tool_call_response(message)

    async def generate_content_stream(
        self,
        model: str,
        contents: list[Content],
        config: Any,
        tools: list[ToolGroup] | None = None,
        system_instructions: str | None = None,
    ) -> AsyncGenerator[StreamChunkFromModel]:
        self.call_log.append(
            {
                "method": "generate_content_stream",
                "model": model,
                "contents": contents,
                "config": config,
                "tools": tools,
                "system_instructions": system_instructions,
            }
        )
        message = self._extract_last_user_message(contents)
        for chunk in self._build_stream_chunks(message):
            yield chunk
