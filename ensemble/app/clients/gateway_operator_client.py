# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License 2.0.

"""Thin client for Gateway-owned Operator protocol endpoints.

This module deliberately contains transport DTOs only. It does not cache,
authenticate, persist, or mutate Operator state in g8ee.
"""

from __future__ import annotations

import base64
from typing import Any

from app.clients.http_client import GATEWAY_IDEMPOTENT_POST_RETRY_CONFIG, HTTPClient
from app.errors import NetworkError
from app.models.http_context import G8eHttpContext


class GatewayOperatorClient:
    """Call the Gateway's Operator API without creating a g8ee Operator service."""

    def __init__(self, http_client: HTTPClient) -> None:
        self._http = http_client

    async def list(self, *, user_id: str) -> list[dict[str, Any]]:
        response = await self._http.get(
            "/api/v1/operators",
            params={"user_id": user_id},
        )
        self._raise_for_failure(response, "list operators")
        payload = response.json()
        if isinstance(payload, dict):
            operators = payload.get("operators", [])
            return operators if isinstance(operators, list) else []
        return payload if isinstance(payload, list) else []

    async def bind(self, *, context: G8eHttpContext, operator_ids: list[str]) -> dict[str, Any]:
        return await self._post(
            "/api/v1/operators/bind",
            {"operator_ids": operator_ids, "context": context.model_dump(mode="json")},
            "bind operators",
        )

    async def unbind(self, *, context: G8eHttpContext, operator_ids: list[str]) -> dict[str, Any]:
        return await self._post(
            "/api/v1/operators/unbind",
            {"operator_ids": operator_ids, "context": context.model_dump(mode="json")},
            "unbind operators",
        )

    async def stop(
        self, *, context: G8eHttpContext, operator_id: str
    ) -> dict[str, Any]:
        return await self._post(
            "/api/v1/operators/stop",
            {"operator_id": operator_id, "context": context.model_dump(mode="json")},
            "stop operator",
        )

    async def terminate(
        self, *, context: G8eHttpContext, operator_id: str
    ) -> dict[str, Any]:
        return await self._post(
            "/api/v1/operators/terminate",
            {"operator_id": operator_id, "context": context.model_dump(mode="json")},
            "terminate operator",
        )

    async def dispatch(
        self,
        *,
        context: G8eHttpContext,
        operator_session_id: str,
        action_type: str,
        payload: bytes,
        target_resource: str | None = None,
    ) -> dict[str, Any]:
        """Dispatch a typed operator payload through Gateway governance."""
        body: dict[str, Any] = {
            "target_operator_session_id": operator_session_id,
            "action_type": action_type,
            "payload": base64.b64encode(payload).decode("ascii"),
            "context": context.model_dump(mode="json"),
        }
        if target_resource:
            body["target_resource"] = target_resource
        return await self._post("/api/v1/operators/commands", body, "dispatch operator command")

    async def _post(
        self, path: str, body: dict[str, Any], operation: str
    ) -> dict[str, Any]:
        response = await self._http.post(
            path,
            json_data=body,
            retry_config=GATEWAY_IDEMPOTENT_POST_RETRY_CONFIG,
        )
        self._raise_for_failure(response, operation)
        payload = response.json()
        return payload if isinstance(payload, dict) else {"result": payload}

    @staticmethod
    def _raise_for_failure(response: Any, operation: str) -> None:
        if response.is_success:
            return
        raise NetworkError(
            f"Gateway failed to {operation}: HTTP {response.status_code}",
            component="g8ee",
            details={"status_code": response.status_code, "response": response.text},
        )
