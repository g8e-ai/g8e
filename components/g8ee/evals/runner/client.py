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

    def __init__(self, base_url: str, ca_cert_path: str | None = None):
        self.base_url = base_url
        self.ca_cert_path = ca_cert_path
        self._session: aiohttp.ClientSession | None = None

    async def __aenter__(self):
        ssl_context = None
        if self.ca_cert_path:
            ssl_context = ssl.create_default_context(cafile=self.ca_cert_path)

        connector = aiohttp.TCPConnector(ssl=ssl_context)
        self._session = aiohttp.ClientSession(connector=connector)
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
        url = f"{self.base_url}/api/investigations"
        payload = {"operator_session_id": operator_session_id}
        async with self._session.post(url, json=payload) as resp:
            resp.raise_for_status()
            return await resp.json()

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
        url = f"{self.base_url}/api/chat"
        payload = {
            "investigation_id": investigation_id,
            "message": message,
            "operator_session_id": operator_session_id,
        }

        async with self._session.post(url, json=payload) as resp:
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
        url = f"{self.base_url}/api/approvals/{approval_id}"
        headers = {"x-operator-session-id": operator_session_id}
        async with self._session.post(url, headers=headers) as resp:
            resp.raise_for_status()
            return await resp.json()
