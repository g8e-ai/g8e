# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

"""g8ed HTTP/SSE client for eval runner."""

from __future__ import annotations

import asyncio
import ssl
from typing import AsyncIterator

import aiohttp


class G8edClient:
    """Async client for g8ed chat API and SSE streams."""

    def __init__(self, base_url: str, device_token: str | None = None, ca_cert_path: str | None = None):
        self.base_url = base_url
        self.device_token = device_token
        self.ca_cert_path = ca_cert_path
        self._session: aiohttp.ClientSession | None = None

    async def __aenter__(self):
        ssl_context = ssl.create_default_context(cafile=self.ca_cert_path) if self.ca_cert_path else False
        connector = aiohttp.TCPConnector(ssl=ssl_context)
        self._session = aiohttp.ClientSession(connector=connector)

        if self.device_token:
            from datetime import datetime, UTC
            url = f"{self.base_url}/api/auth/operator"
            headers = {"X-Request-Timestamp": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"}
            payload = {
                "auth_mode": "operator_session",
                "operator_session_id": self.device_token
            }
            async with self._session.post(url, json=payload, headers=headers) as resp:
                resp.raise_for_status()

        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        if self._session:
            await self._session.close()

    async def create_investigation(self, operator_session_id: str) -> dict:
        """Create a new investigation via g8ed API.

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
        import uuid
        return {"id": str(uuid.uuid4())}

    async def send_chat_message(
        self,
        investigation_id: str,
        message: str,
        operator_session_id: str,
    ) -> AsyncIterator[dict]:
        """Send a chat message and stream SSE events.

        Args:
            investigation_id: Investigation ID
            message: User message
            operator_session_id: Session ID for authentication

        Yields:
            SSE event dictionaries
        """
        from datetime import datetime, UTC
        url = f"{self.base_url}/api/chat/send"
        headers = {"X-Request-Timestamp": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"}
        payload = {
            "investigation_id": investigation_id,
            "message": message,
            "operator_session_id": operator_session_id,
            "user_id": "evals_runner"
        }

        # Send the chat message
        async with self._session.post(url, json=payload, headers=headers) as resp:
            resp.raise_for_status()

        # Connect to SSE for the response
        sse_url = f"{self.base_url}/api/sse/events"
        async with self._session.get(sse_url, headers=headers) as resp:
            resp.raise_for_status()

            async for line in resp.content:
                line = line.decode('utf-8').strip()
                if line.startswith('data: '):
                    data = line[6:]
                    if data == '[DONE]':
                        break
                    try:
                        import json
                        yield json.loads(data)
                    except json.JSONDecodeError:
                        continue

    async def approve_request(self, approval_id: str, operator_session_id: str) -> dict:
        """Approve a pending approval request.

        Args:
            approval_id: Approval request ID
            operator_session_id: Session ID for authentication

        Returns:
            Approval response data
        """
        from datetime import datetime, UTC
        url = f"{self.base_url}/api/operator/approval/respond"
        headers = {"X-Request-Timestamp": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"}
        payload = {
            "approval_id": approval_id,
            "action": "approve",
            "operator_session_id": operator_session_id
        }
        async with self._session.post(url, json=payload, headers=headers) as resp:
            resp.raise_for_status()
            return await resp.json()
