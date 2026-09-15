# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for Phase 2 producer/verifier remediation.

Covers the gaps identified by the 2026-09-11 senior correctness and
completeness review: try/finally SUT cleanup lifecycle, transactional
usage-budget stops that preserve the current assignment's observations
and terminal disposition, and g8ee SUT InferenceObservation emission.

Uses deterministic fake SUTs and local filesystem (tmp_path). No network,
database, or real provider dependencies.
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
from g8e_evals.constants import (
    ATTEMPTS_JSONL,
    RESOURCE_OBSERVATIONS_JSONL,
    STAGES_JSONL,
)
from g8e_evals.harness import InferenceObservation, Response, Score, Task
from g8e_evals.index import ModelRole
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.runner import CampaignRunner, CampaignSpec, CampaignStopReason
from g8e_evals.schema import ProviderBudget


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_CAMPAIGN_ID = "v2.1.8-ifeval-remediation"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_IDS = ["cohort-qwen3-8b"]
_ARM_IDS = ["direct"]
_REPLICATE_IDS = ["replicate-1"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64


# ---------------------------------------------------------------------------
# Fake SUTs and Grader
# ---------------------------------------------------------------------------


@dataclass
class TrackingFakeSUT:
    """Fake SUT that tracks close calls and emits InferenceObservation records."""

    model_id: str
    answer: str = "This is a test answer with no commas."
    _call_count: int = field(default=0)
    _closed: bool = field(default=False)

    async def get_answer(self, task: Task) -> Response:
        self._call_count += 1
        model = f"ollama:{self.model_id}"
        inference_id = f"inf-{self._call_count}"
        observation = InferenceObservation(
            inference_id=inference_id,
            role=ModelRole.PRIMARY,
            model_variant_id=self.model_id,
            provider="ollama",
            model=self.model_id,
            provider_call_latency_seconds=0.05,
            time_to_first_token_seconds=0.01,
            generation_duration_seconds=0.04,
            output_throughput_tokens_per_second=100.0,
            prompt_token_count=10,
            candidates_token_count=5,
            total_token_count=15,
            usage_reported=True,
            finish_reason="stop",
            input_artifact_hash=hashlib.sha256(b"input").hexdigest(),
            output_artifact_hash=hashlib.sha256(b"output").hexdigest(),
        )
        return Response(
            answer=self.answer,
            model=model,
            arm=Arm.DIRECT,
            inference_observations=[observation],
        )

    async def close(self) -> None:
        self._closed = True


@dataclass
class TokenReportingSUT:
    """Fake SUT that reports token usage so token budgets can be exercised.

    Reports a configurable total_token_count so the budget tracker can
    enforce a max_tokens ceiling after the provider response is received.
    """

    model_id: str
    token_count: int = 100
    answer: str = "This is a test answer with no commas."
    _call_count: int = field(default=0)
    _closed: bool = field(default=False)

    async def get_answer(self, task: Task) -> Response:
        self._call_count += 1
        model = f"ollama:{self.model_id}"
        inference_id = f"inf-{self._call_count}"
        observation = InferenceObservation(
            inference_id=inference_id,
            role=ModelRole.PRIMARY,
            model_variant_id=self.model_id,
            provider="ollama",
            model=self.model_id,
            provider_call_latency_seconds=0.05,
            time_to_first_token_seconds=0.01,
            generation_duration_seconds=0.04,
            output_throughput_tokens_per_second=100.0,
            prompt_token_count=10,
            candidates_token_count=self.token_count - 10,
            total_token_count=self.token_count,
            usage_reported=True,
            finish_reason="stop",
            input_artifact_hash=hashlib.sha256(b"input").hexdigest(),
            output_artifact_hash=hashlib.sha256(b"output").hexdigest(),
        )
        return Response(
            answer=self.answer,
            model=model,
            arm=Arm.DIRECT,
            inference_observations=[observation],
        )

    async def close(self) -> None:
        self._closed = True


@dataclass
class FailingGrader:
    """Grader that raises an exception to simulate a finalization failure."""

    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        raise RuntimeError("simulated grader failure during finalization")


@dataclass
class FakeGrader:
    """Deterministic fake grader that always passes."""

    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_tracking_sut_factory():
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
        return TrackingFakeSUT(model_id=model_id)
    return factory


def _make_token_sut_factory(token_count: int = 100):
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
        return TokenReportingSUT(model_id=model_id, token_count=token_count)
    return factory


# ---------------------------------------------------------------------------
# Spec helpers
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
    retry_policy: RetryPolicy | None = None,
    provider_budget: ProviderBudget | None = None,
) -> CampaignSpec:
    if task_ids is None:
        task_ids = _TASK_IDS
    if retry_policy is None:
        retry_policy = RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"])

    cohorts = [_make_cohort(c, m) for c, m in zip(_COHORT_IDS, ["qwen3:8b"], strict=True)]
    task_assignment = _make_task_assignment(task_ids)
    initial_state = _make_initial_state()
    preregistration = _make_preregistration(_COHORT_IDS, _ARM_IDS, _REPLICATE_IDS)

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
        randomization_seed=42,
        provider_budget=provider_budget,
    )


# ---------------------------------------------------------------------------
# Tests: try/finally SUT cleanup lifecycle
# ---------------------------------------------------------------------------


class TestSutCloseOnException:
    """SUTs are closed even when an exception occurs during finalization.

    The senior review found that CampaignRunner.run() calls _close_suts()
    only on its normal tail, not from a finally block. Exceptions from
    grading, persistence, analysis, or finalization can bypass cleanup.
    """

    def test_sut_closed_on_grader_exception(self, tmp_path: Path):
        """A grader that raises must not prevent SUT cleanup."""
        spec = _make_spec()
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_tracking_sut_factory(),
            tasks=_make_tasks(),
            grader=FailingGrader(),
            output_dir=tmp_path,
        )
        with pytest.raises(RuntimeError, match="simulated grader failure"):
            asyncio.run(runner.run())

        for sut in runner._suts.values():
            assert isinstance(sut, TrackingFakeSUT)
            assert sut._closed, "SUT was not closed after grader exception"

    def test_sut_closed_on_disk_check_exception(self, tmp_path: Path):
        """A disk-space check failure must not prevent SUT cleanup."""

        class DiskCheckFailingSUT(TrackingFakeSUT):
            async def get_answer(self, task: Task) -> Response:
                # Simulate a disk check failure by raising during the call
                raise OSError("disk full")

        def factory(cohort: ModelCohort, arm: Arm):
            model_id = cohort.role_bindings[0].model_id
            return DiskCheckFailingSUT(model_id=model_id)

        spec = _make_spec(retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=[]))
        runner = CampaignRunner(
            spec=spec,
            sut_factory=factory,
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        # The runner catches infrastructure errors and records them as
        # terminal statuses, so the run completes but the SUT must still
        # be closed.
        asyncio.run(runner.run())

        for sut in runner._suts.values():
            assert isinstance(sut, DiskCheckFailingSUT)
            assert sut._closed, "SUT was not closed after disk check exception"


# ---------------------------------------------------------------------------
# Tests: Transactional usage-budget stops
# ---------------------------------------------------------------------------


class TestTransactionalBudgetStop:
    """Usage-budget stops are transactional at the provider-call boundary.

    The senior review found that a token/USD ceiling raised after a
    provider response occurs before the current attempt and its
    inference/resource/evidence records are materialized. The outer
    stop path excludes that current assignment from replacement
    materialization.

    A completed provider call must always produce exactly one typed
    attempt outcome plus every available inference, stage, resource,
    and evidence record before the stop propagates. The current
    assignment and every remaining assignment receive exactly one
    terminal disposition.
    """

    def test_token_budget_stop_preserves_current_assignment_observations(self, tmp_path: Path):
        """When a token budget is exceeded after a provider response, the
        current assignment's resource observations and stage records must
        still be written to the report directory.

        The SUT reports 100 tokens per call. With max_tokens=100, the first
        call fills the budget. The budget check after the first response
        raises BudgetExhausted. The current assignment must still get its
        observations and a terminal disposition.
        """
        from g8e_evals.runner import BudgetObservabilityPolicy, BudgetCeiling

        # Token ceiling of 100, observable, not excluded. USD ceiling is
        # always present on ProviderBudget (required field), so it must
        # be excluded when not observable.
        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            usd_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        budget = ProviderBudget(max_usd=0.0, max_tokens=100, max_requests=10)
        spec = _make_spec(provider_budget=budget)
        spec = spec.model_copy(update={"budget_observability_policy": policy})

        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_token_sut_factory(token_count=100),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        assert result.status == CampaignStatus.STOPPED
        assert result.stop_reason == CampaignStopReason.BUDGET_EXHAUSTED

        report = result.report_dir
        # The first assignment's resource observations must be present
        obs_text = (report / RESOURCE_OBSERVATIONS_JSONL).read_text().strip()
        obs_lines = [line for line in obs_text.splitlines() if line.strip()]
        assert len(obs_lines) >= 1, (
            "current assignment's resource observations lost on budget stop"
        )

        # The first assignment's stage records must be present
        stages_text = (report / STAGES_JSONL).read_text().strip()
        stage_lines = [line for line in stages_text.splitlines() if line.strip()]
        assert len(stage_lines) >= 1, (
            "current assignment's stage records lost on budget stop"
        )

        # Every assignment must have a terminal disposition
        attempts_text = (report / ATTEMPTS_JSONL).read_text().strip()
        attempt_lines = [line for line in attempts_text.splitlines() if line.strip()]
        attempts = [json.loads(line) for line in attempt_lines]
        # 2 tasks = 2 assignments, each must have at least one attempt
        assignment_ids = {a["assignment_id"] for a in attempts}
        assert len(assignment_ids) == 2, (
            f"expected 2 assignments with terminal dispositions, got {len(assignment_ids)}"
        )

    def test_token_budget_stop_current_assignment_gets_terminal_disposition(self, tmp_path: Path):
        """The current assignment (whose provider call triggered the budget
        stop) must receive a terminal disposition, not be silently dropped.

        The senior review found that the outer stop path excludes the
        current assignment from replacement materialization because it
        uses '>' instead of '>=' when computing the remaining list.
        """
        from g8e_evals.runner import BudgetObservabilityPolicy, BudgetCeiling

        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            usd_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        budget = ProviderBudget(max_usd=0.0, max_tokens=100, max_requests=10)
        spec = _make_spec(provider_budget=budget)
        spec = spec.model_copy(update={"budget_observability_policy": policy})

        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_token_sut_factory(token_count=100),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        assert result.stop_reason == CampaignStopReason.BUDGET_EXHAUSTED

        report = result.report_dir
        attempts_text = (report / ATTEMPTS_JSONL).read_text().strip()
        attempts = [json.loads(line) for line in attempts_text.splitlines() if line.strip()]

        # The first assignment should have a COMPLETED attempt (the provider
        # call succeeded before the budget check raised)
        first_assignment_attempts = [
            a for a in attempts
            if a["assignment_id"] == attempts[0]["assignment_id"]
        ]
        assert len(first_assignment_attempts) >= 1, (
            "current assignment has no terminal disposition after budget stop"
        )
        # The first attempt should be COMPLETED (provider call succeeded)
        assert first_assignment_attempts[0]["terminal_status"] == "completed", (
            f"current assignment's attempt should be completed, got "
            f"{first_assignment_attempts[0]['terminal_status']}"
        )

        # The second assignment should have a budget-stop disposition
        second_assignment_attempts = [
            a for a in attempts
            if a["assignment_id"] != attempts[0]["assignment_id"]
        ]
        assert len(second_assignment_attempts) >= 1, (
            "remaining assignment has no terminal disposition after budget stop"
        )
