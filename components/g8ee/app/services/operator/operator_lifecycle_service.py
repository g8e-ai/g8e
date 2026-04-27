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
from typing import TYPE_CHECKING, Any, List, Dict

from app.constants.collections import DB_COLLECTION_OPERATORS
from app.constants.status import (
    ComponentName,
    OperatorHistoryEventType,
    OperatorStatus,
    OperatorType,
)
from app.errors import ValidationError
from app.models.operators import OperatorDocument, OperatorHistoryEntry
from app.services.protocols import OperatorDataServiceProtocol, SupervisorServiceProtocol
from app.utils.timestamp import now

if TYPE_CHECKING:
    from app.services.cache.cache_aside import CacheAsideService
    from app.services.infra.settings_service import SettingsService

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
        supervisor_service: SupervisorServiceProtocol,
        settings_service: "SettingsService",
    ):
        self.operator_data_service = operator_data_service
        self.supervisor_service = supervisor_service
        self.settings_service = settings_service
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

        # Append history entry (best-effort, minimal inline append without keyed-lock atomicity)
        try:
            history_entry = OperatorHistoryEntry(
                event_type=OperatorHistoryEventType.SLOT_CONSUMED,
                actor=ComponentName.G8EE,
                summary=f"Operator slot claimed by session {operator_session_id}",
                prev_hash="0" * 64,
                details={
                    "operator_session_id": operator_session_id,
                    "bound_web_session_id": bound_web_session_id,
                    "operator_type": operator_type,
                }
            )
            append_result = await self._cache.append_to_array(
                collection=self.operator_data_service.collection,
                document_id=operator_id,
                array_field="history_trail",
                items=[history_entry.model_dump(mode="json")],
                additional_updates={"updated_at": now()}
            )
            if not append_result.success:
                logger.warning(f"[OPERATOR-LIFECYCLE] Failed to append history entry for operator {operator_id}: {append_result.error}")
        except Exception as e:
            logger.warning(f"[OPERATOR-LIFECYCLE] Failed to append history entry for operator {operator_id}: {e}")

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

        # Append history entry (best-effort, minimal inline append without keyed-lock atomicity)
        try:
            history_entry = OperatorHistoryEntry(
                event_type=OperatorHistoryEventType.TERMINATED,
                actor=actor,
                summary=summary,
                prev_hash="0" * 64,
                details=details or {}
            )
            append_result = await self._cache.append_to_array(
                collection=self.operator_data_service.collection,
                document_id=operator_id,
                array_field="history_trail",
                items=[history_entry.model_dump(mode="json")],
                additional_updates={"updated_at": terminated_at}
            )
            if not append_result.success:
                logger.warning(f"[OPERATOR-LIFECYCLE] Failed to append history entry for operator {operator_id}: {append_result.error}")
        except Exception as e:
            logger.warning(f"[OPERATOR-LIFECYCLE] Failed to append history entry for operator {operator_id}: {e}")

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

    async def activate_g8ep_operator(self, user_id: str) -> None:
        """Orchestrates g8ep operator activation after login.
        
        Authority: g8ee (process owner for g8ep operator).
        """
        try:
            # Find the specific slot designated as the g8ep for this user
            operators = await self.operator_data_service.query_operators([
                {"field": "user_id", "operator": "==", "value": user_id},
                {"field": "is_g8ep", "operator": "==", "value": True}
            ])

            if not operators:
                logger.info(f"[OPERATOR-LIFECYCLE] No g8ep operator slot found for user {user_id}")
                return

            operator = operators[0]
            
            # If already active/bound — nothing to do
            if operator.status in [OperatorStatus.ACTIVE, OperatorStatus.BOUND]:
                logger.info(f"[OPERATOR-LIFECYCLE] g8ep operator already active for user {user_id}", extra={
                    "operator_id": operator.id
                })
                return

            if not operator.api_key:
                logger.warning(f"[OPERATOR-LIFECYCLE] g8ep operator slot has no API key for user {user_id}", extra={
                    "operator_id": operator.id
                })
                return

            await self.launch_g8ep_operator(operator.api_key)

        except Exception as e:
            logger.warning(f"[OPERATOR-LIFECYCLE] g8ep operator activation failed (non-fatal): {str(e)}", extra={
                "user_id": user_id
            })

    async def launch_g8ep_operator(self, api_key: str) -> None:
        """Starts the g8ep operator process via XML-RPC.

        Authority: g8ee (process owner for g8ep operator).
        """
        logger.info("[OPERATOR-LIFECYCLE] Starting g8ep operator via XML-RPC")

        # 1. Persist API key to platform_settings (authority: g8ee)
        await self.settings_service.update_g8ep_operator_api_key(api_key)

        # 2. Start supervised process
        await self.supervisor_service.start_process("operator", wait=False)

        logger.info("[OPERATOR-LIFECYCLE] g8ep operator service signaled")

    async def relaunch_g8ep_operator(self, user_id: str) -> dict[str, object]:
        """Kills running operator, resets slot, and relaunches.
        
        Authority: g8ee (process owner for g8ep operator).
        """
        # Find the specific slot designated as the g8ep for this user
        operators = await self.operator_data_service.query_operators([
            {"field": "user_id", "operator": "==", "value": user_id},
            {"field": "is_g8ep", "operator": "==", "value": True}
        ])

        if not operators:
            logger.warning(f"[OPERATOR-LIFECYCLE] Relaunch requested but no g8ep operator slot found for user {user_id}")
            return {"success": False, "error": "No g8ep operator slot found for user"}

        operator = operators[0]
        operator_id = operator.id

        logger.info(f"[OPERATOR-LIFECYCLE] Relaunching g8ep operator for user {user_id}", extra={
            "operator_id": operator_id
        })

        # 1. Stop process
        await self.supervisor_service.stop_process("operator", wait=True)

        # 2. Reset operator slot (generates new API key)
        import secrets
        operator_suffix = operator_id.split('-')[-1][:8]
        random_token = secrets.token_hex(32)
        new_api_key = f"g8e_{operator_suffix}_{random_token}"

        update_data = {
            "api_key": new_api_key,
            "status": OperatorStatus.AVAILABLE,
            "updated_at": now(),
            "operator_session_id": None,
            "bound_web_session_id": None,
            "system_info": None,
            "system_fingerprint": None,
            "claimed": False,
        }

        reset_result = await self._cache.update_document(
            collection=self.operator_data_service.collection,
            document_id=operator_id,
            data=update_data,
            merge=True,
        )

        if not reset_result.success:
            logger.error(f"[OPERATOR-LIFECYCLE] Failed to reset operator {operator_id}: {reset_result.error}")
            return {"success": False, "error": reset_result.error}

        # 3. Launch with new key
        await self.launch_g8ep_operator(new_api_key)

        return {"success": True, "operator_id": operator_id}
