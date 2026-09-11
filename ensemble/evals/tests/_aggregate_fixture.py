# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Shared aggregate fixture builder for v5 publication tests.

Builds a minimal 4-child campaign-set aggregate using the connected
production path (CampaignRunner with fake SUT/grader), verifies each
child, builds the set index, runs aggregate verification, and returns
the typed plan, index, aggregate result, and child report directory
mapping. This is the connected aggregate handoff that v5 publication
tests consume instead of a single-report substitution.

The fixture uses two cohorts (candidate + anchor) so projection rows
carry real variant diversity. Each child has 2 tasks, 1 arm, 1
repetition, and 2 cohorts = 4 assignments per child, 16 total.
"""

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

from g8e_evals.arms import Arm
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
from g8e_evals.campaign_set import (
    CHILD_COUNT,
    CampaignChildIndexEntry,
    CampaignChildPlan,
    CampaignSetIndex,
    CampaignSetPlan,
    compute_campaign_set_index_hash,
    compute_campaign_set_plan_hash,
    compute_child_campaign_id,
    verify_campaign_set_aggregate,
)
from g8e_evals.campaign_verify import verify_campaign
from g8e_evals.constants import (
    ATTEMPTS_JSONL,
    CAMPAIGN_ASSIGNMENTS_JSONL,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.runner import CampaignRunner, CampaignSpec

from g8e_evals.analysis.canonical import (
    ClaimPolicy,
    ContinuousTestPolicy,
    PreregistrationConfig,
)

_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_INITIAL_STATE_ID = "no_initial-state-v1"

_CANDIDATE_VARIANT = "qwen3:8b"
_ANCHOR_VARIANT = "granite3.3:8b"
_CANDIDATE_COHORT = "cohort-candidate"
_ANCHOR_COHORT = "cohort-anchor"

_VALID_HASH_A = "a" * 64
_VALID_HASH_B = "b" * 64
_VALID_HASH_C = "c" * 64
_VALID_HASH_D = "d" * 64
_VALID_HASH_E = "e" * 64
_VALID_HASH_F = "f" * 64
_VALID_HASH_G = "g" * 64


@dataclass
class _FakeSUT:
    model_id: str
    answer: str = "This is a test answer with no commas."

    async def get_answer(self, task: Task) -> Response:
        return Response(answer=self.answer, model=self.model_id, arm=Arm.DIRECT)


@dataclass
class _FakeGrader:
    grader_id: str = "ifeval_subset_verifier"
    grader_version: str = "1.0.0"

    def grade(self, task: Task, response: Response) -> Score:
        return Score(task_id=task.id, passed=True, details=ScoreDetails())


def _make_two_cohort_spec(campaign_id: str, task_ids: list[str]) -> CampaignSpec:
    """Build a CampaignSpec with two cohorts (candidate + anchor)."""
    candidate_binding = RoleModelBinding(
        role="primary",
        model_id=_CANDIDATE_VARIANT,
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
        timeout_seconds=120.0,
        seed_capable=True,
    )
    anchor_binding = RoleModelBinding(
        role="primary",
        model_id=_ANCHOR_VARIANT,
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
        timeout_seconds=120.0,
        seed_capable=True,
    )
    candidate_cohort = ModelCohort(
        cohort_id=_CANDIDATE_COHORT,
        role_bindings=[candidate_binding],
        content_hash=compute_model_cohort_hash(_CANDIDATE_COHORT, [candidate_binding]),
    )
    anchor_cohort = ModelCohort(
        cohort_id=_ANCHOR_COHORT,
        role_bindings=[anchor_binding],
        content_hash=compute_model_cohort_hash(_ANCHOR_COHORT, [anchor_binding]),
    )
    task_assignment = TaskAssignmentManifest(
        task_assignment_id=f"task-assignment-{campaign_id}",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=task_ids,
        content_hash=compute_task_assignment_hash(
            f"task-assignment-{campaign_id}", "ifeval_subset", _DATASET_HASH, task_ids,
        ),
    )
    initial_state = InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH),
    )
    prereg = PreregistrationConfig(
        config_id=f"config-{campaign_id}",
        config_version="1.0.0",
        baseline_arm_id="direct",
        comparison_arm_ids=[],
        model_cohort_ids=[_CANDIDATE_COHORT, _ANCHOR_COHORT],
        task_assignment_id=f"task-assignment-{campaign_id}",
        initial_state_assignment_id=_INITIAL_STATE_ID,
        required_replicate_ids=["replicate-1"],
        required_replicate_count=1,
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )
    prompt_bundle = "\n".join(f"Prompt for {tid}" for tid in task_ids).encode()
    return CampaignSpec(
        campaign_id=campaign_id,
        release_version="v2.1.8",
        suite="ifeval_subset",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        dataset_hash=_DATASET_HASH,
        prompt_bundle_hash=hashlib.sha256(prompt_bundle).hexdigest(),
        grader_bundle_hash="g" * 64,
        preregistration=prereg,
        cohorts=[candidate_cohort, anchor_cohort],
        task_assignment=task_assignment,
        initial_state=initial_state,
        retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]),
        randomization_seed=42,
    )


def _make_tasks(task_ids: list[str]) -> list[Task]:
    tasks: list[Task] = []
    for tid in task_ids:
        tasks.append(Task(
            id=tid,
            prompt=f"Prompt for {tid}",
            metadata=TaskMetadata(
                benchmark="ifeval_subset",
                instruction_id_list=["punctuation:no_comma"],
                kwargs=[{"no_comma": True}],
            ),
        ))
    return tasks


def _fake_sut_factory(cohort: ModelCohort, arm: Arm):
    return _FakeSUT(model_id=cohort.role_bindings[0].model_id)


def _run_child_campaign(tmp_path: Path, campaign_id: str, task_ids: list[str]) -> Path:
    """Run a fake-provider child campaign and return the report directory."""
    spec = _make_two_cohort_spec(campaign_id, task_ids)
    runner = CampaignRunner(
        spec=spec,
        sut_factory=_fake_sut_factory,
        tasks=_make_tasks(task_ids),
        grader=_FakeGrader(),
        output_dir=tmp_path,
    )
    result = asyncio.run(runner.run())
    return result.report_dir


def _read_assignment_ids(report_dir: Path) -> list[str]:
    assignments_path = report_dir / CAMPAIGN_ASSIGNMENTS_JSONL
    if not assignments_path.exists():
        return []
    assignment_ids: list[str] = []
    for line in assignments_path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        record = json.loads(line)
        aid = record.get("assignment_id")
        if aid:
            assignment_ids.append(str(aid))
    return sorted(assignment_ids)


def _build_index_from_children(
    plan: CampaignSetPlan,
    child_report_dirs: dict[str, Path],
) -> CampaignSetIndex:
    """Build a CampaignSetIndex from actual child report directories."""
    from g8e_evals.campaign_set import (
        _compute_child_verification_report_hash,
        _recompute_report_checksum,
    )

    entries: list[CampaignChildIndexEntry] = []
    total = 0
    for cp in plan.child_plans:
        report_dir = child_report_dirs[cp.child_id]
        assignment_ids = _read_assignment_ids(report_dir)
        count = len(assignment_ids)
        total += count

        verification = verify_campaign(report_dir)
        child_verification_hash = _compute_child_verification_report_hash(report_dir)
        if child_verification_hash is None:
            child_verification_hash = "0" * 64
        report_checksum = _recompute_report_checksum(report_dir)
        if report_checksum is None:
            report_checksum = "0" * 64

        entries.append(CampaignChildIndexEntry(
            child_id=cp.child_id,
            report_campaign_id=verification.campaign_id,
            finalization_generation_hash=verification.verified_index_generation_hash,
            child_verification_report_hash=child_verification_hash,
            report_checksum=report_checksum,
            assignment_count=count,
        ))

    index = CampaignSetIndex.model_construct(
        set_id=plan.set_id,
        set_plan_hash=plan.content_hash,
        child_index_entries=entries,
        total_assignment_count=total,
        content_hash="0" * 64,
    )
    content_hash = compute_campaign_set_index_hash(index)
    return CampaignSetIndex(
        set_id=plan.set_id,
        set_plan_hash=plan.content_hash,
        child_index_entries=entries,
        total_assignment_count=total,
        content_hash=content_hash,
    )


def _build_plan(
    partition_task_ids: list[list[str]],
    expected_child_count: int,
    expected_total_count: int,
) -> CampaignSetPlan:
    """Build a CampaignSetPlan with custom expected counts for integration tests.

    Uses ``model_construct`` for the child plans to bypass the
    ``min_length=30`` constraint on ``partition_task_ids``; the plan-level
    validator still checks disjointness, coverage, and child ID derivation.
    """
    kwargs = {
        "set_id": "p12-rehearsal-set",
        "parent_campaign_id": "p12-rehearsal-parent",
        "parent_campaign_revision": "v2.1.8",
        "population_task_ids": [t for p in partition_task_ids for t in p],
        "population_hash": _VALID_HASH_A,
        "model_registry_hash": _VALID_HASH_B,
        "campaign_profile_hash": _VALID_HASH_C,
        "repetition_ids": ["replicate-1"],
        "seed": 42,
        "retry_policy_hash": _VALID_HASH_D,
        "budget_authority_hash": _VALID_HASH_E,
        "instrumentation_policy_hash": _VALID_HASH_F,
        "expected_record_policy_hash": _VALID_HASH_G,
        "orchestrator_environment_scope": "linux/amd64/cpu",
        "provider_environment_scope": "linux/amd64/remote-ollama",
        "child_revisions": [f"child-rev-{i}" for i in range(CHILD_COUNT)],
    }
    sorted_population = sorted(kwargs["population_task_ids"])
    child_plans: list[CampaignChildPlan] = []
    for i in range(CHILD_COUNT):
        partition = sorted(partition_task_ids[i])
        child_id = compute_child_campaign_id(
            parent_campaign_id=kwargs["parent_campaign_id"],
            partition_index=i,
            partition_task_ids=partition,
        )
        child_plans.append(CampaignChildPlan.model_construct(
            child_id=child_id,
            child_revision=kwargs["child_revisions"][i],
            partition_index=i,
            partition_task_ids=partition,
            expected_assignment_count=expected_child_count,
        ))
    plan = CampaignSetPlan.model_construct(
        set_id=kwargs["set_id"],
        set_version="1.0.0",
        parent_campaign_id=kwargs["parent_campaign_id"],
        parent_campaign_revision=kwargs["parent_campaign_revision"],
        child_plans=child_plans,
        population_task_ids=sorted_population,
        population_hash=kwargs["population_hash"],
        model_registry_hash=kwargs["model_registry_hash"],
        campaign_profile_hash=kwargs["campaign_profile_hash"],
        repetition_ids=kwargs["repetition_ids"],
        seed=kwargs["seed"],
        retry_policy_hash=kwargs["retry_policy_hash"],
        budget_authority_hash=kwargs["budget_authority_hash"],
        instrumentation_policy_hash=kwargs["instrumentation_policy_hash"],
        expected_record_policy_hash=kwargs["expected_record_policy_hash"],
        orchestrator_environment_scope=kwargs["orchestrator_environment_scope"],
        provider_environment_scope=kwargs["provider_environment_scope"],
        expected_child_assignment_count=expected_child_count,
        expected_total_assignment_count=expected_total_count,
        content_hash="0" * 64,
    )
    content_hash = compute_campaign_set_plan_hash(plan)
    return CampaignSetPlan.model_construct(
        set_id=plan.set_id,
        set_version=plan.set_version,
        parent_campaign_id=plan.parent_campaign_id,
        parent_campaign_revision=plan.parent_campaign_revision,
        child_plans=child_plans,
        population_task_ids=sorted_population,
        population_hash=plan.population_hash,
        model_registry_hash=plan.model_registry_hash,
        campaign_profile_hash=plan.campaign_profile_hash,
        repetition_ids=plan.repetition_ids,
        seed=plan.seed,
        retry_policy_hash=plan.retry_policy_hash,
        budget_authority_hash=plan.budget_authority_hash,
        instrumentation_policy_hash=plan.instrumentation_policy_hash,
        expected_record_policy_hash=plan.expected_record_policy_hash,
        orchestrator_environment_scope=plan.orchestrator_environment_scope,
        provider_environment_scope=plan.provider_environment_scope,
        expected_child_assignment_count=plan.expected_child_assignment_count,
        expected_total_assignment_count=plan.expected_total_assignment_count,
        content_hash=content_hash,
    )


@dataclass
class AggregateFixture:
    """Bundle of artifacts from a connected aggregate fixture run."""

    plan: CampaignSetPlan
    index: CampaignSetIndex
    aggregate_result: object
    child_report_dirs: dict[str, Path]


def build_aggregate_fixture(tmp_path: Path) -> AggregateFixture:
    """Build a connected 4-child aggregate fixture for v5 publication tests.

    Runs four deterministic child campaigns with the CampaignRunner and
    fake SUT/grader, builds the campaign-set plan and index, runs
    aggregate verification, and returns the typed plan, index,
    aggregate result, and child report directory mapping.

    The fixture has 2 tasks per child, 2 cohorts (candidate + anchor),
    1 arm, and 1 repetition = 4 assignments per child, 16 total.
    """
    partitions = [
        ["task-001", "task-002"],
        ["task-003", "task-004"],
        ["task-005", "task-006"],
        ["task-007", "task-008"],
    ]
    expected_child_count = 4
    expected_total_count = 16

    plan = _build_plan(partitions, expected_child_count, expected_total_count)

    child_report_dirs: dict[str, Path] = {}
    for i, cp in enumerate(plan.child_plans):
        child_dir = tmp_path / f"child-{i}"
        child_dir.mkdir(parents=True, exist_ok=True)
        report_dir = _run_child_campaign(child_dir, cp.child_id, cp.partition_task_ids)
        child_report_dirs[cp.child_id] = report_dir

    index = _build_index_from_children(plan, child_report_dirs)
    aggregate_result = verify_campaign_set_aggregate(plan, index, child_report_dirs)
    assert aggregate_result.ok, f"aggregate verification failed: {aggregate_result.failures}"

    return AggregateFixture(
        plan=plan,
        index=index,
        aggregate_result=aggregate_result,
        child_report_dirs=child_report_dirs,
    )
