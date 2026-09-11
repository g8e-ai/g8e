# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Typed append-only index generations, supersession, tier binding, and
resource observation models for the campaign.

Each index generation carries a parent-generation hash, creation reason,
complete report checksums, and assignment disposition
(``effective``, ``superseded``, ``qualification``, ``unavailable``).
Exactly one effective valid report exists per publishable assignment.

Supersession is permitted only for interrupted or infrastructure-invalid
assignments. Result-aware discretionary reruns create a new campaign
revision and cannot replace an unfavorable valid result.

Resource observations bind latency, memory, energy, and throughput to
run ID, model variant ID, task block, hardware identity, collection
tool/version, and source evidence hash. VRAM is never inferred from
parameter count or quantization labels.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Self

from pydantic import BaseModel, ConfigDict, Field, model_validator


INDEX_GENERATION_SCHEMA_VERSION = "1.0.0"

_VALID_TIER_NAMES = frozenset({"primary", "assistant", "lite"})
_VALID_ARM_IDS = frozenset({"direct", "ensemble_ungoverned", "doctrine"})
_RETRYABLE_STATUSES = frozenset({"infrastructure_failed"})


class ModelRole(StrEnum):
    """Typed model role within a heterogeneous stack.

    ``PRIMARY``: The primary reasoning model (largest tier).
    ``ASSISTANT``: The assistant model (middle tier).
    ``LITE``: The lite model (smallest tier, e.g. triage).
    """

    PRIMARY = "primary"
    ASSISTANT = "assistant"
    LITE = "lite"


class VerificationStatus(StrEnum):
    """Typed verification status for a record or observation.

    ``PENDING``: Verification has not yet been performed.
    ``VERIFIED``: The record has been independently verified against
    indexed source evidence. A VERIFIED record without source evidence
    references or a source evidence hash is not independently verifiable
    and is rejected.
    ``FAILED``: Verification was attempted and failed.
    ``NOT_APPLICABLE``: Verification does not apply to this record.
    """

    PENDING = "pending"
    VERIFIED = "verified"
    FAILED = "failed"
    NOT_APPLICABLE = "not_applicable"


class MeasurementAvailability(StrEnum):
    """Typed availability state for a measurement field.

    ``MEASURED``: The value was measured and is present.
    ``UNAVAILABLE``: The measurement is unavailable at the observation
    boundary (e.g. remote GPU metrics without an API).
    ``NOT_APPLICABLE``: The measurement does not apply to this
    configuration (e.g. GPU metrics on a CPU-only host).
    ``WITHHELD``: The measurement was withheld for disclosure reasons.
    """

    MEASURED = "measured"
    UNAVAILABLE = "unavailable"
    NOT_APPLICABLE = "not_applicable"
    WITHHELD = "withheld"


class MeasurementScope(StrEnum):
    """Typed scope identifying where a measurement was taken.

    ``ORCHESTRATOR_LOCAL``: Measured on the orchestrator host (the host
    running the eval CLI).
    ``PROVIDER_REMOTE``: Measured on the remote provider host (e.g. the
    remote Ollama server).
    """

    ORCHESTRATOR_LOCAL = "orchestrator_local"
    PROVIDER_REMOTE = "provider_remote"


class UnavailableMeasurement(BaseModel):
    """Typed explanation for an unavailable measurement field.

    Pairs a field name with its availability state, scope, and reason.
    Every ``None`` measurement field on a ``ResourceObservation`` must
    have a matching entry in ``unavailable_measurements``. A measured
    field (non-``None``) must not have an entry.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    field_name: str = Field(min_length=1, description="Name of the unavailable measurement field.")
    availability: MeasurementAvailability = Field(description="Typed availability state.")
    scope: MeasurementScope = Field(description="Where the measurement would have been taken.")
    reason: str = Field(min_length=1, description="Why the measurement is unavailable.")


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


class IndexCreationReason(StrEnum):
    """Reason a new index generation was created.

    ``INITIAL``: The first generation when the campaign starts.
    ``RESUME``: A new generation created after process restart to record
    resumed or missing assignments.
    ``SUPERSESSION``: A new generation created to replace an interrupted
    or infrastructure-invalid assignment.
    ``FINALIZATION``: The final generation written after all assignments
    are complete and the campaign is finalized.
    """

    INITIAL = "initial"
    RESUME = "resume"
    SUPERSESSION = "supersession"
    FINALIZATION = "finalization"


class AssignmentDisposition(StrEnum):
    """Typed disposition of an assignment within an index generation.

    ``EFFECTIVE``: The assignment has exactly one effective valid report.
    ``SUPERSEDED``: The assignment was superseded by a later generation
    due to interruption or infrastructure failure.
    ``QUALIFICATION``: The assignment has a typed qualification outcome
    (unavailable, license-blocked, incompatible, out-of-memory,
    backend-unsupported).
    ``UNAVAILABLE``: The assignment is unavailable and has no report.
    """

    EFFECTIVE = "effective"
    SUPERSEDED = "superseded"
    QUALIFICATION = "qualification"
    UNAVAILABLE = "unavailable"


class AssignmentDispositionEntry(BaseModel):
    """One assignment's disposition within an index generation.

    Binds the assignment ID to its disposition. Multiple entries for
    the same assignment are allowed only when at most one has
    ``EFFECTIVE`` disposition; the rest must be ``SUPERSEDED``.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    assignment_id: str = Field(min_length=1, description="Campaign assignment ID.")
    disposition: AssignmentDisposition = Field(description="Typed disposition of the assignment.")


class IndexGeneration(BaseModel):
    """One append-only index generation.

    Each generation carries a parent-generation hash, creation reason,
    complete report checksums, and assignment dispositions. The first
    generation has a zero parent hash (``"0" * 64``). Each subsequent
    generation's parent hash must match the previous generation's
    content hash.

    The ``content_hash`` is SHA-256 over canonical JSON of the generation
    (sorted assignment dispositions, sorted report checksums, sorted
    keys, no extra whitespace). Changing any field changes the hash and
    invalidates downstream chain validation.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    generation_number: int = Field(ge=0, description="Zero-indexed generation number.")
    parent_generation_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the parent generation's content hash. Zero hash for the first generation.",
    )
    creation_reason: IndexCreationReason = Field(description="Reason this generation was created.")
    report_checksums: list[str] = Field(
        default_factory=list,
        description="SHA-256 checksums of complete reports included in this generation.",
    )
    assignment_dispositions: list[AssignmentDispositionEntry] = Field(
        default_factory=list,
        description="Assignment dispositions for this generation.",
    )
    content_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 over canonical JSON of this generation.",
    )

    @model_validator(mode="after")
    def _validate_generation(self) -> Self:
        expected = compute_index_generation_hash(
            generation_number=self.generation_number,
            parent_generation_hash=self.parent_generation_hash,
            creation_reason=self.creation_reason,
            report_checksums=self.report_checksums,
            assignment_dispositions=[
                {"assignment_id": d.assignment_id, "disposition": d.disposition.value}
                for d in self.assignment_dispositions
            ],
        )
        if self.content_hash != expected:
            raise ValueError(
                f"index generation content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self


def compute_index_generation_hash(
    *,
    generation_number: int,
    parent_generation_hash: str,
    creation_reason: IndexCreationReason | str,
    report_checksums: list[str],
    assignment_dispositions: list[dict],
) -> str:
    """Compute the content hash for an index generation without constructing the full model."""
    reason_value = creation_reason.value if isinstance(creation_reason, IndexCreationReason) else creation_reason
    payload = json.dumps(
        {
            "generation_number": generation_number,
            "parent_generation_hash": parent_generation_hash,
            "creation_reason": reason_value,
            "report_checksums": sorted(report_checksums),
            "assignment_dispositions": sorted(
                assignment_dispositions,
                key=lambda d: (d["assignment_id"], d["disposition"]),
            ),
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


def validate_index_chain(generations: list[IndexGeneration]) -> None:
    """Validate an append-only index generation chain.

    The first generation must have a zero parent hash (``"0" * 64``).
    Each subsequent generation's parent hash must match the previous
    generation's content hash. Generation numbers must be contiguous
    starting from 0.

    Raises ``ValueError`` for empty chains, broken parent chains,
    non-contiguous generation numbers, or a non-zero parent on the first
    generation.
    """
    if not generations:
        raise ValueError("index chain must not be empty")

    zero_hash = "0" * 64

    for i, gen in enumerate(generations):
        if i == 0:
            if gen.parent_generation_hash != zero_hash:
                raise ValueError(
                    f"first generation must have zero parent hash: "
                    f"got {gen.parent_generation_hash!r}"
                )
        else:
            prev = generations[i - 1]
            if gen.parent_generation_hash != prev.content_hash:
                raise ValueError(
                    f"generation {gen.generation_number} parent_generation_hash mismatch: "
                    f"declared {gen.parent_generation_hash!r}, "
                    f"expected previous content_hash {prev.content_hash!r}"
                )

        if gen.generation_number != i:
            raise ValueError(
                f"non-contiguous generation number at position {i}: "
                f"expected {i}, got {gen.generation_number}"
            )


def validate_no_duplicate_effective_assignments(generation: IndexGeneration) -> None:
    """Validate that exactly one effective valid report exists per publishable assignment.

    Two ``EFFECTIVE`` dispositions for the same assignment are rejected.
    ``SUPERSEDED``, ``QUALIFICATION``, and ``UNAVAILABLE`` dispositions
    do not count as effective and may appear multiple times.
    """
    effective_assignments: list[str] = []
    for entry in generation.assignment_dispositions:
        if entry.disposition == AssignmentDisposition.EFFECTIVE:
            effective_assignments.append(entry.assignment_id)

    seen: set[str] = set()
    dupes: list[str] = []
    for aid in effective_assignments:
        if aid in seen:
            dupes.append(aid)
        seen.add(aid)

    if dupes:
        raise ValueError(
            f"duplicate effective assignment in generation {generation.generation_number}: "
            f"{sorted(set(dupes))}"
        )


class SupersessionPolicy(BaseModel):
    """Frozen typed supersession policy for campaign assignments.

    Supersession is permitted only for interrupted or
    infrastructure-invalid assignments. A completed valid assignment
    cannot be superseded. Model, governance, human, timeout, and
    invalid-evidence failures are assignment-scoped and cannot be
    superseded.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    supersession_eligible_statuses: frozenset[str] = Field(
        default=frozenset({"infrastructure_failed"}),
        description="Terminal statuses eligible for supersession.",
    )

    def is_supersession_allowed(
        self,
        *,
        prior_disposition: AssignmentDisposition,
        prior_terminal_status: str,
    ) -> bool:
        """Check whether supersession is allowed for a given prior disposition and terminal status.

        Supersession is allowed only when:
        1. The prior disposition is not ``EFFECTIVE`` (effective reports
        cannot be superseded).
        2. The prior terminal status is in the supersession-eligible set
        (typically only ``infrastructure_failed``).
        """
        if prior_disposition == AssignmentDisposition.EFFECTIVE:
            return False
        return prior_terminal_status in self.supersession_eligible_statuses


def validate_supersession_policy(
    *,
    prior_disposition: AssignmentDisposition,
    prior_terminal_status: str,
    new_creation_reason: IndexCreationReason,
) -> None:
    """Validate that a supersession is policy-valid.

    Raises ``ValueError`` if the creation reason is ``SUPERSESSION`` but
    the prior disposition and terminal status do not allow supersession.
    Non-supersession creation reasons (e.g. ``RESUME``) skip the policy
    check.
    """
    if new_creation_reason != IndexCreationReason.SUPERSESSION:
        return

    policy = SupersessionPolicy()
    if not policy.is_supersession_allowed(
        prior_disposition=prior_disposition,
        prior_terminal_status=prior_terminal_status,
    ):
        raise ValueError(
            f"supersession not allowed for prior disposition "
            f"{prior_disposition.value!r} with terminal status {prior_terminal_status!r}"
        )


class TierObservationRecord(BaseModel):
    """Typed observation of a candidate tier on a task.

    Records whether a candidate tier was observed on a task, bound to
    the model variant ID and evidence hash. Non-invoked roles are
    preserved as explicit non-observations (``observed=False``).
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    task_id: str = Field(min_length=1, description="Task ID.")
    tier: str = Field(min_length=1, description="Tier name (primary, assistant, lite).")
    observed: bool = Field(description="Whether the tier was observed on this task.")
    model_variant_id: str = Field(min_length=1, description="Model variant ID bound to this observation.")
    evidence_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the evidence supporting this observation.")


def validate_observed_tier_binding(
    task_tier_declarations: list[dict],
    observations: list[TierObservationRecord],
) -> None:
    """Validate that every declared tier has matching provider-boundary telemetry.

    For each task, every tier in ``declared_tiers`` must have at least one
    observation with ``observed=True``. Observations for non-declared
    tiers are ignored. Explicit non-observations (``observed=False``) for
    declared tiers are rejected.

    ``task_tier_declarations`` is a list of dicts with keys ``task_id``
    and ``declared_tiers`` (a list of tier name strings).
    """
    obs_by_task: dict[str, dict[str, list[TierObservationRecord]]] = {}
    for obs in observations:
        obs_by_task.setdefault(obs.task_id, {}).setdefault(obs.tier, []).append(obs)

    for declaration in task_tier_declarations:
        task_id = declaration["task_id"]
        declared_tiers = declaration["declared_tiers"]
        task_obs = obs_by_task.get(task_id, {})

        for tier in declared_tiers:
            tier_obs_list = task_obs.get(tier, [])
            if not tier_obs_list:
                raise ValueError(
                    f"missing tier observation: task {task_id!r} declares tier {tier!r} "
                    f"but no observation exists"
                )
            has_positive = any(o.observed for o in tier_obs_list)
            if not has_positive:
                raise ValueError(
                    f"declared tier {tier!r} for task {task_id!r} is not observed: "
                    f"all observations have observed=False"
                )


class VariantMetricAggregate(BaseModel):
    """Aggregated metric result for one model variant.

    Metrics are aggregated per variant and never pooled across distinct
    variants. Each aggregate carries the variant ID, numerator,
    denominator, rate, and unit.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID.")
    numerator: int = Field(ge=0, description="Count of positive outcomes.")
    denominator: int = Field(ge=0, description="Total count of measured outcomes.")
    rate: float = Field(ge=0.0, le=1.0, description="Numerator divided by denominator.")
    unit: str = Field(min_length=1, description="Metric unit (e.g. boolean, ratio).")


def aggregate_metrics_by_variant(metrics: list[dict]) -> list[VariantMetricAggregate]:
    """Aggregate metrics by model variant, never pooling across variants.

    Each metric dict must have keys ``model_variant_id``, ``task_id``,
    ``value``, and ``unit``. Metrics with different units for the same
    variant are rejected. Results are sorted by variant ID for
    deterministic output.
    """
    if not metrics:
        return []

    by_variant: dict[str, list[dict]] = {}
    for m in metrics:
        variant_id = m["model_variant_id"]
        by_variant.setdefault(variant_id, []).append(m)

    results: list[VariantMetricAggregate] = []
    for variant_id in sorted(by_variant.keys()):
        variant_metrics = by_variant[variant_id]
        units = {m["unit"] for m in variant_metrics}
        if len(units) > 1:
            raise ValueError(
                f"unit mismatch for variant {variant_id!r}: {sorted(units)}"
            )
        unit = variant_metrics[0]["unit"]
        denominator = len(variant_metrics)
        numerator = sum(1 for m in variant_metrics if m["value"] >= 1.0)
        rate = numerator / denominator if denominator > 0 else 0.0
        results.append(VariantMetricAggregate(
            variant_id=variant_id,
            numerator=numerator,
            denominator=denominator,
            rate=rate,
            unit=unit,
        ))

    return results


_RESOURCE_MEASUREMENT_FIELDS: tuple[str, ...] = (
    "model_load_time_seconds",
    "peak_resident_memory_bytes",
    "peak_accelerator_memory_bytes",
    "artifact_bytes",
    "measured_energy_joules",
    "end_to_end_latency_seconds",
    "provider_call_latency_seconds",
    "output_throughput_tokens_per_second",
    "hidden_reasoning_throughput_tokens_per_second",
    "time_to_first_token_seconds",
    "generation_duration_seconds",
    "accelerator_memory_before_bytes",
    "gpu_utilization_percent",
    "gpu_temperature_celsius",
    "gpu_power_draw_watts",
    "gpu_clock_mhz",
)


class ResourceObservation(BaseModel):
    """Typed resource observation bound to campaign, report, assignment, attempt, inference, stage, role, and task.

    One resource observation per actual provider inference. A g8ee
    scenario that invokes several role models emits several records
    for one assignment attempt. Direct inference emits one record per
    provider call.

    Every observation binds campaign ID, child/report ID, assignment ID,
    attempt ID, inference ID, stage ID, role, model variant ID, canonical
    task ID, orchestrator scope, provider scope, observation boundary,
    clock domain, collection tool, and indexed source evidence. The
    orchestrator and provider environment scopes are separate identities;
    remote provider hardware may be attested or unavailable, never copied
    from the local CPU identity.

    The ``stage_id`` field binds the observation to the specific
    ``StageObservation`` that produced the inference, enabling exact
    inference-trail count verification and stage-level joining.

    Source evidence is indexed: ``source_evidence_refs`` lists the
    evidence artifact IDs, ``source_evidence_sha256`` is the SHA-256 of
    the source evidence, and ``verification_status`` tracks whether the
    observation has been independently verified. A VERIFIED observation
    without source evidence references or a source evidence hash is not
    independently verifiable and is rejected.

    All numeric measurement fields are optional. ``None`` means
    unavailable. Zero (``0`` or ``0.0``) is always a measured zero.
    The ``unavailable_measurements`` list provides typed availability,
    scope, and reason for every ``None`` field. Every ``None`` field
    must have a matching ``unavailable_measurements`` entry; every
    entry must reference a ``None`` field.

    Accelerator memory is a measured value, never inferred from parameter
    count or quantization labels. Cold-start (model load) is separated
    from warm inference (``time_to_first_token_seconds`` and
    ``generation_duration_seconds``) so the load-vs-inference tradeoff
    is visible. GPU metrics (utilization, temperature, power draw,
    clock) are captured via ``pynvml`` when an NVIDIA GPU is present and
    are ``None`` otherwise.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    # Identity bindings
    campaign_id: str = Field(min_length=1, description="Parent campaign ID.")
    child_id: str = Field(min_length=1, description="Child campaign ID (for campaign sets; equals campaign_id for single campaigns).")
    run_id: str = Field(min_length=1, description="Report/run ID.")
    assignment_id: str = Field(min_length=1, description="Campaign assignment ID.")
    attempt_id: str = Field(min_length=1, description="Attempt ID within the assignment.")
    inference_id: str = Field(min_length=1, description="Unique inference ID within the attempt. One observation per inference.")
    stage_id: str = Field(min_length=1, description="Stage ID from StageObservation that produced this inference. Binds the observation to the specific stage for inference-trail verification.")
    role: ModelRole = Field(description="Model role: primary, assistant, or lite.")
    model_variant_id: str = Field(min_length=1, description="Model variant ID.")
    task_id: str = Field(min_length=1, description="Canonical task ID from the assignment.")

    # Environment scopes (separate orchestrator and provider)
    orchestrator_scope: str = Field(min_length=1, description="Orchestrator hardware scope (e.g. linux/amd64/cpu).")
    provider_scope: str = Field(min_length=1, description="Provider hardware scope (e.g. linux/amd64/rtx-4090 or unavailable).")
    observation_boundary: str = Field(min_length=1, description="What the observation covers (e.g. provider_call, model_load, task_execution).")
    clock_domain: str = Field(min_length=1, description="Clock domain (e.g. monotonic, wall_clock).")

    # Collection metadata
    collection_tool: str = Field(min_length=1, description="Collection tool and version (e.g. psutil-5.9).")

    # Indexed source evidence
    source_evidence_refs: list[str] = Field(default_factory=list, description="Indexed source evidence artifact IDs for independent verification.")
    source_evidence_sha256: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$", description="SHA-256 of the source evidence. Required when verification_status is VERIFIED.")
    verification_status: VerificationStatus = Field(default=VerificationStatus.PENDING, description="Verification status of this observation.")

    # Measurements (all optional — None means unavailable, zero is measured)
    model_load_time_seconds: float | None = Field(default=None, ge=0.0, description="Model load time in seconds (cold start). None when unavailable.")
    peak_resident_memory_bytes: int | None = Field(default=None, ge=0, description="Peak resident memory in bytes. None when unavailable.")
    peak_accelerator_memory_bytes: int | None = Field(default=None, ge=0, description="Peak accelerator memory in bytes. None when unavailable.")
    artifact_bytes: int | None = Field(default=None, ge=0, description="Artifact file size in bytes. None when unavailable.")
    measured_energy_joules: float | None = Field(default=None, ge=0.0, description="Measured energy in joules. None when unavailable.")
    end_to_end_latency_seconds: float | None = Field(default=None, gt=0.0, description="End-to-end task latency in seconds. None when unavailable.")
    provider_call_latency_seconds: float | None = Field(default=None, gt=0.0, description="Provider-call latency in seconds. None when unavailable.")
    output_throughput_tokens_per_second: float | None = Field(default=None, ge=0.0, description="Output throughput in tokens per second. None when unavailable.")
    hidden_reasoning_throughput_tokens_per_second: float | None = Field(default=None, ge=0.0, description="Hidden reasoning throughput in tokens per second. None when unavailable.")
    time_to_first_token_seconds: float | None = Field(default=None, ge=0.0, description="Time to first token in seconds (warm inference start). None when unavailable.")
    generation_duration_seconds: float | None = Field(default=None, ge=0.0, description="Generation duration in seconds (warm inference duration). None when unavailable.")
    accelerator_memory_before_bytes: int | None = Field(default=None, ge=0, description="Accelerator memory baseline before inference in bytes. None when unavailable.")
    gpu_utilization_percent: float | None = Field(default=None, ge=0.0, description="GPU utilization in percent. None when unavailable.")
    gpu_temperature_celsius: float | None = Field(default=None, description="GPU temperature in degrees Celsius. None when unavailable.")
    gpu_power_draw_watts: float | None = Field(default=None, ge=0.0, description="GPU power draw in watts. None when unavailable.")
    gpu_clock_mhz: float | None = Field(default=None, ge=0.0, description="GPU clock in MHz. None when unavailable.")

    # Typed availability for unavailable measurements
    unavailable_measurements: list[UnavailableMeasurement] = Field(
        default_factory=list,
        description="Typed availability, scope, and reason for every None measurement field.",
    )

    @model_validator(mode="after")
    def _validate_unavailable_consistency(self) -> Self:
        unavailable_fields: set[str] = set()
        for um in self.unavailable_measurements:
            if um.field_name not in _RESOURCE_MEASUREMENT_FIELDS:
                raise ValueError(
                    f"unavailable_measurements references unknown field {um.field_name!r}; "
                    f"valid fields: {sorted(_RESOURCE_MEASUREMENT_FIELDS)}"
                )
            if um.field_name in unavailable_fields:
                raise ValueError(
                    f"duplicate unavailable_measurement entry for field {um.field_name!r}"
                )
            unavailable_fields.add(um.field_name)

        none_fields: set[str] = set()
        for field_name in _RESOURCE_MEASUREMENT_FIELDS:
            value = getattr(self, field_name)
            if value is None:
                none_fields.add(field_name)

        missing_explanations = none_fields - unavailable_fields
        if missing_explanations:
            raise ValueError(
                f"None measurement fields without unavailable_measurements entries: "
                f"{sorted(missing_explanations)}"
            )

        extra_explanations = unavailable_fields - none_fields
        if extra_explanations:
            raise ValueError(
                f"unavailable_measurements entries for non-None fields: "
                f"{sorted(extra_explanations)}"
            )
        return self

    @model_validator(mode="after")
    def _validate_source_evidence_binding(self) -> Self:
        if len(self.source_evidence_refs) != len(set(self.source_evidence_refs)):
            raise ValueError(
                "resource observation source evidence references must be unique"
            )
        if self.verification_status == VerificationStatus.VERIFIED and (
            not self.source_evidence_refs or self.source_evidence_sha256 is None
        ):
            raise ValueError(
                "verified resource observation requires source evidence references and hash"
            )
        return self


class ResourceObserverContract(BaseModel):
    """Frozen typed contract for the external resource observer.

    Defines the observed process/container/device scope, baseline
    subtraction policy, sampling interval, synchronization boundaries,
    observer clock domain, multi-tenant exclusion rule, and
    unsupported-platform behavior. Every resource observation must
    comply with this contract.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    process_scope: str = Field(
        min_length=1,
        description="Observed process/container/device scope (e.g. single_process, container, device).",
    )
    baseline_subtraction_policy: str = Field(
        min_length=1,
        description="Baseline subtraction policy (e.g. idle_baseline, no_subtraction).",
    )
    sampling_interval_seconds: float = Field(
        gt=0.0,
        description="Sampling interval in seconds.",
    )
    synchronization_boundary: str = Field(
        min_length=1,
        description="Synchronization boundary (e.g. provider_call, task_start_to_end).",
    )
    observer_clock_domain: str = Field(
        min_length=1,
        description="Observer clock domain (e.g. monotonic, wall_clock).",
    )
    multi_tenant_exclusion_rule: str = Field(
        min_length=1,
        description="Multi-tenant exclusion rule (e.g. exclusive_access, cgroup_isolation).",
    )
    unsupported_platform_behavior: str = Field(
        min_length=1,
        description="Behavior when the platform does not support observation (e.g. skip_observation, fail_closed).",
    )


def validate_resource_observations(
    observations: list[ResourceObservation],
    *,
    contract: ResourceObserverContract | None = None,
) -> None:
    """Validate a list of resource observations.

    Rejects duplicate ``(campaign_id, child_id, run_id, assignment_id,
    attempt_id, inference_id)`` tuples. One observation per inference is
    allowed; multiple observations per attempt are allowed when they
    represent distinct inferences. Each observation must be a valid
    ``ResourceObservation`` with all required bindings. When a
    ``contract`` is provided, the observations are validated against
    the contract's scope and policy.
    """
    seen: set[tuple[str, str, str, str, str, str]] = set()
    for obs in observations:
        key = (obs.campaign_id, obs.child_id, obs.run_id, obs.assignment_id, obs.attempt_id, obs.inference_id)
        if key in seen:
            raise ValueError(
                f"duplicate resource observation for (campaign={obs.campaign_id!r}, "
                f"child={obs.child_id!r}, run={obs.run_id!r}, "
                f"assignment={obs.assignment_id!r}, attempt={obs.attempt_id!r}, "
                f"inference={obs.inference_id!r})"
            )
        seen.add(key)


class CampaignVerificationReport(BaseModel):
    """Typed canonical verification report from the campaign verifier.

    Records the verification status, campaign identity, verified index
    generation hash, checked layers, and typed failures. A campaign
    with missing or inconsistent cells cannot produce a passing
    publication candidate.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    verification_schema_version: str = Field(min_length=1, description="Schema version of the verification report.")
    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    ok: bool = Field(description="True when all verification layers pass.")
    verified_index_generation_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the verified index generation's content hash.",
    )
    checked_layers: list[str] = Field(
        default_factory=list,
        description="Sorted list of verification layer names that were checked.",
    )
    failures: list[str] = Field(
        default_factory=list,
        description="Sorted list of typed verification failure messages.",
    )


__all__ = [
    "INDEX_GENERATION_SCHEMA_VERSION",
    "AssignmentDisposition",
    "AssignmentDispositionEntry",
    "CampaignVerificationReport",
    "IndexCreationReason",
    "IndexGeneration",
    "MeasurementAvailability",
    "MeasurementScope",
    "ModelRole",
    "ResourceObservation",
    "ResourceObserverContract",
    "SupersessionPolicy",
    "TierObservationRecord",
    "UnavailableMeasurement",
    "VariantMetricAggregate",
    "VerificationStatus",
    "aggregate_metrics_by_variant",
    "compute_index_generation_hash",
    "validate_index_chain",
    "validate_no_duplicate_effective_assignments",
    "validate_observed_tier_binding",
    "validate_resource_observations",
    "validate_supersession_policy",
]
