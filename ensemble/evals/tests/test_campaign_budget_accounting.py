# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for campaign budget accounting.

Verifies that the campaign runner counts retries against
``max_requests``, enforces token and USD ceilings from observed SUT
usage when available, rejects unobservable required ceilings, and
computes a ``provider_budget_hash`` that covers all three ceilings
(``max_usd``, ``max_tokens``, ``max_requests``).
"""

from __future__ import annotations

import asyncio
import hashlib
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
from g8e_evals.index import ModelRole
from g8e_evals.harness import Response, Score, Task
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.runner import (
    BudgetCeiling,
    BudgetObservabilityPolicy,
    CampaignRunner,
    CampaignRunnerError,
    CampaignSpec,
    CampaignStopReason,
    compute_provider_budget_hash,
)
from g8e_evals.schema import ProviderBudget
from g8e_evals.sut.direct_provider import DirectCallEvidence, _DirectEvidenceWrapper


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_IDS = ["cohort-qwen3-8b", "cohort-granite-3.3-8b"]
_ARM_IDS = ["direct"]
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
    infrastructure error, modeling a transient failure that retries
    recover from.
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

    def close(self) -> None:
        pass


@dataclass
class FakeGrader:
    """Deterministic fake grader that always passes."""

    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_fake_sut_factory(
    answer: str = "This is a test answer with no commas.",
    fail_alternating: bool = False,
    provider_prefix: bool = False,
):
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
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
# Provider budget hash tests
# ---------------------------------------------------------------------------


class TestProviderBudgetHash:
    """AUTH-3: provider_budget_hash must cover all three ceilings."""

    def test_no_budget_hash_is_stable(self):
        h = compute_provider_budget_hash(None)
        assert h == hashlib.sha256(b"no_budget").hexdigest()

    def test_hash_covers_max_usd(self):
        b1 = ProviderBudget(max_usd=10.0)
        b2 = ProviderBudget(max_usd=20.0)
        assert compute_provider_budget_hash(b1) != compute_provider_budget_hash(b2)

    def test_hash_covers_max_tokens(self):
        b1 = ProviderBudget(max_usd=10.0, max_tokens=1000)
        b2 = ProviderBudget(max_usd=10.0, max_tokens=2000)
        assert compute_provider_budget_hash(b1) != compute_provider_budget_hash(b2)

    def test_hash_covers_max_requests(self):
        b1 = ProviderBudget(max_usd=10.0, max_requests=100)
        b2 = ProviderBudget(max_usd=10.0, max_requests=200)
        assert compute_provider_budget_hash(b1) != compute_provider_budget_hash(b2)

    def test_hash_covers_all_three_ceiling_combinations(self):
        b1 = ProviderBudget(max_usd=10.0, max_tokens=1000, max_requests=100)
        b2 = ProviderBudget(max_usd=10.0, max_tokens=1000, max_requests=200)
        b3 = ProviderBudget(max_usd=10.0, max_tokens=2000, max_requests=100)
        b4 = ProviderBudget(max_usd=20.0, max_tokens=1000, max_requests=100)
        hashes = {
            compute_provider_budget_hash(b1),
            compute_provider_budget_hash(b2),
            compute_provider_budget_hash(b3),
            compute_provider_budget_hash(b4),
        }
        assert len(hashes) == 4

    def test_hash_distinguishes_none_from_zero(self):
        b1 = ProviderBudget(max_usd=10.0, max_tokens=None, max_requests=None)
        b2 = ProviderBudget(max_usd=10.0, max_tokens=0, max_requests=0)
        assert compute_provider_budget_hash(b1) != compute_provider_budget_hash(b2)

    def test_hash_is_64_char_hex(self):
        h = compute_provider_budget_hash(ProviderBudget(max_usd=10.0))
        assert len(h) == 64
        assert all(c in "0123456789abcdef" for c in h)


# ---------------------------------------------------------------------------
# Retry counting tests
# ---------------------------------------------------------------------------


class TestRetryBudgetCounting:
    """AUTH-3: retries count against max_requests."""

    def test_retries_count_against_max_requests(self, tmp_path: Path):
        """With max_retries=1 and fail_alternating, each assignment uses 2
        provider calls (1 fail + 1 success). With 4 assignments (2 tasks x 2
        cohorts x 1 arm x 1 replicate) and max_requests=5, the budget should
        exhaust before all assignments complete because retries count."""
        budget = ProviderBudget(max_usd=1000.0, max_requests=5)
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(fail_alternating=True),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())

        assert result.status == CampaignStatus.STOPPED
        assert result.stop_reason == CampaignStopReason.BUDGET_EXHAUSTED

    def test_no_retries_completes_within_budget(self, tmp_path: Path):
        """Without failures, each assignment uses 1 provider call. With 4
        assignments and max_requests=4, the campaign completes."""
        budget = ProviderBudget(max_usd=1000.0, max_requests=4)
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
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
# Token enforcement from observed SUT usage
# ---------------------------------------------------------------------------


@dataclass
class TokenReportingSUT:
    """Fake SUT that reports token usage via DirectCallEvidence.

    Each call returns a response with ``total_token_count`` tokens and
    ``usage_reported=True`` so the budget tracker can enforce token
    ceilings from observed usage.
    """

    model_id: str
    answer: str = "This is a test answer with no commas."
    tokens_per_call: int = 100
    provider_prefix: bool = False

    async def get_answer(self, task: Task) -> Response:
        model = f"ollama:{self.model_id}" if self.provider_prefix else self.model_id
        evidence = DirectCallEvidence(
            provider="ollama",
            model=self.model_id,
            total_token_count=self.tokens_per_call,
            usage_reported=True,
        )
        return Response(
            answer=self.answer,
            model=model,
            arm=Arm.DIRECT,
            chat_evidence=_DirectEvidenceWrapper(evidence),
        )

    def close(self) -> None:
        pass


def _make_token_reporting_sut_factory(tokens_per_call: int = 100):
    def factory(cohort: ModelCohort, arm: Arm):
        model_id = cohort.role_bindings[0].model_id
        return TokenReportingSUT(model_id=model_id, tokens_per_call=tokens_per_call)
    return factory


class TestTokenEnforcementFromObservedUsage:
    """AUTH-3b: token ceilings enforced from observed SUT usage."""

    def test_token_ceiling_stops_campaign_when_observable(self, tmp_path: Path):
        """With tokens_observable=True and max_tokens=250, each assignment
        uses 100 tokens. After 3 assignments (300 tokens) the token ceiling
        is reached and the campaign stops with BUDGET_EXHAUSTED."""
        budget = ProviderBudget(max_usd=1000.0, max_tokens=250, max_requests=100)
        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
        spec = spec.model_copy(update={"budget_observability_policy": policy})
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_token_reporting_sut_factory(tokens_per_call=100),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())
        assert result.stop_reason == CampaignStopReason.BUDGET_EXHAUSTED

    def test_token_ceiling_not_enforced_when_excluded(self, tmp_path: Path):
        """With MAX_TOKENS excluded, the token ceiling is not enforced even
        when tokens are reported. The campaign completes normally."""
        budget = ProviderBudget(max_usd=1000.0, max_tokens=10, max_requests=100)
        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD, BudgetCeiling.MAX_TOKENS}),
        )
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
        spec = spec.model_copy(update={"budget_observability_policy": policy})
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_token_reporting_sut_factory(tokens_per_call=100),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())
        assert result.status == CampaignStatus.FINALIZED

    def test_token_ceiling_not_enforced_when_not_observable(self, tmp_path: Path):
        """With tokens_observable=False and MAX_TOKENS excluded (default
        policy), the token ceiling is not enforced. The campaign completes
        even though each call reports 100 tokens and max_tokens=10."""
        budget = ProviderBudget(max_usd=1000.0, max_tokens=10, max_requests=100)
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_token_reporting_sut_factory(tokens_per_call=100),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())
        assert result.status == CampaignStatus.FINALIZED


# ---------------------------------------------------------------------------
# Unobservable ceiling rejection tests
# ---------------------------------------------------------------------------


class TestUnobservableCeilingRejection:
    """AUTH-3c: unobservable required ceilings fail before execution."""

    def test_unobservable_max_tokens_rejected(self, tmp_path: Path):
        """A max_tokens ceiling with tokens_observable=False and MAX_TOKENS
        not excluded fails before the report directory is created."""
        budget = ProviderBudget(max_usd=1000.0, max_tokens=100, max_requests=100)
        policy = BudgetObservabilityPolicy(
            tokens_observable=False,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
        spec = spec.model_copy(update={"budget_observability_policy": policy})
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        with pytest.raises(CampaignRunnerError, match="max_tokens"):
            asyncio.run(runner.run())

    def test_unobservable_max_usd_rejected(self, tmp_path: Path):
        """A max_usd ceiling with usd_observable=False and MAX_USD not
        excluded fails before the report directory is created."""
        budget = ProviderBudget(max_usd=1000.0, max_requests=100)
        policy = BudgetObservabilityPolicy(
            tokens_observable=False,
            excluded_ceilings=frozenset(),
        )
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
        spec = spec.model_copy(update={"budget_observability_policy": policy})
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        with pytest.raises(CampaignRunnerError, match="max_usd"):
            asyncio.run(runner.run())

    def test_default_policy_allows_budget_with_only_max_requests(self, tmp_path: Path):
        """The default policy excludes MAX_USD and MAX_TOKENS, so a budget
        with only max_requests declared passes preflight."""
        budget = ProviderBudget(max_usd=1000.0, max_requests=4)
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())
        assert result.status == CampaignStatus.FINALIZED

    def test_observable_tokens_with_no_max_tokens_passes(self, tmp_path: Path):
        """tokens_observable=True with no max_tokens declared passes; the
        policy only rejects declared unobservable ceilings."""
        budget = ProviderBudget(max_usd=1000.0, max_requests=100)
        policy = BudgetObservabilityPolicy(
            tokens_observable=True,
            excluded_ceilings=frozenset({BudgetCeiling.MAX_USD}),
        )
        spec = _make_spec(
            provider_budget=budget,
            retry_policy=RetryPolicy(max_retries=0, retryable_terminal_statuses=["infrastructure_failed"]),
            replicate_ids=["replicate-1"],
        )
        spec = spec.model_copy(update={"budget_observability_policy": policy})
        runner = CampaignRunner(
            spec=spec,
            sut_factory=_make_fake_sut_factory(),
            tasks=_make_tasks(),
            grader=FakeGrader(),
            output_dir=tmp_path,
        )
        result = asyncio.run(runner.run())
        assert result.status == CampaignStatus.FINALIZED
