# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for the campaign-set aggregate verifier.

Verifies that ``verify_campaign_set_aggregate`` validates each child
first, then proves assignment uniqueness and complete 11,160-assignment
coverage across children. The tests build real child report directories
using the campaign runner with a fake SUT, construct a matching
campaign-set plan and index, and verify that the aggregate verifier
accepts a valid set and rejects every D9 mutation case.

Mutation cases tested:
- Tampered child verification content (broken index chain in one child)
- Wrong final index (index entry finalization hash mismatch)
- Non-finalized child (missing FINALIZATION generation)
- Authority mismatch (set_id mismatch between plan and index)
- Overlap (same assignment ID in two children)
- Gap (missing assignment in one child)
- Duplicate assignment (same assignment ID within one child)
- Extra assignment (extra assignment in one child)
- Incomplete expected records (missing required artifact)
- Missing report directory for a child
- Extra report directory for an unknown child
"""

from __future__ import annotations

import asyncio
import hashlib
import json
from dataclasses import dataclass
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
from g8e_evals.constants import (
    CAMPAIGN_ASSIGNMENTS_JSONL,
    CAMPAIGN_INDEX_JSONL,
    METRICS_JSONL,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.index import (
    compute_index_generation_hash,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.runner import (
    CampaignRunner,
    CampaignSpec,
)


_VALID_HASH = "a" * 64
_VALID_HASH_B = "b" * 64
_VALID_HASH_C = "c" * 64
_VALID_HASH_D = "d" * 64
_VALID_HASH_E = "e" * 64
_VALID_HASH_F = "f" * 64
_VALID_HASH_G = "g" * 64
_VALID_HASH_H = "h" * 64
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_INITIAL_STATE_ID = "no_initial-state-v1"


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


def _make_spec(campaign_id: str, task_ids: list[str]) -> CampaignSpec:
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
        task_assignment_id=f"task-assignment-{campaign_id}",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=task_ids,
        content_hash=compute_task_assignment_hash(f"task-assignment-{campaign_id}", "ifeval_subset", _DATASET_HASH, task_ids),
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
        model_cohort_ids=["cohort-qwen3-8b"],
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
        cohorts=[cohort],
        task_assignment=task_assignment,
        initial_state=initial_state,
        retry_policy=RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"]),
        randomization_seed=42,
    )


def _make_tasks(task_ids: list[str]) -> list[Task]:
    tasks = []
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
    spec = _make_spec(campaign_id, task_ids)
    runner = CampaignRunner(
        spec=spec,
        sut_factory=_fake_sut_factory,
        tasks=_make_tasks(task_ids),
        grader=_FakeGrader(),
        output_dir=tmp_path,
    )
    result = asyncio.run(runner.run())
    return result.report_dir


def _make_plan_kwargs(child_campaign_ids: list[str], partition_task_ids: list[list[str]]) -> dict:
    """Build plan kwargs with custom child IDs and partitions.

    For integration tests we use small partitions (2 tasks each) to keep
    test runtime short. The expected assignment counts in the plan are
    overridden to match the actual child campaign sizes.
    """
    return {
        "set_id": "ifeval-expanded-set",
        "parent_campaign_id": "ifeval-expanded-parent",
        "parent_campaign_revision": "v2.1.8",
        "population_task_ids": [t for p in partition_task_ids for t in p],
        "population_hash": _VALID_HASH,
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


def _build_plan_with_custom_counts(
    child_campaign_ids: list[str],
    partition_task_ids: list[list[str]],
    expected_child_count: int,
    expected_total_count: int,
) -> CampaignSetPlan:
    """Build a CampaignSetPlan with custom expected counts for integration tests.

    The integration tests use small partitions (2 tasks each, 1 repetition,
    1 variant, 1 arm) so the expected counts are much smaller than the
    production 2790/11160. This helper constructs a plan with the actual
    expected counts while preserving all D9 invariants. ``model_construct``
    is used for the child plans to bypass the ``min_length=30`` constraint
    on ``partition_task_ids``; the plan-level validator still checks
    disjointness, coverage, and child ID derivation.
    """
    kwargs = _make_plan_kwargs(child_campaign_ids, partition_task_ids)
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
        content_hash=content_hash,
    )


def _build_index_from_children(
    plan: CampaignSetPlan,
    child_report_dirs: dict[str, Path],
) -> CampaignSetIndex:
    """Build a CampaignSetIndex from actual child report directories.

    Reads each child's campaign-assignments.jsonl to count assignments
    and uses the child campaign ID as the report campaign identity.
    The finalization generation hash is derived from the child's
    verification report. The child verification report hash is computed
    from the persisted ``campaign-verification-report.json`` bytes. The
    report checksum is recomputed from ``attempts.jsonl`` and
    ``metrics.jsonl`` to match the aggregate verifier's recomputation.
    """
    from g8e_evals.campaign_set import (
        _compute_child_verification_report_hash,
        _recompute_report_checksum,
    )
    from g8e_evals.campaign_verify import verify_campaign

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


def _setup_four_children(tmp_path: Path) -> tuple[CampaignSetPlan, CampaignSetIndex, dict[str, Path]]:
    """Set up four child campaigns with 2 tasks each (1 variant, 1 arm, 1 rep = 2 assignments per child).

    Returns (plan, index, child_report_dirs).
    """
    partitions = [
        ["task-001", "task-002"],
        ["task-003", "task-004"],
        ["task-005", "task-006"],
        ["task-007", "task-008"],
    ]
    expected_child_count = 2
    expected_total_count = 8

    plan = _build_plan_with_custom_counts(
        child_campaign_ids=[],
        partition_task_ids=partitions,
        expected_child_count=expected_child_count,
        expected_total_count=expected_total_count,
    )

    child_report_dirs: dict[str, Path] = {}
    for i, cp in enumerate(plan.child_plans):
        child_dir = tmp_path / f"child-{i}"
        child_dir.mkdir(parents=True, exist_ok=True)
        report_dir = _run_child_campaign(child_dir, cp.child_id, cp.partition_task_ids)
        child_report_dirs[cp.child_id] = report_dir

    index = _build_index_from_children(plan, child_report_dirs)
    return plan, index, child_report_dirs


class TestAggregateVerifierPass:
    def test_valid_set_passes_aggregate_verification(self, tmp_path: Path):
        """A valid four-child set passes aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.ok
        assert len(result.failures) == 0
        assert result.assignment_uniqueness_ok
        assert result.coverage_ok
        assert result.total_assignments_verified == 8
        assert len(result.child_results) == CHILD_COUNT
        for cr in result.child_results:
            assert cr.ok

    def test_aggregate_result_has_set_identity(self, tmp_path: Path):
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.set_id == plan.set_id
        assert result.set_plan_hash == plan.content_hash

    def test_child_results_are_in_plan_order(self, tmp_path: Path):
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        for i, cr in enumerate(result.child_results):
            assert cr.child_id == plan.child_plans[i].child_id


class TestAggregateVerifierMutations:
    def test_tampered_child_verification_fails(self, tmp_path: Path):
        """Tampering with a child's index chain fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        index_path = child_dirs[first_child_id] / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        generations = [json.loads(line) for line in lines]
        if len(generations) >= 2:
            generations[1]["parent_generation_hash"] = "f" * 64
            generations[1]["content_hash"] = compute_index_generation_hash(
                generation_number=generations[1]["generation_number"],
                parent_generation_hash=generations[1]["parent_generation_hash"],
                creation_reason=generations[1]["creation_reason"],
                report_checksums=generations[1]["report_checksums"],
                assignment_dispositions=generations[1]["assignment_dispositions"],
            )
        index_path.write_text("\n".join(json.dumps(g) for g in generations) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert len(result.failures) > 0

    def test_wrong_final_index_hash_fails(self, tmp_path: Path):
        """An index entry with wrong finalization hash fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        entries = list(index.child_index_entries)
        entries[0] = CampaignChildIndexEntry(
            child_id=entries[0].child_id,
            report_campaign_id=entries[0].report_campaign_id,
            finalization_generation_hash="e" * 64,
            child_verification_report_hash=entries[0].child_verification_report_hash,
            report_checksum=entries[0].report_checksum,
            assignment_count=entries[0].assignment_count,
        )
        bad_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash=index.set_plan_hash,
            child_index_entries=entries,
            total_assignment_count=index.total_assignment_count,
            content_hash=index.content_hash,
        )
        result = verify_campaign_set_aggregate(plan, bad_index, child_dirs)
        assert not result.ok
        assert any("finalization_generation_hash" in f for f in result.failures)

    def test_authority_mismatch_set_id_fails(self, tmp_path: Path):
        """A set_id mismatch between plan and index fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        bad_index = CampaignSetIndex.model_construct(
            set_id="wrong-set-id",
            set_plan_hash=index.set_plan_hash,
            child_index_entries=index.child_index_entries,
            total_assignment_count=index.total_assignment_count,
            content_hash=index.content_hash,
        )
        result = verify_campaign_set_aggregate(plan, bad_index, child_dirs)
        assert not result.ok
        assert any("set_id" in f for f in result.failures)

    def test_missing_report_directory_fails(self, tmp_path: Path):
        """A missing report directory for a child fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        del child_dirs[first_child_id]
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("missing report director" in f for f in result.failures)

    def test_extra_report_directory_fails(self, tmp_path: Path):
        """An extra report directory for an unknown child fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        child_dirs["unknown-child-id"] = child_dirs[plan.child_plans[0].child_id]
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("unexpected report director" in f for f in result.failures)

    def test_wrong_assignment_count_in_index_fails(self, tmp_path: Path):
        """An index entry with wrong assignment count fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        entries = list(index.child_index_entries)
        entries[0] = CampaignChildIndexEntry(
            child_id=entries[0].child_id,
            report_campaign_id=entries[0].report_campaign_id,
            finalization_generation_hash=entries[0].finalization_generation_hash,
            child_verification_report_hash=entries[0].child_verification_report_hash,
            report_checksum=entries[0].report_checksum,
            assignment_count=9999,
        )
        bad_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash=index.set_plan_hash,
            child_index_entries=entries,
            total_assignment_count=index.total_assignment_count,
            content_hash=index.content_hash,
        )
        result = verify_campaign_set_aggregate(plan, bad_index, child_dirs)
        assert not result.ok
        assert any("assignment count" in f for f in result.failures)

    def test_duplicate_assignment_across_children_fails(self, tmp_path: Path):
        """Duplicate assignment IDs across children fail aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        second_child_id = plan.child_plans[1].child_id
        first_assignments = child_dirs[first_child_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        second_assignments = child_dirs[second_child_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        second_lines = second_assignments.read_text().strip().splitlines()
        first_lines = first_assignments.read_text().strip().splitlines()
        if first_lines and second_lines:
            first_record = json.loads(first_lines[0])
            second_record = json.loads(second_lines[0])
            second_record["assignment_id"] = first_record["assignment_id"]
            second_lines[0] = json.dumps(second_record)
            second_assignments.write_text("\n".join(second_lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("duplicate assignment" in f.lower() for f in result.failures)

    def test_missing_assignment_in_child_fails(self, tmp_path: Path):
        """Removing an assignment from a child's report fails coverage check."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_child_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            lines = lines[:-1]
            assignments_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok

    def test_non_finalized_child_fails(self, tmp_path: Path):
        """A child whose final index generation is not FINALIZATION fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        index_path = child_dirs[first_child_id] / CAMPAIGN_INDEX_JSONL
        lines = index_path.read_text().strip().splitlines()
        generations = [json.loads(line) for line in lines]
        if generations:
            last_gen = generations[-1]
            last_gen["creation_reason"] = "initial"
            last_gen["content_hash"] = compute_index_generation_hash(
                generation_number=last_gen["generation_number"],
                parent_generation_hash=last_gen["parent_generation_hash"],
                creation_reason=last_gen["creation_reason"],
                report_checksums=last_gen["report_checksums"],
                assignment_dispositions=last_gen["assignment_dispositions"],
            )
            generations[-1] = last_gen
        index_path.write_text("\n".join(json.dumps(g) for g in generations) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("FINALIZATION" in f for f in result.failures)

    def test_extra_assignment_in_child_fails(self, tmp_path: Path):
        """Adding an extra assignment to a child's report fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_child_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            extra_record = json.loads(lines[0])
            extra_record["assignment_id"] = "extra-assignment-id"
            lines.append(json.dumps(extra_record))
            assignments_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok

    def test_incomplete_expected_records_fails(self, tmp_path: Path):
        """Removing a required artifact from a child's report fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        metrics_path = child_dirs[first_child_id] / METRICS_JSONL
        if metrics_path.exists():
            metrics_path.unlink()
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("missing" in f.lower() and "metrics" in f.lower() for f in result.failures)

    def test_duplicate_assignment_within_child_fails(self, tmp_path: Path):
        """Duplicate assignment IDs within a single child fail aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_child_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            first_record = json.loads(lines[0])
            dup_record = json.loads(lines[0])
            dup_record["assignment_id"] = first_record["assignment_id"]
            lines.append(json.dumps(dup_record))
            assignments_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok

    def test_tampered_child_verification_report_hash_fails(self, tmp_path: Path):
        """An index entry with a wrong child_verification_report_hash fails.

        The verifier recomputes the hash from the persisted
        campaign-verification-report.json bytes and compares against the
        index entry. A tampered index hash is rejected.
        """
        plan, index, child_dirs = _setup_four_children(tmp_path)
        entries = list(index.child_index_entries)
        entries[0] = CampaignChildIndexEntry(
            child_id=entries[0].child_id,
            report_campaign_id=entries[0].report_campaign_id,
            finalization_generation_hash=entries[0].finalization_generation_hash,
            child_verification_report_hash="f" * 64,
            report_checksum=entries[0].report_checksum,
            assignment_count=entries[0].assignment_count,
        )
        bad_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash=index.set_plan_hash,
            child_index_entries=entries,
            total_assignment_count=index.total_assignment_count,
            content_hash=index.content_hash,
        )
        result = verify_campaign_set_aggregate(plan, bad_index, child_dirs)
        assert not result.ok
        assert any("verification report hash" in f for f in result.failures)

    def test_tampered_report_checksum_in_index_fails(self, tmp_path: Path):
        """An index entry with a wrong report_checksum fails.

        The verifier recomputes the checksum from attempts.jsonl and
        metrics.jsonl and compares against the index entry. A tampered
        index checksum is rejected.
        """
        plan, index, child_dirs = _setup_four_children(tmp_path)
        entries = list(index.child_index_entries)
        entries[0] = CampaignChildIndexEntry(
            child_id=entries[0].child_id,
            report_campaign_id=entries[0].report_campaign_id,
            finalization_generation_hash=entries[0].finalization_generation_hash,
            child_verification_report_hash=entries[0].child_verification_report_hash,
            report_checksum="e" * 64,
            assignment_count=entries[0].assignment_count,
        )
        bad_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash=index.set_plan_hash,
            child_index_entries=entries,
            total_assignment_count=index.total_assignment_count,
            content_hash=index.content_hash,
        )
        result = verify_campaign_set_aggregate(plan, bad_index, child_dirs)
        assert not result.ok
        assert any("report checksum" in f.lower() for f in result.failures)

    def test_tampered_attempts_fail_report_checksum(self, tmp_path: Path):
        """Tampering with attempts.jsonl makes the recomputed checksum mismatch.

        The verifier recomputes the checksum from the actual attempts
        and metrics files. A tampered attempts file produces a different
        checksum than the index entry declares.
        """
        from g8e_evals.constants import ATTEMPTS_JSONL

        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        attempts_path = child_dirs[first_child_id] / ATTEMPTS_JSONL
        if attempts_path.exists():
            lines = attempts_path.read_text().strip().splitlines()
            if lines:
                record = json.loads(lines[0])
                record["terminal_status"] = "tampered"
                lines[0] = json.dumps(record)
                attempts_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("report checksum" in f.lower() for f in result.failures)

    def test_alter_assignment_product_preserving_count_fails(self, tmp_path: Path):
        """Altering assignment task IDs while preserving count fails product check.

        Replace a child's assignments with records whose task IDs do not
        match the partition but keep the same count. The unique-assignment
        coverage check passes (count is preserved), but the product check
        rejects the wrong task set.
        """
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_child_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            new_lines: list[str] = []
            for line in lines:
                record = json.loads(line)
                record["task_id"] = "task-999"
                record["assignment_id"] = record["assignment_id"] + "-tampered"
                new_lines.append(json.dumps(record))
            assignments_path.write_text("\n".join(new_lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("task IDs do not match partition" in f for f in result.failures)

    def test_alter_assignment_repetition_preserving_count_fails(self, tmp_path: Path):
        """Altering assignment replicate IDs while preserving count fails product check.

        Replace a child's assignments with records whose replicate IDs do
        not match the plan's repetition IDs but keep the same count.
        """
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_child_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_child_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            new_lines: list[str] = []
            for line in lines:
                record = json.loads(line)
                record["replicate_id"] = "rep-999"
                record["assignment_id"] = record["assignment_id"] + "-tampered"
                new_lines.append(json.dumps(record))
            assignments_path.write_text("\n".join(new_lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("replicate IDs do not match plan" in f for f in result.failures)

    def test_index_total_assignment_count_mismatch_fails(self, tmp_path: Path):
        """An index with a total_assignment_count not matching the plan fails.

        validate_campaign_set_index now enforces that the index's
        total_assignment_count equals the plan's expected_total_assignment_count.
        """
        plan, index, child_dirs = _setup_four_children(tmp_path)
        bad_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash=index.set_plan_hash,
            child_index_entries=index.child_index_entries,
            total_assignment_count=999,
            content_hash=index.content_hash,
        )
        result = verify_campaign_set_aggregate(plan, bad_index, child_dirs)
        assert not result.ok
        assert any("total_assignment_count" in f for f in result.failures)

    def test_aggregate_result_is_content_addressed(self, tmp_path: Path):
        """The AggregateVerificationResult carries a content_hash that recomputes.

        A valid set produces a result whose content_hash matches
        compute_aggregate_verification_result_hash. Two valid runs over
        the same inputs produce the same content_hash.
        """
        from g8e_evals.campaign_set import compute_aggregate_verification_result_hash

        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.content_hash == compute_aggregate_verification_result_hash(result)
        assert result.content_hash != "0" * 64

    def test_aggregate_result_content_hash_changes_on_failure(self, tmp_path: Path):
        """A failing aggregate result has a different content_hash than a passing one.

        The content_hash covers the ok flag, failures, and all child
        results, so a failing verification produces a different hash.
        """
        plan, index, child_dirs = _setup_four_children(tmp_path)
        passing_result = verify_campaign_set_aggregate(plan, index, child_dirs)
        first_child_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_child_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            lines = lines[:-1]
            assignments_path.write_text("\n".join(lines) + "\n")
        failing_result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert passing_result.content_hash != failing_result.content_hash

    def test_aggregate_result_binds_set_index_hash(self, tmp_path: Path):
        """The AggregateVerificationResult binds the set_index_hash."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.set_index_hash == index.content_hash
