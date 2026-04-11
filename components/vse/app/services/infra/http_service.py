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

"""HTTP Service

Owns all HTTP client lifecycle state:
  - HTTP client references by service name
  - _http_ready flag
  - _active_clients dict
  - client registration/deregistration

Provides centralized HTTP client management following the same patterns
as PubSubService for consistency across the VSE service layer.
"""

import logging
from app.clients.http_client import HTTPClient
from app.models.infra import HTTPClientStatus
from app.errors import ValidationError

logger = logging.getLogger(__name__)


class HTTPService:
    """Owns all HTTP client lifecycle state and connection management.

    Provides centralized management of HTTP clients for different services,
    ensuring proper initialization, cleanup, and tracking of active connections.

    Services register their HTTP clients with unique names and can retrieve
    them as needed. The service manages startup/shutdown coordination and
    provides visibility into active client connections.
    """

    def __init__(self) -> None:
        self._http_ready: bool = False
        self._active_clients: dict[str, HTTPClient] = {}

    @property
    def is_ready(self) -> bool:
        """Check if the HTTP service is ready for client operations."""
        return self._http_ready

    def set_http_client(self, client: HTTPClient, service_name: str) -> None:
        """Set an HTTP client for a specific service.
        
        Args:
            client: The HTTP client instance
            service_name: Unique name for the service (e.g., "vsod", "external_api")
        """
        if not client:
            raise ValidationError(f"HTTP client is required for service '{service_name}'", component="vse")
        if not service_name:
            raise ValidationError("service_name is required for HTTP client registration", component="vse")
        
        self._active_clients[service_name] = client
        logger.info(f"[HTTP] HTTP client configured for service: {service_name}")

    def get_client(self, service_name: str) -> HTTPClient | None:
        """Get an HTTP client by service name.
        
        Args:
            service_name: The name of the service whose client to retrieve
            
        Returns:
            The HTTP client if found, None otherwise
        """
        return self._active_clients.get(service_name)

    async def start(self) -> None:
        """Start the HTTP service and initialize all registered clients."""
        if self._http_ready:
            logger.info("[HTTP] HTTP service already ready")
            return
        
        if not self._active_clients:
            logger.warning("[HTTP] No HTTP clients registered - service ready but no clients available")
        
        self._http_ready = True
        logger.info(f"[HTTP] HTTP service ready with {len(self._active_clients)} registered clients")

    async def stop(self) -> None:
        """Stop the HTTP service and cleanup all registered clients."""
        if not self._http_ready:
            logger.info("[HTTP] HTTP service already stopped")
            return

        for service_name, client in list(self._active_clients.items()):
            try:
                await client.close()
                logger.info(f"[HTTP] Closed HTTP client for service: {service_name}")
            except Exception as e:
                logger.error(f"[HTTP] Error closing HTTP client for {service_name}: {e}", exc_info=True)

        self._active_clients.clear()
        self._http_ready = False
        logger.info("[HTTP] HTTP service stopped - all clients closed")

    async def register_service_client(self, service_name: str, client: HTTPClient) -> None:
        """Register a new HTTP client for a service.
        
        Args:
            service_name: Unique name for the service
            client: The HTTP client instance
        """
        if service_name in self._active_clients:
            logger.warning(f"[HTTP] Overwriting existing HTTP client for service: {service_name}")
            # Close the existing client before replacing
            try:
                await self._active_clients[service_name].close()
            except Exception as e:
                logger.error(f"[HTTP] Error closing existing client for {service_name}: {e}")

        self.set_http_client(client, service_name)
        logger.info(f"[HTTP] Registered HTTP client for service: {service_name}")

    async def deregister_service_client(self, service_name: str) -> None:
        """Deregister and close an HTTP client for a service.
        
        Args:
            service_name: The name of the service to deregister
        """
        if service_name not in self._active_clients:
            logger.warning(f"[HTTP] No HTTP client found for service: {service_name}")
            return

        try:
            client = self._active_clients.pop(service_name)
            await client.close()
            logger.info(f"[HTTP] Deregistered and closed HTTP client for service: {service_name}")
        except Exception as e:
            logger.error(f"[HTTP] Error deregistering HTTP client for {service_name}: {e}", exc_info=True)

    def list_active_clients(self) -> list[str]:
        """List all currently active service client names.
        
        Returns:
            List of service names with registered HTTP clients
        """
        return list(self._active_clients.keys())

    def _install_client_capture(self) -> dict[str, HTTPClient]:
        """Capture all registered clients for verification in tests."""
        if not hasattr(self, "captured_clients"):
            self.captured_clients: dict[str, HTTPClient] = {}

        # Store a copy of current clients
        self.captured_clients.update(self._active_clients)
        return self.captured_clients

    @property
    def is_session_closed(self) -> bool:
        """Check if the internal session is closed."""
        if not hasattr(self, "_session"):
            return True
        return self._session is None or self._session.closed

    def get_client_status(self) -> dict[str, HTTPClientStatus]:
        """Get status information for all registered clients.
        
        Returns:
            Dictionary mapping service names to their client status
        """
        status = {}
        for service_name, client in self._active_clients.items():
            client_status = HTTPClientStatus(
                service_name=service_name,
                base_url=getattr(client, "base_url", "unknown"),
                is_session_closed=getattr(client, "is_session_closed", True),
                circuit_breaker_count=len(getattr(client, "circuit_breakers", {})),
            )
            status[service_name] = client_status
        return status
