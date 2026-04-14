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

"""Tests for HTTPService."""

import pytest
from unittest.mock import AsyncMock, MagicMock

from app.clients.http_client import HTTPClient
from app.models.infra import HTTPClientStatus
from app.errors import ValidationError
from app.services.infra.http_service import HTTPService
from app.services.protocols import HTTPServiceProtocol


class TestHTTPService:
    """Test suite for HTTPService implementation."""

    def setup_method(self):
        """Set up test fixtures."""
        self.http_service = HTTPService()
        self.mock_client = MagicMock(spec=HTTPClient)
        self.service_name = "test_service"

    def test_initialization(self):
        """Test HTTPService initializes with correct default state."""
        assert not self.http_service.is_ready
        assert self.http_service.list_active_clients() == []
        assert self.http_service.get_client("nonexistent") is None

    def test_set_http_client_success(self):
        """Test successful HTTP client registration."""
        self.http_service.set_http_client(self.mock_client, self.service_name)
        
        assert self.http_service.get_client(self.service_name) is self.mock_client
        assert self.service_name in self.http_service.list_active_clients()

    def test_set_http_client_validation_errors(self):
        """Test validation errors when setting HTTP client."""
        with pytest.raises(ValidationError, match="HTTP client is required"):
            self.http_service.set_http_client(None, self.service_name)
        
        with pytest.raises(ValidationError, match="service_name is required"):
            self.http_service.set_http_client(self.mock_client, "")

    def test_set_http_client_overwrites_existing(self):
        """Test that setting a client overwrites existing one."""
        original_client = MagicMock(spec=HTTPClient)
        new_client = MagicMock(spec=HTTPClient)
        
        self.http_service.set_http_client(original_client, self.service_name)
        assert self.http_service.get_client(self.service_name) is original_client
        
        self.http_service.set_http_client(new_client, self.service_name)
        assert self.http_service.get_client(self.service_name) is new_client

    @pytest.mark.asyncio
    async def test_start_service(self):
        """Test starting the HTTP service."""
        # Start with no clients
        await self.http_service.start()
        assert self.http_service.is_ready
        
        # Reset and start with clients
        self.http_service._http_ready = False
        self.http_service.set_http_client(self.mock_client, self.service_name)
        await self.http_service.start()
        assert self.http_service.is_ready

    @pytest.mark.asyncio
    async def test_start_already_ready(self):
        """Test starting an already ready service."""
        self.http_service._http_ready = True
        await self.http_service.start()
        assert self.http_service.is_ready

    @pytest.mark.asyncio
    async def test_stop_service(self):
        """Test stopping the HTTP service."""
        # Set up a client with close method
        mock_close = AsyncMock()
        self.mock_client.close = mock_close
        
        self.http_service.set_http_client(self.mock_client, self.service_name)
        self.http_service._http_ready = True
        
        await self.http_service.stop()
        
        assert not self.http_service.is_ready
        assert self.http_service.list_active_clients() == []
        mock_close.assert_called_once()

    @pytest.mark.asyncio
    async def test_stop_already_stopped(self):
        """Test stopping an already stopped service."""
        await self.http_service.stop()
        assert not self.http_service.is_ready

    @pytest.mark.asyncio
    async def test_stop_handles_close_errors(self):
        """Test that stop handles client close errors gracefully."""
        # Set up a client that raises an error on close
        mock_close = AsyncMock(side_effect=Exception("Close error"))
        self.mock_client.close = mock_close
        
        self.http_service.set_http_client(self.mock_client, self.service_name)
        self.http_service._http_ready = True
        
        # Should not raise an exception
        await self.http_service.stop()
        
        assert not self.http_service.is_ready
        assert self.http_service.list_active_clients() == []

    @pytest.mark.asyncio
    async def test_register_service_client(self):
        """Test registering a new service client."""
        await self.http_service.register_service_client(self.service_name, self.mock_client)
        
        assert self.http_service.get_client(self.service_name) is self.mock_client
        assert self.service_name in self.http_service.list_active_clients()

    @pytest.mark.asyncio
    async def test_register_service_client_overwrites(self):
        """Test that register_service_client overwrites existing clients."""
        original_client = MagicMock(spec=HTTPClient)
        original_client.close = AsyncMock()
        
        # Register first client
        await self.http_service.register_service_client(self.service_name, original_client)
        assert self.http_service.get_client(self.service_name) is original_client
        
        # Register second client (should overwrite)
        await self.http_service.register_service_client(self.service_name, self.mock_client)
        assert self.http_service.get_client(self.service_name) is self.mock_client
        original_client.close.assert_called_once()

    @pytest.mark.asyncio
    async def test_deregister_service_client(self):
        """Test deregistering a service client."""
        # Set up client with close method
        mock_close = AsyncMock()
        self.mock_client.close = mock_close
        
        # Register then deregister
        await self.http_service.register_service_client(self.service_name, self.mock_client)
        assert self.service_name in self.http_service.list_active_clients()
        
        await self.http_service.deregister_service_client(self.service_name)
        
        assert self.service_name not in self.http_service.list_active_clients()
        assert self.http_service.get_client(self.service_name) is None
        mock_close.assert_called_once()

    @pytest.mark.asyncio
    async def test_deregister_nonexistent_client(self):
        """Test deregistering a non-existent client."""
        # Should not raise an exception
        await self.http_service.deregister_service_client("nonexistent")
        
        assert self.http_service.list_active_clients() == []

    @pytest.mark.asyncio
    async def test_deregister_handles_close_errors(self):
        """Test that deregister handles client close errors gracefully."""
        # Set up client that raises error on close
        mock_close = AsyncMock(side_effect=Exception("Close error"))
        self.mock_client.close = mock_close
        
        await self.http_service.register_service_client(self.service_name, self.mock_client)
        
        # Should not raise an exception
        await self.http_service.deregister_service_client(self.service_name)
        
        assert self.service_name not in self.http_service.list_active_clients()

    def test_list_active_clients(self):
        """Test listing active clients."""
        assert self.http_service.list_active_clients() == []
        
        self.http_service.set_http_client(self.mock_client, "service1")
        self.http_service.set_http_client(MagicMock(spec=HTTPClient), "service2")
        
        clients = self.http_service.list_active_clients()
        assert len(clients) == 2
        assert "service1" in clients
        assert "service2" in clients

    def test_get_client_status(self):
        """Test getting client status information."""
        # Set up mock client with expected attributes
        self.mock_client.base_url = "http://test.example.com"
        self.mock_client.is_session_closed = False
        self.mock_client.circuit_breakers = {}
        
        self.http_service.set_http_client(self.mock_client, self.service_name)
        
        status = self.http_service.get_client_status()
        
        assert self.service_name in status
        client_status = status[self.service_name]
        assert isinstance(client_status, HTTPClientStatus)
        assert client_status.service_name == self.service_name
        assert client_status.base_url == "http://test.example.com"
        assert client_status.is_session_closed is False
        assert client_status.circuit_breaker_count == 0

    def test_install_client_capture(self):
        """Test the client capture functionality for tests."""
        self.http_service.set_http_client(self.mock_client, self.service_name)
        
        captured = self.http_service._install_client_capture()
        
        assert self.service_name in captured
        assert captured[self.service_name] is self.mock_client

    def test_protocol_compliance(self):
        """Test that HTTPService implements HTTPServiceProtocol correctly."""
        # This is more of a type checking test
        assert isinstance(self.http_service, HTTPServiceProtocol)
        
        # Test all protocol methods exist and are callable
        assert callable(getattr(self.http_service, 'set_http_client'))
        assert callable(getattr(self.http_service, 'get_client'))
        assert callable(getattr(self.http_service, 'start'))
        assert callable(getattr(self.http_service, 'stop'))
        assert callable(getattr(self.http_service, 'register_service_client'))
        assert callable(getattr(self.http_service, 'deregister_service_client'))
        assert callable(getattr(self.http_service, 'list_active_clients'))

    @pytest.mark.asyncio
    async def test_lifecycle_integration(self):
        """Test complete service lifecycle."""
        # Set up multiple clients
        client1 = MagicMock(spec=HTTPClient)
        client1.close = AsyncMock()
        client2 = MagicMock(spec=HTTPClient)
        client2.close = AsyncMock()
        
        # Register clients
        await self.http_service.register_service_client("service1", client1)
        await self.http_service.register_service_client("service2", client2)
        
        # Start service
        await self.http_service.start()
        assert self.http_service.is_ready
        assert len(self.http_service.list_active_clients()) == 2
        
        # Stop service
        await self.http_service.stop()
        assert not self.http_service.is_ready
        assert len(self.http_service.list_active_clients()) == 0
        
        # Verify clients were closed
        client1.close.assert_called_once()
        client2.close.assert_called_once()
