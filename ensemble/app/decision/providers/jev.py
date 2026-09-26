# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""TypeSafe System One (Jev) decision provider adapter."""

from __future__ import annotations

import logging
from typing import Any

import httpx
from pydantic import TypeAdapter, ValidationError

from app.constants import DEFAULT_HTTP_CLIENT_TIMEOUT, JEV_DEFAULT_ENDPOINT
from app.decision.provider import DecisionProvider
from app.decision.types import (
    Answer,
    ChoiceQuestion,
    DecisionState,
    EvaluateResponse,
    EvaluateUsage,
    NoulQuestion,
    Question,
    ScoreQuestion,
)
from app.errors import ExternalServiceError, RateLimitError

logger = logging.getLogger(__name__)

_ANSWER_ADAPTER: TypeAdapter[Answer] = TypeAdapter(Answer)
_QUESTION_WIRE_TYPES = {
    ChoiceQuestion: "choice",
    ScoreQuestion: "score",
    NoulQuestion: "noul",
}


class JevProvider(DecisionProvider):
    """HTTP adapter for the TypeSafe System One API."""

    def __init__(
        self,
        *,
        api_key: str | None,
        endpoint: str = JEV_DEFAULT_ENDPOINT,
        default_model: str | None = None,
        client: httpx.AsyncClient | None = None,
    ) -> None:
        super().__init__()
        self._api_key = api_key
        self._endpoint = endpoint.rstrip("/")
        self._default_model = default_model
        self._client = client
        self._owns_client = client is None

    @staticmethod
    def validate_config(api_key: str | None, endpoint: str | None) -> list[str]:
        errors: list[str] = []
        if not api_key:
            errors.append("Provider 'jev' requires an API key.")
        return errors

    def _build_client(self) -> httpx.AsyncClient:
        return httpx.AsyncClient(timeout=DEFAULT_HTTP_CLIENT_TIMEOUT)

    async def _get_client(self) -> httpx.AsyncClient:
        if self._client is None:
            self._client = self._build_client()
        return self._client

    @staticmethod
    def _question_to_wire(question: Question) -> dict[str, Any]:
        payload = question.model_dump(mode="json", exclude_none=True)
        payload["type"] = _QUESTION_WIRE_TYPES[type(question)]
        return payload

    @staticmethod
    def _build_request_body(
        *,
        model: str,
        state: DecisionState,
        questions: dict[str, Question],
    ) -> dict[str, Any]:
        return {
            "model": model,
            "state": state,
            "questions": {
                name: JevProvider._question_to_wire(question)
                for name, question in questions.items()
            },
        }

    @staticmethod
    def _parse_answers(raw_answers: object) -> dict[str, Answer]:
        if not isinstance(raw_answers, dict) or not raw_answers:
            raise ExternalServiceError(
                "Jev response missing answers.",
                service_name="typesafe",
                details={"operation": "evaluate"},
            )

        parsed: dict[str, Answer] = {}
        for name, raw_answer in raw_answers.items():
            try:
                parsed[name] = _ANSWER_ADAPTER.validate_python(raw_answer)
            except ValidationError as exc:
                raise ExternalServiceError(
                    f"Jev returned malformed answer for question '{name}'.",
                    service_name="typesafe",
                    cause=exc,
                    details={"operation": "evaluate", "question": name},
                ) from exc
        return parsed

    @staticmethod
    def _parse_usage(raw_usage: object) -> EvaluateUsage:
        if not isinstance(raw_usage, dict):
            raise ExternalServiceError(
                "Jev response missing usage.",
                service_name="typesafe",
                details={"operation": "evaluate"},
            )
        try:
            return EvaluateUsage.model_validate(raw_usage)
        except ValidationError as exc:
            raise ExternalServiceError(
                "Jev response contained malformed usage.",
                service_name="typesafe",
                cause=exc,
                details={"operation": "evaluate"},
            ) from exc

    @staticmethod
    def _translate_http_error(response: httpx.Response) -> ExternalServiceError | RateLimitError:
        if response.status_code == 429:
            return RateLimitError(
                "Jev rate limit exceeded.",
                component="typesafe",
            )
        return ExternalServiceError(
            f"Jev request failed with HTTP {response.status_code}.",
            service_name="typesafe",
            details={
                "operation": "evaluate",
                "status_code": response.status_code,
                "response_body": response.text[:500],
            },
        )

    async def evaluate(
        self,
        *,
        model: str,
        state: DecisionState,
        questions: dict[str, Question],
    ) -> EvaluateResponse:
        if not self._api_key:
            raise ExternalServiceError(
                "Jev provider is missing an API key.",
                service_name="typesafe",
                details={"operation": "evaluate"},
            )
        if not questions:
            raise ExternalServiceError(
                "Jev evaluate requires at least one question.",
                service_name="typesafe",
                details={"operation": "evaluate"},
            )

        resolved_model = model or self._default_model
        if not resolved_model:
            raise ExternalServiceError(
                "Jev evaluate requires a model name.",
                service_name="typesafe",
                details={"operation": "evaluate"},
            )

        request_body = self._build_request_body(
            model=resolved_model,
            state=state,
            questions=questions,
        )
        self._record_model_boundary(request_body)

        client = await self._get_client()
        try:
            response = await client.post(
                self._endpoint,
                headers={
                    "Authorization": f"Bearer {self._api_key}",
                    "Content-Type": "application/json",
                },
                json=request_body,
            )
        except httpx.HTTPError as exc:
            raise ExternalServiceError(
                "Jev request failed.",
                service_name="typesafe",
                cause=exc,
                details={"operation": "evaluate"},
            ) from exc

        if response.status_code >= 400:
            raise self._translate_http_error(response)

        try:
            payload = response.json()
        except ValueError as exc:
            raise ExternalServiceError(
                "Jev response was not valid JSON.",
                service_name="typesafe",
                cause=exc,
                details={"operation": "evaluate"},
            ) from exc

        if not isinstance(payload, dict):
            raise ExternalServiceError(
                "Jev response body was not a JSON object.",
                service_name="typesafe",
                details={"operation": "evaluate"},
            )

        resolved_response_model = payload.get("model")
        if not isinstance(resolved_response_model, str) or not resolved_response_model:
            raise ExternalServiceError(
                "Jev response missing model.",
                service_name="typesafe",
                details={"operation": "evaluate"},
            )

        return EvaluateResponse(
            model=resolved_response_model,
            answers=self._parse_answers(payload.get("answers")),
            usage=self._parse_usage(payload.get("usage")),
        )

    async def _close_resources(self) -> None:
        if self._client is not None and self._owns_client:
            await self._client.aclose()
            self._client = None
