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

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

from app.constants.status import (
    ComponentName,
    OperatorHistoryEventType,
    OperatorStatus,
    OperatorType,
)
from app.errors import ValidationError
from app.models.operators import OperatorDocument
from app.services.protocols import OperatorDataServiceProtocol
from app.utils.timestamp import now

if TYPE_CHECKING:
    from app.services.auth.api_key_service import ApiKeyService
    from app.services.cache.cache_aside import CacheAsideService

logger = logging.getLogger(__name__)


class OperatorLifecycleService:
    """Domain service for operator lifecycle orchestration.

    This service handles business-critical orchestration (claim, terminate,
    history sealing) while delegating pure CRUD operations to OperatorDataService.
    Authority: g8ee is the single writer for the operator document.
    """

    def __init__(
        self,
        operator_data_service: OperatorDataServiceProtocol,
    ):
        self.operator_data_service = operator_data_service
        self._api_key_service: ApiKeyService | None = None
        # Access the underlying cache for direct document updates
        self._cache: CacheAsideService = operator_data_service.cache  # type: ignore

    @property
    def api_key_service(self) -> "ApiKeyService | None":
        return self._api_key_service

    def set_api_key_service(self, api_key_service: ApiKeyService) -> None:
        """Inject ApiKeyService after construction.

        ApiKeyService is created in a later factory phase than this service,
        so it is wired in via setter from ``ServiceFactory.create_all_services``
        once both objects exist.
        """
        self._api_key_service = api_key_service

    async def claim_operator_slot(
        self,
        operator_id: str,
        operator_session_id: str,
        bound_web_session_id: str | None,
        operator_type: OperatorType | str | None = None,
    ) -> bool:
        """Claim an operator slot for an active session.

        This orchestrates the transition from AVAILABLE to ACTIVE status,
        sets session bindings, and records first deployment if applicable.

        Authority: g8ee (single writer for the operator document).
        """
        operator = await self.operator_data_service.get_operator(operator_id)
        if not operator:
            logger.warning("[OPERATOR-LIFECYCLE] Cannot claim non-existent operator %s", operator_id)
            return False

        if operator.status != OperatorStatus.OFFLINE:
            logger.warning(
                "[OPERATOR-LIFECYCLE] Cannot claim operator %s: status is %s (expected OFFLINE)",
                operator_id,
                operator.status
            )
            return False

        now_timestamp = now()
        update_data: dict[str, object] = {
            "status": OperatorStatus.ACTIVE,
            "operator_session_id": operator_session_id,
            "bound_web_session_id": bound_web_session_id,
            "claimed": True,
            "updated_at": now_timestamp,
            "claimed_at": now_timestamp,
        }

        # Record first deployment timestamp on initial claim
        if not operator.first_deployed:
            update_data["first_deployed"] = now_timestamp

        if operator_type:
            update_data["operator_type"] = operator_type

        # Perform the status update and history append atomically via data service
        history_summary = f"Operator slot claimed by session {operator_session_id}"
        history_details = {
            "operator_session_id": operator_session_id,
            "bound_web_session_id": bound_web_session_id,
            "operator_type": str(operator_type) if operator_type else None,
        }

        try:
            await self.operator_data_service.add_history_entry(
                operator_id=operator_id,
                event_type=OperatorHistoryEventType.SLOT_CONSUMED,
                actor=ComponentName.G8EE,
                summary=history_summary,
                details=history_details,
                additional_updates=update_data
            )
        except Exception as e:
            logger.error("[OPERATOR-LIFECYCLE] Atomic claim failed for operator %s: %s", operator_id, e)
            return False

        logger.info("[OPERATOR-LIFECYCLE] Operator slot claimed %s", operator_id, extra={
            "operator_id": operator_id,
            "operator_session_id": operator_session_id,
        })

        return True

    async def terminate_operator(
        self,
        operator_id: str,
        actor: ComponentName = ComponentName.G8EE,
        summary: str = "Operator terminated",
        details: dict[str, object] | None = None,
    ) -> OperatorDocument:
        """Mark an operator TERMINATED.

        This orchestrates the termination workflow by updating status,
        timestamps, and clearing session bindings.

        Authority: g8ee (single writer for the operator document).
        """
        operator = await self.operator_data_service.get_operator(operator_id)
        if not operator:
            raise ValidationError(f"Operator {operator_id} not found")

        terminated_at = now()

        # Update status fields
        update_data: dict[str, object] = {
            "status": OperatorStatus.TERMINATED,
            "terminated_at": terminated_at,
            "updated_at": terminated_at,
            "operator_session_id": None,
            "bound_web_session_id": None,
            "claimed": False,
            "claimed_at": None,
        }

        # Perform the status update and history append atomically via data service
        try:
            operator = await self.operator_data_service.add_history_entry(
                operator_id=operator_id,
                event_type=OperatorHistoryEventType.TERMINATED,
                actor=actor,
                summary=summary,
                details=details,
                additional_updates=update_data
            )
        except Exception as e:
            logger.error("[OPERATOR-LIFECYCLE] Atomic termination failed for operator %s: %s", operator_id, e)
            raise ValidationError(f"Failed to terminate operator {operator_id}: {e}") from e

        logger.info("[OPERATOR-LIFECYCLE] Operator terminated %s", operator_id, extra={
            "operator_id": operator_id,
            "actor": actor.value,
        })

        return operator

    async def update_operator_status(
        self,
        operator_id: str,
        status: OperatorStatus,
    ) -> bool:
        """Update operator status.

        This orchestrates status transitions.

        Authority: g8ee (single writer for the operator document).
        """
        if not operator_id:
            raise ValidationError("operator_id is required")

        now_timestamp = now()
        updates: dict[str, object] = {
            "status": status,
        }

        operator = await self.operator_data_service.get_operator(operator_id)

        result = await self._cache.update_document(
            collection=self.operator_data_service.collection,
            document_id=operator_id,
            data=updates,
            merge=True,
        )

        return result.success

