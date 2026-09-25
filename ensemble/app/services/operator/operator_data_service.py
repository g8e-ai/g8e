# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import logging

from app.clients.gateway_operator_client import GatewayOperatorClient
from app.constants.collections import DB_COLLECTION_CLI_SESSIONS, DB_COLLECTION_OPERATORS
from app.errors import ValidationError
from app.models.sessions import CliSessionDocument
from app.models.operators import OperatorDocument
from app.services.cache.cache_aside import CacheAsideService
from app.services.protocols import OperatorDataServiceProtocol
from app.utils.gateway_operator_document import operator_document_from_gateway

logger = logging.getLogger(__name__)


class OperatorDataService(OperatorDataServiceProtocol):
    """Gateway-backed operator reads and local CLI session cache access."""

    def __init__(
        self,
        cache: CacheAsideService,
        gateway_operator_client: GatewayOperatorClient,
    ) -> None:
        self.cache = cache
        self._gateway_operator_client = gateway_operator_client
        self.collection = DB_COLLECTION_OPERATORS

    async def get_operator(
        self, operator_id: str, *, user_id: str | None = None
    ) -> OperatorDocument | None:
        """Get an operator document from the Gateway operator registry."""
        if not operator_id:
            raise ValidationError("operator_id is required")
        if not user_id:
            raise ValidationError("user_id is required for Gateway operator reads")

        operators = await self._gateway_operator_client.list(user_id=user_id)
        for operator_doc in operators:
            if not isinstance(operator_doc, dict):
                continue
            doc_id = operator_doc.get("id") or operator_doc.get("operator_id")
            if str(doc_id) == operator_id:
                return operator_document_from_gateway(operator_doc)
        return None

    async def get_operator_by_session(self, session_id: str) -> OperatorDocument | None:
        """Get an operator document by active operator session ID."""
        if not session_id:
            raise ValidationError("session_id is required")

        operator_doc = await self._gateway_operator_client.get_by_session(session_id=session_id)
        if not operator_doc:
            return None
        if not isinstance(operator_doc, dict):
            return None
        return operator_document_from_gateway(operator_doc)

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
        """Verify that the given cli_session_id is owned by the given operator_session_id."""
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
        *,
        user_id: str,
    ) -> list[OperatorDocument]:
        """List operator documents from the Gateway registry for a user."""
        if not user_id:
            raise ValidationError("user_id is required for Gateway operator reads")

        operators = await self._gateway_operator_client.list(user_id=user_id)
        docs = [
            operator_document_from_gateway(operator_doc)
            for operator_doc in operators
            if isinstance(operator_doc, dict)
        ]

        if field_filters:
            docs = [doc for doc in docs if self._matches_filters(doc, field_filters)]

        return docs[:limit]

    @staticmethod
    def _matches_filters(
        doc: OperatorDocument, field_filters: list[dict[str, object]]
    ) -> bool:
        payload = doc.model_dump(mode="json")
        for field_filter in field_filters:
            field = field_filter.get("field")
            if not isinstance(field, str):
                continue
            expected = field_filter.get("value")
            if payload.get(field) != expected:
                return False
        return True
