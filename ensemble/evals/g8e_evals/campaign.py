# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed campaign, cohort, assignment, schedule, and retry contracts.

These contracts bind the exact Cartesian dimensions of a release
campaign: model cohorts, task assignments, initial-state assignments,
campaign assignments (one per Cartesian slot), execution schedules,
retry policies, and the campaign manifest that ties every authority
by content hash.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace). Secrets
never appear in these records; cohort identity covers provider, model,
endpoint, sampling, timeout, and capability disposition, not credentials.
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


CAMPAIGN_CONTRACT_VERSION = "1.0.0"


def _canonical_json(model: BaseModel) -> str:
    return json.dumps(
        model.model_dump(mode="json", by_alias=True, exclude_none=False),
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class CampaignStatus(StrEnum):
    """Typed status of a campaign through its lifecycle."""

    PLANNED = "planned"
    RUNNING = "running"
    STOPPED = "stopped"
    COMPLETED = "completed"
    FAILED_INTEGRITY = "failed_integrity"
    FINALIZED = "finalized"


class TerminalSelectionPolicy(StrEnum):
    """Assignment-level outcome selection policy.

    Every started try remains immutable. The final try is the assignment
    outcome; earlier infrastructure failures remain linked diagnostics.
    Model, governance, human, timeout, invalid-evidence, or budget
    outcomes are not silently retried unless the frozen retry policy
    explicitly permits them.
    """

    FINAL_TRY_IS_OUTCOME = "final_try_is_outcome"


class SamplingSettings(BaseModel):
    """Frozen sampling settings for one role-to-model binding.

    These values are part of the cohort content hash. Any change to
    sampling parameters changes the cohort identity and invalidates
    downstream assignment and analysis hashes.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    temperature: float = Field(ge=0.0, le=2.0, description="Sampling temperature.")
    top_p: float = Field(ge=0.0, le=1.0, default=1.0, description="Nucleus sampling threshold.")
    max_tokens: int | None = Field(default=None, ge=1, description="Maximum output tokens. None for provider default.")
    seed: int | None = Field(default=None, ge=0, description="Deterministic sampling seed. None when the model or provider does not support seeding.")


class RoleModelBinding(BaseModel):
    """One role-to-model binding within a model cohort.

    The binding covers the canonical model ID, provider identity,
    endpoint, sampling settings, timeout, and seed capability. If a
    role uses a different model, endpoint, or sampling policy, that
    difference is part of the cohort hash.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    role: str = Field(min_length=1, description="Role name (e.g. primary, assistant, lite).")
    model_id: str = Field(min_length=1, description="Canonical model ID (e.g. qwen3:8b).")
    provider: str = Field(min_length=1, description="Provider ID (e.g. ollama).")
    endpoint: str = Field(min_length=1, description="Endpoint URL.")
    sampling_settings: SamplingSettings = Field(description="Frozen sampling parameters.")
    timeout_seconds: float = Field(gt=0.0, description="Request timeout in seconds.")
    seed_capable: bool = Field(description="Whether the model supports deterministic seeding.")


class ModelCohort(BaseModel):
    """Content-addressed model cohort: a complete role-to-model mapping.

    A cohort is not a display string. It is a frozen, typed record whose
    content hash covers every role-to-model identity, provider identity,
    endpoint class, sampling settings, timeout, and seed capability
    disposition used by each arm. Secrets are never part of this record.

    The ``content_hash`` is SHA-256 over the canonical JSON of the cohort
    (sorted role bindings, sorted keys, no extra whitespace). Changing
    any binding, sampling parameter, endpoint, or timeout changes the
    hash and invalidates downstream assignment and analysis hashes.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    cohort_id: str = Field(min_length=1, description="Unique cohort identifier.")
    role_bindings: list[RoleModelBinding] = Field(
        min_length=1,
        description="Complete role-to-model bindings. One per role used by any arm in the campaign.",
    )
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the cohort (role bindings sorted by role).",
    )

    @model_validator(mode="after")
    def _validate_cohort(self) -> Self:
        roles: list[str] = [rb.role for rb in self.role_bindings]
        if len(roles) != len(set(roles)):
            seen: set[str] = set()
            dupes: list[str] = []
            for r in roles:
                if r in seen:
                    dupes.append(r)
                seen.add(r)
            raise ValueError(f"duplicate role in model cohort: {sorted(set(dupes))}")
        expected = self._compute_hash()
        if self.content_hash != expected:
            raise ValueError(
                f"model cohort content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self

    def _compute_hash(self) -> str:
        sorted_bindings = sorted(self.role_bindings, key=lambda rb: rb.role)
        payload = json.dumps(
            {
                "cohort_id": self.cohort_id,
                "role_bindings": [
                    json.loads(rb.model_dump_json()) for rb in sorted_bindings
                ],
            },
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        return _sha256(payload)


class TaskAssignmentManifest(BaseModel):
    """Frozen typed task assignment manifest.

    Binds the exact task IDs, suite and dataset hash, task order or
    canonical set semantics, and a content hash. The manifest is the
    authoritative declaration of which tasks participate in the campaign.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    task_assignment_id: str = Field(min_length=1, description="Identity of the declared task assignment.")
    suite_id: str = Field(min_length=1, description="Suite identity (e.g. ifeval_subset).")
    dataset_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the dataset content.")
    task_ids: list[str] = Field(
        min_length=1,
        description="Exact task IDs in this assignment. Order is canonical set semantics (sorted).",
    )
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the manifest.",
    )

    @model_validator(mode="after")
    def _validate_task_assignment(self) -> Self:
        if len(self.task_ids) != len(set(self.task_ids)):
            raise ValueError(f"duplicate task IDs in task assignment: {self.task_ids}")
        expected = self._compute_hash()
        if self.content_hash != expected:
            raise ValueError(
                f"task assignment content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self

    def _compute_hash(self) -> str:
        payload = json.dumps(
            {
                "task_assignment_id": self.task_assignment_id,
                "suite_id": self.suite_id,
                "dataset_hash": self.dataset_hash,
                "task_ids": sorted(self.task_ids),
            },
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        return _sha256(payload)


class InitialStateAssignmentManifest(BaseModel):
    """Frozen typed initial-state assignment manifest.

    For answer-only campaigns (IFEval), this manifest contains one
    explicit typed ``no_initial_state`` entry with a content hash rather
    than an unverified string such as ``none``. The manifest proves that
    no initial-state semantics are in scope.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    initial_state_assignment_id: str = Field(min_length=1, description="Identity of the declared initial-state assignment.")
    state_type: str = Field(
        min_length=1,
        description="Typed initial-state type. 'no_initial_state' for answer-only campaigns.",
    )
    snapshot_hash: str = Field(
        min_length=64,
        max_length=64,
        description="Content-addressed snapshot hash. For no_initial_state, a fixed hash of the typed exclusion.",
    )
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the manifest.",
    )

    @model_validator(mode="after")
    def _validate_initial_state(self) -> Self:
        expected = self._compute_hash()
        if self.content_hash != expected:
            raise ValueError(
                f"initial-state assignment content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self

    def _compute_hash(self) -> str:
        payload = json.dumps(
            {
                "initial_state_assignment_id": self.initial_state_assignment_id,
                "state_type": self.state_type,
                "snapshot_hash": self.snapshot_hash,
            },
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        return _sha256(payload)


class CampaignAssignment(BaseModel):
    """One Cartesian slot in the campaign.

    Each assignment binds: campaign ID, assignment ID, task ID, model
    cohort ID, arm ID, initial-state assignment ID, replicate ID, and
    the persisted schedule position. The assignment ID is derived from
    canonical content so that the same (task, cohort, arm, initial-state,
    replicate) always produces the same assignment ID.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    assignment_id: str = Field(min_length=1, description="Canonical assignment identity derived from content.")
    task_id: str = Field(min_length=1, description="Task ID from the task assignment manifest.")
    model_cohort_id: str = Field(min_length=1, description="Model cohort ID from the cohort definition.")
    arm_id: str = Field(min_length=1, description="Arm ID (e.g. direct, ensemble_ungoverned).")
    initial_state_assignment_id: str = Field(
        min_length=1,
        description="Initial-state assignment ID from the initial-state manifest.",
    )
    replicate_id: str = Field(min_length=1, description="Replicate ID from the preregistration.")
    schedule_position: int = Field(ge=0, description="Persisted schedule position (0-indexed).")


class ExecutionSchedule(BaseModel):
    """Frozen typed execution schedule.

    Binds the randomization seed, algorithm ID/version, the complete
    ordered assignment ID list, and a content hash. Requires contiguous
    unique positions and exact set equality with the campaign's
    assignment IDs.

    The schedule is built once from the frozen seed and persisted before
    the first assignment starts. Resume reads the same schedule and
    existing immutable attempts; it does not rerandomize or selectively
    discard failures.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    schedule_id: str = Field(min_length=1, description="Schedule identity.")
    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    randomization_seed: int = Field(ge=0, description="Frozen randomization seed for the complete schedule.")
    algorithm_id: str = Field(min_length=1, description="Schedule algorithm identifier.")
    algorithm_version: str = Field(min_length=1, description="Schedule algorithm version.")
    ordered_assignment_ids: list[str] = Field(
        min_length=1,
        description="Complete ordered assignment ID list. Position i maps to schedule position i.",
    )
    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the schedule.",
    )

    @model_validator(mode="after")
    def _validate_schedule(self) -> Self:
        if len(self.ordered_assignment_ids) != len(set(self.ordered_assignment_ids)):
            raise ValueError(
                f"duplicate assignment IDs in schedule: {self.ordered_assignment_ids}"
            )
        expected = self._compute_hash()
        if self.content_hash != expected:
            raise ValueError(
                f"execution schedule content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self

    def _compute_hash(self) -> str:
        payload = json.dumps(
            {
                "schedule_id": self.schedule_id,
                "campaign_id": self.campaign_id,
                "randomization_seed": self.randomization_seed,
                "algorithm_id": self.algorithm_id,
                "algorithm_version": self.algorithm_version,
                "ordered_assignment_ids": self.ordered_assignment_ids,
            },
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        return _sha256(payload)


class RetryPolicy(BaseModel):
    """Frozen typed retry policy for infrastructure retries.

    Defines which terminal statuses are retryable and the maximum number
    of retries per assignment. Model, governance, human, timeout,
    invalid-evidence, or budget outcomes are not silently retried unless
    this policy explicitly permits them.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    max_retries: int = Field(ge=0, description="Maximum infrastructure retries per assignment.")
    retryable_terminal_statuses: list[str] = Field(
        min_length=0,
        description="Terminal status strings that are retryable. Typically only 'infrastructure_failed'.",
    )

    @model_validator(mode="after")
    def _validate_retry_policy(self) -> Self:
        if len(self.retryable_terminal_statuses) != len(set(self.retryable_terminal_statuses)):
            raise ValueError(
                f"duplicate retryable terminal statuses: {self.retryable_terminal_statuses}"
            )
        return self


class CampaignManifest(BaseModel):
    """Immutable campaign manifest binding every authority by content hash.

    The manifest binds preregistration, cohort definitions, task
    assignment, initial-state assignment, schedule, metric registry,
    release metric set, threshold/non-inferiority authority, missingness
    authority, provider budget, source/build provenance requirements,
    and claim exclusions by typed content hash.

    Changing any bound authority changes the manifest content hash and
    invalidates downstream analysis input hashes.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_version: str = Field(min_length=1, description="Campaign contract version.")
    release_version: str = Field(min_length=1, description="Release version (e.g. v2.1.8).")

    preregistration_hash: str = Field(min_length=64, max_length=64)
    cohort_hashes: list[str] = Field(
        min_length=1,
        description="Content hashes of each model cohort, sorted.",
    )
    task_assignment_hash: str = Field(min_length=64, max_length=64)
    initial_state_assignment_hash: str = Field(min_length=64, max_length=64)
    schedule_hash: str = Field(min_length=64, max_length=64)
    retry_policy_hash: str = Field(min_length=64, max_length=64)
    metric_registry_hash: str = Field(min_length=64, max_length=64)
    release_metric_set_hash: str = Field(min_length=64, max_length=64)
    threshold_authority_hash: str = Field(min_length=64, max_length=64)
    missingness_authority_hash: str = Field(min_length=64, max_length=64)
    provider_budget_hash: str = Field(min_length=64, max_length=64)
    source_build_provenance_hash: str = Field(min_length=64, max_length=64)
    claim_exclusion_hash: str = Field(min_length=64, max_length=64)

    content_hash: str = Field(
        min_length=64,
        max_length=64,
        description="SHA-256 over canonical JSON of the manifest.",
    )

    @model_validator(mode="after")
    def _validate_manifest(self) -> Self:
        if len(self.cohort_hashes) != len(set(self.cohort_hashes)):
            raise ValueError(f"duplicate cohort hashes in campaign manifest: {self.cohort_hashes}")
        expected = self._compute_hash()
        if self.content_hash != expected:
            raise ValueError(
                f"campaign manifest content_hash mismatch: declared {self.content_hash!r}, computed {expected!r}"
            )
        return self

    def _compute_hash(self) -> str:
        payload = json.dumps(
            {
                "campaign_id": self.campaign_id,
                "campaign_version": self.campaign_version,
                "release_version": self.release_version,
                "preregistration_hash": self.preregistration_hash,
                "cohort_hashes": sorted(self.cohort_hashes),
                "task_assignment_hash": self.task_assignment_hash,
                "initial_state_assignment_hash": self.initial_state_assignment_hash,
                "schedule_hash": self.schedule_hash,
                "retry_policy_hash": self.retry_policy_hash,
                "metric_registry_hash": self.metric_registry_hash,
                "release_metric_set_hash": self.release_metric_set_hash,
                "threshold_authority_hash": self.threshold_authority_hash,
                "missingness_authority_hash": self.missingness_authority_hash,
                "provider_budget_hash": self.provider_budget_hash,
                "source_build_provenance_hash": self.source_build_provenance_hash,
                "claim_exclusion_hash": self.claim_exclusion_hash,
            },
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        return _sha256(payload)


def compute_model_cohort_hash(cohort_id: str, role_bindings: list[RoleModelBinding]) -> str:
    """Compute the content hash for a model cohort without constructing the full model."""
    sorted_bindings = sorted(role_bindings, key=lambda rb: rb.role)
    payload = json.dumps(
        {
            "cohort_id": cohort_id,
            "role_bindings": [
                json.loads(rb.model_dump_json()) for rb in sorted_bindings
            ],
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_task_assignment_hash(
    task_assignment_id: str,
    suite_id: str,
    dataset_hash: str,
    task_ids: list[str],
) -> str:
    """Compute the content hash for a task assignment manifest."""
    payload = json.dumps(
        {
            "task_assignment_id": task_assignment_id,
            "suite_id": suite_id,
            "dataset_hash": dataset_hash,
            "task_ids": sorted(task_ids),
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_initial_state_hash(
    initial_state_assignment_id: str,
    state_type: str,
    snapshot_hash: str,
) -> str:
    """Compute the content hash for an initial-state assignment manifest."""
    payload = json.dumps(
        {
            "initial_state_assignment_id": initial_state_assignment_id,
            "state_type": state_type,
            "snapshot_hash": snapshot_hash,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_schedule_hash(
    schedule_id: str,
    campaign_id: str,
    randomization_seed: int,
    algorithm_id: str,
    algorithm_version: str,
    ordered_assignment_ids: list[str],
) -> str:
    """Compute the content hash for an execution schedule."""
    payload = json.dumps(
        {
            "schedule_id": schedule_id,
            "campaign_id": campaign_id,
            "randomization_seed": randomization_seed,
            "algorithm_id": algorithm_id,
            "algorithm_version": algorithm_version,
            "ordered_assignment_ids": ordered_assignment_ids,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_retry_policy_hash(
    max_retries: int,
    retryable_terminal_statuses: list[str],
) -> str:
    """Compute the content hash for a retry policy."""
    payload = json.dumps(
        {
            "max_retries": max_retries,
            "retryable_terminal_statuses": sorted(retryable_terminal_statuses),
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_campaign_manifest_hash(
    campaign_id: str,
    campaign_version: str,
    release_version: str,
    preregistration_hash: str,
    cohort_hashes: list[str],
    task_assignment_hash: str,
    initial_state_assignment_hash: str,
    schedule_hash: str,
    retry_policy_hash: str,
    metric_registry_hash: str,
    release_metric_set_hash: str,
    threshold_authority_hash: str,
    missingness_authority_hash: str,
    provider_budget_hash: str,
    source_build_provenance_hash: str,
    claim_exclusion_hash: str,
) -> str:
    """Compute the content hash for a campaign manifest."""
    payload = json.dumps(
        {
            "campaign_id": campaign_id,
            "campaign_version": campaign_version,
            "release_version": release_version,
            "preregistration_hash": preregistration_hash,
            "cohort_hashes": sorted(cohort_hashes),
            "task_assignment_hash": task_assignment_hash,
            "initial_state_assignment_hash": initial_state_assignment_hash,
            "schedule_hash": schedule_hash,
            "retry_policy_hash": retry_policy_hash,
            "metric_registry_hash": metric_registry_hash,
            "release_metric_set_hash": release_metric_set_hash,
            "threshold_authority_hash": threshold_authority_hash,
            "missingness_authority_hash": missingness_authority_hash,
            "provider_budget_hash": provider_budget_hash,
            "source_build_provenance_hash": source_build_provenance_hash,
            "claim_exclusion_hash": claim_exclusion_hash,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def compute_assignment_id(
    campaign_id: str,
    task_id: str,
    model_cohort_id: str,
    arm_id: str,
    initial_state_assignment_id: str,
    replicate_id: str,
) -> str:
    """Derive a canonical assignment ID from the Cartesian slot content.

    The assignment ID is deterministic: the same (campaign, task, cohort,
    arm, initial-state, replicate) always produces the same assignment ID.
    """
    payload = json.dumps(
        {
            "campaign_id": campaign_id,
            "task_id": task_id,
            "model_cohort_id": model_cohort_id,
            "arm_id": arm_id,
            "initial_state_assignment_id": initial_state_assignment_id,
            "replicate_id": replicate_id,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def validate_campaign_assignments(
    assignments: list[CampaignAssignment],
    task_ids: list[str],
    cohort_ids: list[str],
    arm_ids: list[str],
    replicate_ids: list[str],
    initial_state_assignment_id: str,
    campaign_id: str,
) -> None:
    """Validate the complete Cartesian product of campaign assignments.

    Rejects missing, duplicate, or extra slots. Every assignment must
    reference a declared task, cohort, arm, replicate, and initial-state
    assignment. The total count must equal
    ``len(task_ids) * len(cohort_ids) * len(arm_ids) * len(replicate_ids)``.
    """
    expected_count = len(task_ids) * len(cohort_ids) * len(arm_ids) * len(replicate_ids)
    if len(assignments) != expected_count:
        if len(assignments) < expected_count:
            raise ValueError(
                f"missing campaign assignment: count mismatch: expected {expected_count} "
                f"({len(task_ids)} tasks x {len(cohort_ids)} cohorts x {len(arm_ids)} arms "
                f"x {len(replicate_ids)} replicates), found {len(assignments)}"
            )
        raise ValueError(
            f"unexpected campaign assignment: count mismatch: expected {expected_count} "
            f"({len(task_ids)} tasks x {len(cohort_ids)} cohorts x {len(arm_ids)} arms "
            f"x {len(replicate_ids)} replicates), found {len(assignments)}"
        )

    assignment_ids = [a.assignment_id for a in assignments]
    if len(assignment_ids) != len(set(assignment_ids)):
        raise ValueError("duplicate assignment IDs in campaign")

    task_set = set(task_ids)
    cohort_set = set(cohort_ids)
    arm_set = set(arm_ids)
    replicate_set = set(replicate_ids)

    for a in assignments:
        if a.campaign_id != campaign_id:
            raise ValueError(
                f"assignment {a.assignment_id} campaign_id {a.campaign_id!r} != {campaign_id!r}"
            )
        if a.task_id not in task_set:
            raise ValueError(
                f"assignment {a.assignment_id} task_id {a.task_id!r} not in declared task set"
            )
        if a.model_cohort_id not in cohort_set:
            raise ValueError(
                f"assignment {a.assignment_id} model_cohort_id {a.model_cohort_id!r} not in declared cohort set"
            )
        if a.arm_id not in arm_set:
            raise ValueError(
                f"assignment {a.assignment_id} arm_id {a.arm_id!r} not in declared arm set"
            )
        if a.replicate_id not in replicate_set:
            raise ValueError(
                f"assignment {a.assignment_id} replicate_id {a.replicate_id!r} not in declared replicate set"
            )
        if a.initial_state_assignment_id != initial_state_assignment_id:
            raise ValueError(
                f"assignment {a.assignment_id} initial_state_assignment_id "
                f"{a.initial_state_assignment_id!r} != {initial_state_assignment_id!r}"
            )

    expected_slots: set[tuple[str, str, str, str]] = set()
    for task_id in task_ids:
        for cohort_id in cohort_ids:
            for arm_id in arm_ids:
                for replicate_id in replicate_ids:
                    expected_slots.add((task_id, cohort_id, arm_id, replicate_id))

    actual_slots: set[tuple[str, str, str, str]] = set()
    for a in assignments:
        slot = (a.task_id, a.model_cohort_id, a.arm_id, a.replicate_id)
        if slot in actual_slots:
            raise ValueError(
                f"duplicate Cartesian slot: (task={a.task_id}, cohort={a.model_cohort_id}, "
                f"arm={a.arm_id}, replicate={a.replicate_id})"
            )
        actual_slots.add(slot)

    missing = expected_slots - actual_slots
    extra = actual_slots - expected_slots
    if missing:
        raise ValueError(f"missing campaign assignment slots: {sorted(missing)}")
    if extra:
        raise ValueError(f"unexpected campaign assignment slots: {sorted(extra)}")


def validate_schedule(
    schedule: ExecutionSchedule,
    assignments: list[CampaignAssignment],
) -> None:
    """Validate that a schedule covers exactly the campaign assignments.

    Requires contiguous unique positions (0 to N-1) and exact set
    equality between the schedule's ordered assignment IDs and the
    campaign's assignment IDs.
    """

    schedule_ids = schedule.ordered_assignment_ids
    assignment_ids = [a.assignment_id for a in assignments]

    if len(schedule_ids) != len(set(schedule_ids)):
        raise ValueError("duplicate assignment IDs in schedule")

    if set(schedule_ids) != set(assignment_ids):
        schedule_only = set(schedule_ids) - set(assignment_ids)
        assignment_only = set(assignment_ids) - set(schedule_ids)
        raise ValueError(
            f"schedule/assignment set mismatch: schedule_only={sorted(schedule_only)}, "
            f"assignment_only={sorted(assignment_only)}"
        )

    for _i, a in enumerate(assignments):
        if a.schedule_position < 0 or a.schedule_position >= len(schedule_ids):
            raise ValueError(
                f"assignment {a.assignment_id} schedule_position {a.schedule_position} "
                f"out of range [0, {len(schedule_ids)})"
            )
        if schedule_ids[a.schedule_position] != a.assignment_id:
            raise ValueError(
                f"assignment {a.assignment_id} schedule_position {a.schedule_position} "
                f"does not match schedule entry {schedule_ids[a.schedule_position]!r}"
            )


def validate_retry_chain(
    attempts: list,
    assignment_id: str,
    retry_policy: RetryPolicy,
) -> None:
    """Validate the retry chain for one assignment's attempts.

    Rejects cycles, forks, cross-assignment parents, duplicate attempt
    IDs, and attempts exceeding the frozen retry limit. Each attempt
    must have ``assignment_id`` and ``parent_attempt_id`` attributes.

    The retry chain is a linear chain: attempt 0 has no parent, attempt 1
    has parent attempt 0, etc. Forks (multiple children of one parent)
    and cycles are rejected. The total number of attempts must not
    exceed ``max_retries + 1`` (the initial try plus allowed retries).
    """
    assignment_attempts = [a for a in attempts if a.assignment_id == assignment_id]
    if not assignment_attempts:
        return

    attempt_ids = [a.attempt_id for a in assignment_attempts]
    if len(attempt_ids) != len(set(attempt_ids)):
        raise ValueError(
            f"duplicate attempt IDs for assignment {assignment_id}: {attempt_ids}"
        )

    max_attempts = retry_policy.max_retries + 1
    if len(assignment_attempts) > max_attempts:
        raise ValueError(
            f"assignment {assignment_id} has {len(assignment_attempts)} attempts, "
            f"exceeding max_retries+1={max_attempts}"
        )

    for a in assignment_attempts:
        if a.assignment_id != assignment_id:
            raise ValueError(
                f"attempt {a.attempt_id} assignment_id {a.assignment_id!r} != {assignment_id!r}"
            )

    parents = {a.attempt_id: a.parent_attempt_id for a in assignment_attempts}
    for attempt_id, parent_id in parents.items():
        if parent_id is not None and parent_id not in parents:
            raise ValueError(
                f"attempt {attempt_id} parent_attempt_id {parent_id!r} not in assignment {assignment_id}"
            )

    for attempt_id, parent_id in parents.items():
        if parent_id is not None:
            if parent_id == attempt_id:
                raise ValueError(
                    f"attempt {attempt_id} has self-referential parent (cycle)"
                )
            visited: set[str] = set()
            current = parent_id
            while current is not None:
                if current in visited:
                    raise ValueError(
                        f"retry chain cycle detected for attempt {attempt_id}"
                    )
                visited.add(current)
                current = parents.get(current)

    children_by_parent: dict[str | None, list[str]] = {}
    for attempt_id, parent_id in parents.items():
        children_by_parent.setdefault(parent_id, []).append(attempt_id)
    for parent_id, children in children_by_parent.items():
        if len(children) > 1:
            raise ValueError(
                f"retry chain fork: parent {parent_id!r} has multiple children: {sorted(children)}"
            )


__all__ = [
    "CAMPAIGN_CONTRACT_VERSION",
    "CampaignAssignment",
    "CampaignManifest",
    "CampaignStatus",
    "ExecutionSchedule",
    "InitialStateAssignmentManifest",
    "ModelCohort",
    "RetryPolicy",
    "RoleModelBinding",
    "SamplingSettings",
    "TaskAssignmentManifest",
    "TerminalSelectionPolicy",
    "compute_assignment_id",
    "compute_campaign_manifest_hash",
    "compute_initial_state_hash",
    "compute_model_cohort_hash",
    "compute_retry_policy_hash",
    "compute_schedule_hash",
    "compute_task_assignment_hash",
    "validate_campaign_assignments",
    "validate_retry_chain",
    "validate_schedule",
]
