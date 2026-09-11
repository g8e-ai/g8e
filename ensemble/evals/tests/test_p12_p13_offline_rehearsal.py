# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for the P12/P13 offline rehearsal.

Builds four deterministic child fixtures with the same partition and
identity topology as P12, verifies each child, builds the set index,
runs aggregate verification, extracts ``RepetitionCell`` records from
child report attempts/metrics, feeds them into the model-comparison
engine, and confirms the output binds the real aggregate hash.

The fixture uses two cohorts (candidate + anchor) so the
model-comparison engine has real paired data. Each child covers 2
tasks x 2 cohorts x 1 arm x 1 repetition = 4 assignments, 16 total.

Mutation matrix (one mutation per test, each starting from the valid
fixture and mutating exactly one case): overlap, gap, duplicate, extra
assignment, wrong child ID, wrong profile, wrong environment, wrong
final index, wrong verification hash, missing repetition, and
unauthorized comparison. Every mutation fails for the expected typed
reason.

Production commands consume the typed ``AggregateVerificationResult``
rather than directory lists: ``ModelComparisonAuthorityHashes.from_aggregate_verification_result``
extracts the aggregate, plan, and index hashes from the typed result
and rejects a failed aggregate before the engine runs.
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
    compute_aggregate_verification_result_hash,
    compute_campaign_set_index_hash,
    compute_campaign_set_plan_hash,
    compute_child_campaign_id,
    verify_campaign_set_aggregate,
)
from g8e_evals.constants import (
    ATTEMPTS_JSONL,
    CAMPAIGN_ASSIGNMENTS_JSONL,
    METRICS_JSONL,
)
from g8e_evals.harness import Response, Score, Task
from g8e_evals.model_comparison import (
    MODEL_COMPARISON_AUTHORITY_VERSION,
    ClassAnchorBinding,
    ClaimGate,
    ComparisonFamilyKind,
    ComparisonPairKey,
    ComparisonTest,
    CorrectionMethod,
    MissingnessPolicy,
    ModelComparisonAuthorityHashes,
    ModelComparisonPreregistration,
    RepetitionCell,
    RepetitionReductionPolicy,
    compute_model_comparison,
    compute_model_comparison_hash,
)
from g8e_evals.models import ScoreDetails, TaskMetadata
from g8e_evals.registry import WeightClass
from g8e_evals.runner import CampaignRunner, CampaignSpec


_VALID_HASH = "a" * 64
_VALID_HASH_B = "b" * 64
_VALID_HASH_C = "c" * 64
_VALID_HASH_D = "d" * 64
_VALID_HASH_E = "e" * 64
_VALID_HASH_F = "f" * 64
_VALID_HASH_G = "g" * 64
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64
_INITIAL_STATE_ID = "no_initial-state-v1"

_CANDIDATE_MODEL = "qwen3:8b"
_CANDIDATE_COHORT = "cohort-qwen3-8b"
_CANDIDATE_VARIANT = "qwen3-8b"
_ANCHOR_MODEL = "granite-33-8b-instruct"
_ANCHOR_COHORT = "cohort-granite-33-8b"
_ANCHOR_VARIANT = "granite-33-8b-instruct"

_GLOBAL_FAMILY = "global-anchor-family"


# ---------------------------------------------------------------------------
# Fake SUT and grader
# ---------------------------------------------------------------------------


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


# ---------------------------------------------------------------------------
# Spec and task builders
# ---------------------------------------------------------------------------


def _make_role_binding(model_id: str) -> RoleModelBinding:
    return RoleModelBinding(
        role="primary",
        model_id=model_id,
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42),
        timeout_seconds=120.0,
        seed_capable=True,
    )


def _make_cohort(cohort_id: str, model_id: str) -> ModelCohort:
    rb = _make_role_binding(model_id)
    return ModelCohort(
        cohort_id=cohort_id,
        role_bindings=[rb],
        content_hash=compute_model_cohort_hash(cohort_id, [rb]),
    )


def _make_spec(campaign_id: str, task_ids: list[str]) -> CampaignSpec:
    candidate_cohort = _make_cohort(_CANDIDATE_COHORT, _CANDIDATE_MODEL)
    anchor_cohort = _make_cohort(_ANCHOR_COHORT, _ANCHOR_MODEL)
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


# ---------------------------------------------------------------------------
# Plan and index builders
# ---------------------------------------------------------------------------


def _make_plan_kwargs(child_campaign_ids: list[str], partition_task_ids: list[list[str]]) -> dict:
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
    **overrides,
) -> CampaignSetPlan:
    kwargs = _make_plan_kwargs(child_campaign_ids, partition_task_ids)
    kwargs.update(overrides)
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


def _setup_four_children(tmp_path: Path) -> tuple[CampaignSetPlan, CampaignSetIndex, dict[str, Path]]:
    """Set up four child campaigns with 2 tasks x 2 cohorts x 1 arm x 1 rep = 4 assignments per child."""
    partitions = [
        ["task-001", "task-002"],
        ["task-003", "task-004"],
        ["task-005", "task-006"],
        ["task-007", "task-008"],
    ]
    expected_child_count = 4
    expected_total_count = 16

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


# ---------------------------------------------------------------------------
# RepetitionCell extraction
# ---------------------------------------------------------------------------


def _extract_repetition_cells(child_report_dirs: dict[str, Path]) -> list[RepetitionCell]:
    """Extract RepetitionCell records from child report attempts and metrics.

    Maps each completed attempt to a RepetitionCell:
    - variant_id = the cohort's primary model_id (mapped from model_cohort_id)
    - task_id = attempt's task_id
    - repetition_id = attempt's replicate_id
    - passed = metric value == 1.0 for that attempt
    """
    cohort_to_variant = {
        _CANDIDATE_COHORT: _CANDIDATE_VARIANT,
        _ANCHOR_COHORT: _ANCHOR_VARIANT,
    }
    cells: list[RepetitionCell] = []
    for report_dir in child_report_dirs.values():
        attempts_path = report_dir / ATTEMPTS_JSONL
        metrics_path = report_dir / METRICS_JSONL
        if not attempts_path.exists() or not metrics_path.exists():
            continue
        attempt_by_id: dict[str, dict] = {}
        for line in attempts_path.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            attempt_by_id[record["attempt_id"]] = record
        metric_by_attempt: dict[str, float] = {}
        for line in metrics_path.read_text().splitlines():
            line = line.strip()
            if not line:
                continue
            record = json.loads(line)
            metric_by_attempt[record["attempt_id"]] = float(record["value"])
        for attempt_id, attempt in attempt_by_id.items():
            cohort_id = attempt.get("model_cohort_id", "")
            variant_id = cohort_to_variant.get(cohort_id)
            if variant_id is None:
                continue
            task_id = attempt["task_id"]
            rep_id = attempt.get("replicate_id", "replicate-1")
            value = metric_by_attempt.get(attempt_id)
            passed = bool(value == 1.0) if value is not None else None
            cells.append(RepetitionCell(
                variant_id=variant_id,
                task_id=task_id,
                repetition_id=rep_id,
                passed=passed,
            ))
    return cells


# ---------------------------------------------------------------------------
# Model comparison preregistration builder
# ---------------------------------------------------------------------------


def _make_preregistration(
    pair_keys: list[ComparisonPairKey] | None = None,
    repetition_count: int = 1,
) -> ModelComparisonPreregistration:
    pk = pair_keys if pair_keys is not None else [
        ComparisonPairKey(
            candidate_variant_id=_CANDIDATE_VARIANT,
            anchor_variant_id=_ANCHOR_VARIANT,
            family_kind=ComparisonFamilyKind.GLOBAL_ANCHOR,
            family_name=_GLOBAL_FAMILY,
        ),
    ]
    prereg = ModelComparisonPreregistration.model_construct(
        authority_id="model-comparison-authority-1",
        authority_version=MODEL_COMPARISON_AUTHORITY_VERSION,
        schema_version="1.0.0",
        global_anchor_variant_id=_ANCHOR_VARIANT,
        global_anchor_family_name=_GLOBAL_FAMILY,
        class_anchors=[
            ClassAnchorBinding(
                weight_class=WeightClass.HEAVY_SLM,
                anchor_variant_id=_ANCHOR_VARIANT,
                family_name=_GLOBAL_FAMILY,
            ),
        ],
        pair_keys=pk,
        repetition_count=repetition_count,
        repetition_reduction_policy=RepetitionReductionPolicy.MAJORITY_BINARY,
        missingness_policy=MissingnessPolicy.REJECT_PAIR,
        minimum_population=1,
        primary_test=ComparisonTest.EXACT_MCNEMAR,
        correction_method=CorrectionMethod.HOLM_BONFERRONI,
        significance_level=0.05,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=42,
        non_inferiority_margin=None,
        claim_gate=ClaimGate.DESCRIPTIVE_ONLY,
        environment_stratum="single-machine",
        content_hash="0" * 64,
    )
    expected = compute_model_comparison_hash(prereg)
    return ModelComparisonPreregistration.model_construct(
        authority_id="model-comparison-authority-1",
        authority_version=MODEL_COMPARISON_AUTHORITY_VERSION,
        schema_version="1.0.0",
        global_anchor_variant_id=_ANCHOR_VARIANT,
        global_anchor_family_name=_GLOBAL_FAMILY,
        class_anchors=[
            ClassAnchorBinding(
                weight_class=WeightClass.HEAVY_SLM,
                anchor_variant_id=_ANCHOR_VARIANT,
                family_name=_GLOBAL_FAMILY,
            ),
        ],
        pair_keys=pk,
        repetition_count=repetition_count,
        repetition_reduction_policy=RepetitionReductionPolicy.MAJORITY_BINARY,
        missingness_policy=MissingnessPolicy.REJECT_PAIR,
        minimum_population=1,
        primary_test=ComparisonTest.EXACT_MCNEMAR,
        correction_method=CorrectionMethod.HOLM_BONFERRONI,
        significance_level=0.05,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=42,
        non_inferiority_margin=None,
        claim_gate=ClaimGate.DESCRIPTIVE_ONLY,
        environment_stratum="single-machine",
        content_hash=expected,
    )


# ---------------------------------------------------------------------------
# Happy path tests
# ---------------------------------------------------------------------------


class TestRehearsalHappyPath:
    def test_full_pipeline_passes_aggregate_and_comparison(self, tmp_path: Path):
        """Build four children, verify aggregate, run model comparison, bind real hash."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.ok
        assert result.total_assignments_verified == 16
        assert len(result.child_results) == CHILD_COUNT
        for cr in result.child_results:
            assert cr.ok

        cells = _extract_repetition_cells(child_dirs)
        assert len(cells) == 16
        candidate_cells = [c for c in cells if c.variant_id == _CANDIDATE_VARIANT]
        anchor_cells = [c for c in cells if c.variant_id == _ANCHOR_VARIANT]
        assert len(candidate_cells) == 8
        assert len(anchor_cells) == 8

        prereg = _make_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=_VALID_HASH_C,
            registry_hash=_VALID_HASH_B,
            benchmark_population_hash=_VALID_HASH,
            metric_registry_hash=_VALID_HASH_G,
        )
        output = compute_model_comparison(prereg, cells, hashes)
        assert output.aggregate_verification_hash == result.content_hash
        assert output.campaign_set_plan_hash == result.set_plan_hash
        assert output.campaign_set_index_hash == result.set_index_hash
        assert len(output.results) == 1
        cmp_result = output.results[0]
        assert cmp_result.candidate_variant_id == _CANDIDATE_VARIANT
        assert cmp_result.anchor_variant_id == _ANCHOR_VARIANT
        assert cmp_result.paired_task_count == 8

    def test_aggregate_result_is_content_addressed(self, tmp_path: Path):
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.content_hash == compute_aggregate_verification_result_hash(result)
        assert result.content_hash != "0" * 64

    def test_two_runs_produce_same_comparison_output(self, tmp_path: Path):
        """Determinism: two runs over the same fixture produce the same output hash."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        cells = _extract_repetition_cells(child_dirs)
        prereg = _make_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=_VALID_HASH_C,
            registry_hash=_VALID_HASH_B,
            benchmark_population_hash=_VALID_HASH,
            metric_registry_hash=_VALID_HASH_G,
        )
        output1 = compute_model_comparison(prereg, cells, hashes)
        output2 = compute_model_comparison(prereg, cells, hashes)
        assert output1.content_hash == output2.content_hash


# ---------------------------------------------------------------------------
# Mutation matrix
# ---------------------------------------------------------------------------


class TestRehearsalMutations:
    def test_overlap_assignment_across_children_fails(self, tmp_path: Path):
        """Same assignment ID in two children fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_id = plan.child_plans[0].child_id
        second_id = plan.child_plans[1].child_id
        first_assignments = child_dirs[first_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        second_assignments = child_dirs[second_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        first_lines = first_assignments.read_text().strip().splitlines()
        second_lines = second_assignments.read_text().strip().splitlines()
        first_record = json.loads(first_lines[0])
        second_record = json.loads(second_lines[0])
        second_record["assignment_id"] = first_record["assignment_id"]
        second_lines[0] = json.dumps(second_record)
        second_assignments.write_text("\n".join(second_lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok
        assert any("duplicate assignment" in f.lower() for f in result.failures)

    def test_gap_missing_assignment_fails(self, tmp_path: Path):
        """Removing an assignment from a child fails coverage check."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            lines = lines[:-1]
            assignments_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok

    def test_duplicate_assignment_within_child_fails(self, tmp_path: Path):
        """Duplicate assignment IDs within one child fail aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            dup_record = json.loads(lines[0])
            lines.append(json.dumps(dup_record))
            assignments_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok

    def test_extra_assignment_fails(self, tmp_path: Path):
        """Adding an extra assignment to a child fails aggregate verification."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            extra_record = json.loads(lines[0])
            extra_record["assignment_id"] = "extra-assignment-id"
            lines.append(json.dumps(extra_record))
            assignments_path.write_text("\n".join(lines) + "\n")
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not result.ok

    def test_wrong_child_id_in_index_fails(self, tmp_path: Path):
        """An index entry whose child_id does not match the plan fails."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        entries = list(index.child_index_entries)
        entries[0] = CampaignChildIndexEntry(
            child_id="wrong-child-id",
            report_campaign_id=entries[0].report_campaign_id,
            finalization_generation_hash=entries[0].finalization_generation_hash,
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
        assert any("child IDs" in f for f in result.failures)

    def test_wrong_profile_hash_fails(self, tmp_path: Path):
        """A plan with a wrong campaign_profile_hash produces a different plan hash that the index rejects."""
        _plan, index, child_dirs = _setup_four_children(tmp_path)
        mutated_plan = _build_plan_with_custom_counts(
            child_campaign_ids=[],
            partition_task_ids=[
                ["task-001", "task-002"],
                ["task-003", "task-004"],
                ["task-005", "task-006"],
                ["task-007", "task-008"],
            ],
            expected_child_count=4,
            expected_total_count=16,
            campaign_profile_hash="z" * 64,
        )
        result = verify_campaign_set_aggregate(mutated_plan, index, child_dirs)
        assert not result.ok
        assert any("set_plan_hash" in f for f in result.failures)

    def test_wrong_environment_scope_fails(self, tmp_path: Path):
        """A plan with a mutated environment scope produces a different plan hash that the index rejects."""
        _plan, index, child_dirs = _setup_four_children(tmp_path)
        mutated_plan = _build_plan_with_custom_counts(
            child_campaign_ids=[],
            partition_task_ids=[
                ["task-001", "task-002"],
                ["task-003", "task-004"],
                ["task-005", "task-006"],
                ["task-007", "task-008"],
            ],
            expected_child_count=4,
            expected_total_count=16,
            orchestrator_environment_scope="linux/arm64/gpu",
        )
        result = verify_campaign_set_aggregate(mutated_plan, index, child_dirs)
        assert not result.ok
        assert any("set_plan_hash" in f for f in result.failures)

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

    def test_wrong_verification_report_hash_fails(self, tmp_path: Path):
        """An index entry with a wrong child_verification_report_hash fails."""
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

    def test_missing_repetition_fails(self, tmp_path: Path):
        """A child whose assignments omit one repetition ID from the plan fails the product check."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_id] / CAMPAIGN_ASSIGNMENTS_JSONL
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

    def test_unauthorized_comparison_rejected(self, tmp_path: Path):
        """Model-comparison engine rejects cells for a variant not in the preregistration."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.ok
        cells = _extract_repetition_cells(child_dirs)
        unauthorized_cells = [
            RepetitionCell(
                variant_id="unauthorized-variant",
                task_id="task-001",
                repetition_id="replicate-1",
                passed=True,
            )
        ]
        all_cells = cells + unauthorized_cells
        prereg = _make_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=_VALID_HASH_C,
            registry_hash=_VALID_HASH_B,
            benchmark_population_hash=_VALID_HASH,
            metric_registry_hash=_VALID_HASH_G,
        )
        with pytest.raises(ValueError, match="unauthorized variant"):
            compute_model_comparison(prereg, all_cells, hashes)


# ---------------------------------------------------------------------------
# Typed aggregate consumption tests
# ---------------------------------------------------------------------------


class TestTypedAggregateConsumption:
    def test_comparison_output_binds_real_aggregate_hash(self, tmp_path: Path):
        """compute_model_comparison with from_aggregate_verification_result binds the real hash."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert result.ok
        cells = _extract_repetition_cells(child_dirs)
        prereg = _make_preregistration()
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=_VALID_HASH_C,
            registry_hash=_VALID_HASH_B,
            benchmark_population_hash=_VALID_HASH,
            metric_registry_hash=_VALID_HASH_G,
        )
        output = compute_model_comparison(prereg, cells, hashes)
        assert output.aggregate_verification_hash == result.content_hash
        assert output.campaign_set_plan_hash == result.set_plan_hash
        assert output.campaign_set_index_hash == result.set_index_hash

    def test_failed_aggregate_rejected_before_engine(self, tmp_path: Path):
        """A failed aggregate (ok=False) is rejected by from_aggregate_verification_result."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        first_id = plan.child_plans[0].child_id
        assignments_path = child_dirs[first_id] / CAMPAIGN_ASSIGNMENTS_JSONL
        lines = assignments_path.read_text().strip().splitlines()
        if lines:
            lines = lines[:-1]
            assignments_path.write_text("\n".join(lines) + "\n")
        failing_result = verify_campaign_set_aggregate(plan, index, child_dirs)
        assert not failing_result.ok
        with pytest.raises(ValueError, match="failed aggregate"):
            ModelComparisonAuthorityHashes.from_aggregate_verification_result(
                failing_result,
                profile_hash=_VALID_HASH_C,
                registry_hash=_VALID_HASH_B,
                benchmark_population_hash=_VALID_HASH,
                metric_registry_hash=_VALID_HASH_G,
            )

    def test_aggregate_hash_not_caller_supplied(self, tmp_path: Path):
        """The aggregate verification hash comes from the typed result, not caller input."""
        plan, index, child_dirs = _setup_four_children(tmp_path)
        result = verify_campaign_set_aggregate(plan, index, child_dirs)
        hashes = ModelComparisonAuthorityHashes.from_aggregate_verification_result(
            result,
            profile_hash=_VALID_HASH_C,
            registry_hash=_VALID_HASH_B,
            benchmark_population_hash=_VALID_HASH,
            metric_registry_hash=_VALID_HASH_G,
        )
        assert hashes.aggregate_verification_hash == result.content_hash
        assert hashes.aggregate_verification_hash != _VALID_HASH
