# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import base64
from unittest.mock import AsyncMock

import httpx
import pytest
from g8e.operator.v1.operator_pb2 import (
    EXECUTION_STATUS_COMPLETED,
    OllamaModelInventoryRequested,
    OllamaModelInventoryResult,
    ProviderModelInventoryEntry,
)

from app.constants.config import LLMProvider
from app.errors import ExternalServiceError, ServiceUnavailableError, ValidationError
from app.llm.model_catalog import list_governed_models, list_models, parse_models
from app.models.http_context import G8eHttpContext

pytestmark = pytest.mark.unit


def _client(handler) -> httpx.AsyncClient:
    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


async def test_ollama_lists_tags_sorted_and_deduplicated():
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(
            200,
            json={"models": [{"name": "qwen3:4b"}, {"name": "gemma4:e4b"}, {"name": "qwen3:4b"}]},
        )

    async with _client(handler) as client:
        models = await list_models(LLMProvider.OLLAMA, "http://ollama:11434/", None, client)

    assert models == ["gemma4:e4b", "qwen3:4b"]
    assert str(seen[0].url) == "http://ollama:11434/api/tags"
    assert "authorization" not in seen[0].headers


async def test_openai_compatible_adds_v1_and_bearer():
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(200, json={"data": [{"id": "gpt-b"}, {"id": "gpt-a"}]})

    async with _client(handler) as client:
        models = await list_models(LLMProvider.OPENAI, "https://api.example.com", "sk-1", client)

    assert models == ["gpt-a", "gpt-b"]
    assert str(seen[0].url) == "https://api.example.com/v1/models"
    assert seen[0].headers["authorization"] == "Bearer sk-1"


async def test_anthropic_sends_version_and_key_headers():
    seen: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen.append(request)
        return httpx.Response(200, json={"data": [{"id": "claude-x"}]})

    async with _client(handler) as client:
        models = await list_models(LLMProvider.ANTHROPIC, "https://api.anthropic.com", "k", client)

    assert models == ["claude-x"]
    assert seen[0].url.path == "/v1/models"
    assert seen[0].headers["x-api-key"] == "k"
    assert seen[0].headers["anthropic-version"]


def test_gemini_keeps_only_generate_content_models():
    body = {
        "models": [
            {"name": "models/gemini-pro", "supportedGenerationMethods": ["generateContent"]},
            {"name": "models/embedding-001", "supportedGenerationMethods": ["embedContent"]},
        ]
    }
    assert parse_models(LLMProvider.GEMINI, body) == ["gemini-pro"]


async def test_upstream_error_does_not_echo_body():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(401, text="secret upstream detail")

    async with _client(handler) as client:
        with pytest.raises(ExternalServiceError) as exc:
            await list_models(LLMProvider.OPENAI, "https://api.example.com", "bad", client)

    assert "HTTP 401" in exc.value.message
    assert "secret upstream detail" not in exc.value.message


async def test_unreachable_endpoint_is_external_service_error():
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("refused", request=request)

    async with _client(handler) as client:
        with pytest.raises(ExternalServiceError):
            await list_models(LLMProvider.OLLAMA, "http://nowhere:1", None, client)


@pytest.mark.parametrize(
    ("provider", "endpoint", "key"),
    [
        (LLMProvider.G8E, None, None),
        (LLMProvider.OLLAMA, None, None),
        (LLMProvider.ANTHROPIC, "https://api.anthropic.com", None),
        (LLMProvider.GEMINI, None, None),
    ],
)
async def test_missing_inputs_are_validation_errors(provider, endpoint, key):
    with pytest.raises(ValidationError):
        await list_models(provider, endpoint, key)


async def test_governed_models_use_owner_registered_inference_endpoint():
    context = G8eHttpContext(user_id="owner-1", web_session_id="ws-1")
    operator_client = AsyncMock()
    operator_client.list.return_value = [
        {
            "status": "active",
            "operator_type": "remote",
            "operator_session_id": "data-session",
            "runtime_config": {"inference_enabled": False},
        },
        {
            "status": "active",
            "operator_type": "remote",
            "operator_session_id": "inference-session",
            "runtime_config": {
                "inference_enabled": True,
                "inference_ollama_endpoint": "http://approved-ollama:11434",
            },
        },
    ]
    inventory = OllamaModelInventoryResult(
        status=EXECUTION_STATUS_COMPLETED,
        entries=[ProviderModelInventoryEntry(served_model_tag="qwen3:4b")],
    )
    operator_client.dispatch.return_value = {
        "success": True,
        "result_payload": base64.b64encode(inventory.SerializeToString()).decode(),
    }
    assert await list_governed_models(operator_client, context) == ["qwen3:4b"]
    operator_client.list.assert_awaited_once_with(user_id="owner-1")
    call = operator_client.dispatch.await_args.kwargs
    assert call["context"] is context
    assert call["operator_session_id"] == "inference-session"
    assert call["event_type"] == "g8e.v1.operator.ollama.model.inventory.requested"
    assert OllamaModelInventoryRequested.FromString(call["payload"]).execution_id


@pytest.mark.parametrize("count", [0, 2])
async def test_governed_models_require_one_active_inference_operator(count):
    operator_client = AsyncMock()
    operator_client.list.return_value = [
        {
            "status": "active",
            "operator_type": "remote",
            "operator_session_id": f"inference-{i}",
            "runtime_config": {
                "inference_enabled": True,
                "inference_ollama_endpoint": "http://approved-ollama:11434",
            },
        }
        for i in range(count)
    ]
    with pytest.raises(ServiceUnavailableError):
        await list_governed_models(
            operator_client, G8eHttpContext(user_id="owner-1", web_session_id="ws-1")
        )
