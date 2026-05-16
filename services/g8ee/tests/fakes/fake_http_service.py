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

"""Typed fake for HTTPServiceProtocol."""

from __future__ import annotations

from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from app.clients.http_client import HTTPClient
from app.services.protocols import HTTPServiceProtocol


class FakeHTTPService:
    """Typed fake implementing HTTPServiceProtocol.

    Records all calls for assertion in tests. Does not perform any real I/O.
    Provides mock HTTP clients that can be configured for testing.
    """

    def __init__(self) -> None:
        self._ready = False
        self.started = False
        self.stopped = False
        self.registered_clients: dict[str, HTTPClient] = {}
        self.deregistered_clients: list[str] = []
        self._http_clients: dict[str, HTTPClient] = {}
        self.set_client_calls: list[tuple[HTTPClient, str]] = []

    @property
    def is_ready(self) -> bool:
        return self._ready

    def set_http_client(self, client: HTTPClient, service_name: str) -> None:
        self.set_client_calls.append((client, service_name))
        self._http_clients[service_name] = client

    def get_client(self, service_name: str) -> HTTPClient | None:
        return self._http_clients.get(service_name)

    async def start(self) -> None:
        self._ready = True
        self.started = True

    async def stop(self) -> None:
        self._ready = False
        self.stopped = True
        # Clear all clients on stop
        self._http_clients.clear()

    async def register_service_client(self, service_name: str, client: HTTPClient) -> None:
        self.registered_clients[service_name] = client
        self._http_clients[service_name] = client

    async def deregister_service_client(self, service_name: str) -> None:
        self.deregistered_clients.append(service_name)
        self._http_clients.pop(service_name, None)
        self.registered_clients.pop(service_name, None)

    def list_active_clients(self) -> list[str]:
        return list(self._http_clients.keys())

    def _install_client_capture(self) -> dict[str, HTTPClient]:
        """Capture all registered clients for verification in tests."""
        return self._http_clients.copy()

    def get_client_status(self) -> dict[str, dict[str, object]]:
        """Mock implementation of client status for tests."""
        status = {}
        for service_name in self._http_clients:
            status[service_name] = {
                "service_name": service_name,
                "base_url": "mock-url",
                "is_session_closed": False,
                "circuit_breakers": 0,
            }
        return status


_: HTTPServiceProtocol = FakeHTTPService()
