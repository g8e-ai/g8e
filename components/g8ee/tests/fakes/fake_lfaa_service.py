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

"""Typed fake for LFAAServiceProtocol."""

from app.services.protocols import LFAAServiceProtocol


class FakeLFAAService:
    """Typed fake implementing LFAAServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self, *, return_value: bool = True) -> None:
        self._return_value = return_value
        self.audit_events: list[dict] = []

    async def send_direct_exec_audit_event(
        self,
        command: str,
        execution_id: str,
        operator_id: str,
        operator_session_id: str,
        web_session_id: str,
        case_id: str,
        investigation_id: str,
    ) -> bool:
        self.audit_events.append({
            "command": command,
            "execution_id": execution_id,
            "operator_id": operator_id,
            "operator_session_id": operator_session_id,
            "web_session_id": web_session_id,
            "case_id": case_id,
            "investigation_id": investigation_id,
        })
        return self._return_value


_: LFAAServiceProtocol = FakeLFAAService()
