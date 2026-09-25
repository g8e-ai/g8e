# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from app.clients.http_client import HTTPClient
    from app.services.cache.cache_aside import CacheAsideService
from app.constants.collections import (
    DB_COLLECTION_OPERATORS,
    DB_COLLECTION_CLI_SESSIONS,
)
from app.errors import ValidationError
from app.models.sessions import CliSessionDocument
from app.models.operators import OperatorDocument
from app.services.cache.cache_aside import CacheAsideService
from app.services.protocols import OperatorDataServiceProtocol
from app.clients.http_client import HTTPClient

logger = logging.getLogger(__name__)


class OperatorDataService(OperatorDataServiceProtocol):
    """Read-oriented operator document access for application orchestration."""

    def __init__(self, cache: CacheAsideService, internal_http_client: HTTPClient):
        self.cache = cache
        self.internal_http_client = internal_http_client
        self.collection = DB_COLLECTION_OPERATORS

    async def get_operator(self, operator_id: str) -> OperatorDocument | None:
        """Get Operator document using cache-aside pattern."""
        if not operator_id:
            raise ValidationError("operator_id is required")

        data = await self.cache.get_document_with_cache(self.collection, operator_id)
        if not data:
            return None

        return OperatorDocument.model_validate(data)

    async def get_cli_session(self, cli_session_id: str) -> CliSessionDocument | None:
        """Get CLI session document using cache-aside pattern."""
        if not cli_session_id:
            raise ValidationError("cli_session_id is required")

        data = await self.cache.get_document_with_cache(DB_COLLECTION_CLI_SESSIONS, cli_session_id)
        if not data:
            return None

        return CliSessionDocument.model_validate(data)

    async def validate_cli_session_ownership(
        self, cli_session_id: str, operator_session_id: str
    ) -> bool:
        """Verify that the given cli_session_id is owned by the given operator_session_id.

        This prevents a malicious client with a valid operator session from draining
        or publishing to someone else's CLI session.
        """
        if not cli_session_id or not operator_session_id:
            return False

        session = await self.get_cli_session(cli_session_id)
        if not session:
            logger.warning("[OPERATOR-DATA-SERVICE] CLI session not found: %s", cli_session_id)
            return False

        if session.operator_session_id != operator_session_id:
            logger.warning(
                "[OPERATOR-DATA-SERVICE] CLI session ownership mismatch: cli=%s, expected_op=%s, actual_op=%s",
                cli_session_id,
                operator_session_id,
                session.operator_session_id,
            )
            return False

        return True

    async def query_operators(
        self,
        field_filters: list[dict[str, object]] | None = None,
        limit: int = 1000,
        bypass_cache: bool = False,
    ) -> list[OperatorDocument]:
        """Query Operator documents.

        ``bypass_cache=True`` mirrors client's ``queryOperatorsFresh`` and is used
        by Gateway-owned reconcilers where stale query
        cache results would produce false STALE/OFFLINE transitions.
        """
        rows = await self.cache.query_documents(
            collection=self.collection,
            field_filters=field_filters or [],
            limit=limit,
            bypass_cache=bypass_cache,
        )
        return [OperatorDocument.model_validate(row) for row in rows]
