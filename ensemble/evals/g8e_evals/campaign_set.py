# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed campaign-set plan, post-execution index, and aggregate verifier
for the expanded IFEval campaign (D9 four-child campaign-set).

The expanded IFEval campaign partitions the 120-task IFEval population
into exactly four disjoint 30-task children. Each child runs 31 variants
across its 30 tasks with 3 repetitions, producing 2,790 assignments per
child and 11,160 assignments total. The campaign-set authority binds
the parent identity, exact child identities, partitions, registry,
profile, and population hashes, repetition IDs, seed, retry and budget
authorities, instrumentation and expected-record policy hashes,
environment scopes, and expected assignment counts.

The ``CampaignSetPlan`` is the frozen preregistration artifact. It never
mutates after the first dependent provider call. Evidence-dependent
fields live on the post-execution ``CampaignSetIndex``, which binds
accepted report identities, finalization generation hashes, complete
child verification-report hashes, and report checksums. Paths are
never used as identity; the index carries typed hashes and campaign
identities only.

The aggregate verifier validates each child first (delegating to the
existing ``verify_campaign``), then proves assignment uniqueness and
complete 11,160-assignment coverage across children. It rejects an
extra assignment or disposition as well as a missing one. A caller
cannot pass four arbitrary directories; the verifier consumes the
typed plan and index, plus a child-ID-to-report-directory mapping that
is resolved at the call boundary.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from pathlib import Path
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

from g8e_evals.campaign_verify import verify_campaign
from g8e_evals.constants import (
    ATTEMPTS_JSONL,
    CAMPAIGN_ASSIGNMENTS_JSONL,
    CAMPAIGN_VERIFICATION_REPORT_JSON,
    METRICS_JSONL,
    REPORT_CHECKSUM_JSON,
)
from g8e_evals.index import CampaignVerificationReport


CAMPAIGN_SET_SCHEMA_VERSION = "1.0.0"

CHILD_COUNT = 4
TASKS_PER_CHILD = 30
TOTAL_IFEVAL_TASKS = 120
REPETITION_COUNT = 3
EXPECTED_CHILD_ASSIGNMENT_COUNT = 2790
EXPECTED_TOTAL_ASSIGNMENT_COUNT = 11160


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class CampaignSetStatus(StrEnum):
    """Lifecycle status of a campaign set.

    ``PLANNED``: The set plan is frozen but no child has started.
    ``RUNNING``: One or more children are executing.
    ``COMPLETED``: All children are finalized and the set index is written.
    ``VERIFIED``: The aggregate verifier has accepted the set.
    ``FAILED``: The aggregate verifier rejected the set.
    """

    PLANNED = "planned"
    RUNNING = "running"
    COMPLETED = "completed"
    VERIFIED = "verified"
    FAILED = "failed"


class CampaignChildPlan(BaseModel):
    """One child campaign within the frozen campaign-set plan.

    Binds the child campaign ID (derived under the frozen identity rule
    from the parent campaign ID, partition index, and partition task
    IDs), the child revision, the ordered 30-task partition, the
    partition index (0-3), and the expected assignment count for this
    child.

    The child ID is content-addressed: the same parent, partition
    index, and partition task IDs always produce the same child ID.
    A child ID that does not match the derived value is rejected during
    plan validation.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    child_id: str = Field(
        min_length=1,
        description="Child campaign ID derived from parent ID, partition index, and partition task IDs.",
    )
    child_revision: str = Field(
        min_length=1,
        description="Child campaign revision identifier.",
    )
    partition_index: int = Field(
        ge=0,
        lt=CHILD_COUNT,
        description="Zero-indexed partition position (0-3).",
    )
    partition_task_ids: list[str] = Field(
        min_length=TASKS_PER_CHILD,
        max_length=TASKS_PER_CHILD,
        description="Exactly 30 task IDs in this partition (sorted).",
    )
    expected_assignment_count: int = Field(
        ge=1,
        description="Expected assignment count for this child (31 variants x 30 tasks x 3 repetitions = 2790).",
    )


class CampaignSetPlan(BaseModel):
    """Frozen typed campaign-set plan for the expanded IFEval campaign.

    Binds the parent campaign identity, exact child IDs and revisions,
    ordered 30-task partitions, registry/profile/population hashes,
    three repetition IDs, seed, retry and budget authorities,
    instrumentation and expected-record policy hashes, environment
    scopes, and expected child/parent assignment counts.

    The plan is the preregistration authority. Evidence-dependent
    fields never mutate this artifact; they live on the
    ``CampaignSetIndex``. Changing any bound field changes the content
    hash and creates a new campaign-set identity.

    The ``content_hash`` is SHA-256 over canonical JSON of the plan.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    set_id: str = Field(min_length=1, description="Campaign-set identity.")
    set_version: str = Field(min_length=1, description="Campaign-set schema version.")
    parent_campaign_id: str = Field(min_length=1, description="Parent campaign identity.")
    parent_campaign_revision: str = Field(min_length=1, description="Parent campaign revision.")
    child_plans: list[CampaignChildPlan] = Field(
        min_length=CHILD_COUNT,
        max_length=CHILD_COUNT,
        description="Exactly four child plans, one per partition.",
    )
    population_task_ids: list[str] = Field(
        min_length=TOTAL_IFEVAL_TASKS,
        max_length=TOTAL_IFEVAL_TASKS,
        description="The full 120-task IFEval population authority (sorted).",
    )
    population_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the 120-task IFEval benchmark population.",
    )
    model_registry_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the frozen model registry.",
    )
    campaign_profile_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the frozen campaign profile (post-AUTH-1).",
    )
    repetition_ids: list[str] = Field(
        min_length=REPETITION_COUNT,
        max_length=REPETITION_COUNT,
        description="Exactly three repetition IDs (sorted).",
    )
    seed: int = Field(ge=0, description="Frozen randomization seed for schedule generation.")
    retry_policy_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the frozen retry policy.",
    )
    budget_authority_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the frozen budget authority.",
    )
    instrumentation_policy_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the frozen instrumentation policy.",
    )
    expected_record_policy_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the frozen expected-record policy.",
    )
    orchestrator_environment_scope: str = Field(
        min_length=1,
        description="Orchestrator-host environment identity (e.g. linux/amd64/cpu).",
    )
    provider_environment_scope: str = Field(
        min_length=1,
        description="Provider-host environment identity (e.g. linux/amd64/remote-ollama).",
    )
    expected_child_assignment_count: int = Field(
        ge=1,
        description="Expected assignment count per child (2790).",
    )
    expected_total_assignment_count: int = Field(
        ge=1,
        description="Expected total assignment count across all children (11160).",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the plan.",
    )

    @model_validator(mode="after")
    def _validate_plan(self) -> Self:
        validate_campaign_set_plan(self)
        expected = compute_campaign_set_plan_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"campaign set plan content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class CampaignChildIndexEntry(BaseModel):
    """One child's post-execution index entry.

    Binds the child campaign ID to its accepted report identity,
    finalization generation hash, complete child verification-report
    hash, and report checksum. Paths are never used as identity; all
    bindings are typed hashes and campaign identities.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    child_id: str = Field(
        min_length=1,
        description="Child campaign ID matching the child plan.",
    )
    report_campaign_id: str = Field(
        min_length=1,
        description="Accepted report campaign identity (not a path).",
    )
    finalization_generation_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the child's finalization index generation.",
    )
    child_verification_report_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the complete child verification report bytes.",
    )
    report_checksum: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 checksum of the complete child report.",
    )
    assignment_count: int = Field(
        ge=0,
        description="Number of assignments in this child's accepted report.",
    )


class CampaignSetIndex(BaseModel):
    """Post-execution campaign-set index binding accepted child reports.

    The index is written after all children are finalized. It binds the
    set identity, the set plan hash (proving the index corresponds to a
    specific frozen plan), and one index entry per child. Evidence-
    dependent fields live here, never on the plan.

    The ``content_hash`` is SHA-256 over canonical JSON of the index.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    set_id: str = Field(min_length=1, description="Campaign-set identity matching the plan.")
    set_plan_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the frozen CampaignSetPlan this index corresponds to.",
    )
    child_index_entries: list[CampaignChildIndexEntry] = Field(
        min_length=CHILD_COUNT,
        max_length=CHILD_COUNT,
        description="Exactly four child index entries, one per child.",
    )
    total_assignment_count: int = Field(
        ge=0,
        description="Total assignment count across all children (11160 when complete).",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the index.",
    )

    @model_validator(mode="after")
    def _validate_index(self) -> Self:
        child_ids = [e.child_id for e in self.child_index_entries]
        if len(child_ids) != len(set(child_ids)):
            raise ValueError(f"duplicate child_id in campaign set index: {child_ids}")
        expected = compute_campaign_set_index_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"campaign set index content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


class ChildVerificationResult(BaseModel):
    """Typed verification result for one child within the aggregate.

    Carries the child campaign ID, the child verification report (from
    ``verify_campaign``), the assignment IDs found in the child's
    report, the assignment count, and the task/repetition/variant
    product verification fields.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    child_id: str = Field(min_length=1, description="Child campaign ID.")
    ok: bool = Field(description="True when the child's own verification passed.")
    campaign_id: str = Field(description="Campaign identity from the child verification report.")
    verified_index_generation_hash: str = Field(
        min_length=64, max_length=64,
        description="Verified index generation hash from the child verification report.",
    )
    assignment_ids: list[str] = Field(
        default_factory=list,
        description="Assignment IDs found in the child's campaign-assignments.jsonl (sorted).",
    )
    assignment_count: int = Field(ge=0, description="Number of assignments in the child report.")
    failures: list[str] = Field(
        default_factory=list,
        description="Sorted failure messages from the child verification.",
    )
    task_ids: list[str] = Field(
        default_factory=list,
        description="Sorted task IDs found in the child's assignments.",
    )
    replicate_ids: list[str] = Field(
        default_factory=list,
        description="Sorted replicate IDs found in the child's assignments.",
    )
    variant_count: int = Field(
        ge=0,
        description="Number of distinct (model_cohort_id, arm_id) pairs in the child's assignments.",
    )
    product_ok: bool = Field(
        description="True when the child's assignments form the exact task x repetition x variant product.",
    )
    child_verification_report_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the persisted child verification report bytes.",
    )
    report_checksum: str = Field(
        min_length=64, max_length=64,
        description="Recomputed SHA-256 report checksum from attempts and metrics.",
    )


class AggregateVerificationResult(BaseModel):
    """Typed aggregate verification result for the campaign set.

    The aggregate verifier validates each child first, then proves
    assignment uniqueness, complete coverage, the exact task x
    repetition x variant product, and hash binding to the persisted
    child verification reports and report checksums. This result is
    used directly by analysis and projection; a caller cannot pass
    four arbitrary directories.

    The ``ok`` field is ``True`` only when every child passes its own
    verification, all child identities match the plan and index, all
    assignment IDs are unique across children, the total assignment
    count equals the expected total, the exact authority product is
    proven for every child, and every declared hash recomputes from
    the report directory.

    The ``content_hash`` is SHA-256 over canonical JSON of the result,
    so Delta can consume a recomputed aggregate verification hash
    directly from ``result.content_hash``.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    verification_schema_version: str = Field(min_length=1, description="Schema version.")
    set_id: str = Field(min_length=1, description="Campaign-set identity.")
    set_plan_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the CampaignSetPlan used for verification.",
    )
    set_index_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the CampaignSetIndex used for verification.",
    )
    ok: bool = Field(description="True when all aggregate checks pass.")
    child_results: list[ChildVerificationResult] = Field(
        default_factory=list,
        description="Per-child verification results (one per child, in plan order).",
    )
    total_assignments_verified: int = Field(
        ge=0,
        description="Total unique assignments verified across all children.",
    )
    assignment_uniqueness_ok: bool = Field(
        description="True when no assignment ID appears in more than one child.",
    )
    coverage_ok: bool = Field(
        description="True when the total assignment count equals the expected total.",
    )
    product_coverage_ok: bool = Field(
        description="True when every child's assignments form the exact task x repetition x variant product.",
    )
    hash_binding_ok: bool = Field(
        description="True when every child verification report hash and report checksum recomputes from the report directory.",
    )
    failures: list[str] = Field(
        default_factory=list,
        description="Sorted aggregate failure messages.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of the aggregate verification result.",
    )

    @model_validator(mode="after")
    def _validate_result(self) -> Self:
        expected = compute_aggregate_verification_result_hash(self)
        if self.content_hash != expected:
            raise ValueError(
                f"aggregate verification result content_hash mismatch: "
                f"declared {self.content_hash!r}, computed {expected!r}"
            )
        return self


def compute_child_campaign_id(
    parent_campaign_id: str,
    partition_index: int,
    partition_task_ids: list[str],
) -> str:
    """Derive a canonical child campaign ID from the parent identity and partition.

    The child ID is deterministic: the same (parent_campaign_id,
    partition_index, partition_task_ids) always produces the same child
    ID. A child plan whose ``child_id`` does not match this derivation
    is rejected during plan validation.
    """
    payload = json.dumps(
        {
            "parent_campaign_id": parent_campaign_id,
            "partition_index": partition_index,
            "partition_task_ids": sorted(partition_task_ids),
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_campaign_set_plan_hash(plan: CampaignSetPlan) -> str:
    """Compute the content hash for a campaign-set plan.

    Serializes every material field (excluding ``content_hash`` itself)
    into canonical JSON and returns SHA-256. The same function is used
    by the model validator and by plan builders.
    """
    data = plan.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    payload = json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_campaign_set_index_hash(index: CampaignSetIndex) -> str:
    """Compute the content hash for a campaign-set index.

    Serializes every material field (excluding ``content_hash`` itself)
    into canonical JSON and returns SHA-256.
    """
    data = index.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    payload = json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_aggregate_verification_result_hash(result: AggregateVerificationResult) -> str:
    """Compute the content hash for an aggregate verification result.

    Serializes every material field (excluding ``content_hash`` itself)
    into canonical JSON and returns SHA-256. The same function is used
    by the model validator and by callers that need to recompute the
    hash from a constructed result.
    """
    data = result.model_dump(mode="json", by_alias=True)
    data.pop("content_hash", None)
    payload = json.dumps(
        data,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def validate_campaign_set_plan(plan: CampaignSetPlan) -> None:
    """Validate all D9 invariants for a campaign-set plan.

    Checks:
    - Exactly four child plans with contiguous partition indices 0-3.
    - Each partition has exactly 30 tasks.
    - Partitions are disjoint (no task appears in two partitions).
    - The union of all partitions equals the 120-task population authority.
    - No gaps or extra tasks in the union.
    - No duplicate child IDs.
    - Each child ID matches the derived ID from parent, partition index,
      and partition task IDs.
    - Repetition IDs are sorted and unique.
    - Expected assignment counts match the frozen dimensions.
    - Population task IDs are sorted and unique.

    Raises ``ValueError`` on any violation.
    """
    if len(plan.child_plans) != CHILD_COUNT:
        raise ValueError(
            f"child_plans must have exactly {CHILD_COUNT} entries: got {len(plan.child_plans)}"
        )

    partition_indices = [cp.partition_index for cp in plan.child_plans]
    if partition_indices != list(range(CHILD_COUNT)):
        raise ValueError(
            f"partition indices must be contiguous 0-{CHILD_COUNT - 1}: got {sorted(partition_indices)}"
        )

    child_ids = [cp.child_id for cp in plan.child_plans]
    if len(child_ids) != len(set(child_ids)):
        raise ValueError(f"duplicate child_id in campaign set plan: {child_ids}")

    for cp in plan.child_plans:
        if len(cp.partition_task_ids) != TASKS_PER_CHILD:
            raise ValueError(
                f"child {cp.child_id} partition must have exactly {TASKS_PER_CHILD} tasks: "
                f"got {len(cp.partition_task_ids)}"
            )
        if cp.partition_task_ids != sorted(cp.partition_task_ids):
            raise ValueError(
                f"child {cp.child_id} partition_task_ids must be sorted: {cp.partition_task_ids}"
            )
        if len(cp.partition_task_ids) != len(set(cp.partition_task_ids)):
            raise ValueError(
                f"child {cp.child_id} partition_task_ids must not contain duplicates: {cp.partition_task_ids}"
            )

        expected_child_id = compute_child_campaign_id(
            parent_campaign_id=plan.parent_campaign_id,
            partition_index=cp.partition_index,
            partition_task_ids=cp.partition_task_ids,
        )
        if cp.child_id != expected_child_id:
            raise ValueError(
                f"child_id {cp.child_id!r} does not match derived ID {expected_child_id!r} "
                f"for partition_index {cp.partition_index}"
            )

        if cp.expected_assignment_count != plan.expected_child_assignment_count:
            raise ValueError(
                f"child {cp.child_id} expected_assignment_count {cp.expected_assignment_count} "
                f"!= plan expected_child_assignment_count {plan.expected_child_assignment_count}"
            )

    all_partition_tasks: list[str] = []
    for cp in plan.child_plans:
        all_partition_tasks.extend(cp.partition_task_ids)

    if len(all_partition_tasks) != len(set(all_partition_tasks)):
        overlap_set: set[str] = set()
        seen: set[str] = set()
        for tid in all_partition_tasks:
            if tid in seen:
                overlap_set.add(tid)
            seen.add(tid)
        raise ValueError(f"partitions overlap on tasks: {sorted(overlap_set)}")

    if len(all_partition_tasks) != TOTAL_IFEVAL_TASKS:
        raise ValueError(
            f"union of partitions must have exactly {TOTAL_IFEVAL_TASKS} tasks: "
            f"got {len(all_partition_tasks)}"
        )

    if plan.population_task_ids != sorted(plan.population_task_ids):
        raise ValueError(
            f"population_task_ids must be sorted: {plan.population_task_ids}"
        )
    if len(plan.population_task_ids) != len(set(plan.population_task_ids)):
        raise ValueError(
            f"population_task_ids must not contain duplicates: {plan.population_task_ids}"
        )
    if len(plan.population_task_ids) != TOTAL_IFEVAL_TASKS:
        raise ValueError(
            f"population_task_ids must have exactly {TOTAL_IFEVAL_TASKS} entries: "
            f"got {len(plan.population_task_ids)}"
        )

    union_sorted = sorted(set(all_partition_tasks))
    if union_sorted != plan.population_task_ids:
        partition_only = set(union_sorted) - set(plan.population_task_ids)
        population_only = set(plan.population_task_ids) - set(union_sorted)
        raise ValueError(
            f"partition union does not match population authority: "
            f"partition_only={sorted(partition_only)}, "
            f"population_only={sorted(population_only)}"
        )

    if plan.repetition_ids != sorted(plan.repetition_ids):
        raise ValueError(
            f"repetition_ids must be sorted: {plan.repetition_ids}"
        )
    if len(plan.repetition_ids) != len(set(plan.repetition_ids)):
        raise ValueError(
            f"repetition_ids must not contain duplicates: {plan.repetition_ids}"
        )
    if len(plan.repetition_ids) != REPETITION_COUNT:
        raise ValueError(
            f"repetition_ids must have exactly {REPETITION_COUNT} entries: "
            f"got {len(plan.repetition_ids)}"
        )

    if plan.expected_child_assignment_count != EXPECTED_CHILD_ASSIGNMENT_COUNT:
        raise ValueError(
            f"expected_child_assignment_count must be {EXPECTED_CHILD_ASSIGNMENT_COUNT}: "
            f"got {plan.expected_child_assignment_count}"
        )
    if plan.expected_total_assignment_count != EXPECTED_TOTAL_ASSIGNMENT_COUNT:
        raise ValueError(
            f"expected_total_assignment_count must be {EXPECTED_TOTAL_ASSIGNMENT_COUNT}: "
            f"got {plan.expected_total_assignment_count}"
        )
    if plan.expected_total_assignment_count != plan.expected_child_assignment_count * CHILD_COUNT:
        raise ValueError(
            f"expected_total_assignment_count ({plan.expected_total_assignment_count}) != "
            f"expected_child_assignment_count ({plan.expected_child_assignment_count}) * "
            f"CHILD_COUNT ({CHILD_COUNT})"
        )


def validate_campaign_set_index(
    index: CampaignSetIndex,
    plan: CampaignSetPlan,
) -> None:
    """Validate that a campaign-set index corresponds to a frozen plan.

    Checks:
    - The set_id matches the plan.
    - The set_plan_hash matches the plan's content hash.
    - Exactly four child index entries with child IDs matching the plan.
    - The total assignment count matches the expected total when all
      children are complete.

    Raises ``ValueError`` on any violation.
    """
    if index.set_id != plan.set_id:
        raise ValueError(
            f"index set_id {index.set_id!r} != plan set_id {plan.set_id!r}"
        )
    if index.set_plan_hash != plan.content_hash:
        raise ValueError(
            f"index set_plan_hash {index.set_plan_hash!r} != plan content_hash {plan.content_hash!r}"
        )
    if len(index.child_index_entries) != CHILD_COUNT:
        raise ValueError(
            f"index must have exactly {CHILD_COUNT} child entries: got {len(index.child_index_entries)}"
        )

    plan_child_ids = {cp.child_id for cp in plan.child_plans}
    index_child_ids = {e.child_id for e in index.child_index_entries}
    if plan_child_ids != index_child_ids:
        raise ValueError(
            f"index child IDs do not match plan child IDs: "
            f"index_only={sorted(index_child_ids - plan_child_ids)}, "
            f"plan_only={sorted(plan_child_ids - index_child_ids)}"
        )

    index_child_id_list = [e.child_id for e in index.child_index_entries]
    if len(index_child_id_list) != len(set(index_child_id_list)):
        raise ValueError(
            f"duplicate child_id in index entries: {index_child_id_list}"
        )

    if index.total_assignment_count != plan.expected_total_assignment_count:
        raise ValueError(
            f"index total_assignment_count {index.total_assignment_count} != "
            f"plan expected_total_assignment_count {plan.expected_total_assignment_count}"
        )


def _read_assignment_ids(report_dir: Path) -> list[str]:
    """Read assignment IDs from a child report directory's campaign-assignments.jsonl."""
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


def _read_assignment_records(report_dir: Path) -> list[dict[str, object]]:
    """Read full assignment records from a child report's campaign-assignments.jsonl.

    Returns a list of parsed dicts, each carrying ``assignment_id``,
    ``task_id``, ``model_cohort_id``, ``arm_id``, and ``replicate_id``.
    """
    assignments_path = report_dir / CAMPAIGN_ASSIGNMENTS_JSONL
    if not assignments_path.exists():
        return []
    records: list[dict[str, object]] = []
    for line in assignments_path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        records.append(json.loads(line))
    return records


def _compute_child_verification_report_hash(report_dir: Path) -> str | None:
    """Compute SHA-256 of the persisted child verification report bytes.

    Reads ``campaign-verification-report.json`` from the report directory
    and returns SHA-256 of its raw file bytes. Returns ``None`` when the
    file does not exist.
    """
    report_path = report_dir / CAMPAIGN_VERIFICATION_REPORT_JSON
    if not report_path.exists():
        return None
    return _sha256(report_path.read_text())


def _recompute_report_checksum(report_dir: Path) -> str | None:
    """Recompute the report checksum from attempts.jsonl and metrics.jsonl.

    Matches the standalone report validator's computation: SHA-256 over
    canonical JSON of ``{"attempts": [...], "metrics": [...]}``. Returns
    ``None`` when neither file exists.
    """
    attempts_path = report_dir / ATTEMPTS_JSONL
    metrics_path = report_dir / METRICS_JSONL

    attempts_data: list[dict[str, object]] = []
    if attempts_path.exists():
        for line in attempts_path.read_text().splitlines():
            line = line.strip()
            if line:
                attempts_data.append(json.loads(line))

    metrics_data: list[dict[str, object]] = []
    if metrics_path.exists():
        for line in metrics_path.read_text().splitlines():
            line = line.strip()
            if line:
                metrics_data.append(json.loads(line))

    if not attempts_path.exists() and not metrics_path.exists():
        return None

    payload = json.dumps(
        {"attempts": attempts_data, "metrics": metrics_data},
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def _read_stored_report_checksum(report_dir: Path) -> str | None:
    """Read the declared checksum from report-checksum.json.

    Returns ``None`` when the file does not exist.
    """
    checksum_path = report_dir / REPORT_CHECKSUM_JSON
    if not checksum_path.exists():
        return None
    data = json.loads(checksum_path.read_text())
    return data.get("checksum")


def verify_campaign_set_aggregate(
    plan: CampaignSetPlan,
    index: CampaignSetIndex,
    child_report_dirs: dict[str, Path],
) -> AggregateVerificationResult:
    """Verify a complete campaign set: each child, then aggregate coverage.

    The verifier:
    1. Validates the index against the plan (set_id, set_plan_hash,
       child IDs, total assignment count).
    2. For each child in plan order, runs ``verify_campaign`` on the
       corresponding report directory and collects assignment records.
    3. Proves no assignment ID appears in more than one child (uniqueness).
    4. Proves the total unique assignment count equals the expected total
       (complete 11,160-assignment coverage).
    5. Cross-checks each child's verification report campaign ID and
       verified index generation hash against the index entry.
    6. Recomputes each child's verification report hash from the persisted
       ``campaign-verification-report.json`` bytes and compares against
       the index entry's ``child_verification_report_hash``.
    7. Recomputes each child's report checksum from ``attempts.jsonl`` and
       ``metrics.jsonl`` and compares against both the stored
       ``report-checksum.json`` and the index entry's ``report_checksum``.
    8. Proves the exact task x repetition x variant authority product for
       each child: the set of ``(task_id, replicate_id,
       model_cohort_id, arm_id)`` tuples forms a complete Cartesian
       product of the partition's task IDs, the plan's repetition IDs,
       and the distinct variant pairs, with no duplicates or missing
       combinations.

    A caller cannot pass four arbitrary directories; the child_report_dirs
    mapping must contain exactly the child IDs from the plan, and each
    directory is verified against the typed index entry.

    Returns a content-addressed ``AggregateVerificationResult`` with
    ``ok=True`` only when all checks pass. The ``content_hash`` allows
    downstream consumers (e.g. the model-comparison engine) to bind
    directly to the accepted aggregate verification hash.
    """
    failures: list[str] = []

    try:
        validate_campaign_set_index(index, plan)
    except ValueError as e:
        failures.append(f"index/plan validation failed: {e}")

    plan_child_ids = [cp.child_id for cp in plan.child_plans]

    missing_dirs = [cid for cid in plan_child_ids if cid not in child_report_dirs]
    if missing_dirs:
        failures.append(
            f"missing report directories for child IDs: {sorted(missing_dirs)}"
        )

    extra_dirs = [cid for cid in child_report_dirs if cid not in set(plan_child_ids)]
    if extra_dirs:
        failures.append(
            f"unexpected report directories for child IDs: {sorted(extra_dirs)}"
        )

    child_results: list[ChildVerificationResult] = []
    all_assignment_ids: list[str] = []
    seen_assignment_ids: set[str] = set()
    duplicate_assignment_ids: set[str] = set()
    product_failures: list[str] = []
    hash_failures: list[str] = []

    for cp in plan.child_plans:
        child_id = cp.child_id
        report_dir = child_report_dirs.get(child_id)

        if report_dir is None:
            child_results.append(ChildVerificationResult(
                child_id=child_id,
                ok=False,
                campaign_id="unknown",
                verified_index_generation_hash="0" * 64,
                assignment_ids=[],
                assignment_count=0,
                failures=[f"no report directory provided for child {child_id}"],
                task_ids=[],
                replicate_ids=[],
                variant_count=0,
                product_ok=False,
                child_verification_report_hash="0" * 64,
                report_checksum="0" * 64,
            ))
            failures.append(f"no report directory provided for child {child_id}")
            product_failures.append(f"child {child_id}: no report directory")
            hash_failures.append(f"child {child_id}: no report directory")
            continue

        child_verification: CampaignVerificationReport = verify_campaign(report_dir)

        assignment_ids = _read_assignment_ids(report_dir)
        assignment_records = _read_assignment_records(report_dir)

        for aid in assignment_ids:
            if aid in seen_assignment_ids:
                duplicate_assignment_ids.add(aid)
            seen_assignment_ids.add(aid)
        all_assignment_ids.extend(assignment_ids)

        index_entry: CampaignChildIndexEntry | None = None
        for entry in index.child_index_entries:
            if entry.child_id == child_id:
                index_entry = entry
                break

        if index_entry is not None:
            if child_verification.campaign_id != index_entry.report_campaign_id:
                failures.append(
                    f"child {child_id} verification campaign_id "
                    f"{child_verification.campaign_id!r} != index report_campaign_id "
                    f"{index_entry.report_campaign_id!r}"
                )
            if child_verification.verified_index_generation_hash != index_entry.finalization_generation_hash:
                failures.append(
                    f"child {child_id} verified_index_generation_hash "
                    f"{child_verification.verified_index_generation_hash!r} != "
                    f"index finalization_generation_hash "
                    f"{index_entry.finalization_generation_hash!r}"
                )
            if len(assignment_ids) != index_entry.assignment_count:
                failures.append(
                    f"child {child_id} assignment count {len(assignment_ids)} != "
                    f"index assignment_count {index_entry.assignment_count}"
                )
        else:
            failures.append(f"child {child_id} has no index entry")

        if len(assignment_ids) != cp.expected_assignment_count:
            failures.append(
                f"child {child_id} assignment count {len(assignment_ids)} != "
                f"expected {cp.expected_assignment_count}"
            )

        if not child_verification.ok:
            failures.extend(
                f"child {child_id}: {f}" for f in child_verification.failures
            )

        # --- Recompute child verification report hash ---
        recomputed_vr_hash = _compute_child_verification_report_hash(report_dir)
        vr_hash_for_result: str
        if recomputed_vr_hash is None:
            hash_failures.append(
                f"child {child_id}: campaign-verification-report.json not found"
            )
            vr_hash_for_result = "0" * 64
        else:
            vr_hash_for_result = recomputed_vr_hash
            if index_entry is not None and recomputed_vr_hash != index_entry.child_verification_report_hash:
                hash_failures.append(
                    f"child {child_id} verification report hash "
                    f"{recomputed_vr_hash!r} != index "
                    f"child_verification_report_hash "
                    f"{index_entry.child_verification_report_hash!r}"
                )

        # --- Recompute report checksum ---
        recomputed_checksum = _recompute_report_checksum(report_dir)
        stored_checksum = _read_stored_report_checksum(report_dir)
        checksum_for_result: str
        if recomputed_checksum is None:
            hash_failures.append(
                f"child {child_id}: attempts.jsonl and metrics.jsonl not found for checksum"
            )
            checksum_for_result = "0" * 64
        else:
            checksum_for_result = recomputed_checksum
            if stored_checksum is not None and recomputed_checksum != stored_checksum:
                hash_failures.append(
                    f"child {child_id} recomputed report checksum "
                    f"{recomputed_checksum!r} != stored checksum "
                    f"{stored_checksum!r}"
                )
            if index_entry is not None and recomputed_checksum != index_entry.report_checksum:
                hash_failures.append(
                    f"child {child_id} recomputed report checksum "
                    f"{recomputed_checksum!r} != index report_checksum "
                    f"{index_entry.report_checksum!r}"
                )

        # --- Prove exact task x repetition x variant product ---
        child_product_ok = True
        task_ids_found: list[str] = []
        replicate_ids_found: list[str] = []
        variant_count = 0

        if not assignment_records:
            child_product_ok = False
            product_failures.append(
                f"child {child_id}: no assignment records to verify product"
            )
        else:
            task_ids_found = sorted({str(r.get("task_id", "")) for r in assignment_records})
            replicate_ids_found = sorted({str(r.get("replicate_id", "")) for r in assignment_records})
            variant_pairs: set[tuple[str, str]] = {
                (str(r.get("model_cohort_id", "")), str(r.get("arm_id", "")))
                for r in assignment_records
            }
            variant_count = len(variant_pairs)

            expected_task_set = set(cp.partition_task_ids)
            actual_task_set = set(task_ids_found)
            if actual_task_set != expected_task_set:
                child_product_ok = False
                extra_tasks = actual_task_set - expected_task_set
                missing_tasks = expected_task_set - actual_task_set
                product_failures.append(
                    f"child {child_id} task IDs do not match partition: "
                    f"extra={sorted(extra_tasks)}, missing={sorted(missing_tasks)}"
                )

            expected_rep_set = set(plan.repetition_ids)
            actual_rep_set = set(replicate_ids_found)
            if actual_rep_set != expected_rep_set:
                child_product_ok = False
                extra_reps = actual_rep_set - expected_rep_set
                missing_reps = expected_rep_set - actual_rep_set
                product_failures.append(
                    f"child {child_id} replicate IDs do not match plan: "
                    f"extra={sorted(extra_reps)}, missing={sorted(missing_reps)}"
                )

            product_tuples: set[tuple[str, str, str, str]] = {
                (
                    str(r.get("task_id", "")),
                    str(r.get("replicate_id", "")),
                    str(r.get("model_cohort_id", "")),
                    str(r.get("arm_id", "")),
                )
                for r in assignment_records
            }

            expected_product_size = len(expected_task_set) * len(expected_rep_set) * variant_count
            if len(product_tuples) != len(assignment_records):
                child_product_ok = False
                product_failures.append(
                    f"child {child_id} duplicate (task, rep, variant) tuples: "
                    f"unique={len(product_tuples)}, total={len(assignment_records)}"
                )

            if child_product_ok and len(product_tuples) != expected_product_size:
                child_product_ok = False
                product_failures.append(
                    f"child {child_id} product size {len(product_tuples)} != "
                    f"expected {expected_product_size} "
                    f"({len(expected_task_set)} tasks x {len(expected_rep_set)} reps "
                    f"x {variant_count} variants)"
                )

            if child_product_ok and expected_product_size != cp.expected_assignment_count:
                child_product_ok = False
                product_failures.append(
                    f"child {child_id} product size {expected_product_size} != "
                    f"child expected_assignment_count {cp.expected_assignment_count}"
                )

        if not child_product_ok:
            failures.extend(product_failures[-1:])

        child_results.append(ChildVerificationResult(
            child_id=child_id,
            ok=child_verification.ok,
            campaign_id=child_verification.campaign_id,
            verified_index_generation_hash=child_verification.verified_index_generation_hash,
            assignment_ids=assignment_ids,
            assignment_count=len(assignment_ids),
            failures=sorted(child_verification.failures),
            task_ids=task_ids_found,
            replicate_ids=replicate_ids_found,
            variant_count=variant_count,
            product_ok=child_product_ok,
            child_verification_report_hash=vr_hash_for_result,
            report_checksum=checksum_for_result,
        ))

    if duplicate_assignment_ids:
        failures.append(
            f"duplicate assignment IDs across children: {sorted(duplicate_assignment_ids)}"
        )

    assignment_uniqueness_ok = len(duplicate_assignment_ids) == 0
    total_unique = len(seen_assignment_ids)
    coverage_ok = total_unique == plan.expected_total_assignment_count
    product_coverage_ok = len(product_failures) == 0
    hash_binding_ok = len(hash_failures) == 0

    if not coverage_ok:
        failures.append(
            f"total unique assignments {total_unique} != expected "
            f"{plan.expected_total_assignment_count}"
        )

    failures.extend(product_failures)
    failures.extend(hash_failures)

    ok = (
        len(failures) == 0
        and assignment_uniqueness_ok
        and coverage_ok
        and product_coverage_ok
        and hash_binding_ok
    )

    result = AggregateVerificationResult.model_construct(
        verification_schema_version=CAMPAIGN_SET_SCHEMA_VERSION,
        set_id=plan.set_id,
        set_plan_hash=plan.content_hash,
        set_index_hash=index.content_hash,
        ok=ok,
        child_results=child_results,
        total_assignments_verified=total_unique,
        assignment_uniqueness_ok=assignment_uniqueness_ok,
        coverage_ok=coverage_ok,
        product_coverage_ok=product_coverage_ok,
        hash_binding_ok=hash_binding_ok,
        failures=sorted(failures),
        content_hash="0" * 64,
    )
    content_hash = compute_aggregate_verification_result_hash(result)
    return result.model_copy(update={"content_hash": content_hash})


def compute_dry_run_plan(plan: CampaignSetPlan) -> dict[str, object]:
    """Compute deterministic dry-run output for a campaign-set plan.

    Returns a dict proving the expected assignment counts per child and
    total, without reading any report directories or making provider
    calls. The output is deterministic: the same plan always produces
    the same output.
    """
    children: list[dict[str, object]] = []
    for cp in plan.child_plans:
        children.append({
            "child_id": cp.child_id,
            "child_revision": cp.child_revision,
            "partition_index": cp.partition_index,
            "tasks_per_child": len(cp.partition_task_ids),
            "repetition_count": len(plan.repetition_ids),
            "expected_assignment_count": cp.expected_assignment_count,
        })
    return {
        "set_id": plan.set_id,
        "set_version": plan.set_version,
        "parent_campaign_id": plan.parent_campaign_id,
        "parent_campaign_revision": plan.parent_campaign_revision,
        "child_count": len(plan.child_plans),
        "total_tasks": len(plan.population_task_ids),
        "repetition_count": len(plan.repetition_ids),
        "expected_child_assignment_count": plan.expected_child_assignment_count,
        "expected_total_assignment_count": plan.expected_total_assignment_count,
        "children": children,
        "content_hash": plan.content_hash,
    }


def build_campaign_set_plan(
    *,
    set_id: str,
    parent_campaign_id: str,
    parent_campaign_revision: str,
    population_task_ids: list[str],
    population_hash: str,
    model_registry_hash: str,
    campaign_profile_hash: str,
    repetition_ids: list[str],
    seed: int,
    retry_policy_hash: str,
    budget_authority_hash: str,
    instrumentation_policy_hash: str,
    expected_record_policy_hash: str,
    orchestrator_environment_scope: str,
    provider_environment_scope: str,
    child_revisions: list[str],
    set_version: str = CAMPAIGN_SET_SCHEMA_VERSION,
) -> CampaignSetPlan:
    """Build a frozen campaign-set plan from the frozen authority inputs.

    Constructs exactly four child plans by partitioning the 120-task
    population into four disjoint 30-task partitions in order. Child IDs
    are derived under the frozen identity rule. The content hash is
    computed and validated.

    Args:
        set_id: Campaign-set identity.
        parent_campaign_id: Parent campaign identity.
        parent_campaign_revision: Parent campaign revision.
        population_task_ids: The full 120-task IFEval population (sorted).
        population_hash: SHA-256 of the 120-task population.
        model_registry_hash: SHA-256 of the frozen model registry.
        campaign_profile_hash: SHA-256 of the frozen campaign profile.
        repetition_ids: Exactly three repetition IDs (sorted).
        seed: Frozen randomization seed.
        retry_policy_hash: SHA-256 of the frozen retry policy.
        budget_authority_hash: SHA-256 of the frozen budget authority.
        instrumentation_policy_hash: SHA-256 of the frozen instrumentation policy.
        expected_record_policy_hash: SHA-256 of the frozen expected-record policy.
        orchestrator_environment_scope: Orchestrator-host environment identity.
        provider_environment_scope: Provider-host environment identity.
        child_revisions: Exactly four child revision identifiers.
        set_version: Campaign-set schema version.

    Returns:
        Frozen ``CampaignSetPlan`` record.
    """
    if len(population_task_ids) != TOTAL_IFEVAL_TASKS:
        raise ValueError(
            f"population_task_ids must have exactly {TOTAL_IFEVAL_TASKS} entries: "
            f"got {len(population_task_ids)}"
        )
    if len(child_revisions) != CHILD_COUNT:
        raise ValueError(
            f"child_revisions must have exactly {CHILD_COUNT} entries: got {len(child_revisions)}"
        )

    sorted_population = sorted(population_task_ids)
    child_plans: list[CampaignChildPlan] = []
    for i in range(CHILD_COUNT):
        start = i * TASKS_PER_CHILD
        end = start + TASKS_PER_CHILD
        partition = sorted(sorted_population[start:end])
        child_id = compute_child_campaign_id(
            parent_campaign_id=parent_campaign_id,
            partition_index=i,
            partition_task_ids=partition,
        )
        child_plans.append(CampaignChildPlan(
            child_id=child_id,
            child_revision=child_revisions[i],
            partition_index=i,
            partition_task_ids=partition,
            expected_assignment_count=EXPECTED_CHILD_ASSIGNMENT_COUNT,
        ))

    plan = CampaignSetPlan.model_construct(
        set_id=set_id,
        set_version=set_version,
        parent_campaign_id=parent_campaign_id,
        parent_campaign_revision=parent_campaign_revision,
        child_plans=child_plans,
        population_task_ids=sorted_population,
        population_hash=population_hash,
        model_registry_hash=model_registry_hash,
        campaign_profile_hash=campaign_profile_hash,
        repetition_ids=sorted(repetition_ids),
        seed=seed,
        retry_policy_hash=retry_policy_hash,
        budget_authority_hash=budget_authority_hash,
        instrumentation_policy_hash=instrumentation_policy_hash,
        expected_record_policy_hash=expected_record_policy_hash,
        orchestrator_environment_scope=orchestrator_environment_scope,
        provider_environment_scope=provider_environment_scope,
        expected_child_assignment_count=EXPECTED_CHILD_ASSIGNMENT_COUNT,
        expected_total_assignment_count=EXPECTED_TOTAL_ASSIGNMENT_COUNT,
        content_hash="0" * 64,
    )
    content_hash = compute_campaign_set_plan_hash(plan)
    return CampaignSetPlan(
        set_id=set_id,
        set_version=set_version,
        parent_campaign_id=parent_campaign_id,
        parent_campaign_revision=parent_campaign_revision,
        child_plans=child_plans,
        population_task_ids=sorted_population,
        population_hash=population_hash,
        model_registry_hash=model_registry_hash,
        campaign_profile_hash=campaign_profile_hash,
        repetition_ids=sorted(repetition_ids),
        seed=seed,
        retry_policy_hash=retry_policy_hash,
        budget_authority_hash=budget_authority_hash,
        instrumentation_policy_hash=instrumentation_policy_hash,
        expected_record_policy_hash=expected_record_policy_hash,
        orchestrator_environment_scope=orchestrator_environment_scope,
        provider_environment_scope=provider_environment_scope,
        expected_child_assignment_count=EXPECTED_CHILD_ASSIGNMENT_COUNT,
        expected_total_assignment_count=EXPECTED_TOTAL_ASSIGNMENT_COUNT,
        content_hash=content_hash,
    )


def write_campaign_set_plan(plan: CampaignSetPlan, output_path: Path) -> None:
    """Write a campaign-set plan to a JSON file."""
    output_path.write_text(plan.model_dump_json(indent=2))


def write_campaign_set_index(index: CampaignSetIndex, output_path: Path) -> None:
    """Write a campaign-set index to a JSON file."""
    output_path.write_text(index.model_dump_json(indent=2))


def load_campaign_set_plan(path: Path) -> CampaignSetPlan:
    """Load a campaign-set plan from a JSON file."""
    return CampaignSetPlan.model_validate_json(path.read_text())


def load_campaign_set_index(path: Path) -> CampaignSetIndex:
    """Load a campaign-set index from a JSON file."""
    return CampaignSetIndex.model_validate_json(path.read_text())


__all__ = [
    "CAMPAIGN_SET_SCHEMA_VERSION",
    "CHILD_COUNT",
    "EXPECTED_CHILD_ASSIGNMENT_COUNT",
    "EXPECTED_TOTAL_ASSIGNMENT_COUNT",
    "REPETITION_COUNT",
    "TASKS_PER_CHILD",
    "TOTAL_IFEVAL_TASKS",
    "AggregateVerificationResult",
    "CampaignChildIndexEntry",
    "CampaignChildPlan",
    "CampaignSetIndex",
    "CampaignSetPlan",
    "CampaignSetStatus",
    "ChildVerificationResult",
    "build_campaign_set_plan",
    "compute_aggregate_verification_result_hash",
    "compute_campaign_set_index_hash",
    "compute_campaign_set_plan_hash",
    "compute_child_campaign_id",
    "compute_dry_run_plan",
    "load_campaign_set_index",
    "load_campaign_set_plan",
    "validate_campaign_set_index",
    "validate_campaign_set_plan",
    "verify_campaign_set_aggregate",
    "write_campaign_set_index",
    "write_campaign_set_plan",
]
