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

_ENDPOINT = "https://api.typesafe.ai/v1/systemone"
_API_KEY = "ts_test_key"


def _mock_transport(handler) -> httpx.AsyncClient:
    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


def _success_payload() -> dict:
    return {
        "model": "jev-1.13.0",
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
    def test_validate_config_requires_api_key(self):
        assert JevProvider.validate_config(api_key=None, endpoint=_ENDPOINT) == [
            "Provider 'jev' requires an API key."
        ]

    def test_validate_config_accepts_api_key(self):
        assert JevProvider.validate_config(api_key=_API_KEY, endpoint=None) == []


class TestJevProviderEvaluate:
    @pytest.mark.asyncio
    async def test_evaluate_maps_happy_path_response(self):
        captured: dict[str, object] = {}

        def handler(request: httpx.Request) -> httpx.Response:
            captured["authorization"] = request.headers.get("Authorization")
            captured["body"] = json.loads(request.content.decode())
            return httpx.Response(200, json=_success_payload())

        provider = JevProvider(
            api_key=_API_KEY,
            endpoint=_ENDPOINT,
            client=_mock_transport(handler),
        )

        response = await provider.evaluate(
            model="jev-latest",
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

        assert response.model == "jev-1.13.0"
        assert response.answers["topic"].choice == "billing"
        assert response.answers["severity"].score == 3.0
        assert response.answers["escalate"].noul == 0.8
        assert response.usage.input_tokens == 434
        assert captured["authorization"] == f"Bearer {_API_KEY}"
        assert captured["body"]["model"] == "jev-latest"
        assert provider.input_artifact_hash

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
                model="jev-latest",
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
                model="jev-latest",
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
                model="jev-latest",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )

    @pytest.mark.asyncio
    async def test_evaluate_raises_on_empty_answers(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            return httpx.Response(
                200,
                json={
                    "model": "jev-latest",
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
                model="jev-latest",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )

    @pytest.mark.asyncio
    async def test_evaluate_raises_on_malformed_answer(self):
        def handler(_request: httpx.Request) -> httpx.Response:
            return httpx.Response(
                200,
                json={
                    "model": "jev-latest",
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
                model="jev-latest",
                state="hello",
                questions={"topic": NoulQuestion(instructions="Escalate?")},
            )
