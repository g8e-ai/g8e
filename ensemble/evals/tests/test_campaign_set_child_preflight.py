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
    REPETITION_COUNT,
    TASKS_PER_CHILD,
    TOTAL_IFEVAL_TASKS,
    CampaignSetPlan,
    build_campaign_set_plan,
)
from g8e_evals.cli import (
    validate_campaign_set_child_preflight,
    validate_profile_cli_cross_check,
)
from g8e_evals.runner import (
    BudgetObservabilityPolicy,
    compute_budget_observability_policy_hash,
)
from g8e_evals.schema import ReportRole

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
        h1 = compute_campaign_manifest_hash(
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
            budget_observability_policy_hash=_VALID_HASH,
        )
        h2 = compute_campaign_manifest_hash(
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


# ---------------------------------------------------------------------------
# ReportRole.CHILD in _build_campaign_binding
# ---------------------------------------------------------------------------


class TestBuildCampaignBindingReportRole:
    """AUTH-2 re-audit: when a CampaignSetPlan is provided to the runner,
    _build_campaign_binding must emit ReportRole.CHILD and populate
    child_campaign_id/child_campaign_revision from the matching child
    plan. Without a plan, it emits ReportRole.SINGLE."""

    def _make_runner(
        self,
        *,
        campaign_id: str,
        campaign_set_plan: CampaignSetPlan | None = None,
    ) -> object:
        """Build a minimal CampaignRunner with profile+registry and an
        optional campaign_set_plan. Uses model_construct to bypass
        validation for test brevity."""
        from g8e_evals.profile import CampaignProfile, CampaignLifecycleStatus, CAMPAIGN_PROFILE_VERSION, TrackArmAssignment, ClaimBoundary
        from g8e_evals.registry import ModelRegistry, ModelVariant, WeightClass, PublicationEligibility, MODEL_REGISTRY_VERSION, compute_model_registry_hash
        from g8e_evals.runner import CampaignRunner, CampaignSpec
        from g8e_evals.campaign import (
            InitialStateAssignmentManifest,
            ModelCohort,
            RetryPolicy,
            RoleModelBinding,
            SamplingSettings,
            TaskAssignmentManifest,
            compute_initial_state_hash,
            compute_model_cohort_hash,
            compute_task_assignment_hash,
        )
        from g8e_evals.schema import CampaignTrack
        from g8e_evals.analysis.canonical import PreregistrationConfig

        variant = ModelVariant(
            variant_id="qwen3-8b-q4_0",
            canonical_display_name="Qwen3 8B",
            source_list_alias="qwen3:8b",
            hf_repo="Qwen/Qwen3-8B",
            hf_sha="a" * 40,
            retrieval_date="2026-09-01",
            license_id="Apache-2.0",
            license_text_hash=_VALID_HASH,
            gated=False,
            publication_eligibility=PublicationEligibility.ELIGIBLE,
            parameter_count=8_000_000_000,
            parameter_count_display="8.0B",
            architecture="QwenForCausalLM",
            model_type="qwen3",
            dtype="BF16",
            format="gguf",
            quantization="q4_0",
            context_length=32768,
            supported_modalities=["text"],
            reasoning_mode="non-reasoning",
            tool_call_support=True,
            chat_template_family="qwen3",
            chat_template_hash="d" * 64,
            tokenizer_digest="c" * 64,
            weight_class=WeightClass.HEAVY_SLM,
            backend_name="ollama",
            backend_version="0.1.48",
            served_model_tag="qwen3:8b",
            artifact_digest="b" * 64,
            artifact_bytes=8_000_000_000,
            tensor_format="gguf",
            hidden_reasoning_tokens=False,
        )
        registry = ModelRegistry(
            registry_id="registry-v1",
            registry_version="1",
            schema_version=MODEL_REGISTRY_VERSION,
            created_at="2026-09-01T00:00:00Z",
            variants=[variant],
            qualification_records=[],
            content_hash=compute_model_registry_hash("registry-v1", "1", [variant], []),
        )

        profile_temp = CampaignProfile.model_construct(
            campaign_id=campaign_id,
            campaign_revision="1",
            schema_version=CAMPAIGN_PROFILE_VERSION,
            purpose="test",
            created_at="2026-09-01T00:00:00Z",
            lifecycle_status=CampaignLifecycleStatus.FROZEN,
            generative_variant_ids=["qwen3-8b-q4_0"],
            benchmark_ids=["ifeval_subset"],
            dataset_hashes=["5" * 64],
            grader_hashes=["g" * 64],
            prompt_serialization_hash=_VALID_HASH,
            task_ids=["task-1001"],
            repetitions=1,
            track_arm_assignments=[TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct")],
            model_tier_assignments=[],
            baseline_tier_mappings={},
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
            hardware_identity="linux/amd64/cpu",
            environment_stratum="single-machine",
            primary_metrics=["ifeval_subset_verifier"],
            unit_of_analysis="task",
            claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
            model_registry_hash=registry.content_hash,
            content_hash="0" * 64,
        )
        from g8e_evals.profile import compute_campaign_profile_hash
        profile_content_hash = compute_campaign_profile_hash(profile_temp)
        profile = CampaignProfile(
            campaign_id=campaign_id,
            campaign_revision="1",
            schema_version=CAMPAIGN_PROFILE_VERSION,
            purpose="test",
            created_at="2026-09-01T00:00:00Z",
            lifecycle_status=CampaignLifecycleStatus.FROZEN,
            generative_variant_ids=["qwen3-8b-q4_0"],
            benchmark_ids=["ifeval_subset"],
            dataset_hashes=["5" * 64],
            grader_hashes=["g" * 64],
            prompt_serialization_hash=_VALID_HASH,
            task_ids=["task-1001"],
            repetitions=1,
            track_arm_assignments=[TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct")],
            model_tier_assignments=[],
            baseline_tier_mappings={},
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
            hardware_identity="linux/amd64/cpu",
            environment_stratum="single-machine",
            primary_metrics=["ifeval_subset_verifier"],
            unit_of_analysis="task",
            claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
            model_registry_hash=registry.content_hash,
            content_hash=profile_content_hash,
        )

        cohort = ModelCohort(
            cohort_id="cohort-qwen3-8b",
            role_bindings=[RoleModelBinding(
                role="primary",
                model_id="qwen3:8b",
                provider="ollama",
                endpoint="http://192.168.1.2:11434",
                sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
                timeout_seconds=120.0,
                seed_capable=True,
            )],
            content_hash=compute_model_cohort_hash("cohort-qwen3-8b", [RoleModelBinding(
                role="primary",
                model_id="qwen3:8b",
                provider="ollama",
                endpoint="http://192.168.1.2:11434",
                sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
                timeout_seconds=120.0,
                seed_capable=True,
            )]),
        )
        task_assignment = TaskAssignmentManifest(
            task_assignment_id="ta-v1",
            suite_id="ifeval_subset",
            dataset_hash="5" * 64,
            task_ids=["task-1001"],
            content_hash=compute_task_assignment_hash("ta-v1", "ifeval_subset", "5" * 64, ["task-1001"]),
        )
        initial_state = InitialStateAssignmentManifest(
            initial_state_assignment_id="no-initial-state-v1",
            state_type="no_initial_state",
            snapshot_hash="0" * 64,
            content_hash=compute_initial_state_hash("no-initial-state-v1", "no_initial_state", "0" * 64),
        )
        spec = CampaignSpec(
            campaign_id=campaign_id,
            release_version="v2.1.8",
            suite="ifeval_subset",
            suite_id="ifeval_subset",
            suite_version="1.0.0",
            dataset_hash="5" * 64,
            prompt_bundle_hash="p" * 64,
            grader_bundle_hash="g" * 64,
            preregistration=PreregistrationConfig(
                config_id="test",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=[],
                model_cohort_ids=["cohort-qwen3-8b"],
                task_assignment_id="ta-v1",
                initial_state_assignment_id="no-initial-state-v1",
                required_replicate_ids=["rep-1"],
                required_replicate_count=1,
                primary_metric_ids=["ifeval_subset_verifier"],
                continuous_test_policy="paired_t",
                bootstrap_count=10000,
                bootstrap_confidence=0.95,
                bootstrap_seed=0,
                significance_level=0.05,
                claim_policy="descriptive_only",
            ),
            cohorts=[cohort],
            task_assignment=task_assignment,
            initial_state=initial_state,
            retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]),
            randomization_seed=42,
        )
        return CampaignRunner(
            spec=spec,
            sut_factory=lambda c, a: None,
            tasks=[],
            grader=None,
            output_dir=Path("/tmp"),
            campaign_profile=profile,
            model_registry=registry,
            cohort_variant_map={"cohort-qwen3-8b": "qwen3-8b-q4_0"},
            campaign_set_plan=campaign_set_plan,
        )

    def test_single_report_role_without_plan(self):
        """_build_campaign_binding returns ReportRole.SINGLE when no
        campaign_set_plan is provided."""
        runner = self._make_runner(campaign_id="test-campaign")
        binding = runner._build_campaign_binding()
        assert binding is not None
        assert binding.report_role == ReportRole.SINGLE
        assert binding.child_campaign_id is None
        assert binding.child_campaign_revision is None

    def test_child_report_role_with_plan(self):
        """_build_campaign_binding returns ReportRole.CHILD and populates
        child_campaign_id/child_campaign_revision when a campaign_set_plan
        is provided and the campaign_id matches a child."""
        plan = _make_campaign_set_plan()
        child = plan.child_plans[0]
        runner = self._make_runner(
            campaign_id=child.child_id,
            campaign_set_plan=plan,
        )
        binding = runner._build_campaign_binding()
        assert binding is not None
        assert binding.report_role == ReportRole.CHILD
        assert binding.child_campaign_id == child.child_id
        assert binding.child_campaign_revision == child.child_revision

    def test_child_binding_raises_on_unknown_campaign_id(self):
        """_build_campaign_binding raises CampaignRunnerError when a
        campaign_set_plan is provided but the campaign_id does not match
        any child in the plan."""
        from g8e_evals.runner import CampaignRunnerError
        plan = _make_campaign_set_plan()
        runner = self._make_runner(
            campaign_id="nonexistent-child",
            campaign_set_plan=plan,
        )
        with pytest.raises(CampaignRunnerError, match="does not match any child"):
            runner._build_campaign_binding()
