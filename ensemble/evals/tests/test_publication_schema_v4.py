# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for publication schema v4 models.

Verifies that v4 publication models are frozen, reject unknown fields
(``extra="forbid"``), enforce a closed caveat vocabulary, validate
content hashes, and round-trip through canonical JSON serialization.
"""

# pyright: reportCallIssue=false

from __future__ import annotations

import json

import pytest
from pydantic import ValidationError

from g8e_evals.constants import PUBLICATION_SCHEMA_V4
from g8e_evals.publication import (
    CampaignCaveat,
    CampaignComparisonRow,
    CampaignDispositionRow,
    CampaignEfficiencyObservation,
    CampaignProjectionRow,
    CampaignProvenanceRef,
    CampaignStatisticalAnalysisRef,
    ModelCampaignRef,
    PublicationSchemaV4,
    SafeCampaignProfileProjection,
    SafeModelVariantProjection,
    compute_model_campaign_hash,
)


pytestmark = pytest.mark.unit

_HASH = "a" * 64


def _make_projection_row(
    *,
    variant_id: str = "qwen3-8b-q4_0",
    task_id: str = "task-1",
    metric_id: str = "ifeval_subset_verifier",
    repetition: int = 1,
) -> CampaignProjectionRow:
    return CampaignProjectionRow(
        campaign_id="campaign-1",
        campaign_revision="1",
        variant_id=variant_id,
        task_id=task_id,
        metric_id=metric_id,
        numerator=8,
        denominator=10,
        rate=0.8,
        unit="boolean",
        verification_status="verified",
        evidence_link="proofs/run-1/analysis.json",
        repetition=repetition,
    )


def _make_disposition_row(
    *,
    disposition: str = "effective",
    reason: str = "",
) -> CampaignDispositionRow:
    return CampaignDispositionRow(
        assignment_id="assignment-1",
        disposition=disposition,  # pyright: ignore[reportArgumentType]
        variant_id="qwen3-8b-q4_0",
        task_id="task-1",
        reason=reason,
    )


def _make_comparison_row() -> CampaignComparisonRow:
    return CampaignComparisonRow(
        task_id="task-1",
        combination_id="combo-1",
        primary_variant_id="qwen3-8b-q4_0",
        assistant_variant_id="qwen3-4b-q4_0",
        lite_variant_id="qwen3-0.6b-q4_0",
        metric_id="ifeval_subset_verifier",
        repetition=1,
        numerator=8,
        denominator=10,
        rate=0.8,
        unit="boolean",
    )


def _make_efficiency_observation() -> CampaignEfficiencyObservation:
    return CampaignEfficiencyObservation(
        variant_id="qwen3-8b-q4_0",
        hardware_identity="linux/amd64/rtx-4090",
        end_to_end_latency_seconds=12.5,
        output_throughput_tokens_per_second=42.0,
        artifact_bytes=4_800_000_000,
        peak_resident_memory_bytes=2_000_000_000,
    )


def _make_statistical_analysis_ref() -> CampaignStatisticalAnalysisRef:
    return CampaignStatisticalAnalysisRef(
        method="mcnemar",
        independent_unit="task",
        population=10,
        correction_family="holm_bonferroni",
        claim_status="descriptive_only",
        content_hash=_HASH,
    )


def _make_provenance_ref() -> CampaignProvenanceRef:
    return CampaignProvenanceRef(
        manifest_hash=_HASH,
        entry_count=5,
        schema_version="1.0.0",
    )


def _make_profile_projection() -> SafeCampaignProfileProjection:
    return SafeCampaignProfileProjection(
        campaign_id="campaign-1",
        campaign_revision="1",
        content_hash=_HASH,
        benchmark_ids=["ifeval_subset"],
        task_ids=["task-1", "task-2"],
        repetitions=3,
        hardware_identity="linux/amd64/rtx-4090",
        claim_boundary="descriptive_only",
    )


def _make_variant_projection(
    *,
    variant_id: str = "qwen3-8b-q4_0",
) -> SafeModelVariantProjection:
    return SafeModelVariantProjection(
        variant_id=variant_id,
        canonical_display_name="Qwen3-8B (q4_0)",
        weight_class="heavy-slm",
        parameter_count_display="8.0B",
        backend_name="ollama",
        served_model_tag="qwen3:8b",
        artifact_digest=_HASH,
        quantization="q4_0",
        publication_eligibility="eligible",
    )


def _make_model_campaign(
    *,
    caveats: list[str] | None = None,
    content_hash: str | None = None,
) -> ModelCampaignRef:
    profile = _make_profile_projection()
    variants = [_make_variant_projection()]
    projections = [_make_projection_row()]
    dispositions = [_make_disposition_row()]
    comparison_rows = [_make_comparison_row()]
    efficiency = [_make_efficiency_observation()]
    stat_ref = _make_statistical_analysis_ref()
    prov_ref = _make_provenance_ref()
    effective_caveats = caveats if caveats is not None else ["descriptive_only"]
    if content_hash is None:
        content_hash = compute_model_campaign_hash(
            campaign_id="campaign-1",
            campaign_revision="1",
            publication_schema_version=PUBLICATION_SCHEMA_V4,
            campaign_profile=profile,
            model_variants=variants,
            model_registry_hash=_HASH,
            verification_ok=True,
            verified_index_generation_hash=_HASH,
            checked_layers=["file_safety", "index_chain"],
            projections=projections,
            dispositions=dispositions,
            comparison_rows=comparison_rows,
            efficiency_observations=efficiency,
            statistical_analysis=stat_ref,
            provenance=prov_ref,
            caveats=effective_caveats,
        )
    return ModelCampaignRef(
        campaign_id="campaign-1",
        campaign_revision="1",
        publication_schema_version=PUBLICATION_SCHEMA_V4,
        campaign_profile=profile,
        model_variants=variants,
        model_registry_hash=_HASH,
        verification_ok=True,
        verified_index_generation_hash=_HASH,
        checked_layers=["file_safety", "index_chain"],
        projections=projections,
        dispositions=dispositions,
        comparison_rows=comparison_rows,
        efficiency_observations=efficiency,
        statistical_analysis=stat_ref,
        provenance=prov_ref,
        caveats=effective_caveats,  # pyright: ignore[reportArgumentType]
        content_hash=content_hash,
    )


# --- Frozen model tests ---


def test_projection_row_is_frozen() -> None:
    row = _make_projection_row()
    with pytest.raises(ValidationError):
        row.numerator = 99  # type: ignore[misc]


def test_disposition_row_is_frozen() -> None:
    row = _make_disposition_row()
    with pytest.raises(ValidationError):
        row.disposition = "superseded"  # type: ignore[misc]


def test_model_campaign_ref_is_frozen() -> None:
    ref = _make_model_campaign()
    with pytest.raises(ValidationError):
        ref.campaign_id = "other"  # type: ignore[misc]


def test_publication_schema_v4_is_frozen() -> None:
    ref = _make_model_campaign()
    schema = PublicationSchemaV4(
        publication_schema_version=PUBLICATION_SCHEMA_V4,
        evidence_cutoff="2026-09-10",
        platform_version="v2.1.8",
        model_campaign=ref,
    )
    with pytest.raises(ValidationError):
        schema.evidence_cutoff = "2026-01-01"  # type: ignore[misc]


# --- extra="forbid" unknown field rejection ---


def test_projection_row_rejects_unknown_field() -> None:
    with pytest.raises(ValidationError):
        CampaignProjectionRow(
            campaign_id="c",
            campaign_revision="1",
            variant_id="v",
            task_id="t",
            metric_id="m",
            numerator=1,
            denominator=2,
            rate=0.5,
            unit="boolean",
            verification_status="verified",
            evidence_link="p.json",
            repetition=1,
            raw_prompt="secret",  # type: ignore[call-arg]
        )


def test_model_campaign_ref_rejects_unknown_field() -> None:
    ref = _make_model_campaign()
    data = json.loads(ref.model_dump_json())
    data["secret_field"] = "leak"
    with pytest.raises(ValidationError):
        ModelCampaignRef.model_validate(data)


def test_publication_schema_v4_rejects_unknown_field() -> None:
    ref = _make_model_campaign()
    schema = PublicationSchemaV4(
        publication_schema_version=PUBLICATION_SCHEMA_V4,
        evidence_cutoff="2026-09-10",
        platform_version="v2.1.8",
        model_campaign=ref,
    )
    data = json.loads(schema.model_dump_json())
    data["extra_top"] = "no"
    with pytest.raises(ValidationError):
        PublicationSchemaV4.model_validate(data)


def test_safe_variant_projection_rejects_unknown_field() -> None:
    with pytest.raises(ValidationError):
        SafeModelVariantProjection(
            variant_id="v",
            canonical_display_name="V",
            weight_class="heavy-slm",
            parameter_count_display="8.0B",
            backend_name="ollama",
            served_model_tag="v:8b",
            artifact_digest=_HASH,
            quantization="q4_0",
            publication_eligibility="eligible",
            api_key="leak",  # type: ignore[call-arg]
        )


# --- Closed caveat vocabulary ---


def test_model_campaign_accepts_known_caveats() -> None:
    ref = _make_model_campaign(caveats=["descriptive_only", "single_hardware_class"])  # pyright: ignore[reportArgumentType]
    assert set(ref.caveats) == {"descriptive_only", "single_hardware_class"}


def test_model_campaign_rejects_unknown_caveat() -> None:
    with pytest.raises(ValidationError):
        _make_model_campaign(caveats=["bogus_caveat"])


def test_caveat_enum_values_are_closed() -> None:
    valid = {c.value for c in CampaignCaveat}
    assert "descriptive_only" in valid
    assert "single_hardware_class" in valid
    assert "no_inferential_claims" in valid
    assert "bogus" not in valid


# --- Content hash validation ---


def test_model_campaign_content_hash_mismatch_rejected() -> None:
    with pytest.raises(ValidationError, match="content_hash mismatch"):
        _make_model_campaign(content_hash="b" * 64)


def test_model_campaign_content_hash_match_accepted() -> None:
    ref = _make_model_campaign()
    assert ref.content_hash == compute_model_campaign_hash(
        campaign_id=ref.campaign_id,
        campaign_revision=ref.campaign_revision,
        publication_schema_version=ref.publication_schema_version,
        campaign_profile=ref.campaign_profile,
        model_variants=ref.model_variants,
        model_registry_hash=ref.model_registry_hash,
        verification_ok=ref.verification_ok,
        verified_index_generation_hash=ref.verified_index_generation_hash,
        checked_layers=ref.checked_layers,
        projections=ref.projections,
        dispositions=ref.dispositions,
        comparison_rows=ref.comparison_rows,
        efficiency_observations=ref.efficiency_observations,
        statistical_analysis=ref.statistical_analysis,
        provenance=ref.provenance,
        caveats=ref.caveats,
    )


def test_content_hash_is_deterministic() -> None:
    ref1 = _make_model_campaign()
    ref2 = _make_model_campaign()
    assert ref1.content_hash == ref2.content_hash


def test_content_hash_changes_with_caveats() -> None:
    ref1 = _make_model_campaign(caveats=["descriptive_only"])
    ref2 = _make_model_campaign(caveats=["single_hardware_class"])
    assert ref1.content_hash != ref2.content_hash


# --- Round-trip serialization ---


def test_model_campaign_round_trip() -> None:
    ref = _make_model_campaign()
    data = ref.model_dump_json()
    restored = ModelCampaignRef.model_validate_json(data)
    assert restored == ref


def test_publication_schema_v4_round_trip() -> None:
    ref = _make_model_campaign()
    schema = PublicationSchemaV4(
        publication_schema_version=PUBLICATION_SCHEMA_V4,
        evidence_cutoff="2026-09-10",
        platform_version="v2.1.8",
        model_campaign=ref,
    )
    data = schema.model_dump_json()
    restored = PublicationSchemaV4.model_validate_json(data)
    assert restored == schema


def test_projection_row_round_trip() -> None:
    row = _make_projection_row()
    restored = CampaignProjectionRow.model_validate_json(row.model_dump_json())
    assert restored == row


def test_comparison_row_round_trip() -> None:
    row = _make_comparison_row()
    restored = CampaignComparisonRow.model_validate_json(row.model_dump_json())
    assert restored == row


# --- Field constraints ---


def test_projection_row_rejects_traversal_in_evidence_link() -> None:
    with pytest.raises(ValidationError):
        CampaignProjectionRow(
            campaign_id="c",
            campaign_revision="1",
            variant_id="v",
            task_id="t",
            metric_id="m",
            numerator=1,
            denominator=2,
            rate=0.5,
            unit="boolean",
            verification_status="verified",
            evidence_link="../secret/analysis.json",
            repetition=1,
        )


def test_projection_row_rejects_non_finite_rate() -> None:
    with pytest.raises(ValidationError):
        CampaignProjectionRow(
            campaign_id="c",
            campaign_revision="1",
            variant_id="v",
            task_id="t",
            metric_id="m",
            numerator=1,
            denominator=2,
            rate=float("nan"),
            unit="boolean",
            verification_status="verified",
            evidence_link="p.json",
            repetition=1,
        )


def test_disposition_row_rejects_unknown_disposition() -> None:
    with pytest.raises(ValidationError):
        CampaignDispositionRow(
            assignment_id="a",
            disposition="bogus",  # pyright: ignore[reportArgumentType]
            variant_id="v",
            task_id="t",
            reason="",
        )


def test_publication_schema_v4_requires_model_campaign() -> None:
    with pytest.raises(ValidationError):
        PublicationSchemaV4(  # type: ignore[call-arg]
            publication_schema_version=PUBLICATION_SCHEMA_V4,
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
        )


def test_publication_schema_v4_rejects_wrong_version() -> None:
    ref = _make_model_campaign()
    with pytest.raises(ValidationError):
        PublicationSchemaV4(
            publication_schema_version="3.0.0",
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
            model_campaign=ref,
        )
