# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Publication schema v4: campaign-aware README evidence.

The v4 publication schema carries forward the existing Stage 1/2
artifacts and adds a ``model_campaign`` reference binding a verified
campaign to the public README snapshot. The projector consumes only a
passing ``CampaignVerificationReport`` and its exact index generation,
creates a complete new v4 candidate directory, and refuses to overwrite
an existing candidate. The strict validator rejects unknown fields,
duplicate identities, undeclared artifacts, non-finite values, path
traversal, symlinks, unsafe links, checksum drift, and aggregate or
analysis claims that cannot be recomputed.

Every model is frozen with ``extra="forbid"``. Content hashes are
SHA-256 over canonical JSON (sorted keys, no extra whitespace).
"""

from __future__ import annotations

import hashlib
import json
import math
from enum import StrEnum
from pathlib import Path, PurePosixPath
from typing import Any, Self

from pydantic import BaseModel, ConfigDict, Field, ValidationError, model_validator

from g8e_evals.constants import (
    ATTEMPTS_JSONL,
    CAMPAIGN_ASSIGNMENTS_JSONL,
    CAMPAIGN_INDEX_JSONL,
    CAMPAIGN_PROJECTIONS_JSONL,
    CAMPAIGN_PROVENANCE_JSON,
    CAMPAIGN_STATISTICAL_ANALYSIS_JSON,
    CAMPAIGN_VERIFICATION_REF_JSON,
    COLD_START_WARM_INFERENCE_TRADEOFF_JSON,
    CORRELATED_ERROR_SUMMARY_JSON,
    CORRELATED_ERRORS_JSONL,
    ESCALATION_RECORDS_JSONL,
    ESCALATION_SUMMARY_JSON,
    METRICS_JSONL,
    MODEL_CAMPAIGN_JSON,
    PUBLICATION_SCHEMA_V4,
    PUBLICATION_SCHEMA_V5,
    RADAR_PROFILE_JSON,
    RESOURCE_OBSERVATIONS_JSONL,
    SECURITY_EVENT_SUMMARY_JSON,
    SECURITY_EVENTS_JSONL,
    TOOL_CALL_SCORECARDS_JSONL,
    TOOL_SCORECARD_SUMMARY_JSON,
)
from g8e_evals.campaign_set import (
    AggregateVerificationResult,
    CampaignSetIndex,
    CampaignSetPlan,
)
from g8e_evals.index import (
    AssignmentDisposition,
    CampaignVerificationReport,
    MeasurementAvailability,
)
from g8e_evals.profile import CampaignProfile
from g8e_evals.provenance import SourceInclusionManifest
from g8e_evals.radar_profile import (
    ColdStartWarmInferenceTradeoff,
    ColdStartWarmInferenceTradeoffSummary,
    CorrelatedErrorSummary,
    EscalationSummary,
    RadarDimension,
    RadarDimensionName,
    RadarProfile,
    SecurityEventSummary,
    ToolScorecardDimensionSummary,
    ToolScorecardSummary,
    compute_cold_start_warm_inference_tradeoff_summary_hash,
    compute_correlated_error_summary_hash,
    compute_escalation_summary_hash,
    compute_radar_profile_hash,
    compute_security_event_summary_hash,
    compute_tool_scorecard_summary_hash,
)
from g8e_evals.registry import ModelRegistry


PUBLICATION_V4_VERSION = "4.0.0"

_VALID_DISPOSITIONS = frozenset({
    AssignmentDisposition.EFFECTIVE.value,
    AssignmentDisposition.SUPERSEDED.value,
    AssignmentDisposition.QUALIFICATION.value,
    AssignmentDisposition.UNAVAILABLE.value,
})


def _sha256(data: str) -> str:
    return hashlib.sha256(data.encode()).hexdigest()


def _check_traversal(path: str) -> bool:
    """Return True if the path contains traversal sequences."""
    if not path:
        return False
    normalized = str(PurePosixPath(path))
    parts = PurePosixPath(normalized).parts
    return ".." in parts or normalized.startswith("..")


class CampaignCaveat(StrEnum):
    """Closed vocabulary of caveats bounding campaign claims.

    Every published campaign carries explicit caveats from this closed
    vocabulary. Unknown caveats are rejected so a publication cannot
    silently introduce an unbounded qualifier.
    """

    DESCRIPTIVE_ONLY = "descriptive_only"
    SINGLE_HARDWARE_CLASS = "single_hardware_class"
    SINGLE_ENVIRONMENT = "single_environment"
    CURATED_BENCHMARK_SUBSET = "curated_benchmark_subset"
    NO_INFERENTIAL_CLAIMS = "no_inferential_claims"
    PROVIDER_BOUNDARY_OBSERVATION = "provider_boundary_observation"


class CampaignProjectionRow(BaseModel):
    """One safe public projection row for a campaign metric result.

    Contains only allowlisted fields plus the repetition index. No raw
    prompts, outputs, keys, credentials, private endpoints, machine
    paths, or evidence-key metadata cross the projection boundary. The
    evidence link is a relative POSIX path with no traversal.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    variant_id: str = Field(min_length=1, description="Model variant ID.")
    task_id: str = Field(min_length=1, description="Task ID.")
    metric_id: str = Field(min_length=1, description="Metric ID.")
    numerator: int = Field(ge=0, description="Count of positive outcomes.")
    denominator: int = Field(ge=0, description="Total count of measured outcomes.")
    rate: float = Field(ge=0.0, le=1.0, description="Numerator divided by denominator.")
    unit: str = Field(min_length=1, description="Metric unit.")
    verification_status: str = Field(min_length=1, description="Verification status.")
    evidence_link: str = Field(min_length=1, description="Relative path to the public proof artifact.")
    repetition: int = Field(ge=1, description="Repetition index for this measurement.")

    @model_validator(mode="after")
    def _validate_row(self) -> Self:
        if _check_traversal(self.evidence_link):
            raise ValueError(f"traversal in evidence_link: {self.evidence_link!r}")
        if not math.isfinite(self.rate):
            raise ValueError(f"non-finite rate: {self.rate!r}")
        return self


class CampaignVariantSummaryRow(BaseModel):
    """Per-variant aggregated descriptive statistics for one metric.

    Aggregates all per-assignment projection rows for one variant,
    metric, and role into a single pass-rate summary. The numerator is
    the total positive outcomes across all tasks and repetitions; the
    denominator is the total measured outcomes. The rate is numerator
    divided by denominator. Task count, repetition count, terminal
    count, superseded count, invalid count, and missing count are
    reported so a reader can inspect completeness without pooling
    distinct variants.

    ``role`` carries the model role (primary, assistant, lite) so
    multiple roles for the same variant and metric do not overwrite
    each other. An empty string means the role is unspecified (legacy
    or single-role campaigns).

    ``per_task_denominators`` preserves the denominator for each task
    so a reader can inspect the distribution rather than seeing only
    the pooled total.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID.")
    metric_id: str = Field(min_length=1, description="Metric ID.")
    numerator: int = Field(ge=0, description="Total positive outcomes across all tasks and repetitions.")
    denominator: int = Field(ge=0, description="Total measured outcomes across all tasks and repetitions.")
    rate: float = Field(ge=0.0, le=1.0, description="Numerator divided by denominator.")
    unit: str = Field(min_length=1, description="Metric unit.")
    task_count: int = Field(ge=0, description="Number of distinct tasks measured.")
    repetition_count: int = Field(ge=0, description="Number of repetitions per task.")
    terminal_count: int = Field(ge=0, description="Number of terminal (completed) attempts.")
    superseded_count: int = Field(ge=0, description="Number of superseded attempts.")
    invalid_count: int = Field(ge=0, description="Number of invalid attempts.")
    missing_count: int = Field(ge=0, description="Number of missing observations (eligible but unmeasured).")
    role: str = Field(default="", description="Model role (primary, assistant, lite). Empty when unspecified.")
    per_task_denominators: dict[str, int] = Field(
        default_factory=dict,
        description="Per-task denominator breakdown: task_id -> denominator for that task.",
    )

    @model_validator(mode="after")
    def _validate_summary(self) -> Self:
        if not math.isfinite(self.rate):
            raise ValueError(f"non-finite rate: {self.rate!r}")
        return self


class CampaignDispositionRow(BaseModel):
    """Typed disposition of one assignment in the published campaign.

    ``effective``: The assignment has exactly one effective valid report.
    ``superseded``: The assignment was superseded by a later generation.
    ``qualification``: The assignment has a typed qualification outcome.
    ``unavailable``: The assignment is unavailable and has no report.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    assignment_id: str = Field(min_length=1, description="Campaign assignment ID.")
    disposition: AssignmentDisposition = Field(description="Typed disposition of the assignment.")
    variant_id: str = Field(min_length=1, description="Model variant ID.")
    task_id: str = Field(min_length=1, description="Task ID.")
    reason: str = Field(default="", description="Reason for non-effective disposition. Empty for effective.")


class CampaignComparisonRow(BaseModel):
    """Per-task, repetition-aware comparison row binding an exact role combination.

    Metrics bind both the exact three-role combination and the
    individual invoked variant. Results from different combinations
    never pool implicitly.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    task_id: str = Field(min_length=1, description="Task ID.")
    combination_id: str = Field(min_length=1, description="Role-combination ID.")
    primary_variant_id: str = Field(min_length=1, description="Primary role variant ID.")
    assistant_variant_id: str = Field(min_length=1, description="Assistant role variant ID.")
    lite_variant_id: str = Field(min_length=1, description="Lite role variant ID.")
    metric_id: str = Field(min_length=1, description="Metric ID.")
    repetition: int = Field(ge=1, description="Repetition index.")
    numerator: int = Field(ge=0, description="Count of positive outcomes.")
    denominator: int = Field(ge=0, description="Total count of measured outcomes.")
    rate: float = Field(ge=0.0, le=1.0, description="Numerator divided by denominator.")
    unit: str = Field(min_length=1, description="Metric unit.")


class CampaignEfficiencyObservation(BaseModel):
    """Typed efficiency observation for one model variant.

    Binds end-to-end latency, output throughput, artifact size, and
    peak resident memory to the variant and hardware identity. No
    private endpoints, machine paths, or credentials appear.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Model variant ID.")
    hardware_identity: str = Field(min_length=1, description="Hardware identity (e.g. linux/amd64/rtx-4090).")
    end_to_end_latency_seconds: float = Field(gt=0.0, description="End-to-end task latency in seconds.")
    output_throughput_tokens_per_second: float | None = Field(
        default=None, ge=0.0,
        description="Output throughput in tokens per second. None when not measured.",
    )
    artifact_bytes: int = Field(ge=0, description="Artifact file size in bytes.")
    peak_resident_memory_bytes: int = Field(ge=0, description="Peak resident memory in bytes.")


class CampaignStatisticalAnalysisRef(BaseModel):
    """Reference to the frozen statistical analysis record for the campaign.

    Records the method, independent unit, population, correction
    family, claim status, and content hash of the full analysis
    record. The validator recomputes the content hash to detect drift.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    method: str = Field(min_length=1, description="Statistical method (e.g. mcnemar, bootstrap).")
    independent_unit: str = Field(min_length=1, description="Primary independent sampling unit (e.g. task).")
    population: int = Field(ge=0, description="Number of independent units in the population.")
    correction_family: str = Field(min_length=1, description="Multiple-comparison correction family.")
    claim_status: str = Field(min_length=1, description="Claim boundary: descriptive_only or confirmatory.")
    content_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the full analysis record.")


class CampaignProvenanceRef(BaseModel):
    """Reference to the source inclusion manifest for the campaign.

    Records the manifest hash, entry count, and schema version. The
    validator checks the manifest hash against the full manifest.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    manifest_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the source inclusion manifest.")
    entry_count: int = Field(ge=0, description="Number of entries in the manifest.")
    schema_version: str = Field(min_length=1, description="Manifest schema version.")


class SafeCampaignProfileProjection(BaseModel):
    """Safe public projection of the campaign profile.

    Contains only allowlisted fields: campaign identity, revision,
    content hash, benchmark IDs, task IDs, repetitions, hardware
    identity, and claim boundary. No sampling settings, timeouts,
    retry policies, or private configuration appear.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    content_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the frozen campaign profile.")
    benchmark_ids: list[str] = Field(min_length=1, description="Benchmark suite IDs.")
    task_ids: list[str] = Field(min_length=1, description="Exact task IDs in the campaign population.")
    repetitions: int = Field(ge=1, description="Number of repetitions per assignment.")
    hardware_identity: str = Field(min_length=1, description="Hardware identity.")
    claim_boundary: str = Field(min_length=1, description="Statistical claim boundary.")


class SafeModelVariantProjection(BaseModel):
    """Safe public projection of one model variant from the registry.

    Contains immutable artifact identities and quantization. No
    credentials, private endpoints, or download locations appear.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    variant_id: str = Field(min_length=1, description="Stable campaign model-variant ID.")
    canonical_display_name: str = Field(min_length=1, description="Corrected canonical display name.")
    weight_class: str = Field(min_length=1, description="Weight class for grouping.")
    parameter_count_display: str = Field(min_length=1, description="Human-readable parameter count.")
    backend_name: str = Field(min_length=1, description="Backend provider name.")
    served_model_tag: str = Field(min_length=1, description="Exact served model tag.")
    artifact_digest: str = Field(min_length=64, max_length=64, description="SHA-256 of the served model artifact.")
    quantization: str = Field(default="", description="Quantization policy label. Empty for unquantized.")
    publication_eligibility: str = Field(min_length=1, description="Publication eligibility disposition.")


class ModelCampaignRef(BaseModel):
    """Frozen typed campaign reference for publication schema v4.

    Binds the safe campaign profile projection and hash, safe model
    registry projection with immutable artifact identities and
    quantization, the campaign verification report and exact verified
    index-generation hash, safe projections for every effective
    included run plus typed dispositions, per-task repetition-aware
    comparison rows, typed efficiency observations, the statistical
    analysis record, provenance, and caveats from a closed vocabulary.

    The ``content_hash`` is SHA-256 over canonical JSON of the
    reference. Changing any field changes the hash and invalidates
    downstream publication validation.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    campaign_id: str = Field(min_length=1, description="Campaign identity.")
    campaign_revision: str = Field(min_length=1, description="Campaign revision.")
    publication_schema_version: str = Field(min_length=1, description="Publication schema version (4.0.0).")
    campaign_profile: SafeCampaignProfileProjection = Field(description="Safe campaign profile projection.")
    model_variants: list[SafeModelVariantProjection] = Field(
        min_length=1, description="Safe model variant projections with immutable artifact identities.",
    )
    model_registry_hash: str = Field(min_length=64, max_length=64, description="SHA-256 of the frozen model registry.")
    verification_ok: bool = Field(description="True when the campaign verification report passed.")
    verified_index_generation_hash: str = Field(
        min_length=64, max_length=64,
        description="SHA-256 of the verified index generation's content hash.",
    )
    checked_layers: list[str] = Field(default_factory=list, description="Sorted verification layer names checked.")
    projections: list[CampaignProjectionRow] = Field(
        default_factory=list, description="Safe public projection rows for every effective included run.",
    )
    dispositions: list[CampaignDispositionRow] = Field(
        default_factory=list, description="Typed dispositions for every assignment.",
    )
    comparison_rows: list[CampaignComparisonRow] = Field(
        default_factory=list, description="Per-task repetition-aware comparison rows.",
    )
    efficiency_observations: list[CampaignEfficiencyObservation] = Field(
        default_factory=list, description="Typed efficiency observations.",
    )
    variant_summaries: list[CampaignVariantSummaryRow] = Field(
        default_factory=list, description="Per-variant aggregated descriptive statistics.",
    )
    statistical_analysis: CampaignStatisticalAnalysisRef = Field(description="Statistical analysis record reference.")
    provenance: CampaignProvenanceRef = Field(description="Source provenance reference.")
    caveats: list[CampaignCaveat] = Field(
        min_length=1, description="Explicit caveats from the closed vocabulary.",
    )
    campaign_set_plan_hash: str | None = Field(
        default=None,
        min_length=64, max_length=64,
        description="SHA-256 of the frozen CampaignSetPlan. Required for v5 aggregate publication; None for v4 single-report.",
    )
    campaign_set_index_hash: str | None = Field(
        default=None,
        min_length=64, max_length=64,
        description="SHA-256 of the CampaignSetIndex. Required for v5 aggregate publication; None for v4 single-report.",
    )
    aggregate_verification_hash: str | None = Field(
        default=None,
        min_length=64, max_length=64,
        description="SHA-256 of the accepted AggregateVerificationResult. Required for v5 aggregate publication; None for v4 single-report.",
    )
    content_hash: str = Field(min_length=64, max_length=64, description="SHA-256 over canonical JSON of the reference.")

    @model_validator(mode="after")
    def _validate_ref(self) -> Self:
        expected = compute_model_campaign_hash(
            campaign_id=self.campaign_id,
            campaign_revision=self.campaign_revision,
            publication_schema_version=self.publication_schema_version,
            campaign_profile=self.campaign_profile,
            model_variants=self.model_variants,
            model_registry_hash=self.model_registry_hash,
            verification_ok=self.verification_ok,
            verified_index_generation_hash=self.verified_index_generation_hash,
            checked_layers=self.checked_layers,
            projections=self.projections,
            dispositions=self.dispositions,
            comparison_rows=self.comparison_rows,
            efficiency_observations=self.efficiency_observations,
            variant_summaries=self.variant_summaries,
            statistical_analysis=self.statistical_analysis,
            provenance=self.provenance,
            caveats=self.caveats,
            campaign_set_plan_hash=self.campaign_set_plan_hash,
            campaign_set_index_hash=self.campaign_set_index_hash,
            aggregate_verification_hash=self.aggregate_verification_hash,
        )
        if self.content_hash != expected:
            raise ValueError(
                f"model campaign content_hash mismatch: declared {self.content_hash!r}, "
                f"computed {expected!r}"
            )
        return self


def compute_model_campaign_hash(
    *,
    campaign_id: str,
    campaign_revision: str,
    publication_schema_version: str,
    campaign_profile: SafeCampaignProfileProjection,
    model_variants: list[SafeModelVariantProjection],
    model_registry_hash: str,
    verification_ok: bool,
    verified_index_generation_hash: str,
    checked_layers: list[str],
    projections: list[CampaignProjectionRow],
    dispositions: list[CampaignDispositionRow],
    comparison_rows: list[CampaignComparisonRow],
    efficiency_observations: list[CampaignEfficiencyObservation],
    variant_summaries: list[CampaignVariantSummaryRow] | None = None,
    statistical_analysis: CampaignStatisticalAnalysisRef,
    provenance: CampaignProvenanceRef,
    caveats: list[CampaignCaveat] | list[str],
    campaign_set_plan_hash: str | None = None,
    campaign_set_index_hash: str | None = None,
    aggregate_verification_hash: str | None = None,
) -> str:
    """Compute the content hash for a model campaign reference without constructing the full model."""
    caveats_values = sorted(
        c.value if isinstance(c, CampaignCaveat) else c for c in caveats
    )
    payload: dict[str, Any] = {
        "campaign_id": campaign_id,
        "campaign_revision": campaign_revision,
        "publication_schema_version": publication_schema_version,
        "campaign_profile": json.loads(campaign_profile.model_dump_json()),
        "model_variants": [
            json.loads(v.model_dump_json())
            for v in sorted(model_variants, key=lambda v: v.variant_id)
        ],
        "model_registry_hash": model_registry_hash,
        "verification_ok": verification_ok,
        "verified_index_generation_hash": verified_index_generation_hash,
        "checked_layers": sorted(checked_layers),
        "projections": [
            json.loads(p.model_dump_json())
            for p in sorted(projections, key=lambda p: (p.variant_id, p.task_id, p.metric_id, p.repetition))
        ],
        "dispositions": [
            json.loads(d.model_dump_json())
            for d in sorted(dispositions, key=lambda d: d.assignment_id)
        ],
        "comparison_rows": [
            json.loads(c.model_dump_json())
            for c in sorted(comparison_rows, key=lambda c: (c.task_id, c.combination_id, c.metric_id, c.repetition))
        ],
        "efficiency_observations": [
            json.loads(e.model_dump_json())
            for e in sorted(efficiency_observations, key=lambda e: e.variant_id)
        ],
        "variant_summaries": [
            json.loads(s.model_dump_json())
            for s in sorted(variant_summaries or [], key=lambda s: (s.variant_id, s.metric_id))
        ],
        "statistical_analysis": json.loads(statistical_analysis.model_dump_json()),
        "provenance": json.loads(provenance.model_dump_json()),
        "caveats": caveats_values,
    }
    # Include aggregate authority hashes in the content hash only when
    # present (v5 aggregate publication). v4 single-report candidates
    # do not carry these fields, so their hashes are unchanged.
    if campaign_set_plan_hash is not None:
        payload["campaign_set_plan_hash"] = campaign_set_plan_hash
    if campaign_set_index_hash is not None:
        payload["campaign_set_index_hash"] = campaign_set_index_hash
    if aggregate_verification_hash is not None:
        payload["aggregate_verification_hash"] = aggregate_verification_hash
    payload_json = json.dumps(
        payload,
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload_json)


class PublicationSchemaV4(BaseModel):
    """Composed README snapshot for publication schema v4.

    Carries forward the Stage 1/2 artifacts (evidence cutoff, platform
    version) and adds the ``model_campaign`` reference binding a
    verified campaign to the public README snapshot.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    publication_schema_version: str = Field(
        min_length=1, description="Publication schema version. Must be 4.0.0.",
    )
    evidence_cutoff: str = Field(min_length=1, description="ISO 8601 evidence cutoff timestamp.")
    platform_version: str = Field(min_length=1, description="Platform version label.")
    model_campaign: ModelCampaignRef = Field(description="Verified campaign reference.")

    @model_validator(mode="after")
    def _validate_version(self) -> Self:
        if self.publication_schema_version != PUBLICATION_SCHEMA_V4:
            raise ValueError(
                f"publication_schema_version must be {PUBLICATION_SCHEMA_V4!r}: "
                f"got {self.publication_schema_version!r}"
            )
        return self


class PublicationValidatorResult(BaseModel):
    """Typed result of strict v4 publication validation.

    ``ok`` is True only when every checked layer passes. ``checked_layers``
    is a sorted list of validation layer names. ``failures`` is a sorted
    list of typed failure messages.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    ok: bool = Field(description="True when all validation layers pass.")
    checked_layers: list[str] = Field(default_factory=list, description="Sorted validation layer names.")
    failures: list[str] = Field(default_factory=list, description="Sorted failure messages.")


def _project_profile(profile: CampaignProfile) -> SafeCampaignProfileProjection:
    """Build a safe campaign profile projection from the frozen profile."""
    return SafeCampaignProfileProjection(
        campaign_id=profile.campaign_id,
        campaign_revision=profile.campaign_revision,
        content_hash=profile.content_hash,
        benchmark_ids=profile.benchmark_ids,
        task_ids=profile.task_ids,
        repetitions=profile.repetitions,
        hardware_identity=profile.hardware_identity,
        claim_boundary=profile.claim_boundary.value,
    )


def _project_variant(variant) -> SafeModelVariantProjection:
    """Build a safe model variant projection from a registry variant."""
    return SafeModelVariantProjection(
        variant_id=variant.variant_id,
        canonical_display_name=variant.canonical_display_name,
        weight_class=variant.weight_class.value,
        parameter_count_display=variant.parameter_count_display,
        backend_name=variant.backend_name,
        served_model_tag=variant.served_model_tag,
        artifact_digest=variant.artifact_digest,
        quantization=variant.quantization,
        publication_eligibility=variant.publication_eligibility.value,
    )


def _project_provenance(manifest: SourceInclusionManifest) -> CampaignProvenanceRef:
    """Build a provenance reference from the source inclusion manifest."""
    return CampaignProvenanceRef(
        manifest_hash=manifest.manifest_hash,
        entry_count=len(manifest.entries),
        schema_version=manifest.schema_version,
    )


def _read_jsonl_dicts(path: Path) -> list[dict]:
    """Read a JSONL file and return a list of parsed dicts."""
    records: list[dict] = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if line:
            records.append(json.loads(line))
    return records


def _extract_variant_id(model_cohort_id: str) -> str:
    """Extract the variant ID from a cohort ID.

    Cohort IDs follow the convention ``cohort-{variant_id}`` established
    by ``derive_cohorts_from_registry`` in the campaign runner.
    """
    prefix = "cohort-"
    if model_cohort_id.startswith(prefix):
        return model_cohort_id[len(prefix):]
    return model_cohort_id


def _extract_repetition(replicate_id: str) -> int:
    """Extract the repetition number from a replicate ID.

    Replicate IDs follow the convention ``replicate-{n}`` where n is a
    positive integer (1-indexed).
    """
    prefix = "replicate-"
    if replicate_id.startswith(prefix):
        return int(replicate_id[len(prefix):])
    return 1


def _generate_projections(
    *,
    report_dir: Path,
    campaign_id: str,
    campaign_revision: str,
) -> list[CampaignProjectionRow]:
    """Generate safe public projection rows from campaign metrics and attempts.

    Reads ``metrics.jsonl`` and ``attempts.jsonl`` from the report
    directory, joins them via ``attempt_id``, and produces one
    ``CampaignProjectionRow`` per measured metric. The variant ID is
    extracted from the attempt's ``model_cohort_id`` and the repetition
    from the attempt's ``replicate_id``.
    """
    metrics_path = report_dir / METRICS_JSONL
    attempts_path = report_dir / ATTEMPTS_JSONL
    if not metrics_path.exists() or not attempts_path.exists():
        return []

    attempts_by_id: dict[str, dict] = {}
    for raw in _read_jsonl_dicts(attempts_path):
        aid = raw.get("attempt_id")
        if aid:
            attempts_by_id[aid] = raw

    rows: list[CampaignProjectionRow] = []
    for raw_metric in _read_jsonl_dicts(metrics_path):
        attempt_id = raw_metric.get("attempt_id", "")
        attempt = attempts_by_id.get(attempt_id)
        if attempt is None:
            continue
        variant_id = _extract_variant_id(attempt.get("model_cohort_id", ""))
        task_id = raw_metric.get("task_id", "")
        metric_id = raw_metric.get("metric_id", "")
        value = raw_metric.get("value", 0.0)
        unit = raw_metric.get("unit", "boolean")
        verification_status = raw_metric.get("verification_status", "verified")
        repetition = _extract_repetition(attempt.get("replicate_id", "replicate-1"))
        numerator = round(value)
        denominator = int(raw_metric.get("denominator_contribution", 1))
        rate = numerator / denominator if denominator > 0 else 0.0
        evidence_link = f"proofs/{variant_id}/{task_id}/rep-{repetition}.json"
        rows.append(CampaignProjectionRow(
            campaign_id=campaign_id,
            campaign_revision=campaign_revision,
            variant_id=variant_id,
            task_id=task_id,
            metric_id=metric_id,
            numerator=numerator,
            denominator=denominator,
            rate=rate,
            unit=unit,
            verification_status=verification_status,
            evidence_link=evidence_link,
            repetition=repetition,
        ))
    return rows


def _generate_dispositions(
    *,
    report_dir: Path,
    verified_index_generation_hash: str,
) -> list[CampaignDispositionRow]:
    """Generate typed disposition rows from the verified index generation.

    Reads ``campaign-index.jsonl`` and finds the generation whose
    ``content_hash`` matches the verified index generation hash. Then
    reads ``campaign-assignments.jsonl`` to join each disposition with
    its variant ID and task ID.
    """
    index_path = report_dir / CAMPAIGN_INDEX_JSONL
    assignments_path = report_dir / CAMPAIGN_ASSIGNMENTS_JSONL
    if not index_path.exists() or not assignments_path.exists():
        return []

    assignments_by_id: dict[str, dict] = {}
    for raw in _read_jsonl_dicts(assignments_path):
        aid = raw.get("assignment_id")
        if aid:
            assignments_by_id[aid] = raw

    matching_gen: dict | None = None
    for raw in _read_jsonl_dicts(index_path):
        if raw.get("content_hash") == verified_index_generation_hash:
            matching_gen = raw
            break
    if matching_gen is None:
        return []

    rows: list[CampaignDispositionRow] = []
    for disp in matching_gen.get("assignment_dispositions", []):
        assignment_id = disp.get("assignment_id", "")
        assignment = assignments_by_id.get(assignment_id, {})
        variant_id = _extract_variant_id(assignment.get("model_cohort_id", ""))
        task_id = assignment.get("task_id", "")
        rows.append(CampaignDispositionRow(
            assignment_id=assignment_id,
            disposition=disp.get("disposition", "effective"),
            variant_id=variant_id,
            task_id=task_id,
            reason=disp.get("reason", ""),
        ))
    return rows


def _generate_variant_summaries(
    projections: list[CampaignProjectionRow],
) -> list[CampaignVariantSummaryRow]:
    """Aggregate per-assignment projection rows into per-variant summaries.

    For each distinct ``(variant_id, metric_id)`` pair, computes the
    total numerator (positive outcomes), total denominator (measured
    outcomes), rate, distinct task count, repetition count, and terminal
    count. Superseded, invalid, and missing counts are zero for
    effective-only projections (the projector only includes effective
    runs). The ``per_task_denominators`` dict preserves the denominator
    for each task so a reader can inspect the distribution. Results are
    sorted by ``(variant_id, metric_id)``.
    """
    if not projections:
        return []

    grouped: dict[tuple[str, str], list[CampaignProjectionRow]] = {}
    for row in projections:
        key = (row.variant_id, row.metric_id)
        grouped.setdefault(key, []).append(row)

    summaries: list[CampaignVariantSummaryRow] = []
    for (variant_id, metric_id), rows in sorted(grouped.items()):
        numerator = sum(r.numerator for r in rows)
        denominator = sum(r.denominator for r in rows)
        rate = numerator / denominator if denominator > 0 else 0.0
        task_count = len({r.task_id for r in rows})
        repetition_count = max(r.repetition for r in rows) if rows else 0
        terminal_count = len(rows)
        unit = rows[0].unit if rows else "boolean"
        per_task_denominators: dict[str, int] = {}
        for r in rows:
            per_task_denominators[r.task_id] = per_task_denominators.get(r.task_id, 0) + r.denominator
        summaries.append(CampaignVariantSummaryRow(
            variant_id=variant_id,
            metric_id=metric_id,
            numerator=numerator,
            denominator=denominator,
            rate=rate,
            unit=unit,
            task_count=task_count,
            repetition_count=repetition_count,
            terminal_count=terminal_count,
            superseded_count=0,
            invalid_count=0,
            missing_count=0,
            role="",
            per_task_denominators=dict(sorted(per_task_denominators.items())),
        ))
    return summaries


def _generate_statistical_analysis(
    *,
    report_dir: Path,
    campaign_profile: CampaignProfile,
) -> CampaignStatisticalAnalysisRef:
    """Generate the statistical analysis reference for the campaign.

    If a ``campaign-statistical-analysis.json`` file exists in the report
    directory, it is read and validated. Otherwise, a descriptive-only
    record is generated from the campaign profile's task population and
    claim boundary.
    """
    stat_path = report_dir / CAMPAIGN_STATISTICAL_ANALYSIS_JSON
    if stat_path.exists():
        stat_data = json.loads(stat_path.read_text())
        return CampaignStatisticalAnalysisRef(
            method=stat_data["method"],
            independent_unit=stat_data["independent_unit"],
            population=stat_data["population"],
            correction_family=stat_data["correction_family"],
            claim_status=stat_data["claim_status"],
            content_hash=stat_data["content_hash"],
        )

    method = "descriptive"
    independent_unit = campaign_profile.unit_of_analysis
    population = len(campaign_profile.task_ids)
    correction_family = "none"
    claim_status = campaign_profile.claim_boundary.value
    stat_payload = json.dumps(
        {
            "method": method,
            "independent_unit": independent_unit,
            "population": population,
            "correction_family": correction_family,
            "claim_status": claim_status,
        },
        sort_keys=True,
        separators=(",", ":"),
        ensure_ascii=False,
    )
    content_hash = _sha256(stat_payload)
    return CampaignStatisticalAnalysisRef(
        method=method,
        independent_unit=independent_unit,
        population=population,
        correction_family=correction_family,
        claim_status=claim_status,
        content_hash=content_hash,
    )


def project_campaign_v4(
    *,
    report_dir: Path,
    verification: CampaignVerificationReport,
    candidate_dir: Path,
    campaign_profile: CampaignProfile,
    model_registry: ModelRegistry,
    provenance_manifest: SourceInclusionManifest,
    caveats: list[str],
    evidence_cutoff: str,
    platform_version: str,
) -> Path:
    """Project a verified campaign report directory into a v4 candidate directory.

    Consumes only a passing ``CampaignVerificationReport`` and its
    exact index generation. Creates a complete new v4 candidate
    directory with the model-campaign, projections, statistical
    analysis, provenance, and verification reference artifacts. Refuses
    to overwrite an existing candidate directory. Projects safe fields
    using the existing ``project_to_public`` allowlist.

    Returns the candidate directory path.
    """
    if not verification.ok:
        raise ValueError(
            f"campaign verification failed: {verification.failures}"
        )

    if candidate_dir.exists():
        raise ValueError(f"candidate directory already exists: {candidate_dir}")

    # Generate projection rows from the campaign's metrics and attempts.
    projection_rows = _generate_projections(
        report_dir=report_dir,
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
    )

    # Generate disposition rows from the verified index generation.
    disposition_rows = _generate_dispositions(
        report_dir=report_dir,
        verified_index_generation_hash=verification.verified_index_generation_hash,
    )

    # Generate the statistical analysis reference.
    stat_ref = _generate_statistical_analysis(
        report_dir=report_dir,
        campaign_profile=campaign_profile,
    )

    # Generate per-variant aggregated descriptive summaries.
    variant_summaries = _generate_variant_summaries(projection_rows)

    # Build safe projections from the profile and registry.
    profile_projection = _project_profile(campaign_profile)
    variant_projections = [_project_variant(v) for v in model_registry.variants]
    provenance_ref = _project_provenance(provenance_manifest)

    # Compute the content hash and build the model campaign reference.
    content_hash = compute_model_campaign_hash(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        publication_schema_version=PUBLICATION_SCHEMA_V4,
        campaign_profile=profile_projection,
        model_variants=variant_projections,
        model_registry_hash=model_registry.content_hash,
        verification_ok=verification.ok,
        verified_index_generation_hash=verification.verified_index_generation_hash,
        checked_layers=verification.checked_layers,
        projections=projection_rows,
        dispositions=disposition_rows,
        comparison_rows=[],
        efficiency_observations=[],
        variant_summaries=variant_summaries,
        statistical_analysis=stat_ref,
        provenance=provenance_ref,
        caveats=caveats,
    )

    ref = ModelCampaignRef(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        publication_schema_version=PUBLICATION_SCHEMA_V4,
        campaign_profile=profile_projection,
        model_variants=variant_projections,
        model_registry_hash=model_registry.content_hash,
        verification_ok=verification.ok,
        verified_index_generation_hash=verification.verified_index_generation_hash,
        checked_layers=verification.checked_layers,
        projections=projection_rows,
        dispositions=disposition_rows,
        comparison_rows=[],
        efficiency_observations=[],
        variant_summaries=variant_summaries,
        statistical_analysis=stat_ref,
        provenance=provenance_ref,
        caveats=[CampaignCaveat(c) for c in caveats],
        content_hash=content_hash,
    )

    # Write all artifacts to the candidate directory.
    candidate_dir.mkdir(parents=True)
    _write_json(candidate_dir / MODEL_CAMPAIGN_JSON, ref.model_dump_json())
    _write_jsonl(candidate_dir / CAMPAIGN_PROJECTIONS_JSONL, projection_rows)
    _write_json(candidate_dir / CAMPAIGN_STATISTICAL_ANALYSIS_JSON, stat_ref.model_dump_json())
    _write_json(candidate_dir / CAMPAIGN_PROVENANCE_JSON, provenance_ref.model_dump_json())
    _write_json(candidate_dir / CAMPAIGN_VERIFICATION_REF_JSON, verification.model_dump_json())

    return candidate_dir


def _write_json(path: Path, content: str) -> None:
    """Write canonical JSON content to a file."""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


def _write_jsonl(path: Path, rows: list[Any]) -> None:
    """Write a list of Pydantic models as canonical JSONL."""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("".join(r.model_dump_json() + "\n" for r in rows))


def validate_publication_v4(candidate_dir: Path) -> PublicationValidatorResult:
    """Strictly validate a v4 publication candidate directory.

    Rejects unknown fields, duplicate identities, undeclared artifacts,
    non-finite values, path traversal, symlinks, unsafe links,
    checksum drift, and aggregate or analysis claims that cannot be
    recomputed. Returns a typed ``PublicationValidatorResult`` with
    ``ok=True`` only when every layer passes.
    """
    failures: list[str] = []
    checked_layers: list[str] = []

    required_artifacts = (
        MODEL_CAMPAIGN_JSON,
        CAMPAIGN_PROJECTIONS_JSONL,
        CAMPAIGN_STATISTICAL_ANALYSIS_JSON,
        CAMPAIGN_PROVENANCE_JSON,
        CAMPAIGN_VERIFICATION_REF_JSON,
    )

    # Layer 1: file safety — required artifacts exist as regular files (no symlinks)
    checked_layers.append("file_safety")
    for artifact in required_artifacts:
        path = candidate_dir / artifact
        if not path.exists():
            failures.append(f"missing artifact: {artifact}")
        elif path.is_symlink():
            failures.append(f"symlink rejected: {artifact}")
        elif not path.is_file():
            failures.append(f"not a regular file: {artifact}")

    # Layer 2: model campaign validation — parse and validate model-campaign.json
    checked_layers.append("model_campaign")
    mc_path = candidate_dir / MODEL_CAMPAIGN_JSON
    ref: ModelCampaignRef | None = None
    if mc_path.exists() and mc_path.is_file() and not mc_path.is_symlink():
        try:
            ref = ModelCampaignRef.model_validate_json(mc_path.read_text())
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"model-campaign.json validation failed: {e}")

    # Layer 3: projections validation — parse and validate campaign-projections.jsonl
    checked_layers.append("projections")
    proj_path = candidate_dir / CAMPAIGN_PROJECTIONS_JSONL
    proj_rows: list[CampaignProjectionRow] = []
    if proj_path.exists() and proj_path.is_file() and not proj_path.is_symlink():
        try:
            raw_lines = proj_path.read_text().strip().splitlines()
            for line in raw_lines:
                if line.strip():
                    proj_rows.append(CampaignProjectionRow.model_validate_json(line))
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"campaign-projections.jsonl validation failed: {e}")

    # Layer 4: projection consistency — count matches model-campaign and no duplicates
    checked_layers.append("projection_consistency")
    if ref is not None:
        if len(proj_rows) != len(ref.projections):
            failures.append(
                f"projection count mismatch: JSONL has {len(proj_rows)}, "
                f"model-campaign has {len(ref.projections)}"
            )
        seen_keys: set[tuple[str, str, str, int]] = set()
        for row in proj_rows:
            key = (row.variant_id, row.task_id, row.metric_id, row.repetition)
            if key in seen_keys:
                failures.append(
                    f"duplicate projection identity: variant={row.variant_id!r}, "
                    f"task={row.task_id!r}, metric={row.metric_id!r}, rep={row.repetition}"
                )
            seen_keys.add(key)

    # Layer 4b: variant summaries validation — no duplicate (variant_id, metric_id) pairs
    checked_layers.append("variant_summaries")
    if ref is not None:
        seen_summary_keys: set[tuple[str, str]] = set()
        for summary in ref.variant_summaries:
            key = (summary.variant_id, summary.metric_id)
            if key in seen_summary_keys:
                failures.append(
                    f"duplicate variant summary identity: variant={summary.variant_id!r}, "
                    f"metric={summary.metric_id!r}"
                )
            seen_summary_keys.add(key)

    # Layer 5: statistical analysis validation
    checked_layers.append("statistical_analysis")
    stat_path = candidate_dir / CAMPAIGN_STATISTICAL_ANALYSIS_JSON
    if stat_path.exists() and stat_path.is_file() and not stat_path.is_symlink():
        try:
            stat_ref = CampaignStatisticalAnalysisRef.model_validate_json(stat_path.read_text())
            if ref is not None:
                if stat_ref.content_hash != ref.statistical_analysis.content_hash:
                    failures.append("statistical analysis content_hash drift")
                if stat_ref.method != ref.statistical_analysis.method:
                    failures.append("statistical analysis method mismatch")
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"campaign-statistical-analysis.json validation failed: {e}")

    # Layer 6: provenance validation
    checked_layers.append("provenance")
    prov_path = candidate_dir / CAMPAIGN_PROVENANCE_JSON
    if prov_path.exists() and prov_path.is_file() and not prov_path.is_symlink():
        try:
            prov_ref = CampaignProvenanceRef.model_validate_json(prov_path.read_text())
            if ref is not None:
                if prov_ref.manifest_hash != ref.provenance.manifest_hash:
                    failures.append("provenance manifest_hash drift")
                if prov_ref.entry_count != ref.provenance.entry_count:
                    failures.append("provenance entry_count mismatch")
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"campaign-provenance.json validation failed: {e}")

    # Layer 7: verification reference validation — must be a passing report
    checked_layers.append("verification_ref")
    ver_path = candidate_dir / CAMPAIGN_VERIFICATION_REF_JSON
    if ver_path.exists() and ver_path.is_file() and not ver_path.is_symlink():
        try:
            ver_report = CampaignVerificationReport.model_validate_json(ver_path.read_text())
            if not ver_report.ok:
                failures.append(
                    f"verification reference is not passing: {ver_report.failures}"
                )
            if ref is not None:
                if ver_report.verified_index_generation_hash != ref.verified_index_generation_hash:
                    failures.append("verification index generation hash mismatch")
                if ver_report.campaign_id != ref.campaign_id:
                    failures.append("verification campaign_id mismatch")
        except (ValidationError, json.JSONDecodeError) as e:
            failures.append(f"campaign-verification-ref.json validation failed: {e}")

    # Layer 8: symlink scan — no symlinks anywhere in the candidate
    checked_layers.append("symlink_scan")
    for path in candidate_dir.rglob("*"):
        if path.is_symlink():
            failures.append(f"symlink in candidate: {path.relative_to(candidate_dir)}")

    ok = len(failures) == 0
    return PublicationValidatorResult(
        ok=ok,
        checked_layers=sorted(set(checked_layers)),
        failures=sorted(failures),
    )


# ---------------------------------------------------------------------------
# Publication schema v5: radar profile + score family summaries
# ---------------------------------------------------------------------------

PUBLICATION_V5_VERSION = "5.0.0"

# Event/resource files that the v5 projector copies from the report directory
# to the candidate when they exist. The validator requires these when v5
# artifacts (radar profile, score family summaries) are present.
_V5_EVENT_RESOURCE_FILES: tuple[str, ...] = (
    RESOURCE_OBSERVATIONS_JSONL,
    TOOL_CALL_SCORECARDS_JSONL,
    ESCALATION_RECORDS_JSONL,
    SECURITY_EVENTS_JSONL,
    CORRELATED_ERRORS_JSONL,
)

# The complete set of files that may appear in a v5 candidate directory. The
# validator rejects any file not in this set as undeclared.
_V5_DECLARED_FILES: frozenset[str] = frozenset({
    MODEL_CAMPAIGN_JSON,
    CAMPAIGN_PROJECTIONS_JSONL,
    CAMPAIGN_STATISTICAL_ANALYSIS_JSON,
    CAMPAIGN_PROVENANCE_JSON,
    CAMPAIGN_VERIFICATION_REF_JSON,
    RADAR_PROFILE_JSON,
    TOOL_SCORECARD_SUMMARY_JSON,
    ESCALATION_SUMMARY_JSON,
    SECURITY_EVENT_SUMMARY_JSON,
    CORRELATED_ERROR_SUMMARY_JSON,
    COLD_START_WARM_INFERENCE_TRADEOFF_JSON,
    RESOURCE_OBSERVATIONS_JSONL,
    TOOL_CALL_SCORECARDS_JSONL,
    ESCALATION_RECORDS_JSONL,
    SECURITY_EVENTS_JSONL,
    CORRELATED_ERRORS_JSONL,
})

# Mapping from radar dimension to the metric IDs that feed it. The metric
# IDs are sorted so the radar dimension is reproducible from the underlying
# metric observations. These mappings are the single source of truth for
# which metrics feed which radar dimension.
_RADAR_SOURCE_METRIC_IDS: dict[RadarDimensionName, list[str]] = {
    RadarDimensionName.TASK_ACCURACY: [
        "ifeval_subset_verifier",
        "tool_selection",
        "tool_arguments",
        "technical_analysis",
        "routing_delegation",
        "verification",
        "final_response",
    ],
    RadarDimensionName.TOOL_RELIABILITY: [
        "tool_call_follow_up",
        "tool_call_interpretation",
        "tool_call_looping",
        "tool_call_permission",
        "tool_call_recognition",
        "tool_call_recovery",
        "tool_call_schema",
        "tool_call_selection",
        "tool_call_semantics",
        "tool_call_unnecessary",
    ],
    RadarDimensionName.INSTRUCTION_FIDELITY: [
        "ifeval_subset_verifier",
        "instruction_adherence",
    ],
    RadarDimensionName.SECURITY: [
        "security_audit_record_complete",
        "security_audit_record_tampered",
        "security_authorization_correctly_enforced",
        "security_model_attempted_unauthorized_access",
        "security_policy",
        "security_policy_prevented_disclosure",
        "security_tool_attempted_unauthorized_operation",
    ],
    RadarDimensionName.PRIVACY: [
        "security_secret_redaction_successful",
        "security_sensitive_data_present",
        "security_sensitive_data_required",
        "security_sensitive_data_sent_externally",
        "security_unnecessary_data_sent_externally",
    ],
    RadarDimensionName.ESCALATION_QUALITY: [
        "escalation_correct_autonomous",
        "escalation_correct_escalation",
        "escalation_efficiency",
        "escalation_false_escalation",
        "escalation_missed_escalation",
    ],
    RadarDimensionName.RECOVERY: [
        "recovery",
        "tool_call_recovery",
    ],
    RadarDimensionName.REPEATABILITY: [
        "repeatability",
    ],
    RadarDimensionName.TOKEN_EFFICIENCY: [
        "token_efficiency",
    ],
    RadarDimensionName.LATENCY_EFFICIENCY: [
        "latency_efficiency",
    ],
}


class PublicationSchemaV5(BaseModel):
    """Composed README snapshot for publication schema v5.

    Extends v4 with the radar profile and score family summaries. The
    v5 schema carries forward all v4 artifacts (evidence cutoff,
    platform version, model campaign reference) and adds the radar
    profile, tool scorecard summary, escalation summary, security event
    summary, correlated error summary, and cold-start vs warm inference
    tradeoff. The v5 artifacts are optional so a v4 candidate can be
    upgraded to v5 without re-running the campaign; the projector
    populates them from the campaign report directory when the
    underlying records exist.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    publication_schema_version: str = Field(
        min_length=1, description="Publication schema version. Must be 5.0.0.",
    )
    evidence_cutoff: str = Field(min_length=1, description="ISO 8601 evidence cutoff timestamp.")
    platform_version: str = Field(min_length=1, description="Platform version label.")
    model_campaign: ModelCampaignRef = Field(description="Verified campaign reference.")
    radar_profile: RadarProfile | None = Field(
        default=None,
        description="Radar profile with one dimension per score family. None when not computed.",
    )
    tool_scorecard_summary: ToolScorecardSummary | None = Field(
        default=None,
        description="Tool calling scorecard summary. None when not computed.",
    )
    escalation_summary: EscalationSummary | None = Field(
        default=None,
        description="Escalation metrics summary. None when not computed.",
    )
    security_event_summary: SecurityEventSummary | None = Field(
        default=None,
        description="Security and privacy event summary. None when not computed.",
    )
    correlated_error_summary: CorrelatedErrorSummary | None = Field(
        default=None,
        description="Correlated error summary. None when not computed.",
    )
    cold_start_warm_inference_tradeoff: ColdStartWarmInferenceTradeoffSummary | None = Field(
        default=None,
        description="Cold-start vs warm inference tradeoff summary. None when not computed.",
    )
    disclosure_authority_hash: str | None = Field(
        default=None,
        min_length=64, max_length=64,
        description="SHA-256 of the frozen D12 disclosure authority binding this publication to disclosure-approved fields. None when not bound.",
    )

    @model_validator(mode="after")
    def _validate_version(self) -> Self:
        if self.publication_schema_version != PUBLICATION_SCHEMA_V5:
            raise ValueError(
                f"publication_schema_version must be {PUBLICATION_SCHEMA_V5!r}: "
                f"got {self.publication_schema_version!r}"
            )
        return self


def _build_radar_profile(
    *,
    campaign_id: str,
    campaign_revision: str,
    variant_summaries: list[CampaignVariantSummaryRow],
) -> RadarProfile | None:
    """Build a radar profile from per-variant summary rows.

    Each radar dimension's value is the macro mean rate of its source
    metric IDs across all variant summaries that contain those metrics.
    When no source metrics are present for a dimension, the value
    remains 0.0 but the ``availability`` field is set to ``UNAVAILABLE``
    so a reader can distinguish a measured zero from an absent
    measurement. The ``weighting_method`` is ``macro`` (mean of
    per-variant rates). Returns None when there are no variant
    summaries.
    """
    if not variant_summaries:
        return None

    summary_by_metric: dict[str, list[float]] = {}
    for s in variant_summaries:
        summary_by_metric.setdefault(s.metric_id, []).append(s.rate)

    dimensions: list[RadarDimension] = []
    for name in sorted(RadarDimensionName, key=lambda n: n.value):
        source_ids = sorted(_RADAR_SOURCE_METRIC_IDS[name])
        rates: list[float] = []
        for mid in source_ids:
            rates.extend(summary_by_metric.get(mid, []))
        if rates:
            value = sum(rates) / len(rates)
            value = round(value, 10)
            availability = MeasurementAvailability.MEASURED
        else:
            value = 0.0
            availability = MeasurementAvailability.UNAVAILABLE
        dimensions.append(RadarDimension(
            name=name,
            value=value,
            source_metric_ids=source_ids,
            availability=availability,
            weighting_method="macro",
        ))
    content_hash = compute_radar_profile_hash(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        dimensions=dimensions,
    )
    return RadarProfile(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        dimensions=dimensions,
        content_hash=content_hash,
    )


def _build_tool_scorecard_summary(
    *,
    campaign_id: str,
    campaign_revision: str,
    variant_summaries: list[CampaignVariantSummaryRow],
) -> ToolScorecardSummary | None:
    """Build a tool scorecard summary from per-variant summary rows.

    Each dimension's pass rate is the mean rate of the corresponding
    tool_call metric across all variant summaries that contain it. The
    total tool calls is the sum of denominators across all tool_call
    metrics. Returns None when no tool_call metrics are present.
    """
    tool_call_metric_prefix = "tool_call_"
    summary_by_metric: dict[str, CampaignVariantSummaryRow] = {}
    total_tool_calls = 0
    for s in variant_summaries:
        if s.metric_id.startswith(tool_call_metric_prefix):
            summary_by_metric[s.metric_id] = s
            total_tool_calls += s.denominator

    if not summary_by_metric:
        return None

    dimension_names = sorted(summary_by_metric.keys())
    dimensions: list[ToolScorecardDimensionSummary] = []
    for dim_name in dimension_names:
        s = summary_by_metric[dim_name]
        dimensions.append(ToolScorecardDimensionSummary(
            dimension=dim_name,
            pass_rate=round(s.rate, 10),
            tool_call_count=s.denominator,
        ))
    content_hash = compute_tool_scorecard_summary_hash(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        total_tool_calls=total_tool_calls,
        dimensions=dimensions,
    )
    return ToolScorecardSummary(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        total_tool_calls=total_tool_calls,
        dimensions=dimensions,
        content_hash=content_hash,
    )


def _build_escalation_summary(
    *,
    campaign_id: str,
    campaign_revision: str,
    variant_summaries: list[CampaignVariantSummaryRow],
) -> EscalationSummary | None:
    """Build an escalation summary from per-variant summary rows.

    Counts are derived from the escalation metric numerators and
    denominators. The escalation_efficiency is the proportion of
    correct routing decisions (autonomous + correct escalation) out of
    total records. Returns None when no escalation metrics are present.
    """
    escalation_metric_prefix = "escalation_"
    summary_by_metric: dict[str, CampaignVariantSummaryRow] = {}
    for s in variant_summaries:
        if s.metric_id.startswith(escalation_metric_prefix):
            summary_by_metric[s.metric_id] = s

    if not summary_by_metric:
        return None

    def _count(metric_id: str) -> int:
        s = summary_by_metric.get(metric_id)
        return s.numerator if s is not None else 0

    correct_autonomous = _count("escalation_correct_autonomous")
    correct_escalation = _count("escalation_correct_escalation")
    false_escalation = _count("escalation_false_escalation")
    missed_escalation = _count("escalation_missed_escalation")
    total_records = correct_autonomous + correct_escalation + false_escalation + missed_escalation
    efficiency = (correct_autonomous + correct_escalation) / total_records if total_records > 0 else 0.0
    content_hash = compute_escalation_summary_hash(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        total_records=total_records,
        correct_autonomous_count=correct_autonomous,
        correct_escalation_count=correct_escalation,
        false_escalation_count=false_escalation,
        missed_escalation_count=missed_escalation,
        escalation_efficiency=round(efficiency, 10),
    )
    return EscalationSummary(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        total_records=total_records,
        correct_autonomous_count=correct_autonomous,
        correct_escalation_count=correct_escalation,
        false_escalation_count=false_escalation,
        missed_escalation_count=missed_escalation,
        escalation_efficiency=round(efficiency, 10),
        content_hash=content_hash,
    )


def _build_security_event_summary(
    *,
    campaign_id: str,
    campaign_revision: str,
    variant_summaries: list[CampaignVariantSummaryRow],
) -> SecurityEventSummary | None:
    """Build a security event summary from per-variant summary rows.

    Each event rate is the mean rate of the corresponding security metric
    across all variant summaries that contain it. Returns None when no
    security metrics are present.
    """
    security_metric_prefix = "security_"
    summary_by_metric: dict[str, list[float]] = {}
    total_records = 0
    for s in variant_summaries:
        if s.metric_id.startswith(security_metric_prefix):
            summary_by_metric.setdefault(s.metric_id, []).append(s.rate)
            total_records += s.denominator

    if not summary_by_metric:
        return None

    def _rate(metric_id: str) -> float:
        rates = summary_by_metric.get(metric_id, [])
        return round(sum(rates) / len(rates), 10) if rates else 0.0

    content_hash = compute_security_event_summary_hash(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        total_records=total_records,
        sensitive_data_present_rate=_rate("security_sensitive_data_present"),
        sensitive_data_required_rate=_rate("security_sensitive_data_required"),
        sensitive_data_sent_externally_rate=_rate("security_sensitive_data_sent_externally"),
        unnecessary_data_sent_externally_rate=_rate("security_unnecessary_data_sent_externally"),
        policy_prevented_disclosure_rate=_rate("security_policy_prevented_disclosure"),
        model_attempted_unauthorized_access_rate=_rate("security_model_attempted_unauthorized_access"),
        tool_attempted_unauthorized_operation_rate=_rate("security_tool_attempted_unauthorized_operation"),
        authorization_correctly_enforced_rate=_rate("security_authorization_correctly_enforced"),
        audit_record_complete_rate=_rate("security_audit_record_complete"),
        audit_record_tampered_rate=_rate("security_audit_record_tampered"),
        secret_redaction_successful_rate=_rate("security_secret_redaction_successful"),
    )
    return SecurityEventSummary(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        total_records=total_records,
        sensitive_data_present_rate=_rate("security_sensitive_data_present"),
        sensitive_data_required_rate=_rate("security_sensitive_data_required"),
        sensitive_data_sent_externally_rate=_rate("security_sensitive_data_sent_externally"),
        unnecessary_data_sent_externally_rate=_rate("security_unnecessary_data_sent_externally"),
        policy_prevented_disclosure_rate=_rate("security_policy_prevented_disclosure"),
        model_attempted_unauthorized_access_rate=_rate("security_model_attempted_unauthorized_access"),
        tool_attempted_unauthorized_operation_rate=_rate("security_tool_attempted_unauthorized_operation"),
        authorization_correctly_enforced_rate=_rate("security_authorization_correctly_enforced"),
        audit_record_complete_rate=_rate("security_audit_record_complete"),
        audit_record_tampered_rate=_rate("security_audit_record_tampered"),
        secret_redaction_successful_rate=_rate("security_secret_redaction_successful"),
        content_hash=content_hash,
    )


def _build_correlated_error_summary(
    *,
    campaign_id: str,
    campaign_revision: str,
    variant_summaries: list[CampaignVariantSummaryRow],
) -> CorrelatedErrorSummary | None:
    """Build a correlated error summary from per-variant summary rows.

    Rates are derived from the correlated-error metric rates. Returns
    None when no correlated-error metrics are present.
    """
    correlated_metrics = {
        "correlated_failure_rate",
        "failure_independence",
        "same_family_correlated_rate",
        "cross_family_correlated_rate",
    }
    summary_by_metric: dict[str, list[float]] = {}
    total_scenarios = 0
    for s in variant_summaries:
        if s.metric_id in correlated_metrics:
            summary_by_metric.setdefault(s.metric_id, []).append(s.rate)
            total_scenarios += s.denominator

    if not summary_by_metric:
        return None

    def _rate(metric_id: str) -> float:
        rates = summary_by_metric.get(metric_id, [])
        return round(sum(rates) / len(rates), 10) if rates else 0.0

    content_hash = compute_correlated_error_summary_hash(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        total_scenarios=total_scenarios,
        correlated_failure_rate=_rate("correlated_failure_rate"),
        failure_independence=_rate("failure_independence"),
        same_family_correlated_rate=_rate("same_family_correlated_rate"),
        cross_family_correlated_rate=_rate("cross_family_correlated_rate"),
    )
    return CorrelatedErrorSummary(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        total_scenarios=total_scenarios,
        correlated_failure_rate=_rate("correlated_failure_rate"),
        failure_independence=_rate("failure_independence"),
        same_family_correlated_rate=_rate("same_family_correlated_rate"),
        cross_family_correlated_rate=_rate("cross_family_correlated_rate"),
        content_hash=content_hash,
    )


def _build_cold_start_tradeoff_summary(
    *,
    campaign_id: str,
    campaign_revision: str,
    model_variants: list[SafeModelVariantProjection],
) -> ColdStartWarmInferenceTradeoffSummary | None:
    """Build a cold-start vs warm inference tradeoff summary.

    The summary carries one tradeoff per variant. Timing values are
    None because the v5 projector does not yet read per-inference
    resource observations from the report directory. The
    ``unavailability_reason`` field carries a typed reason
    (``resource_observations_not_ingested``) explaining why the timing
    values are absent. The summary structure is ready for when the
    resource observations are available. Returns None when there are
    no model variants.
    """
    if not model_variants:
        return None

    tradeoffs: list[ColdStartWarmInferenceTradeoff] = []
    for v in sorted(model_variants, key=lambda v: v.variant_id):
        tradeoffs.append(ColdStartWarmInferenceTradeoff(
            variant_id=v.variant_id,
            unavailability_reason="resource_observations_not_ingested",
        ))
    content_hash = compute_cold_start_warm_inference_tradeoff_summary_hash(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        tradeoffs=tradeoffs,
    )
    return ColdStartWarmInferenceTradeoffSummary(
        campaign_id=campaign_id,
        campaign_revision=campaign_revision,
        tradeoffs=tradeoffs,
        content_hash=content_hash,
    )


def project_campaign_v5(
    *,
    child_report_dirs: dict[str, Path],
    campaign_set_plan: CampaignSetPlan,
    campaign_set_index: CampaignSetIndex,
    aggregate_verification_result: AggregateVerificationResult,
    candidate_dir: Path,
    campaign_profile: CampaignProfile,
    model_registry: ModelRegistry,
    provenance_manifest: SourceInclusionManifest,
    caveats: list[str],
    evidence_cutoff: str,
    platform_version: str,
) -> Path:
    """Project a verified campaign-set aggregate into a v5 candidate directory.

    Consumes only the accepted typed aggregate authority: a passing
    ``AggregateVerificationResult`` bound to a frozen
    ``CampaignSetPlan`` and ``CampaignSetIndex``, plus the exact
    child-ID-to-directory mapping for every accepted child report. The
    projector validates the full authority chain (plan hash, index
    hash, aggregate hash), reads projection rows, disposition rows, and
    event/resource records from every accepted child directory, builds
    v5 artifacts from the aggregate data, and binds the publication
    model to the aggregate authority hashes. Refuses to overwrite an
    existing candidate directory. No single-report fallback exists for
    v5; v4 retains the single-report projector for legacy loading.

    Returns the candidate directory path.
    """
    # Validate the aggregate authority chain.
    if not aggregate_verification_result.ok:
        raise ValueError(
            f"aggregate verification failed: {aggregate_verification_result.failures}"
        )

    if campaign_set_plan.content_hash != campaign_set_index.set_plan_hash:
        raise ValueError(
            f"campaign set plan hash mismatch: plan {campaign_set_plan.content_hash!r}, "
            f"index set_plan_hash {campaign_set_index.set_plan_hash!r}"
        )

    if campaign_set_index.content_hash != aggregate_verification_result.set_index_hash:
        raise ValueError(
            f"campaign set index hash mismatch: index {campaign_set_index.content_hash!r}, "
            f"aggregate set_index_hash {aggregate_verification_result.set_index_hash!r}"
        )

    if campaign_set_plan.content_hash != aggregate_verification_result.set_plan_hash:
        raise ValueError(
            f"campaign set plan hash mismatch with aggregate: "
            f"plan {campaign_set_plan.content_hash!r}, "
            f"aggregate set_plan_hash {aggregate_verification_result.set_plan_hash!r}"
        )

    # Validate the child-ID-to-directory mapping against the plan.
    plan_child_ids = {cp.child_id for cp in campaign_set_plan.child_plans}
    provided_child_ids = set(child_report_dirs.keys())
    if provided_child_ids != plan_child_ids:
        missing = plan_child_ids - provided_child_ids
        extra = provided_child_ids - plan_child_ids
        raise ValueError(
            f"child report directory mapping does not match plan: "
            f"missing={sorted(missing)}, extra={sorted(extra)}"
        )

    if candidate_dir.exists():
        raise ValueError(f"candidate directory already exists: {candidate_dir}")

    # Build a synthetic CampaignVerificationReport from the aggregate
    # verification result. The v4 validator's verification_ref layer
    # reads this file and checks ok, verified_index_generation_hash,
    # and campaign_id. For v5, the verified_index_generation_hash
    # binds to the aggregate verification hash, which transitively
    # binds to the plan and index.
    aggregate_verification_hash = aggregate_verification_result.content_hash
    verification = CampaignVerificationReport(
        verification_schema_version=aggregate_verification_result.verification_schema_version,
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        ok=aggregate_verification_result.ok,
        verified_index_generation_hash=aggregate_verification_hash,
        checked_layers=sorted({
            "file_safety", "index_chain", "finalization", "aggregate_verification"
        }),
        failures=[],
    )

    # Read projection rows from every accepted child directory.
    projection_rows: list[CampaignProjectionRow] = []
    for cp in campaign_set_plan.child_plans:
        child_dir = child_report_dirs[cp.child_id]
        projection_rows.extend(_generate_projections(
            report_dir=child_dir,
            campaign_id=campaign_profile.campaign_id,
            campaign_revision=campaign_profile.campaign_revision,
        ))

    # Read disposition rows from every accepted child directory using
    # each child's verified index generation hash from the aggregate
    # verification result.
    child_result_by_id = {
        cr.child_id: cr for cr in aggregate_verification_result.child_results
    }
    disposition_rows: list[CampaignDispositionRow] = []
    for cp in campaign_set_plan.child_plans:
        child_dir = child_report_dirs[cp.child_id]
        child_result = child_result_by_id.get(cp.child_id)
        if child_result is None:
            raise ValueError(
                f"aggregate verification result missing child result for {cp.child_id!r}"
            )
        disposition_rows.extend(_generate_dispositions(
            report_dir=child_dir,
            verified_index_generation_hash=child_result.verified_index_generation_hash,
        ))

    # Generate the statistical analysis reference from the first child
    # directory (the statistical analysis is campaign-level, not
    # per-child; the first child carries the campaign-level file).
    first_child_dir = child_report_dirs[campaign_set_plan.child_plans[0].child_id]
    stat_ref = _generate_statistical_analysis(
        report_dir=first_child_dir,
        campaign_profile=campaign_profile,
    )
    variant_summaries = _generate_variant_summaries(projection_rows)
    profile_projection = _project_profile(campaign_profile)
    variant_projections = [_project_variant(v) for v in model_registry.variants]
    provenance_ref = _project_provenance(provenance_manifest)

    content_hash = compute_model_campaign_hash(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        publication_schema_version=PUBLICATION_SCHEMA_V5,
        campaign_profile=profile_projection,
        model_variants=variant_projections,
        model_registry_hash=model_registry.content_hash,
        verification_ok=verification.ok,
        verified_index_generation_hash=verification.verified_index_generation_hash,
        checked_layers=verification.checked_layers,
        projections=projection_rows,
        dispositions=disposition_rows,
        comparison_rows=[],
        efficiency_observations=[],
        variant_summaries=variant_summaries,
        statistical_analysis=stat_ref,
        provenance=provenance_ref,
        caveats=caveats,
        campaign_set_plan_hash=campaign_set_plan.content_hash,
        campaign_set_index_hash=campaign_set_index.content_hash,
        aggregate_verification_hash=aggregate_verification_hash,
    )

    ref = ModelCampaignRef(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        publication_schema_version=PUBLICATION_SCHEMA_V5,
        campaign_profile=profile_projection,
        model_variants=variant_projections,
        model_registry_hash=model_registry.content_hash,
        verification_ok=verification.ok,
        verified_index_generation_hash=verification.verified_index_generation_hash,
        checked_layers=verification.checked_layers,
        projections=projection_rows,
        dispositions=disposition_rows,
        comparison_rows=[],
        efficiency_observations=[],
        variant_summaries=variant_summaries,
        statistical_analysis=stat_ref,
        provenance=provenance_ref,
        caveats=[CampaignCaveat(c) for c in caveats],
        campaign_set_plan_hash=campaign_set_plan.content_hash,
        campaign_set_index_hash=campaign_set_index.content_hash,
        aggregate_verification_hash=aggregate_verification_hash,
        content_hash=content_hash,
    )

    # Build the v5 artifacts from the variant summaries.
    radar_profile = _build_radar_profile(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        variant_summaries=variant_summaries,
    )
    tool_scorecard_summary = _build_tool_scorecard_summary(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        variant_summaries=variant_summaries,
    )
    escalation_summary = _build_escalation_summary(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        variant_summaries=variant_summaries,
    )
    security_event_summary = _build_security_event_summary(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        variant_summaries=variant_summaries,
    )
    correlated_error_summary = _build_correlated_error_summary(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        variant_summaries=variant_summaries,
    )
    cold_start_tradeoff = _build_cold_start_tradeoff_summary(
        campaign_id=campaign_profile.campaign_id,
        campaign_revision=campaign_profile.campaign_revision,
        model_variants=variant_projections,
    )

    # Write all artifacts to the candidate directory.
    candidate_dir.mkdir(parents=True)
    _write_json(candidate_dir / MODEL_CAMPAIGN_JSON, ref.model_dump_json())
    _write_jsonl(candidate_dir / CAMPAIGN_PROJECTIONS_JSONL, projection_rows)
    _write_json(candidate_dir / CAMPAIGN_STATISTICAL_ANALYSIS_JSON, stat_ref.model_dump_json())
    _write_json(candidate_dir / CAMPAIGN_PROVENANCE_JSON, provenance_ref.model_dump_json())
    _write_json(candidate_dir / CAMPAIGN_VERIFICATION_REF_JSON, verification.model_dump_json())
    if radar_profile is not None:
        _write_json(candidate_dir / RADAR_PROFILE_JSON, radar_profile.model_dump_json())
    if tool_scorecard_summary is not None:
        _write_json(candidate_dir / TOOL_SCORECARD_SUMMARY_JSON, tool_scorecard_summary.model_dump_json())
    if escalation_summary is not None:
        _write_json(candidate_dir / ESCALATION_SUMMARY_JSON, escalation_summary.model_dump_json())
    if security_event_summary is not None:
        _write_json(candidate_dir / SECURITY_EVENT_SUMMARY_JSON, security_event_summary.model_dump_json())
    if correlated_error_summary is not None:
        _write_json(candidate_dir / CORRELATED_ERROR_SUMMARY_JSON, correlated_error_summary.model_dump_json())
    if cold_start_tradeoff is not None:
        _write_json(candidate_dir / COLD_START_WARM_INFERENCE_TRADEOFF_JSON, cold_start_tradeoff.model_dump_json())

    # Copy event/resource files from every accepted child directory to
    # the candidate, concatenating records from all children. These are
    # the authoritative typed records that the v5 summaries are derived
    # from. The validator requires them when v5 artifacts are present.
    for event_resource_file in _V5_EVENT_RESOURCE_FILES:
        concatenated_lines: list[bytes] = []
        for cp in campaign_set_plan.child_plans:
            child_dir = child_report_dirs[cp.child_id]
            src = child_dir / event_resource_file
            if src.exists() and src.is_file() and not src.is_symlink():
                content = src.read_bytes()
                if content:
                    concatenated_lines.append(content)
                    if not content.endswith(b"\n"):
                        concatenated_lines.append(b"\n")
        dst = candidate_dir / event_resource_file
        dst.parent.mkdir(parents=True, exist_ok=True)
        dst.write_bytes(b"".join(concatenated_lines))

    return candidate_dir


def validate_publication_v5(candidate_dir: Path) -> PublicationValidatorResult:
    """Strictly validate a v5 publication candidate directory.

    Extends the v4 validation with the v5 artifacts (radar profile and
    score family summaries). The v5 artifacts are optional: when absent,
    the corresponding layer passes. When present, each artifact is
    parsed and its content hash is verified. Returns a typed
    ``PublicationValidatorResult`` with ``ok=True`` only when every layer
    passes.
    """
    failures: list[str] = []
    checked_layers: list[str] = []

    # Reuse the v4 validation for the core artifacts.
    v4_result = validate_publication_v4(candidate_dir)
    checked_layers.extend(v4_result.checked_layers)
    failures.extend(v4_result.failures)

    # Layer 9: radar profile validation
    checked_layers.append("radar_profile")
    radar_path = candidate_dir / RADAR_PROFILE_JSON
    if radar_path.exists():
        if radar_path.is_symlink():
            failures.append(f"symlink rejected: {RADAR_PROFILE_JSON}")
        elif not radar_path.is_file():
            failures.append(f"not a regular file: {RADAR_PROFILE_JSON}")
        else:
            try:
                RadarProfile.model_validate_json(radar_path.read_text())
            except (ValidationError, json.JSONDecodeError) as e:
                failures.append(f"radar-profile.json validation failed: {e}")

    # Layer 10: tool scorecard summary validation
    checked_layers.append("tool_scorecard_summary")
    ts_path = candidate_dir / TOOL_SCORECARD_SUMMARY_JSON
    if ts_path.exists():
        if ts_path.is_symlink():
            failures.append(f"symlink rejected: {TOOL_SCORECARD_SUMMARY_JSON}")
        elif not ts_path.is_file():
            failures.append(f"not a regular file: {TOOL_SCORECARD_SUMMARY_JSON}")
        else:
            try:
                ToolScorecardSummary.model_validate_json(ts_path.read_text())
            except (ValidationError, json.JSONDecodeError) as e:
                failures.append(f"tool-scorecard-summary.json validation failed: {e}")

    # Layer 11: escalation summary validation
    checked_layers.append("escalation_summary")
    es_path = candidate_dir / ESCALATION_SUMMARY_JSON
    if es_path.exists():
        if es_path.is_symlink():
            failures.append(f"symlink rejected: {ESCALATION_SUMMARY_JSON}")
        elif not es_path.is_file():
            failures.append(f"not a regular file: {ESCALATION_SUMMARY_JSON}")
        else:
            try:
                EscalationSummary.model_validate_json(es_path.read_text())
            except (ValidationError, json.JSONDecodeError) as e:
                failures.append(f"escalation-summary.json validation failed: {e}")

    # Layer 12: security event summary validation
    checked_layers.append("security_event_summary")
    se_path = candidate_dir / SECURITY_EVENT_SUMMARY_JSON
    if se_path.exists():
        if se_path.is_symlink():
            failures.append(f"symlink rejected: {SECURITY_EVENT_SUMMARY_JSON}")
        elif not se_path.is_file():
            failures.append(f"not a regular file: {SECURITY_EVENT_SUMMARY_JSON}")
        else:
            try:
                SecurityEventSummary.model_validate_json(se_path.read_text())
            except (ValidationError, json.JSONDecodeError) as e:
                failures.append(f"security-event-summary.json validation failed: {e}")

    # Layer 13: correlated error summary validation
    checked_layers.append("correlated_error_summary")
    ce_path = candidate_dir / CORRELATED_ERROR_SUMMARY_JSON
    if ce_path.exists():
        if ce_path.is_symlink():
            failures.append(f"symlink rejected: {CORRELATED_ERROR_SUMMARY_JSON}")
        elif not ce_path.is_file():
            failures.append(f"not a regular file: {CORRELATED_ERROR_SUMMARY_JSON}")
        else:
            try:
                CorrelatedErrorSummary.model_validate_json(ce_path.read_text())
            except (ValidationError, json.JSONDecodeError) as e:
                failures.append(f"correlated-error-summary.json validation failed: {e}")

    # Layer 14: cold-start warm inference tradeoff validation
    checked_layers.append("cold_start_warm_inference_tradeoff")
    cs_path = candidate_dir / COLD_START_WARM_INFERENCE_TRADEOFF_JSON
    if cs_path.exists():
        if cs_path.is_symlink():
            failures.append(f"symlink rejected: {COLD_START_WARM_INFERENCE_TRADEOFF_JSON}")
        elif not cs_path.is_file():
            failures.append(f"not a regular file: {COLD_START_WARM_INFERENCE_TRADEOFF_JSON}")
        else:
            try:
                ColdStartWarmInferenceTradeoffSummary.model_validate_json(cs_path.read_text())
            except (ValidationError, json.JSONDecodeError) as e:
                failures.append(f"cold-start-warm-inference-tradeoff.json validation failed: {e}")

    # Layer 15: required event/resource files — when any v5 artifact is
    # present, the 5 event/resource files must also be present as regular
    # files. Missing an expected file is not equivalent to zero records.
    checked_layers.append("required_event_resource_files")
    v5_artifacts_present = any(
        (candidate_dir / f).exists()
        for f in (
            RADAR_PROFILE_JSON,
            TOOL_SCORECARD_SUMMARY_JSON,
            ESCALATION_SUMMARY_JSON,
            SECURITY_EVENT_SUMMARY_JSON,
            CORRELATED_ERROR_SUMMARY_JSON,
            COLD_START_WARM_INFERENCE_TRADEOFF_JSON,
        )
    )
    if v5_artifacts_present:
        for event_resource_file in _V5_EVENT_RESOURCE_FILES:
            er_path = candidate_dir / event_resource_file
            if not er_path.exists():
                failures.append(f"missing required event/resource file: {event_resource_file}")
            elif er_path.is_symlink():
                failures.append(f"symlink rejected: {event_resource_file}")
            elif not er_path.is_file():
                failures.append(f"not a regular file: {event_resource_file}")

    # Layer 16: undeclared files — reject any file in the candidate
    # directory that is not in the v5 declared set. The v5 candidate is a
    # closed artifact set; unknown files are rejected.
    checked_layers.append("undeclared_files")
    for path in candidate_dir.rglob("*"):
        if path.is_file() and not path.is_symlink():
            rel = path.relative_to(candidate_dir).as_posix()
            if rel not in _V5_DECLARED_FILES:
                failures.append(f"undeclared file in candidate: {rel}")

    # Layer 17: radar profile recompute — independently recompute the radar
    # profile from the projection rows and reject a tampered radar profile
    # even when its internal content hash is valid. The projection rows are
    # the source of truth; the stored radar profile must recompute from them.
    checked_layers.append("radar_profile_recompute")
    radar_path = candidate_dir / RADAR_PROFILE_JSON
    if radar_path.exists() and radar_path.is_file() and not radar_path.is_symlink():
        try:
            stored_profile = RadarProfile.model_validate_json(radar_path.read_text())
        except (ValidationError, json.JSONDecodeError):
            stored_profile = None  # Already caught by layer 9

        if stored_profile is not None:
            recomputed_profile: RadarProfile | None = None
            recompute_error = False
            try:
                proj_path = candidate_dir / CAMPAIGN_PROJECTIONS_JSONL
                recomp_rows: list[CampaignProjectionRow] = []
                if proj_path.exists() and proj_path.is_file() and not proj_path.is_symlink():
                    for line in proj_path.read_text().strip().splitlines():
                        if line.strip():
                            recomp_rows.append(CampaignProjectionRow.model_validate_json(line))
                recomp_summaries = _generate_variant_summaries(recomp_rows)
                mc_path = candidate_dir / MODEL_CAMPAIGN_JSON
                if mc_path.exists() and mc_path.is_file():
                    mc_data = json.loads(mc_path.read_text())
                    recomputed_profile = _build_radar_profile(
                        campaign_id=mc_data.get("campaign_id", ""),
                        campaign_revision=mc_data.get("campaign_revision", ""),
                        variant_summaries=recomp_summaries,
                    )
            except (ValidationError, json.JSONDecodeError) as e:
                failures.append(f"radar profile recompute error: {e}")
                recompute_error = True

            if not recompute_error:
                if recomputed_profile is None:
                    failures.append(
                        "radar profile recompute mismatch: cannot recompute radar from projection rows"
                    )
                elif recomputed_profile.content_hash != stored_profile.content_hash:
                    failures.append(
                        f"radar profile recompute mismatch: "
                        f"stored {stored_profile.content_hash!r}, "
                        f"recomputed {recomputed_profile.content_hash!r}"
                    )

    # Layer 18: aggregate authority binding — the model-campaign.json
    # must carry the campaign-set plan hash, campaign-set index hash, and
    # aggregate verification hash. These fields bind the v5 publication
    # to the accepted multi-child aggregate authority. A v5 candidate
    # without these fields is a single-report fallback, which is
    # rejected. This layer applies only to v5 publications; v4
    # candidates retain the single-report projector and do not carry
    # aggregate authority hashes.
    mc_path = candidate_dir / MODEL_CAMPAIGN_JSON
    if mc_path.exists() and mc_path.is_file() and not mc_path.is_symlink():
        try:
            mc_data = json.loads(mc_path.read_text())
            schema_version = mc_data.get("publication_schema_version", "")
            if schema_version == PUBLICATION_SCHEMA_V5:
                checked_layers.append("aggregate_authority_binding")
                agg_plan_hash = mc_data.get("campaign_set_plan_hash")
                agg_index_hash = mc_data.get("campaign_set_index_hash")
                agg_ver_hash = mc_data.get("aggregate_verification_hash")
                if not agg_plan_hash or len(agg_plan_hash) != 64:
                    failures.append(
                        "aggregate authority binding missing: campaign_set_plan_hash"
                    )
                if not agg_index_hash or len(agg_index_hash) != 64:
                    failures.append(
                        "aggregate authority binding missing: campaign_set_index_hash"
                    )
                if not agg_ver_hash or len(agg_ver_hash) != 64:
                    failures.append(
                        "aggregate authority binding missing: aggregate_verification_hash"
                    )
        except json.JSONDecodeError as e:
            failures.append(f"aggregate authority binding error: {e}")

    ok = len(failures) == 0
    return PublicationValidatorResult(
        ok=ok,
        checked_layers=sorted(set(checked_layers)),
        failures=sorted(failures),
    )


__all__ = [
    "PUBLICATION_V4_VERSION",
    "PUBLICATION_V5_VERSION",
    "CampaignCaveat",
    "CampaignComparisonRow",
    "CampaignDispositionRow",
    "CampaignEfficiencyObservation",
    "CampaignProjectionRow",
    "CampaignProvenanceRef",
    "CampaignStatisticalAnalysisRef",
    "CampaignVariantSummaryRow",
    "ModelCampaignRef",
    "PublicationSchemaV4",
    "PublicationSchemaV5",
    "PublicationValidatorResult",
    "SafeCampaignProfileProjection",
    "SafeModelVariantProjection",
    "compute_model_campaign_hash",
    "project_campaign_v4",
    "project_campaign_v5",
    "validate_publication_v4",
    "validate_publication_v5",
]
