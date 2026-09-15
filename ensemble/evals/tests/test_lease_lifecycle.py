# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json
from pathlib import Path

import pytest

from g8e_evals.lease_lifecycle import (
    LeaseCommandFamily,
    LeaseInspectRequest,
    LeaseIssueRequest,
    LeaseLifecycleError,
    LeaseTransitionRequest,
    inspect_lease,
    issue_lease,
    transition_lease,
)
from g8e_evals.live_operations_authority import (
    CandidateIdentity,
    LeaseStatus,
    OperationKind,
)
from g8e_evals.operation_config import (
    AuthorityRef,
    BudgetCeilings,
    DiagnosticConfig,
    EvidenceKeyRef,
    ProviderEndpointRef,
    StopConditions,
    write_operation_config,
)

pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _candidate() -> CandidateIdentity:
    return CandidateIdentity(
        source_tree_hash="1" * 64,
        execution_source_manifest_hash="2" * 64,
        binary_sha256="3" * 64,
        image_ids=[],
    )


def _write_config(tmp_path: Path) -> Path:
    config = DiagnosticConfig(
        operation_id="test-diagnostic",
        revision="rev-1",
        suite="ifeval_subset",
        seed=42,
        report_root="reports/test-diagnostic",
        gold_set=AuthorityRef(path="authorities/gold.json", sha256=_VALID_HASH),
        evidence_key=EvidenceKeyRef(path="keys/eval.key", key_id="eval-key-1"),
        provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="remote"),
        budget=BudgetCeilings(max_requests=10, max_tokens=10000, max_usd=0.0),
        stop_conditions=StopConditions(idle_timeout_s=180.0),
        model_variant_id="qwen3:4b",
        arm="direct",
    )
    config_path = tmp_path / "config.json"
    write_operation_config(config, config_path)
    return config_path


def _issue_request(
    config_path: str, lease_store_dir: str, expires_in: int = 1800
) -> LeaseIssueRequest:
    return LeaseIssueRequest(
        config_path=config_path,
        lease_store_dir=lease_store_dir,
        command_family=LeaseCommandFamily.CAMPAIGN_RUN,
        command_version="1.0.0",
        candidate=_candidate(),
        model_inventory_digest="5" * 64,
        endpoint="http://192.168.1.2:11434",
        app_identity="spiffe://g8e.local/app/g8ee",
        operator_session_identity="spiffe://g8e.local/operator/org/operator/session",
        expires_in_seconds=expires_in,
        start_deadline_seconds=300,
        operation_kind=OperationKind.EMBEDDED_AUTHORITY_DIAGNOSTIC,
        required_runtime_authority_names=["gold_set", "evidence_key"],
    )


def test_issue_creates_active_lease(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))

    result = issue_lease(req)

    assert result.status == LeaseStatus.ACTIVE
    assert result.lease_id.startswith("test-diagnostic-")
    assert (lease_store / f"{result.lease_id}.json").exists()
    assert len(result.content_hash) == 64
    assert len(result.request_digest) == 64
    assert result.operation_config_content_hash


def test_issue_enforces_one_active_lease(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))

    issue_lease(req)

    with pytest.raises(LeaseLifecycleError, match="active_lease_exists"):
        issue_lease(req)


def test_issue_rejects_existing_lease_for_same_config(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))
    issue_lease(req)

    transition_req = LeaseTransitionRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        transition="stop",
    )
    transition_lease(transition_req)

    with pytest.raises(LeaseLifecycleError, match="lease_already_issued"):
        issue_lease(req)


def test_inspect_returns_lease_when_found(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))
    issue_lease(req)

    inspect_req = LeaseInspectRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
    )
    result = inspect_lease(inspect_req)

    assert result.found is True
    assert result.lease is not None
    assert result.lease.status == LeaseStatus.ACTIVE


def test_inspect_returns_not_found_when_no_lease(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"

    inspect_req = LeaseInspectRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
    )
    result = inspect_lease(inspect_req)

    assert result.found is False
    assert result.lease is None


def test_stop_transitions_active_to_stopped(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))
    issue_lease(req)

    transition_req = LeaseTransitionRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        transition="stop",
    )
    result = transition_lease(transition_req)

    assert result.previous_status == LeaseStatus.ACTIVE
    assert result.new_status == LeaseStatus.STOPPED

    inspect_req = LeaseInspectRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
    )
    inspect_result = inspect_lease(inspect_req)
    assert inspect_result.lease is not None
    assert inspect_result.lease.status == LeaseStatus.STOPPED


def test_expire_transitions_active_to_expired(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))
    issue_lease(req)

    transition_req = LeaseTransitionRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        transition="expire",
    )
    result = transition_lease(transition_req)

    assert result.previous_status == LeaseStatus.ACTIVE
    assert result.new_status == LeaseStatus.EXPIRED


def test_stop_rejects_already_stopped_lease(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))
    issue_lease(req)

    transition_req = LeaseTransitionRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        transition="stop",
    )
    transition_lease(transition_req)

    with pytest.raises(LeaseLifecycleError, match="lease_not_active"):
        transition_lease(transition_req)


def test_stop_rejects_nonexistent_lease(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"

    transition_req = LeaseTransitionRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        transition="stop",
    )
    with pytest.raises(LeaseLifecycleError, match="lease_not_found"):
        transition_lease(transition_req)


def test_stopped_lease_does_not_block_new_lease_for_different_config(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))
    issue_lease(req)

    transition_req = LeaseTransitionRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        transition="stop",
    )
    transition_lease(transition_req)

    other_config = DiagnosticConfig(
        operation_id="other-diagnostic",
        revision="rev-1",
        suite="ifeval_subset",
        seed=42,
        report_root="reports/other-diagnostic",
        gold_set=AuthorityRef(path="authorities/gold.json", sha256=_VALID_HASH),
        evidence_key=EvidenceKeyRef(path="keys/eval.key", key_id="eval-key-1"),
        provider_endpoint=ProviderEndpointRef(provider="ollama", endpoint_class="remote"),
        budget=BudgetCeilings(max_requests=10, max_tokens=10000, max_usd=0.0),
        stop_conditions=StopConditions(idle_timeout_s=180.0),
        model_variant_id="qwen3:4b",
        arm="direct",
    )
    other_config_path = tmp_path / "other-config.json"
    write_operation_config(other_config, other_config_path)

    other_req = _issue_request(str(other_config_path), str(lease_store))
    result = issue_lease(other_req)

    assert result.status == LeaseStatus.ACTIVE


def test_lease_file_is_valid_json_with_content_hash(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    req = _issue_request(str(config_path), str(lease_store))

    result = issue_lease(req)

    lease_path = lease_store / f"{result.lease_id}.json"
    data = json.loads(lease_path.read_text())
    assert data["content_hash"] == result.content_hash
    assert data["status"] == "active"
    assert data["request_digest"] == result.request_digest
    assert data["command_version"] == "1.0.0"


def test_inspect_missing_config_raises_typed_error(tmp_path: Path) -> None:
    """Passing a lease ID (or any non-config path) where a config path is
    expected must produce a typed ``config_not_found`` error, not a raw
    ``FileNotFoundError`` traceback.
    """
    lease_store = tmp_path / "leases"
    inspect_req = LeaseInspectRequest(
        config_path=str(tmp_path / "diag-u7-acceptance-009-20260915"),
        lease_store_dir=str(lease_store),
    )
    with pytest.raises(LeaseLifecycleError, match="config_not_found") as exc_info:
        inspect_lease(inspect_req)
    assert exc_info.value.code == "config_not_found"


def test_issue_missing_config_raises_typed_error(tmp_path: Path) -> None:
    """Issue against a missing config must produce a typed
    ``config_not_found`` error, not a raw ``FileNotFoundError`` traceback.
    """
    lease_store = tmp_path / "leases"
    req = _issue_request(str(tmp_path / "nonexistent-config.json"), str(lease_store))
    with pytest.raises(LeaseLifecycleError, match="config_not_found") as exc_info:
        issue_lease(req)
    assert exc_info.value.code == "config_not_found"


def test_transition_missing_config_raises_typed_error(tmp_path: Path) -> None:
    """Stop/expire/complete against a missing config must produce a typed
    ``config_not_found`` error, not a raw ``FileNotFoundError`` traceback.
    """
    lease_store = tmp_path / "leases"
    transition_req = LeaseTransitionRequest(
        config_path=str(tmp_path / "nonexistent-config.json"),
        lease_store_dir=str(lease_store),
        transition="stop",
    )
    with pytest.raises(LeaseLifecycleError, match="config_not_found") as exc_info:
        transition_lease(transition_req)
    assert exc_info.value.code == "config_not_found"
