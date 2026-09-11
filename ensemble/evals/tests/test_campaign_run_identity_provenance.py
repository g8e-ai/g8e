# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for campaign run CLI identity mismatch rejection and
source/build provenance enforcement.

Verifies that ``validate_campaign_identity`` rejects a mismatch between
``--campaign-id`` and ``campaign_profile.campaign_id``, rejects a
mismatch between the profile's ``task_ids`` and the loaded gold-set task
IDs (when no ``--task-offset``/``--task-limit`` slice is in use), and
that ``load_source_build_provenance_or_reject`` rejects
production-posture collection when source/build provenance is absent or
malformed.
"""

from __future__ import annotations

import pytest

from g8e_evals.cli import (
    load_source_build_provenance_or_reject,
    validate_campaign_identity,
)
from g8e_evals.preflight import PreflightError, PreflightFailureCode
from g8e_evals.profile import (
    CAMPAIGN_PROFILE_VERSION,
    CampaignLifecycleStatus,
    CampaignProfile,
    ClaimBoundary,
    ModelTierAssignment,
    TrackArmAssignment,
    compute_campaign_profile_hash,
)
from g8e_evals.schema import CampaignTrack


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _make_profile(
    *,
    campaign_id: str = "generative-campaign-v1",
    task_ids: list[str] | None = None,
) -> CampaignProfile:
    if task_ids is None:
        task_ids = ["task-1", "task-2", "task-3"]
    track_assignments = [
        TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
    ]
    tier_assignments = [
        ModelTierAssignment(variant_id="qwen3-8b-q4_0", target_tier="primary"),
    ]
    temp = CampaignProfile.model_construct(
        campaign_id=campaign_id,
        campaign_revision="1",
        schema_version=CAMPAIGN_PROFILE_VERSION,
        purpose="46-model generative comparison campaign",
        created_at="2026-09-10T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=["qwen3-8b-q4_0"],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_VALID_HASH],
        grader_hashes=[_VALID_HASH],
        prompt_serialization_hash=_VALID_HASH,
        task_ids=task_ids,
        repetitions=3,
        track_arm_assignments=track_assignments,
        model_tier_assignments=tier_assignments,
        baseline_tier_mappings={
            "primary": "qwen3:8b",
            "assistant": "granite3.3:8b",
            "lite": "qwen3:0.6b",
        },
        routing_policy="default",
        temperature=0.0,
        top_p=1.0,
        max_tokens=4096,
        seed=42,
        context_limit=32768,
        timeout_seconds=120.0,
        max_retries=1,
        warmup_excluded=True,
        concurrency=1,
        hardware_identity="linux/amd64/rtx-4090",
        environment_stratum="single-machine",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=_VALID_HASH,
        content_hash="0" * 64,
    )
    ch = compute_campaign_profile_hash(temp)
    return temp.model_copy(update={"content_hash": ch})


class TestValidateCampaignIdentity:
    """AUTH-2 instruction 4: reject child --campaign-id that differs from
    campaign_profile.campaign_id before report-directory creation, and
    validate that the profile's task_ids match the loaded gold-set task IDs
    when no task slice is in use."""

    def test_matched_campaign_id_and_task_ids_passes(self):
        profile = _make_profile(campaign_id="generative-campaign-v1", task_ids=["task-1", "task-2", "task-3"])
        # Should not raise
        validate_campaign_identity(
            campaign_id="generative-campaign-v1",
            campaign_profile=profile,
            gold_set_task_ids=["task-1", "task-2", "task-3"],
            task_slice_in_use=False,
        )

    def test_mismatched_campaign_id_rejected(self):
        profile = _make_profile(campaign_id="generative-campaign-v1")
        with pytest.raises(ValueError, match="campaign_id"):
            validate_campaign_identity(
                campaign_id="different-campaign-id",
                campaign_profile=profile,
                gold_set_task_ids=["task-1", "task-2", "task-3"],
                task_slice_in_use=False,
            )

    def test_task_id_mismatch_rejected_without_slice(self):
        profile = _make_profile(task_ids=["task-1", "task-2", "task-3"])
        with pytest.raises(ValueError, match="task"):
            validate_campaign_identity(
                campaign_id="generative-campaign-v1",
                campaign_profile=profile,
                gold_set_task_ids=["task-1", "task-2", "task-4"],
                task_slice_in_use=False,
            )

    def test_task_id_count_mismatch_rejected_without_slice(self):
        profile = _make_profile(task_ids=["task-1", "task-2", "task-3"])
        with pytest.raises(ValueError, match="task"):
            validate_campaign_identity(
                campaign_id="generative-campaign-v1",
                campaign_profile=profile,
                gold_set_task_ids=["task-1", "task-2"],
                task_slice_in_use=False,
            )

    def test_task_id_order_mismatch_rejected_without_slice(self):
        profile = _make_profile(task_ids=["task-1", "task-2", "task-3"])
        with pytest.raises(ValueError, match="task"):
            validate_campaign_identity(
                campaign_id="generative-campaign-v1",
                campaign_profile=profile,
                gold_set_task_ids=["task-3", "task-2", "task-1"],
                task_slice_in_use=False,
            )

    def test_task_id_mismatch_skipped_with_slice(self):
        """When a task slice is in use, the task_id check is skipped
        because the slice intentionally selects a subset."""
        profile = _make_profile(task_ids=["task-1", "task-2", "task-3", "task-4", "task-5"])
        # Should not raise even though gold_set_task_ids differ
        validate_campaign_identity(
            campaign_id="generative-campaign-v1",
            campaign_profile=profile,
            gold_set_task_ids=["task-1", "task-2", "task-3"],
            task_slice_in_use=True,
        )

    def test_matched_campaign_id_with_slice_passes(self):
        profile = _make_profile(campaign_id="generative-campaign-v1", task_ids=["task-1", "task-2", "task-3"])
        validate_campaign_identity(
            campaign_id="generative-campaign-v1",
            campaign_profile=profile,
            gold_set_task_ids=["task-1"],
            task_slice_in_use=True,
        )


class TestLoadSourceBuildProvenanceOrReject:
    """AUTH-2 instruction 5: reject production-posture collection when
    source/build provenance is absent or malformed."""

    def test_valid_provenance_from_env(self, monkeypatch):
        monkeypatch.setenv("G8E_EVALS_SOURCE_REVISION", "abc123def456")
        monkeypatch.setenv("G8E_EVALS_SOURCE_TREE_STATE_HASH", _VALID_HASH)
        provenance = load_source_build_provenance_or_reject(is_production_posture=True)
        assert provenance is not None
        assert provenance.source_revision == "abc123def456"
        assert provenance.source_tree_state_hash == _VALID_HASH

    def test_missing_provenance_rejected_for_production_posture(self, monkeypatch):
        monkeypatch.delenv("G8E_EVALS_SOURCE_REVISION", raising=False)
        monkeypatch.delenv("G8E_EVALS_SOURCE_TREE_STATE_HASH", raising=False)
        with pytest.raises(PreflightError) as exc_info:
            load_source_build_provenance_or_reject(is_production_posture=True)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_REVISION_MISSING

    def test_missing_tree_state_hash_rejected_for_production_posture(self, monkeypatch):
        monkeypatch.setenv("G8E_EVALS_SOURCE_REVISION", "abc123def456")
        monkeypatch.delenv("G8E_EVALS_SOURCE_TREE_STATE_HASH", raising=False)
        with pytest.raises(PreflightError) as exc_info:
            load_source_build_provenance_or_reject(is_production_posture=True)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_TREE_STATE_HASH_MISSING

    def test_malformed_tree_state_hash_rejected(self, monkeypatch):
        monkeypatch.setenv("G8E_EVALS_SOURCE_REVISION", "abc123def456")
        monkeypatch.setenv("G8E_EVALS_SOURCE_TREE_STATE_HASH", "not-a-valid-hash")
        with pytest.raises(PreflightError) as exc_info:
            load_source_build_provenance_or_reject(is_production_posture=True)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_TREE_STATE_HASH_INVALID

    def test_empty_provenance_rejected_for_production_posture(self, monkeypatch):
        monkeypatch.setenv("G8E_EVALS_SOURCE_REVISION", "")
        monkeypatch.setenv("G8E_EVALS_SOURCE_TREE_STATE_HASH", "")
        with pytest.raises(PreflightError) as exc_info:
            load_source_build_provenance_or_reject(is_production_posture=True)
        assert exc_info.value.code == PreflightFailureCode.SOURCE_REVISION_MISSING

    def test_missing_provenance_allowed_for_non_production_posture(self, monkeypatch):
        monkeypatch.delenv("G8E_EVALS_SOURCE_REVISION", raising=False)
        monkeypatch.delenv("G8E_EVALS_SOURCE_TREE_STATE_HASH", raising=False)
        provenance = load_source_build_provenance_or_reject(is_production_posture=False)
        assert provenance is None

    def test_valid_provenance_optional_fields_populated(self, monkeypatch):
        monkeypatch.setenv("G8E_EVALS_SOURCE_REVISION", "abc123def456")
        monkeypatch.setenv("G8E_EVALS_SOURCE_TREE_STATE_HASH", _VALID_HASH)
        monkeypatch.setenv("G8E_EVALS_BUILD_ID", "build-42")
        monkeypatch.setenv("G8E_EVALS_BUILD_SYSTEM", "github-actions")
        monkeypatch.setenv("G8E_EVALS_CI_RUN_ID", "run-99")
        monkeypatch.setenv("G8E_EVALS_CI_URL", "https://example.com/run/99")
        provenance = load_source_build_provenance_or_reject(is_production_posture=True)
        assert provenance is not None
        assert provenance.build_id == "build-42"
        assert provenance.build_system == "github-actions"
        assert provenance.ci_run_id == "run-99"
        assert provenance.ci_url == "https://example.com/run/99"
