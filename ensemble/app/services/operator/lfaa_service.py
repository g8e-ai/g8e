# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Operator LFAA Service

Ingests Local-First Audit Architecture audit events through the Gateway-owned
HTTP surface. Records are acknowledged with chain metadata {seq, hash}.
"""

from __future__ import annotations

import asyncio
import logging
from typing import TYPE_CHECKING

from app.constants import EventType, G8EE_COMPONENT
from app.constants.generated_status import AITaskId
from app.errors import NetworkError
from app.models.command_request_payloads import DirectCommandAuditRequestPayload
from app.models.http_context import G8eHttpContext
from app.models.pubsub_messages import G8eMessage

if TYPE_CHECKING:
    from app.clients.gateway_operator_client import GatewayOperatorClient

logger = logging.getLogger(__name__)

_AUDIT_RECORDED_TO_REQUESTED: dict[str, str] = {
    EventType.OPERATOR_AUDIT_USER_RECORDED: EventType.OPERATOR_AUDIT_USER_RECORD_REQUESTED,
    EventType.OPERATOR_AUDIT_AI_RECORDED: EventType.OPERATOR_AUDIT_AI_RECORD_REQUESTED,
    EventType.OPERATOR_AUDIT_COMMAND_RECORDED: EventType.OPERATOR_AUDIT_COMMAND_RECORD_REQUESTED,
    EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORDED: EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORD_REQUESTED,
    EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RESULT_RECORDED: EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RESULT_RECORD_REQUESTED,
    EventType.OPERATOR_AUDIT_MCP_CALL_RECORDED: EventType.OPERATOR_AUDIT_MCP_CALL_RECORD_REQUESTED,
}

_MAX_AUDIT_RETRIES = 3


class AuditRecordError(Exception):
    """Raised when an LFAA audit record cannot be ingested after retries."""


class OperatorLFAAService:
    """Ingests LFAA audit events through Gateway HTTP."""

    def __init__(self, gateway_operator_client: GatewayOperatorClient) -> None:
        self._gateway_operator_client = gateway_operator_client

    async def send_audit_event(
        self,
        g8e_message: G8eMessage,
    ) -> bool:
        """Ingest a pre-constructed G8eMessage as an LFAA audit record."""
        operator_id = g8e_message.operator_id
        operator_session_id = g8e_message.operator_session_id

        if not g8e_message.payload or not operator_id or not operator_session_id:
            logger.info(
                "[LFAA] Skipping audit event - missing required fields",
                extra={
                    "has_payload": bool(g8e_message.payload),
                    "has_operator_id": bool(operator_id),
                    "has_operator_session_id": bool(operator_session_id),
                },
            )
            return False

        event_type = _AUDIT_RECORDED_TO_REQUESTED.get(
            g8e_message.event_type, g8e_message.event_type
        )
        idempotency_key = g8e_message.id or f"audit_{operator_session_id}_{event_type}"

        try:
            ack = await self._ingest_with_retry(
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                event_type=event_type,
                payload=g8e_message.payload.to_protobuf().SerializeToString(),
                idempotency_key=idempotency_key,
                case_id=g8e_message.case_id,
                investigation_id=g8e_message.investigation_id,
                task_id=g8e_message.task_id,
                web_session_id=g8e_message.web_session_id,
                cli_session_id=g8e_message.cli_session_id,
                user_id=g8e_message.user_id,
            )
            logger.info(
                "[LFAA] Audit record ingested",
                extra={
                    "event_type": event_type,
                    "operator_id": operator_id,
                    "seq": ack.get("seq"),
                    "hash": ack.get("hash"),
                },
            )
            return True
        except AuditRecordError as exc:
            logger.error("[LFAA] Audit record ingest failed: %s", exc)
            return False
        except Exception as exc:
            logger.warning("[LFAA] Failed to send audit event: %s", exc)
            return False

    async def send_direct_exec_audit_event(
        self,
        command: str,
        execution_id: str,
        g8e_context: G8eHttpContext,
    ) -> bool:
        """Ingest an LFAA audit record for a direct operator terminal command."""
        if not g8e_context.bound_operators:
            return False
        bound = g8e_context.bound_operators[0]
        if not bound.operator_session_id:
            return False

        g8e_message = G8eMessage(
            id=f"audit_{execution_id}",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORD_REQUESTED,
            case_id=g8e_context.case_id,
            task_id=AITaskId.DIRECT_COMMAND,
            investigation_id=g8e_context.investigation_id,
            web_session_id=g8e_context.web_session_id,
            user_id=g8e_context.user_id,
            cli_session_id=g8e_context.cli_session_id,
            operator_session_id=bound.operator_session_id,
            operator_id=bound.operator_id,
            payload=DirectCommandAuditRequestPayload(
                command=command,
                execution_id=execution_id,
                operator_session_id=bound.operator_session_id,
            ),
        )
        return await self.send_audit_event(g8e_message)

    async def _ingest_with_retry(
        self,
        *,
        operator_id: str,
        operator_session_id: str,
        event_type: str,
        payload: bytes,
        idempotency_key: str,
        case_id: str | None = None,
        investigation_id: str | None = None,
        task_id: str | None = None,
        web_session_id: str | None = None,
        cli_session_id: str | None = None,
        user_id: str | None = None,
    ) -> dict:
        delay = 0.25
        last_error: Exception | None = None
        for attempt in range(_MAX_AUDIT_RETRIES):
            try:
                return await self._gateway_operator_client.ingest_audit_record(
                    operator_id=operator_id,
                    operator_session_id=operator_session_id,
                    event_type=event_type,
                    payload=payload,
                    idempotency_key=idempotency_key,
                    case_id=case_id,
                    investigation_id=investigation_id,
                    task_id=task_id,
                    web_session_id=web_session_id,
                    cli_session_id=cli_session_id,
                    user_id=user_id,
                )
            except NetworkError as exc:
                last_error = exc
                if attempt == _MAX_AUDIT_RETRIES - 1:
                    break
                await asyncio.sleep(delay)
                delay *= 2
        raise AuditRecordError(str(last_error) if last_error else "audit ingest failed")
