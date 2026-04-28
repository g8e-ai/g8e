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

"""
Operator Command Execution Security Validator

Provides security validation for Operator commands before execution.
Validates session tokens, Operator status, and authorization.
"""

import logging
from typing import Any

from app.constants import HEARTBEAT_STALE_THRESHOLD_SECONDS
from app.errors import AuthenticationError, AuthorizationError, ResourceNotFoundError
from app.models.operators import BindingValidationResult, HealthCheckResultModel, OperatorStatus
from app.utils.timestamp import now

from .request_timestamp import validate_request_timestamp

logger = logging.getLogger(__name__)


class OperatorCommandValidator:
    """
    Security validator for Operator command execution.
    
    Ensures that:
    1. WebSession token is valid and not expired
    2. Operator status allows command execution
    3. System fingerprint matches (if provided)
    4. User authorization is valid
    """

    def __init__(self, operator_cache, session_service=None):
        self.operator_cache = operator_cache
        self.session_service = session_service
        self.allowed_statuses = {
            OperatorStatus.ACTIVE,
            OperatorStatus.BOUND,
            OperatorStatus.STALE
        }

    async def validate_command_execution(
        self,
        operator_id: str,
        session_token: str | None = None,
        system_info: dict[str, object] | None = None,
        command: str | None = None,
        request_timestamp: str | None = None,
        request_nonce: str | None = None,
        cache_aside_service: Any | None = None,
    ) -> bool:
        """
        Validate that command execution is authorized for this operator.

        Args:
            system_info: Request-scoped client attestation dict (NOT the operator doc field).
                This is a per-command attestation bag from the operator, threaded to
                session_service.validate_session. It is unrelated to the deprecated
                OperatorDocument.system_info field.

        Returns:
            bool: True if command execution is authorized

        Raises:
            AuthenticationError: If session is invalid or expired
            AuthorizationError: If Operator status prohibits execution
            ResourceNotFoundError: If Operator does not exist
        """
        if request_timestamp:
            ts_result = await validate_request_timestamp(
                timestamp_str=request_timestamp,
                nonce=request_nonce,
                require_nonce=False,
                cache_aside_service=cache_aside_service,
                context={"operator_id": operator_id, "command": command if command else None}
            )
            if not ts_result.is_valid:
                logger.error("Command rejected - timestamp validation failed", extra={
                    "operator_id": operator_id,
                    "error": ts_result.error,
                    "error_code": ts_result.error_code,
                    "security_event": "command_timestamp_rejected"
                })
                raise AuthenticationError(f"Request timestamp validation failed: {ts_result.error}")

        operator = await self.operator_cache.get_operator(operator_id)

        if not operator:
            logger.error("Operator not found", extra={"operator_id": operator_id})
            raise ResourceNotFoundError(
                message=f"Operator {operator_id} not found",
                resource_type="operator",
                resource_id=operator_id,
                component="g8ee"
            )

        if operator.status not in self.allowed_statuses:
            logger.error("Operator status prohibits command execution", extra={
                "operator_id": operator_id,
                "current_status": operator.status,
                "allowed_statuses": list(self.allowed_statuses)
            })
            raise AuthorizationError(
                f"Operator status '{operator.status}' does not allow command execution",
                component="g8ee"
            )

        if session_token and self.session_service:
            await self._validate_session(
                operator_id=operator_id,
                session_token=session_token,
                operator=operator,
                system_info=system_info
            )
        elif operator.session_token and operator.session_expires_at:
            logger.error("WebSession token required but not provided", extra={"operator_id": operator_id})
            raise AuthenticationError("WebSession token required for command execution", component="g8ee")

        if operator.last_heartbeat:
            time_since_heartbeat = (now() - operator.last_heartbeat).total_seconds()
            if time_since_heartbeat > HEARTBEAT_STALE_THRESHOLD_SECONDS:
                logger.warning("Operator heartbeat is stale", extra={
                    "operator_id": operator_id,
                    "last_heartbeat": operator.last_heartbeat.isoformat(),
                    "seconds_since": time_since_heartbeat
                })

        logger.info("Command execution authorized", extra={
            "operator_id": operator_id,
            "user_id": operator.user_id,
            "status": operator.status,
            "command": command[:50] if command else None
        })

        return True

    async def _validate_session(
        self,
        operator_id: str,
        session_token: str,
        operator,
        system_info: dict[str, object] | None
    ):
        if operator.session_expires_at:
            if now() > operator.session_expires_at:
                logger.error("WebSession expired", extra={
                    "operator_id": operator_id,
                    "expires_at": operator.session_expires_at.isoformat()
                })
                raise AuthenticationError("WebSession expired - Operator must re-bootstrap", component="g8ee")

        if operator.session_token and operator.session_token != session_token:
            logger.error("WebSession token mismatch", extra={
                "operator_id": operator_id,
                "provided_token_prefix": session_token[:20] + "...",
                "expected_token_prefix": operator.session_token[:20] + "..."
            })
            raise AuthenticationError("Invalid session token", component="g8ee")

        if self.session_service:
            try:
                await self.session_service.validate_session(
                    session_token=session_token,
                    operator_id=operator_id,
                    system_info=system_info
                )
            except Exception as e:
                logger.error("WebSession service validation failed", extra={
                    "operator_id": operator_id,
                    "error": str(e)
                })
                raise AuthenticationError(f"WebSession validation failed: {e!s}", component="g8ee")

        logger.info("WebSession validated successfully", extra={
            "operator_id": operator_id,
            "session_token_prefix": session_token[:25] + "..."
        })

    async def validate_operator_ownership(
        self,
        operator_id: str,
        user_id: str
    ) -> bool:
        """
        Validate that user owns the operator.

        Raises:
            ResourceNotFoundError: If Operator does not exist
            AuthorizationError: If user doesn't own operator
        """
        operator = await self.operator_cache.get_operator(operator_id)

        if not operator:
            raise ResourceNotFoundError(
                message=f"Operator {operator_id} not found",
                resource_type="operator",
                resource_id=operator_id,
                component="g8ee"
            )

        if operator.user_id != user_id:
            logger.error("User does not own operator", extra={
                "operator_id": operator_id,
                "requesting_user_id": user_id,
                "owner_user_id": operator.user_id
            })
            raise AuthorizationError("Not authorized to control this operator", component="g8ee")

        return True

    async def validate_operator_binding(
        self,
        operator_session_id: str,
        web_session_id: str | None,
        operator_id: str,
        investigation_id: str = "unknown"
    ) -> BindingValidationResult:
        """
        CRITICAL BINDING ENFORCEMENT: Validate that operator_session_id is properly bound to web_session_id.

        This prevents commands from being routed to operators that are not bound to the requesting
        web session, which would be a critical security violation.
        """
        if not operator_session_id:
            return BindingValidationResult(
                valid=False,
                reason="operator_session_id is required",
                operator_session_id=None,
                web_session_id=web_session_id,
                operator_id=operator_id,
                operator_bound_web_session_id=None
            )

        if not web_session_id:
            return BindingValidationResult(
                valid=False,
                reason="web_session_id is required for bound operators",
                operator_session_id=operator_session_id,
                web_session_id=None,
                operator_id=operator_id,
                operator_bound_web_session_id=None
            )

        operator = await self.operator_cache.get_operator(operator_id)
        if not operator:
            return BindingValidationResult(
                valid=False,
                reason=f"Operator {operator_id} not found",
                operator_session_id=operator_session_id,
                web_session_id=web_session_id,
                operator_id=operator_id,
                operator_bound_web_session_id=None
            )

        operator_bound_web_session_id = operator.bound_web_session_id
        operator_operator_session_id = operator.operator_session_id
        operator_status = operator.status

        if operator_status != OperatorStatus.BOUND:
            return BindingValidationResult(
                valid=False,
                reason=f"Operator {operator_id} is not bound (status={operator_status})",
                operator_session_id=operator_session_id,
                web_session_id=web_session_id,
                operator_id=operator_id,
                operator_status=operator_status,
                operator_bound_web_session_id=operator_bound_web_session_id
            )

        if operator_bound_web_session_id != web_session_id:
            return BindingValidationResult(
                valid=False,
                reason="Operator bound_web_session_id mismatch - Operator bound to different session",
                operator_session_id=operator_session_id,
                web_session_id=web_session_id,
                operator_id=operator_id,
                operator_bound_web_session_id=operator_bound_web_session_id
            )

        if operator_operator_session_id != operator_session_id:
            return BindingValidationResult(
                valid=False,
                reason="Operator session ID mismatch - stale session ID provided",
                operator_session_id=operator_session_id,
                expected_operator_session_id=operator_operator_session_id,
                web_session_id=web_session_id,
                operator_id=operator_id,
                operator_bound_web_session_id=operator_bound_web_session_id
            )

        logger.info("[BINDING-VALIDATED] Operator binding validated successfully", extra={
            "operator_id": operator_id,
            "operator_session_id": operator_session_id,
            "web_session_id": web_session_id,
            "investigation_id": investigation_id,
            "operator_status": operator_status
        })

        return BindingValidationResult(
            valid=True,
            reason="Binding validated successfully",
            operator_session_id=operator_session_id,
            web_session_id=web_session_id,
            operator_id=operator_id,
            operator_status=operator_status,
            operator_bound_web_session_id=operator_bound_web_session_id
        )

    async def check_operator_health(self, operator_id: str) -> HealthCheckResultModel:
        """Check Operator health status."""
        operator = await self.operator_cache.get_operator(operator_id)

        if not operator:
            return HealthCheckResultModel(
                healthy=False,
                reason="Operator not found",
                operator_id=operator_id,
                status=OperatorStatus.UNAVAILABLE
            )

        health_status = HealthCheckResultModel(
            healthy=operator.status in self.allowed_statuses,
            operator_id=operator_id,
            status=operator.status,
            last_heartbeat=operator.last_heartbeat,
            session_expires_at=operator.session_expires_at
        )

        if operator.last_heartbeat:
            seconds_since_heartbeat = (now() - operator.last_heartbeat).total_seconds()
            health_status.seconds_since_heartbeat = seconds_since_heartbeat
            health_status.heartbeat_stale = seconds_since_heartbeat > HEARTBEAT_STALE_THRESHOLD_SECONDS

        if operator.session_expires_at:
            health_status.session_expired = now() > operator.session_expires_at

        if not health_status.healthy:
            health_status.reason = f"Operator status is {operator.status}"
        elif health_status.session_expired:
            health_status.healthy = False
            health_status.reason = "WebSession expired"
        elif health_status.heartbeat_stale:
            health_status.reason = "Heartbeat is stale"

        return health_status
