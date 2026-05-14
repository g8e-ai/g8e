# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0

import pytest
import json
from unittest.mock import MagicMock, AsyncMock, patch
from app.evals.runner.client import G8eClient

@pytest.mark.asyncio
async def test_client_context_manager():
    with patch('aiohttp.ClientSession') as mock_session_cls:
        mock_session = MagicMock()
        mock_session.close = AsyncMock()
        mock_session_cls.return_value = mock_session
        
        async with G8eClient("https://localhost", "https://localhost:8443") as client:
            assert client._session is not None
        
        mock_session.close.assert_called_once()

@pytest.mark.asyncio
async def test_create_investigation():
    async with G8eClient("https://localhost", "https://localhost:8443") as client:
        res = await client.create_investigation("session-123")
        assert "id" in res
        assert isinstance(res["id"], str)

@pytest.mark.asyncio
async def test_send_chat_message():
    base_url = "https://localhost"
    investigation_id = "inv-123"
    message = "hello"
    
    with patch('aiohttp.ClientSession') as mock_session_cls:
        mock_session = MagicMock()
        mock_session.close = AsyncMock()
        mock_session_cls.return_value = mock_session
        
        # Mock POST /chat/send
        mock_post_resp = MagicMock()
        mock_post_resp.raise_for_status = MagicMock()
        mock_post_resp.__aenter__ = AsyncMock(return_value=mock_post_resp)
        mock_post_resp.__aexit__ = AsyncMock()
        mock_session.post.return_value = mock_post_resp
        
        # Mock GET /sse/events
        mock_get_resp = MagicMock()
        mock_get_resp.raise_for_status = MagicMock()
        mock_get_resp.content = AsyncMock()
        
        # Simulate SSE data
        mock_get_resp.content.__aiter__.return_value = [
            b'data: {"type": "text_chunk", "data": "Hello"}\n',
            b'data: {"type": "text_chunk", "data": " world"}\n',
            b'data: [DONE]\n'
        ]
        
        mock_get_resp.__aenter__ = AsyncMock(return_value=mock_get_resp)
        mock_get_resp.__aexit__ = AsyncMock()
        mock_session.get.return_value = mock_get_resp
        
        async with G8eClient(base_url, "https://localhost:8443") as client:
            events = []
            async for event in client.send_chat_message(investigation_id, message, operator_session_id="session-123"):
                events.append(event)
            
            assert len(events) == 2
            assert events[0]["data"] == "Hello"
            assert events[1]["data"] == " world"
            
            # Verify header was passed
            mock_session.post.assert_called_once()
            _, kwargs = mock_session.post.call_args
            assert kwargs["headers"]["X-G8E-Operator-Session-ID"] == "session-123"

@pytest.mark.asyncio
async def test_client_context_manager_no_session():
    client = G8eClient("https://localhost", "https://localhost:8443")
    # Should not raise even if __aexit__ is called before __aenter__
    await client.__aexit__(None, None, None)

@pytest.mark.asyncio
async def test_send_chat_message_bad_json():
    base_url = "https://localhost"
    investigation_id = "inv-123"
    message = "hello"
    
    with patch('aiohttp.ClientSession') as mock_session_cls:
        mock_session = MagicMock()
        mock_session.close = AsyncMock()
        mock_session_cls.return_value = mock_session
        
        mock_post_resp = MagicMock()
        mock_post_resp.raise_for_status = MagicMock()
        mock_post_resp.__aenter__ = AsyncMock(return_value=mock_post_resp)
        mock_post_resp.__aexit__ = AsyncMock()
        mock_session.post.return_value = mock_post_resp
        
        mock_get_resp = MagicMock()
        mock_get_resp.raise_for_status = MagicMock()
        mock_get_resp.content = AsyncMock()
        mock_get_resp.content.__aiter__.return_value = [
            b'data: {invalid json}\n',
            b'not data line\n',
            b'data: [DONE]\n'
        ]
        mock_get_resp.__aenter__ = AsyncMock(return_value=mock_get_resp)
        mock_get_resp.__aexit__ = AsyncMock()
        mock_session.get.return_value = mock_get_resp
        
        async with G8eClient(base_url, "https://localhost:8443") as client:
            events = []
            async for event in client.send_chat_message(investigation_id, message):
                events.append(event)
            assert len(events) == 0

@pytest.mark.asyncio
async def test_approve_request():
    base_url = "https://localhost"
    approval_id = "app-123"
    
    with patch('aiohttp.ClientSession') as mock_session_cls:
        mock_session = MagicMock()
        mock_session.close = AsyncMock()
        mock_session_cls.return_value = mock_session
        
        mock_resp = MagicMock()
        mock_resp.raise_for_status = MagicMock()
        mock_resp.json = AsyncMock(return_value={"status": "approved"})
        mock_resp.__aenter__ = AsyncMock(return_value=mock_resp)
        mock_resp.__aexit__ = AsyncMock()
        mock_session.post.return_value = mock_resp
        
        async with G8eClient(base_url, "https://localhost:8443") as client:
            res = await client.approve_request(approval_id, operator_session_id="session-123")
            assert res["status"] == "approved"
            mock_session.post.assert_called_once()
            args, kwargs = mock_session.post.call_args
            assert kwargs["json"]["approval_id"] == approval_id
            assert kwargs["headers"]["X-G8E-Operator-Session-ID"] == "session-123"
