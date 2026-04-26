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

"""Typed fake for OperatorDataServiceProtocol."""

from app.constants import OperatorStatus
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

    async def get_operator(self, operator_id: str):
        return None

    async def query_operators(self, field_filters=None, limit=1000, bypass_cache=False):
        return []

    async def update_operator_heartbeat(self, operator_id, heartbeat, investigation_id, case_id):
        return True

    async def append_command_result(self, operator_id, command_result):
        return True

    async def add_operator_activity(self, operator_id, sender, content, metadata):
        return True

    async def add_operator_approval(self, operator_id, event_type, metadata):
        return True

    async def bind_operators(self, operator_ids, web_session_id, context):
        return True


_: OperatorDataServiceProtocol = FakeOperatorCache()
