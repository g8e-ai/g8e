# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for campaign, cohort, assignment, schedule, and retry contracts.

These tests verify the typed model structure, content-hash validation,
Cartesian completeness, schedule integrity, retry-chain validation,
and PreregistrationConfig model-level validation. No external
dependencies (no files, network, or DB).
"""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis.canonical import PreregistrationConfig
from g8e_evals.arms import Arm
from g8e_evals.campaign import (
    CAMPAIGN_CONTRACT_VERSION,
    CampaignAssignment,
    CampaignManifest,
    CampaignStatus,
    ExecutionSchedule,
    InitialStateAssignmentManifest,
    ModelCohort,
    RetryPolicy,
    RoleModelBinding,
    SamplingSettings,
    TaskAssignmentManifest,
    TerminalSelectionPolicy,
    compute_assignment_id,
    compute_campaign_manifest_hash,
    compute_initial_state_hash,
    compute_model_cohort_hash,
    compute_schedule_hash,
    compute_task_assignment_hash,
    validate_campaign_assignments,
    validate_retry_chain,
    validate_schedule,
)
from g8e_evals.schema import AttemptRecord, TerminalStatus


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019", "task-1051", "task-1072", "task-1075"]
_COHORT_IDS = ["cohort-qwen3-8b", "cohort-granite-3.3-8b"]
_ARM_IDS = ["direct", "ensemble_ungoverned"]
_REPLICATE_IDS = ["replicate-1", "replicate-2"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64


def _make_sampling() -> SamplingSettings:
    return SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42)


def _make_role_binding(role: str = "primary", model_id: str = "qwen3:8b") -> RoleModelBinding:
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
    bindings = [_make_role_binding("primary", model_id)]
    ch = compute_model_cohort_hash(cohort_id, bindings)
    return ModelCohort(cohort_id=cohort_id, role_bindings=bindings, content_hash=ch)


def _make_task_assignment() -> TaskAssignmentManifest:
    ch = compute_task_assignment_hash("task-assignment-v1", "ifeval_subset", _DATASET_HASH, _TASK_IDS)
    return TaskAssignmentManifest(
        task_assignment_id="task-assignment-v1",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=_TASK_IDS,
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


def _make_all_assignments() -> list[CampaignAssignment]:
    assignments: list[CampaignAssignment] = []
    pos = 0
    for task_id in _TASK_IDS:
        for cohort_id in _COHORT_IDS:
            for arm_id in _ARM_IDS:
                for replicate_id in _REPLICATE_IDS:
                    aid = compute_assignment_id(
                        _CAMPAIGN_ID, task_id, cohort_id, arm_id, _INITIAL_STATE_ID, replicate_id
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


def _make_schedule(assignments: list[CampaignAssignment] | None = None) -> ExecutionSchedule:
    if assignments is None:
        assignments = _make_all_assignments()
    ordered_ids: list[str] = [""] * len(assignments)
    for a in assignments:
        ordered_ids[a.schedule_position] = a.assignment_id
    ch = compute_schedule_hash(
        "schedule-v1", _CAMPAIGN_ID, 42, "shuffle", "1.0.0", ordered_ids
    )
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


def _make_attempt(
    attempt_id: str = "attempt-1",
    assignment_id: str = "",
    terminal_status: TerminalStatus = TerminalStatus.COMPLETED,
    parent_attempt_id: str | None = None,
) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id="run-1",
        task_id=_TASK_IDS[0],
        arm_id=Arm.DIRECT,
        model_cohort_id=_COHORT_IDS[0],
        replicate_id="replicate-1",
        assignment_id=assignment_id,
        terminal_status=terminal_status,
        parent_attempt_id=parent_attempt_id,
    )


# ---------------------------------------------------------------------------
# ModelCohort tests
# ---------------------------------------------------------------------------

class TestModelCohort:
    def test_valid_cohort_accepts_correct_hash(self):
        cohort = _make_cohort()
        assert cohort.cohort_id == "cohort-qwen3-8b"
        assert len(cohort.content_hash) == 64

    def test_cohort_rejects_wrong_content_hash(self):
        bindings = [_make_role_binding()]
        with pytest.raises(ValueError, match="content_hash mismatch"):
            ModelCohort(
                cohort_id="cohort-qwen3-8b",
                role_bindings=bindings,
                content_hash="f" * 64,
            )

    def test_cohort_rejects_duplicate_roles(self):
        bindings = [_make_role_binding("primary"), _make_role_binding("primary", "granite3.3:8b")]
        ch = compute_model_cohort_hash("cohort-x", bindings)
        with pytest.raises(ValueError, match="duplicate role"):
            ModelCohort(
                cohort_id="cohort-x",
                role_bindings=bindings,
                content_hash=ch,
            )

    def test_cohort_rejects_empty_role_bindings(self):
        with pytest.raises(Exception, match="should have at least 1 item"):
            ModelCohort(
                cohort_id="cohort-x",
                role_bindings=[],
                content_hash="0" * 64,
            )

    def test_different_model_changes_hash(self):
        ch1 = compute_model_cohort_hash("c1", [_make_role_binding("primary", "qwen3:8b")])
        ch2 = compute_model_cohort_hash("c1", [_make_role_binding("primary", "granite3.3:8b")])
        assert ch1 != ch2

    def test_different_sampling_changes_hash(self):
        s1 = SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42)
        s2 = SamplingSettings(temperature=0.7, top_p=1.0, max_tokens=4096, seed=42)
        rb1 = RoleModelBinding(role="primary", model_id="qwen3:8b", provider="ollama",
                               endpoint="http://x", sampling_settings=s1, timeout_seconds=120.0, seed_capable=True)
        rb2 = RoleModelBinding(role="primary", model_id="qwen3:8b", provider="ollama",
                               endpoint="http://x", sampling_settings=s2, timeout_seconds=120.0, seed_capable=True)
        ch1 = compute_model_cohort_hash("c1", [rb1])
        ch2 = compute_model_cohort_hash("c1", [rb2])
        assert ch1 != ch2

    def test_different_endpoint_changes_hash(self):
        rb1 = RoleModelBinding(role="primary", model_id="qwen3:8b", provider="ollama",
                               endpoint="http://a:11434", sampling_settings=_make_sampling(),
                               timeout_seconds=120.0, seed_capable=True)
        rb2 = RoleModelBinding(role="primary", model_id="qwen3:8b", provider="ollama",
                               endpoint="http://b:11434", sampling_settings=_make_sampling(),
                               timeout_seconds=120.0, seed_capable=True)
        ch1 = compute_model_cohort_hash("c1", [rb1])
        ch2 = compute_model_cohort_hash("c1", [rb2])
        assert ch1 != ch2

    def test_different_timeout_changes_hash(self):
        rb1 = RoleModelBinding(role="primary", model_id="qwen3:8b", provider="ollama",
                               endpoint="http://x", sampling_settings=_make_sampling(),
                               timeout_seconds=120.0, seed_capable=True)
        rb2 = RoleModelBinding(role="primary", model_id="qwen3:8b", provider="ollama",
                               endpoint="http://x", sampling_settings=_make_sampling(),
                               timeout_seconds=60.0, seed_capable=True)
        ch1 = compute_model_cohort_hash("c1", [rb1])
        ch2 = compute_model_cohort_hash("c1", [rb2])
        assert ch1 != ch2


# ---------------------------------------------------------------------------
# TaskAssignmentManifest tests
# ---------------------------------------------------------------------------

class TestTaskAssignmentManifest:
    def test_valid_task_assignment_accepts_correct_hash(self):
        ta = _make_task_assignment()
        assert ta.task_assignment_id == "task-assignment-v1"
        assert len(ta.content_hash) == 64

    def test_task_assignment_rejects_wrong_content_hash(self):
        with pytest.raises(ValueError, match="content_hash mismatch"):
            TaskAssignmentManifest(
                task_assignment_id="task-assignment-v1",
                suite_id="ifeval_subset",
                dataset_hash=_DATASET_HASH,
                task_ids=_TASK_IDS,
                content_hash="f" * 64,
            )

    def test_task_assignment_rejects_duplicate_task_ids(self):
        ch = compute_task_assignment_hash("ta-1", "suite", _DATASET_HASH, ["a", "a"])
        with pytest.raises(ValueError, match="duplicate task IDs"):
            TaskAssignmentManifest(
                task_assignment_id="ta-1",
                suite_id="suite",
                dataset_hash=_DATASET_HASH,
                task_ids=["a", "a"],
                content_hash=ch,
            )


# ---------------------------------------------------------------------------
# InitialStateAssignmentManifest tests
# ---------------------------------------------------------------------------

class TestInitialStateAssignmentManifest:
    def test_valid_initial_state_accepts_correct_hash(self):
        istate = _make_initial_state()
        assert istate.state_type == "no_initial_state"
        assert len(istate.content_hash) == 64

    def test_initial_state_rejects_wrong_content_hash(self):
        with pytest.raises(ValueError, match="content_hash mismatch"):
            InitialStateAssignmentManifest(
                initial_state_assignment_id=_INITIAL_STATE_ID,
                state_type="no_initial_state",
                snapshot_hash=_NO_STATE_HASH,
                content_hash="f" * 64,
            )


# ---------------------------------------------------------------------------
# ExecutionSchedule tests
# ---------------------------------------------------------------------------

class TestExecutionSchedule:
    def test_valid_schedule_accepts_correct_hash(self):
        schedule = _make_schedule()
        assert len(schedule.content_hash) == 64
        assert len(schedule.ordered_assignment_ids) == 40

    def test_schedule_rejects_wrong_content_hash(self):
        assignments = _make_all_assignments()
        ordered_ids = [a.assignment_id for a in assignments]
        with pytest.raises(ValueError, match="content_hash mismatch"):
            ExecutionSchedule(
                schedule_id="schedule-v1",
                campaign_id=_CAMPAIGN_ID,
                randomization_seed=42,
                algorithm_id="shuffle",
                algorithm_version="1.0.0",
                ordered_assignment_ids=ordered_ids,
                content_hash="f" * 64,
            )

    def test_schedule_rejects_duplicate_assignment_ids(self):
        ordered_ids = ["a", "a"]
        ch = compute_schedule_hash("s1", "c1", 42, "shuffle", "1.0.0", ordered_ids)
        with pytest.raises(ValueError, match="duplicate assignment IDs"):
            ExecutionSchedule(
                schedule_id="s1",
                campaign_id="c1",
                randomization_seed=42,
                algorithm_id="shuffle",
                algorithm_version="1.0.0",
                ordered_assignment_ids=ordered_ids,
                content_hash=ch,
            )


# ---------------------------------------------------------------------------
# RetryPolicy tests
# ---------------------------------------------------------------------------

class TestRetryPolicy:
    def test_valid_retry_policy(self):
        rp = _make_retry_policy()
        assert rp.max_retries == 1
        assert rp.retryable_terminal_statuses == ["infrastructure_failed"]

    def test_retry_policy_rejects_duplicate_statuses(self):
        with pytest.raises(ValueError, match="duplicate retryable terminal statuses"):
            RetryPolicy(max_retries=2, retryable_terminal_statuses=["infrastructure_failed", "infrastructure_failed"])

    def test_zero_retries_allowed(self):
        rp = RetryPolicy(max_retries=0, retryable_terminal_statuses=[])
        assert rp.max_retries == 0


# ---------------------------------------------------------------------------
# CampaignManifest tests
# ---------------------------------------------------------------------------

class TestCampaignManifest:
    def test_valid_campaign_manifest(self):
        ch_cohort = "a" * 64
        manifest = CampaignManifest(
            campaign_id=_CAMPAIGN_ID,
            campaign_version=CAMPAIGN_CONTRACT_VERSION,
            release_version=_RELEASE_VERSION,
            preregistration_hash="b" * 64,
            cohort_hashes=[ch_cohort],
            task_assignment_hash="c" * 64,
            initial_state_assignment_hash="d" * 64,
            schedule_hash="e" * 64,
            retry_policy_hash="f" * 64,
            metric_registry_hash="1" * 64,
            release_metric_set_hash="2" * 64,
            threshold_authority_hash="3" * 64,
            missingness_authority_hash="4" * 64,
            provider_budget_hash="5" * 64,
            source_build_provenance_hash="6" * 64,
            claim_exclusion_hash="7" * 64,
            budget_observability_policy_hash="8" * 64,
            content_hash=compute_campaign_manifest_hash(
                _CAMPAIGN_ID, CAMPAIGN_CONTRACT_VERSION, _RELEASE_VERSION,
                "b" * 64, [ch_cohort], "c" * 64, "d" * 64, "e" * 64, "f" * 64,
                "1" * 64, "2" * 64, "3" * 64, "4" * 64, "5" * 64, "6" * 64, "7" * 64,
                "8" * 64,
            ),
        )
        assert manifest.campaign_id == _CAMPAIGN_ID

    def test_campaign_manifest_rejects_wrong_content_hash(self):
        with pytest.raises(ValueError, match="content_hash mismatch"):
            CampaignManifest(
                campaign_id=_CAMPAIGN_ID,
                campaign_version=CAMPAIGN_CONTRACT_VERSION,
                release_version=_RELEASE_VERSION,
                preregistration_hash="b" * 64,
                cohort_hashes=["a" * 64],
                task_assignment_hash="c" * 64,
                initial_state_assignment_hash="d" * 64,
                schedule_hash="e" * 64,
                retry_policy_hash="f" * 64,
                metric_registry_hash="1" * 64,
                release_metric_set_hash="2" * 64,
                threshold_authority_hash="3" * 64,
                missingness_authority_hash="4" * 64,
                provider_budget_hash="5" * 64,
                source_build_provenance_hash="6" * 64,
                claim_exclusion_hash="7" * 64,
                budget_observability_policy_hash="8" * 64,
                content_hash="0" * 64,
            )

    def test_campaign_manifest_rejects_duplicate_cohort_hashes(self):
        ch = "a" * 64
        expected_hash = compute_campaign_manifest_hash(
            _CAMPAIGN_ID, CAMPAIGN_CONTRACT_VERSION, _RELEASE_VERSION,
            "b" * 64, [ch, ch], "c" * 64, "d" * 64, "e" * 64, "f" * 64,
            "1" * 64, "2" * 64, "3" * 64, "4" * 64, "5" * 64, "6" * 64, "7" * 64,
            "8" * 64,
        )
        with pytest.raises(ValueError, match="duplicate cohort hashes"):
            CampaignManifest(
                campaign_id=_CAMPAIGN_ID,
                campaign_version=CAMPAIGN_CONTRACT_VERSION,
                release_version=_RELEASE_VERSION,
                preregistration_hash="b" * 64,
                cohort_hashes=[ch, ch],
                task_assignment_hash="c" * 64,
                initial_state_assignment_hash="d" * 64,
                schedule_hash="e" * 64,
                retry_policy_hash="f" * 64,
                metric_registry_hash="1" * 64,
                release_metric_set_hash="2" * 64,
                threshold_authority_hash="3" * 64,
                missingness_authority_hash="4" * 64,
                provider_budget_hash="5" * 64,
                source_build_provenance_hash="6" * 64,
                claim_exclusion_hash="7" * 64,
                budget_observability_policy_hash="8" * 64,
                content_hash=expected_hash,
            )


# ---------------------------------------------------------------------------
# CampaignStatus and TerminalSelectionPolicy tests
# ---------------------------------------------------------------------------

class TestEnums:
    def test_campaign_status_values(self):
        assert CampaignStatus.PLANNED == "planned"
        assert CampaignStatus.RUNNING == "running"
        assert CampaignStatus.STOPPED == "stopped"
        assert CampaignStatus.COMPLETED == "completed"
        assert CampaignStatus.FAILED_INTEGRITY == "failed_integrity"
        assert CampaignStatus.FINALIZED == "finalized"

    def test_terminal_selection_policy(self):
        assert TerminalSelectionPolicy.FINAL_TRY_IS_OUTCOME == "final_try_is_outcome"


# ---------------------------------------------------------------------------
# validate_campaign_assignments tests
# ---------------------------------------------------------------------------

class TestValidateCampaignAssignments:
    def test_valid_complete_cartesian_product(self):
        assignments = _make_all_assignments()
        validate_campaign_assignments(
            assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
            _INITIAL_STATE_ID, _CAMPAIGN_ID,
        )

    def test_rejects_missing_assignment(self):
        assignments = _make_all_assignments()
        assignments = assignments[:-1]
        with pytest.raises(ValueError, match="count mismatch"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_extra_assignment(self):
        assignments = _make_all_assignments()
        extra = CampaignAssignment(
            campaign_id=_CAMPAIGN_ID,
            assignment_id="extra-id",
            task_id=_TASK_IDS[0],
            model_cohort_id=_COHORT_IDS[0],
            arm_id="direct",
            initial_state_assignment_id=_INITIAL_STATE_ID,
            replicate_id="replicate-1",
            schedule_position=99,
        )
        assignments = [*assignments, extra]
        with pytest.raises(ValueError, match="count mismatch"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_duplicate_assignment_ids(self):
        assignments = _make_all_assignments()
        assignments[1] = assignments[0].model_copy(update={"schedule_position": 1})
        with pytest.raises(ValueError, match="duplicate assignment IDs"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_duplicate_cartesian_slot(self):
        assignments = _make_all_assignments()
        assignments[-1] = CampaignAssignment(
            campaign_id=_CAMPAIGN_ID,
            assignment_id="unique-id",
            task_id=_TASK_IDS[0],
            model_cohort_id=_COHORT_IDS[0],
            arm_id="direct",
            initial_state_assignment_id=_INITIAL_STATE_ID,
            replicate_id="replicate-1",
            schedule_position=39,
        )
        with pytest.raises(ValueError, match="duplicate Cartesian slot"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_unknown_task(self):
        assignments = _make_all_assignments()
        assignments[0] = assignments[0].model_copy(update={"task_id": "unknown-task"})
        with pytest.raises(ValueError, match="not in declared task set"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_unknown_cohort(self):
        assignments = _make_all_assignments()
        assignments[0] = assignments[0].model_copy(update={"model_cohort_id": "unknown-cohort"})
        with pytest.raises(ValueError, match="not in declared cohort set"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_unknown_arm(self):
        assignments = _make_all_assignments()
        assignments[0] = assignments[0].model_copy(update={"arm_id": "unknown-arm"})
        with pytest.raises(ValueError, match="not in declared arm set"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_unknown_replicate(self):
        assignments = _make_all_assignments()
        assignments[0] = assignments[0].model_copy(update={"replicate_id": "replicate-99"})
        with pytest.raises(ValueError, match="not in declared replicate set"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_wrong_campaign_id(self):
        assignments = _make_all_assignments()
        assignments[0] = assignments[0].model_copy(update={"campaign_id": "wrong-campaign"})
        with pytest.raises(ValueError, match="campaign_id"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )

    def test_rejects_wrong_initial_state(self):
        assignments = _make_all_assignments()
        assignments[0] = assignments[0].model_copy(update={"initial_state_assignment_id": "wrong-state"})
        with pytest.raises(ValueError, match="initial_state_assignment_id"):
            validate_campaign_assignments(
                assignments, _TASK_IDS, _COHORT_IDS, _ARM_IDS, _REPLICATE_IDS,
                _INITIAL_STATE_ID, _CAMPAIGN_ID,
            )


# ---------------------------------------------------------------------------
# validate_schedule tests
# ---------------------------------------------------------------------------

class TestValidateSchedule:
    def test_valid_schedule_matches_assignments(self):
        assignments = _make_all_assignments()
        schedule = _make_schedule(assignments)
        validate_schedule(schedule, assignments)

    def test_rejects_schedule_missing_assignment(self):
        assignments = _make_all_assignments()
        schedule = _make_schedule(assignments)
        truncated = schedule.model_copy(update={"ordered_assignment_ids": schedule.ordered_assignment_ids[:-1]})
        with pytest.raises(ValueError, match="set mismatch"):
            validate_schedule(truncated, assignments)

    def test_rejects_schedule_with_extra_assignment(self):
        assignments = _make_all_assignments()
        schedule = _make_schedule(assignments)
        extended = schedule.model_copy(update={
            "ordered_assignment_ids": [*schedule.ordered_assignment_ids, "extra-id"],
        })
        with pytest.raises(ValueError, match="set mismatch"):
            validate_schedule(extended, assignments)

    def test_rejects_schedule_with_duplicate_ids(self):
        assignments = _make_all_assignments()
        ordered = [a.assignment_id for a in assignments]
        ordered[1] = ordered[0]
        ch = compute_schedule_hash("s1", _CAMPAIGN_ID, 42, "shuffle", "1.0.0", ordered)
        with pytest.raises(Exception, match="duplicate assignment IDs"):
            ExecutionSchedule(
                schedule_id="s1", campaign_id=_CAMPAIGN_ID, randomization_seed=42,
                algorithm_id="shuffle", algorithm_version="1.0.0",
                ordered_assignment_ids=ordered, content_hash=ch,
            )

    def test_rejects_assignment_position_mismatch(self):
        assignments = _make_all_assignments()
        assignments[0] = assignments[0].model_copy(update={"schedule_position": 1})
        with pytest.raises(ValueError, match="does not match schedule entry"):
            validate_schedule(_make_schedule(), assignments)

    def test_rejects_assignment_position_out_of_range(self):
        assignments = _make_all_assignments()
        assignments[0] = assignments[0].model_copy(update={"schedule_position": 999})
        with pytest.raises(ValueError, match="out of range"):
            validate_schedule(_make_schedule(), assignments)


# ---------------------------------------------------------------------------
# validate_retry_chain tests
# ---------------------------------------------------------------------------

class TestValidateRetryChain:
    def test_valid_single_attempt_no_retry(self):
        a1 = _make_attempt("a1", "asg-1")
        rp = _make_retry_policy()
        validate_retry_chain([a1], "asg-1", rp)

    def test_valid_retry_chain_two_attempts(self):
        a1 = _make_attempt("a1", "asg-1", TerminalStatus.INFRASTRUCTURE_FAILED)
        a2 = _make_attempt("a2", "asg-1", TerminalStatus.COMPLETED, parent_attempt_id="a1")
        rp = _make_retry_policy()
        validate_retry_chain([a1, a2], "asg-1", rp)

    def test_rejects_exceeding_retry_limit(self):
        a1 = _make_attempt("a1", "asg-1", TerminalStatus.INFRASTRUCTURE_FAILED)
        a2 = _make_attempt("a2", "asg-1", TerminalStatus.INFRASTRUCTURE_FAILED, parent_attempt_id="a1")
        a3 = _make_attempt("a3", "asg-1", TerminalStatus.COMPLETED, parent_attempt_id="a2")
        rp = _make_retry_policy()
        with pytest.raises(ValueError, match="exceeding max_retries"):
            validate_retry_chain([a1, a2, a3], "asg-1", rp)

    def test_rejects_duplicate_attempt_ids(self):
        a1 = _make_attempt("a1", "asg-1")
        a2 = _make_attempt("a1", "asg-1")
        rp = _make_retry_policy()
        with pytest.raises(ValueError, match="duplicate attempt IDs"):
            validate_retry_chain([a1, a2], "asg-1", rp)

    def test_rejects_cross_assignment_parent(self):
        a1 = _make_attempt("a1", "asg-1")
        a2 = _make_attempt("a2", "asg-2", parent_attempt_id="a1")
        rp = _make_retry_policy()
        with pytest.raises(ValueError, match="parent_attempt_id"):
            validate_retry_chain([a1, a2], "asg-2", rp)

    def test_rejects_self_referential_cycle(self):
        a1 = _make_attempt("a1", "asg-1", parent_attempt_id="a1")
        rp = _make_retry_policy()
        with pytest.raises(ValueError, match="self-referential"):
            validate_retry_chain([a1], "asg-1", rp)

    def test_rejects_retry_chain_cycle(self):
        a1 = _make_attempt("a1", "asg-1", parent_attempt_id="a2")
        a2 = _make_attempt("a2", "asg-1", parent_attempt_id="a1")
        rp = _make_retry_policy()
        with pytest.raises(ValueError, match="cycle"):
            validate_retry_chain([a1, a2], "asg-1", rp)

    def test_rejects_fork_in_retry_chain(self):
        a1 = _make_attempt("a1", "asg-1", TerminalStatus.INFRASTRUCTURE_FAILED)
        a2 = _make_attempt("a2", "asg-1", parent_attempt_id="a1")
        a3 = _make_attempt("a3", "asg-1", parent_attempt_id="a1")
        rp = RetryPolicy(max_retries=2, retryable_terminal_statuses=["infrastructure_failed"])
        with pytest.raises(ValueError, match="fork"):
            validate_retry_chain([a1, a2, a3], "asg-1", rp)


# ---------------------------------------------------------------------------
# PreregistrationConfig model-level validation tests
# ---------------------------------------------------------------------------

class TestPreregistrationConfigValidation:
    def _make_valid_prereg(self) -> PreregistrationConfig:
        return PreregistrationConfig(
            config_id="prereg-v1",
            config_version="1.0.0",
            baseline_arm_id="direct",
            comparison_arm_ids=["ensemble_ungoverned"],
            model_cohort_ids=_COHORT_IDS,
            task_assignment_id="task-assignment-v1",
            initial_state_assignment_id=_INITIAL_STATE_ID,
            required_replicate_ids=_REPLICATE_IDS,
            required_replicate_count=2,
            primary_metric_ids=["ifeval_subset_verifier"],
            bootstrap_count=1000,
            bootstrap_confidence=0.95,
            bootstrap_seed=42,
            significance_level=0.05,
        )

    def test_valid_preregistration_accepted(self):
        prereg = self._make_valid_prereg()
        assert prereg.baseline_arm_id == "direct"

    def test_rejects_baseline_equals_comparison(self):
        with pytest.raises(ValueError, match="cannot also be a comparison arm"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["direct"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_duplicate_comparison_arms(self):
        with pytest.raises(ValueError, match="duplicate comparison arm"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["ensemble_ungoverned", "ensemble_ungoverned"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_duplicate_cohort_ids(self):
        with pytest.raises(ValueError, match="duplicate model cohort"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["ensemble_ungoverned"],
                model_cohort_ids=["cohort-a", "cohort-a"],
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_duplicate_replicate_ids(self):
        with pytest.raises(ValueError, match="duplicate replicate"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["ensemble_ungoverned"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=["replicate-1", "replicate-1"],
                required_replicate_count=2,
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_replicate_count_list_mismatch(self):
        with pytest.raises(ValueError, match="required_replicate_count"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["ensemble_ungoverned"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=["replicate-1", "replicate-2"],
                required_replicate_count=3,
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_unknown_arm_id(self):
        with pytest.raises(ValueError, match="not a known arm"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["unknown_arm"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_unknown_baseline_arm_id(self):
        with pytest.raises(ValueError, match="not a known arm"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="unknown_arm",
                comparison_arm_ids=["ensemble_ungoverned"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_duplicate_secondary_family_names(self):
        from g8e_evals.analysis.canonical import SecondaryFamily
        with pytest.raises(ValueError, match="duplicate secondary family names"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["ensemble_ungoverned"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                secondary_families=[
                    SecondaryFamily(family_name="fam-a", metric_ids=["m1"]),
                    SecondaryFamily(family_name="fam-a", metric_ids=["m2"]),
                ],
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_metric_in_multiple_families(self):
        from g8e_evals.analysis.canonical import SecondaryFamily
        with pytest.raises(ValueError, match="multiple secondary families"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["ensemble_ungoverned"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                secondary_families=[
                    SecondaryFamily(family_name="fam-a", metric_ids=["m1"]),
                    SecondaryFamily(family_name="fam-b", metric_ids=["m1"]),
                ],
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_rejects_primary_metric_in_secondary_family(self):
        from g8e_evals.analysis.canonical import SecondaryFamily
        with pytest.raises(ValueError, match="cannot be in a secondary family"):
            PreregistrationConfig(
                config_id="prereg-v1",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=["ensemble_ungoverned"],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                primary_metric_ids=["m1"],
                secondary_families=[
                    SecondaryFamily(family_name="fam-a", metric_ids=["m1"]),
                ],
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
            )

    def test_accepts_empty_comparison_arms_for_single_arm_campaign(self):
        from g8e_evals.analysis.canonical import ClaimPolicy
        prereg = PreregistrationConfig(
            config_id="prereg-single-arm",
            config_version="1.0.0",
            baseline_arm_id="direct",
            comparison_arm_ids=[],
            model_cohort_ids=_COHORT_IDS,
            task_assignment_id="task-assignment-v1",
            initial_state_assignment_id=_INITIAL_STATE_ID,
            required_replicate_ids=_REPLICATE_IDS,
            required_replicate_count=2,
            primary_metric_ids=["ifeval_subset_verifier"],
            bootstrap_count=1000,
            bootstrap_confidence=0.95,
            bootstrap_seed=42,
            significance_level=0.05,
            claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
        )
        assert prereg.comparison_arm_ids == []
        assert prereg.baseline_arm_id == "direct"

    def test_rejects_superiority_claim_policy_with_empty_comparison_arms(self):
        from g8e_evals.analysis.canonical import ClaimPolicy
        with pytest.raises(ValueError, match=r"SUPERIORITY.*requires at least one comparison arm"):
            PreregistrationConfig(
                config_id="prereg-single-arm",
                config_version="1.0.0",
                baseline_arm_id="direct",
                comparison_arm_ids=[],
                model_cohort_ids=_COHORT_IDS,
                task_assignment_id="task-assignment-v1",
                initial_state_assignment_id=_INITIAL_STATE_ID,
                required_replicate_ids=_REPLICATE_IDS,
                required_replicate_count=2,
                primary_metric_ids=["ifeval_subset_verifier"],
                bootstrap_count=1000,
                bootstrap_confidence=0.95,
                bootstrap_seed=42,
                significance_level=0.05,
                claim_policy=ClaimPolicy.SUPERIORITY,
            )


# ---------------------------------------------------------------------------
# CampaignAssignment determinism tests
# ---------------------------------------------------------------------------

class TestAssignmentIdDeterminism:
    def test_same_inputs_produce_same_assignment_id(self):
        aid1 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[0], "direct", _INITIAL_STATE_ID, "replicate-1"
        )
        aid2 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[0], "direct", _INITIAL_STATE_ID, "replicate-1"
        )
        assert aid1 == aid2

    def test_different_task_produces_different_assignment_id(self):
        aid1 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[0], "direct", _INITIAL_STATE_ID, "replicate-1"
        )
        aid2 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[1], _COHORT_IDS[0], "direct", _INITIAL_STATE_ID, "replicate-1"
        )
        assert aid1 != aid2

    def test_different_cohort_produces_different_assignment_id(self):
        aid1 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[0], "direct", _INITIAL_STATE_ID, "replicate-1"
        )
        aid2 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[1], "direct", _INITIAL_STATE_ID, "replicate-1"
        )
        assert aid1 != aid2

    def test_different_arm_produces_different_assignment_id(self):
        aid1 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[0], "direct", _INITIAL_STATE_ID, "replicate-1"
        )
        aid2 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[0], "ensemble_ungoverned", _INITIAL_STATE_ID, "replicate-1"
        )
        assert aid1 != aid2

    def test_different_replicate_produces_different_assignment_id(self):
        aid1 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[0], "direct", _INITIAL_STATE_ID, "replicate-1"
        )
        aid2 = compute_assignment_id(
            _CAMPAIGN_ID, _TASK_IDS[0], _COHORT_IDS[0], "direct", _INITIAL_STATE_ID, "replicate-2"
        )
        assert aid1 != aid2

    def test_full_campaign_produces_40_unique_assignment_ids(self):
        assignments = _make_all_assignments()
        ids = [a.assignment_id for a in assignments]
        assert len(ids) == 40
        assert len(set(ids)) == 40
