# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed fake for LFAAServiceProtocol."""

from app.services.protocols import LFAAServiceProtocol


class FakeLFAAService:
    """Typed fake implementing LFAAServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    """

    def __init__(self, *, return_value: bool = True) -> None:
        self._return_value = return_value
        self.audit_events: list[dict] = []

    async def send_audit_event(self, g8e_message) -> bool:
        return self._return_value

    async def send_direct_exec_audit_event(
        self,
        command: str,
        execution_id: str,
        g8e_context,
    ) -> bool:
        self.audit_events.append(
            {
                "command": command,
                "execution_id": execution_id,
                "g8e_context": g8e_context,
            }
        )
        return self._return_value


_: LFAAServiceProtocol = FakeLFAAService()
