# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for cohort-stratified and assignment-aware canonical analysis.

These tests verify that the canonical analysis engine stratifies metric
results, confusion matrices, domain strata, gate decisions, and paired
comparisons by model cohort when campaign records are present. They also
verify campaign assignment validation, terminal outcome selection per
assignment, retry chain handling, descriptive-only claim policy
enforcement, and authority hash binding.

No external dependencies (no files, network, or DB).
"""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis.canonical import (
    ClaimPolicy,
    ContinuousTestPolicy,
    GateDecisionStatus,
    MetricAnalysisResult,
    PairedComparison,
    PreregistrationConfig,
)
from g8e_evals.analysis.engine import compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.arms import Arm
from g8e_evals.campaign import (
    CampaignAssignment,
    CampaignManifest,
    ExecutionSchedule,
    InitialStateAssignmentManifest,
    ModelCohort,
    RetryPolicy,
    RoleModelBinding,
    SamplingSettings,
    TaskAssignmentManifest,
    compute_assignment_id,
    compute_campaign_manifest_hash,
    compute_initial_state_hash,
    compute_model_cohort_hash,
    compute_retry_policy_hash,
    compute_schedule_hash,
    compute_task_assignment_hash,
)
from g8e_evals.index import ModelRole
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY, MetricDirection
from g8e_evals.release_metric_set import MetricDomain
from g8e_evals.schema import (
    AttemptRecord,
    GraderReference,
    MetricObservation,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_RUN_ID = "run-cohort-test-1"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_IDS = ["cohort-qwen3-8b", "cohort-granite-3.3-8b"]
_ARM_IDS = ["direct", "ensemble_ungoverned"]
_REPLICATE_IDS = ["replicate-1", "replicate-2"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_METRIC_ID = "ifeval_subset_verifier"
_METRIC_VERSION = "1.0.0"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_sampling() -> SamplingSettings:
    return SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42)


def _make_role_binding(role: ModelRole = ModelRole.PRIMARY, model_id: str = "qwen3:8b") -> RoleModelBinding:
    return RoleModelBinding(
        role=role,
        model_id=model_id,
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=_make_sampling(),
        timeout_seconds=120.0,
        seed_capable=True,
    )


def _make_cohort(cohort_id: str = "cohort-qwen3-8b", model_id: str = "qwen3:8b") -> ModelCohort:
    bindings = [_make_role_binding(ModelRole.PRIMARY, model_id)]
    variant_id = cohort_id[len("cohort-"):] if cohort_id.startswith("cohort-") else cohort_id
    ch = compute_model_cohort_hash(cohort_id, variant_id, ModelRole.PRIMARY, bindings)
    return ModelCohort(
        cohort_id=cohort_id,
        candidate_variant_id=variant_id,
        candidate_role=ModelRole.PRIMARY,
        role_bindings=bindings,
        content_hash=ch,
    )


def _make_task_assignment(task_ids: list[str] | None = None) -> TaskAssignmentManifest:
    if task_ids is None:
        task_ids = _TASK_IDS
    ch = compute_task_assignment_hash("task-assignment-v1", "ifeval_subset", _DATASET_HASH, task_ids)
    return TaskAssignmentManifest(
        task_assignment_id="task-assignment-v1",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=task_ids,
        content_hash=ch,
    )


def _make_initial_state() -> InitialStateAssignmentManifest:
    ch = compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH)
    return InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=ch,
    )


def _make_all_assignments(
    task_ids: list[str] | None = None,
    cohort_ids: list[str] | None = None,
    arm_ids: list[str] | None = None,
    replicate_ids: list[str] | None = None,
) -> list[CampaignAssignment]:
    if task_ids is None:
        task_ids = _TASK_IDS
    if cohort_ids is None:
        cohort_ids = _COHORT_IDS
    if arm_ids is None:
        arm_ids = _ARM_IDS
    if replicate_ids is None:
        replicate_ids = _REPLICATE_IDS
    assignments: list[CampaignAssignment] = []
    pos = 0
    for task_id in task_ids:
        for cohort_id in cohort_ids:
            for arm_id in arm_ids:
                for replicate_id in replicate_ids:
                    aid = compute_assignment_id(
                        _CAMPAIGN_ID, task_id, cohort_id, arm_id, _INITIAL_STATE_ID, replicate_id,
                    )
                    assignments.append(CampaignAssignment(
                        campaign_id=_CAMPAIGN_ID,
                        assignment_id=aid,
                        task_id=task_id,
                        model_cohort_id=cohort_id,
                        arm_id=arm_id,
                        initial_state_assignment_id=_INITIAL_STATE_ID,
                        replicate_id=replicate_id,
                        schedule_position=pos,
                    ))
                    pos += 1
    return assignments


def _make_schedule(assignments: list[CampaignAssignment]) -> ExecutionSchedule:
    ordered_ids: list[str] = [""] * len(assignments)
    for a in assignments:
        ordered_ids[a.schedule_position] = a.assignment_id
    ch = compute_schedule_hash("schedule-v1", _CAMPAIGN_ID, 42, "shuffle", "1.0.0", ordered_ids)
    return ExecutionSchedule(
        schedule_id="schedule-v1",
        campaign_id=_CAMPAIGN_ID,
        randomization_seed=42,
        algorithm_id="shuffle",
        algorithm_version="1.0.0",
        ordered_assignment_ids=ordered_ids,
        content_hash=ch,
    )


def _make_retry_policy() -> RetryPolicy:
    return RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"])


def _make_campaign_manifest(
    prereg_hash: str = "a" * 64,
    cohort_hashes: list[str] | None = None,
) -> CampaignManifest:
    if cohort_hashes is None:
        cohort_hashes = [_make_cohort(c).content_hash for c in _COHORT_IDS]
    ch = compute_campaign_manifest_hash(
        campaign_id=_CAMPAIGN_ID,
        campaign_version="1.0.0",
        release_version=_RELEASE_VERSION,
        preregistration_hash=prereg_hash,
        cohort_hashes=cohort_hashes,
        task_assignment_hash=_make_task_assignment().content_hash,
        initial_state_assignment_hash=_make_initial_state().content_hash,
        schedule_hash=_make_schedule(_make_all_assignments()).content_hash,
        retry_policy_hash=compute_retry_policy_hash(1, ["infrastructure_failed"]),
        metric_registry_hash="m" * 64,
        release_metric_set_hash="r" * 64,
        threshold_authority_hash="t" * 64,
        missingness_authority_hash="n" * 64,
        provider_budget_hash="p" * 64,
        source_build_provenance_hash="s" * 64,
        claim_exclusion_hash="c" * 64,
        budget_observability_policy_hash="b" * 64,
    )
    return CampaignManifest(
        campaign_id=_CAMPAIGN_ID,
        campaign_version="1.0.0",
        release_version=_RELEASE_VERSION,
        preregistration_hash=prereg_hash,
        cohort_hashes=cohort_hashes,
        task_assignment_hash=_make_task_assignment().content_hash,
        initial_state_assignment_hash=_make_initial_state().content_hash,
        schedule_hash=_make_schedule(_make_all_assignments()).content_hash,
        retry_policy_hash=compute_retry_policy_hash(1, ["infrastructure_failed"]),
        metric_registry_hash="m" * 64,
        release_metric_set_hash="r" * 64,
        threshold_authority_hash="t" * 64,
        missingness_authority_hash="n" * 64,
        provider_budget_hash="p" * 64,
        source_build_provenance_hash="s" * 64,
        claim_exclusion_hash="c" * 64,
        budget_observability_policy_hash="b" * 64,
        content_hash=ch,
    )


def _make_preregistration(
    cohort_ids: list[str] | None = None,
    arm_ids: list[str] | None = None,
    replicate_ids: list[str] | None = None,
    claim_policy: ClaimPolicy = ClaimPolicy.DESCRIPTIVE_ONLY,
) -> PreregistrationConfig:
    if cohort_ids is None:
        cohort_ids = _COHORT_IDS
    if arm_ids is None:
        arm_ids = _ARM_IDS
    if replicate_ids is None:
        replicate_ids = _REPLICATE_IDS
    return PreregistrationConfig(
        config_id="test-config-1",
        config_version="1.0.0",
        baseline_arm_id=arm_ids[0],
        comparison_arm_ids=arm_ids[1:],
        model_cohort_ids=cohort_ids,
        task_assignment_id="task-assignment-v1",
        initial_state_assignment_id=_INITIAL_STATE_ID,
        required_replicate_ids=replicate_ids,
        required_replicate_count=len(replicate_ids),
        primary_metric_ids=[_METRIC_ID],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=claim_policy,
    )


def _make_task(task_id: str = "task-1001") -> TaskDefinition:
    return TaskDefinition(
        task_id=task_id,
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        prompt_hash="abc123",
        prompt_length=10,
        expected_action_class="IFEVAL",
        compatible_arms=[Arm.DIRECT, Arm.ENSEMBLE_UNGOVERNED],
        graders=[GraderReference(grader_id=_METRIC_ID, grader_version=_METRIC_VERSION)],
    )


def _make_attempt(
    attempt_id: str,
    assignment_id: str,
    task_id: str,
    arm_id: Arm,
    cohort_id: str,
    replicate_id: str,
    terminal_status: TerminalStatus = TerminalStatus.COMPLETED,
    parent_attempt_id: str | None = None,
) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=task_id,
        arm_id=arm_id,
        model_cohort_id=cohort_id,
        state_snapshot_hash=_NO_STATE_HASH,
        replicate_id=replicate_id,
        assignment_id=assignment_id,
        terminal_status=terminal_status,
        parent_attempt_id=parent_attempt_id,
    )


def _make_metric_obs(
    attempt_id: str,
    task_id: str,
    arm_id: Arm,
    value: float | None = 1.0,
) -> MetricObservation:
    definition = DEFAULT_METRIC_REGISTRY.get(_METRIC_ID, _METRIC_VERSION)
    return MetricObservation(
        metric_id=_METRIC_ID,
        metric_version=_METRIC_VERSION,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        arm_id=arm_id,
        task_id=task_id,
        value=value,
        unit=definition.unit,
        eligible=True,
        verification_status=VerificationStatus.VERIFIED,
        grader_class=definition.grader_class,
    )


def _make_campaign_record(
    attempts: list[AttemptRecord],
    observations: list[MetricObservation],
    tasks: list[TaskDefinition] | None = None,
    assignments: list[CampaignAssignment] | None = None,
    preregistration: PreregistrationConfig | None = None,
    cohorts: list[ModelCohort] | None = None,
    campaign_manifest: CampaignManifest | None = None,
) -> AnalysisInputRecord:
    if tasks is None:
        tasks = [_make_task(tid) for tid in _TASK_IDS]
    if assignments is None:
        assignments = _make_all_assignments()
    if preregistration is None:
        preregistration = _make_preregistration()
    if cohorts is None:
        cohorts = [_make_cohort(c) for c in _COHORT_IDS]
    if campaign_manifest is None:
        campaign_manifest = _make_campaign_manifest()
    return AnalysisInputRecord(
        run_id=_RUN_ID,
        release_version=_RELEASE_VERSION,
        tasks=tasks,
        attempts=attempts,
        metric_observations=observations,
        campaign_manifest=campaign_manifest,
        model_cohorts=cohorts,
        campaign_assignments=assignments,
        execution_schedule=_make_schedule(assignments),
        retry_policy=_make_retry_policy(),
        preregistration=preregistration,
    )


def _make_terminal_attempts_for_all_assignments(
    assignments: list[CampaignAssignment] | None = None,
    values: dict[str, float] | None = None,
    terminal_status: TerminalStatus = TerminalStatus.COMPLETED,
) -> tuple[list[AttemptRecord], list[MetricObservation]]:
    """Create one terminal attempt per assignment with metric observations.

    Returns (attempts, observations). Each attempt is linked to its
    assignment by assignment_id. The values dict maps assignment_id to
    metric value; defaults to 1.0 for all.
    """
    if assignments is None:
        assignments = _make_all_assignments()
    if values is None:
        values = {}
    attempts: list[AttemptRecord] = []
    observations: list[MetricObservation] = []
    for i, a in enumerate(assignments):
        arm = Arm(a.arm_id)
        att_id = f"att-{i}"
        attempt = _make_attempt(
            attempt_id=att_id,
            assignment_id=a.assignment_id,
            task_id=a.task_id,
            arm_id=arm,
            cohort_id=a.model_cohort_id,
            replicate_id=a.replicate_id,
            terminal_status=terminal_status,
        )
        attempts.append(attempt)
        if terminal_status == TerminalStatus.COMPLETED:
            val = values.get(a.assignment_id, 1.0)
            observations.append(_make_metric_obs(
                attempt_id=att_id,
                task_id=a.task_id,
                arm_id=arm,
                value=val,
            ))
    return attempts, observations


# ---------------------------------------------------------------------------
# Cohort-stratified metric results
# ---------------------------------------------------------------------------


class TestCohortStratifiedMetricResults:
    """Metric results must be stratified by model cohort."""

    def test_metric_result_has_model_cohort_id_field(self) -> None:
        """MetricAnalysisResult model has a model_cohort_id field."""
        # This will fail until the field is added to the model.
        mr = MetricAnalysisResult(
            metric_id=_METRIC_ID,
            metric_version=_METRIC_VERSION,
            arm_id="direct",
            domain=MetricDomain.UTILITY,
            direction=MetricDirection.BINARY_PASS_FAIL,
            unit="boolean",
            numerator=1.0,
            denominator=1,
            value=1.0,
            eligible_count=1,
            not_eligible_count=0,
            missing_count=0,
            evidence_ref_count=0,
            model_cohort_id="cohort-qwen3-8b",
        )
        assert mr.model_cohort_id == "cohort-qwen3-8b"

    def test_two_cohorts_produce_separate_metric_results(self) -> None:
        """Two cohorts with the same task, arm, state, and replicate produce separate metric results, not one pooled result."""
        assignments = _make_all_assignments()
        attempts, observations = _make_terminal_attempts_for_all_assignments(assignments)
        record = _make_campaign_record(attempts, observations, assignments=assignments)
        analysis = compute_canonical_analysis_from_record(record)

        ri_results = [r for r in analysis.metric_results if r.metric_id == _METRIC_ID]
        # Two cohorts x two arms = 4 metric results (one per cohort per arm)
        assert len(ri_results) == 4, (
            f"Expected 4 metric results (2 cohorts x 2 arms), got {len(ri_results)}. "
            f"Currently the engine pools cohorts into one result per arm."
        )
        # Each result must carry its cohort identity
        cohort_arm_pairs = {(r.model_cohort_id, r.arm_id) for r in ri_results}
        expected_pairs = {
            ("cohort-qwen3-8b", "direct"),
            ("cohort-qwen3-8b", "ensemble_ungoverned"),
            ("cohort-granite-3.3-8b", "direct"),
            ("cohort-granite-3.3-8b", "ensemble_ungoverned"),
        }
        assert cohort_arm_pairs == expected_pairs, (
            f"Expected cohort-arm pairs {expected_pairs}, got {cohort_arm_pairs}"
        )

    def test_metric_results_sorted_by_cohort_then_arm(self) -> None:
        """Metric results are sorted by (metric_id, metric_version, model_cohort_id, arm_id)."""
        assignments = _make_all_assignments()
        attempts, observations = _make_terminal_attempts_for_all_assignments(assignments)
        record = _make_campaign_record(attempts, observations, assignments=assignments)
        analysis = compute_canonical_analysis_from_record(record)

        ri_results = [r for r in analysis.metric_results if r.metric_id == _METRIC_ID]
        sort_keys = [(r.metric_id, r.metric_version, r.model_cohort_id, r.arm_id) for r in ri_results]
        assert sort_keys == sorted(sort_keys), (
            f"Metric results not sorted by (metric_id, metric_version, model_cohort_id, arm_id): {sort_keys}"
        )


# ---------------------------------------------------------------------------
# Campaign assignment validation
# ---------------------------------------------------------------------------


class TestCampaignAssignmentValidation:
    """The engine validates the exact Cartesian product before computing metrics."""

    def test_missing_assignment_rejected(self) -> None:
        """When campaign records are present, a missing assignment slot is rejected."""
        assignments = _make_all_assignments()
        # Remove one assignment to create a missing slot
        incomplete_assignments = assignments[:-1]
        attempts, observations = _make_terminal_attempts_for_all_assignments(incomplete_assignments)
        record = _make_campaign_record(
            attempts, observations, assignments=incomplete_assignments,
        )
        with pytest.raises(ValueError, match="missing campaign assignment"):
            compute_canonical_analysis_from_record(record)

    def test_extra_assignment_rejected(self) -> None:
        """When campaign records are present, an extra assignment slot is rejected."""
        assignments = _make_all_assignments()
        # Add a duplicate-slot assignment with a different assignment_id
        extra = CampaignAssignment(
            campaign_id=_CAMPAIGN_ID,
            assignment_id="extra-assignment-id",
            task_id=_TASK_IDS[0],
            model_cohort_id=_COHORT_IDS[0],
            arm_id=_ARM_IDS[0],
            initial_state_assignment_id=_INITIAL_STATE_ID,
            replicate_id=_REPLICATE_IDS[0],
            schedule_position=len(assignments),
        )
        extra_assignments = [*assignments, extra]
        attempts, observations = _make_terminal_attempts_for_all_assignments(extra_assignments)
        record = _make_campaign_record(
            attempts, observations, assignments=extra_assignments,
        )
        with pytest.raises(ValueError, match="unexpected campaign assignment"):
            compute_canonical_analysis_from_record(record)


# ---------------------------------------------------------------------------
# Terminal outcome selection
# ---------------------------------------------------------------------------


class TestTerminalOutcomeSelection:
    """One terminal outcome per assignment; retry attempts do not inflate denominators."""

    def test_retry_chain_counts_terminal_only(self) -> None:
        """When an assignment has a retry chain, only the terminal attempt is counted in the denominator."""
        assignments = _make_all_assignments()
        attempts, observations = _make_terminal_attempts_for_all_assignments(assignments)
        # Add a retry attempt for the first assignment
        first_assignment = assignments[0]
        arm = Arm(first_assignment.arm_id)
        retry_attempt = _make_attempt(
            attempt_id="att-retry-0",
            assignment_id=first_assignment.assignment_id,
            task_id=first_assignment.task_id,
            arm_id=arm,
            cohort_id=first_assignment.model_cohort_id,
            replicate_id=first_assignment.replicate_id,
            terminal_status=TerminalStatus.INFRASTRUCTURE_FAILED,
            parent_attempt_id=attempts[0].attempt_id,
        )
        attempts.append(retry_attempt)
        record = _make_campaign_record(attempts, observations, assignments=assignments)
        analysis = compute_canonical_analysis_from_record(record)

        # The denominator for the first assignment's (cohort, arm) should be
        # the number of assignments for that (cohort, arm), not the number
        # of attempts. With 2 tasks x 2 replicates = 4 assignments per
        # (cohort, arm), the denominator should be 4, not 5.
        ri_results = [
            r for r in analysis.metric_results
            if r.metric_id == _METRIC_ID
            and r.model_cohort_id == first_assignment.model_cohort_id
            and r.arm_id == first_assignment.arm_id
        ]
        assert len(ri_results) == 1
        result = ri_results[0]
        assert result.denominator == 4, (
            f"Expected denominator 4 (one per assignment), got {result.denominator}. "
            f"Retry attempts must not inflate the denominator."
        )

    def test_failed_terminal_outcome_stays_in_denominator(self) -> None:
        """A failed terminal outcome stays in the denominator (missingness at assignment level)."""
        assignments = _make_all_assignments()
        attempts, observations = _make_terminal_attempts_for_all_assignments(
            assignments, terminal_status=TerminalStatus.MODEL_FAILED,
        )
        # No metric observations for failed attempts
        record = _make_campaign_record(attempts, observations, assignments=assignments)
        analysis = compute_canonical_analysis_from_record(record)

        ri_results = [r for r in analysis.metric_results if r.metric_id == _METRIC_ID]
        # Every assignment contributes to the denominator regardless of failure
        for result in ri_results:
            # 2 tasks x 2 replicates = 4 assignments per (cohort, arm)
            assert result.denominator == 4, (
                f"Expected denominator 4 for ({result.model_cohort_id}, {result.arm_id}), "
                f"got {result.denominator}. Failed assignments must stay in the denominator."
            )
            assert result.missing_count == 4, (
                f"Expected missing_count 4 for ({result.model_cohort_id}, {result.arm_id}), "
                f"got {result.missing_count}. Failed terminal outcomes are missing evidence."
            )


# ---------------------------------------------------------------------------
# Cohort-stratified paired comparisons
# ---------------------------------------------------------------------------


class TestCohortStratifiedPairedComparisons:
    """Paired comparisons are within cohorts, never across cohorts."""

    def test_paired_comparison_has_model_cohort_id_field(self) -> None:
        """PairedComparison model has a model_cohort_id field."""
        pc = PairedComparison(
            metric_id=_METRIC_ID,
            metric_version=_METRIC_VERSION,
            baseline_arm_id="direct",
            comparison_arm_id="ensemble_ungoverned",
            paired_count=2,
            model_cohort_id="cohort-qwen3-8b",
            selected_test="paired_t",
            replicate_aggregation_policy="mean",
        )
        assert pc.model_cohort_id == "cohort-qwen3-8b"

    def test_two_cohorts_produce_separate_comparisons(self) -> None:
        """Two cohorts produce separate paired comparisons per cohort, not one pooled comparison."""
        assignments = _make_all_assignments()
        # Give different values per cohort so we can verify stratification
        values: dict[str, float] = {}
        for a in assignments:
            if a.model_cohort_id == _COHORT_IDS[0]:
                values[a.assignment_id] = 1.0 if a.arm_id == "direct" else 0.0
            else:
                values[a.assignment_id] = 0.5
        attempts, observations = _make_terminal_attempts_for_all_assignments(assignments, values=values)
        record = _make_campaign_record(attempts, observations, assignments=assignments)
        analysis = compute_canonical_analysis_from_record(record)

        ri_comparisons = [c for c in analysis.comparisons if c.metric_id == _METRIC_ID]
        # Two cohorts = 2 comparisons (one per cohort)
        assert len(ri_comparisons) == 2, (
            f"Expected 2 comparisons (one per cohort), got {len(ri_comparisons)}. "
            f"Currently the engine pools cohorts into one comparison."
        )
        cohort_ids = {c.model_cohort_id for c in ri_comparisons}
        assert cohort_ids == set(_COHORT_IDS), (
            f"Expected cohort IDs {set(_COHORT_IDS)}, got {cohort_ids}"
        )

    def test_no_cross_cohort_pairing(self) -> None:
        """Observations from different cohorts are never paired together."""
        assignments = _make_all_assignments()
        attempts, observations = _make_terminal_attempts_for_all_assignments(assignments)
        record = _make_campaign_record(attempts, observations, assignments=assignments)
        analysis = compute_canonical_analysis_from_record(record)

        for comparison in analysis.comparisons:
            if comparison.metric_id != _METRIC_ID:
                continue
            # Each comparison's paired_count should be the number of tasks x replicates
            # within one cohort (2 tasks x 2 replicates = 4), not across cohorts
            assert comparison.paired_count == 4, (
                f"Expected paired_count 4 (within one cohort), got {comparison.paired_count}. "
                f"Cross-cohort pairing must not occur."
            )


# ---------------------------------------------------------------------------
# Descriptive-only claim policy
# ---------------------------------------------------------------------------


class TestDescriptiveOnlyPolicy:
    """Descriptive-only claim policy prevents p-values from setting a superiority gate."""

    def test_descriptive_only_prevents_superiority_gate(self) -> None:
        """When claim_policy is descriptive_only, paired comparison gate is never PASS for superiority."""
        assignments = _make_all_assignments()
        # Create a scenario where the comparison arm is clearly better
        values: dict[str, float] = {}
        for a in assignments:
            if a.arm_id == "direct":
                values[a.assignment_id] = 0.0
            else:
                values[a.assignment_id] = 1.0
        attempts, observations = _make_terminal_attempts_for_all_assignments(assignments, values=values)
        prereg = _make_preregistration(claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY)
        record = _make_campaign_record(
            attempts, observations, assignments=assignments, preregistration=prereg,
        )
        analysis = compute_canonical_analysis_from_record(record)

        ri_comparisons = [c for c in analysis.comparisons if c.metric_id == _METRIC_ID]
        for comparison in ri_comparisons:
            if comparison.non_inferiority_margin is not None:
                continue
            # Descriptive-only policy prevents superiority PASS
            assert comparison.gate_decision != GateDecisionStatus.PASS, (
                f"Descriptive-only policy must prevent superiority PASS, "
                f"got {comparison.gate_decision} for cohort {comparison.model_cohort_id}"
            )

    def test_claim_policy_field_exists_on_preregistration(self) -> None:
        """PreregistrationConfig has a claim_policy field."""
        prereg = _make_preregistration(claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY)
        assert prereg.claim_policy == ClaimPolicy.DESCRIPTIVE_ONLY

    def test_claim_policy_defaults_to_descriptive_only(self) -> None:
        """PreregistrationConfig defaults to descriptive_only claim policy."""
        prereg = PreregistrationConfig(
            config_id="test-config-1",
            config_version="1.0.0",
            baseline_arm_id="direct",
            comparison_arm_ids=["ensemble_ungoverned"],
            model_cohort_ids=_COHORT_IDS,
            task_assignment_id="task-assignment-v1",
            initial_state_assignment_id=_INITIAL_STATE_ID,
            required_replicate_ids=_REPLICATE_IDS,
            required_replicate_count=2,
            primary_metric_ids=[_METRIC_ID],
            bootstrap_count=10000,
            bootstrap_confidence=0.95,
            bootstrap_seed=0,
            significance_level=0.05,
        )
        assert prereg.claim_policy == ClaimPolicy.DESCRIPTIVE_ONLY


# ---------------------------------------------------------------------------
# Authority hash binding
# ---------------------------------------------------------------------------


class TestAuthorityHashBinding:
    """Campaign authority hashes are bound into analysis input and output summaries."""

    def test_campaign_manifest_hash_in_input_summary(self) -> None:
        """AnalysisInputSummary includes the campaign manifest hash when campaign records are present."""
        assignments = _make_all_assignments()
        attempts, observations = _make_terminal_attempts_for_all_assignments(assignments)
        record = _make_campaign_record(attempts, observations, assignments=assignments)
        analysis = compute_canonical_analysis_from_record(record)

        # The input summary should include the campaign manifest hash
        assert hasattr(analysis.input_summary, "campaign_manifest_hash"), (
            "AnalysisInputSummary must have a campaign_manifest_hash field"
        )
        assert analysis.input_summary.campaign_manifest_hash is not None, (
            "campaign_manifest_hash must be set when campaign records are present"
        )

    def test_assignment_count_in_input_summary(self) -> None:
        """AnalysisInputSummary includes the campaign assignment count."""
        assignments = _make_all_assignments()
        attempts, observations = _make_terminal_attempts_for_all_assignments(assignments)
        record = _make_campaign_record(attempts, observations, assignments=assignments)
        analysis = compute_canonical_analysis_from_record(record)

        assert hasattr(analysis.input_summary, "assignment_count"), (
            "AnalysisInputSummary must have an assignment_count field"
        )
        # 2 tasks x 2 cohorts x 2 arms x 2 replicates = 16 assignments
        assert analysis.input_summary.assignment_count == 16, (
            f"Expected assignment_count 16, got {analysis.input_summary.assignment_count}"
        )
