# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Operator LFAA Service

Publishes Local-First Audit Architecture audit events to g8eo operators
over pub/sub. Pure fire-and-forget publish - no approval, no pending store.
"""

import logging

from app.constants import EventType, G8EE_COMPONENT
from app.constants.generated_status import AITaskId
from app.models.command_request_payloads import DirectCommandAuditRequestPayload
from app.models.pubsub_messages import G8eMessage
from app.models.http_context import G8eHttpContext
from app.services.protocols import PubSubServiceProtocol

logger = logging.getLogger(__name__)


class OperatorLFAAService:
    """Publishes LFAA audit events to the Operator over pub/sub."""

    def __init__(self, pubsub_service: PubSubServiceProtocol) -> None:
        self.pubsub_service = pubsub_service

    async def send_audit_event(
        self,
        g8e_message: G8eMessage,
    ) -> bool:
        """
        Publishes a pre-constructed G8eMessage as an LFAA audit event.
        """
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

        try:
            if not self.pubsub_service or not self.pubsub_service.pubsub_client:
                logger.warning("[LFAA] Pub/sub client not initialized, cannot send audit event")
                return False

            subscribers = await self.pubsub_service.publish_command(
                operator_id=operator_id,
                operator_session_id=operator_session_id,
                command_data=g8e_message,
            )

            if subscribers > 0:
                logger.info(
                    "[LFAA] Sent audit event to operator",
                    extra={
                        "event_type": g8e_message.event_type,
                        "operator_id": operator_id,
                    },
                )
                return True
            logger.info(
                "[LFAA] No Operator listening for audit event",
                extra={"operator_id": operator_id},
            )
            return False

        except Exception as e:
            logger.warning("[LFAA] Failed to send audit event: %s", e)
            return False

    async def send_direct_exec_audit_event(
        self,
        command: str,
        execution_id: str,
        g8e_context: G8eHttpContext,
    ) -> bool:
        """
        Publishes an LFAA audit event for a direct operator terminal command.
        """
        if not g8e_context.bound_operators:
            return False
        bound = g8e_context.bound_operators[0]
        if not bound.operator_session_id:
            return False

        g8e_message = G8eMessage(
            id=f"audit_{execution_id}",
            source_component=G8EE_COMPONENT,
            event_type=EventType.OPERATOR_AUDIT_DIRECT_COMMAND_RECORDED,
            case_id=g8e_context.case_id,
            task_id=AITaskId.DIRECT_COMMAND,
            investigation_id=g8e_context.investigation_id,
            web_session_id=g8e_context.web_session_id,
            operator_session_id=bound.operator_session_id,
            operator_id=bound.operator_id,
            payload=DirectCommandAuditRequestPayload(
                command=command,
                execution_id=execution_id,
                operator_session_id=bound.operator_session_id,
            ),
        )
        return await self.send_audit_event(g8e_message)
