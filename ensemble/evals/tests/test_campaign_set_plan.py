# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the campaign-set plan, index, and aggregate
verifier (D9 four-child campaign-set for the expanded IFEval campaign).

Verifies that the plan validates all D9 invariants (exactly four
disjoint 30-task partitions whose union equals the 120-task authority,
child IDs derived under the frozen identity rule, expected assignment
counts), the index binds accepted report identities by hash (not path),
and the aggregate verifier rejects every mutation case including
tampered child verification content, wrong final index, non-finalized
child, authority mismatch, overlap, gap, duplicate assignment, extra
assignment, and incomplete expected records.

No external dependencies (no files, network, or DB). The aggregate
verifier tests use stubbed child report directories only where the
integration tier is appropriate; unit tests focus on plan/index model
validation and the pure validation functions.
"""

from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

from g8e_evals.campaign_set import (
    CAMPAIGN_SET_SCHEMA_VERSION,
    CHILD_COUNT,
    EXPECTED_CHILD_ASSIGNMENT_COUNT,
    EXPECTED_TOTAL_ASSIGNMENT_COUNT,
    REPETITION_COUNT,
    TASKS_PER_CHILD,
    TOTAL_IFEVAL_TASKS,
    AggregateVerificationResult,
    CampaignChildIndexEntry,
    CampaignChildPlan,
    CampaignSetIndex,
    CampaignSetPlan,
    ChildVerificationResult,
    build_campaign_set_plan,
    compute_aggregate_verification_result_hash,
    compute_campaign_set_index_hash,
    compute_child_campaign_id,
    compute_dry_run_plan,
    validate_campaign_set_index,
    validate_campaign_set_plan,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64
_VALID_HASH_B = "b" * 64
_VALID_HASH_C = "c" * 64
_VALID_HASH_D = "d" * 64
_VALID_HASH_E = "e" * 64
_VALID_HASH_F = "f" * 64
_VALID_HASH_G = "g" * 64
_VALID_HASH_H = "h" * 64
_ZERO_HASH = "0" * 64


def _make_population_task_ids() -> list[str]:
    """Return 120 sorted unique task IDs for the IFEval population."""
    return [f"ifeval-{i:04d}" for i in range(TOTAL_IFEVAL_TASKS)]


def _make_repetition_ids() -> list[str]:
    return ["rep-1", "rep-2", "rep-3"]


def _make_child_revisions() -> list[str]:
    return [f"child-rev-{i}" for i in range(CHILD_COUNT)]


def _make_plan_kwargs() -> dict:
    """Return valid kwargs for build_campaign_set_plan."""
    return {
        "set_id": "ifeval-expanded-set",
        "parent_campaign_id": "ifeval-expanded-parent",
        "parent_campaign_revision": "v2.1.8",
        "population_task_ids": _make_population_task_ids(),
        "population_hash": _VALID_HASH,
        "model_registry_hash": _VALID_HASH_B,
        "campaign_profile_hash": _VALID_HASH_C,
        "repetition_ids": _make_repetition_ids(),
        "seed": 42,
        "retry_policy_hash": _VALID_HASH_D,
        "budget_authority_hash": _VALID_HASH_E,
        "instrumentation_policy_hash": _VALID_HASH_F,
        "expected_record_policy_hash": _VALID_HASH_G,
        "orchestrator_environment_scope": "linux/amd64/cpu",
        "provider_environment_scope": "linux/amd64/remote-ollama",
        "child_revisions": _make_child_revisions(),
    }


def _make_plan() -> CampaignSetPlan:
    return build_campaign_set_plan(**_make_plan_kwargs())


def _make_index(plan: CampaignSetPlan) -> CampaignSetIndex:
    """Build a valid CampaignSetIndex matching the plan."""
    entries: list[CampaignChildIndexEntry] = []
    for cp in plan.child_plans:
        entries.append(CampaignChildIndexEntry(
            child_id=cp.child_id,
            report_campaign_id=cp.child_id,
            finalization_generation_hash=_VALID_HASH,
            child_verification_report_hash=_VALID_HASH_B,
            report_checksum=_VALID_HASH_C,
            assignment_count=cp.expected_assignment_count,
        ))
    total = sum(e.assignment_count for e in entries)
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


class TestBuildCampaignSetPlan:
    def test_builds_valid_plan(self):
        plan = _make_plan()
        assert plan.set_id == "ifeval-expanded-set"
        assert len(plan.child_plans) == CHILD_COUNT
        assert len(plan.population_task_ids) == TOTAL_IFEVAL_TASKS
        assert plan.content_hash != _ZERO_HASH

    def test_partitions_are_disjoint_and_complete(self):
        plan = _make_plan()
        all_tasks: list[str] = []
        for cp in plan.child_plans:
            all_tasks.extend(cp.partition_task_ids)
        assert sorted(all_tasks) == plan.population_task_ids

    def test_each_partition_has_30_tasks(self):
        plan = _make_plan()
        for cp in plan.child_plans:
            assert len(cp.partition_task_ids) == TASKS_PER_CHILD

    def test_partition_indices_are_contiguous(self):
        plan = _make_plan()
        indices = [cp.partition_index for cp in plan.child_plans]
        assert indices == list(range(CHILD_COUNT))

    def test_child_ids_are_derived(self):
        plan = _make_plan()
        for cp in plan.child_plans:
            expected = compute_child_campaign_id(
                parent_campaign_id=plan.parent_campaign_id,
                partition_index=cp.partition_index,
                partition_task_ids=cp.partition_task_ids,
            )
            assert cp.child_id == expected

    def test_child_ids_are_unique(self):
        plan = _make_plan()
        child_ids = [cp.child_id for cp in plan.child_plans]
        assert len(child_ids) == len(set(child_ids))

    def test_expected_counts_are_correct(self):
        plan = _make_plan()
        assert plan.expected_child_assignment_count == EXPECTED_CHILD_ASSIGNMENT_COUNT
        assert plan.expected_total_assignment_count == EXPECTED_TOTAL_ASSIGNMENT_COUNT

    def test_rejects_wrong_population_size(self):
        kwargs = _make_plan_kwargs()
        kwargs["population_task_ids"] = _make_population_task_ids()[:119]
        with pytest.raises(ValueError, match="exactly 120"):
            build_campaign_set_plan(**kwargs)

    def test_rejects_wrong_child_revision_count(self):
        kwargs = _make_plan_kwargs()
        kwargs["child_revisions"] = _make_child_revisions()[:3]
        with pytest.raises(ValueError, match="exactly 4"):
            build_campaign_set_plan(**kwargs)


class TestCampaignSetPlanValidation:
    def test_valid_plan_passes_validation(self):
        plan = _make_plan()
        validate_campaign_set_plan(plan)

    def test_rejects_wrong_child_count(self):
        plan = _make_plan()
        child_plans = list(plan.child_plans)[:3]
        with pytest.raises(ValidationError, match="child_plans"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "child_plans": [cp.model_dump() for cp in child_plans],
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_non_contiguous_partition_indices(self):
        plan = _make_plan()
        child_plans_data = [cp.model_dump() for cp in plan.child_plans]
        child_plans_data[0]["partition_index"] = 3
        child_plans_data[3]["partition_index"] = 0
        with pytest.raises((ValidationError, ValueError), match="partition indices"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "child_plans": child_plans_data,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_duplicate_child_ids(self):
        plan = _make_plan()
        child_plans_data = [cp.model_dump() for cp in plan.child_plans]
        child_plans_data[1]["child_id"] = child_plans_data[0]["child_id"]
        with pytest.raises((ValidationError, ValueError), match="duplicate child_id"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "child_plans": child_plans_data,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_child_id_not_derived(self):
        plan = _make_plan()
        child_plans_data = [cp.model_dump() for cp in plan.child_plans]
        child_plans_data[0]["child_id"] = "tampered-child-id-not-derived"
        with pytest.raises((ValidationError, ValueError), match="does not match derived ID"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "child_plans": child_plans_data,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_wrong_tasks_per_child(self):
        plan = _make_plan()
        child_plans_data = [cp.model_dump() for cp in plan.child_plans]
        child_plans_data[0]["partition_task_ids"] = child_plans_data[0]["partition_task_ids"][:29]
        with pytest.raises(ValidationError):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "child_plans": child_plans_data,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_unsorted_partition_tasks(self):
        plan = _make_plan()
        child_plans_data = [cp.model_dump() for cp in plan.child_plans]
        tasks = list(child_plans_data[0]["partition_task_ids"])
        tasks[0], tasks[1] = tasks[1], tasks[0]
        child_plans_data[0]["partition_task_ids"] = tasks
        with pytest.raises((ValidationError, ValueError), match="must be sorted"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "child_plans": child_plans_data,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_overlapping_partitions(self):
        plan = _make_plan()
        child_plans_data = [cp.model_dump() for cp in plan.child_plans]
        stolen_task = child_plans_data[1]["partition_task_ids"][0]
        child_plans_data[0]["partition_task_ids"] = sorted(
            [*child_plans_data[0]["partition_task_ids"][:-1], stolen_task]
        )
        child_plans_data[1]["partition_task_ids"] = sorted(
            [*child_plans_data[1]["partition_task_ids"][1:], child_plans_data[0]["partition_task_ids"][-2]]
        )
        for i, cpd in enumerate(child_plans_data):
            cpd["child_id"] = compute_child_campaign_id(
                parent_campaign_id=plan.parent_campaign_id,
                partition_index=i,
                partition_task_ids=cpd["partition_task_ids"],
            )
        with pytest.raises((ValidationError, ValueError), match="overlap"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "child_plans": child_plans_data,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_gap_in_population_coverage(self):
        plan = _make_plan()
        child_plans_data = [cp.model_dump() for cp in plan.child_plans]
        last_partition = child_plans_data[3]["partition_task_ids"]
        last_partition[29] = "ifeval-9999"
        child_plans_data[3]["partition_task_ids"] = sorted(last_partition)
        child_plans_data[3]["child_id"] = compute_child_campaign_id(
            parent_campaign_id=plan.parent_campaign_id,
            partition_index=3,
            partition_task_ids=child_plans_data[3]["partition_task_ids"],
        )
        with pytest.raises((ValidationError, ValueError), match="does not match population"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "child_plans": child_plans_data,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_extra_task_in_population(self):
        plan = _make_plan()
        population = list(plan.population_task_ids)
        population.append("ifeval-extra")
        with pytest.raises(ValidationError):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "population_task_ids": population,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_unsorted_population(self):
        plan = _make_plan()
        population = list(plan.population_task_ids)
        population[0], population[1] = population[1], population[0]
        with pytest.raises((ValidationError, ValueError), match="population_task_ids must be sorted"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "population_task_ids": population,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_duplicate_population_tasks(self):
        plan = _make_plan()
        population = list(plan.population_task_ids)
        population[1] = population[0]
        with pytest.raises((ValidationError, ValueError), match="must not contain duplicates"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "population_task_ids": population,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_unsorted_repetition_ids(self):
        plan = _make_plan()
        with pytest.raises((ValidationError, ValueError), match="repetition_ids must be sorted"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "repetition_ids": ["rep-3", "rep-1", "rep-2"],
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_wrong_repetition_count(self):
        plan = _make_plan()
        with pytest.raises(ValidationError):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "repetition_ids": ["rep-1", "rep-2"],
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_wrong_expected_child_count(self):
        plan = _make_plan()
        with pytest.raises((ValidationError, ValueError), match="expected_child_assignment_count"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "expected_child_assignment_count": 999,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_wrong_expected_total_count(self):
        plan = _make_plan()
        with pytest.raises((ValidationError, ValueError), match="expected_total_assignment_count"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "expected_total_assignment_count": 999,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_inconsistent_total_vs_child_count(self):
        plan = _make_plan()
        with pytest.raises((ValidationError, ValueError), match="expected_total_assignment_count"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "expected_total_assignment_count": EXPECTED_TOTAL_ASSIGNMENT_COUNT + 1,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_content_hash_mismatch(self):
        plan = _make_plan()
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            CampaignSetPlan(
                **{
                    **plan.model_dump(),
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_unknown_field(self):
        plan = _make_plan()
        data = plan.model_dump()
        data["unknown_field"] = "bad"
        with pytest.raises(ValidationError):
            CampaignSetPlan(**data)

    def test_hash_changes_when_seed_changes(self):
        kwargs_a = _make_plan_kwargs()
        kwargs_b = {**kwargs_a, "seed": 999}
        plan_a = build_campaign_set_plan(**kwargs_a)
        plan_b = build_campaign_set_plan(**kwargs_b)
        assert plan_a.content_hash != plan_b.content_hash

    def test_hash_changes_when_population_hash_changes(self):
        kwargs_a = _make_plan_kwargs()
        kwargs_b = {**kwargs_a, "population_hash": _VALID_HASH_H}
        plan_a = build_campaign_set_plan(**kwargs_a)
        plan_b = build_campaign_set_plan(**kwargs_b)
        assert plan_a.content_hash != plan_b.content_hash

    def test_hash_changes_when_registry_hash_changes(self):
        kwargs_a = _make_plan_kwargs()
        kwargs_b = {**kwargs_a, "model_registry_hash": _VALID_HASH_H}
        plan_a = build_campaign_set_plan(**kwargs_a)
        plan_b = build_campaign_set_plan(**kwargs_b)
        assert plan_a.content_hash != plan_b.content_hash

    def test_hash_changes_when_profile_hash_changes(self):
        kwargs_a = _make_plan_kwargs()
        kwargs_b = {**kwargs_a, "campaign_profile_hash": _VALID_HASH_H}
        plan_a = build_campaign_set_plan(**kwargs_a)
        plan_b = build_campaign_set_plan(**kwargs_b)
        assert plan_a.content_hash != plan_b.content_hash

    def test_hash_changes_when_environment_scope_changes(self):
        kwargs_a = _make_plan_kwargs()
        kwargs_b = {**kwargs_a, "provider_environment_scope": "linux/arm64/remote-ollama"}
        plan_a = build_campaign_set_plan(**kwargs_a)
        plan_b = build_campaign_set_plan(**kwargs_b)
        assert plan_a.content_hash != plan_b.content_hash

    def test_hash_is_deterministic(self):
        plan_a = build_campaign_set_plan(**_make_plan_kwargs())
        plan_b = build_campaign_set_plan(**_make_plan_kwargs())
        assert plan_a.content_hash == plan_b.content_hash


class TestCampaignSetIndexValidation:
    def test_valid_index_passes_validation(self):
        plan = _make_plan()
        index = _make_index(plan)
        validate_campaign_set_index(index, plan)

    def test_rejects_set_id_mismatch(self):
        plan = _make_plan()
        index = _make_index(plan)
        bad_index = CampaignSetIndex.model_construct(
            set_id="wrong-set-id",
            set_plan_hash=index.set_plan_hash,
            child_index_entries=index.child_index_entries,
            total_assignment_count=index.total_assignment_count,
            content_hash=index.content_hash,
        )
        with pytest.raises((ValidationError, ValueError), match="set_id"):
            validate_campaign_set_index(bad_index, plan)

    def test_rejects_set_plan_hash_mismatch(self):
        plan = _make_plan()
        index = _make_index(plan)
        bad_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash="f" * 64,
            child_index_entries=index.child_index_entries,
            total_assignment_count=index.total_assignment_count,
            content_hash=index.content_hash,
        )
        with pytest.raises((ValidationError, ValueError), match="set_plan_hash"):
            validate_campaign_set_index(bad_index, plan)

    def test_rejects_wrong_child_count(self):
        plan = _make_plan()
        index = _make_index(plan)
        entries = list(index.child_index_entries)[:3]
        bad_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash=index.set_plan_hash,
            child_index_entries=entries,
            total_assignment_count=index.total_assignment_count,
            content_hash=index.content_hash,
        )
        with pytest.raises((ValidationError, ValueError), match="exactly 4"):
            validate_campaign_set_index(bad_index, plan)

    def test_rejects_child_id_not_in_plan(self):
        plan = _make_plan()
        index = _make_index(plan)
        entries = list(index.child_index_entries)
        entries[0] = CampaignChildIndexEntry(
            child_id="nonexistent-child-id",
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
        with pytest.raises((ValidationError, ValueError), match="child IDs do not match"):
            validate_campaign_set_index(bad_index, plan)

    def test_rejects_duplicate_child_ids_in_index(self):
        plan = _make_plan()
        index = _make_index(plan)
        entries = [e.model_dump() for e in index.child_index_entries]
        entries[1]["child_id"] = entries[0]["child_id"]
        with pytest.raises((ValidationError, ValueError), match="duplicate child_id"):
            CampaignSetIndex(
                **{
                    **index.model_dump(),
                    "child_index_entries": entries,
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_content_hash_mismatch(self):
        plan = _make_plan()
        index = _make_index(plan)
        with pytest.raises(ValidationError, match="content_hash mismatch"):
            CampaignSetIndex(
                **{
                    **index.model_dump(),
                    "content_hash": "0" * 64,
                }
            )

    def test_rejects_unknown_field(self):
        plan = _make_plan()
        index = _make_index(plan)
        data = index.model_dump()
        data["unknown_field"] = "bad"
        with pytest.raises(ValidationError):
            CampaignSetIndex(**data)

    def test_rejects_total_assignment_count_mismatch(self):
        """validate_campaign_set_index rejects an index whose total_assignment_count
        does not match the plan's expected_total_assignment_count."""
        plan = _make_plan()
        index = _make_index(plan)
        bad_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash=index.set_plan_hash,
            child_index_entries=index.child_index_entries,
            total_assignment_count=999,
            content_hash=index.content_hash,
        )
        with pytest.raises(ValueError, match="total_assignment_count"):
            validate_campaign_set_index(bad_index, plan)

    def test_index_hash_is_deterministic(self):
        plan = _make_plan()
        index_a = _make_index(plan)
        index_b = _make_index(plan)
        assert index_a.content_hash == index_b.content_hash

    def test_index_hash_changes_when_assignment_count_changes(self):
        plan = _make_plan()
        index = _make_index(plan)
        entries = [e.model_dump() for e in index.child_index_entries]
        entries[0]["assignment_count"] = 9999
        new_index = CampaignSetIndex.model_construct(
            set_id=index.set_id,
            set_plan_hash=index.set_plan_hash,
            child_index_entries=[CampaignChildIndexEntry(**e) for e in entries],
            total_assignment_count=index.total_assignment_count,
            content_hash="0" * 64,
        )
        new_hash = compute_campaign_set_index_hash(new_index)
        assert new_hash != index.content_hash


class TestComputeChildCampaignId:
    def test_is_deterministic(self):
        ids_a = compute_child_campaign_id("parent", 0, ["t1", "t2"])
        ids_b = compute_child_campaign_id("parent", 0, ["t1", "t2"])
        assert ids_a == ids_b

    def test_changes_with_parent_id(self):
        a = compute_child_campaign_id("parent-a", 0, ["t1", "t2"])
        b = compute_child_campaign_id("parent-b", 0, ["t1", "t2"])
        assert a != b

    def test_changes_with_partition_index(self):
        a = compute_child_campaign_id("parent", 0, ["t1", "t2"])
        b = compute_child_campaign_id("parent", 1, ["t1", "t2"])
        assert a != b

    def test_changes_with_partition_tasks(self):
        a = compute_child_campaign_id("parent", 0, ["t1", "t2"])
        b = compute_child_campaign_id("parent", 0, ["t1", "t3"])
        assert a != b

    def test_order_independent(self):
        a = compute_child_campaign_id("parent", 0, ["t2", "t1"])
        b = compute_child_campaign_id("parent", 0, ["t1", "t2"])
        assert a == b


class TestDryRunPlan:
    def test_produces_correct_counts(self):
        plan = _make_plan()
        dry = compute_dry_run_plan(plan)
        assert dry["child_count"] == CHILD_COUNT
        assert dry["total_tasks"] == TOTAL_IFEVAL_TASKS
        assert dry["repetition_count"] == REPETITION_COUNT
        assert dry["expected_child_assignment_count"] == EXPECTED_CHILD_ASSIGNMENT_COUNT
        assert dry["expected_total_assignment_count"] == EXPECTED_TOTAL_ASSIGNMENT_COUNT

    def test_each_child_has_2790_assignments(self):
        plan = _make_plan()
        dry = compute_dry_run_plan(plan)
        for child in dry["children"]:
            assert child["expected_assignment_count"] == EXPECTED_CHILD_ASSIGNMENT_COUNT
            assert child["tasks_per_child"] == TASKS_PER_CHILD
            assert child["repetition_count"] == REPETITION_COUNT

    def test_total_is_11160(self):
        plan = _make_plan()
        dry = compute_dry_run_plan(plan)
        assert dry["expected_total_assignment_count"] == EXPECTED_TOTAL_ASSIGNMENT_COUNT
        child_total = sum(c["expected_assignment_count"] for c in dry["children"])
        assert child_total == EXPECTED_TOTAL_ASSIGNMENT_COUNT

    def test_is_deterministic(self):
        plan = _make_plan()
        dry_a = compute_dry_run_plan(plan)
        dry_b = compute_dry_run_plan(plan)
        assert dry_a == dry_b

    def test_child_ids_match_plan(self):
        plan = _make_plan()
        dry = compute_dry_run_plan(plan)
        for i, child in enumerate(dry["children"]):
            assert child["child_id"] == plan.child_plans[i].child_id

    def test_is_json_serializable(self):
        plan = _make_plan()
        dry = compute_dry_run_plan(plan)
        serialized = json.dumps(dry, sort_keys=True)
        deserialized = json.loads(serialized)
        assert deserialized == dry


class TestAggregateVerificationResultModel:
    def test_construction_succeeds(self):
        result = AggregateVerificationResult.model_construct(
            verification_schema_version=CAMPAIGN_SET_SCHEMA_VERSION,
            set_id="test-set",
            set_plan_hash=_VALID_HASH,
            set_index_hash=_VALID_HASH_B,
            ok=True,
            child_results=[],
            total_assignments_verified=0,
            assignment_uniqueness_ok=True,
            coverage_ok=True,
            product_coverage_ok=True,
            hash_binding_ok=True,
            failures=[],
            content_hash="0" * 64,
        )
        content_hash = compute_aggregate_verification_result_hash(result)
        result = result.model_copy(update={"content_hash": content_hash})
        assert result.ok is True
        assert result.content_hash == compute_aggregate_verification_result_hash(result)

    def test_rejects_unknown_field(self):
        with pytest.raises(ValidationError):
            AggregateVerificationResult(
                verification_schema_version=CAMPAIGN_SET_SCHEMA_VERSION,
                set_id="test-set",
                set_plan_hash=_VALID_HASH,
                set_index_hash=_VALID_HASH_B,
                ok=True,
                child_results=[],
                total_assignments_verified=0,
                assignment_uniqueness_ok=True,
                coverage_ok=True,
                product_coverage_ok=True,
                hash_binding_ok=True,
                failures=[],
                content_hash=_VALID_HASH,
                unknown_field="bad",
            )

    def test_content_hash_mismatch_fails(self):
        with pytest.raises(ValidationError):
            AggregateVerificationResult(
                verification_schema_version=CAMPAIGN_SET_SCHEMA_VERSION,
                set_id="test-set",
                set_plan_hash=_VALID_HASH,
                set_index_hash=_VALID_HASH_B,
                ok=True,
                child_results=[],
                total_assignments_verified=0,
                assignment_uniqueness_ok=True,
                coverage_ok=True,
                product_coverage_ok=True,
                hash_binding_ok=True,
                failures=[],
                content_hash="0" * 64,
            )


class TestChildVerificationResultModel:
    def test_construction_succeeds(self):
        result = ChildVerificationResult(
            child_id="child-0",
            ok=True,
            campaign_id="child-0",
            verified_index_generation_hash=_VALID_HASH,
            assignment_ids=["a1", "a2"],
            assignment_count=2,
            failures=[],
            task_ids=["t1", "t2"],
            replicate_ids=["r1"],
            variant_count=1,
            product_ok=True,
            child_verification_report_hash=_VALID_HASH,
            report_checksum=_VALID_HASH,
        )
        assert result.ok is True
        assert result.assignment_count == 2
        assert result.product_ok is True

    def test_rejects_unknown_field(self):
        with pytest.raises(ValidationError):
            ChildVerificationResult(
                child_id="child-0",
                ok=True,
                campaign_id="child-0",
                verified_index_generation_hash=_VALID_HASH,
                assignment_ids=[],
                assignment_count=0,
                failures=[],
                task_ids=[],
                replicate_ids=[],
                variant_count=0,
                product_ok=False,
                child_verification_report_hash=_VALID_HASH,
                report_checksum=_VALID_HASH,
                unknown_field="bad",
            )


class TestCampaignChildPlanModel:
    def test_rejects_unknown_field(self):
        with pytest.raises(ValidationError):
            CampaignChildPlan(
                child_id="child-0",
                child_revision="rev-0",
                partition_index=0,
                partition_task_ids=[f"t{i}" for i in range(TASKS_PER_CHILD)],
                expected_assignment_count=EXPECTED_CHILD_ASSIGNMENT_COUNT,
                unknown_field="bad",
            )

    def test_rejects_partition_index_out_of_range(self):
        with pytest.raises(ValidationError):
            CampaignChildPlan(
                child_id="child-0",
                child_revision="rev-0",
                partition_index=CHILD_COUNT,
                partition_task_ids=[f"t{i}" for i in range(TASKS_PER_CHILD)],
                expected_assignment_count=EXPECTED_CHILD_ASSIGNMENT_COUNT,
            )

    def test_rejects_wrong_task_count(self):
        with pytest.raises(ValidationError):
            CampaignChildPlan(
                child_id="child-0",
                child_revision="rev-0",
                partition_index=0,
                partition_task_ids=[f"t{i}" for i in range(TASKS_PER_CHILD - 1)],
                expected_assignment_count=EXPECTED_CHILD_ASSIGNMENT_COUNT,
            )
