# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

"""Typed fake for OperatorDataServiceProtocol."""

from app.constants import HistoryActor, OperatorHistoryEventType, OperatorStatus
from app.models.cache import CacheOperationResult
from app.services.cache.cache_aside import CacheAsideService
from app.services.protocols import OperatorDataServiceProtocol


class FakeOperatorCache:
    """Typed fake implementing OperatorDataServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self) -> None:
        self.status_updates: list[dict] = []

    async def update_operator_status(self, operator_id: str, status: OperatorStatus) -> bool:
        self.status_updates.append({"operator_id": operator_id, "status": status})
        return True

    collection: str = "operators"
    cache: CacheAsideService = None

    async def get_operator(self, operator_id: str):
        return None

    async def get_cli_session(self, cli_session_id: str):
        return None

    async def validate_cli_session_ownership(
        self, cli_session_id: str, operator_session_id: str
    ) -> bool:
        return True

    async def query_operators(self, field_filters=None, limit=1000, bypass_cache=False):
        return []

    async def create_operator(self, operator) -> bool:
        return True

    async def update_operator(self, operator) -> bool:
        return True

    async def add_history_entry(
        self,
        operator_id: str,
        event_type: OperatorHistoryEventType,
        actor: HistoryActor,
        summary: str,
        details: dict[str, object] | None = None,
        additional_updates: dict[str, object] | None = None,
        status_check: tuple[OperatorStatus, ...] | None = None,
    ):
        return None

    async def update_document(
        self, collection: str, document_id: str, data: dict, merge: bool = True
    ) -> CacheOperationResult:
        return CacheOperationResult(success=True, document_id=document_id)

    async def update_operator_heartbeat(self, operator_id, heartbeat, investigation_id, case_id):
        return True

    async def append_command_result(self, operator_id, command_result):
        return True

    async def add_operator_activity(self, operator_id, sender, content, metadata):
        return True

    async def add_operator_approval(self, operator_id, event_type, metadata):
        return True


_: OperatorDataServiceProtocol = FakeOperatorCache()
