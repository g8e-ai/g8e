# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed engine protocol shared between the Go facade and the Python engine.

This module is the Python-side mirror of ``internal/models/eval_engine.go``.
Every constant value, field name, and JSON tag must match the Go types
byte-for-byte. The contract test ``tests/test_engine_protocol_contract.py``
asserts parity by importing the Go constant set from a generated JSON
fixture and comparing it against the Python values defined here.

The Go facade constructs an ``EvalEngineRequest``, writes it to a typed
JSON file, and invokes ``python -m g8e_evals.engine_cli <request-path>``.
The engine reads, validates, dispatches, and emits an ``EvalEngineResult``
as a single JSON object on stdout. The facade translates error codes into
Go sentinel errors without parsing human prose.
"""

from __future__ import annotations

from enum import StrEnum
from typing import Any

from pydantic import BaseModel, Field

# The schema version the Go facade and the Python engine exchange. Bumped
# in lockstep with the contract test that asserts Go/Python parity.
EVAL_ENGINE_REQUEST_SCHEMA_VERSION = "1.0.0"


class EvalOperation(StrEnum):
    """Engine-backed operations the Go facade dispatches to the Python engine.

    Read-only facade-only operations (setup, doctor, draft, plan, check,
    lease lifecycle) are not engine operations and are absent from this
    enum. The string values are the stable wire identifiers shared with Go
    through a contract-tested registry.
    """

    DIAGNOSTIC_START = "diagnostic_start"
    DIAGNOSTIC_STATUS = "diagnostic_status"
    DIAGNOSTIC_STOP = "diagnostic_stop"
    DIAGNOSTIC_VERIFY = "diagnostic_verify"
    CAMPAIGN_START = "campaign_start"
    CAMPAIGN_STATUS = "campaign_status"
    CAMPAIGN_STOP = "campaign_stop"
    CAMPAIGN_VERIFY = "campaign_verify"
    CAMPAIGN_PUBLISH = "campaign_publish"
    CAMPAIGN_SET_PLAN = "campaign_set_plan"
    CAMPAIGN_SET_VALIDATE = "campaign_set_validate"
    CAMPAIGN_SET_VERIFY = "campaign_set_verify"
    CONTROLLER_RUN = "controller_run"
    CONTROLLER_STATUS = "controller_status"
    CONTROLLER_STOP = "controller_stop"
    CONTROLLER_RECOVER = "controller_recover"
    BUNDLE = "bundle"
    VERIFY = "verify"
    VERIFY_RECEIPTS = "verify_receipts"
    PUBLISH = "publish"
    QUALIFICATION_HASH_SOURCE = "qualification_hash_source"
    QUALIFICATION_CANDIDATE = "qualification_candidate"
    QUALIFICATION_COLLECT_RUNTIME = "qualification_collect_runtime"
    QUALIFICATION_RUN_GATE = "qualification_run_gate"
    QUALIFICATION_BUILD = "qualification_build"
    BENCH_SYNTHETIC = "bench_synthetic"


class EvalEngineStatus(StrEnum):
    """Terminal status the Python engine reports back to the Go facade."""

    SUCCEEDED = "succeeded"
    FAILED = "failed"
    INTERRUPTED = "interrupted"
    STOPPED = "stopped"


class EvalErrorCode(StrEnum):
    """Stable error code registry shared between Go and Python.

    The Go facade maps a Python code to a Go sentinel in one adapter.
    ``NONE`` is the zero value for the success case.
    """

    NONE = ""
    ENGINE_NOT_SET_UP = "engine_not_set_up"
    ENGINE_PROTOCOL_MISMATCH = "engine_protocol_mismatch"
    CONFIG_INVALID = "config_invalid"
    AUTHORITY_INVALID = "authority_invalid"
    LEASE_MISSING = "lease_missing"
    LEASE_INACTIVE = "lease_inactive"
    LEASE_EXPIRED = "lease_expired"
    LEASE_MISMATCHED = "lease_mismatched"
    LEASE_CONSUMED = "lease_consumed"
    CANDIDATE_DRIFT = "candidate_drift"
    INVENTORY_DRIFT = "inventory_drift"
    REPORT_ROOT_REUSED = "report_root_reused"
    EVIDENCE_KEY_INVALID = "evidence_key_invalid"
    PLATFORM_IDENTITY_UNAVAILABLE = "platform_identity_unavailable"
    PLATFORM_UNHEALTHY = "platform_unhealthy"
    PROVIDER_UNREACHABLE = "provider_unreachable"
    BUDGET_PREFLIGHT_FAILED = "budget_preflight_failed"
    CHILD_START_FAILED = "child_start_failed"
    CHILD_EXIT_NON_ZERO = "child_exit_non_zero"
    CHILD_INTERRUPTED = "child_interrupted"
    STATUS_RECONCILIATION_FAILED = "status_reconciliation_failed"


class EvalPlatformContext(BaseModel):
    """Platform-owned identity and paths the facade injects from the
    selected repository/runtime context. Configs never carry these
    fields; the facade is their only source.
    """

    repository_root: str
    eval_project: str
    g8e_binary_path: str
    g8e_binary_sha256: str
    platform_version: str
    auth_project_root: str
    runtime_dir: str
    trust_bundle_path: str
    gateway_http_url: str
    gateway_https_url: str
    ensemble_url: str


class EvalEngineFlags(BaseModel):
    """Non-secret engine flags. Secret values (provider API keys,
    evidence-key bytes) are never placed here; they remain in the
    supported environment or OS secret mechanism and are inherited by
    the child process environment.
    """

    json_output: bool = False
    verbose: bool = False
    idle_timeout_s: float = 0.0
    immediate_stop: bool = False


class EvalEngineRequest(BaseModel):
    """Typed contract the Go facade sends to the internal Python engine.

    The request is passed as a typed JSON file path argument, not
    positional reconstruction. The facade never constructs a giant shell
    vector.
    """

    schema_version: str
    operation: EvalOperation
    operation_id: str
    revision: str
    config_path: str
    lease_path: str = ""
    report_root: str = ""
    platform: EvalPlatformContext
    flags: EvalEngineFlags = Field(default_factory=EvalEngineFlags)


class EvalEngineResult(BaseModel):
    """Typed contract the Python engine returns to the Go facade.

    The facade translates ``error_code`` and ``error_stage`` into
    consistent g8e errors without parsing human prose. ``payload`` is an
    operation-specific typed JSON object selected by ``operation``.
    """

    schema_version: str
    operation: EvalOperation
    operation_id: str
    status: EvalEngineStatus
    error_code: EvalErrorCode = EvalErrorCode.NONE
    error_stage: str = ""
    safe_detail: str = ""
    payload: dict[str, Any] | None = None


def succeeded_result(
    request: EvalEngineRequest,
    payload: dict[str, Any] | None = None,
) -> EvalEngineResult:
    """Construct a succeeded result from a request and optional payload."""
    return EvalEngineResult(
        schema_version=EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
        operation=request.operation,
        operation_id=request.operation_id,
        status=EvalEngineStatus.SUCCEEDED,
        payload=payload,
    )


def failed_result(
    request: EvalEngineRequest,
    error_code: EvalErrorCode,
    error_stage: str,
    safe_detail: str = "",
    payload: dict[str, Any] | None = None,
) -> EvalEngineResult:
    """Construct a failed result from a request, error code, and stage."""
    return EvalEngineResult(
        schema_version=EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
        operation=request.operation,
        operation_id=request.operation_id,
        status=EvalEngineStatus.FAILED,
        error_code=error_code,
        error_stage=error_stage,
        safe_detail=safe_detail,
        payload=payload,
    )


def stopped_result(
    request: EvalEngineRequest,
    payload: dict[str, Any] | None = None,
) -> EvalEngineResult:
    """Construct a stopped result from a request and optional payload.

    A stopped result is a successful launcher execution (exit 0): the
    operator requested a graceful stop and the running loop honored it
    at a safe boundary. It is distinct from ``failed`` (a lifecycle
    defect) and ``succeeded`` (the full run completed).
    """
    return EvalEngineResult(
        schema_version=EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
        operation=request.operation,
        operation_id=request.operation_id,
        status=EvalEngineStatus.STOPPED,
        payload=payload,
    )


# Exit code mapping matching the Go facade's exit classification. The Go
# facade owns the authoritative mapping from typed Go sentinel errors to
# exit codes; this table is the Python-side reference for the engine's own
# process exit when invoked directly.
ERROR_CODE_EXIT_MAP: dict[EvalErrorCode, int] = {
    EvalErrorCode.NONE: 0,
    EvalErrorCode.ENGINE_NOT_SET_UP: 2,
    EvalErrorCode.CONFIG_INVALID: 3,
    EvalErrorCode.AUTHORITY_INVALID: 4,
    EvalErrorCode.LEASE_MISSING: 5,
    EvalErrorCode.LEASE_INACTIVE: 5,
    EvalErrorCode.LEASE_EXPIRED: 5,
    EvalErrorCode.LEASE_MISMATCHED: 5,
    EvalErrorCode.LEASE_CONSUMED: 5,
    EvalErrorCode.CANDIDATE_DRIFT: 6,
    EvalErrorCode.INVENTORY_DRIFT: 6,
    EvalErrorCode.REPORT_ROOT_REUSED: 7,
    EvalErrorCode.CHILD_START_FAILED: 8,
    EvalErrorCode.CHILD_EXIT_NON_ZERO: 9,
    EvalErrorCode.CHILD_INTERRUPTED: 10,
    EvalErrorCode.PROVIDER_UNREACHABLE: 11,
    EvalErrorCode.BUDGET_PREFLIGHT_FAILED: 12,
    EvalErrorCode.PLATFORM_IDENTITY_UNAVAILABLE: 13,
    EvalErrorCode.PLATFORM_UNHEALTHY: 14,
    EvalErrorCode.STATUS_RECONCILIATION_FAILED: 15,
    EvalErrorCode.ENGINE_PROTOCOL_MISMATCH: 1,
    EvalErrorCode.EVIDENCE_KEY_INVALID: 1,
}
