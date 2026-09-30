# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Unit tests for the Jev decision provider HTTP adapter."""

from __future__ import annotations

import json

import httpx
import pytest

from app.decision.providers.jev import JevProvider
from app.decision.types import ChoiceQuestion, NoulQuestion, ScoreQuestion
from app.errors import ExternalServiceError, RateLimitError

pytestmark = pytest.mark.unit

_ENDPOINT = "http://localhost:11434"
_API_KEY = "proxy_token"


def _mock_transport(handler) -> httpx.AsyncClient:
    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


def _success_payload() -> dict:
    return {
        "model": "nimble",
        "answers": {
            "topic": {
                "type": "choice",
                "choice": "billing",
                "confidence": 1.0,
                "probabilities": {"billing": 1.0, "bug": 0.0},
            },
            "severity": {
                "type": "score",
                "score": 3.0,
                "confidence": 1.0,
                "probabilities": {"0": 0.0, "3": 1.0},
            },
            "escalate": {"type": "noul", "noul": 0.8},
        },
        "usage": {"input_tokens": 434, "output_tokens": 75},
    }


class TestJevProviderValidateConfig:
    def test_validate_config_does_not_require_api_key(self):
        assert JevProvider.validate_config(api_key=None, endpoint=_ENDPOINT) == []

    def test_validate_config_requires_endpoint(self):
        assert JevProvider.validate_config(api_key=None, endpoint=None) == [
            "Provider 'jev' requires an Ollama endpoint URL."
        ]


class TestJevProviderEvaluate:
    @pytest.mark.asyncio
    async def test_evaluate_maps_happy_path_response(self):
        captured: dict[str, object] = {}

        def handler(request: httpx.Request) -> httpx.Response:
            captured["authorization"] = request.headers.get("Authorization")
            captured["url"] = str(request.url)
            captured["body"] = json.loads(request.content.decode())
            return httpx.Response(200, json=_success_payload())

        provider = JevProvider(
            api_key=None,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        response = await provider.evaluate(
            model="nimble",
            state="Customer: I was charged twice.",
            questions={
                "topic": ChoiceQuestion(
                    instructions="What is the issue about?",
                    criteria={"billing": "money problems", "bug": "broken product"},
                ),
                "severity": ScoreQuestion(
                    instructions="How urgent is this?",
                    criteria=["routine", "today", "urgent", "critical"],
                ),
                "escalate": NoulQuestion(instructions="Escalate to a human now?"),
            },
        )

        assert response.model == "nimble"
        assert response.answers["topic"].choice == "billing"
        assert response.answers["severity"].score == 3.0
        assert response.answers["escalate"].noul == 0.8
        assert response.usage.input_tokens == 434
        assert captured["authorization"] is None
        assert captured["url"] == "http://localhost:11434/v1/systemone"
        assert captured["body"]["model"] == "nimble"
        assert provider.input_artifact_hash

    @pytest.mark.asyncio
    async def test_evaluate_sends_bearer_token_when_proxy_key_configured(self):
        captured: dict[str, object] = {}

        def handler(request: httpx.Request) -> httpx.Response:
            captured["authorization"] = request.headers.get("Authorization")
            return httpx.Response(200, json=_success_payload())

        provider = JevProvider(
            api_key=_API_KEY,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        await provider.evaluate(
            model="nimble",
            state="hello",
            questions={"topic": NoulQuestion(instructions="Escalate?")},
        )

        assert captured["authorization"] == f"Bearer {_API_KEY}"

    @pytest.mark.asyncio
    async def test_evaluate_rejects_oversized_request_before_sending(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            raise AssertionError("oversized request must not reach the network")

        provider = JevProvider(
            api_key=None,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        with pytest.raises(ExternalServiceError, match="limit is 65536"):
            await provider.evaluate(
                model="nimble",
                state="x" * 70_000,
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )

    @pytest.mark.asyncio
    async def test_evaluate_raises_on_404_when_model_not_pulled(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            return httpx.Response(404, text="model not found")

        provider = JevProvider(
            api_key=None,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        with pytest.raises(ExternalServiceError, match="HTTP 404"):
            await provider.evaluate(
                model="nimble",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )

    @pytest.mark.asyncio
    async def test_evaluate_raises_on_401(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            return httpx.Response(401, text="unauthorized")

        provider = JevProvider(
            api_key=_API_KEY,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        with pytest.raises(ExternalServiceError, match="HTTP 401"):
            await provider.evaluate(
                model="nimble",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )

    @pytest.mark.asyncio
    async def test_evaluate_raises_rate_limit_on_429(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            return httpx.Response(429, text="slow down")

        provider = JevProvider(
            api_key=_API_KEY,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        with pytest.raises(RateLimitError, match="rate limit"):
            await provider.evaluate(
                model="nimble",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )

    @pytest.mark.asyncio
    async def test_evaluate_raises_on_malformed_body(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            return httpx.Response(200, text="not-json")

        provider = JevProvider(
            api_key=_API_KEY,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        with pytest.raises(ExternalServiceError, match="not valid JSON"):
            await provider.evaluate(
                model="nimble",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )

    @pytest.mark.asyncio
    async def test_evaluate_raises_on_empty_answers(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            return httpx.Response(
                200,
                json={
                    "model": "nimble",
                    "answers": {},
                    "usage": {"input_tokens": 1, "output_tokens": 1},
                },
            )

        provider = JevProvider(
            api_key=_API_KEY,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        with pytest.raises(ExternalServiceError, match="missing answers"):
            await provider.evaluate(
                model="nimble",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )

    @pytest.mark.asyncio
    async def test_evaluate_raises_on_malformed_answer(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            return httpx.Response(
                200,
                json={
                    "model": "nimble",
                    "answers": {"topic": {"type": "choice", "choice": "billing"}},
                    "usage": {"input_tokens": 1, "output_tokens": 1},
                },
            )

        provider = JevProvider(
            api_key=_API_KEY,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        with pytest.raises(ExternalServiceError, match="malformed answer"):
            await provider.evaluate(
                model="nimble",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )
