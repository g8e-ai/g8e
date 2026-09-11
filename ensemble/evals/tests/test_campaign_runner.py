# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for the authoritative campaign runner.

These tests verify the campaign runner's schedule generation, resume,
retry, budget stop, cohort drift detection, and fake SUT execution
using local filesystem (tmp_path) and deterministic fake SUTs/graders.
No network, database, or real provider dependencies.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass, field
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.analysis.canonical import (
    ClaimPolicy,
    ContinuousTestPolicy,
    PreregistrationConfig,
)
from g8e_evals.arms import Arm
from g8e_evals.campaign import (
    CampaignStatus,
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
from g8e_evals.harness import Response, Score, Task
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.runner import (
    CampaignRunner,
    CampaignSpec,
    CampaignStopReason,
    build_campaign_state,
)
from g8e_evals.schema import ProviderBudget, TerminalStatus
from g8e_evals.constants import (
    ANALYSIS_INPUT_JSON,
    ANALYSIS_JSON,
    ATTEMPTS_JSONL,
    CAMPAIGN_ASSIGNMENTS_JSONL,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_SCHEDULE_JSON,
    CAMPAIGN_STATUS_JSON,
    METRICS_JSONL,
)


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_IDS = ["cohort-qwen3-8b", "cohort-granite-3.3-8b"]
_ARM_IDS = ["direct", "ensemble_ungoverned"]
_REPLICATE_IDS = ["replicate-1", "replicate-2"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64


# ---------------------------------------------------------------------------
# Fake SUT and Grader
# ---------------------------------------------------------------------------


@dataclass
class FakeSUT:
    """Deterministic fake SUT that returns a fixed answer.

    When ``fail_alternating`` is True, every other call raises an
    infrastructure error: the first call fails, the second succeeds,
    the third fails, etc. This models a transient infrastructure
    failure that retries can recover from, with one failure per
    assignment when the runner retries once.

    When ``provider_prefix`` is True, the response model string is
    prefixed with ``"ollama:"`` (e.g. ``"ollama:qwen3:8b"``) to
    simulate the real ``DirectProviderSUT`` which reports model as
    ``provider:model_tag`` format.
    """

    model_id: str
    answer: str = "This is a test answer with no commas."
    fail_alternating: bool = False
    provider_prefix: bool = False
    _call_count: int = field(default=0)

    async def get_answer(self, task: Task) -> Response:
        self._call_count += 1
        if self.fail_alternating and self._call_count % 2 == 1:
            raise RuntimeError("simulated infrastructure failure")
        model = f"ollama:{self.model_id}" if self.provider_prefix else self.model_id
        return Response(answer=self.answer, model=model, arm=Arm.DIRECT)


@dataclass
class DriftSUT:
    """Fake SUT that returns a different model than the cohort declares."""

    declared_model_id: str
    drift_model_id: str

    async def get_answer(self, task: Task) -> Response:
        return Response(answer="drift answer", model=self.drift_model_id, arm=Arm.DIRECT)


@dataclass
class FakeGrader:
    """Deterministic fake grader that always passes."""

    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


@dataclass
class FailingGrader:
    """Deterministic fake grader that always fails."""

    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=False, details=ScoreDetails())


def _make_fake_sut_factory(
    answer: str = "This is a test answer with no commas.",
    fail_alternating: bool = False,
    drift_model_id: str | None = None,
    provider_prefix: bool = False,
):
    """Create a SUT factory that returns deterministic fake SUTs."""
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
        if drift_model_id is not None:
            return DriftSUT(declared_model_id=model_id, drift_model_id=drift_model_id)
        return FakeSUT(
            model_id=model_id,
            answer=answer,
            fail_alternating=fail_alternating,
            provider_prefix=provider_prefix,
        )
    return factory


# ---------------------------------------------------------------------------
# Spec helpers
# ---------------------------------------------------------------------------


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


def _make_preregistration(
    cohort_ids: list[str] | None = None,
    arm_ids: list[str] | None = None,
    replicate_ids: list[str] | None = None,
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
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )


def _make_tasks(task_ids: list[str] | None = None) -> list[Task]:
    if task_ids is None:
        task_ids = _TASK_IDS
    tasks = []
    for tid in task_ids:
        tasks.append(
            Task(
                id=tid,
                prompt=f"Prompt for {tid}",
                metadata=TaskMetadata(
                    benchmark="ifeval_subset",
                    instruction_id_list=["punctuation:no_comma"],
                    kwargs=[{"no_comma": True}],
                ),
            )
        )
    return tasks


def _make_spec(
    task_ids: list[str] | None = None,
    cohort_ids: list[str] | None = None,
    arm_ids: list[str] | None = None,
    replicate_ids: list[str] | None = None,
    randomization_seed: int = 42,
    retry_policy: RetryPolicy | None = None,
    provider_budget: ProviderBudget | None = None,
) -> CampaignSpec:
    if task_ids is None:
        task_ids = _TASK_IDS
    if cohort_ids is None:
        cohort_ids = _COHORT_IDS
    if arm_ids is None:
        arm_ids = _ARM_IDS
    if replicate_ids is None:
        replicate_ids = _REPLICATE_IDS
    if retry_policy is None:
        retry_policy = RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"])

    cohorts = [_make_cohort(c, m) for c, m in zip(cohort_ids, ["qwen3:8b", "granite3.3:8b"], strict=True)]
    task_assignment = _make_task_assignment(task_ids)
    initial_state = _make_initial_state()
    preregistration = _make_preregistration(cohort_ids, arm_ids, replicate_ids)

    prompt_bundle = "\n".join(f"Prompt for {tid}" for tid in task_ids).encode()
    prompt_bundle_hash = hashlib.sha256(prompt_bundle).hexdigest()

    return CampaignSpec(
        campaign_id=_CAMPAIGN_ID,
        release_version=_RELEASE_VERSION,
        suite="ifeval_subset",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        dataset_hash=_DATASET_HASH,
        prompt_bundle_hash=prompt_bundle_hash,
        grader_bundle_hash="g" * 64,
        preregistration=preregistration,
        cohorts=cohorts,
        task_assignment=task_assignment,
        initial_state=initial_state,
        retry_policy=retry_policy,
        randomization_seed=randomization_seed,
        provider_budget=provider_budget,
    )


# ---------------------------------------------------------------------------
# Schedule tests
# ---------------------------------------------------------------------------


class TestScheduleGeneration:
    def test_schedule_is_deterministic_for_same_seed(self):
        spec = _make_spec(randomization_seed=42)
        assignments1, schedule1 = build_campaign_state(spec)
        assignments2, schedule2 = build_campaign_state(spec)
        assert schedule1.ordered_assignment_ids == schedule2.ordered_assignment_ids
        assert [a.assignment_id for a in assignments1] == [a.assignment_id for a in assignments2]

    def test_schedule_changes_with_different_seed(self):
        spec1 = _make_spec(randomization_seed=42)
        spec2 = _make_spec(randomization_seed=99)
        _, schedule1 = build_campaign_state(spec1)
        _, schedule2 = build_campaign_state(spec2)
        assert schedule1.ordered_assignment_ids != schedule2.ordered_assignment_ids

    def test_schedule_covers_all_assignments_exactly_once(self):
        spec = _make_spec()
        assignments, schedule = build_campaign_state(spec)
        expected_count = len(_TASK_IDS) * len(_COHORT_IDS) * len(_ARM_IDS) * len(_REPLICATE_IDS)
        assert len(assignments) == expected_count
        assert len(schedule.ordered_assignment_ids) == expected_count
        assert len(set(schedule.ordered_assignment_ids)) == expected_count
        assignment_ids = {a.assignment_id for a in assignments}
        assert set(schedule.ordered_assignment_ids) == assignment_ids

    def test_schedule_positions_are_contiguous_zero_indexed(self):
        spec = _make_spec()
        assignments, _ = build_campaign_state(spec)
        positions = sorted(a.schedule_position for a in assignments)
        assert positions == list(range(len(assignments)))

    def test_single_arm_campaign_produces_only_baseline_arm_assignments(self):
        spec = _make_spec(arm_ids=["direct"])
        assignments, schedule = build_campaign_state(spec)
        expected_count = len(_TASK_IDS) * len(_COHORT_IDS) * 1 * len(_REPLICATE_IDS)
        assert len(assignments) == expected_count
        assert all(a.arm_id == "direct" for a in assignments)
        assert len(schedule.ordered_assignment_ids) == expected_count


# ---------------------------------------------------------------------------
# Full execution tests
# ---------------------------------------------------------------------------


class TestFullExecution:
    def test_campaign_completes_all_assignments(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        assert result.status == CampaignStatus.FINALIZED
        assert result.stop_reason == CampaignStopReason.COMPLETED
        expected = len(_TASK_IDS) * len(_COHORT_IDS) * len(_ARM_IDS) * len(_REPLICATE_IDS)
        assert result.assignment_count == expected

        # All persisted artifacts exist
        report = result.report_dir
        assert (report / CAMPAIGN_MANIFEST_JSON).exists()
        assert (report / CAMPAIGN_SCHEDULE_JSON).exists()
        assert (report / CAMPAIGN_ASSIGNMENTS_JSONL).exists()
        assert (report / ATTEMPTS_JSONL).exists()
        assert (report / METRICS_JSONL).exists()
        assert (report / ANALYSIS_INPUT_JSON).exists()
        assert (report / ANALYSIS_JSON).exists()
        assert (report / CAMPAIGN_STATUS_JSON).exists()

    def test_attempt_records_populate_campaign_fields(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        attempts_path = result.report_dir / ATTEMPTS_JSONL
        attempts_text = attempts_path.read_text().strip().splitlines()
        assert len(attempts_text) > 0
        for line in attempts_text:
            attempt = json.loads(line)
            assert attempt["model_cohort_id"] in _COHORT_IDS
            assert attempt["assignment_id"] != ""
            assert attempt["replicate_id"] in _REPLICATE_IDS
            assert attempt["assignment_order"] >= 0
            assert attempt["terminal_status"] == TerminalStatus.COMPLETED.value

    def test_campaign_status_transitions_to_finalized(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        status_path = result.report_dir / CAMPAIGN_STATUS_JSON
        status = json.loads(status_path.read_text())
        assert status["status"] == CampaignStatus.FINALIZED.value
        assert status["stop_reason"] == CampaignStopReason.COMPLETED.value


# ---------------------------------------------------------------------------
# Resume tests
# ---------------------------------------------------------------------------


class TestResume:
    def test_resume_skips_completed_assignments(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result1 = asyncio.run(runner.run())
        assert result1.status == CampaignStatus.FINALIZED

        # Second run on the same report dir should reuse the schedule and
        # not re-execute any assignments.
        runner2 = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        # Force the same report directory
        runner2._report_dir = result1.report_dir
        result2 = asyncio.run(runner2.run())

        assert result2.status == CampaignStatus.FINALIZED
        assert result2.report_dir == result1.report_dir
        # Same number of attempts (no re-execution)
        assert result2.terminal_attempt_count == result1.terminal_attempt_count

    def test_resume_reuses_persisted_schedule(self, tmp_path: Path):
        spec = _make_spec(randomization_seed=42)
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result1 = asyncio.run(runner.run())

        schedule1_text = (result1.report_dir / CAMPAIGN_SCHEDULE_JSON).read_text()

        runner2 = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        runner2._report_dir = result1.report_dir
        asyncio.run(runner2.run())

        schedule2_text = (result1.report_dir / CAMPAIGN_SCHEDULE_JSON).read_text()
        assert schedule1_text == schedule2_text


# ---------------------------------------------------------------------------
# Retry tests
# ---------------------------------------------------------------------------


class TestRetry:
    def test_infrastructure_failure_retried_then_succeeds(self, tmp_path: Path):
        spec = _make_spec(retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]))
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(fail_alternating=True),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        assert result.status == CampaignStatus.FINALIZED
        # Each assignment should have 2 attempts (1 failed + 1 success)
        attempts_text = (result.report_dir / ATTEMPTS_JSONL).read_text().strip().splitlines()
        expected_assignments = len(_TASK_IDS) * len(_COHORT_IDS) * len(_ARM_IDS) * len(_REPLICATE_IDS)
        assert len(attempts_text) == expected_assignments * 2

    def test_retry_chain_preserves_parent_links(self, tmp_path: Path):
        spec = _make_spec(retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]))
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(fail_alternating=True),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        attempts_text = (result.report_dir / ATTEMPTS_JSONL).read_text().strip().splitlines()
        attempts = [json.loads(line) for line in attempts_text]

        # Group by assignment_id
        by_assignment: dict[str, list[dict]] = {}
        for a in attempts:
            by_assignment.setdefault(a["assignment_id"], []).append(a)

        for assignment_attempts in by_assignment.values():
            assert len(assignment_attempts) == 2
            # First attempt has no parent, second has first as parent
            first = min(assignment_attempts, key=lambda a: a["attempt_id"])
            second = max(assignment_attempts, key=lambda a: a["attempt_id"])
            assert first["parent_attempt_id"] is None
            assert second["parent_attempt_id"] == first["attempt_id"]

    def test_non_retryable_failure_not_retried(self, tmp_path: Path):
        # MODEL_FAILED is not in retryable_terminal_statuses
        spec = _make_spec(retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]))
        runner = CampaignRunner(
            spec=spec,
            # SUT that returns empty answer -> MODEL_FAILED
            sut_factory=_make_fake_sut_factory(answer=""),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        attempts_text = (result.report_dir / ATTEMPTS_JSONL).read_text().strip().splitlines()
        # One attempt per assignment (no retry)
        expected_assignments = len(_TASK_IDS) * len(_COHORT_IDS) * len(_ARM_IDS) * len(_REPLICATE_IDS)
        assert len(attempts_text) == expected_assignments


# ---------------------------------------------------------------------------
# Budget stop tests
# ---------------------------------------------------------------------------


class TestBudgetStop:
    def test_budget_exhaustion_stops_campaign(self, tmp_path: Path):
        # Budget of 2 requests, but 8 assignments needed
        budget = ProviderBudget(max_usd=100.0, max_requests=2)
        spec = _make_spec(provider_budget=budget)
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        assert result.status == CampaignStatus.STOPPED
        assert result.stop_reason == CampaignStopReason.BUDGET_EXHAUSTED

    def test_budget_stop_materializes_terminal_outcomes(self, tmp_path: Path):
        budget = ProviderBudget(max_usd=100.0, max_requests=2)
        spec = _make_spec(provider_budget=budget)
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        attempts_text = (result.report_dir / ATTEMPTS_JSONL).read_text().strip().splitlines()
        attempts = [json.loads(line) for line in attempts_text]
        # All assignments get a terminal attempt (executed + budget-stopped)
        expected_assignments = len(_TASK_IDS) * len(_COHORT_IDS) * len(_ARM_IDS) * len(_REPLICATE_IDS)
        assert len(attempts) == expected_assignments

        # Some attempts should be INFRASTRUCTURE_FAILED (budget-stopped)
        infra_failed = [a for a in attempts if a["terminal_status"] == TerminalStatus.INFRASTRUCTURE_FAILED.value]
        assert len(infra_failed) > 0
        for a in infra_failed:
            assert a["missingness_or_failure"] == "budget_exhausted"

    def test_no_budget_completes_all(self, tmp_path: Path):
        spec = _make_spec(provider_budget=None)
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())
        assert result.status == CampaignStatus.FINALIZED


# ---------------------------------------------------------------------------
# Cohort drift tests
# ---------------------------------------------------------------------------


class TestCohortDrift:
    def test_cohort_drift_detected(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            # SUT returns a model that doesn't match the cohort
            sut_factory=_make_fake_sut_factory(drift_model_id="wrong-model:7b"),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        attempts_text = (result.report_dir / ATTEMPTS_JSONL).read_text().strip().splitlines()
        attempts = [json.loads(line) for line in attempts_text]
        # Drifted attempts should be MODEL_FAILED
        model_failed = [a for a in attempts if a["terminal_status"] == TerminalStatus.MODEL_FAILED.value]
        assert len(model_failed) > 0

    def test_provider_prefix_model_not_false_drift(self, tmp_path: Path):
        """Regression: DirectProviderSUT reports model as 'ollama:qwen3:8b'
        (provider:model_tag format) while the cohort declares 'qwen3:8b'.
        The old exact-string comparison marked every completed attempt as
        model_failed with cohort drift. The _model_matches helper accepts
        the provider:model prefix, so the campaign completes successfully.
        """
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(provider_prefix=True),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        attempts_text = (result.report_dir / ATTEMPTS_JSONL).read_text().strip().splitlines()
        attempts = [json.loads(line) for line in attempts_text]
        model_failed = [a for a in attempts if a["terminal_status"] == TerminalStatus.MODEL_FAILED.value]
        assert len(model_failed) == 0, (
            f"Provider-prefixed model strings were marked as cohort drift: "
            f"{len(model_failed)} model_failed attempts"
        )
        completed = [a for a in attempts if a["terminal_status"] == TerminalStatus.COMPLETED.value]
        assert len(completed) > 0, "No attempts completed successfully"


# ---------------------------------------------------------------------------
# Atomic writes tests
# ---------------------------------------------------------------------------


class TestAtomicWrites:
    def test_no_temp_files_left_after_run(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        # No .tmp files should remain
        tmp_files = list(result.report_dir.rglob("*.tmp"))
        assert len(tmp_files) == 0


# ---------------------------------------------------------------------------
# Canonical analysis tests
# ---------------------------------------------------------------------------


class TestCanonicalAnalysis:
    def test_analysis_written_before_campaign_error(self, tmp_path: Path):
        # Even if the campaign stops, analysis must be written
        budget = ProviderBudget(max_usd=100.0, max_requests=1)
        spec = _make_spec(provider_budget=budget)
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        assert result.status == CampaignStatus.STOPPED
        # Analysis must exist even though the campaign stopped
        assert (result.report_dir / ANALYSIS_INPUT_JSON).exists()
        assert (result.report_dir / ANALYSIS_JSON).exists()

    def test_analysis_input_contains_campaign_records(self, tmp_path: Path):
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        analysis_input_text = (result.report_dir / ANALYSIS_INPUT_JSON).read_text()
        analysis_input = json.loads(analysis_input_text)
        assert analysis_input["campaign_manifest"] is not None
        assert len(analysis_input["campaign_assignments"]) > 0
        assert analysis_input["execution_schedule"] is not None
        assert analysis_input["retry_policy"] is not None
        assert len(analysis_input["model_cohorts"]) > 0
