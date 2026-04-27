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

from app.constants import DB_COLLECTION_USERS, OperatorStatus
from app.services.auth.api_key_service import ApiKeyService
from app.services.auth.certificate_service import CertificateService
from app.services.operator.operator_session_service import OperatorSessionService
from app.services.operator.operator_data_service import OperatorDataService
from app.services.protocols import OperatorLifecycleServiceProtocol
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
        lifecycle_service: OperatorLifecycleServiceProtocol,
        certificate_service: CertificateService,
        cache_aside: CacheAsideService,
    ):
        self._api_key_service = api_key_service
        self._session_service = session_service
        self._operator_data_service = operator_data_service
        self._lifecycle_service = lifecycle_service
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
    def lifecycle_service(self) -> OperatorLifecycleServiceProtocol:
        return self._lifecycle_service

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
        request_context: dict[str, Any] | None,
    ) -> dict[str, Any]:
        """Authenticate operator process via api key (Bearer)."""
        return await self._authenticate_via_api_key(authorization_header, body, request_context)

    async def register_device_link_operator(
        self,
        operator_id: str,
        user_id: str,
        organization_id: str | None,
        operator_type: str,
        system_info: dict,
        request_context: dict[str, Any] | None,
    ) -> dict[str, Any]:
        """Bootstrap an operator after device-link consumption.

        Trust model: caller is g8ed via internal mTLS. No authorization header.
        Inputs are taken at face value (operator_id, user_id come from a verified
        device-link token on g8ed's side).
        """
        # 1. Get operator
        operator = await self._operator_data_service.get_operator(operator_id)
        if not operator:
            return {"success": False, "error": "Operator not found"}

        # 2. Ownership check
        if operator.user_id != user_id:
            return {"success": False, "error": "Operator ownership mismatch"}

        # 3. Resolve api_key from operator document (device-link path)
        api_key = operator.api_key
        if not api_key:
            logger.error(f"[OPERATOR-AUTH] Operator {operator_id} has no api_key on slot - slot creation bug")
            return {"success": False, "error": "Operator slot missing api_key - configuration error"}

        # 4. Create session
        user_data = await self._cache.get_document_with_cache(DB_COLLECTION_USERS, user_id)
        session_data = {
            "user_id": user_id,
            "organization_id": organization_id,
            "user_data": user_data,
            "api_key": api_key,
            "operator_id": operator_id,
            "operator_status": operator.status,
        }
        session = await self._session_service.create_operator_session(session_data, request_context)

        # 5. Claim slot
        claim_success = await self._lifecycle_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id=session.id,
            bound_web_session_id=operator.bound_web_session_id,
            system_info=system_info,
            operator_type=operator_type,
        )
        if not claim_success:
            return {"success": False, "error": "Failed to claim operator slot"}

        # 6. Generate certificate
        certs = await self._certificate_service.generate_operator_certificate(
            operator_id=operator_id,
            user_id=user_id,
            organization_id=organization_id or user_id,
        )

        # 7. Return response
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
            },
        }

    async def _authenticate_via_api_key(
        self,
        authorization_header: str | None,
        body: dict[str, Any],
        request_context: dict[str, Any] | None,
    ) -> dict[str, Any]:
        """Authenticate using an API key (Bearer)."""
        # Extract api_key from Bearer header
        api_key = None
        if authorization_header and authorization_header.startswith("Bearer "):
            api_key = authorization_header[len("Bearer "):]

        if not api_key:
            return {"success": False, "error": "Missing API key"}

        # Validate api_key
        success, key_doc, error = await self._api_key_service.validate_key(api_key)
        if not success:
            return {"success": False, "error": error or "Invalid API key"}

        await self._api_key_service.record_usage(api_key)

        user_id = key_doc.user_id
        operator_id = key_doc.operator_id

        if not operator_id:
            return {"success": False, "error": "API key not tied to an operator"}

        # 1. Get operator
        operator = await self._operator_data_service.get_operator(operator_id)
        if not operator:
            return {"success": False, "error": "Operator not found"}

        # 2. Ownership check
        if operator.user_id != user_id:
            return {"success": False, "error": "Unauthorized"}

        # 3. Resolve api_key (already validated from bearer token)
        # api_key is the validated bearer token itself

        # 4. Create session
        user_data = await self._cache.get_document_with_cache(DB_COLLECTION_USERS, user_id)
        session_data = {
            "user_id": user_id,
            "organization_id": key_doc.organization_id,
            "user_data": user_data,
            "api_key": api_key,
            "operator_id": operator_id,
            "operator_status": operator.status,
        }
        session = await self._session_service.create_operator_session(session_data, request_context)

        # 5. Claim slot
        claim_success = await self._lifecycle_service.claim_operator_slot(
            operator_id=operator_id,
            operator_session_id=session.id,
            bound_web_session_id=operator.bound_web_session_id,
            system_info=body.get("system_info", {}),
            operator_type=operator.operator_type,
        )
        if not claim_success:
            return {"success": False, "error": "Failed to claim operator slot"}

        # 6. Generate certificate
        certs = await self._certificate_service.generate_operator_certificate(
            operator_id=operator_id,
            user_id=user_id,
            organization_id=key_doc.organization_id or user_id,
        )

        # 7. Return response
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
            },
        }
