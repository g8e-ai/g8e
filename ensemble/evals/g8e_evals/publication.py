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
    METRICS_JSONL,
    MODEL_CAMPAIGN_JSON,
    PUBLICATION_SCHEMA_V4,
)
from g8e_evals.index import (
    AssignmentDisposition,
    CampaignVerificationReport,
)
from g8e_evals.profile import CampaignProfile
from g8e_evals.provenance import SourceInclusionManifest
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

    Aggregates all per-assignment projection rows for one variant and
    one metric into a single pass-rate summary. The numerator is the
    total positive outcomes across all tasks and repetitions; the
    denominator is the total measured outcomes. The rate is numerator
    divided by denominator. Task count, repetition count, terminal
    count, superseded count, invalid count, and missing count are
    reported so a reader can inspect completeness without pooling
    distinct variants.
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
    variant_summaries: list[CampaignVariantSummaryRow] = (),
    statistical_analysis: CampaignStatisticalAnalysisRef,
    provenance: CampaignProvenanceRef,
    caveats: list[CampaignCaveat] | list[str],
) -> str:
    """Compute the content hash for a model campaign reference without constructing the full model."""
    caveats_values = sorted(
        c.value if isinstance(c, CampaignCaveat) else c for c in caveats
    )
    payload = json.dumps(
        {
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
                for s in sorted(variant_summaries, key=lambda s: (s.variant_id, s.metric_id))
            ],
            "statistical_analysis": json.loads(statistical_analysis.model_dump_json()),
            "provenance": json.loads(provenance.model_dump_json()),
            "caveats": caveats_values,
        },
        allow_nan=False,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return _sha256(payload)


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
    runs). Results are sorted by ``(variant_id, metric_id)``.
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


__all__ = [
    "PUBLICATION_V4_VERSION",
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
    "PublicationValidatorResult",
    "SafeCampaignProfileProjection",
    "SafeModelVariantProjection",
    "compute_model_campaign_hash",
    "project_campaign_v4",
    "validate_publication_v4",
]
