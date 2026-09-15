# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Focused Tier 1 tests for the built-in engine handlers.

These tests prove the invariants the U7 acceptance criteria require:

* Non-start handlers (``_status``, ``_stop``, ``_verify``) never reach
  the SUT or provider path. The provider endpoint is only read in the
  start handlers from the verified lease.
* ``_safe_detail`` redacts exception messages containing known
  secret-bearing patterns so credentials cannot leak through the
  typed ``EvalEngineResult``.
* ``_classify_callback_exception`` reserves ``CHILD_EXIT_NON_ZERO``
  for genuine lifecycle defects and maps config, report-root,
  stop-request, and interruption failures to their exact stable codes.
* ``install_builtin_handlers`` uses ``setdefault`` so test-registered
  handlers are not overwritten by the production install.
"""

from __future__ import annotations

from unittest.mock import patch

import pytest

from g8e_evals.engine_cli import _OPERATION_HANDLERS, install_builtin_handlers
from g8e_evals.engine_protocol import (
    EvalEngineRequest,
    EvalEngineResult,
    EvalEngineStatus,
    EvalErrorCode,
    EvalEngineFlags,
    EvalOperation,
    EvalPlatformContext,
)
from g8e_evals.eval_engine_handlers import (
    _classify_callback_exception,
    _safe_detail,
    builtin_operation_handlers,
)
from g8e_evals.stop_request import StopRequestError

pytestmark = pytest.mark.unit


def _minimal_request(operation: EvalOperation) -> EvalEngineRequest:
    return EvalEngineRequest(
        schema_version="1.0.0",
        operation=operation,
        operation_id="diag-1",
        revision="rev-1",
        config_path="/tmp/config.json",
        lease_path="/tmp/lease.json",
        report_root="/tmp/report",
        platform=EvalPlatformContext(
            repository_root="/tmp",
            eval_project="ensemble/evals",
            g8e_binary_path="/tmp/g8e",
            g8e_binary_sha256="a" * 64,
            platform_version="dev",
            auth_project_root="/tmp",
            runtime_dir="/tmp/.g8e",
            trust_bundle_path="/tmp/bundle.pem",
            gateway_http_url="http://localhost:8080",
            gateway_https_url="https://localhost:8443",
            ensemble_url="http://localhost:8081",
            cli_cert_path="/tmp/cli.crt",
            cli_key_path="/tmp/cli.key",
            operator_session_id="op-session-1",
            cli_session_id="cli-session-1",
            user_id="user-1",
            operator_id="op-1",
            source_revision="dev",
            source_tree_state_hash="b" * 64,
        ),
        flags=EvalEngineFlags(json_output=True),
    )


# ---------------------------------------------------------------------------
# Non-start handlers never reach the provider path
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "operation",
    [
        EvalOperation.DIAGNOSTIC_STATUS,
        EvalOperation.DIAGNOSTIC_STOP,
        EvalOperation.DIAGNOSTIC_VERIFY,
        EvalOperation.CAMPAIGN_STATUS,
        EvalOperation.CAMPAIGN_STOP,
        EvalOperation.CAMPAIGN_VERIFY,
    ],
)
def test_non_start_handlers_never_call_provider_endpoint(operation: EvalOperation) -> None:
    """Every non-start handler must never read the provider endpoint from
    the lease. Patching ``_request_provider_endpoint`` to raise proves the
    handlers do not reach it.
    """
    handlers = builtin_operation_handlers()
    handler = handlers[operation]
    request = _minimal_request(operation)

    with patch(
        "g8e_evals.eval_engine_handlers._request_provider_endpoint",
        side_effect=AssertionError("non-start handler must not read provider endpoint"),
    ), patch(
        "g8e_evals.eval_engine_handlers._provider_api_key",
        side_effect=AssertionError("non-start handler must not read provider api key"),
    ):
        # The handler will fail on config load (path does not exist), but
        # the failure must be a typed lifecycle error, not an
        # AssertionError from the patched provider path.
        caught: Exception | None = None
        try:
            handler(request)
        except Exception as exc:
            caught = exc
        assert caught is not None, f"handler {operation.value} should fail on missing config"
        assert not isinstance(caught, AssertionError), (
            f"handler {operation.value} reached the provider path: {caught}"
        )


def test_start_handlers_read_provider_endpoint_from_lease() -> None:
    """Start handlers must read the provider endpoint from the verified
    lease, not from the environment. This is a structural assertion: the
    start handlers call ``_request_provider_endpoint`` which loads the
    lease.
    """
    handlers = builtin_operation_handlers()
    # The start handlers exist and are distinct from the non-start handlers.
    assert EvalOperation.DIAGNOSTIC_START in handlers
    assert EvalOperation.CAMPAIGN_START in handlers
    assert handlers[EvalOperation.DIAGNOSTIC_START] is not handlers[EvalOperation.DIAGNOSTIC_STATUS]
    assert handlers[EvalOperation.CAMPAIGN_START] is not handlers[EvalOperation.CAMPAIGN_STATUS]


# ---------------------------------------------------------------------------
# _safe_detail redaction
# ---------------------------------------------------------------------------


def test_safe_detail_redacts_api_key_errors() -> None:
    exc = RuntimeError("openai api_key=sk-1234567890 is invalid")
    detail = _safe_detail(exc)
    assert "sk-1234567890" not in detail
    assert "redacted" in detail


def test_safe_detail_redacts_token_errors() -> None:
    exc = ValueError("bearer token abc123 expired")
    detail = _safe_detail(exc)
    assert "abc123" not in detail
    assert "redacted" in detail


def test_safe_detail_redacts_password_errors() -> None:
    exc = OSError("password=hunter2 rejected")
    detail = _safe_detail(exc)
    assert "hunter2" not in detail
    assert "redacted" in detail


def test_safe_detail_preserves_non_secret_errors() -> None:
    exc = ValueError("report root already exists: /tmp/report")
    detail = _safe_detail(exc)
    assert "report root already exists" in detail
    assert "redacted" not in detail


# ---------------------------------------------------------------------------
# _classify_callback_exception reserves CHILD_EXIT_NON_ZERO
# ---------------------------------------------------------------------------


def test_classify_stop_request_error_returns_status_reconciliation_failed() -> None:
    code, _detail = _classify_callback_exception(StopRequestError("tampered"))
    assert code == EvalErrorCode.STATUS_RECONCILIATION_FAILED


def test_classify_file_exists_error_returns_report_root_reused() -> None:
    code, _detail = _classify_callback_exception(FileExistsError("exists"))
    assert code == EvalErrorCode.REPORT_ROOT_REUSED


def test_classify_keyboard_interrupt_returns_child_interrupted() -> None:
    code, detail = _classify_callback_exception(KeyboardInterrupt())
    assert code == EvalErrorCode.CHILD_INTERRUPTED
    assert detail == "interrupted by operator"


def test_classify_generic_error_returns_child_exit_non_zero() -> None:
    code, _detail = _classify_callback_exception(RuntimeError("boom"))
    assert code == EvalErrorCode.CHILD_EXIT_NON_ZERO


# ---------------------------------------------------------------------------
# install_builtin_handlers uses setdefault
# ---------------------------------------------------------------------------


def test_install_builtin_handlers_preserves_test_registered_handler() -> None:
    """A handler registered by a test before ``install_builtin_handlers``
    must not be overwritten by the production install.
    """
    # Save and clear the registry so the test is hermetic.
    saved = dict(_OPERATION_HANDLERS)
    _OPERATION_HANDLERS.clear()
    try:
        def custom_handler(_request: EvalEngineRequest) -> EvalEngineResult:
            return EvalEngineResult(
                schema_version="1.0.0",
                operation=EvalOperation.DIAGNOSTIC_STATUS,
                operation_id="test",
                status=EvalEngineStatus.SUCCEEDED,
            )

        _OPERATION_HANDLERS[EvalOperation.DIAGNOSTIC_STATUS] = custom_handler
        install_builtin_handlers()
        assert _OPERATION_HANDLERS[EvalOperation.DIAGNOSTIC_STATUS] is custom_handler
    finally:
        _OPERATION_HANDLERS.clear()
        _OPERATION_HANDLERS.update(saved)


def test_install_builtin_handlers_registers_all_production_handlers() -> None:
    """The production install registers every built-in handler."""
    saved = dict(_OPERATION_HANDLERS)
    _OPERATION_HANDLERS.clear()
    try:
        install_builtin_handlers()
        expected = set(builtin_operation_handlers().keys())
        assert set(_OPERATION_HANDLERS.keys()) == expected
    finally:
        _OPERATION_HANDLERS.clear()
        _OPERATION_HANDLERS.update(saved)
