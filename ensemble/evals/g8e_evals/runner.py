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

Each ``run`` starts a fresh report directory; the runner does not reload a
prior campaign. Explicit batching via ``--task-offset`` and ``--task-limit``
replaces the historical resume behavior. Assignment-scoped failures
(model, governance, timeout, invalid evidence) are persisted as typed
terminal outcomes and the campaign continues unless the stop policy fires.
Global preflight defects, corrupted persisted state, unsafe path handling,
or an inability to persist authoritative records raise immediately.

Files are written atomically through a staging directory that is renamed
to the final report directory only after the campaign manifest, schedule,
and status are complete. A typed campaign status distinguishes planned,
running, stopped, completed, failed-integrity, and finalized.
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import platform
import random
import shutil
import sys
import time
import uuid
from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import StrEnum
from pathlib import Path
from collections.abc import Sequence
from typing import TYPE_CHECKING, Protocol

from pydantic import BaseModel, ConfigDict, Field

if TYPE_CHECKING:
    from g8e_evals.campaign_set import CampaignSetPlan

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
    RoleModelBinding,
    SamplingSettings,
    TaskAssignmentManifest,
    compute_assignment_id,
    compute_campaign_manifest_hash,
    compute_model_cohort_hash,
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
    CAMPAIGN_INDEX_JSONL,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_PROGRESS_JSON,
    CAMPAIGN_RETRY_POLICY_JSON,
    CAMPAIGN_SCHEDULE_JSON,
    CAMPAIGN_STATUS_JSON,
    CORRELATED_ERRORS_JSONL,
    EVIDENCE_INDEX_JSONL,
    ESCALATION_RECORDS_JSONL,
    MANIFEST_JSON,
    METRICS_JSONL,
    REPORT_CHECKSUM_JSON,
    RESOURCE_OBSERVATIONS_JSONL,
    SECURITY_EVENTS_JSONL,
    STAGES_JSONL,
    TASKS_JSONL,
    TOOL_CALL_SCORECARDS_JSONL,
)
from g8e_evals.harness import InferenceObservation, Response, Score, SUTConfig, Task
from g8e_evals.index import (
    AssignmentDisposition,
    AssignmentDispositionEntry,
    IndexCreationReason,
    IndexGeneration,
    MeasurementAvailability,
    MeasurementScope,
    ModelRole,
    ResourceObservation,
    UnavailableMeasurement,
    VerificationStatus as IndexVerificationStatus,
    compute_index_generation_hash,
)
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.profile import CampaignProfile
from g8e_evals.registry import ModelRegistry
from g8e_evals.report.validate import validate_standalone_report
from g8e_evals.schema import (
    ArmManifestEntry,
    AttemptRecord,
    CampaignBinding,
    ContentHash,
    CorrelatedErrorRecord,
    EscalationRecord,
    EvidenceIndex,
    EvidenceMediaType,
    GraderClass,
    GraderReference,
    MetricObservation,
    ModelIdentity,
    PostureObservation,
    PrivacyClassification,
    ProviderBudget,
    ReportRole,
    RoleToModelMapping,
    RunManifest,
    SecurityEventRecord,
    SourceBuildProvenance,
    StackEnvironment,
    StageKind,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    ToolCallScorecard,
    VerificationStatus,
)

logger = logging.getLogger(__name__)

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


class BudgetExhausted(Exception):
    """Raised inside _execute_assignment when a budget ceiling is reached.

    The main loop catches this to materialize budget-stop outcomes for
    remaining assignments.
    """


class BudgetExhaustedDuringExecution(BudgetExhausted):
    """Raised when a usage budget ceiling is reached after a provider call
    succeeds but before ``_execute_assignment`` returns.

    Carries the current attempt's materialized records (attempts, metrics,
    resource observations, stages, evidence) so the caller can persist them
    before propagating the stop. The current assignment's provider call
    completed, so its attempt is ``COMPLETED``; the budget stop applies to
    remaining assignments only.
    """

    def __init__(
        self,
        message: str,
        *,
        attempts: list[AttemptRecord],
        metrics: list[MetricObservation],
        resource_observations: list[ResourceObservation],
        stages: list[StageObservation],
        evidence: list[EvidenceIndex],
    ) -> None:
        super().__init__(message)
        self.attempts = attempts
        self.metrics = metrics
        self.resource_observations = resource_observations
        self.stages = stages
        self.evidence = evidence


class CleanupError(Exception):
    """Raised when one or more SUTs fail to close during cleanup.

    The primary exception (if any) takes precedence over cleanup errors.
    When no primary exception is in flight, cleanup errors propagate so
    resource leaks are never silently swallowed.
    """


class BudgetCeiling(StrEnum):
    """Named budget ceilings for the observability policy."""

    MAX_REQUESTS = "max_requests"
    MAX_TOKENS = "max_tokens"
    MAX_USD = "max_usd"


class BudgetObservabilityPolicy(BaseModel):
    """Frozen policy declaring which budget ceilings are observable and enforceable.

    The request ceiling (``max_requests``) is always observable because the
    runner counts every provider call internally. Token and USD ceilings are
    observable only when the SUT reports usage (tokens) or a pricing authority
    supplies per-model cost (USD). A ceiling declared in ``ProviderBudget``
    that is not observable and not explicitly excluded via
    ``excluded_ceilings`` fails preflight — the campaign authority refuses to
    start a run that declares a ceiling it cannot enforce.

    The default policy excludes ``MAX_TOKENS`` and ``MAX_USD`` because no
    pricing authority is wired yet and g8ee arms do not report token usage.
    A campaign that wants to enforce token budgets sets
    ``tokens_observable=True`` and removes ``MAX_TOKENS`` from
    ``excluded_ceilings``; the SUT must then report usage for every
    inference or the runner raises at runtime.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    tokens_observable: bool = Field(
        default=False,
        description="Whether token usage is observable from SUT responses.",
    )
    usd_observable: bool = Field(
        default=False,
        description="Whether USD usage is observable from SUT responses or a pricing authority.",
    )
    excluded_ceilings: frozenset[BudgetCeiling] = Field(
        default=frozenset({BudgetCeiling.MAX_TOKENS, BudgetCeiling.MAX_USD}),
        description="Ceilings explicitly excluded from enforcement even when declared in the budget.",
    )

    @property
    def requests_observable(self) -> bool:
        """Requests are always observable; the runner counts every call."""
        return True

    def is_ceiling_enforced(self, ceiling: BudgetCeiling) -> bool:
        """Return True when the ceiling is observable and not excluded."""
        if ceiling in self.excluded_ceilings:
            return False
        if ceiling == BudgetCeiling.MAX_REQUESTS:
            return self.requests_observable
        if ceiling == BudgetCeiling.MAX_TOKENS:
            return self.tokens_observable
        if ceiling == BudgetCeiling.MAX_USD:
            return self.usd_observable
        return False

    def validate_budget(self, budget: ProviderBudget | None) -> None:
        """Reject a budget whose declared ceiling is unobservable and not excluded.

        Raises ``CampaignRunnerError`` when a non-None ceiling is not
        observable and not in ``excluded_ceilings``. ``max_usd`` is always
        present on ``ProviderBudget`` (required field); it passes only when
        ``usd_observable`` is True or ``MAX_USD`` is excluded.
        """
        if budget is None:
            return
        if budget.max_requests is not None and not self.requests_observable:
            if BudgetCeiling.MAX_REQUESTS not in self.excluded_ceilings:
                raise CampaignRunnerError(
                    "max_requests ceiling is declared but not observable and not excluded"
                )
        if budget.max_tokens is not None and not self.tokens_observable:
            if BudgetCeiling.MAX_TOKENS not in self.excluded_ceilings:
                raise CampaignRunnerError(
                    "max_tokens ceiling is declared but not observable and not excluded; "
                    "set tokens_observable=True or add MAX_TOKENS to excluded_ceilings"
                )
        if not self.usd_observable:
            if BudgetCeiling.MAX_USD not in self.excluded_ceilings:
                raise CampaignRunnerError(
                    "max_usd ceiling is declared but not observable and not excluded; "
                    "set usd_observable=True or add MAX_USD to excluded_ceilings"
                )


DEFAULT_BUDGET_OBSERVABILITY_POLICY = BudgetObservabilityPolicy()


def compute_budget_observability_policy_hash(policy: BudgetObservabilityPolicy) -> str:
    """Compute a content-addressed hash over the budget observability policy."""
    return _compute_simple_hash(
        f"budget_observability:"
        f"tokens_observable={policy.tokens_observable}:"
        f"usd_observable={policy.usd_observable}:"
        f"excluded={sorted(c.value for c in policy.excluded_ceilings)}"
    )


class BudgetTracker:
    """Tracks provider request, token, and USD usage against budget ceilings.

    Every provider call (including retries) counts against ``max_requests``.
    Observed token and USD usage from SUT responses counts against
    ``max_tokens`` and ``max_usd`` when those ceilings are declared,
    observable per the ``BudgetObservabilityPolicy``, and not excluded. A
    ceiling that is declared but not observable or explicitly excluded is
    not enforced from observed usage; the request ceiling remains the
    primary enforcement mechanism for unobservable ceilings.
    """

    def __init__(
        self,
        budget: ProviderBudget | None,
        policy: BudgetObservabilityPolicy | None = None,
    ) -> None:
        self.budget = budget
        self.policy = policy or DEFAULT_BUDGET_OBSERVABILITY_POLICY
        self.request_count = 0
        self.total_tokens = 0
        self.total_usd = 0.0

    @property
    def max_requests(self) -> int | None:
        return self.budget.max_requests if self.budget else None

    @property
    def max_tokens(self) -> int | None:
        return self.budget.max_tokens if self.budget else None

    @property
    def max_usd(self) -> float | None:
        return self.budget.max_usd if self.budget else None

    def check_request_budget(self) -> None:
        """Raise BudgetExhausted if the request ceiling is reached."""
        if self.max_requests is not None and self.request_count >= self.max_requests:
            raise BudgetExhausted("request ceiling reached")

    def record_provider_call(self) -> None:
        """Record one provider call (including retries) against the request budget."""
        self.request_count += 1

    def record_usage(self, tokens: int | None, usd: float | None) -> None:
        """Record observed token and USD usage from a SUT response.

        Only records non-None values; unobserved usage does not count
        against the ceiling.
        """
        if tokens is not None and tokens > 0:
            self.total_tokens += tokens
        if usd is not None and usd > 0:
            self.total_usd += usd

    def check_usage_budgets(self) -> None:
        """Raise BudgetExhausted if an observable, non-excluded ceiling is reached."""
        if (
            self.max_tokens is not None
            and self.policy.is_ceiling_enforced(BudgetCeiling.MAX_TOKENS)
            and self.total_tokens >= self.max_tokens
        ):
            raise BudgetExhausted(f"token ceiling reached: {self.total_tokens} >= {self.max_tokens}")
        if (
            self.max_usd is not None
            and self.policy.is_ceiling_enforced(BudgetCeiling.MAX_USD)
            and self.total_usd >= self.max_usd
        ):
            raise BudgetExhausted(f"usd ceiling reached: {self.total_usd} >= {self.max_usd}")


def _extract_usage_from_response(response: Response) -> tuple[int | None, float | None]:
    """Extract observed token and USD usage from a SUT response.

    Reads from ``inference_observations`` first (the typed SUT response
    boundary carrying ``InferenceObservation`` records with
    ``total_token_count`` and ``usage_reported``), then falls back to
    ``chat_evidence.model_dump()`` (the ``DirectCallEvidence`` schema)
    for legacy SUTs that do not emit ``InferenceObservation`` records.
    Returns ``(None, None)`` when the response carries no usage, the
    evidence does not report usage, or ``usage_reported`` is False. USD
    is not available from SUT responses without a pricing authority; the
    USD component is always None until a pricing authority is wired.
    """
    # Primary path: typed InferenceObservation records on the SUT
    # response boundary. Sum token counts across all observations.
    if response.inference_observations:
        total_tokens = 0
        any_reported = False
        for obs in response.inference_observations:
            if obs.usage_reported and obs.total_token_count is not None:
                total_tokens += obs.total_token_count
                any_reported = True
        if any_reported and total_tokens > 0:
            return total_tokens, None

    # Fallback: legacy chat_evidence path
    if response.chat_evidence is None:
        return None, None
    dump = response.chat_evidence.model_dump()
    if not dump.get("usage_reported", False):
        return None, None
    tokens = dump.get("total_token_count")
    if isinstance(tokens, int) and tokens > 0:
        return tokens, None
    return None, None


class CampaignRunnerError(Exception):
    """Raised for global preflight defects, corrupted persisted state, or
    an inability to persist authoritative records."""


class DiskSpacePreflightError(Exception):
    """Raised when the available disk space is below the required threshold."""


def check_disk_space(path: Path, *, min_bytes: int) -> None:
    """Check that the filesystem containing ``path`` has at least ``min_bytes`` free.

    Raises ``DiskSpacePreflightError`` when the available space is below
    the threshold. A zero threshold always passes.
    """
    if min_bytes <= 0:
        return
    usage = shutil.disk_usage(path)
    if usage.free < min_bytes:
        raise DiskSpacePreflightError(
            f"insufficient disk space at {path}: available {usage.free} bytes, "
            f"required {min_bytes} bytes"
        )


class SUTProtocol(Protocol):
    """Minimal SUT interface the runner consumes."""

    async def get_answer(self, task: Task) -> Response: ...


class SUTFactory(Protocol):
    """Factory that creates a SUT for a (cohort, arm) pair."""

    def __call__(self, cohort: ModelCohort, arm: Arm) -> SUTProtocol: ...


class GraderProtocol(Protocol):
    """Minimal grader interface the runner consumes.

    The runner calls ``grade(task, response)`` for every completed
    assignment. Each grader extracts the suite-specific metadata it needs
    from the ``Task`` (prompt, metadata) and the ``Response`` (answer,
    chat evidence) and returns a ``Score``. Suite-specific entry points
    such as the IFEval ``verify`` method remain available for direct
    unit testing of instruction-checking logic.

    The ``grader_id`` and ``grader_version`` attributes identify the
    grader in metric observations and task definitions, replacing the
    previous hardcoded IFEval-only constants.
    """

    grader_id: str
    grader_version: str

    def grade(self, task: Task, response: Response) -> Score: ...


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
    budget_observability_policy: BudgetObservabilityPolicy = Field(
        default_factory=BudgetObservabilityPolicy,
    )
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
    budget_observability_policy_hash: str,
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
        budget_observability_policy_hash,
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
        budget_observability_policy_hash=budget_observability_policy_hash,
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


def _append_jsonl(path: Path, records: Sequence[BaseModel]) -> None:
    """Append records to a JSONL file (incremental persistence)."""
    if not records:
        return
    with open(path, "a", encoding="utf-8") as f:
        for r in records:
            f.write(r.model_dump_json() + "\n")


def _read_jsonl(path: Path, model_cls: type[BaseModel]) -> list[BaseModel]:
    """Read a JSONL file into a list of Pydantic models.

    Handles trailing null-byte padding that can result from a process
    kill mid-write (the OS may have allocated a full block that was
    only partially filled with valid JSON lines).
    """
    if not path.exists():
        return []
    raw = path.read_bytes()
    # Strip trailing null-byte padding from process-kill corruption
    raw = raw.rstrip(b"\x00")
    records: list[BaseModel] = []
    for line in raw.decode("utf-8", errors="replace").splitlines():
        line = line.strip()
        if not line:
            continue
        records.append(model_cls.model_validate_json(line))
    return records


def _compute_prereg_hash(prereg: PreregistrationConfig) -> str:
    return hashlib.sha256(canonical_model_json(prereg).encode()).hexdigest()


def _compute_simple_hash(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


def compute_provider_budget_hash(budget: ProviderBudget | None) -> str:
    """Compute a content-addressed hash over the provider budget.

    Covers all three ceilings: ``max_usd``, ``max_tokens``, and
    ``max_requests``. ``None`` (no budget) and a budget with zero
    ceilings produce distinct hashes. ``None`` ceilings are distinct
    from zero ceilings.
    """
    if budget is None:
        return _compute_simple_hash("no_budget")
    max_tokens_str = str(budget.max_tokens) if budget.max_tokens is not None else "none"
    max_requests_str = str(budget.max_requests) if budget.max_requests is not None else "none"
    return _compute_simple_hash(
        f"budget:max_usd={budget.max_usd}:max_tokens={max_tokens_str}:max_requests={max_requests_str}"
    )


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


_RUNNER_COLLECTION_TOOL = "g8e_evals-runner-1.0.0"

# Canonical machine-token vocabulary is GOARCH-style (amd64, arm64), the
# same vocabulary the frozen campaign profiles declare in
# ``hardware_identity``. ``platform.machine()`` returns ``x86_64`` on
# Linux and ``AMD64`` on Windows; both canonicalize to ``amd64``.
_MACHINE_TOKEN_CANONICALIZATION = {
    "x86_64": "amd64",
    "amd64": "amd64",
    "aarch64": "arm64",
    "arm64": "arm64",
}


def _canonical_machine_token(machine: str) -> str:
    """Canonicalize a ``platform.machine()`` token to GOARCH vocabulary."""
    return _MACHINE_TOKEN_CANONICALIZATION.get(machine.lower(), machine.lower())


_ORCHESTRATOR_SCOPE = (
    f"{platform.system().lower()}/{_canonical_machine_token(platform.machine())}/cpu"
)


def _observed_orchestrator_scope_prefix() -> str:
    """Return the observed ``system/machine`` prefix of the orchestrator scope."""
    return "/".join(_ORCHESTRATOR_SCOPE.split("/")[:2])


def _validate_orchestrator_scope(hardware_identity: str) -> None:
    """Reject a declared orchestrator hardware identity that the host contradicts.

    The frozen profile's ``hardware_identity`` is the declared authority
    (e.g. ``linux/amd64/rtx-4090``). The observed orchestrator scope is
    canonicalized to the same ``system/machine[/accelerator]``
    vocabulary; the accelerator suffix is not observable via
    ``platform``, so only the ``system/machine`` prefix is compared. A
    prefix mismatch means the run is executing on a different
    orchestrator environment than the frozen authority declares, which
    invalidates the evidence identity. Raises ``CampaignRunnerError``
    before the report directory is created.
    """
    declared_parts = hardware_identity.split("/")
    if len(declared_parts) < 2:
        raise CampaignRunnerError(
            f"declared orchestrator hardware identity {hardware_identity!r} "
            "has no system/machine prefix to validate against"
        )
    declared_prefix = "/".join(declared_parts[:2])
    observed_prefix = _observed_orchestrator_scope_prefix()
    if observed_prefix != declared_prefix:
        raise CampaignRunnerError(
            f"orchestrator scope mismatch: observed {observed_prefix!r} "
            f"does not match declared hardware identity prefix {declared_prefix!r}"
        )


def _validate_arm_coherence(
    profile: CampaignProfile,
    preregistration: PreregistrationConfig,
) -> None:
    """Reject a preregistration whose arms the bound profile does not declare.

    The runner derives assignment arms solely from the preregistration
    (``baseline_arm_id`` + ``comparison_arm_ids``), while the bound
    profile's ``track_arm_assignments`` declares which arms the campaign
    is authorized to execute. Every preregistration arm must appear in
    the profile's declared arm IDs — a subset check, so a partially
    unauthorized arm set cannot slip through. Raises
    ``CampaignRunnerError`` before the report directory is created.
    """
    declared_arms = {a.arm_id for a in profile.track_arm_assignments}
    prereg_arms = {
        preregistration.baseline_arm_id,
        *preregistration.comparison_arm_ids,
    }
    unauthorized = sorted(prereg_arms - declared_arms)
    if unauthorized:
        raise CampaignRunnerError(
            f"preregistration declares arms {unauthorized} not present in the "
            f"profile's track_arm_assignments {sorted(declared_arms)}; "
            "a profile-bound campaign may only execute declared arms"
        )


def _unavailable(
    field_name: str,
    availability: MeasurementAvailability,
    reason: str,
    scope: MeasurementScope = MeasurementScope.PROVIDER_REMOTE,
) -> UnavailableMeasurement:
    """Build a typed unavailable-measurement entry."""
    return UnavailableMeasurement(
        field_name=field_name,
        availability=availability,
        scope=scope,
        reason=reason,
    )


def _build_unavailable_measurements(obs: InferenceObservation) -> list[UnavailableMeasurement]:
    """Build typed unavailable-measurement entries for every None field.

    GPU metrics are UNAVAILABLE at the provider-remote scope (no API to
    read them from the remote Ollama server). Cold-start load time,
    resident memory, accelerator memory, artifact bytes, energy, hidden
    reasoning throughput, and accelerator-before baseline are
    NOT_APPLICABLE for the direct arm (no local model load, no local
    process memory observation, no energy meter, no hidden reasoning
    tokens reported).
    """
    entries: list[UnavailableMeasurement] = []

    # GPU metrics: unavailable from the remote provider host
    gpu_fields = (
        "gpu_utilization_percent",
        "gpu_temperature_celsius",
        "gpu_power_draw_watts",
        "gpu_clock_mhz",
    )
    for fname in gpu_fields:
        entries.append(_unavailable(
            fname,
            MeasurementAvailability.UNAVAILABLE,
            "remote GPU metrics not exposed by the provider API",
        ))

    # Cold-start and local-process metrics: not applicable for direct arm
    not_applicable_remote = (
        ("model_load_time_seconds", "direct arm has no local model load"),
        ("peak_resident_memory_bytes", "resident memory not observed for remote provider calls"),
        ("peak_accelerator_memory_bytes", "accelerator memory not observed for remote provider calls"),
        ("artifact_bytes", "artifact size not measured for direct provider calls"),
        ("measured_energy_joules", "energy not measured for direct provider calls"),
        ("accelerator_memory_before_bytes", "accelerator baseline not observed for remote provider calls"),
    )
    for fname, reason in not_applicable_remote:
        entries.append(_unavailable(
            fname,
            MeasurementAvailability.NOT_APPLICABLE,
            reason,
        ))

    # End-to-end latency: not applicable (we measure provider-call latency instead)
    entries.append(_unavailable(
        "end_to_end_latency_seconds",
        MeasurementAvailability.NOT_APPLICABLE,
        "end-to-end task latency not measured for direct provider calls; provider_call_latency_seconds is measured instead",
    ))

    # Conditional declarations for fields that are None on failed calls,
    # unreported usage, or boundaries that cannot observe them. Measured
    # values (including measured zero) never get an entry.
    conditional_fields: tuple[tuple[str, float | None, MeasurementScope, str], ...] = (
        (
            "provider_call_latency_seconds",
            obs.provider_call_latency_seconds,
            MeasurementScope.ORCHESTRATOR_LOCAL,
            "provider call latency not measured at the observation boundary",
        ),
        (
            "time_to_first_token_seconds",
            obs.time_to_first_token_seconds,
            MeasurementScope.ORCHESTRATOR_LOCAL,
            "time to first token not observed at the observation boundary",
        ),
        (
            "generation_duration_seconds",
            obs.generation_duration_seconds,
            MeasurementScope.PROVIDER_REMOTE,
            "generation duration not reported by the provider",
        ),
        (
            "output_throughput_tokens_per_second",
            obs.output_throughput_tokens_per_second,
            MeasurementScope.PROVIDER_REMOTE,
            "output throughput not measurable: token count or duration unavailable",
        ),
    )
    for fname, value, scope, reason in conditional_fields:
        if value is None:
            entries.append(_unavailable(
                fname,
                MeasurementAvailability.UNAVAILABLE,
                reason,
                scope=scope,
            ))

    # Hidden reasoning throughput: unavailable when not reported
    if obs.hidden_reasoning_throughput_tokens_per_second is None:
        entries.append(_unavailable(
            "hidden_reasoning_throughput_tokens_per_second",
            MeasurementAvailability.UNAVAILABLE,
            "hidden reasoning token count not reported by the provider",
        ))

    return entries


def _build_resource_observation(
    *,
    obs: InferenceObservation,
    campaign_id: str,
    child_id: str,
    run_id: str,
    assignment_id: str,
    attempt_id: str,
    task_id: str,
    stage_id: str,
    orchestrator_scope: str,
) -> ResourceObservation:
    """Build a typed ResourceObservation from an InferenceObservation.

    Only measurements available from the provider response are carried.
    Remote GPU metrics remain None with typed unavailable_measurements
    entries; they are never inferred from model metadata.

    ``orchestrator_scope`` is the declared hardware identity from the
    bound campaign profile when one exists (the frozen declaration is
    the authority), or the observed canonical scope otherwise.
    """
    unavailable = _build_unavailable_measurements(obs)
    return ResourceObservation(
        campaign_id=campaign_id,
        child_id=child_id,
        run_id=run_id,
        assignment_id=assignment_id,
        attempt_id=attempt_id,
        inference_id=obs.inference_id,
        stage_id=stage_id,
        role=ModelRole(obs.role),
        model_variant_id=obs.model_variant_id,
        task_id=task_id,
        orchestrator_scope=orchestrator_scope,
        provider_scope="unavailable",
        observation_boundary="provider_call",
        clock_domain="monotonic",
        collection_tool=_RUNNER_COLLECTION_TOOL,
        provider_call_latency_seconds=obs.provider_call_latency_seconds,
        time_to_first_token_seconds=obs.time_to_first_token_seconds,
        generation_duration_seconds=obs.generation_duration_seconds,
        output_throughput_tokens_per_second=obs.output_throughput_tokens_per_second,
        hidden_reasoning_throughput_tokens_per_second=obs.hidden_reasoning_throughput_tokens_per_second,
        unavailable_measurements=unavailable,
        verification_status=IndexVerificationStatus.PENDING,
    )


def _build_stage_observation(
    *,
    obs: InferenceObservation,
    stage_id: str,
    attempt_id: str,
    run_id: str,
    task_id: str,
) -> StageObservation:
    """Build a typed StageObservation for one provider inference."""
    return StageObservation(
        stage_id=stage_id,
        attempt_id=attempt_id,
        run_id=run_id,
        kind=StageKind.MODEL_INFERENCE,
        provider=obs.provider,
        model=obs.model,
        monotonic_start=obs.monotonic_start or None,
        monotonic_end=obs.monotonic_end or None,
        clock_domain="monotonic",
        timing_source="runner_sut_boundary",
        input_tokens=obs.prompt_token_count,
        output_tokens=obs.candidates_token_count,
        thinking_tokens=obs.thinking_token_count,
        cache_tokens=obs.cache_token_count,
        usage_reported=obs.usage_reported,
        finish_reason=obs.finish_reason,
        input_artifact_hash=obs.input_artifact_hash,
        output_artifact_hash=obs.output_artifact_hash,
        task_id=task_id,
    )


def _build_evidence_index_entries(
    *,
    obs: InferenceObservation,
    run_id: str,
    attempt_id: str,
) -> list[EvidenceIndex]:
    """Build EvidenceIndex entries for the input and output artifacts of one inference.

    Returns an empty list when no artifact hashes are present. Each
    artifact with a hash produces one EvidenceIndex entry with
    INTERNAL privacy classification (raw provider artifacts are not
    public).
    """
    entries: list[EvidenceIndex] = []
    if obs.input_artifact_hash:
        artifact_id = f"{attempt_id}:{obs.inference_id}:input"
        entries.append(EvidenceIndex(
            artifact_id=artifact_id,
            run_id=run_id,
            attempt_id=attempt_id,
            media_type=EvidenceMediaType.APPLICATION_JSON,
            schema_ref="g8e_evals.harness.InferenceObservation/input",
            sha256=obs.input_artifact_hash,
            producer_identity=_RUNNER_COLLECTION_TOOL,
            privacy_classification=PrivacyClassification.INTERNAL,
            storage_location="inline",
        ))
    if obs.output_artifact_hash:
        artifact_id = f"{attempt_id}:{obs.inference_id}:output"
        entries.append(EvidenceIndex(
            artifact_id=artifact_id,
            run_id=run_id,
            attempt_id=attempt_id,
            media_type=EvidenceMediaType.TEXT_PLAIN,
            schema_ref="g8e_evals.harness.InferenceObservation/output",
            sha256=obs.output_artifact_hash,
            producer_identity=_RUNNER_COLLECTION_TOOL,
            privacy_classification=PrivacyClassification.INTERNAL,
            storage_location="inline",
        ))
    return entries


def derive_cohorts_from_registry(
    profile: CampaignProfile,
    registry: ModelRegistry,
) -> tuple[list[ModelCohort], dict[str, str]]:
    """Derive campaign cohorts from the model registry.

    Builds one cohort per runnable variant in the profile's
    ``generative_variant_ids``. Each cohort's primary role binding
    carries the variant's served model tag and backend name, with
    sampling settings and timeout from the profile. The cohort ID is
    ``cohort-{variant_id}``.

    Returns the list of cohorts and a mapping from cohort ID to variant
    ID for SUT factory wiring.
    """
    runnable_ids = set(registry.runnable_variant_ids())
    measured_ids = [
        vid for vid in profile.generative_variant_ids
        if vid in runnable_ids
    ]

    cohorts: list[ModelCohort] = []
    cohort_variant_map: dict[str, str] = {}
    for variant_id in measured_ids:
        variant = registry.get_variant(variant_id)
        cohort_id = f"cohort-{variant_id}"
        bindings = [RoleModelBinding(
            role="primary",
            model_id=variant.served_model_tag,
            provider=variant.backend_name,
            endpoint="http://192.168.1.2:11434",
            sampling_settings=SamplingSettings(
                temperature=profile.temperature,
                top_p=profile.top_p,
                max_tokens=profile.max_tokens,
                seed=profile.seed,
            ),
            timeout_seconds=profile.timeout_seconds,
            seed_capable=True,
        )]
        ch = compute_model_cohort_hash(cohort_id, bindings)
        cohorts.append(ModelCohort(
            cohort_id=cohort_id,
            role_bindings=bindings,
            content_hash=ch,
        ))
        cohort_variant_map[cohort_id] = variant_id

    return cohorts, cohort_variant_map


_ALL_TIER_NAMES = ("primary", "assistant", "lite")


def build_tier_fitness_sut_config(
    cohort: ModelCohort,
    arm: Arm,
    campaign_profile: CampaignProfile,
    cohort_variant_map: dict[str, str],
    g8ee_url: str,
    operator_url: str | None = None,
    operator_session_id: str | None = None,
    auth_context: object | None = None,
) -> SUTConfig:
    """Build a ``SUTConfig`` for a tier-fitness assignment.

    The candidate model (from the cohort's primary role binding) replaces
    the target tier declared in the campaign profile's
    ``model_tier_assignments``. Non-target tiers are filled from
    ``baseline_tier_mappings``.

    Raises ``ValueError`` when the cohort's variant is not found in
    ``cohort_variant_map``, when no ``model_tier_assignment`` exists for
    the variant, or when a ``baseline_tier_mapping`` is missing for a
    non-target tier.
    """
    from g8e_evals.harness import LLMRoleConfig

    variant_id = cohort_variant_map.get(cohort.cohort_id)
    if variant_id is None:
        raise ValueError(
            f"cohort {cohort.cohort_id!r} not found in cohort_variant_map"
        )

    tier_assignment = None
    for assignment in campaign_profile.model_tier_assignments:
        if assignment.variant_id == variant_id:
            tier_assignment = assignment
            break
    if tier_assignment is None:
        raise ValueError(
            f"no model_tier_assignment found for variant {variant_id!r}"
        )

    target_tier = tier_assignment.target_tier
    candidate_model = cohort.role_bindings[0].model_id
    candidate_endpoint = cohort.role_bindings[0].endpoint
    candidate_provider = cohort.role_bindings[0].provider

    role_configs: dict[str, LLMRoleConfig] = {}
    for tier_name in _ALL_TIER_NAMES:
        if tier_name == target_tier:
            role_configs[tier_name] = LLMRoleConfig(
                provider=candidate_provider,
                model=candidate_model,
                endpoint=candidate_endpoint,
            )
        else:
            baseline_tag = campaign_profile.baseline_tier_mappings.get(tier_name)
            if baseline_tag is None:
                raise ValueError(
                    f"missing baseline_tier_mapping for {tier_name}"
                )
            role_configs[tier_name] = LLMRoleConfig(
                provider="ollama",
                model=baseline_tag,
                endpoint=candidate_endpoint,
            )

    return SUTConfig(
        g8ee_url=g8ee_url,
        primary=role_configs["primary"],
        assistant=role_configs["assistant"],
        lite=role_configs["lite"],
        arm=arm,
        operator_url=operator_url or "https://localhost:8444",
        operator_session_id=operator_session_id,
        auth_context=auth_context,  # type: ignore[arg-type]
        candidate_model=candidate_model,
    )


def build_campaign_sut_factory(
    campaign_profile: CampaignProfile | None,
    cohort_variant_map: dict[str, str] | None,
    g8ee_url: str = "",
    operator_url: str | None = None,
    operator_session_id: str | None = None,
    auth_context: object | None = None,
) -> SUTFactory:
    """Build a ``SUTFactory`` that dispatches by arm type.

    For the ``direct`` arm, creates a ``DirectProviderSUT`` using the
    cohort's primary model. For ``ensemble_ungoverned`` and ``doctrine``
    arms, creates a ``G8eeChatSUT`` with tier replacement logic from the
    campaign profile's ``model_tier_assignments`` and
    ``baseline_tier_mappings``.

    When ``campaign_profile`` is ``None`` (no profile provided), the
    factory always creates ``DirectProviderSUT`` regardless of arm,
    preserving the existing behavior for direct-track-only campaigns.
    """
    from g8e_evals.harness import LLMRoleConfig
    from g8e_evals.sut.direct_provider import DirectProviderSUT

    def factory(cohort: ModelCohort, arm: Arm) -> SUTProtocol:
        if arm == Arm.DIRECT or campaign_profile is None or cohort_variant_map is None:
            model_id = cohort.role_bindings[0].model_id
            endpoint = cohort.role_bindings[0].endpoint
            config = SUTConfig(
                g8ee_url="",
                primary=LLMRoleConfig(provider="ollama", model=model_id, endpoint=endpoint),
                arm=arm,
            )
            return DirectProviderSUT(config)

        from g8e_evals.sut.g8ee_chat import G8eeChatSUT

        config = build_tier_fitness_sut_config(
            cohort=cohort,
            arm=arm,
            campaign_profile=campaign_profile,
            cohort_variant_map=cohort_variant_map,
            g8ee_url=g8ee_url,
            operator_url=operator_url,
            operator_session_id=operator_session_id,
            auth_context=auth_context,
        )
        return G8eeChatSUT(config)

    return factory


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

    When ``campaign_profile`` and ``model_registry`` are provided, the
    runner populates the report-level ``CampaignBinding`` on the
    ``RunManifest`` with the campaign identity, frozen profile and
    registry hashes, expected-record policy hash, both environment
    scopes, and report role. Assignment, model, repetition, role, and
    inference identities remain on their own records. The
    ``cohort_variant_map`` maps cohort IDs to registry variant IDs for
    SUT factory wiring.

    Each ``run`` starts a fresh report directory. The runner does not
    reload a prior campaign during ``run``; explicit batching via
    ``--task-offset`` and ``--task-limit`` replaces the historical resume
    behavior.
    """

    spec: CampaignSpec
    sut_factory: SUTFactory
    tasks: list[Task]
    grader: GraderProtocol
    output_dir: Path
    evidence_key: object | None = None
    campaign_profile: CampaignProfile | None = None
    model_registry: ModelRegistry | None = None
    cohort_variant_map: dict[str, str] | None = None
    campaign_set_plan: CampaignSetPlan | None = None
    disk_space_min_bytes: int = 0
    _run_id: str = field(default_factory=lambda: str(uuid.uuid4()))
    _report_dir: Path | None = None
    _suts: dict[tuple[str, str], SUTProtocol] = field(default_factory=dict)

    @property
    def report_dir(self) -> Path:
        if self._report_dir is None:
            self._report_dir = self._allocate_fresh_report_dir()
        return self._report_dir

    def _allocate_fresh_report_dir(self) -> Path:
        """Allocate a new unique report directory under output_dir.

        Uses a timestamp prefix and appends a numeric suffix when a
        directory with the same timestamp already exists.
        """
        ts = datetime.now(UTC).strftime("%Y%m%d-%H%M%S")
        base = self.output_dir / f"{self.spec.suite}-campaign-{ts}"
        if not base.exists():
            return base
        # Collision: append a numeric suffix to avoid overwriting
        for i in range(2, 1000):
            candidate = self.output_dir / f"{self.spec.suite}-campaign-{ts}-{i:03d}"
            if not candidate.exists():
                return candidate
        # Extremely unlikely: 999 dirs in the same second
        return self.output_dir / f"{self.spec.suite}-campaign-{ts}-{uuid.uuid4().hex[:8]}"

    def _get_sut(self, cohort: ModelCohort, arm: Arm) -> SUTProtocol:
        key = (cohort.cohort_id, arm.value)
        if key not in self._suts:
            self._suts[key] = self.sut_factory(cohort, arm)
        return self._suts[key]

    async def _close_suts(self) -> None:
        """Close every SUT created during the run.

        Called from the ``finally`` block in ``run()`` so SUTs are closed
        on success, typed stop, and exception. Close errors are collected
        and raised as a ``CleanupError`` when no primary exception is in
        flight; when a primary exception is already propagating, cleanup
        errors are logged with context so the primary exception is
        preserved.
        """
        cleanup_errors: list[Exception] = []
        for sut in self._suts.values():
            close = getattr(sut, "close", None)
            if close is None:
                continue
            try:
                result = close()
                if hasattr(result, "__await__"):
                    await result
            except Exception as e:
                cleanup_errors.append(e)

        if cleanup_errors:
            detail = "; ".join(f"{type(e).__name__}: {e}" for e in cleanup_errors)
            if sys.exc_info()[0] is not None:
                logger.error("SUT cleanup failed after primary exception: %s", detail)
            else:
                raise CleanupError(f"SUT cleanup failed ({len(cleanup_errors)} error(s): {detail})") from cleanup_errors[0]

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

    def _write_progress(
        self,
        total: int,
        completed: int,
        current_assignment: CampaignAssignment | None,
        terminal_status: str | None,
        elapsed: float,
        status_counts: dict[str, int],
    ) -> None:
        """Write a pollable progress JSON file for live observation."""
        record = {
            "campaign_id": self.spec.campaign_id,
            "run_id": self._run_id,
            "total_assignments": total,
            "completed_assignments": completed,
            "remaining_assignments": total - completed,
            "current_cohort": current_assignment.model_cohort_id if current_assignment else None,
            "current_task": current_assignment.task_id if current_assignment else None,
            "current_arm": current_assignment.arm_id if current_assignment else None,
            "current_status": terminal_status,
            "elapsed_seconds": round(elapsed, 2),
            "status_counts": status_counts,
            "updated_at": datetime.now(UTC).isoformat(),
        }
        _write_atomic(self.report_dir / CAMPAIGN_PROGRESS_JSON, json.dumps(record, indent=2))

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

    def _load_existing_generations(self) -> list[IndexGeneration]:
        """Load existing index generations from the report directory."""
        path = self.report_dir / CAMPAIGN_INDEX_JSONL
        if not path.exists():
            return []
        records: list[IndexGeneration] = []
        for line in path.read_text().splitlines():
            if line.strip():
                records.append(IndexGeneration.model_validate_json(line))
        return records

    def _compute_assignment_dispositions(
        self,
        assignments: list[CampaignAssignment],
        attempts: list[AttemptRecord],
    ) -> list[AssignmentDispositionEntry]:
        """Compute assignment dispositions from terminal attempts.

        An assignment with a terminal ``COMPLETED`` attempt is ``EFFECTIVE``.
        An assignment with a terminal ``INFRASTRUCTURE_FAILED`` attempt is
        ``SUPERSEDED`` (eligible for replacement). Any other terminal
        failure is ``QUALIFICATION``. An assignment with no terminal
        attempt is ``UNAVAILABLE``.
        """
        terminal_by_assignment: dict[str, AttemptRecord] = {}
        for attempt in attempts:
            if not attempt.assignment_id:
                continue
            terminal_by_assignment[attempt.assignment_id] = attempt

        dispositions: list[AssignmentDispositionEntry] = []
        for assignment in assignments:
            terminal = terminal_by_assignment.get(assignment.assignment_id)
            if terminal is None:
                disposition = AssignmentDisposition.UNAVAILABLE
            elif terminal.terminal_status == TerminalStatus.COMPLETED:
                disposition = AssignmentDisposition.EFFECTIVE
            elif terminal.terminal_status == TerminalStatus.INFRASTRUCTURE_FAILED:
                disposition = AssignmentDisposition.SUPERSEDED
            else:
                disposition = AssignmentDisposition.QUALIFICATION
            dispositions.append(AssignmentDispositionEntry(
                assignment_id=assignment.assignment_id,
                disposition=disposition,
            ))
        return dispositions

    def _compute_report_checksums(
        self,
        attempts: list[AttemptRecord],
        metrics: list[MetricObservation] | None = None,
    ) -> list[str]:
        """Compute SHA-256 checksums of complete reports (attempts and metrics).

        The checksum covers canonical JSON of both attempts and metrics,
        matching the standalone report validator's computation.
        """
        attempts_data = [json.loads(a.model_dump_json()) for a in attempts]
        metrics_data = (
            [json.loads(m.model_dump_json()) for m in metrics] if metrics else []
        )
        payload = json.dumps(
            {"attempts": attempts_data, "metrics": metrics_data},
            allow_nan=False,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        return [hashlib.sha256(payload.encode()).hexdigest()]

    def _write_index_generation(
        self,
        generation_number: int,
        parent_generation_hash: str,
        creation_reason: IndexCreationReason,
        report_checksums: list[str],
        assignment_dispositions: list[AssignmentDispositionEntry],
    ) -> IndexGeneration:
        """Append a new index generation to the campaign index file."""
        content_hash = compute_index_generation_hash(
            generation_number=generation_number,
            parent_generation_hash=parent_generation_hash,
            creation_reason=creation_reason,
            report_checksums=report_checksums,
            assignment_dispositions=[
                {"assignment_id": d.assignment_id, "disposition": d.disposition.value}
                for d in assignment_dispositions
            ],
        )
        generation = IndexGeneration(
            generation_number=generation_number,
            parent_generation_hash=parent_generation_hash,
            creation_reason=creation_reason,
            report_checksums=report_checksums,
            assignment_dispositions=assignment_dispositions,
            content_hash=content_hash,
        )
        path = self.report_dir / CAMPAIGN_INDEX_JSONL
        with open(path, "a") as f:
            f.write(generation.model_dump_json() + "\n")
        return generation

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
                            grader_id=self.grader.grader_id,
                            grader_version=self.grader.grader_version,
                            grader_class=GraderClass.DETERMINISTIC,
                        ),
                    ],
                    metadata={"instruction_id_list": t.metadata.instruction_id_list},
                )
            )
        return task_defs

    def _build_run_manifest(self, task_defs: list[TaskDefinition]) -> RunManifest:
        """Build the RunManifest for the report directory.

        When ``campaign_profile`` and ``model_registry`` are provided,
        populates the report-level ``CampaignBinding`` with the campaign
        identity, frozen profile and registry hashes, expected-record
        policy hash, both environment scopes, and report role.
        """
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
        # Use the first cohort's role bindings for the run manifest's
        # role_to_model mapping. The report-level CampaignBinding carries
        # campaign-level authority hashes and environment scopes, not
        # per-cohort or per-assignment identity. In a multi-cohort
        # campaign each cohort has its own model; the run manifest
        # represents the campaign-level identity, not per-cohort state.
        first_cohort = self.spec.cohorts[0] if self.spec.cohorts else None
        if first_cohort is not None:
            for rb in first_cohort.role_bindings:
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

        campaign_binding = self._build_campaign_binding()

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
            campaign_binding=campaign_binding,
        )

    def _effective_orchestrator_scope(self) -> str:
        """Return the orchestrator scope stamped on resource observations.

        When a campaign profile is bound, its ``hardware_identity`` is
        the frozen declaration and the authority (already preflighted
        against the observed host prefix in ``_run_body``). Without a
        bound profile, the observed canonical scope is used.
        """
        if self.campaign_profile is not None:
            return self.campaign_profile.hardware_identity
        return _ORCHESTRATOR_SCOPE

    def _build_campaign_binding(self) -> CampaignBinding | None:
        """Build report-level CampaignBinding from the campaign profile and model registry.

        Returns None when either ``campaign_profile`` or ``model_registry``
        is not provided. When both are present, populates the binding
        with parent/child campaign identity, frozen profile and registry
        hashes, expected-record policy hash, both environment scopes, and
        report role. Assignment, model, repetition, role, and inference
        identities remain on their own records, not on this report-level
        binding.

        When ``campaign_set_plan`` is provided, the binding identifies
        this report as a child campaign (``ReportRole.CHILD``) and
        populates ``child_campaign_id``/``child_campaign_revision`` from
        the matching child plan. Otherwise the report is a standalone
        campaign (``ReportRole.SINGLE``).
        """
        if self.campaign_profile is None or self.model_registry is None:
            return None

        child_campaign_id: str | None = None
        child_campaign_revision: str | None = None
        report_role = ReportRole.SINGLE

        if self.campaign_set_plan is not None:
            child_plan = next(
                (cp for cp in self.campaign_set_plan.child_plans
                 if cp.child_id == self.spec.campaign_id),
                None,
            )
            if child_plan is None:
                raise CampaignRunnerError(
                    f"campaign_id {self.spec.campaign_id!r} does not match any "
                    f"child in campaign-set plan {self.campaign_set_plan.set_id!r}"
                )
            child_campaign_id = child_plan.child_id
            child_campaign_revision = child_plan.child_revision
            report_role = ReportRole.CHILD

        return CampaignBinding(
            campaign_id=self.campaign_profile.campaign_id,
            campaign_revision=self.campaign_profile.campaign_revision,
            report_role=report_role,
            child_campaign_id=child_campaign_id,
            child_campaign_revision=child_campaign_revision,
            campaign_profile_hash=self.campaign_profile.content_hash,
            model_registry_hash=self.model_registry.content_hash,
            required_record_policy_hash=self.campaign_profile.required_record_policy_hash,
            orchestrator_hardware_identity=self.campaign_profile.hardware_identity,
            orchestrator_environment_stratum=self.campaign_profile.environment_stratum,
            provider_hardware_identity=self.campaign_profile.provider_hardware_identity,
            provider_environment_stratum=self.campaign_profile.provider_environment_stratum,
            track_arm_assignments=list(self.campaign_profile.track_arm_assignments),
        )

    async def _execute_assignment(
        self,
        assignment: CampaignAssignment,
        cohort: ModelCohort,
        arm_def: ArmDefinition,
        task: Task,
        budget_tracker: BudgetTracker | None = None,
    ) -> tuple[
        list[AttemptRecord],
        list[MetricObservation],
        list[ResourceObservation],
        list[StageObservation],
        list[EvidenceIndex],
    ]:
        """Execute one assignment, handling retries.

        Returns all attempts (including retry attempts), metric
        observations from the terminal attempt, resource observations
        (one per actual provider inference), stage observations (one
        per inference), and evidence-index entries (one per indexed
        artifact). Each provider call (including retries) is counted
        against the request budget when a ``budget_tracker`` is
        provided.
        """
        attempt_num = 0
        parent_attempt_id: str | None = None
        orchestrator_scope = self._effective_orchestrator_scope()

        sut = self._get_sut(cohort, arm_def.arm_id)
        expected_model = _model_id_for_cohort(cohort)

        new_attempts: list[AttemptRecord] = []
        all_resource_observations: list[ResourceObservation] = []
        all_stages: list[StageObservation] = []
        all_evidence: list[EvidenceIndex] = []

        while True:
            # Check request budget before each provider call (including retries)
            if budget_tracker is not None:
                budget_tracker.check_request_budget()

            attempt_id = f"{self._run_id}:{assignment.assignment_id}:{attempt_num}"
            started_at = datetime.now(UTC)

            infrastructure_error: Exception | None = None
            response: Response | None = None
            try:
                response = await sut.get_answer(task)
            except Exception as exc:
                infrastructure_error = exc

            # Record this provider call against the request budget
            if budget_tracker is not None:
                budget_tracker.record_provider_call()

            # Record observed token/USD usage from the SUT response and
            # check token/USD ceilings when the policy declares them
            # observable and not excluded. When the ceiling is reached
            # after a successful provider call, the current attempt's
            # records must still be materialized before the stop
            # propagates (transactional budget stop).
            usage_budget_exhausted = False
            if budget_tracker is not None and response is not None:
                tokens, usd = _extract_usage_from_response(response)
                budget_tracker.record_usage(tokens, usd)
                try:
                    budget_tracker.check_usage_budgets()
                except BudgetExhausted:
                    usage_budget_exhausted = True

            ended_at = datetime.now(UTC)

            # Build instrumentation records for each actual provider
            # inference exposed by the SUT response boundary. One
            # ResourceObservation per inference; one StageObservation per
            # inference; one EvidenceIndex entry per indexed artifact.
            if response is not None:
                for inf_obs in response.inference_observations:
                    stage_id = f"{attempt_id}:{inf_obs.inference_id}"
                    resource_obs = _build_resource_observation(
                        obs=inf_obs,
                        campaign_id=self.spec.campaign_id,
                        child_id=self.spec.campaign_id,
                        run_id=self._run_id,
                        assignment_id=assignment.assignment_id,
                        attempt_id=attempt_id,
                        task_id=task.id,
                        stage_id=stage_id,
                        orchestrator_scope=orchestrator_scope,
                    )
                    all_resource_observations.append(resource_obs)
                    all_stages.append(_build_stage_observation(
                        obs=inf_obs,
                        stage_id=stage_id,
                        attempt_id=attempt_id,
                        run_id=self._run_id,
                        task_id=task.id,
                    ))
                    all_evidence.extend(_build_evidence_index_entries(
                        obs=inf_obs,
                        run_id=self._run_id,
                        attempt_id=attempt_id,
                    ))

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
                score = self.grader.grade(task, response)
                passed = bool(getattr(score, "passed", False))
                metric = MetricObservation(
                    metric_id=self.grader.grader_id,
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
            new_attempts.append(attempt)

            # If the usage budget was exhausted after this provider call
            # succeeded, the current attempt is COMPLETED and its records
            # are materialized. Raise a transactional stop carrying the
            # records so the caller persists them before propagating
            # the stop to remaining assignments. Do not retry.
            if usage_budget_exhausted:
                raise BudgetExhaustedDuringExecution(
                    f"usage budget exhausted after provider call for {attempt_id}",
                    attempts=new_attempts,
                    metrics=metrics,
                    resource_observations=all_resource_observations,
                    stages=all_stages,
                    evidence=all_evidence,
                )

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

        # Validate the retry chain (new attempts only)
        validate_retry_chain(new_attempts, assignment.assignment_id, self.spec.retry_policy)

        return new_attempts, metrics, all_resource_observations, all_stages, all_evidence

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
        analysis. Each ``run`` starts a fresh report directory; the
        runner does not reload a prior campaign. Explicit batching via
        ``--task-offset`` and ``--task-limit`` replaces the historical
        resume behavior.

        SUTs are closed in a ``finally`` block so cleanup runs on
        success, typed stop, and every exception path. Cleanup errors
        propagate when no primary exception is in flight; otherwise
        they are logged with context so the primary exception is
        preserved.
        """
        try:
            return await self._run_body()
        finally:
            await self._close_suts()

    async def _run_body(self) -> CampaignResult:
        """Implementation of ``run()`` without the cleanup lifecycle.

        All SUT cleanup is handled by the ``finally`` block in ``run()``.
        """
        # 0. Validate budget observability before any work begins. A
        # declared ceiling that is not observable and not explicitly
        # excluded fails before the report directory is created.
        self.spec.budget_observability_policy.validate_budget(self.spec.provider_budget)

        # 0b. Profile-bound preflights. When a frozen campaign profile is
        # bound, the declared orchestrator hardware identity is the
        # authority: the observed host prefix must match it, and every
        # arm the preregistration declares must be an arm the profile
        # declares. Both checks fail before the report directory is
        # created.
        if self.campaign_profile is not None:
            _validate_orchestrator_scope(self.campaign_profile.hardware_identity)
            _validate_arm_coherence(self.campaign_profile, self.spec.preregistration)

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
        provider_budget_hash = compute_provider_budget_hash(self.spec.provider_budget)
        budget_observability_policy_hash = compute_budget_observability_policy_hash(
            self.spec.budget_observability_policy
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
            budget_observability_policy_hash,
        )

        # 3. Create report directory and persist campaign state
        self.report_dir.mkdir(parents=True, exist_ok=True)

        self._persist_campaign_state(assignments, schedule, manifest)

        # 4. Write run manifest and task definitions
        task_defs = self._build_task_definitions()
        run_manifest = self._build_run_manifest(task_defs)
        _write_atomic(self.report_dir / MANIFEST_JSON, run_manifest.model_dump_json(indent=2))
        _write_jsonl(self.report_dir / TASKS_JSONL, task_defs)

        # 5. Set campaign status to running
        self._write_campaign_status(CampaignStatus.RUNNING)

        # 5b. Write the initial index generation
        self._write_index_generation(
            generation_number=0,
            parent_generation_hash="0" * 64,
            creation_reason=IndexCreationReason.INITIAL,
            report_checksums=[],
            assignment_dispositions=[],
        )

        # 6. Execute assignments in schedule order
        cohort_by_id = self._cohort_by_id()
        task_by_id = self._task_by_id()

        all_attempts: list[AttemptRecord] = []
        all_metrics: list[MetricObservation] = []
        all_resource_observations: list[ResourceObservation] = []
        all_stages: list[StageObservation] = []
        all_evidence_index: list[EvidenceIndex] = []
        all_tool_scorecards: list[ToolCallScorecard] = []
        all_escalation_records: list[EscalationRecord] = []
        all_security_events: list[SecurityEventRecord] = []
        all_correlated_errors: list[CorrelatedErrorRecord] = []
        stop_reason: CampaignStopReason | None = None
        budget_tracker = BudgetTracker(
            self.spec.provider_budget,
            policy=self.spec.budget_observability_policy,
        )

        total_assignments = len(schedule.ordered_assignment_ids)
        completed_count = 0
        status_counts: dict[str, int] = {}
        campaign_start = time.monotonic()
        attempts_path = self.report_dir / ATTEMPTS_JSONL
        metrics_path = self.report_dir / METRICS_JSONL

        logger.info(
            "campaign %s starting: %d total assignments, %d remaining",
            self.spec.campaign_id,
            total_assignments,
            total_assignments,
        )

        for assignment_id in schedule.ordered_assignment_ids:
            # Disk-space preflight: check before every block
            check_disk_space(self.report_dir, min_bytes=self.disk_space_min_bytes)

            assignment = next(a for a in assignments if a.assignment_id == assignment_id)
            cohort = cohort_by_id[assignment.model_cohort_id]
            arm_def = get_arm_definition(Arm(assignment.arm_id))
            task = task_by_id[assignment.task_id]

            # Budget enforcement: check request ceiling before dispatch.
            # Each provider call (including retries) is counted inside
            # _execute_assignment via the budget_tracker.
            try:
                budget_tracker.check_request_budget()
            except BudgetExhausted:
                # Materialize terminal outcomes for remaining assignments
                remaining = [
                    a for a in assignments
                    if schedule.ordered_assignment_ids.index(a.assignment_id) >= schedule.ordered_assignment_ids.index(assignment_id)
                ]
                for rem in remaining:
                    rem_arm_def = get_arm_definition(Arm(rem.arm_id))
                    stop_attempt = self._materialize_budget_stop(rem, rem_arm_def)
                    all_attempts.append(stop_attempt)
                stop_reason = CampaignStopReason.BUDGET_EXHAUSTED
                break

            assignment_start = time.monotonic()

            logger.info(
                "assignment %d/%d: cohort=%s task=%s arm=%s — dispatching",
                completed_count + 1,
                total_assignments,
                assignment.model_cohort_id,
                assignment.task_id,
                assignment.arm_id,
            )

            try:
                new_attempts, metrics, resource_obs, stages, evidence = (
                    await self._execute_assignment(
                        assignment, cohort, arm_def, task, budget_tracker=budget_tracker
                    )
                )
            except BudgetExhaustedDuringExecution as exc:
                # The current assignment's provider call completed but a
                # usage ceiling (tokens/USD) was reached. The current
                # attempt is COMPLETED and its records are materialized.
                # Persist them before propagating the stop to remaining
                # assignments (transactional budget stop).
                all_attempts.extend(exc.attempts)
                all_metrics.extend(exc.metrics)
                all_resource_observations.extend(exc.resource_observations)
                all_stages.extend(exc.stages)
                all_evidence_index.extend(exc.evidence)

                _append_jsonl(attempts_path, exc.attempts)
                _append_jsonl(metrics_path, exc.metrics)
                _append_jsonl(self.report_dir / RESOURCE_OBSERVATIONS_JSONL, exc.resource_observations)
                _append_jsonl(self.report_dir / STAGES_JSONL, exc.stages)
                _append_jsonl(self.report_dir / EVIDENCE_INDEX_JSONL, exc.evidence)

                completed_count += 1

                # Materialize terminal outcomes for remaining assignments
                remaining = [
                    a for a in assignments
                    if schedule.ordered_assignment_ids.index(a.assignment_id) > schedule.ordered_assignment_ids.index(assignment_id)
                ]
                for rem in remaining:
                    rem_arm_def = get_arm_definition(Arm(rem.arm_id))
                    stop_attempt = self._materialize_budget_stop(rem, rem_arm_def)
                    all_attempts.append(stop_attempt)
                stop_reason = CampaignStopReason.BUDGET_EXHAUSTED
                break
            except BudgetExhausted:
                # Budget exhausted during execution (e.g. retry pushed
                # request count over the ceiling). Materialize remaining.
                remaining = [
                    a for a in assignments
                    if schedule.ordered_assignment_ids.index(a.assignment_id) > schedule.ordered_assignment_ids.index(assignment_id)
                ]
                for rem in remaining:
                    rem_arm_def = get_arm_definition(Arm(rem.arm_id))
                    stop_attempt = self._materialize_budget_stop(rem, rem_arm_def)
                    all_attempts.append(stop_attempt)
                stop_reason = CampaignStopReason.BUDGET_EXHAUSTED
                break
            all_attempts.extend(new_attempts)
            all_metrics.extend(metrics)
            all_resource_observations.extend(resource_obs)
            all_stages.extend(stages)
            all_evidence_index.extend(evidence)
            completed_count += 1

            # Incremental persistence: append new attempts, metrics, and
            # instrumentation records immediately so the report directory
            # reflects live state.
            _append_jsonl(attempts_path, new_attempts)
            _append_jsonl(metrics_path, metrics)
            _append_jsonl(self.report_dir / RESOURCE_OBSERVATIONS_JSONL, resource_obs)
            _append_jsonl(self.report_dir / STAGES_JSONL, stages)
            _append_jsonl(self.report_dir / EVIDENCE_INDEX_JSONL, evidence)

            # Track status counts
            terminal_attempt = new_attempts[-1]
            status_key = terminal_attempt.terminal_status.value
            status_counts[status_key] = status_counts.get(status_key, 0) + 1

            elapsed = time.monotonic() - assignment_start
            total_elapsed = time.monotonic() - campaign_start

            logger.info(
                "assignment %d/%d: cohort=%s task=%s arm=%s status=%s elapsed=%.1fs total=%.1fs",
                completed_count,
                total_assignments,
                assignment.model_cohort_id,
                assignment.task_id,
                assignment.arm_id,
                status_key,
                elapsed,
                total_elapsed,
            )

            # Write pollable progress file
            self._write_progress(
                total=total_assignments,
                completed=completed_count,
                current_assignment=assignment,
                terminal_status=status_key,
                elapsed=total_elapsed,
                status_counts=dict(status_counts),
            )

            # Cohort drift check
            if terminal_attempt.terminal_status == TerminalStatus.MODEL_FAILED:
                # The drift is detected inside _execute_assignment via the
                # response model check. If the terminal status is
                # MODEL_FAILED due to drift, stop the campaign.
                if terminal_attempt.missingness_or_failure and "drift" in terminal_attempt.missingness_or_failure:
                    logger.warning(
                        "campaign stopping: cohort drift detected on %s",
                        assignment.model_cohort_id,
                    )
                    stop_reason = CampaignStopReason.COHORT_DRIFT
                    break

        logger.info(
            "campaign %s finished: %d/%d assignments completed, %d attempts, elapsed=%.1fs",
            self.spec.campaign_id,
            completed_count,
            total_assignments,
            len(all_attempts),
            time.monotonic() - campaign_start,
        )

        # 8. Write attempts, metrics, and instrumentation records (full authoritative write)
        _write_jsonl(self.report_dir / ATTEMPTS_JSONL, all_attempts)
        _write_jsonl(self.report_dir / METRICS_JSONL, all_metrics)
        _write_jsonl(self.report_dir / RESOURCE_OBSERVATIONS_JSONL, all_resource_observations)
        _write_jsonl(self.report_dir / STAGES_JSONL, all_stages)
        _write_jsonl(self.report_dir / EVIDENCE_INDEX_JSONL, all_evidence_index)
        _write_jsonl(self.report_dir / TOOL_CALL_SCORECARDS_JSONL, all_tool_scorecards)
        _write_jsonl(self.report_dir / ESCALATION_RECORDS_JSONL, all_escalation_records)
        _write_jsonl(self.report_dir / SECURITY_EVENTS_JSONL, all_security_events)
        _write_jsonl(self.report_dir / CORRELATED_ERRORS_JSONL, all_correlated_errors)

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
            stages=all_stages,
            resource_observations=all_resource_observations,
            tool_call_scorecards=all_tool_scorecards,
            escalation_records=all_escalation_records,
            security_events=all_security_events,
            correlated_error_records=all_correlated_errors,
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

        # 10b. Write report checksum
        report_checksum = self._compute_report_checksums(all_attempts, all_metrics)[0]
        _write_atomic(
            self.report_dir / REPORT_CHECKSUM_JSON,
            json.dumps({"checksum": report_checksum}, indent=2),
        )

        # 10c. Finalize only when the campaign completed and the report validates
        if final_status == CampaignStatus.COMPLETED:
            standalone = validate_standalone_report(self.report_dir)
            if standalone.ok:
                all_generations = self._load_existing_generations()
                last_gen = all_generations[-1]
                final_dispositions = self._compute_assignment_dispositions(assignments, all_attempts)
                self._write_index_generation(
                    generation_number=last_gen.generation_number + 1,
                    parent_generation_hash=last_gen.content_hash,
                    creation_reason=IndexCreationReason.FINALIZATION,
                    report_checksums=[report_checksum],
                    assignment_dispositions=final_dispositions,
                )
                final_status = CampaignStatus.FINALIZED
            else:
                final_status = CampaignStatus.FAILED_INTEGRITY

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
    "DEFAULT_BUDGET_OBSERVABILITY_POLICY",
    "BudgetCeiling",
    "BudgetExhausted",
    "BudgetExhaustedDuringExecution",
    "BudgetObservabilityPolicy",
    "BudgetTracker",
    "CampaignResult",
    "CampaignRunner",
    "CampaignRunnerError",
    "CampaignSpec",
    "CampaignStopReason",
    "CleanupError",
    "DiskSpacePreflightError",
    "GraderProtocol",
    "SUTFactory",
    "SUTProtocol",
    "build_campaign_manifest",
    "build_campaign_state",
    "build_campaign_sut_factory",
    "build_tier_fitness_sut_config",
    "check_disk_space",
    "compute_budget_observability_policy_hash",
    "compute_provider_budget_hash",
    "derive_cohorts_from_registry",
]
