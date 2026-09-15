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
    LeaseIssueRequest,
    issue_lease,
)
from g8e_evals.lease_start_verification_cli import (
    LeaseStartVerificationRequest,
    verify_lease_for_start_request,
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


def _verification_request(
    config_path: str, lease_store_dir: str, repository_root: str
) -> LeaseStartVerificationRequest:
    return LeaseStartVerificationRequest(
        config_path=config_path,
        lease_store_dir=lease_store_dir,
        repository_root=repository_root,
        candidate=_candidate(),
        model_inventory_digest="5" * 64,
        command_family="campaign_run",
        command_version="1.0.0",
    )


def test_verify_passes_for_valid_active_lease(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    req = _verification_request(str(config_path), str(lease_store), str(tmp_path))
    result = verify_lease_for_start_request(req)

    assert result.verified is True
    assert result.operation_id == "test-diagnostic"
    assert result.revision == "rev-1"
    assert result.report_root == "reports/test-diagnostic"
    assert result.lease_id
    assert result.lease_path
    assert len(result.content_hash) == 64


def test_verify_fails_when_no_lease_exists(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"

    req = _verification_request(str(config_path), str(lease_store), str(tmp_path))
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "lease_missing"


def test_verify_fails_when_lease_is_stopped(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    from g8e_evals.lease_lifecycle import LeaseTransitionRequest, transition_lease

    transition_lease(
        LeaseTransitionRequest(
            config_path=str(config_path),
            lease_store_dir=str(lease_store),
            transition="stop",
        )
    )

    req = _verification_request(str(config_path), str(lease_store), str(tmp_path))
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "lease_stopped"


def test_verify_fails_when_lease_is_expired(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    from g8e_evals.lease_lifecycle import LeaseTransitionRequest, transition_lease

    transition_lease(
        LeaseTransitionRequest(
            config_path=str(config_path),
            lease_store_dir=str(lease_store),
            transition="expire",
        )
    )

    req = _verification_request(str(config_path), str(lease_store), str(tmp_path))
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "lease_expired"


def test_verify_fails_when_lease_is_completed(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    from g8e_evals.lease_lifecycle import LeaseTransitionRequest, transition_lease

    transition_lease(
        LeaseTransitionRequest(
            config_path=str(config_path),
            lease_store_dir=str(lease_store),
            transition="complete",
        )
    )

    req = _verification_request(str(config_path), str(lease_store), str(tmp_path))
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "lease_consumed"


def test_verify_fails_on_candidate_binary_drift(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    drifted = CandidateIdentity(
        source_tree_hash="1" * 64,
        execution_source_manifest_hash="2" * 64,
        binary_sha256="9" * 64,
        image_ids=[],
    )
    req = LeaseStartVerificationRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        repository_root=str(tmp_path),
        candidate=drifted,
        model_inventory_digest="5" * 64,
        command_family="campaign_run",
        command_version="1.0.0",
    )
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "candidate_binary_drift"


def test_verify_fails_on_model_inventory_drift(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    req = LeaseStartVerificationRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        repository_root=str(tmp_path),
        candidate=_candidate(),
        model_inventory_digest="9" * 64,
        command_family="campaign_run",
        command_version="1.0.0",
    )
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "model_inventory_drift"


def test_verify_fails_on_report_root_reused(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    report_root = tmp_path / "reports" / "test-diagnostic"
    report_root.mkdir(parents=True)

    req = _verification_request(str(config_path), str(lease_store), str(tmp_path))
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "report_root_reused"


def test_verify_fails_on_command_version_mismatch(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    req = LeaseStartVerificationRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        repository_root=str(tmp_path),
        candidate=_candidate(),
        model_inventory_digest="5" * 64,
        command_family="campaign_run",
        command_version="2.0.0",
    )
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "command_version_mismatch"


def test_verify_fails_on_command_family_mismatch(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    req = LeaseStartVerificationRequest(
        config_path=str(config_path),
        lease_store_dir=str(lease_store),
        repository_root=str(tmp_path),
        candidate=_candidate(),
        model_inventory_digest="5" * 64,
        command_family="controller_run",
        command_version="1.0.0",
    )
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "command_family_mismatch"


def test_complete_transition_produces_completed_status(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    from g8e_evals.lease_lifecycle import LeaseTransitionRequest, transition_lease

    result = transition_lease(
        LeaseTransitionRequest(
            config_path=str(config_path),
            lease_store_dir=str(lease_store),
            transition="complete",
        )
    )

    assert result.previous_status == LeaseStatus.ACTIVE
    assert result.new_status == LeaseStatus.COMPLETED


def test_verify_result_json_round_trip(tmp_path: Path) -> None:
    config_path = _write_config(tmp_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store)))

    req = _verification_request(str(config_path), str(lease_store), str(tmp_path))
    result = verify_lease_for_start_request(req)

    payload = result.model_dump_json(exclude_none=True)
    data = json.loads(payload)
    assert data["verified"] is True
    assert data["operation_id"] == "test-diagnostic"
    assert data["lease_id"]


def test_verify_fails_when_operation_deadline_exceeds_lease(tmp_path: Path) -> None:
    """A config whose ``max_duration_s`` pushes the operation deadline
    past the lease expiry must fail closed with
    ``lease_operation_deadline_exceeds_lease``.
    """
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
        stop_conditions=StopConditions(idle_timeout_s=180.0, max_duration_s=3600),
        model_variant_id="qwen3:4b",
        arm="direct",
    )
    config_path = tmp_path / "config.json"
    write_operation_config(config, config_path)
    lease_store = tmp_path / "leases"
    issue_lease(_issue_request(str(config_path), str(lease_store), expires_in=1800))

    req = _verification_request(str(config_path), str(lease_store), str(tmp_path))
    result = verify_lease_for_start_request(req)

    assert result.verified is False
    assert result.failure_code == "lease_operation_deadline_exceeds_lease"
