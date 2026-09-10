# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for per-variant descriptive summary rows and the
variant-summaries generation function.

Verifies that ``CampaignVariantSummaryRow`` is frozen with
``extra="forbid"``, that field constraints are enforced, that
``_generate_variant_summaries`` correctly aggregates per-assignment
projection rows into per-variant pass rates with denominators, and that
the ``ModelCampaignRef`` includes the ``variant_summaries`` field in its
content hash.
"""

# pyright: reportCallIssue=false

from __future__ import annotations

import pytest

pytestmark = pytest.mark.unit

from pydantic import ValidationError

from g8e_evals.publication import (
    CampaignProjectionRow,
    CampaignVariantSummaryRow,
    compute_model_campaign_hash,
)


# --- Model validation tests ---


def test_variant_summary_row_is_frozen() -> None:
    row = CampaignVariantSummaryRow(
        variant_id="variant-a",
        metric_id="ifeval_subset_verifier",
        numerator=10,
        denominator=15,
        rate=10.0 / 15.0,
        unit="boolean",
        task_count=5,
        repetition_count=3,
        terminal_count=15,
        superseded_count=0,
        invalid_count=0,
        missing_count=0,
    )
    with pytest.raises(ValidationError):
        row.numerator = 99  # type: ignore[misc]


def test_variant_summary_row_rejects_unknown_fields() -> None:
    with pytest.raises(ValidationError):
        CampaignVariantSummaryRow(
            variant_id="variant-a",
            metric_id="ifeval_subset_verifier",
            numerator=10,
            denominator=15,
            rate=10.0 / 15.0,
            unit="boolean",
            task_count=5,
            repetition_count=3,
            terminal_count=15,
            superseded_count=0,
            invalid_count=0,
            missing_count=0,
            secret_field="leak",  # type: ignore[call-arg]
        )


def test_variant_summary_row_rejects_negative_numerator() -> None:
    with pytest.raises(ValidationError):
        CampaignVariantSummaryRow(
            variant_id="variant-a",
            metric_id="ifeval_subset_verifier",
            numerator=-1,
            denominator=15,
            rate=0.0,
            unit="boolean",
            task_count=5,
            repetition_count=3,
            terminal_count=15,
            superseded_count=0,
            invalid_count=0,
            missing_count=0,
        )


def test_variant_summary_row_rejects_rate_above_one() -> None:
    with pytest.raises(ValidationError):
        CampaignVariantSummaryRow(
            variant_id="variant-a",
            metric_id="ifeval_subset_verifier",
            numerator=20,
            denominator=15,
            rate=1.5,
            unit="boolean",
            task_count=5,
            repetition_count=3,
            terminal_count=15,
            superseded_count=0,
            invalid_count=0,
            missing_count=0,
        )


def test_variant_summary_row_round_trip() -> None:
    row = CampaignVariantSummaryRow(
        variant_id="variant-a",
        metric_id="ifeval_subset_verifier",
        numerator=10,
        denominator=15,
        rate=10.0 / 15.0,
        unit="boolean",
        task_count=5,
        repetition_count=3,
        terminal_count=15,
        superseded_count=0,
        invalid_count=0,
        missing_count=0,
    )
    restored = CampaignVariantSummaryRow.model_validate_json(row.model_dump_json())
    assert restored == row


# --- _generate_variant_summaries tests ---


def _make_proj(variant: str, task: str, rep: int, value: float) -> CampaignProjectionRow:
    return CampaignProjectionRow(
        campaign_id="campaign-1",
        campaign_revision="1",
        variant_id=variant,
        task_id=task,
        metric_id="ifeval_subset_verifier",
        numerator=int(value),
        denominator=1,
        rate=value,
        unit="boolean",
        verification_status="verified",
        evidence_link=f"proofs/{variant}/{task}/rep-{rep}.json",
        repetition=rep,
    )


def test_generate_variant_summaries_aggregates_per_variant() -> None:
    """Three variants with 2 tasks x 2 repetitions each = 4 assignments per variant."""
    from g8e_evals.publication import _generate_variant_summaries

    projections = [
        _make_proj("variant-a", "task-1", 1, 1.0),
        _make_proj("variant-a", "task-1", 2, 0.0),
        _make_proj("variant-a", "task-2", 1, 1.0),
        _make_proj("variant-a", "task-2", 2, 1.0),
        _make_proj("variant-b", "task-1", 1, 0.0),
        _make_proj("variant-b", "task-1", 2, 0.0),
        _make_proj("variant-b", "task-2", 1, 1.0),
        _make_proj("variant-b", "task-2", 2, 0.0),
        _make_proj("variant-c", "task-1", 1, 1.0),
        _make_proj("variant-c", "task-1", 2, 1.0),
        _make_proj("variant-c", "task-2", 1, 1.0),
        _make_proj("variant-c", "task-2", 2, 1.0),
    ]
    summaries = _generate_variant_summaries(projections)
    assert len(summaries) == 3

    # variant-a: 3/4 = 0.75
    assert summaries[0].variant_id == "variant-a"
    assert summaries[0].numerator == 3
    assert summaries[0].denominator == 4
    assert summaries[0].rate == 0.75
    assert summaries[0].task_count == 2
    assert summaries[0].repetition_count == 2
    assert summaries[0].terminal_count == 4
    assert summaries[0].superseded_count == 0
    assert summaries[0].invalid_count == 0
    assert summaries[0].missing_count == 0

    # variant-b: 1/4 = 0.25
    assert summaries[1].variant_id == "variant-b"
    assert summaries[1].numerator == 1
    assert summaries[1].denominator == 4
    assert summaries[1].rate == 0.25

    # variant-c: 4/4 = 1.0
    assert summaries[2].variant_id == "variant-c"
    assert summaries[2].numerator == 4
    assert summaries[2].denominator == 4
    assert summaries[2].rate == 1.0


def test_generate_variant_summaries_sorted_by_variant_id() -> None:
    from g8e_evals.publication import _generate_variant_summaries

    projections = [
        _make_proj("zebra", "task-1", 1, 1.0),
        _make_proj("alpha", "task-1", 1, 0.0),
        _make_proj("mid", "task-1", 1, 1.0),
    ]
    summaries = _generate_variant_summaries(projections)
    assert [s.variant_id for s in summaries] == ["alpha", "mid", "zebra"]


def test_generate_variant_summaries_empty_projections() -> None:
    from g8e_evals.publication import _generate_variant_summaries

    summaries = _generate_variant_summaries([])
    assert summaries == []


def test_generate_variant_summaries_single_variant_all_pass() -> None:
    from g8e_evals.publication import _generate_variant_summaries

    projections = [
        _make_proj("variant-a", "task-1", 1, 1.0),
        _make_proj("variant-a", "task-1", 2, 1.0),
        _make_proj("variant-a", "task-1", 3, 1.0),
    ]
    summaries = _generate_variant_summaries(projections)
    assert len(summaries) == 1
    assert summaries[0].numerator == 3
    assert summaries[0].denominator == 3
    assert summaries[0].rate == 1.0
    assert summaries[0].task_count == 1
    assert summaries[0].repetition_count == 3


def test_generate_variant_summaries_single_variant_all_fail() -> None:
    from g8e_evals.publication import _generate_variant_summaries

    projections = [
        _make_proj("variant-a", "task-1", 1, 0.0),
        _make_proj("variant-a", "task-2", 1, 0.0),
    ]
    summaries = _generate_variant_summaries(projections)
    assert len(summaries) == 1
    assert summaries[0].numerator == 0
    assert summaries[0].denominator == 2
    assert summaries[0].rate == 0.0
    assert summaries[0].task_count == 2


def test_generate_variant_summaries_multiple_metrics() -> None:
    """When multiple metrics exist, one summary row per variant per metric."""
    from g8e_evals.publication import _generate_variant_summaries

    projections = [
        CampaignProjectionRow(
            campaign_id="campaign-1",
            campaign_revision="1",
            variant_id="variant-a",
            task_id="task-1",
            metric_id="metric-x",
            numerator=1,
            denominator=1,
            rate=1.0,
            unit="boolean",
            verification_status="verified",
            evidence_link="proofs/variant-a/task-1/rep-1.json",
            repetition=1,
        ),
        CampaignProjectionRow(
            campaign_id="campaign-1",
            campaign_revision="1",
            variant_id="variant-a",
            task_id="task-1",
            metric_id="metric-y",
            numerator=0,
            denominator=1,
            rate=0.0,
            unit="boolean",
            verification_status="verified",
            evidence_link="proofs/variant-a/task-1/rep-1.json",
            repetition=1,
        ),
    ]
    summaries = _generate_variant_summaries(projections)
    assert len(summaries) == 2
    assert summaries[0].metric_id == "metric-x"
    assert summaries[1].metric_id == "metric-y"


# --- ModelCampaignRef includes variant_summaries in hash ---


def test_variant_summaries_included_in_model_campaign_hash() -> None:
    """Changing variant_summaries changes the content hash."""
    from g8e_evals.publication import (
        CampaignProvenanceRef,
        CampaignStatisticalAnalysisRef,
        SafeCampaignProfileProjection,
        SafeModelVariantProjection,
    )

    _HASH = "a" * 64

    profile = SafeCampaignProfileProjection(
        campaign_id="campaign-1",
        campaign_revision="1",
        content_hash=_HASH,
        benchmark_ids=["ifeval_subset"],
        task_ids=["task-1"],
        repetitions=1,
        hardware_identity="linux/amd64/cpu",
        claim_boundary="descriptive_only",
    )
    variant = SafeModelVariantProjection(
        variant_id="variant-a",
        canonical_display_name="Variant A",
        weight_class="heavy-slm",
        parameter_count_display="8.0B",
        backend_name="ollama",
        served_model_tag="variant-a:latest",
        artifact_digest=_HASH,
        quantization="Q4_K_M",
        publication_eligibility="eligible",
    )
    stat_ref = CampaignStatisticalAnalysisRef(
        method="descriptive",
        independent_unit="task",
        population=1,
        correction_family="none",
        claim_status="descriptive_only",
        content_hash=_HASH,
    )
    prov_ref = CampaignProvenanceRef(
        manifest_hash=_HASH,
        entry_count=1,
        schema_version="1.0.0",
    )

    summary_a = CampaignVariantSummaryRow(
        variant_id="variant-a",
        metric_id="ifeval_subset_verifier",
        numerator=1,
        denominator=1,
        rate=1.0,
        unit="boolean",
        task_count=1,
        repetition_count=1,
        terminal_count=1,
        superseded_count=0,
        invalid_count=0,
        missing_count=0,
    )
    summary_b = CampaignVariantSummaryRow(
        variant_id="variant-a",
        metric_id="ifeval_subset_verifier",
        numerator=0,
        denominator=1,
        rate=0.0,
        unit="boolean",
        task_count=1,
        repetition_count=1,
        terminal_count=1,
        superseded_count=0,
        invalid_count=0,
        missing_count=0,
    )

    common_kwargs = {
        "campaign_id": "campaign-1",
        "campaign_revision": "1",
        "publication_schema_version": "4.0.0",
        "campaign_profile": profile,
        "model_variants": [variant],
        "model_registry_hash": _HASH,
        "verification_ok": True,
        "verified_index_generation_hash": _HASH,
        "checked_layers": ["file_safety"],
        "projections": [],
        "dispositions": [],
        "comparison_rows": [],
        "efficiency_observations": [],
        "statistical_analysis": stat_ref,
        "provenance": prov_ref,
        "caveats": ["descriptive_only"],
    }

    hash_with_a = compute_model_campaign_hash(variant_summaries=[summary_a], **common_kwargs)
    hash_with_b = compute_model_campaign_hash(variant_summaries=[summary_b], **common_kwargs)
    hash_empty = compute_model_campaign_hash(variant_summaries=[], **common_kwargs)

    assert hash_with_a != hash_with_b
    assert hash_with_a != hash_empty
    assert hash_with_b != hash_empty
