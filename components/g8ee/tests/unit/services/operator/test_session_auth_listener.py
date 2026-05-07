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

import asyncio
import hashlib
from unittest.mock import AsyncMock, MagicMock, patch
import pytest

from app.services.operator.session_auth_listener import SessionAuthListener
from app.clients.pubsub_client import PubSubClient
from app.services.operator.operator_session_service import OperatorSessionService
from app.services.operator.operator_data_service import OperatorDataService

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]

class TestSessionAuthListener:
    @pytest.fixture
    def mock_pubsub_client(self):
        client = MagicMock(spec=PubSubClient)
        client.publish = AsyncMock()
        client.subscribe = AsyncMock()
        client.unsubscribe = AsyncMock()
        client.on_channel_message = MagicMock()
        client.off_channel_message = MagicMock()
        return client

    @pytest.fixture
    def mock_session_service(self):
        return AsyncMock(spec=OperatorSessionService)

    @pytest.fixture
    def mock_operator_data_service(self):
        return AsyncMock(spec=OperatorDataService)

    @pytest.fixture
    def listener(self, mock_pubsub_client, mock_session_service, mock_operator_data_service):
        return SessionAuthListener(
            pubsub_client=mock_pubsub_client,
            session_service=mock_session_service,
            operator_data_service=mock_operator_data_service
        )

    async def test_listen_subscribes_to_channel(self, listener, mock_pubsub_client):
        operator_session_id = "ops_123"
        operator_id = "op_456"
        user_id = "user_789"
        org_id = "org_000"
        
        session_hash = hashlib.sha256(operator_session_id.encode()).hexdigest()
        expected_channel = f"auth.publish:session:{session_hash}"

        await listener.listen(operator_session_id, operator_id, user_id, org_id)

        mock_pubsub_client.subscribe.assert_called_once_with(expected_channel)
        mock_pubsub_client.on_channel_message.assert_called_once()
        assert expected_channel in listener._active_listeners

    async def test_listen_duplicate_ignored(self, listener, mock_pubsub_client):
        operator_session_id = "ops_123"
        await listener.listen(operator_session_id, "op1", "u1", "org1")
        await listener.listen(operator_session_id, "op1", "u1", "org1")

        assert mock_pubsub_client.subscribe.call_count == 1
        assert mock_pubsub_client.on_channel_message.call_count == 1

    async def test_message_handler_success(self, listener, mock_pubsub_client, mock_session_service, mock_operator_data_service):
        operator_session_id = "ops_123"
        operator_id = "op_456"
        user_id = "user_789"
        org_id = "org_000"
        api_key = "g8e_test_key"

        session_hash = hashlib.sha256(operator_session_id.encode()).hexdigest()
        auth_channel = f"auth.publish:session:{session_hash}"
        response_channel = f"auth.response:session:{session_hash}"

        # Setup mocks
        mock_session = MagicMock()
        mock_session.is_active = True
        mock_session_service.validate_session.return_value = mock_session

        mock_operator = MagicMock()
        mock_operator.api_key = api_key
        mock_operator_data_service.get_operator.return_value = mock_operator

        # Start listening
        await listener.listen(operator_session_id, operator_id, user_id, org_id)
        
        # Get the registered handler
        handler = mock_pubsub_client.on_channel_message.call_args[0][1]

        # Simulate message
        await handler(auth_channel, {})

        # Assertions
        mock_session_service.validate_session.assert_called_once_with(operator_session_id)
        mock_operator_data_service.get_operator.assert_called_once_with(operator_id)
        
        # Verify response published
        mock_pubsub_client.publish.assert_called_once()
        publish_args = mock_pubsub_client.publish.call_args
        assert publish_args[0][0] == response_channel
        response_data = publish_args[0][1]
        assert response_data["success"] is True
        assert response_data["operator_session_id"] == operator_session_id
        assert response_data["api_key"] == api_key

        # Verify cleanup happened
        mock_pubsub_client.unsubscribe.assert_called_once_with(auth_channel)
        mock_pubsub_client.off_channel_message.assert_called_once_with(auth_channel, handler)
        assert auth_channel not in listener._active_listeners

    async def test_message_handler_invalid_session(self, listener, mock_pubsub_client, mock_session_service):
        operator_session_id = "ops_123"
        session_hash = hashlib.sha256(operator_session_id.encode()).hexdigest()
        auth_channel = f"auth.publish:session:{session_hash}"
        response_channel = f"auth.response:session:{session_hash}"

        mock_session_service.validate_session.return_value = None

        await listener.listen(operator_session_id, "op1", "u1", "org1")
        handler = mock_pubsub_client.on_channel_message.call_args[0][1]
        await handler(auth_channel, {})

        mock_pubsub_client.publish.assert_called_once()
        response_data = mock_pubsub_client.publish.call_args[0][1]
        assert response_data["success"] is False
        assert "Session not found" in response_data["error"]

    async def test_message_handler_internal_error(self, listener, mock_pubsub_client, mock_session_service):
        operator_session_id = "ops_123"
        session_hash = hashlib.sha256(operator_session_id.encode()).hexdigest()
        auth_channel = f"auth.publish:session:{session_hash}"
        response_channel = f"auth.response:session:{session_hash}"

        mock_session_service.validate_session.side_effect = Exception("boom")

        await listener.listen(operator_session_id, "op1", "u1", "org1")
        handler = mock_pubsub_client.on_channel_message.call_args[0][1]
        await handler(auth_channel, {})

        mock_pubsub_client.publish.assert_called_once()
        response_data = mock_pubsub_client.publish.call_args[0][1]
        assert response_data["success"] is False
        assert response_data["error"] == "Internal error"

    async def test_auto_cleanup(self, listener, mock_pubsub_client):
        operator_session_id = "ops_123"
        session_hash = hashlib.sha256(operator_session_id.encode()).hexdigest()
        auth_channel = f"auth.publish:session:{session_hash}"

        with patch("asyncio.sleep", new=AsyncMock()) as mock_sleep:
            await listener.listen(operator_session_id, "op1", "u1", "org1")
            
            # Find the background task for auto_cleanup
            cleanup_task = None
            for task in listener._background_tasks:
                if "auto_cleanup" in str(task):
                    cleanup_task = task
                    break
            
            # We need to wait for the background task to reach sleep or finish
            # Since we mocked sleep, it will finish immediately if we await it
            if cleanup_task:
                await cleanup_task

            mock_pubsub_client.unsubscribe.assert_called_once_with(auth_channel)
            assert auth_channel not in listener._active_listeners

    async def test_message_handler_wrong_channel(self, listener, mock_pubsub_client, mock_session_service):
        operator_session_id = "ops_123"
        await listener.listen(operator_session_id, "op1", "u1", "org1")
        handler = mock_pubsub_client.on_channel_message.call_args[0][1]
        
        await handler("wrong_channel", {})
        
        mock_session_service.validate_session.assert_not_called()

    async def test_cleanup_handles_missing_channel(self, listener, mock_pubsub_client):
        # Should not raise
        await listener.cleanup("missing_channel")
        mock_pubsub_client.unsubscribe.assert_not_called()
