# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed fake for OperatorDataServiceProtocol."""

from __future__ import annotations

from unittest.mock import MagicMock

from app.db.document_service import DocumentService
from app.models.cache import CacheOperationResult
from app.models.operators import OperatorDocument
from app.models.sessions import CliSessionDocument
from app.services.protocols import OperatorDataServiceProtocol


class FakeOperatorCache:
    """Typed fake implementing OperatorDataServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    collection: str = "operators"
    cache: DocumentService

    def __init__(self) -> None:
        self.cache = MagicMock(spec=DocumentService)

    async def get_operator(
        self, operator_id: str, *, user_id: str | None = None
    ) -> OperatorDocument | None:
        return None

    async def get_operator_by_session(self, session_id: str) -> OperatorDocument | None:
        return None

    async def get_cli_session(self, cli_session_id: str) -> CliSessionDocument | None:
        return None

    async def validate_cli_session_ownership(
        self, cli_session_id: str, operator_session_id: str
    ) -> bool:
        return True

    async def query_operators(
        self,
        field_filters: list[dict[str, object]] | None = None,
        limit: int = 1000,
        *,
        user_id: str,
    ) -> list[OperatorDocument]:
        return []

    async def update_document(
        self, collection: str, document_id: str, data: dict[str, object], merge: bool = True
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True, document_id=document_id)


_: OperatorDataServiceProtocol = FakeOperatorCache()
