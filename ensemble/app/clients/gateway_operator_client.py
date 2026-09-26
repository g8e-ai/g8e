# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Thin client for Gateway-owned Operator protocol endpoints.

This module deliberately contains transport DTOs only. It does not cache,
authenticate, persist, or mutate Operator state in g8ee.
"""

from __future__ import annotations

import base64
from typing import TYPE_CHECKING, Any

from app.clients.http_client import GATEWAY_IDEMPOTENT_POST_RETRY_CONFIG
from app.errors import NetworkError
from app.models.http_context import G8eHttpContext

if TYPE_CHECKING:
    from app.services.infra.internal_http_client import InternalHttpClient


class GatewayOperatorClient:
    """Call the Gateway's Operator API without creating a g8ee Operator service."""

    def __init__(self, internal_http_client: InternalHttpClient) -> None:
        self._internal_http_client = internal_http_client

    async def list(self, *, user_id: str) -> list[dict[str, Any]]:
        self._ensure_mtls()
        response = await self._internal_http_client.client.get(
            "/api/v1/operators",
            params={"user_id": user_id},
        )
        self._raise_for_failure(response, "list operators")
        payload = response.json()
        if isinstance(payload, dict):
            operators = payload.get("operators", [])
            return operators if isinstance(operators, list) else []
        return payload if isinstance(payload, list) else []

    async def get_by_session(self, *, session_id: str) -> dict[str, Any] | None:
        self._ensure_mtls()
        response = await self._internal_http_client.client.get(
            f"/api/v1/operators/session/{session_id}"
        )
        if response.status_code == 404:
            return None
        self._raise_for_failure(response, "get operator by session")
        payload = response.json()
        if isinstance(payload, dict) and "operator" in payload:
            return payload["operator"]
        return payload if isinstance(payload, dict) else None

    async def bind(self, *, context: G8eHttpContext, operator_ids: list[str]) -> dict[str, Any]:
        return await self._post(
            "/api/v1/operators/bind",
            {
                "operator_ids": operator_ids,
                "user_id": context.user_id,
                "web_session_id": context.web_session_id or "",
            },
            "bind operators",
        )

    async def unbind(self, *, context: G8eHttpContext, operator_ids: list[str]) -> dict[str, Any]:
        return await self._post(
            "/api/v1/operators/unbind",
            {
                "operator_ids": operator_ids,
                "user_id": context.user_id,
                "web_session_id": context.web_session_id or "",
            },
            "unbind operators",
        )

    async def stop(
        self, *, context: G8eHttpContext, operator_session_id: str, reason: str = ""
    ) -> dict[str, Any]:
        return await self._post(
            "/api/v1/operators/stop",
            {"operator_session_id": operator_session_id, "reason": reason},
            "stop operator",
        )

    async def terminate(
        self, *, context: G8eHttpContext, operator_id: str, reason: str = ""
    ) -> dict[str, Any]:
        return await self._post(
            f"/api/v1/operators/{operator_id}",
            {"operator_id": operator_id, "user_id": context.user_id, "reason": reason},
            "terminate operator",
        )

    async def validate_session(
        self,
        *,
        context: G8eHttpContext,
        operator_session_id: str,
        cli_session_id: str,
    ) -> dict[str, Any]:
        return await self._post(
            "/api/v1/operators/validate",
            {
                "operator_session_id": operator_session_id,
                "cli_session_id": cli_session_id,
                "user_id": context.user_id,
            },
            "validate operator session",
        )

    async def dispatch(
        self,
        *,
        context: G8eHttpContext,
        operator_session_id: str,
        event_type: str,
        payload: bytes,
        target_resource: str | None = None,
    ) -> dict[str, Any]:
        """Dispatch a typed operator request event through Gateway governance."""
        body: dict[str, Any] = {
            "target_operator_session_id": operator_session_id,
            "event_type": event_type,
            "payload": base64.b64encode(payload).decode("ascii"),
        }
        if target_resource:
            body["target_resource"] = target_resource
        if context.case_id:
            body["case_id"] = context.case_id
        if context.investigation_id:
            body["investigation_id"] = context.investigation_id
        if context.task_id:
            body["task_id"] = context.task_id
        if context.web_session_id:
            body["web_session_id"] = context.web_session_id
        if context.cli_session_id:
            body["cli_session_id"] = context.cli_session_id
        return await self._post("/api/v1/operators/commands", body, "dispatch operator command")

    def _ensure_mtls(self) -> None:
        self._internal_http_client._ensure_mtls()

    async def _post(
        self, path: str, body: dict[str, Any], operation: str
    ) -> dict[str, Any]:
        self._ensure_mtls()
        response = await self._internal_http_client.client.post(
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
