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

"""OperatorCommandValidator tests."""

from unittest.mock import AsyncMock

import pytest

from app.constants import OperatorStatus
from app.errors import AuthorizationError, ResourceNotFoundError
from app.models.operators import OperatorDocument
from app.security.operator_command_validator import OperatorCommandValidator

pytestmark = [pytest.mark.unit, pytest.mark.asyncio(loop_scope="session")]


class TestOperatorCommandValidator:

    @pytest.fixture
    def mock_operator_cache(self):
        cache = AsyncMock()
        cache.get_operator = AsyncMock()
        return cache

    @pytest.fixture
    def validator(self, mock_operator_cache):
        return OperatorCommandValidator(operator_cache=mock_operator_cache)

    async def test_validate_command_execution_active_operator(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.ACTIVE,
        )

        result = await validator.validate_command_execution(
            operator_id="op-test-1",
            command="ls -la",
        )
        assert result is True

    async def test_validate_command_execution_bound_operator(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.BOUND,
        )

        result = await validator.validate_command_execution(
            operator_id="op-test-1",
            command="ls -la",
        )
        assert result is True

    async def test_validate_command_execution_stale_operator(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.STALE,
        )

        result = await validator.validate_command_execution(
            operator_id="op-test-1",
            command="ls -la",
        )
        assert result is True

    async def test_validate_command_execution_stopped_operator_rejected(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.STOPPED,
        )

        with pytest.raises(AuthorizationError):
            await validator.validate_command_execution(
                operator_id="op-test-1",
                command="ls -la",
            )

    async def test_validate_command_execution_offline_operator_rejected(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.OFFLINE,
        )

        with pytest.raises(AuthorizationError):
            await validator.validate_command_execution(
                operator_id="op-test-1",
                command="ls -la",
            )

    async def test_validate_command_execution_not_found(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = None

        with pytest.raises(ResourceNotFoundError) as exc_info:
            await validator.validate_command_execution(
                operator_id="op-nonexistent",
                command="ls -la",
            )
        assert exc_info.value.get_http_status() == 404

    async def test_validate_ownership_not_found(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = None

        with pytest.raises(ResourceNotFoundError) as exc_info:
            await validator.validate_operator_ownership("op-nonexistent", "user-123")
        assert exc_info.value.get_http_status() == 404

    async def test_validate_ownership_correct_user(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-123",
        )

        result = await validator.validate_operator_ownership("op-test-1", "user-123")
        assert result is True

    async def test_validate_ownership_wrong_user(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-123",
        )

        with pytest.raises(AuthorizationError, match="Not authorized"):
            await validator.validate_operator_ownership("op-test-1", "user-wrong")

    async def test_validate_binding_success(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.BOUND,
            operator_session_id="op-session-1",
            bound_web_session_id="web-session-1",
        )

        result = await validator.validate_operator_binding(
            operator_session_id="op-session-1",
            web_session_id="web-session-1",
            operator_id="op-test-1",
        )
        assert result.valid is True

    async def test_validate_binding_not_bound(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.ACTIVE,
            operator_session_id="op-session-1",
            bound_web_session_id="web-session-1",
        )

        result = await validator.validate_operator_binding(
            operator_session_id="op-session-1",
            web_session_id="web-session-1",
            operator_id="op-test-1",
        )
        assert result.valid is False
        assert "not bound" in result.reason

    async def test_validate_binding_web_session_mismatch(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.BOUND,
            operator_session_id="op-session-1",
            bound_web_session_id="web-session-DIFFERENT",
        )

        result = await validator.validate_operator_binding(
            operator_session_id="op-session-1",
            web_session_id="web-session-1",
            operator_id="op-test-1",
        )
        assert result.valid is False
        assert "mismatch" in result.reason

    async def test_validate_binding_operator_session_mismatch(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.BOUND,
            operator_session_id="op-session-DIFFERENT",
            bound_web_session_id="web-session-1",
        )

        result = await validator.validate_operator_binding(
            operator_session_id="op-session-1",
            web_session_id="web-session-1",
            operator_id="op-test-1",
        )
        assert result.valid is False
        assert "session ID mismatch" in result.reason

    async def test_validate_binding_missing_operator_session_id(self, validator):
        result = await validator.validate_operator_binding(
            operator_session_id=None,
            web_session_id="web-session-1",
            operator_id="op-test-1",
        )
        assert result.valid is False

    async def test_validate_binding_missing_web_session_id(self, validator):
        result = await validator.validate_operator_binding(
            operator_session_id="op-session-1",
            web_session_id=None,
            operator_id="op-test-1",
        )
        assert result.valid is False

    async def test_check_health_active_operator(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.ACTIVE,
        )

        result = await validator.check_operator_health("op-test-1")
        assert result.healthy is True
        assert result.status == OperatorStatus.ACTIVE

    async def test_check_health_stopped_operator(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = OperatorDocument(
            id="op-test-1",
            user_id="user-test",
            status=OperatorStatus.STOPPED,
        )

        result = await validator.check_operator_health("op-test-1")
        assert result.healthy is False

    async def test_check_health_not_found(self, validator, mock_operator_cache):
        mock_operator_cache.get_operator.return_value = None

        result = await validator.check_operator_health("op-nonexistent")
        assert result.healthy is False
        assert result.reason == "Operator not found"
        assert result.status == OperatorStatus.UNAVAILABLE
