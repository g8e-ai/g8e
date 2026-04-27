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
from typing import Any

from app.constants import (
    DB_COLLECTION_USERS,
    ComponentName,
    OperatorHistoryEventType,
    OperatorStatus,
)
from app.models.sessions import OperatorSessionDocument
from app.services.auth.api_key_service import ApiKeyService
from app.services.auth.certificate_service import CertificateService
from app.services.operator.operator_session_service import OperatorSessionService
from app.services.operator.operator_data_service import OperatorDataService
from app.services.cache.cache_aside import CacheAsideService

logger = logging.getLogger(__name__)

class OperatorAuthService:
    """
    OperatorAuthService for g8ee.
    Handles operator authentication logic, moving it from g8ed.
    """

    def __init__(
        self,
        api_key_service: ApiKeyService,
        session_service: OperatorSessionService,
        operator_data_service: OperatorDataService,
        certificate_service: CertificateService,
        cache_aside: CacheAsideService,
    ):
        self._api_key_service = api_key_service
        self._session_service = session_service
        self._operator_data_service = operator_data_service
        self._certificate_service = certificate_service
        self._cache = cache_aside

    @property
    def api_key_service(self) -> ApiKeyService:
        return self._api_key_service

    @property
    def session_service(self) -> OperatorSessionService:
        return self._session_service

    @property
    def operator_data_service(self) -> OperatorDataService:
        return self._operator_data_service

    @property
    def certificate_service(self) -> CertificateService:
        return self._certificate_service

    @property
    def cache(self) -> CacheAsideService:
        return self._cache

    async def authenticate_operator(
        self,
        authorization_header: str | None,
        body: dict[str, Any],
        request_context: dict[str, Any] | None
    ) -> dict[str, Any]:
        """Authenticate an operator process."""
        auth_mode = body.get("auth_mode")
        device_link_session_id = body.get("operator_session_id")

        if auth_mode == "operator_session" and device_link_session_id:
            return await self._authenticate_via_device_link(device_link_session_id, body.get("system_info"), authorization_header, request_context)

        return await self._authenticate_via_api_key(authorization_header, body, request_context)

    async def _authenticate_via_api_key(
        self,
        authorization_header: str | None,
        body: dict[str, Any],
        request_context: dict[str, Any]
    ) -> dict[str, Any]:
        """Authenticate using an API key."""
        api_key = None
        if authorization_header and authorization_header.startswith("Bearer "):
            api_key = authorization_header[len("Bearer "):]

        if not api_key:
            return {"success": False, "status_code": 401, "error": "Missing API key"}

        success, key_doc, error = await self.api_key_service.validate_key(api_key)
        if not success:
            return {"success": False, "status_code": 401, "error": error or "Invalid API key"}

        await self.api_key_service.record_usage(api_key)

        user_id = key_doc.user_id
        operator_id = key_doc.operator_id

        if not operator_id:
            # Handle CLI auth or other cases if needed
            return {"success": False, "status_code": 401, "error": "API key not tied to an operator"}

        # Get operator doc
        operator = await self.operator_data_service.get_operator(operator_id)
        if not operator:
            return {"success": False, "status_code": 404, "error": "Operator not found"}

        if operator.user_id != user_id:
            return {"success": False, "status_code": 403, "error": "Unauthorized"}

        # Get user doc for user_data
        user_data = await self.cache.get_document_with_cache(DB_COLLECTION_USERS, user_id)

        # Create session
        session_data = {
            "user_id": user_id,
            "organization_id": key_doc.organization_id,
            "user_data": user_data,
            "api_key": api_key,
            "operator_id": operator_id,
            "operator_status": operator.status,
        }
        
        session = await self.session_service.create_operator_session(session_data, request_context)

        # Claim slot
        await self.operator_data_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id=session.id,
            bound_web_session_id=operator.bound_web_session_id,
            system_info=body.get("system_info", {}),
            operator_type=operator.operator_type,
        )

        # Generate per-operator certificate (Authority: g8ee)
        certs = await self.certificate_service.generate_operator_certificate(
            operator_id=operator_id,
            user_id=user_id,
            organization_id=key_doc.organization_id or user_id
        )

        # Add history entry (Authority: g8ee)
        try:
            await self.operator_data_service.add_history_entry(
                operator_id=operator_id,
                event_type=OperatorHistoryEventType.AUTHENTICATED,
                summary="Operator authenticated via API key",
                actor=ComponentName.G8EE,
                details={
                    "operator_session_id": session.id,
                    "user_id": user_id,
                    "hostname": body.get("system_info", {}).get("hostname"),
                }
            )
        except Exception as e:
            logger.warning(f"[OPERATOR-AUTH] Failed to add history entry: {e}")

        return {
            "success": True,
            "operator_session_id": session.id,
            "operator_id": operator_id,
            "user_id": user_id,
            "api_key": api_key,
            "operator_cert": certs["cert"],
            "operator_cert_key": certs["key"],
            "session": {
                "id": session.id,
                "expires_at": session.absolute_expires_at,
                "created_at": session.created_at,
            }
        }

    async def _authenticate_via_device_link(
        self,
        device_link_session_id: str,
        system_info: dict | None,
        authorization_header: str | None,
        request_context: dict[str, Any]
    ) -> dict[str, Any]:
        """Authenticate using a device link session ID."""
        session = await self.session_service.validate_session(device_link_session_id)
        if not session:
            return {"success": False, "status_code": 401, "error": "Invalid or expired session"}

        user_id = session.user_id
        operator_id = session.operator_id

        # Similar to _authenticate_via_api_key but reusing existing session ID
        # (This usually happens during bootstrap after device registration)
        
        # Get user data if not in session
        user_data = session.user_data or await self.cache.get_document_with_cache(DB_COLLECTION_USERS, user_id)

        # In device link flow, the session is already created by g8ed.
        # g8ee might just need to update it or return it.
        
        # Generate per-operator certificate (Authority: g8ee)
        certs = await self.certificate_service.generate_operator_certificate(
            operator_id=operator_id,
            user_id=user_id,
            organization_id=session.organization_id or user_id
        )

        # Add history entry (Authority: g8ee)
        try:
            await self.operator_data_service.add_history_entry(
                operator_id=operator_id,
                event_type=OperatorHistoryEventType.RECONNECTED,
                summary="Operator re-authenticated via device link",
                actor=ComponentName.G8EE,
                details={
                    "operator_session_id": session.id,
                    "user_id": user_id,
                    "hostname": system_info.get("hostname") if system_info else None,
                }
            )
        except Exception as e:
            logger.warning(f"[OPERATOR-AUTH] Failed to add history entry for device link: {e}")

        return {
            "success": True,
            "operator_session_id": session.id,
            "operator_id": operator_id,
            "user_id": user_id,
            "api_key": session.api_key, # Decrypted in a real impl
            "operator_cert": certs["cert"],
            "operator_cert_key": certs["key"],
            "session": {
                "id": session.id,
                "expires_at": session.absolute_expires_at,
                "created_at": session.created_at,
            }
        }
