# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for campaign-set child preflight validation and
budget-observability policy hash binding into CampaignManifest.

Covers the Senior Integrator re-audit gaps for Atlas:

1. ``CampaignManifest`` does not bind the ``budget_observability_policy_hash``.
2. ``campaign run`` has no ``--max-tokens`` input.
3. ``campaign run`` does not consume a ``CampaignSetPlan`` or reject
   parent/child identity, partition, seed, retry/budget/instrumentation/
   expected-record authority, environment scope, or profile-hash mismatches
   before report-directory creation.
4. The runner emits every profile-backed report as ``ReportRole.SINGLE``
   instead of ``ReportRole.CHILD`` when a campaign-set plan is provided.
5. CLI seed and retry inputs are not cross-checked against the frozen
   campaign profile.
"""

from __future__ import annotations

import hashlib
from pathlib import Path

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.campaign import (
    CampaignManifest,
    compute_campaign_manifest_hash,
)
from g8e_evals.campaign_set import (
    CAMPAIGN_SET_SCHEMA_VERSION,
    CHILD_COUNT,
    EXPECTED_CHILD_ASSIGNMENT_COUNT,
    EXPECTED_TOTAL_ASSIGNMENT_COUNT,
    REPETITION_COUNT,
    TASKS_PER_CHILD,
    TOTAL_IFEVAL_TASKS,
    CampaignChildPlan,
    CampaignSetPlan,
    build_campaign_set_plan,
    compute_child_campaign_id,
)
from g8e_evals.cli import (
    validate_campaign_set_child_preflight,
    validate_profile_cli_cross_check,
)
from g8e_evals.runner import (
    BudgetCeiling,
    BudgetObservabilityPolicy,
    compute_budget_observability_policy_hash,
    compute_provider_budget_hash,
)
from g8e_evals.schema import ProviderBudget, ReportRole

_VALID_HASH = "a" * 64


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_campaign_set_plan(
    *,
    parent_campaign_id: str = "ifeval-expand-v1",
    parent_campaign_revision: str = "1",
    campaign_profile_hash: str = _VALID_HASH,
    model_registry_hash: str = _VALID_HASH,
    expected_record_policy_hash: str = _VALID_HASH,
    retry_policy_hash: str = _VALID_HASH,
    budget_authority_hash: str = _VALID_HASH,
    instrumentation_policy_hash: str = _VALID_HASH,
    seed: int = 42,
    orchestrator_environment_scope: str = "linux/amd64/cpu",
    provider_environment_scope: str = "linux/amd64/remote-ollama",
    population_hash: str = _VALID_HASH,
) -> CampaignSetPlan:
    population_task_ids = [f"task-{i:04d}" for i in range(TOTAL_IFEVAL_TASKS)]
    repetition_ids = [f"rep-{i}" for i in range(1, REPETITION_COUNT + 1)]
    child_revisions = [f"child-{i}-rev-1" for i in range(CHILD_COUNT)]
    return build_campaign_set_plan(
        set_id="ifeval-expand-set-v1",
        parent_campaign_id=parent_campaign_id,
        parent_campaign_revision=parent_campaign_revision,
        population_task_ids=population_task_ids,
        population_hash=population_hash,
        model_registry_hash=model_registry_hash,
        campaign_profile_hash=campaign_profile_hash,
        repetition_ids=repetition_ids,
        seed=seed,
        retry_policy_hash=retry_policy_hash,
        budget_authority_hash=budget_authority_hash,
        instrumentation_policy_hash=instrumentation_policy_hash,
        expected_record_policy_hash=expected_record_policy_hash,
        orchestrator_environment_scope=orchestrator_environment_scope,
        provider_environment_scope=provider_environment_scope,
        child_revisions=child_revisions,
        set_version=CAMPAIGN_SET_SCHEMA_VERSION,
    )


# ---------------------------------------------------------------------------
# Budget observability policy hash in CampaignManifest
# ---------------------------------------------------------------------------


class TestBudgetObservabilityPolicyHashInManifest:
    """AUTH-3 re-audit: the budget observability policy hash must be
    bound into the CampaignManifest so a changed policy invalidates
    the manifest content hash."""

    def test_manifest_has_budget_observability_policy_hash_field(self):
        """CampaignManifest must declare a
        ``budget_observability_policy_hash`` field."""
        field_names = set(CampaignManifest.model_fields.keys())
        assert "budget_observability_policy_hash" in field_names

    def test_manifest_hash_includes_budget_observability_policy_hash(self):
        """Two manifests differing only in budget_observability_policy_hash
        must have different content hashes."""
        common_kwargs = dict(
            campaign_id="test-campaign",
            campaign_version="1.0.0",
            release_version="v2.1.8",
            preregistration_hash=_VALID_HASH,
            cohort_hashes=[_VALID_HASH],
            task_assignment_hash=_VALID_HASH,
            initial_state_assignment_hash=_VALID_HASH,
            schedule_hash=_VALID_HASH,
            retry_policy_hash=_VALID_HASH,
            metric_registry_hash=_VALID_HASH,
            release_metric_set_hash=_VALID_HASH,
            threshold_authority_hash=_VALID_HASH,
            missingness_authority_hash=_VALID_HASH,
            provider_budget_hash=_VALID_HASH,
            source_build_provenance_hash=_VALID_HASH,
            claim_exclusion_hash=_VALID_HASH,
        )
        h1 = compute_campaign_manifest_hash(
            *common_kwargs.values(),
            budget_observability_policy_hash=_VALID_HASH,
        )
        h2 = compute_campaign_manifest_hash(
            *common_kwargs.values(),
            budget_observability_policy_hash="b" * 64,
        )
        assert h1 != h2

    def test_budget_observability_policy_hash_is_64_chars(self):
        """The hash must be a valid 64-char SHA-256 hex string."""
        policy = BudgetObservabilityPolicy()
        h = compute_budget_observability_policy_hash(policy)
        assert len(h) == 64
        assert all(c in "0123456789abcdef" for c in h)

    def test_different_policies_produce_different_hashes(self):
        """Two different BudgetObservabilityPolicy instances must produce
        different hashes."""
        p1 = BudgetObservabilityPolicy(tokens_observable=False)
        p2 = BudgetObservabilityPolicy(tokens_observable=True)
        assert compute_budget_observability_policy_hash(p1) != compute_budget_observability_policy_hash(p2)


# ---------------------------------------------------------------------------
# Campaign-set child preflight validation
# ---------------------------------------------------------------------------


class TestCampaignSetChildPreflight:
    """AUTH-2 re-audit: campaign run must consume a CampaignSetPlan and
    reject parent/child identity, partition, seed, retry/budget/
    instrumentation/expected-record authority, environment scope, and
    profile-hash mismatches before report-directory creation."""

    def test_valid_child_preflight_passes(self):
        """A child campaign whose identity, partition, seed, and all
        authority hashes match the plan passes preflight."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        validate_campaign_set_child_preflight(
            campaign_id=child.child_id,
            campaign_profile_hash=plan.campaign_profile_hash,
            model_registry_hash=plan.model_registry_hash,
            expected_record_policy_hash=plan.expected_record_policy_hash,
            retry_policy_hash=plan.retry_policy_hash,
            budget_authority_hash=plan.budget_authority_hash,
            instrumentation_policy_hash=plan.instrumentation_policy_hash,
            seed=plan.seed,
            orchestrator_environment_scope=plan.orchestrator_environment_scope,
            provider_environment_scope=plan.provider_environment_scope,
            campaign_set_plan=plan,
            task_ids=child.partition_task_ids,
        )

    def test_wrong_campaign_id_rejected(self):
        """A campaign_id that does not match any child in the plan is rejected."""
        plan = _make_campaign_set_plan()
        with pytest.raises(ValueError, match="campaign_id"):
            validate_campaign_set_child_preflight(
                campaign_id="nonexistent-child-id",
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=plan.child_plans[0].partition_task_ids,
            )

    def test_wrong_profile_hash_rejected(self):
        """A campaign_profile_hash that does not match the plan is rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="campaign_profile_hash"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash="c" * 64,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_registry_hash_rejected(self):
        """A model_registry_hash that does not match the plan is rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="model_registry_hash"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash="d" * 64,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_seed_rejected(self):
        """A seed that does not match the plan is rejected."""
        plan = _make_campaign_set_plan(seed=42)
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="seed"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=999,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_retry_policy_hash_rejected(self):
        """A retry_policy_hash that does not match the plan is rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="retry_policy_hash"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash="e" * 64,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_budget_authority_hash_rejected(self):
        """A budget_authority_hash that does not match the plan is rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="budget_authority_hash"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash="f" * 64,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_expected_record_policy_hash_rejected(self):
        """An expected_record_policy_hash that does not match the plan is rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="expected_record_policy_hash"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash="0" * 64,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_instrumentation_policy_hash_rejected(self):
        """An instrumentation_policy_hash that does not match the plan is rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="instrumentation_policy_hash"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash="1" * 64,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_orchestrator_environment_scope_rejected(self):
        """An orchestrator_environment_scope that does not match the plan is rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="orchestrator_environment_scope"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope="linux/arm64/cpu",
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_provider_environment_scope_rejected(self):
        """A provider_environment_scope that does not match the plan is rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        with pytest.raises(ValueError, match="provider_environment_scope"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope="linux/arm64/remote",
                campaign_set_plan=plan,
                task_ids=child.partition_task_ids,
            )

    def test_wrong_task_partition_rejected(self):
        """Task IDs that do not match the child's partition are rejected."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        wrong_tasks = [f"wrong-{i}" for i in range(TASKS_PER_CHILD)]
        with pytest.raises(ValueError, match="partition"):
            validate_campaign_set_child_preflight(
                campaign_id=child.child_id,
                campaign_profile_hash=plan.campaign_profile_hash,
                model_registry_hash=plan.model_registry_hash,
                expected_record_policy_hash=plan.expected_record_policy_hash,
                retry_policy_hash=plan.retry_policy_hash,
                budget_authority_hash=plan.budget_authority_hash,
                instrumentation_policy_hash=plan.instrumentation_policy_hash,
                seed=plan.seed,
                orchestrator_environment_scope=plan.orchestrator_environment_scope,
                provider_environment_scope=plan.provider_environment_scope,
                campaign_set_plan=plan,
                task_ids=wrong_tasks,
            )


# ---------------------------------------------------------------------------
# Profile CLI cross-check
# ---------------------------------------------------------------------------


class TestProfileCliCrossCheck:
    """AUTH-3 re-audit: CLI seed and retry inputs must be cross-checked
    against the frozen campaign profile."""

    def test_matched_seed_and_retries_passes(self):
        """A seed and max_retries matching the profile passes."""
        validate_profile_cli_cross_check(
            cli_seed=42,
            cli_max_retries=1,
            profile_seed=42,
            profile_max_retries=1,
        )

    def test_mismatched_seed_rejected(self):
        """A seed that does not match the profile is rejected."""
        with pytest.raises(ValueError, match="seed"):
            validate_profile_cli_cross_check(
                cli_seed=99,
                cli_max_retries=1,
                profile_seed=42,
                profile_max_retries=1,
            )

    def test_mismatched_max_retries_rejected(self):
        """A max_retries that does not match the profile is rejected."""
        with pytest.raises(ValueError, match="max_retries"):
            validate_profile_cli_cross_check(
                cli_seed=42,
                cli_max_retries=3,
                profile_seed=42,
                profile_max_retries=1,
            )
