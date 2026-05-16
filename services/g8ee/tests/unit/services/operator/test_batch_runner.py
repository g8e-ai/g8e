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

"""Unit tests for BatchRunner."""

import asyncio
import pytest
from unittest.mock import AsyncMock

from app.models.settings import BatchExecutionSettings
from app.models.tool_results import PerOperatorResultBase
from app.services.operator.batch_runner import BatchRunner, LifecycleEmitter


class MockPerOperatorResult(PerOperatorResultBase):
    """Mock per-operator result for testing."""
    execution_id: str
    operator_id: str
    hostname: str
    success: bool
    error: str | None = None


class MockLifecycleEmitter(LifecycleEmitter):
    """Mock lifecycle emitter for testing."""

    def __init__(self):
        self.started_calls = []
        self.completed_calls = []
        self.failed_calls = []

    async def emit_started(self, operator_id: str, hostname: str, execution_id: str, batch_id: str) -> None:
        self.started_calls.append((operator_id, hostname, execution_id, batch_id))

    async def emit_completed(self, operator_id: str, hostname: str, execution_id: str, batch_id: str, result: MockPerOperatorResult) -> None:
        self.completed_calls.append((operator_id, hostname, execution_id, batch_id, result))

    async def emit_failed(self, operator_id: str, hostname: str, execution_id: str, batch_id: str, error: str) -> None:
        self.failed_calls.append((operator_id, hostname, execution_id, batch_id, error))


class MockOperatorDocument:
    """Mock operator document for testing."""
    def __init__(self, operator_id: str, hostname: str):
        self.operator_id = operator_id
        self.hostname = hostname


pytestmark = [pytest.mark.unit]


class TestBatchRunner:

    @pytest.mark.asyncio
    async def test_single_operator_success(self):
        settings = BatchExecutionSettings(max_concurrency=10, fail_fast=False)
        runner = BatchRunner(settings)

        targets = [MockOperatorDocument("op-1", "host-1")]
        lifecycle = MockLifecycleEmitter()

        async def mock_dispatch(op, execution_id):
            return MockPerOperatorResult(
                execution_id=execution_id,
                operator_id=op.operator_id,
                hostname=op.hostname,
                success=True,
            )

        execution_id_generator = lambda: f"exec_{len(lifecycle.started_calls)}"

        result = await runner.run(
            targets=targets,
            batch_id="batch-123",
            correlation_id="corr-456",
            dispatch=mock_dispatch,
            lifecycle=lifecycle,
            execution_id_generator=execution_id_generator,
        )

        assert result.batch_execution is False
        assert result.operators_used == 1
        assert result.successful_count == 1
        assert result.failed_count == 0
        assert len(result.per_operator_results) == 1
        assert len(lifecycle.started_calls) == 1
        assert len(lifecycle.completed_calls) == 1
        assert len(lifecycle.failed_calls) == 0

    @pytest.mark.asyncio
    async def test_multiple_operators_success(self):
        settings = BatchExecutionSettings(max_concurrency=2, fail_fast=False)
        runner = BatchRunner(settings)

        targets = [
            MockOperatorDocument("op-1", "host-1"),
            MockOperatorDocument("op-2", "host-2"),
            MockOperatorDocument("op-3", "host-3"),
        ]
        lifecycle = MockLifecycleEmitter()

        async def mock_dispatch(op, execution_id):
            return MockPerOperatorResult(
                execution_id=execution_id,
                operator_id=op.operator_id,
                hostname=op.hostname,
                success=True,
            )

        execution_id_generator = lambda: f"exec_{len(lifecycle.started_calls)}"

        result = await runner.run(
            targets=targets,
            batch_id="batch-123",
            correlation_id="corr-456",
            dispatch=mock_dispatch,
            lifecycle=lifecycle,
            execution_id_generator=execution_id_generator,
        )

        assert result.batch_execution is True
        assert result.operators_used == 3
        assert result.successful_count == 3
        assert result.failed_count == 0
        assert len(result.per_operator_results) == 3
        assert len(lifecycle.started_calls) == 3
        assert len(lifecycle.completed_calls) == 3
        assert len(lifecycle.failed_calls) == 0

    @pytest.mark.asyncio
    async def test_fail_fast_on_first_failure(self):
        settings = BatchExecutionSettings(max_concurrency=2, fail_fast=True)
        runner = BatchRunner(settings)

        targets = [
            MockOperatorDocument("op-1", "host-1"),
            MockOperatorDocument("op-2", "host-2"),
            MockOperatorDocument("op-3", "host-3"),
        ]
        lifecycle = MockLifecycleEmitter()

        call_count = 0

        async def mock_dispatch(op, execution_id):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                raise Exception("First operator fails")
            return MockPerOperatorResult(
                execution_id=execution_id,
                operator_id=op.operator_id,
                hostname=op.hostname,
                success=True,
            )

        def error_factory(op, execution_id, error_msg):
            return MockPerOperatorResult(
                execution_id=execution_id,
                operator_id=op.operator_id,
                hostname=op.hostname,
                success=False,
                error=error_msg,
            )

        execution_id_generator = lambda: f"exec_{len(lifecycle.started_calls)}"

        result = await runner.run(
            targets=targets,
            batch_id="batch-123",
            correlation_id="corr-456",
            dispatch=mock_dispatch,
            lifecycle=lifecycle,
            execution_id_generator=execution_id_generator,
            error_factory=error_factory,
        )

        assert result.operators_used >= 1
        assert result.failed_count >= 1
        assert len(lifecycle.failed_calls) >= 1

    @pytest.mark.asyncio
    async def test_empty_targets_raises_error(self):
        settings = BatchExecutionSettings(max_concurrency=10, fail_fast=False)
        runner = BatchRunner(settings)

        lifecycle = MockLifecycleEmitter()

        with pytest.raises(ValueError, match="targets list cannot be empty"):
            await runner.run(
                targets=[],
                batch_id="batch-123",
                correlation_id="corr-456",
                dispatch=AsyncMock(),
                lifecycle=lifecycle,
                execution_id_generator=lambda: "exec-1",
            )

    @pytest.mark.asyncio
    async def test_concurrency_limit(self):
        settings = BatchExecutionSettings(max_concurrency=1, fail_fast=False)
        runner = BatchRunner(settings)

        targets = [
            MockOperatorDocument("op-1", "host-1"),
            MockOperatorDocument("op-2", "host-2"),
        ]
        lifecycle = MockLifecycleEmitter()

        concurrent_count = 0
        max_concurrent = 0

        async def mock_dispatch(op, execution_id):
            nonlocal concurrent_count, max_concurrent
            concurrent_count += 1
            max_concurrent = max(max_concurrent, concurrent_count)
            await asyncio.sleep(0.01)
            concurrent_count -= 1
            return MockPerOperatorResult(
                execution_id=execution_id,
                operator_id=op.operator_id,
                hostname=op.hostname,
                success=True,
            )

        execution_id_generator = lambda: f"exec_{len(lifecycle.started_calls)}"

        await runner.run(
            targets=targets,
            batch_id="batch-123",
            correlation_id="corr-456",
            dispatch=mock_dispatch,
            lifecycle=lifecycle,
            execution_id_generator=execution_id_generator,
        )

        assert max_concurrent <= 1
