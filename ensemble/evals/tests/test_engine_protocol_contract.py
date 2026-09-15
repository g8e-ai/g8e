# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Cross-language contract test for the eval engine protocol.

Asserts that the Python protocol constants in ``g8e_evals.engine_protocol``
match the Go constants in ``internal/models/eval_engine.go``. The same
expected registry is pinned in the Go contract test
``internal/models/eval_engine_contract_test.go`` so that a change on
either side fails the test on the other.

Also exercises the engine_cli dispatch framework: request loading,
schema-version validation, unregistered-operation handling, and result
emission.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from g8e_evals.engine_cli import (
    ProtocolMismatchError,
    dispatch,
    load_request,
    validate_schema_version,
)
from g8e_evals.engine_protocol import (
    EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
    ERROR_CODE_EXIT_MAP,
    EvalErrorCode,
    EvalEngineRequest,
    EvalEngineResult,
    EvalEngineStatus,
    EvalOperation,
    EvalPlatformContext,
    failed_result,
    succeeded_result,
)

pytestmark = pytest.mark.unit

# The expected registry pinned in lockstep with the Go contract test.
# If either side adds, removes, or renames a value, both tests fail.
_EXPECTED_SCHEMA_VERSION = "1.0.0"

_EXPECTED_OPERATIONS = {
    "diagnostic_start",
    "diagnostic_status",
    "diagnostic_stop",
    "diagnostic_verify",
    "campaign_start",
    "campaign_status",
    "campaign_stop",
    "campaign_verify",
    "campaign_publish",
    "campaign_set_plan",
    "campaign_set_validate",
    "campaign_set_verify",
    "controller_run",
    "controller_status",
    "controller_stop",
    "controller_recover",
    "bundle",
    "verify",
    "verify_receipts",
    "publish",
    "qualification_hash_source",
    "qualification_candidate",
    "qualification_collect_runtime",
    "qualification_run_gate",
    "qualification_build",
    "bench_synthetic",
}

_EXPECTED_ERROR_CODES = {
    "",
    "engine_not_set_up",
    "engine_protocol_mismatch",
    "config_invalid",
    "authority_invalid",
    "lease_missing",
    "lease_inactive",
    "lease_expired",
    "lease_mismatched",
    "lease_consumed",
    "candidate_drift",
    "inventory_drift",
    "report_root_reused",
    "evidence_key_invalid",
    "platform_identity_unavailable",
    "platform_unhealthy",
    "provider_unreachable",
    "budget_preflight_failed",
    "child_start_failed",
    "child_exit_non_zero",
    "child_interrupted",
    "status_reconciliation_failed",
}

_EXPECTED_STATUSES = {
    "succeeded",
    "failed",
    "interrupted",
    "stopped",
}

# The platform context field set pinned in lockstep with the Go contract
# test ``TestEvalPlatformContextFieldContract`` in
# ``internal/models/eval_engine_contract_test.go``. If either side adds,
# removes, or renames a field, both tests fail.
_EXPECTED_PLATFORM_FIELDS = {
    "repository_root",
    "eval_project",
    "g8e_binary_path",
    "g8e_binary_sha256",
    "platform_version",
    "auth_project_root",
    "runtime_dir",
    "trust_bundle_path",
    "gateway_http_url",
    "gateway_https_url",
    "ensemble_url",
    "cli_cert_path",
    "cli_key_path",
    "operator_session_id",
    "cli_session_id",
    "user_id",
    "operator_id",
    "source_revision",
    "source_tree_state_hash",
}


def test_schema_version_matches_go_contract():
    assert EVAL_ENGINE_REQUEST_SCHEMA_VERSION == _EXPECTED_SCHEMA_VERSION


def test_operation_enum_matches_go_contract():
    python_ops = {op.value for op in EvalOperation}
    assert python_ops == _EXPECTED_OPERATIONS


def test_error_code_enum_matches_go_contract():
    python_codes = {code.value for code in EvalErrorCode}
    assert python_codes == _EXPECTED_ERROR_CODES


def test_engine_status_enum_matches_go_contract():
    python_statuses = {status.value for status in EvalEngineStatus}
    assert python_statuses == _EXPECTED_STATUSES


def test_platform_context_fields_match_go_contract():
    assert set(EvalPlatformContext.model_fields) == _EXPECTED_PLATFORM_FIELDS


def test_operation_enum_is_exhaustive_and_unique():
    values = [op.value for op in EvalOperation]
    assert len(values) == len(set(values)), "EvalOperation values must be unique"


def test_error_code_enum_is_exhaustive_and_unique():
    values = [code.value for code in EvalErrorCode]
    assert len(values) == len(set(values)), "EvalErrorCode values must be unique"


def _sample_request(
    operation: EvalOperation = EvalOperation.BUNDLE,
    schema_version: str = EVAL_ENGINE_REQUEST_SCHEMA_VERSION,
) -> EvalEngineRequest:
    return EvalEngineRequest(
        schema_version=schema_version,
        operation=operation,
        operation_id="test-op-001",
        revision="rev-001",
        config_path="/tmp/config.json",
        lease_path="/tmp/lease.json",
        report_root="/tmp/report",
        platform=EvalPlatformContext(
            repository_root="/repo",
            eval_project="/repo/ensemble/evals",
            g8e_binary_path="/repo/g8e",
            g8e_binary_sha256="abc123",
            platform_version="2.1.0",
            auth_project_root="/repo",
            runtime_dir="/repo/.g8e",
            trust_bundle_path="/repo/.g8e/pki/trust/bundle.pem",
            gateway_http_url="http://127.0.0.1:8080",
            gateway_https_url="https://127.0.0.1:8443",
            ensemble_url="http://127.0.0.1:8000",
            cli_cert_path="/repo/.g8e/cli.crt",
            cli_key_path="/repo/.g8e/cli.key",
            operator_session_id="op-session-1",
            cli_session_id="cli-session-1",
            user_id="user-1",
            operator_id="op-1",
            source_revision="deadbeef",
            source_tree_state_hash="a" * 64,
        ),
    )


def test_request_round_trip_json():
    original = _sample_request()
    raw = original.model_dump_json(by_alias=True)
    restored = EvalEngineRequest.model_validate_json(raw)
    assert restored.operation == original.operation
    assert restored.operation_id == original.operation_id
    assert restored.platform.repository_root == original.platform.repository_root


def test_result_round_trip_json():
    result = succeeded_result(_sample_request(), payload={"count": 42})
    raw = result.model_dump_json(by_alias=True, exclude_none=True)
    restored = EvalEngineResult.model_validate_json(raw)
    assert restored.status == EvalEngineStatus.SUCCEEDED
    assert restored.payload == {"count": 42}


def test_failed_result_has_error_code_and_stage():
    request = _sample_request()
    result = failed_result(
        request,
        error_code=EvalErrorCode.LEASE_MISSING,
        error_stage="preflight",
        safe_detail="no active lease for operation",
    )
    assert result.status == EvalEngineStatus.FAILED
    assert result.error_code == EvalErrorCode.LEASE_MISSING
    assert result.error_stage == "preflight"
    assert result.safe_detail == "no active lease for operation"


def test_validate_schema_version_accepts_matching():
    request = _sample_request()
    validate_schema_version(request)


def test_validate_schema_version_rejects_mismatch():
    request = _sample_request(schema_version="0.0.1")
    with pytest.raises(ProtocolMismatchError):
        validate_schema_version(request)


def test_dispatch_unregistered_operation_returns_typed_failed_result():
    request = _sample_request(operation=EvalOperation.BUNDLE)
    result = dispatch(request)
    assert result.status == EvalEngineStatus.FAILED
    assert result.error_stage == "dispatch"
    assert "not yet wired" in result.safe_detail


def test_load_request_reads_valid_json(tmp_path: Path):
    request = _sample_request()
    path = tmp_path / "request.json"
    path.write_text(request.model_dump_json(by_alias=True))
    loaded = load_request(str(path))
    assert loaded.operation == request.operation
    assert loaded.operation_id == request.operation_id


def test_load_request_raises_on_missing_file():
    with pytest.raises(FileNotFoundError):
        load_request("/nonexistent/path/to/request.json")


def test_load_request_raises_on_invalid_json(tmp_path: Path):
    path = tmp_path / "bad.json"
    path.write_text("not json at all")
    with pytest.raises(json.JSONDecodeError):
        load_request(str(path))


def test_exit_code_map_covers_all_error_codes():
    for code in EvalErrorCode:
        assert code in ERROR_CODE_EXIT_MAP, f"missing exit code for {code.value!r}"


def test_exit_code_map_succeeded_is_zero():
    assert ERROR_CODE_EXIT_MAP[EvalErrorCode.NONE] == 0
