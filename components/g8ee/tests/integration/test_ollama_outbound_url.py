# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0

"""
Integration test: outbound URL used by OllamaProvider.

Regression test for the bug where a stored endpoint of
`http://host:11434/v1` produced an outbound request to
`http://host:11434/v1/api/chat`. The normalization layer strips the
legacy `/v1` suffix; this test exercises the full construction path
(OllamaProvider -> ollama.AsyncClient -> httpx.AsyncClient) and asserts
the httpx request actually sent has path `/api/chat`.

Uses httpx.MockTransport to intercept traffic at the transport layer so
the real ollama Python client and all of its request-building logic
runs unmodified.
"""

import json

import httpx
import pytest

from app.constants import ThinkingLevel
from app.llm.llm_types import (
    Content,
    Part,
    PrimaryLLMSettings,
    ThinkingConfig,
    ToolCallingConfig,
    ToolConfig,
)
from app.llm.providers.ollama import OllamaProvider


pytestmark = [pytest.mark.integration, pytest.mark.asyncio]


def _install_mock_transport(provider: OllamaProvider, handler):
    """Swap the httpx transport on the provider's underlying client."""
    httpx_client = provider._client._client  # ollama AsyncClient -> httpx AsyncClient
    httpx_client._transport = httpx.MockTransport(handler)


@pytest.mark.parametrize("stored_endpoint", [
    "http://10.0.0.5:11434",
    "http://10.0.0.5:11434/v1",            # legacy double-append source
    "http://10.0.0.5:11434/v1/v1",         # paranoia: multiple /v1 strips
    "10.0.0.5:11434",                      # bare host:port (new preferred form)
])
async def test_outbound_url_is_api_chat(stored_endpoint):
    """Regardless of how the endpoint was stored historically, the actual
    outbound request must hit /api/chat -- never /v1/api/chat."""
    captured: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        body = (
            json.dumps({
                "model": "gemma4:e4b",
                "created_at": "2026-01-01T00:00:00Z",
                "message": {"role": "assistant", "content": "ok"},
                "done": True,
                "done_reason": "stop",
                "total_duration": 1,
                "load_duration": 1,
                "prompt_eval_count": 1,
                "eval_count": 1,
            }) + "\n"
        )
        return httpx.Response(
            200,
            content=body.encode("utf-8"),
            headers={"Content-Type": "application/x-ndjson"},
        )

    provider = OllamaProvider(endpoint=stored_endpoint, api_key="unused")
    _install_mock_transport(provider, handler)

    settings = PrimaryLLMSettings(
        system_instructions="",
        max_output_tokens=16,
        top_p_nucleus_sampling=1.0,
        top_k_filtering=40,
        stop_sequences=[],
        response_modalities=["TEXT"],
        tools=[],
        thinking_config=ThinkingConfig(thinking_level=ThinkingLevel.OFF, include_thoughts=False),
        tool_config=ToolConfig(tool_calling_config=ToolCallingConfig(mode="AUTO")),
    )
    contents = [Content(role="user", parts=[Part(text="ping")])]

    stream = provider.generate_content_stream_primary(
        model="gemma4:e4b",
        contents=contents,
        primary_llm_settings=settings,
    )
    async for _ in stream:
        pass

    await provider.close()

    assert captured, "OllamaProvider did not send any HTTP request"
    req = captured[0]
    assert req.url.path == "/api/chat", (
        f"Outbound path must be /api/chat, got {req.url.path!r} "
        f"(full url: {req.url})"
    )
    # Host must not retain /v1 in any form
    assert "/v1" not in str(req.url), f"Legacy /v1 leaked into URL: {req.url}"
    assert req.url.host == "10.0.0.5"
    assert req.url.port == 11434
