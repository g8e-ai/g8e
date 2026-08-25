# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed fake for MemoryDataServiceProtocol."""

from app.models.http_context import RequestContext
from app.models.investigations import InvestigationModel
from app.models.memory import InvestigationMemory
from app.services.protocols import MemoryDataServiceProtocol


class FakeMemoryDataService:
    """Typed fake implementing MemoryDataServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self) -> None:
        self.create_calls: list[InvestigationModel] = []
        self.save_calls: list[tuple[InvestigationMemory, bool]] = []
        self.get_calls: list[str] = []
        self.get_user_memories_calls: list[str] = []
        self.get_case_memories_calls: list[tuple[str, str]] = []
        self._memory_to_return: InvestigationMemory | None = None
        self._user_memories_to_return: list[InvestigationMemory] = []
        self._case_memories_to_return: list[InvestigationMemory] = []

    def set_memory_to_return(self, memory: InvestigationMemory | None) -> None:
        self._memory_to_return = memory

    def set_user_memories_to_return(self, memories: list[InvestigationMemory]) -> None:
        self._user_memories_to_return = memories

    def set_case_memories_to_return(self, memories: list[InvestigationMemory]) -> None:
        self._case_memories_to_return = memories

    async def create_memory(
        self, investigation: InvestigationModel, context: RequestContext | None = None
    ) -> InvestigationMemory:
        self.create_calls.append(investigation)
        return InvestigationMemory(
            case_id=investigation.case_id,
            investigation_id=investigation.id,
            user_id=investigation.user_id,
            status=investigation.status,
            case_title=investigation.case_title,
        )

    async def save_memory(
        self, memory: InvestigationMemory, is_new: bool, context: RequestContext | None = None
    ) -> None:
        self.save_calls.append((memory, is_new))

    async def get_memory(self, investigation_id: str) -> InvestigationMemory | None:
        self.get_calls.append(investigation_id)
        return self._memory_to_return

    async def get_user_memories(self, user_id: str) -> list[InvestigationMemory]:
        self.get_user_memories_calls.append(user_id)
        return self._user_memories_to_return

    async def get_case_memories(self, case_id: str, user_id: str) -> list[InvestigationMemory]:
        self.get_case_memories_calls.append((case_id, user_id))
        return self._case_memories_to_return


_: MemoryDataServiceProtocol = FakeMemoryDataService()
