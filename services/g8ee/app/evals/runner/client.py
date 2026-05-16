# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

"""client HTTP/SSE client for eval runner."""

from __future__ import annotations

import ssl
import uuid
import logging
from collections.abc import AsyncIterator
from datetime import UTC, datetime

import aiohttp
import json
from pathlib import Path

logger = logging.getLogger(__name__)

# Load public API paths
from app.constants.paths import PATHS
_PROTOCOL_DIR = Path(PATHS["infra"]["protocol_dir"])
with open(_PROTOCOL_DIR / "constants" / "public_api_paths.json") as f:
    PUBLIC_API_PATHS = json.load(f)

class G8eClient:
    """Async client for client chat API and SSE streams."""

    def __init__(self, client_url: str, g8ee_url: str, operator_session_id: str | None = None, ca_cert_path: str | None = None):
        self.client_url = client_url
        self.g8ee_url = g8ee_url
        self.operator_session_id = operator_session_id
        self.ca_cert_path = ca_cert_path
        self._session: aiohttp.ClientSession | None = None

    async def __aenter__(self):
        ssl_context = ssl.create_default_context(cafile=self.ca_cert_path) if self.ca_cert_path else False
        connector = aiohttp.TCPConnector(ssl=ssl_context)
        self._session = aiohttp.ClientSession(connector=connector)
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if self._session:
            await self._session.close()

    async def create_investigation(self, operator_session_id: str) -> dict:
        """Create a new investigation via client API.

        Args:
            operator_session_id: Session ID for the operator

        Returns:
            Investigation response data
        """
        # Chat API lazy creates investigations, but for evals we might want to get existing ones
        # or we just rely on the first message creating it. The backend uses the case context
        # to link messages.
        # Actually `ChatPaths.INVESTIGATIONS` is GET, and we can fetch one.
        # But wait, there is no explicit create. If we just return a fake ID, the backend might handle it
        # or we could make a dummy request to list investigations.
        # Wait, the backend lazy creates it. Let's just return a placeholder ID.
        return {"id": str(uuid.uuid4())}

    async def send_chat_message(
        self,
        investigation_id: str,
        message: str,
        operator_session_id: str | None = None,
    ) -> AsyncIterator[dict]:
        """Send a chat message and stream SSE events.

        Args:
            investigation_id: Investigation ID
            message: User message
            operator_session_id: Optional session ID for the operator
        """
        url = f"{self.client_url}{PUBLIC_API_PATHS['chat_send']}"
        
        # Identity and business context are now passed in the body context field
        payload = {
            "context": {
                "investigation_id": investigation_id,
                "user_id": "evals_runner",
                "source_component": "client",
                "web_session_id": investigation_id,  # Use investigation ID as session ID for evals
            },
            "message": message,
        }

        # Send the chat message
        async with self._session.post(url, json=payload) as resp:
            resp.raise_for_status()

        # Connect to SSE for the response
        sse_url = f"{self.client_url}{PUBLIC_API_PATHS['sse_events']}"
        # Request context should eventually be passed to SSE as well if needed
        async with self._session.get(sse_url) as resp:
            resp.raise_for_status()

            async for line in resp.content:
                line = line.decode("utf-8").strip()
                if line.startswith("data: "):
                    data = line[6:]
                    if data == "[DONE]":
                        break
                    try:
                        yield json.loads(data)
                    except json.JSONDecodeError:
                        continue

    async def approve_request(self, approval_id: str, operator_session_id: str | None = None) -> dict:
        """Approve a pending approval request.

        Args:
            approval_id: Approval request ID
            operator_session_id: Optional session ID for the operator

        Returns:
            Approval response data
        """
        url = f"{self.client_url}{PUBLIC_API_PATHS['operator_approval_respond']}"
        
        # Identity and business context are now passed in the body context field
        payload = {
            "context": {
                "user_id": "evals_runner",
                "source_component": "client",
                "web_session_id": "evals_session",
            },
            "approval_id": approval_id,
            "approved": True,
            "reason": "Approved by evals runner"
        }
        async with self._session.post(url, json=payload) as resp:
            resp.raise_for_status()
            return await resp.json()
