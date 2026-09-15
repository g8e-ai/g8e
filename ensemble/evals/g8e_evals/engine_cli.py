# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Private engine entry point for Go facade invocation.

The Go ``./g8e eval`` facade invokes this module through the project
interpreter::

    <eval-venv-python> -m g8e_evals.engine_cli <request-path>

The module reads a typed ``EvalEngineRequest`` JSON file, validates the
schema version, dispatches to the registered operation handler, and emits
an ``EvalEngineResult`` as a single JSON object on stdout. The facade
translates error codes into Go sentinel errors without parsing human prose.

This module is not a public CLI. Operators discover evaluation through
``./g8e eval``; they never invoke ``python -m g8e_evals.engine_cli``
directly. The legacy Click CLI in ``g8e_evals.cli`` remains available as
an internal developer entry point but is no longer a documented product
and is not installed as a console script.
"""

from __future__ import annotations

import json
import sys
from collections.abc import Callable
from pathlib import Path

from pydantic import ValidationError

from g8e_evals.engine_protocol import (
    EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
    ERROR_CODE_EXIT_MAP,
    EvalErrorCode,
    EvalEngineRequest,
    EvalEngineResult,
    EvalEngineStatus,
    EvalOperation,
    failed_result,
)

# Type alias for an operation handler: receives a validated request and
# returns a typed result.
OperationHandler = Callable[[EvalEngineRequest], EvalEngineResult]

# Registry of operation handlers. Later phases (U5-U8) register handlers
# as operations are wired through the engine protocol. An unregistered
# operation produces a typed failed result so the Go facade receives a
# consistent machine-readable response rather than a Python traceback.
_OPERATION_HANDLERS: dict[EvalOperation, OperationHandler] = {}


def register_handler(operation: EvalOperation) -> Callable[[OperationHandler], OperationHandler]:
    """Decorator that registers an operation handler in the dispatch table."""

    def decorator(handler: OperationHandler) -> OperationHandler:
        _OPERATION_HANDLERS[operation] = handler
        return handler

    return decorator


def load_request(request_path: str) -> EvalEngineRequest:
    """Read and validate an EvalEngineRequest from a JSON file path."""
    path = Path(request_path)
    if not path.is_file():
        raise FileNotFoundError(f"request file not found: {request_path}")
    return EvalEngineRequest.model_validate_json(path.read_bytes())


def validate_schema_version(request: EvalEngineRequest) -> None:
    """Raise a protocol-mismatch error if the schema version does not match."""
    if request.schema_version != EVAL_ENGINE_REQUEST_SCHEMA_VERSION:
        raise ProtocolMismatchError(
            f"schema_version {request.schema_version!r} does not match "
            f"expected {EVAL_ENGINE_REQUEST_SCHEMA_VERSION!r}"
        )


class ProtocolMismatchError(Exception):
    """Raised when the request schema version does not match the engine."""


class EngineError(Exception):
    """Base class for engine-level errors that map to typed error codes."""

    def __init__(self, code: EvalErrorCode, stage: str, safe_detail: str = "") -> None:
        super().__init__(safe_detail or code.value)
        self.code = code
        self.stage = stage
        self.safe_detail = safe_detail


def install_builtin_handlers() -> None:
    from g8e_evals.eval_engine_handlers import builtin_operation_handlers

    # setdefault preserves test-registered handlers so the production
    # install never overwrites a handler a test wired before dispatch.
    for operation, handler in builtin_operation_handlers().items():
        _OPERATION_HANDLERS.setdefault(operation, handler)


def dispatch(request: EvalEngineRequest) -> EvalEngineResult:
    """Dispatch a validated request to its registered operation handler.

    Unregistered operations return a typed failed result so the Go facade
    receives a consistent machine-readable response. Later phases register
    handlers as operations are wired through the engine protocol.
    """
    handler = _OPERATION_HANDLERS.get(request.operation)
    if handler is None:
        return failed_result(
            request,
            error_code=EvalErrorCode.NONE,
            error_stage="dispatch",
            safe_detail=f"operation {request.operation.value} not yet wired through engine protocol",
        )
    return handler(request)


def emit_result(result: EvalEngineResult, *, json_output: bool) -> None:
    """Emit the result as a single JSON object on stdout.

    In JSON mode the result is always emitted as canonical JSON. In human
    mode a concise summary is printed for succeeded results and the safe
    detail for failed results; the JSON object is still available for
    machine consumers that read stdout.
    """
    payload = result.model_dump_json(exclude_none=True, by_alias=True)
    if json_output:
        sys.stdout.write(payload)
        sys.stdout.write("\n")
        sys.stdout.flush()
        return
    if result.status == EvalEngineStatus.SUCCEEDED:
        sys.stdout.write(f"{result.operation.value}: {result.status.value}\n")
    else:
        detail = result.safe_detail or result.error_code.value or "failed"
        sys.stdout.write(f"{result.operation.value}: {result.status.value}: {detail}\n")
    sys.stdout.flush()


def exit_code_for(result: EvalEngineResult) -> int:
    """Map a result's error code to the process exit code."""
    if result.status == EvalEngineStatus.SUCCEEDED:
        return 0
    return ERROR_CODE_EXIT_MAP.get(result.error_code, 1)


def run_engine(request_path: str) -> int:
    """Load, validate, dispatch, and emit. Returns the process exit code."""
    try:
        request = load_request(request_path)
    except FileNotFoundError as exc:
        result = EvalEngineResult(
            schema_version=EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
            operation=EvalOperation.BUNDLE,
            operation_id="",
            status=EvalEngineStatus.FAILED,
            error_code=EvalErrorCode.CONFIG_INVALID,
            error_stage="load_request",
            safe_detail=str(exc),
        )
        emit_result(result, json_output=True)
        return exit_code_for(result)
    except (json.JSONDecodeError, ValidationError) as exc:
        result = EvalEngineResult(
            schema_version=EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
            operation=EvalOperation.BUNDLE,
            operation_id="",
            status=EvalEngineStatus.FAILED,
            error_code=EvalErrorCode.CONFIG_INVALID,
            error_stage="load_request",
            safe_detail=f"invalid request JSON: {exc}",
        )
        emit_result(result, json_output=True)
        return exit_code_for(result)

    json_output = request.flags.json_output if request.flags else False

    try:
        validate_schema_version(request)
    except ProtocolMismatchError as exc:
        result = failed_result(
            request,
            error_code=EvalErrorCode.ENGINE_PROTOCOL_MISMATCH,
            error_stage="validate_schema",
            safe_detail=str(exc),
        )
        emit_result(result, json_output=json_output)
        return exit_code_for(result)

    try:
        install_builtin_handlers()
        result = dispatch(request)
    except EngineError as exc:
        result = failed_result(
            request,
            error_code=exc.code,
            error_stage=exc.stage,
            safe_detail=exc.safe_detail,
        )
    except Exception as exc:
        result = failed_result(
            request,
            error_code=EvalErrorCode.CHILD_EXIT_NON_ZERO,
            error_stage="dispatch",
            safe_detail=f"unexpected engine error: {type(exc).__name__}",
        )

    emit_result(result, json_output=json_output)
    return exit_code_for(result)


def main(argv: list[str] | None = None) -> int:
    """Entry point for ``python -m g8e_evals.engine_cli <request-path>``."""
    args = argv if argv is not None else sys.argv[1:]
    if len(args) != 1:
        sys.stderr.write("usage: python -m g8e_evals.engine_cli <request-path>\n")
        sys.stderr.write(f"got {len(args)} argument(s)\n")
        return 1
    return run_engine(args[0])


if __name__ == "__main__":
    sys.modules.setdefault("g8e_evals.engine_cli", sys.modules[__name__])
    sys.exit(main())
