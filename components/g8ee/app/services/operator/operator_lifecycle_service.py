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

import logging
from typing import TYPE_CHECKING

from app.constants.collections import DB_COLLECTION_OPERATORS
from app.constants.status import (
    ComponentName,
    OperatorStatus,
    OperatorType,
)
from app.errors import ValidationError
from app.models.operators import OperatorDocument
from app.services.protocols import OperatorDataServiceProtocol
from app.utils.timestamp import now

if TYPE_CHECKING:
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
        # Access the underlying cache for direct document updates
        self._cache: "CacheAsideService" = operator_data_service.cache  # type: ignore

    async def claim_operator_slot(
        self,
        operator_id: str,
        operator_session_id: str,
        bound_web_session_id: str | None,
        system_info: dict,
        operator_type: OperatorType | str | None = None,
    ) -> bool:
        """Claim an operator slot for an active session.

        This orchestrates the transition from AVAILABLE to ACTIVE status,
        sets session bindings, and records first deployment if applicable.

        Authority: g8ee (single writer for the operator document).
        """
        operator = await self.operator_data_service.get_operator(operator_id)
        if not operator:
            logger.warning(f"[OPERATOR-LIFECYCLE] Cannot claim non-existent operator {operator_id}")
            return False

        now_timestamp = now()
        update_data: dict[str, object] = {
            "status": OperatorStatus.ACTIVE,
            "operator_session_id": operator_session_id,
            "bound_web_session_id": bound_web_session_id,
            "system_info": system_info,
            "system_fingerprint": system_info.get("system_fingerprint"),
            "claimed": True,
            "updated_at": now_timestamp,
            "last_heartbeat": now_timestamp,
        }

        # Record first deployment timestamp on initial claim
        if not operator.first_deployed:
            update_data["first_deployed"] = now_timestamp

        if operator_type:
            update_data["operator_type"] = operator_type

        # Perform the status update via cache
        result = await self._cache.update_document(
            collection=self.operator_data_service.collection,
            document_id=operator_id,
            data=update_data,
            merge=True,
        )

        if not result.success:
            logger.error(f"[OPERATOR-LIFECYCLE] Failed to update operator {operator_id} during claim: {result.error}")
            return False

        logger.info(f"[OPERATOR-LIFECYCLE] Operator slot claimed {operator_id}", extra={
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
        }

        result = await self._cache.update_document(
            collection=self.operator_data_service.collection,
            document_id=operator_id,
            data=update_data,
            merge=True,
        )

        if not result.success:
            raise ValidationError(f"Failed to terminate operator {operator_id}: {result.error}")

        # Update in-memory object to reflect the changes
        operator.status = OperatorStatus.TERMINATED
        operator.terminated_at = terminated_at
        operator.updated_at = terminated_at
        operator.operator_session_id = None
        operator.bound_web_session_id = None

        logger.info(f"[OPERATOR-LIFECYCLE] Operator terminated {operator_id}", extra={
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
        if operator and status == OperatorStatus.ACTIVE and not operator.last_heartbeat:
            updates["last_heartbeat"] = now_timestamp

        result = await self._cache.update_document(
            collection=self.operator_data_service.collection,
            document_id=operator_id,
            data=updates,
            merge=True,
        )

        return result.success
