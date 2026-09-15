# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest

from g8e_evals.lease_verification import (
    LeaseVerificationContext,
    LeaseVerificationError,
    LeaseVerificationFailureCode,
    verify_lease_for_start,
)
from g8e_evals.live_operations_authority import (
    ArtifactBinding,
    BudgetAuthority,
    CandidateIdentity,
    CommandFamily,
    LeaseStatus,
    LiveOperationLease,
    LiveOperationLeaseTemplate,
    OperationKind,
    build_budget_authority,
    build_live_operation_lease,
    build_live_operation_lease_template,
    compute_request_digest,
)

pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64
_OTHER_HASH = "b" * 64
_NOW = datetime(2026, 9, 13, 12, 3, tzinfo=UTC)


def _budget() -> BudgetAuthority:
    return build_budget_authority(
        operation_id="governed-inference-smoke",
        max_requests=3,
        max_tokens_per_request=16_384,
        max_tokens=49_152,
        max_usd=0.0,
        concurrency=1,
        min_free_disk_bytes=10_000_000_000,
        max_duration_seconds=1080,
        max_retries=0,
        max_replacements=0,
    )


def _template() -> LiveOperationLeaseTemplate:
    return build_live_operation_lease_template(
        template_id="governed-inference-smoke-v1",
        operation_kind=OperationKind.GOVERNED_INFERENCE_SMOKE,
        operation_authority_hash=_VALID_HASH,
        budget=_budget(),
        endpoint="http://192.168.1.2:11434",
        permitted_command_family=CommandFamily.GOVERNED_INFERENCE_SMOKE,
        required_runtime_authority_names=["governed_inference_smoke"],
    )


def _candidate() -> CandidateIdentity:
    return CandidateIdentity(
        source_tree_hash="1" * 64,
        execution_source_manifest_hash="2" * 64,
        binary_sha256="3" * 64,
        image_ids=["sha256:" + "4" * 64],
    )


def _lease(
    status: LeaseStatus = LeaseStatus.ACTIVE,
    request_digest: str | None = None,
) -> LiveOperationLease:
    issued_at = datetime(2026, 9, 13, 12, 0, tzinfo=UTC)
    digest = request_digest or compute_request_digest(
        operation_config_content_hash=_VALID_HASH,
        operation_id="governed-inference-smoke",
        command_family=CommandFamily.GOVERNED_INFERENCE_SMOKE,
        command_version="1.0.0",
    )
    return build_live_operation_lease(
        lease_id="lease-1",
        template=_template(),
        candidate=_candidate(),
        model_inventory_digest="5" * 64,
        operation_identity="governed-inference-smoke",
        runtime_authorities=[
            ArtifactBinding(
                name="governed_inference_smoke", path="live-packet/smoke.json", sha256=_VALID_HASH
            )
        ],
        request_digest=digest,
        command_family=CommandFamily.GOVERNED_INFERENCE_SMOKE,
        command_version="1.0.0",
        report_root="reports/governed-inference-smoke-v1",
        app_identity="spiffe://g8e.local/app/g8ee",
        operator_session_identity="spiffe://g8e.local/operator/org/operator/session",
        issued_at=issued_at,
        start_deadline=issued_at + timedelta(minutes=5),
        expires_at=issued_at + timedelta(minutes=18),
        status=status,
    )


def _ctx(**overrides: object) -> LeaseVerificationContext:
    defaults: dict[str, object] = {
        "operation_config_content_hash": _VALID_HASH,
        "operation_id": "governed-inference-smoke",
        "command_family": CommandFamily.GOVERNED_INFERENCE_SMOKE,
        "command_version": "1.0.0",
        "candidate": _candidate(),
        "model_inventory_digest": "5" * 64,
        "report_root_exists": False,
        "now": _NOW,
    }
    defaults.update(overrides)
    return LeaseVerificationContext(**defaults)  # type: ignore[arg-type]


def test_verify_passes_for_valid_active_lease() -> None:
    verify_lease_for_start(_lease(), _ctx())


def test_verify_fails_when_lease_is_none() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(None, _ctx())
    assert exc_info.value.code == LeaseVerificationFailureCode.LEASE_MISSING


def test_verify_fails_for_completed_lease() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(status=LeaseStatus.COMPLETED), _ctx())
    assert exc_info.value.code == LeaseVerificationFailureCode.LEASE_CONSUMED


def test_verify_fails_for_stopped_lease() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(status=LeaseStatus.STOPPED), _ctx())
    assert exc_info.value.code == LeaseVerificationFailureCode.LEASE_STOPPED


def test_verify_fails_for_expired_lease_status() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(status=LeaseStatus.EXPIRED), _ctx())
    assert exc_info.value.code == LeaseVerificationFailureCode.LEASE_EXPIRED


def test_verify_fails_for_draft_lease() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(status=LeaseStatus.DRAFT), _ctx())
    assert exc_info.value.code == LeaseVerificationFailureCode.LEASE_INACTIVE


def test_verify_fails_when_now_before_issued_at() -> None:
    early = datetime(2026, 9, 13, 11, 0, tzinfo=UTC)
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(now=early))
    assert exc_info.value.code == LeaseVerificationFailureCode.LEASE_NOT_YET_VALID


def test_verify_fails_when_now_after_expires_at() -> None:
    late = datetime(2026, 9, 13, 12, 30, tzinfo=UTC)
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(now=late))
    assert exc_info.value.code == LeaseVerificationFailureCode.LEASE_EXPIRED


def test_verify_fails_when_start_deadline_passed() -> None:
    past_deadline = datetime(2026, 9, 13, 12, 6, tzinfo=UTC)
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(now=past_deadline))
    assert exc_info.value.code == LeaseVerificationFailureCode.LEASE_EXPIRED


def test_verify_fails_on_request_digest_mismatch() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(operation_config_content_hash=_OTHER_HASH))
    assert exc_info.value.code == LeaseVerificationFailureCode.REQUEST_DIGEST_MISMATCH


def test_verify_fails_on_command_family_mismatch() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(command_family=CommandFamily.CAMPAIGN_RUN))
    assert exc_info.value.code == LeaseVerificationFailureCode.COMMAND_FAMILY_MISMATCH


def test_verify_fails_on_command_version_mismatch() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(command_version="2.0.0"))
    assert exc_info.value.code == LeaseVerificationFailureCode.COMMAND_VERSION_MISMATCH


def test_verify_fails_on_operation_identity_mismatch() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(operation_id="other-operation"))
    assert exc_info.value.code == LeaseVerificationFailureCode.OPERATION_IDENTITY_MISMATCH


def test_verify_fails_on_source_tree_drift() -> None:
    drifted = CandidateIdentity(
        source_tree_hash="9" * 64,
        execution_source_manifest_hash="2" * 64,
        binary_sha256="3" * 64,
        image_ids=["sha256:" + "4" * 64],
    )
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(candidate=drifted))
    assert exc_info.value.code == LeaseVerificationFailureCode.CANDIDATE_SOURCE_TREE_DRIFT


def test_verify_fails_on_binary_drift() -> None:
    drifted = CandidateIdentity(
        source_tree_hash="1" * 64,
        execution_source_manifest_hash="2" * 64,
        binary_sha256="9" * 64,
        image_ids=["sha256:" + "4" * 64],
    )
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(candidate=drifted))
    assert exc_info.value.code == LeaseVerificationFailureCode.CANDIDATE_BINARY_DRIFT


def test_verify_fails_on_image_drift() -> None:
    drifted = CandidateIdentity(
        source_tree_hash="1" * 64,
        execution_source_manifest_hash="2" * 64,
        binary_sha256="3" * 64,
        image_ids=["sha256:" + "9" * 64],
    )
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(candidate=drifted))
    assert exc_info.value.code == LeaseVerificationFailureCode.CANDIDATE_IMAGE_DRIFT


def test_verify_fails_on_model_inventory_drift() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(model_inventory_digest="9" * 64))
    assert exc_info.value.code == LeaseVerificationFailureCode.MODEL_INVENTORY_DRIFT


def test_verify_fails_on_report_root_reused() -> None:
    with pytest.raises(LeaseVerificationError) as exc_info:
        verify_lease_for_start(_lease(), _ctx(report_root_exists=True))
    assert exc_info.value.code == LeaseVerificationFailureCode.REPORT_ROOT_REUSED
