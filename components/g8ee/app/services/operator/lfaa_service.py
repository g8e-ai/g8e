# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Operator LFAA Service

Publishes Local-First Audit Architecture audit events to VSA operators
over pub/sub. Pure fire-and-forget publish — no approval, no pending store.
"""

import logging
import uuid

from app.models.pubsub_messages import VSOMessage
from app.services.protocols import PubSubServiceProtocol

logger = logging.getLogger(__name__)


class OperatorLFAAService:
    """Publishes LFAA audit events to the Operator over pub/sub."""

    def __init__(self, pubsub_service: PubSubServiceProtocol) -> None:
        self.pubsub_service = pubsub_service

    async def send_audit_event(
        self,
        vso_message: VSOMessage,
    ) -> bool:
        """
        Publishes a pre-constructed VSOMessage as an LFAA audit event.
        """
        operator_id = vso_message.operator_id
        operator_session_id = vso_message.operator_session_id

        if not vso_message.payload or not operator_id or not operator_session_id:
            logger.info(
                "[LFAA] Skipping audit event - missing required fields",
                extra={
                    "has_payload": bool(vso_message.payload),
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
                command_data=vso_message,
            )

            if subscribers > 0:
                logger.info(
                    "[LFAA] Sent audit event to operator",
                    extra={
                        "event_type": vso_message.event_type,
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

