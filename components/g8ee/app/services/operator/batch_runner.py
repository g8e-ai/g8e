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

"""Batch execution dispatcher for operator tools.

Provides a unified runner for fan-out operations across multiple operators
with configurable concurrency and fail-fast behavior.
"""

from __future__ import annotations

import asyncio
import logging
from typing import TYPE_CHECKING, Callable, TypeVar

from app.models.settings import BatchExecutionSettings
from app.models.tool_results import BatchExecutionMeta, PerOperatorResultBase

if TYPE_CHECKING:
    from app.models.operators import OperatorDocument

T = TypeVar("T", bound=PerOperatorResultBase)

logger = logging.getLogger(__name__)


class BatchRunResult(BatchExecutionMeta):
    """Result of a batch execution run.

    Carries per-operator results and aggregated batch metadata.
    """
    per_operator_results: list[T]

    @classmethod
    def create(cls, per_operator_results: list[T], batch_id: str | None = None) -> "BatchRunResult[T]":
        """Create a BatchRunResult from per-operator results."""
        successful_count = sum(1 for r in per_operator_results if r.success)
        failed_count = len(per_operator_results) - successful_count
        
        return cls(
            batch_execution=len(per_operator_results) > 1,
            batch_id=batch_id,
            operators_used=len(per_operator_results),
            successful_count=successful_count,
            failed_count=failed_count,
            per_operator_results=per_operator_results,
        )


class LifecycleEmitter:
    """Protocol for lifecycle event emission during batch execution.

    The caller supplies an implementation that knows the specific tool's
    event family (COMMAND, FILE_EDIT, PORT_CHECK, etc.).
    """

    async def emit_started(self, operator_id: str, hostname: str, execution_id: str, batch_id: str) -> None:
        """Emitted when per-operator execution begins."""
        raise NotImplementedError("LifecycleEmitter.emit_started must be implemented by subclasses")

    async def emit_completed(self, operator_id: str, hostname: str, execution_id: str, batch_id: str, result: T) -> None:
        """Emitted when per-operator execution succeeds."""
        raise NotImplementedError("LifecycleEmitter.emit_completed must be implemented by subclasses")

    async def emit_failed(self, operator_id: str, hostname: str, execution_id: str, batch_id: str, error: str) -> None:
        """Emitted when per-operator execution fails."""
        raise NotImplementedError("LifecycleEmitter.emit_failed must be implemented by subclasses")


class BatchRunner:
    """Universal batch execution dispatcher for operator tools.

    Coordinates fan-out execution across multiple operators with bounded
    concurrency and optional fail-fast behavior.
    """

    def __init__(self, batch_settings: BatchExecutionSettings):
        """Initialize the batch runner.

        Args:
            batch_settings: Batch execution configuration including max_concurrency and fail_fast.
        """
        self._settings = batch_settings
        self._logger = logging.getLogger(__name__)

    async def run(
        self,
        *,
        targets: list[OperatorDocument],
        batch_id: str,
        correlation_id: str | None,
        dispatch: Callable[[OperatorDocument, str], T],
        lifecycle: LifecycleEmitter,
        execution_id_generator: Callable[[], str],
        error_factory: Callable[[OperatorDocument, str, str], T] | None = None,
    ) -> BatchRunResult[T]:
        """Execute a batch operation across multiple operators.

        Args:
            targets: List of operator documents to execute on.
            batch_id: Correlation ID for this batch run.
            correlation_id: Optional correlation ID from upstream (e.g. Tribunal).
            dispatch: Async callable that executes the operation on a single operator.
            lifecycle: Lifecycle emitter for tool-specific event publishing.
            execution_id_generator: Callable that generates per-operator execution IDs.
            error_factory: Optional callable that creates error results for failed executions.
                Signature: (operator, execution_id, error_msg) -> result.

        Returns:
            BatchRunResult containing per-operator results and aggregated metadata.
        """
        if not targets:
            raise ValueError("targets list cannot be empty")

        fail_fast_event = asyncio.Event()
        semaphore = asyncio.Semaphore(self._settings.max_concurrency)
        results: list[T] = []

        async def execute_one(operator: OperatorDocument) -> tuple[str, T]:
            """Execute the operation on a single operator with concurrency control.

            Returns a tuple of (execution_id, result).
            """
            execution_id = execution_id_generator()

            async with semaphore:
                if self._settings.fail_fast and fail_fast_event.is_set():
                    self._logger.info(
                        "[BatchRunner] Skipping execution for %s due to fail_fast",
                        operator.operator_id,
                    )
                    raise asyncio.CancelledError("fail_fast triggered")

                try:
                    await lifecycle.emit_started(
                        operator_id=operator.operator_id,
                        hostname=operator.hostname or operator.operator_id,
                        execution_id=execution_id,
                        batch_id=batch_id,
                    )

                    result = await dispatch(operator, execution_id)

                    await lifecycle.emit_completed(
                        operator_id=operator.operator_id,
                        hostname=operator.hostname or operator.operator_id,
                        execution_id=execution_id,
                        batch_id=batch_id,
                        result=result,
                    )

                    return (execution_id, result)

                except Exception as exc:
                    error_msg = str(exc)
                    self._logger.error(
                        "[BatchRunner] Execution failed for %s: %s",
                        operator.operator_id,
                        error_msg,
                    )

                    await lifecycle.emit_failed(
                        operator_id=operator.operator_id,
                        hostname=operator.hostname or operator.operator_id,
                        execution_id=execution_id,
                        batch_id=batch_id,
                        error=error_msg,
                    )

                    if self._settings.fail_fast:
                        self._logger.info("[BatchRunner] Setting fail_fast event")
                        fail_fast_event.set()

                    # Attach execution_id to exception for error_factory
                    exc.execution_id = execution_id
                    raise

        tasks = [execute_one(op) for op in targets]

        completed_results = await asyncio.gather(*tasks, return_exceptions=True)

        for idx, result in enumerate(completed_results):
            operator = targets[idx]

            if isinstance(result, asyncio.CancelledError):
                self._logger.debug("[BatchRunner] Task cancelled for %s (fail_fast)", operator.operator_id)
            elif isinstance(result, Exception):
                self._logger.error("[BatchRunner] Task failed for %s: %s", operator.operator_id, result)
                if error_factory:
                    execution_id = getattr(result, 'execution_id', 'unknown')
                    error_result = error_factory(operator, execution_id, str(result))
                    results.append(error_result)
            elif isinstance(result, tuple) and len(result) == 2:
                execution_id, op_result = result
                results.append(op_result)
            else:
                self._logger.error("[BatchRunner] Unexpected result type: %s", type(result))

        return BatchRunResult.create(per_operator_results=results, batch_id=batch_id)
