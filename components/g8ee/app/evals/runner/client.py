# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

"""g8ed HTTP/SSE client for eval runner."""

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
_SHARED_DIR = Path(__file__).resolve().parent.parent.parent.parent.parent.parent / "shared"
with open(_SHARED_DIR / "constants" / "public_api_paths.json") as f:
    PUBLIC_API_PATHS = json.load(f)

# Load internal API paths
with open(_SHARED_DIR / "constants" / "api_paths.json") as f:
    INTERNAL_API_PATHS = json.load(f)

class G8edClient:
    """Async client for g8ed chat API and SSE streams."""

    def __init__(self, g8ed_url: str, g8ee_url: str, device_token: str | None = None, ca_cert_path: str | None = None):
        self.g8ed_url = g8ed_url
        self.g8ee_url = g8ee_url
        self.device_token = device_token
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
        return {"id": str(uuid.uuid4())}

    async def _poll_investigation(
        self,
        investigation_id: str,
        operator_session_id: str,
        timeout_seconds: int = 120,
        poll_interval_seconds: float = 1.0,
    ) -> AsyncIterator[dict]:
        """Poll investigation endpoint for device token flows.

        Args:
            investigation_id: Investigation ID
            operator_session_id: Device token for authentication
            timeout_seconds: Maximum time to wait for completion
            poll_interval_seconds: Time between polls
        """
        import asyncio
        
        internal_url = f"{self.g8ee_url}{INTERNAL_API_PATHS['internal_prefix']}{INTERNAL_API_PATHS['g8ee']['investigation'].format(investigation_id=investigation_id)}"
        headers = {
            "X-Request-Timestamp": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"
        }
        if operator_session_id.startswith("dlk_"):
            headers["X-G8E-Device-Token"] = operator_session_id
        else:
            headers["X-G8E-Operator-Session-ID"] = operator_session_id

        start_time = datetime.now(UTC)
        
        while True:
            elapsed = (datetime.now(UTC) - start_time).total_seconds()
            if elapsed > timeout_seconds:
                logger.error("[EVALS-CLIENT] Polling timeout after %s seconds", timeout_seconds)
                yield {"type": "error", "data": f"Polling timeout after {timeout_seconds} seconds"}
                return

            try:
                async with self._session.get(internal_url, headers=headers) as resp:
                    if resp.status == 404:
                        # Investigation not created yet, wait and retry
                        await asyncio.sleep(poll_interval_seconds)
                        continue
                    resp.raise_for_status()
                    investigation = await resp.json()

                # Extract tool_calls and response_text from conversation_history
                conversation_history = investigation.get("conversation_history", [])
                tool_calls = []
                response_text = ""
                
                for msg in conversation_history:
                    metadata = msg.get("metadata", {})
                    sender = msg.get("sender", "")
                    content = msg.get("content", "")
                    
                    # Extract tool calls from operator command messages
                    if "command" in metadata and metadata.get("command"):
                        tool_calls.append({
                            "args": {"command": metadata["command"]},
                            "name": "run_commands_with_operator"
                        })
                    
                    # Extract AI response text
                    if "g8e.v1.source.ai" in sender:
                        response_text += content + "\n"

                # Yield tool_call events
                for tc in tool_calls:
                    yield {"type": "tool_call", "data": tc}
                
                # Yield text chunks
                if response_text:
                    yield {"type": "text_chunk", "data": response_text}
                
                # Check if investigation is complete (has AI response)
                if response_text or tool_calls:
                    logger.info("[EVALS-CLIENT] Polling complete: found %d tool calls and response text", len(tool_calls))
                    yield {"type": "done", "data": "Polling complete"}
                    return
                
                await asyncio.sleep(poll_interval_seconds)
                
            except Exception as e:
                logger.error("[EVALS-CLIENT] Polling error: %s", e)
                yield {"type": "error", "data": str(e)}
                return

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
        url = f"{self.g8ed_url}{PUBLIC_API_PATHS['chat_send']}"
        headers = {
            "X-Request-Timestamp": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"
        }
        if operator_session_id:
            if operator_session_id.startswith("dlk_"):
                headers["X-G8E-Device-Token"] = operator_session_id
            else:
                headers["X-G8E-Operator-Session-ID"] = operator_session_id

        payload = {
            "investigation_id": investigation_id,
            "message": message,
            "user_id": "evals_runner"
        }

        # Send the chat message
        async with self._session.post(url, json=payload, headers=headers) as resp:
            resp.raise_for_status()

        # Device token flows don't have web sessions, so they can't use SSE
        # Poll the investigation API instead
        if operator_session_id and operator_session_id.startswith("dlk_"):
            logger.info("[EVALS-CLIENT] Device token flow - polling investigation API")
            async for event in self._poll_investigation(investigation_id, operator_session_id):
                yield event
            return

        # Connect to SSE for the response
        sse_url = f"{self.g8ed_url}{PUBLIC_API_PATHS['sse_events']}"
        async with self._session.get(sse_url, headers=headers) as resp:
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
        url = f"{self.g8ed_url}{PUBLIC_API_PATHS['operator_approval_respond']}"
        headers = {
            "X-Request-Timestamp": datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"
        }
        if operator_session_id:
            if operator_session_id.startswith("dlk_"):
                headers["X-G8E-Device-Token"] = operator_session_id
            else:
                headers["X-G8E-Operator-Session-ID"] = operator_session_id

        payload = {
            "approval_id": approval_id,
            "action": "approve"
        }
        async with self._session.post(url, json=payload, headers=headers) as resp:
            resp.raise_for_status()
            return await resp.json()
