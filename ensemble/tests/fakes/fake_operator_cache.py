# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

"""Typed fake for OperatorDataServiceProtocol."""

from app.models.cache import CacheOperationResult
from app.services.cache.cache_aside import CacheAsideService
from app.services.protocols import OperatorDataServiceProtocol


class FakeOperatorCache:
    """Typed fake implementing OperatorDataServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    collection: str = "operators"
    cache: CacheAsideService = None

    async def get_operator(self, operator_id: str, *, user_id: str | None = None):
        return None

    async def get_operator_by_session(self, session_id: str):
        return None

    async def get_cli_session(self, cli_session_id: str):
        return None

    async def validate_cli_session_ownership(
        self, cli_session_id: str, operator_session_id: str
    ) -> bool:
        return True

    async def query_operators(
        self,
        field_filters=None,
        limit=1000,
        bypass_cache=False,
        *,
        user_id: str,
    ):
        return []

    async def update_document(
        self, collection: str, document_id: str, data: dict, merge: bool = True
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True, document_id=document_id)


_: OperatorDataServiceProtocol = FakeOperatorCache()
