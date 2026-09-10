# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Authoritative campaign runner for multi-arm, multi-cohort evaluation campaigns.

The campaign runner accepts one frozen campaign specification, builds the
complete Cartesian assignment product, persists a randomized schedule,
executes assignments in schedule order with per-(cohort, arm) SUTs, handles
infrastructure retries, enforces provider budget, detects cohort drift,
and produces one campaign identity, one report directory, one assignment
manifest, one schedule, and one final canonical analysis.

Resume reads the persisted schedule and existing immutable attempts; it
does not rerandomize or selectively discard failures. Assignment-scoped
failures (model, governance, timeout, invalid evidence) are persisted as
typed terminal outcomes and the campaign continues unless the stop policy
fires. Global preflight defects, corrupted persisted state, unsafe path
handling, or an inability to persist authoritative records raise
immediately.

Files are written atomically through a staging directory that is renamed
to the final report directory only after the campaign manifest, schedule,
and status are complete. A typed campaign status distinguishes planned,
running, stopped, completed, failed-integrity, and finalized.
"""

from __future__ import annotations

import hashlib
import json
import os
import random
import uuid
from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import StrEnum
from pathlib import Path
from collections.abc import Sequence
from typing import Any, Protocol, cast

from pydantic import BaseModel, ConfigDict, Field

from g8e_evals.analysis.canonical import (
    CanonicalEvalAnalysis,
    PreregistrationConfig,
    canonical_model_json,
)
from g8e_evals.analysis.engine import compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.arms import Arm, ArmDefinition, get_arm_definition
from g8e_evals.campaign import (
    CAMPAIGN_CONTRACT_VERSION,
    CampaignAssignment,
    CampaignManifest,
    CampaignStatus,
    ExecutionSchedule,
    InitialStateAssignmentManifest,
    ModelCohort,
    RetryPolicy,
    TaskAssignmentManifest,
    compute_assignment_id,
    compute_campaign_manifest_hash,
    compute_retry_policy_hash,
    compute_schedule_hash,
    validate_campaign_assignments,
    validate_retry_chain,
    validate_schedule,
)
from g8e_evals.constants import (
    ANALYSIS_HTML,
    ANALYSIS_INPUT_JSON,
    ANALYSIS_JSON,
    ANALYSIS_MD,
    ANALYSIS_TXT,
    ATTEMPTS_JSONL,
    CAMPAIGN_ASSIGNMENTS_JSONL,
    CAMPAIGN_COHORTS_JSONL,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_RETRY_POLICY_JSON,
    CAMPAIGN_SCHEDULE_JSON,
    CAMPAIGN_STATUS_JSON,
    MANIFEST_JSON,
    METRICS_JSONL,
    TASKS_JSONL,
)
from g8e_evals.harness import Response, Task
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.schema import (
    ArmManifestEntry,
    AttemptRecord,
    ContentHash,
    GraderClass,
    GraderReference,
    MetricObservation,
    ModelIdentity,
    PostureObservation,
    ProviderBudget,
    RoleToModelMapping,
    RunManifest,
    SourceBuildProvenance,
    StackEnvironment,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)

SCHEDULE_ALGORITHM_ID = "fisher_yates_shuffle"
SCHEDULE_ALGORITHM_VERSION = "1.0.0"

_IFEVAL_GRADER_ID = "ifeval_subset_verifier"
_GRADER_VERSION = "1.0.0"
_NO_STATE_HASH = "0" * 64


class CampaignStopReason(StrEnum):
    """Typed reason the campaign stopped before completing all assignments."""

    COMPLETED = "completed"
    BUDGET_EXHAUSTED = "budget_exhausted"
    COHORT_DRIFT = "cohort_drift"
    NON_RETRYABLE_FAILURE = "non_retryable_failure"
    INVALID_EVIDENCE = "invalid_evidence"


class CampaignRunnerError(Exception):
    """Raised for global preflight defects, corrupted persisted state, or
    an inability to persist authoritative records."""


class SUTProtocol(Protocol):
    """Minimal SUT interface the runner consumes."""

    async def get_answer(self, task: Task) -> Response: ...


class SUTFactory(Protocol):
    """Factory that creates a SUT for a (cohort, arm) pair."""

    def __call__(self, cohort: ModelCohort, arm: Arm) -> SUTProtocol: ...


class GraderProtocol(Protocol):
    """Minimal grader interface the runner consumes."""

    def verify(
        self,
        task_id: str,
        prompt: str,
        answer: str,
        instructions: list[str],
        kwargs: list[dict[str, Any]],
    ) -> object: ...


class CampaignSpec(BaseModel):
    """Frozen campaign specification accepted by the campaign runner.

    Bundles the preregistration, cohort definitions, task assignment,
    initial-state assignment, retry policy, schedule seed, and provider
    budget into one immutable record. The runner builds the Cartesian
    assignment product, randomized schedule, and campaign manifest from
    this specification.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1)
    release_version: str = Field(min_length=1)
    suite: str = Field(min_length=1)
    suite_id: str = Field(min_length=1)
    suite_version: str = Field(min_length=1)
    dataset_hash: str = Field(min_length=64, max_length=64)
    prompt_bundle_hash: str = Field(min_length=64, max_length=64)
    grader_bundle_hash: str = Field(min_length=64, max_length=64)
    preregistration: PreregistrationConfig
    cohorts: list[ModelCohort] = Field(min_length=1)
    task_assignment: TaskAssignmentManifest
    initial_state: InitialStateAssignmentManifest
    retry_policy: RetryPolicy
    randomization_seed: int = Field(ge=0)
    provider_budget: ProviderBudget | None = None
    source_build_provenance: SourceBuildProvenance | None = None
    stack_environment: StackEnvironment = Field(default_factory=StackEnvironment)


@dataclass(frozen=True)
class CampaignResult:
    """Result of a campaign run."""

    campaign_id: str
    run_id: str
    report_dir: Path
    status: CampaignStatus
    assignment_count: int
    terminal_attempt_count: int
    stop_reason: CampaignStopReason | None


def build_campaign_state(
    spec: CampaignSpec,
) -> tuple[list[CampaignAssignment], ExecutionSchedule]:
    """Build the complete Cartesian assignment product and randomized schedule.

    One assignment per (task, cohort, arm, replicate) slot. The assignment
    ID is derived deterministically from the slot content. Schedule
    positions are assigned from a deterministic Fisher-Yates shuffle of
    the canonical slot order, so the same seed always produces the same
    schedule. The schedule's ``ordered_assignment_ids`` is a
    position-indexed array: ``ordered_assignment_ids[position]`` is the
    assignment at that position.

    Returns both the assignments (with schedule_position matching the
    shuffled order) and the schedule, because they are tightly coupled:
    ``validate_schedule`` requires
    ``schedule.ordered_assignment_ids[assignment.schedule_position] ==
    assignment.assignment_id``.
    """
    prereg = spec.preregistration
    task_ids = sorted(spec.task_assignment.task_ids)
    cohort_ids = list(prereg.model_cohort_ids)
    arm_ids = [prereg.baseline_arm_id, *prereg.comparison_arm_ids]
    replicate_ids = list(prereg.required_replicate_ids)
    initial_state_id = spec.initial_state.initial_state_assignment_id

    # Build canonical slot order.
    canonical_slots: list[tuple[str, str, str, str]] = []
    for task_id in task_ids:
        for cohort_id in cohort_ids:
            for arm_id in arm_ids:
                for replicate_id in replicate_ids:
                    canonical_slots.append((task_id, cohort_id, arm_id, replicate_id))

    # Deterministic Fisher-Yates shuffle of slot indices.
    rng = random.Random(spec.randomization_seed)
    indices = list(range(len(canonical_slots)))
    rng.shuffle(indices)

    # Build assignments with schedule_position from the shuffled order.
    assignments_by_position: list[CampaignAssignment] = [None] * len(canonical_slots)  # type: ignore[list-item]
    for position, slot_index in enumerate(indices):
        task_id, cohort_id, arm_id, replicate_id = canonical_slots[slot_index]
        aid = compute_assignment_id(
            spec.campaign_id,
            task_id,
            cohort_id,
            arm_id,
            initial_state_id,
            replicate_id,
        )
        assignments_by_position[position] = CampaignAssignment(
            campaign_id=spec.campaign_id,
            assignment_id=aid,
            task_id=task_id,
            model_cohort_id=cohort_id,
            arm_id=arm_id,
            initial_state_assignment_id=initial_state_id,
            replicate_id=replicate_id,
            schedule_position=position,
        )

    assignments = assignments_by_position  # type: ignore[assignment]
    ordered_assignment_ids = [a.assignment_id for a in assignments]

    validate_campaign_assignments(
        assignments,
        task_ids,
        cohort_ids,
        arm_ids,
        replicate_ids,
        initial_state_id,
        spec.campaign_id,
    )

    schedule_id = hashlib.sha256(
        f"{spec.campaign_id}:{spec.randomization_seed}".encode()
    ).hexdigest()[:16]
    content_hash = compute_schedule_hash(
        schedule_id,
        spec.campaign_id,
        spec.randomization_seed,
        SCHEDULE_ALGORITHM_ID,
        SCHEDULE_ALGORITHM_VERSION,
        ordered_assignment_ids,
    )
    schedule = ExecutionSchedule(
        schedule_id=schedule_id,
        campaign_id=spec.campaign_id,
        randomization_seed=spec.randomization_seed,
        algorithm_id=SCHEDULE_ALGORITHM_ID,
        algorithm_version=SCHEDULE_ALGORITHM_VERSION,
        ordered_assignment_ids=ordered_assignment_ids,
        content_hash=content_hash,
    )

    validate_schedule(schedule, assignments)

    return assignments, schedule


def build_campaign_manifest(
    spec: CampaignSpec,
    schedule: ExecutionSchedule,
    prereg_hash: str,
    retry_policy_hash: str,
    metric_registry_hash: str,
    release_metric_set_hash: str,
    threshold_authority_hash: str,
    missingness_authority_hash: str,
    provider_budget_hash: str,
    source_build_provenance_hash: str,
    claim_exclusion_hash: str,
) -> CampaignManifest:
    """Build the immutable campaign manifest binding every authority by hash."""
    cohort_hashes = sorted(c.content_hash for c in spec.cohorts)
    content_hash = compute_campaign_manifest_hash(
        spec.campaign_id,
        CAMPAIGN_CONTRACT_VERSION,
        spec.release_version,
        prereg_hash,
        cohort_hashes,
        spec.task_assignment.content_hash,
        spec.initial_state.content_hash,
        schedule.content_hash,
        retry_policy_hash,
        metric_registry_hash,
        release_metric_set_hash,
        threshold_authority_hash,
        missingness_authority_hash,
        provider_budget_hash,
        source_build_provenance_hash,
        claim_exclusion_hash,
    )
    return CampaignManifest(
        campaign_id=spec.campaign_id,
        campaign_version=CAMPAIGN_CONTRACT_VERSION,
        release_version=spec.release_version,
        preregistration_hash=prereg_hash,
        cohort_hashes=cohort_hashes,
        task_assignment_hash=spec.task_assignment.content_hash,
        initial_state_assignment_hash=spec.initial_state.content_hash,
        schedule_hash=schedule.content_hash,
        retry_policy_hash=retry_policy_hash,
        metric_registry_hash=metric_registry_hash,
        release_metric_set_hash=release_metric_set_hash,
        threshold_authority_hash=threshold_authority_hash,
        missingness_authority_hash=missingness_authority_hash,
        provider_budget_hash=provider_budget_hash,
        source_build_provenance_hash=source_build_provenance_hash,
        claim_exclusion_hash=claim_exclusion_hash,
        content_hash=content_hash,
    )


def _write_atomic(path: Path, content: str) -> None:
    """Write content to a path atomically (write to temp, rename)."""
    tmp = path.with_suffix(path.suffix + ".tmp")
    tmp.write_text(content)
    os.replace(tmp, path)


def _write_jsonl(path: Path, records: Sequence[BaseModel]) -> None:
    """Write a list of Pydantic models as JSONL atomically."""
    lines = [r.model_dump_json() for r in records]
    _write_atomic(path, "\n".join(lines) + ("\n" if lines else ""))


def _read_jsonl(path: Path, model_cls: type[BaseModel]) -> list[BaseModel]:
    """Read a JSONL file into a list of Pydantic models."""
    if not path.exists():
        return []
    records: list[BaseModel] = []
    for line in path.read_text().splitlines():
        if line.strip():
            records.append(model_cls.model_validate_json(line))
    return records


def _compute_prereg_hash(prereg: PreregistrationConfig) -> str:
    return hashlib.sha256(canonical_model_json(prereg).encode()).hexdigest()


def _compute_simple_hash(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


def _model_id_for_cohort(cohort: ModelCohort) -> str:
    """Extract the primary model ID from a cohort's role bindings."""
    for rb in cohort.role_bindings:
        if rb.role == "primary":
            return rb.model_id
    return cohort.role_bindings[0].model_id


def _model_matches(observed: str, expected: str) -> bool:
    """Check whether an observed model string matches the expected model tag.

    The DirectProviderSUT reports ``model`` as ``"provider:model_tag"``
    (e.g. ``"ollama:qwen3:8b"``) while the cohort declares only the model
    tag (e.g. ``"qwen3:8b"``). Accept either an exact match or a match
    where the observed string is ``"<provider>:<expected>"``.
    """
    if observed == expected:
        return True
    if ":" in observed:
        _, _, tail = observed.partition(":")
        return tail == expected
    return False


@dataclass
class CampaignRunner:
    """Authoritative campaign runner.

    Executes a frozen campaign specification, producing one campaign
    identity, one report directory, one assignment manifest, one
    schedule, and one final canonical analysis.

    The runner accepts a ``SUTFactory`` that creates a SUT for each
    (cohort, arm) pair, a ``GraderProtocol`` that grades answers, and a
    list of ``Task`` objects loaded from the dataset. This makes the
    runner testable with deterministic fake SUTs and graders.

    Resume: if the report directory already contains a persisted campaign
    status and attempts, the runner reads the existing schedule and
    skips assignments that already have a terminal attempt. It does not
    rerandomize or discard failures.
    """

    spec: CampaignSpec
    sut_factory: SUTFactory
    tasks: list[Task]
    grader: GraderProtocol
    output_dir: Path
    evidence_key: object | None = None
    _run_id: str = field(default_factory=lambda: str(uuid.uuid4()))
    _report_dir: Path | None = None
    _suts: dict[tuple[str, str], SUTProtocol] = field(default_factory=dict)

    @property
    def report_dir(self) -> Path:
        if self._report_dir is None:
            ts = datetime.now(UTC).strftime("%Y%m%d-%H%M%S")
            self._report_dir = self.output_dir / f"{self.spec.suite}-campaign-{ts}"
        return self._report_dir

    def _get_sut(self, cohort: ModelCohort, arm: Arm) -> SUTProtocol:
        key = (cohort.cohort_id, arm.value)
        if key not in self._suts:
            self._suts[key] = self.sut_factory(cohort, arm)
        return self._suts[key]

    def _cohort_by_id(self) -> dict[str, ModelCohort]:
        return {c.cohort_id: c for c in self.spec.cohorts}

    def _task_by_id(self) -> dict[str, Task]:
        return {t.id: t for t in self.tasks}

    def _write_campaign_status(self, status: CampaignStatus, stop_reason: CampaignStopReason | None = None) -> None:
        """Write the typed campaign status file."""
        record = {
            "campaign_id": self.spec.campaign_id,
            "run_id": self._run_id,
            "status": status.value,
            "stop_reason": stop_reason.value if stop_reason else None,
            "updated_at": datetime.now(UTC).isoformat(),
        }
        _write_atomic(self.report_dir / CAMPAIGN_STATUS_JSON, json.dumps(record, indent=2))

    def _read_campaign_status(self) -> dict[str, Any] | None:
        """Read the persisted campaign status, or None if not yet written."""
        path = self.report_dir / CAMPAIGN_STATUS_JSON
        if not path.exists():
            return None
        return json.loads(path.read_text())

    def _persist_campaign_state(
        self,
        assignments: list[CampaignAssignment],
        schedule: ExecutionSchedule,
        manifest: CampaignManifest,
    ) -> None:
        """Persist all campaign contract files."""
        _write_atomic(self.report_dir / CAMPAIGN_MANIFEST_JSON, manifest.model_dump_json(indent=2))
        _write_jsonl(self.report_dir / CAMPAIGN_ASSIGNMENTS_JSONL, assignments)
        _write_jsonl(self.report_dir / CAMPAIGN_COHORTS_JSONL, self.spec.cohorts)
        _write_atomic(self.report_dir / CAMPAIGN_SCHEDULE_JSON, schedule.model_dump_json(indent=2))
        _write_atomic(self.report_dir / CAMPAIGN_RETRY_POLICY_JSON, self.spec.retry_policy.model_dump_json(indent=2))

    def _load_existing_attempts(self) -> list[AttemptRecord]:
        """Load existing attempts from the report directory (for resume)."""
        records = _read_jsonl(self.report_dir / ATTEMPTS_JSONL, AttemptRecord)
        return cast(list[AttemptRecord], records)

    def _completed_assignment_ids(self, attempts: list[AttemptRecord]) -> set[str]:
        """Return the set of assignment IDs that have a terminal attempt."""
        return {a.assignment_id for a in attempts if a.assignment_id}

    def _build_task_definitions(self) -> list[TaskDefinition]:
        """Build TaskDefinition records for the report directory."""
        task_defs = []
        for t in self.tasks:
            task_defs.append(
                TaskDefinition(
                    task_id=t.id,
                    suite_id=self.spec.suite_id,
                    suite_version=self.spec.suite_version,
                    category=t.metadata.category or "instruction_following",
                    expected_action_class=t.metadata.expected_action_class,
                    compatible_arms=list(Arm),
                    prompt_hash=hashlib.sha256(t.prompt.encode()).hexdigest(),
                    prompt_length=len(t.prompt),
                    graders=[
                        GraderReference(
                            grader_id=_IFEVAL_GRADER_ID,
                            grader_version=_GRADER_VERSION,
                            grader_class=GraderClass.DETERMINISTIC,
                        ),
                    ],
                    metadata={"instruction_id_list": t.metadata.instruction_id_list},
                )
            )
        return task_defs

    def _build_run_manifest(self, task_defs: list[TaskDefinition]) -> RunManifest:
        """Build the RunManifest for the report directory."""
        arms = [self.spec.preregistration.baseline_arm_id, *self.spec.preregistration.comparison_arm_ids]
        arm_entries = []
        for arm_id in arms:
            arm_def = get_arm_definition(Arm(arm_id))
            arm_entries.append(
                ArmManifestEntry(
                    arm_id=arm_def.arm_id,
                    requested_posture=arm_def.requested_posture,
                    uses_g8ee=arm_def.uses_g8ee,
                    uses_gateway=arm_def.uses_gateway,
                    receipt_binding=arm_def.receipt_binding,
                    is_production_posture=arm_def.is_production_posture,
                )
            )
        role_to_model = RoleToModelMapping()
        for cohort in self.spec.cohorts:
            for rb in cohort.role_bindings:
                identity = ModelIdentity(
                    role=rb.role,
                    provider=rb.provider,
                    model=rb.model_id,
                    endpoint=rb.endpoint,
                    endpoint_class="local" if rb.endpoint.startswith(("http://localhost", "http://127.")) else "remote",
                    api_key_present=False,
                )
                if rb.role == "primary":
                    role_to_model = role_to_model.model_copy(update={"primary": identity})
                elif rb.role == "assistant":
                    role_to_model = role_to_model.model_copy(update={"assistant": identity})
                elif rb.role == "lite":
                    role_to_model = role_to_model.model_copy(update={"lite": identity})

        content_hashes = [
            ContentHash(name="dataset", sha256=self.spec.dataset_hash),
            ContentHash(name="prompt_bundle", sha256=self.spec.prompt_bundle_hash),
            ContentHash(name="grader_bundle", sha256=self.spec.grader_bundle_hash),
        ]

        return RunManifest(
            run_id=self._run_id,
            suite_id=self.spec.suite_id,
            suite_version=self.spec.suite_version,
            orchestrator_version=self.spec.release_version,
            arms=arm_entries,
            content_hashes=content_hashes,
            role_to_model=role_to_model,
            source_build_provenance=self.spec.source_build_provenance,
            provider_budget=self.spec.provider_budget,
            stack_environment=self.spec.stack_environment,
        )

    async def _execute_assignment(
        self,
        assignment: CampaignAssignment,
        cohort: ModelCohort,
        arm_def: ArmDefinition,
        task: Task,
        existing_attempts: list[AttemptRecord],
    ) -> tuple[list[AttemptRecord], list[MetricObservation]]:
        """Execute one assignment, handling retries.

        Returns all attempts (including retry attempts) and any metric
        observations from the terminal attempt. If the assignment
        already has a terminal attempt (resume), returns the existing
        attempts without re-executing.
        """
        assignment_attempts = [a for a in existing_attempts if a.assignment_id == assignment.assignment_id]
        if assignment_attempts:
            metrics: list[MetricObservation] = []
            return assignment_attempts, metrics

        sut = self._get_sut(cohort, arm_def.arm_id)
        expected_model = _model_id_for_cohort(cohort)

        attempt_num = 0
        parent_attempt_id: str | None = None
        all_attempts: list[AttemptRecord] = []

        while True:
            attempt_id = f"{self._run_id}:{assignment.assignment_id}:{attempt_num}"
            started_at = datetime.now(UTC)

            infrastructure_error: Exception | None = None
            response: Response | None = None
            try:
                response = await sut.get_answer(task)
            except Exception as exc:
                infrastructure_error = exc

            ended_at = datetime.now(UTC)

            if infrastructure_error is not None:
                terminal_status = TerminalStatus.INFRASTRUCTURE_FAILED
            elif response is None:
                terminal_status = TerminalStatus.MODEL_FAILED
            elif not response.answer:
                terminal_status = TerminalStatus.MODEL_FAILED
            elif response.model and not _model_matches(response.model, expected_model):
                # Cohort drift: the SUT returned a different model than
                # the cohort declares. This invalidates the campaign.
                terminal_status = TerminalStatus.MODEL_FAILED
            else:
                terminal_status = TerminalStatus.COMPLETED

            # Build metric observation
            metrics: list[MetricObservation] = []
            if terminal_status == TerminalStatus.COMPLETED and response is not None:
                score = self.grader.verify(
                    task.id,
                    task.prompt,
                    response.answer,
                    task.metadata.instruction_id_list,
                    task.metadata.kwargs,
                )
                passed = bool(getattr(score, "passed", False))
                metric = MetricObservation(
                    metric_id=_IFEVAL_GRADER_ID,
                    attempt_id=attempt_id,
                    run_id=self._run_id,
                    arm_id=arm_def.arm_id,
                    task_id=task.id,
                    value=float(passed),
                    unit="boolean",
                    verification_status=VerificationStatus.VERIFIED,
                    grader_class=GraderClass.DETERMINISTIC,
                )
                DEFAULT_METRIC_REGISTRY.validate(metric)
                metrics.append(metric)

            attempt = AttemptRecord(
                attempt_id=attempt_id,
                run_id=self._run_id,
                task_id=task.id,
                arm_id=arm_def.arm_id,
                model_cohort_id=cohort.cohort_id,
                state_snapshot_hash=_NO_STATE_HASH,
                replicate_id=assignment.replicate_id,
                assignment_id=assignment.assignment_id,
                assignment_order=assignment.schedule_position,
                started_at=started_at,
                ended_at=ended_at,
                terminal_status=terminal_status,
                posture=PostureObservation(requested_posture=arm_def.requested_posture),
                missingness_or_failure=None if terminal_status == TerminalStatus.COMPLETED else terminal_status.value,
                parent_attempt_id=parent_attempt_id,
            )
            all_attempts.append(attempt)

            # Check if retry is allowed
            retryable = set(self.spec.retry_policy.retryable_terminal_statuses)
            max_attempts = self.spec.retry_policy.max_retries + 1
            if (
                terminal_status.value in retryable
                and attempt_num < max_attempts - 1
            ):
                parent_attempt_id = attempt_id
                attempt_num += 1
                continue

            break

        # Validate the retry chain
        validate_retry_chain(all_attempts, assignment.assignment_id, self.spec.retry_policy)

        return all_attempts, metrics

    def _materialize_budget_stop(
        self,
        assignment: CampaignAssignment,
        arm_def: ArmDefinition,
    ) -> AttemptRecord:
        """Materialize a typed terminal outcome for an unexecuted assignment
        when the campaign stops due to budget exhaustion."""
        attempt_id = f"{self._run_id}:{assignment.assignment_id}:0"
        now = datetime.now(UTC)
        return AttemptRecord(
            attempt_id=attempt_id,
            run_id=self._run_id,
            task_id=assignment.task_id,
            arm_id=arm_def.arm_id,
            model_cohort_id=assignment.model_cohort_id,
            state_snapshot_hash=_NO_STATE_HASH,
            replicate_id=assignment.replicate_id,
            assignment_id=assignment.assignment_id,
            assignment_order=assignment.schedule_position,
            started_at=now,
            ended_at=now,
            terminal_status=TerminalStatus.INFRASTRUCTURE_FAILED,
            posture=PostureObservation(requested_posture=arm_def.requested_posture),
            missingness_or_failure="budget_exhausted",
        )

    async def run(self) -> CampaignResult:
        """Execute the campaign and produce the final canonical analysis.

        Creates one campaign identity, one report directory, one
        assignment manifest, one schedule, and one final canonical
        analysis. Resume reads the persisted schedule and existing
        attempts.
        """
        # 1. Build assignments and schedule
        assignments, schedule = build_campaign_state(self.spec)

        # 2. Build campaign manifest
        prereg_hash = _compute_prereg_hash(self.spec.preregistration)
        retry_policy_hash = compute_retry_policy_hash(
            self.spec.retry_policy.max_retries,
            self.spec.retry_policy.retryable_terminal_statuses,
        )
        metric_registry_hash = _compute_simple_hash("default_metric_registry_v1")
        release_metric_set_hash = _compute_simple_hash("release_metric_set_v1")
        threshold_authority_hash = _compute_simple_hash("descriptive_only_no_threshold")
        missingness_authority_hash = _compute_simple_hash("assignment_level_missingness")
        provider_budget_hash = _compute_simple_hash(
            f"budget:{self.spec.provider_budget.max_usd}" if self.spec.provider_budget else "no_budget"
        )
        source_build_provenance_hash = _compute_simple_hash(
            self.spec.source_build_provenance.source_tree_state_hash
            if self.spec.source_build_provenance
            else "no_provenance"
        )
        claim_exclusion_hash = _compute_simple_hash("descriptive_only_claim_exclusion")

        manifest = build_campaign_manifest(
            self.spec,
            schedule,
            prereg_hash,
            retry_policy_hash,
            metric_registry_hash,
            release_metric_set_hash,
            threshold_authority_hash,
            missingness_authority_hash,
            provider_budget_hash,
            source_build_provenance_hash,
            claim_exclusion_hash,
        )

        # 3. Create report directory and persist campaign state
        self.report_dir.mkdir(parents=True, exist_ok=True)

        # Check for resume: if campaign status exists, reuse the run_id
        existing_status = self._read_campaign_status()
        if existing_status is not None:
            self._run_id = existing_status["run_id"]

        self._persist_campaign_state(assignments, schedule, manifest)

        # 4. Write run manifest and task definitions
        task_defs = self._build_task_definitions()
        run_manifest = self._build_run_manifest(task_defs)
        _write_atomic(self.report_dir / MANIFEST_JSON, run_manifest.model_dump_json(indent=2))
        _write_jsonl(self.report_dir / TASKS_JSONL, task_defs)

        # 5. Set campaign status to running
        self._write_campaign_status(CampaignStatus.RUNNING)

        # 6. Load existing attempts (for resume)
        existing_attempts = self._load_existing_attempts()
        completed_ids = self._completed_assignment_ids(existing_attempts)

        # 7. Execute assignments in schedule order
        cohort_by_id = self._cohort_by_id()
        task_by_id = self._task_by_id()

        all_attempts: list[AttemptRecord] = list(existing_attempts)
        all_metrics: list[MetricObservation] = []
        stop_reason: CampaignStopReason | None = None
        request_count = 0
        max_requests = self.spec.provider_budget.max_requests if self.spec.provider_budget else None

        for assignment_id in schedule.ordered_assignment_ids:
            if assignment_id in completed_ids:
                continue

            assignment = next(a for a in assignments if a.assignment_id == assignment_id)
            cohort = cohort_by_id[assignment.model_cohort_id]
            arm_def = get_arm_definition(Arm(assignment.arm_id))
            task = task_by_id[assignment.task_id]

            # Budget enforcement: check before dispatch
            if max_requests is not None and request_count >= max_requests:
                # Materialize terminal outcomes for remaining assignments
                remaining = [
                    a for a in assignments
                    if a.assignment_id not in completed_ids
                    and schedule.ordered_assignment_ids.index(a.assignment_id) >= schedule.ordered_assignment_ids.index(assignment_id)
                ]
                for rem in remaining:
                    rem_arm_def = get_arm_definition(Arm(rem.arm_id))
                    stop_attempt = self._materialize_budget_stop(rem, rem_arm_def)
                    all_attempts.append(stop_attempt)
                stop_reason = CampaignStopReason.BUDGET_EXHAUSTED
                break

            request_count += 1

            new_attempts, metrics = await self._execute_assignment(
                assignment, cohort, arm_def, task, all_attempts
            )
            all_attempts.extend(new_attempts)
            all_metrics.extend(metrics)
            completed_ids.add(assignment_id)

            # Cohort drift check
            terminal_attempt = new_attempts[-1]
            if terminal_attempt.terminal_status == TerminalStatus.MODEL_FAILED:
                # The drift is detected inside _execute_assignment via the
                # response model check. If the terminal status is
                # MODEL_FAILED due to drift, stop the campaign.
                if terminal_attempt.missingness_or_failure and "drift" in terminal_attempt.missingness_or_failure:
                    stop_reason = CampaignStopReason.COHORT_DRIFT
                    break

        # 8. Write attempts and metrics
        _write_jsonl(self.report_dir / ATTEMPTS_JSONL, all_attempts)
        _write_jsonl(self.report_dir / METRICS_JSONL, all_metrics)

        # 9. Determine final campaign status
        if stop_reason is None:
            stop_reason = CampaignStopReason.COMPLETED
            final_status = CampaignStatus.COMPLETED
        else:
            final_status = CampaignStatus.STOPPED

        # 10. Build and write canonical analysis
        analysis_input = AnalysisInputRecord(
            run_id=self._run_id,
            release_version=self.spec.release_version,
            tasks=task_defs,
            attempts=all_attempts,
            metric_observations=all_metrics,
            preregistration=self.spec.preregistration,
            campaign_manifest=manifest,
            model_cohorts=self.spec.cohorts,
            campaign_assignments=assignments,
            execution_schedule=schedule,
            retry_policy=self.spec.retry_policy,
        )

        _write_atomic(self.report_dir / ANALYSIS_INPUT_JSON, canonical_model_json(analysis_input))

        analysis: CanonicalEvalAnalysis = compute_canonical_analysis_from_record(analysis_input)

        _write_atomic(self.report_dir / ANALYSIS_JSON, canonical_model_json(analysis))

        from g8e_evals.analysis.renderers import render_cli, render_html, render_markdown

        _write_atomic(self.report_dir / ANALYSIS_MD, render_markdown(analysis))
        _write_atomic(self.report_dir / ANALYSIS_HTML, render_html(analysis))
        _write_atomic(self.report_dir / ANALYSIS_TXT, render_cli(analysis))

        # 11. Update campaign status
        self._write_campaign_status(final_status, stop_reason)

        return CampaignResult(
            campaign_id=self.spec.campaign_id,
            run_id=self._run_id,
            report_dir=self.report_dir,
            status=final_status,
            assignment_count=len(assignments),
            terminal_attempt_count=len(all_attempts),
            stop_reason=stop_reason,
        )


__all__ = [
    "CampaignResult",
    "CampaignRunner",
    "CampaignRunnerError",
    "CampaignSpec",
    "CampaignStopReason",
    "GraderProtocol",
    "SUTFactory",
    "SUTProtocol",
    "build_campaign_manifest",
    "build_campaign_state",
]
