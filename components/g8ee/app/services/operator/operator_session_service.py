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
from typing import Any, Optional
import uuid

from app.constants import (
    DB_COLLECTION_OPERATOR_SESSIONS,
    SessionType,
    SessionEndReason,
)
from app.models.sessions import (
    OperatorSessionDocument,
)
from app.services.cache.cache_aside import CacheAsideService
from app.utils.timestamp import now, add_seconds, seconds_between

logger = logging.getLogger(__name__)

# Default timeouts in seconds
DEFAULT_SESSION_TTL = 3600 * 24 # 1 hour idle
DEFAULT_ABSOLUTE_TIMEOUT = 3600 * 24 * 7 # 7 days absolute

class OperatorSessionService:
    """
    OperatorSessionService for g8ee.
    Manages operator daemon sessions in the database and KV cache.
    """

    def __init__(self, cache_aside: CacheAsideService):
        self.cache = cache_aside
        self.collection = DB_COLLECTION_OPERATOR_SESSIONS
        self.session_ttl = DEFAULT_SESSION_TTL
        self.absolute_timeout = DEFAULT_ABSOLUTE_TIMEOUT

    def _generate_session_id(self) -> str:
        return f"ops_{uuid.uuid4().hex}"

    async def create_operator_session(
        self,
        session_data: dict[str, Any],
        request_context: dict[str, Any] = None,
        ttl_seconds: Optional[int] = None
    ) -> OperatorSessionDocument:
        """Create a new operator session."""
        session_id = self._generate_session_id()
        ts = now()
        
        ttl = ttl_seconds if ttl_seconds is not None else self.session_ttl
        absolute_ttl = ttl_seconds if ttl_seconds is not None else self.absolute_timeout
        
        absolute_expires_at = add_seconds(ts, absolute_ttl)
        idle_expires_at = add_seconds(ts, ttl)

        ctx = request_context or {}
        
        session = OperatorSessionDocument(
            id=session_id,
            session_type=SessionType.OPERATOR,
            user_id=session_data.get("user_id"),
            organization_id=session_data.get("organization_id"),
            user_data=session_data.get("user_data"),
            api_key=session_data.get("api_key"), # Should be encrypted in a real impl
            operator_id=session_data.get("operator_id"),
            client_ip=ctx.get("ip"),
            user_agent=ctx.get("user_agent"),
            login_method=ctx.get("login_method", "api_key"),
            created_at=ts,
            absolute_expires_at=absolute_expires_at,
            idle_expires_at=idle_expires_at,
            last_activity=ts,
            last_ip=ctx.get("ip"),
            is_active=True,
            operator_status=session_data.get("operator_status"),
            metadata=session_data.get("metadata"),
        )

        result = await self.cache.create_document(
            collection=self.collection,
            document_id=session_id,
            data=session
        )
        
        if not result.success:
            raise Exception(f"Failed to persist operator session: {result.error}")

        logger.info(f"[OPERATOR-SESSION-SERVICE] Operator session created: {session_id}")
        return session

    async def validate_session(self, session_id: str) -> Optional[OperatorSessionDocument]:
        """Validate an operator session and check for expiry."""
        if not session_id:
            return None

        data = await self.cache.get_document_with_cache(self.collection, session_id)
        if not data:
            return None

        session = OperatorSessionDocument.model_validate(data)
        
        if not session.is_active:
            return None

        check_time = now()
        
        if session.absolute_expires_at and check_time > session.absolute_expires_at:
            logger.warning(f"[OPERATOR-SESSION-SERVICE] Session {session_id} absolute timeout")
            await self.end_session(session_id, reason=SessionEndReason.TIMEOUT_ABSOLUTE)
            return None
            
        if session.idle_expires_at and check_time > session.idle_expires_at:
            logger.warning(f"[OPERATOR-SESSION-SERVICE] Session {session_id} idle timeout")
            await self.end_session(session_id, reason=SessionEndReason.TIMEOUT_IDLE)
            return None

        return session

    async def refresh_session(self, session_id: str, session: Optional[OperatorSessionDocument] = None) -> bool:
        """Refresh session idle timeout."""
        if not session:
            session = await self.validate_session(session_id)
            if not session:
                return False

        ts = now()
        new_idle_expiry = add_seconds(ts, self.session_ttl)
        
        updates = {
            "last_activity": ts,
            "idle_expires_at": new_idle_expiry
        }
        
        result = await self.cache.update_document(
            collection=self.collection,
            document_id=session_id,
            data=updates,
            merge=True
        )
        return result.success

    async def end_session(self, session_id: str, reason: str = SessionEndReason.LOGOUT) -> bool:
        """End an operator session."""
        result = await self.cache.delete_document(
            collection=self.collection,
            document_id=session_id
        )
        
        if result.success:
            logger.info(f"[OPERATOR-SESSION-SERVICE] Operator session ended: {session_id} (reason: {reason})")
            
        return result.success
