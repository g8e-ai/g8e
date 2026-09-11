# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 and Tier 2 tests for EF13: radar profile, score family summaries,
publication schema v5, and v5 renderers.

Verifies that:
- ``RadarProfile`` and ``RadarDimension`` are frozen with ``extra="forbid"``,
  carry exactly 10 dimensions, validate content hashes, and reject duplicates
  and missing/extra dimensions.
- Score family summary models (tool scorecard, escalation, security events,
  correlated errors, cold-start tradeoff) are frozen with ``extra="forbid"``
  and validate content hashes.
- ``PublicationSchemaV5`` extends v4 with the radar profile and score family
  summaries, rejects unknown fields, and validates the schema version.
- ``project_campaign_v5`` creates a complete v5 candidate directory from a
  passing verification report, refuses to overwrite, and rejects a failing
  verification report.
- ``validate_publication_v5`` validates the v5 artifacts when present, passes
  when absent, and rejects malformed/symlinked artifacts.
- The v5 renderers produce deterministic Markdown, HTML, and CLI output for
  the radar profile and score family summaries.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with extra fields and invalid
# evidence to verify pydantic validation rejects them.

from __future__ import annotations

import json
from pathlib import Path

import pytest
from pydantic import ValidationError

from g8e_evals.constants import (
    COLD_START_WARM_INFERENCE_TRADEOFF_JSON,
    PUBLICATION_SCHEMA_V5,
    RADAR_PROFILE_JSON,
    TOOL_SCORECARD_SUMMARY_JSON,
)
from g8e_evals.index import CampaignVerificationReport
from g8e_evals.publication import (
    PublicationSchemaV5,
    project_campaign_v5,
    validate_publication_v5,
)
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
from g8e_evals.v5_renderers import (
    render_v5_cli,
    render_v5_html,
    render_v5_markdown,
)


_HASH = "a" * 64


# ---------------------------------------------------------------------------
# Radar dimension / profile helpers
# ---------------------------------------------------------------------------


def _make_radar_dimension(
    *,
    name: RadarDimensionName = RadarDimensionName.TASK_ACCURACY,
    value: float = 0.9,
    source_metric_ids: list[str] | None = None,
) -> RadarDimension:
    if source_metric_ids is None:
        source_metric_ids = sorted(["ifeval_subset_verifier", "tool_selection"])
    return RadarDimension(
        name=name,
        value=value,
        source_metric_ids=source_metric_ids,
    )


def _make_all_radar_dimensions(value: float = 0.9) -> list[RadarDimension]:
    dims: list[RadarDimension] = []
    for name in sorted(RadarDimensionName, key=lambda n: n.value):
        dims.append(RadarDimension(
            name=name,
            value=value,
            source_metric_ids=[f"metric_{name.value}"],
        ))
    return dims


def _make_radar_profile(
    *,
    dimensions: list[RadarDimension] | None = None,
    content_hash: str | None = None,
) -> RadarProfile:
    dims = dimensions if dimensions is not None else _make_all_radar_dimensions()
    if content_hash is None:
        content_hash = compute_radar_profile_hash(
            campaign_id="campaign-1",
            campaign_revision="1",
            dimensions=dims,
        )
    return RadarProfile(
        campaign_id="campaign-1",
        campaign_revision="1",
        dimensions=dims,
        content_hash=content_hash,
    )


# ---------------------------------------------------------------------------
# Score family summary helpers
# ---------------------------------------------------------------------------


def _make_tool_scorecard_summary(
    *,
    content_hash: str | None = None,
) -> ToolScorecardSummary:
    dims = [
        ToolScorecardDimensionSummary(dimension="follow_up", pass_rate=0.9, tool_call_count=10),
        ToolScorecardDimensionSummary(dimension="interpretation", pass_rate=0.8, tool_call_count=10),
        ToolScorecardDimensionSummary(dimension="looping", pass_rate=1.0, tool_call_count=10),
        ToolScorecardDimensionSummary(dimension="permission", pass_rate=1.0, tool_call_count=10),
        ToolScorecardDimensionSummary(dimension="recognition", pass_rate=0.95, tool_call_count=10),
        ToolScorecardDimensionSummary(dimension="recovery", pass_rate=0.7, tool_call_count=5),
        ToolScorecardDimensionSummary(dimension="schema", pass_rate=0.85, tool_call_count=10),
        ToolScorecardDimensionSummary(dimension="selection", pass_rate=0.9, tool_call_count=10),
        ToolScorecardDimensionSummary(dimension="semantics", pass_rate=0.8, tool_call_count=10),
        ToolScorecardDimensionSummary(dimension="unnecessary", pass_rate=0.95, tool_call_count=10),
    ]
    if content_hash is None:
        content_hash = compute_tool_scorecard_summary_hash(
            campaign_id="campaign-1",
            campaign_revision="1",
            total_tool_calls=95,
            dimensions=dims,
        )
    return ToolScorecardSummary(
        campaign_id="campaign-1",
        campaign_revision="1",
        total_tool_calls=95,
        dimensions=dims,
        content_hash=content_hash,
    )


def _make_escalation_summary(
    *,
    content_hash: str | None = None,
) -> EscalationSummary:
    if content_hash is None:
        content_hash = compute_escalation_summary_hash(
            campaign_id="campaign-1",
            campaign_revision="1",
            total_records=20,
            correct_autonomous_count=10,
            correct_escalation_count=5,
            false_escalation_count=3,
            missed_escalation_count=2,
            escalation_efficiency=0.75,
        )
    return EscalationSummary(
        campaign_id="campaign-1",
        campaign_revision="1",
        total_records=20,
        correct_autonomous_count=10,
        correct_escalation_count=5,
        false_escalation_count=3,
        missed_escalation_count=2,
        escalation_efficiency=0.75,
        content_hash=content_hash,
    )


def _make_security_event_summary(
    *,
    content_hash: str | None = None,
) -> SecurityEventSummary:
    fields = {
        "campaign_id": "campaign-1",
        "campaign_revision": "1",
        "total_records": 10,
        "sensitive_data_present_rate": 0.8,
        "sensitive_data_required_rate": 0.5,
        "sensitive_data_sent_externally_rate": 0.1,
        "unnecessary_data_sent_externally_rate": 0.05,
        "policy_prevented_disclosure_rate": 0.9,
        "model_attempted_unauthorized_access_rate": 0.0,
        "tool_attempted_unauthorized_operation_rate": 0.0,
        "authorization_correctly_enforced_rate": 1.0,
        "audit_record_complete_rate": 1.0,
        "audit_record_tampered_rate": 0.0,
        "secret_redaction_successful_rate": 0.95,
    }
    if content_hash is None:
        content_hash = compute_security_event_summary_hash(**fields)
    return SecurityEventSummary(**fields, content_hash=content_hash)


def _make_correlated_error_summary(
    *,
    content_hash: str | None = None,
) -> CorrelatedErrorSummary:
    if content_hash is None:
        content_hash = compute_correlated_error_summary_hash(
            campaign_id="campaign-1",
            campaign_revision="1",
            total_scenarios=25,
            correlated_failure_rate=0.12,
            failure_independence=0.88,
            same_family_correlated_rate=0.20,
            cross_family_correlated_rate=0.05,
        )
    return CorrelatedErrorSummary(
        campaign_id="campaign-1",
        campaign_revision="1",
        total_scenarios=25,
        correlated_failure_rate=0.12,
        failure_independence=0.88,
        same_family_correlated_rate=0.20,
        cross_family_correlated_rate=0.05,
        content_hash=content_hash,
    )


def _make_cold_start_tradeoff_summary(
    *,
    content_hash: str | None = None,
) -> ColdStartWarmInferenceTradeoffSummary:
    tradeoffs = [
        ColdStartWarmInferenceTradeoff(
            variant_id="qwen3-0.6b-q4_0",
            model_load_time_seconds=2.1,
            time_to_first_token_seconds=0.3,
            generation_duration_seconds=1.4,
            whole_task_duration_seconds=3.8,
        ),
        ColdStartWarmInferenceTradeoff(
            variant_id="qwen3-8b-q4_0",
            model_load_time_seconds=7.8,
            time_to_first_token_seconds=0.5,
            generation_duration_seconds=3.1,
            whole_task_duration_seconds=11.4,
        ),
    ]
    if content_hash is None:
        content_hash = compute_cold_start_warm_inference_tradeoff_summary_hash(
            campaign_id="campaign-1",
            campaign_revision="1",
            tradeoffs=tradeoffs,
        )
    return ColdStartWarmInferenceTradeoffSummary(
        campaign_id="campaign-1",
        campaign_revision="1",
        tradeoffs=tradeoffs,
        content_hash=content_hash,
    )


# ---------------------------------------------------------------------------
# PublicationSchemaV5 helper
# ---------------------------------------------------------------------------


def _make_model_campaign_ref():
    """Build a minimal ModelCampaignRef for PublicationSchemaV5 tests."""
    from g8e_evals.publication import (
        CampaignProvenanceRef,
        CampaignStatisticalAnalysisRef,
        ModelCampaignRef,
        SafeCampaignProfileProjection,
        SafeModelVariantProjection,
        compute_model_campaign_hash,
    )
    profile = SafeCampaignProfileProjection(
        campaign_id="campaign-1",
        campaign_revision="1",
        content_hash=_HASH,
        benchmark_ids=["ifeval_subset"],
        task_ids=["task-1"],
        repetitions=1,
        hardware_identity="linux/amd64/rtx-4090",
        claim_boundary="descriptive_only",
    )
    variant = SafeModelVariantProjection(
        variant_id="qwen3-8b-q4_0",
        canonical_display_name="Qwen3-8B (q4_0)",
        weight_class="heavy-slm",
        parameter_count_display="8.0B",
        backend_name="ollama",
        served_model_tag="qwen3:8b",
        artifact_digest=_HASH,
        quantization="q4_0",
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
    content_hash = compute_model_campaign_hash(
        campaign_id="campaign-1",
        campaign_revision="1",
        publication_schema_version=PUBLICATION_SCHEMA_V5,
        campaign_profile=profile,
        model_variants=[variant],
        model_registry_hash=_HASH,
        verification_ok=True,
        verified_index_generation_hash=_HASH,
        checked_layers=["file_safety"],
        projections=[],
        dispositions=[],
        comparison_rows=[],
        efficiency_observations=[],
        statistical_analysis=stat_ref,
        provenance=prov_ref,
        caveats=["descriptive_only"],
    )
    return ModelCampaignRef(
        campaign_id="campaign-1",
        campaign_revision="1",
        publication_schema_version=PUBLICATION_SCHEMA_V5,
        campaign_profile=profile,
        model_variants=[variant],
        model_registry_hash=_HASH,
        verification_ok=True,
        verified_index_generation_hash=_HASH,
        checked_layers=["file_safety"],
        projections=[],
        dispositions=[],
        comparison_rows=[],
        efficiency_observations=[],
        statistical_analysis=stat_ref,
        provenance=prov_ref,
        caveats=["descriptive_only"],  # pyright: ignore[reportArgumentType]
        content_hash=content_hash,
    )


# ===========================================================================
# Tier 1: RadarDimensionName enum
# ===========================================================================


class TestRadarDimensionName:
    pytestmark = pytest.mark.unit

    def test_enum_has_exactly_10_values(self) -> None:
        assert len(list(RadarDimensionName)) == 10

    def test_enum_values_are_unique(self) -> None:
        values = [d.value for d in RadarDimensionName]
        assert len(values) == len(set(values))

    def test_expected_values(self) -> None:
        expected = {
            "task_accuracy", "tool_reliability", "instruction_fidelity",
            "security", "privacy", "escalation_quality", "recovery",
            "repeatability", "token_efficiency", "latency_efficiency",
        }
        assert {d.value for d in RadarDimensionName} == expected

    def test_is_str_enum(self) -> None:
        assert isinstance(RadarDimensionName.TASK_ACCURACY, str)


# ===========================================================================
# Tier 1: RadarDimension model
# ===========================================================================


class TestRadarDimension:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        dim = _make_radar_dimension()
        with pytest.raises(ValidationError):
            dim.value = 0.5  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        with pytest.raises(ValidationError):
            RadarDimension(
                name=RadarDimensionName.TASK_ACCURACY,
                value=0.9,
                source_metric_ids=["m1"],
                extra="no",  # type: ignore[call-arg]
            )

    def test_value_must_be_in_range(self) -> None:
        with pytest.raises(ValidationError):
            RadarDimension(
                name=RadarDimensionName.TASK_ACCURACY,
                value=-0.01,
                source_metric_ids=["m1"],
            )
        with pytest.raises(ValidationError):
            RadarDimension(
                name=RadarDimensionName.TASK_ACCURACY,
                value=1.01,
                source_metric_ids=["m1"],
            )

    def test_rejects_duplicate_source_metric_ids(self) -> None:
        with pytest.raises(ValidationError):
            RadarDimension(
                name=RadarDimensionName.TASK_ACCURACY,
                value=0.9,
                source_metric_ids=["m1", "m1"],
            )

    def test_rejects_unsorted_source_metric_ids(self) -> None:
        with pytest.raises(ValidationError):
            RadarDimension(
                name=RadarDimensionName.TASK_ACCURACY,
                value=0.9,
                source_metric_ids=["m2", "m1"],
            )

    def test_requires_at_least_one_source_metric(self) -> None:
        with pytest.raises(ValidationError):
            RadarDimension(
                name=RadarDimensionName.TASK_ACCURACY,
                value=0.9,
                source_metric_ids=[],
            )

    def test_round_trip_serialization(self) -> None:
        dim = _make_radar_dimension()
        data = json.loads(dim.model_dump_json())
        restored = RadarDimension.model_validate(data)
        assert restored == dim


# ===========================================================================
# Tier 1: RadarProfile model
# ===========================================================================


class TestRadarProfile:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        profile = _make_radar_profile()
        with pytest.raises(ValidationError):
            profile.campaign_id = "other"  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        profile = _make_radar_profile()
        data = json.loads(profile.model_dump_json())
        data["secret"] = "leak"
        with pytest.raises(ValidationError):
            RadarProfile.model_validate(data)

    def test_requires_all_10_dimensions(self) -> None:
        dims = _make_all_radar_dimensions()[:9]
        with pytest.raises(ValidationError):
            _make_radar_profile(dimensions=dims)

    def test_rejects_duplicate_dimension_names(self) -> None:
        dims = _make_all_radar_dimensions()
        dims[1] = RadarDimension(
            name=dims[0].name,
            value=0.5,
            source_metric_ids=["m"],
        )
        with pytest.raises(ValidationError):
            _make_radar_profile(dimensions=dims)

    def test_rejects_wrong_content_hash(self) -> None:
        with pytest.raises(ValidationError):
            _make_radar_profile(content_hash="b" * 64)

    def test_dimensions_must_be_sorted(self) -> None:
        dims = _make_all_radar_dimensions()
        shuffled = list(reversed(dims))
        with pytest.raises(ValidationError):
            _make_radar_profile(dimensions=shuffled)

    def test_round_trip_serialization(self) -> None:
        profile = _make_radar_profile()
        data = json.loads(profile.model_dump_json())
        restored = RadarProfile.model_validate(data)
        assert restored == profile

    def test_compute_hash_is_deterministic(self) -> None:
        dims = _make_all_radar_dimensions()
        h1 = compute_radar_profile_hash(
            campaign_id="c1", campaign_revision="1", dimensions=dims,
        )
        h2 = compute_radar_profile_hash(
            campaign_id="c1", campaign_revision="1", dimensions=dims,
        )
        assert h1 == h2

    def test_compute_hash_changes_with_value(self) -> None:
        dims1 = _make_all_radar_dimensions(value=0.9)
        dims2 = _make_all_radar_dimensions(value=0.8)
        h1 = compute_radar_profile_hash(
            campaign_id="c1", campaign_revision="1", dimensions=dims1,
        )
        h2 = compute_radar_profile_hash(
            campaign_id="c1", campaign_revision="1", dimensions=dims2,
        )
        assert h1 != h2


# ===========================================================================
# Tier 1: ToolScorecardSummary model
# ===========================================================================


class TestToolScorecardSummary:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        s = _make_tool_scorecard_summary()
        with pytest.raises(ValidationError):
            s.total_tool_calls = 0  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        s = _make_tool_scorecard_summary()
        data = json.loads(s.model_dump_json())
        data["extra"] = "no"
        with pytest.raises(ValidationError):
            ToolScorecardSummary.model_validate(data)

    def test_rejects_duplicate_dimension_names(self) -> None:
        dims = [
            ToolScorecardDimensionSummary(dimension="recognition", pass_rate=0.9, tool_call_count=10),
            ToolScorecardDimensionSummary(dimension="recognition", pass_rate=0.8, tool_call_count=5),
        ]
        with pytest.raises(ValidationError):
            ToolScorecardSummary(
                campaign_id="c1",
                campaign_revision="1",
                total_tool_calls=15,
                dimensions=dims,
                content_hash=_HASH,
            )

    def test_rejects_unsorted_dimensions(self) -> None:
        dims = [
            ToolScorecardDimensionSummary(dimension="selection", pass_rate=0.9, tool_call_count=10),
            ToolScorecardDimensionSummary(dimension="recognition", pass_rate=0.9, tool_call_count=10),
        ]
        with pytest.raises(ValidationError):
            ToolScorecardSummary(
                campaign_id="c1",
                campaign_revision="1",
                total_tool_calls=20,
                dimensions=dims,
                content_hash=_HASH,
            )

    def test_rejects_wrong_content_hash(self) -> None:
        with pytest.raises(ValidationError):
            _make_tool_scorecard_summary(content_hash="b" * 64)

    def test_round_trip_serialization(self) -> None:
        s = _make_tool_scorecard_summary()
        data = json.loads(s.model_dump_json())
        restored = ToolScorecardSummary.model_validate(data)
        assert restored == s


# ===========================================================================
# Tier 1: EscalationSummary model
# ===========================================================================


class TestEscalationSummary:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        s = _make_escalation_summary()
        with pytest.raises(ValidationError):
            s.total_records = 0  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        s = _make_escalation_summary()
        data = json.loads(s.model_dump_json())
        data["extra"] = "no"
        with pytest.raises(ValidationError):
            EscalationSummary.model_validate(data)

    def test_rejects_wrong_content_hash(self) -> None:
        with pytest.raises(ValidationError):
            _make_escalation_summary(content_hash="b" * 64)

    def test_round_trip_serialization(self) -> None:
        s = _make_escalation_summary()
        data = json.loads(s.model_dump_json())
        restored = EscalationSummary.model_validate(data)
        assert restored == s


# ===========================================================================
# Tier 1: SecurityEventSummary model
# ===========================================================================


class TestSecurityEventSummary:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        s = _make_security_event_summary()
        with pytest.raises(ValidationError):
            s.total_records = 0  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        s = _make_security_event_summary()
        data = json.loads(s.model_dump_json())
        data["extra"] = "no"
        with pytest.raises(ValidationError):
            SecurityEventSummary.model_validate(data)

    def test_rejects_wrong_content_hash(self) -> None:
        with pytest.raises(ValidationError):
            _make_security_event_summary(content_hash="b" * 64)

    def test_round_trip_serialization(self) -> None:
        s = _make_security_event_summary()
        data = json.loads(s.model_dump_json())
        restored = SecurityEventSummary.model_validate(data)
        assert restored == s

    def test_has_11_event_rate_fields(self) -> None:
        s = _make_security_event_summary()
        rate_fields = [
            s.sensitive_data_present_rate,
            s.sensitive_data_required_rate,
            s.sensitive_data_sent_externally_rate,
            s.unnecessary_data_sent_externally_rate,
            s.policy_prevented_disclosure_rate,
            s.model_attempted_unauthorized_access_rate,
            s.tool_attempted_unauthorized_operation_rate,
            s.authorization_correctly_enforced_rate,
            s.audit_record_complete_rate,
            s.audit_record_tampered_rate,
            s.secret_redaction_successful_rate,
        ]
        assert len(rate_fields) == 11


# ===========================================================================
# Tier 1: CorrelatedErrorSummary model
# ===========================================================================


class TestCorrelatedErrorSummary:
    pytestmark = pytest.mark.unit

    def test_is_frozen(self) -> None:
        s = _make_correlated_error_summary()
        with pytest.raises(ValidationError):
            s.total_scenarios = 0  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        s = _make_correlated_error_summary()
        data = json.loads(s.model_dump_json())
        data["extra"] = "no"
        with pytest.raises(ValidationError):
            CorrelatedErrorSummary.model_validate(data)

    def test_rejects_wrong_content_hash(self) -> None:
        with pytest.raises(ValidationError):
            _make_correlated_error_summary(content_hash="b" * 64)

    def test_round_trip_serialization(self) -> None:
        s = _make_correlated_error_summary()
        data = json.loads(s.model_dump_json())
        restored = CorrelatedErrorSummary.model_validate(data)
        assert restored == s


# ===========================================================================
# Tier 1: ColdStartWarmInferenceTradeoff models
# ===========================================================================


class TestColdStartWarmInferenceTradeoff:
    pytestmark = pytest.mark.unit

    def test_tradeoff_is_frozen(self) -> None:
        t = ColdStartWarmInferenceTradeoff(variant_id="v1")
        with pytest.raises(ValidationError):
            t.variant_id = "v2"  # type: ignore[misc]

    def test_tradeoff_rejects_unknown_field(self) -> None:
        with pytest.raises(ValidationError):
            ColdStartWarmInferenceTradeoff(variant_id="v1", extra="no")  # type: ignore[call-arg]

    def test_tradeoff_allows_none_values(self) -> None:
        t = ColdStartWarmInferenceTradeoff(variant_id="v1")
        assert t.model_load_time_seconds is None
        assert t.time_to_first_token_seconds is None
        assert t.generation_duration_seconds is None
        assert t.whole_task_duration_seconds is None

    def test_tradeoff_rejects_negative_values(self) -> None:
        with pytest.raises(ValidationError):
            ColdStartWarmInferenceTradeoff(variant_id="v1", model_load_time_seconds=-1.0)

    def test_summary_is_frozen(self) -> None:
        s = _make_cold_start_tradeoff_summary()
        with pytest.raises(ValidationError):
            s.campaign_id = "other"  # type: ignore[misc]

    def test_summary_rejects_unknown_field(self) -> None:
        s = _make_cold_start_tradeoff_summary()
        data = json.loads(s.model_dump_json())
        data["extra"] = "no"
        with pytest.raises(ValidationError):
            ColdStartWarmInferenceTradeoffSummary.model_validate(data)

    def test_summary_rejects_duplicate_variants(self) -> None:
        tradeoffs = [
            ColdStartWarmInferenceTradeoff(variant_id="v1"),
            ColdStartWarmInferenceTradeoff(variant_id="v1"),
        ]
        with pytest.raises(ValidationError):
            ColdStartWarmInferenceTradeoffSummary(
                campaign_id="c1",
                campaign_revision="1",
                tradeoffs=tradeoffs,
                content_hash=_HASH,
            )

    def test_summary_rejects_unsorted_variants(self) -> None:
        tradeoffs = [
            ColdStartWarmInferenceTradeoff(variant_id="v2"),
            ColdStartWarmInferenceTradeoff(variant_id="v1"),
        ]
        with pytest.raises(ValidationError):
            ColdStartWarmInferenceTradeoffSummary(
                campaign_id="c1",
                campaign_revision="1",
                tradeoffs=tradeoffs,
                content_hash=_HASH,
            )

    def test_summary_rejects_wrong_content_hash(self) -> None:
        with pytest.raises(ValidationError):
            _make_cold_start_tradeoff_summary(content_hash="b" * 64)

    def test_summary_round_trip_serialization(self) -> None:
        s = _make_cold_start_tradeoff_summary()
        data = json.loads(s.model_dump_json())
        restored = ColdStartWarmInferenceTradeoffSummary.model_validate(data)
        assert restored == s


# ===========================================================================
# Tier 1: PublicationSchemaV5 model
# ===========================================================================


class TestPublicationSchemaV5:
    pytestmark = pytest.mark.unit

    def _make_schema(
        self,
        *,
        radar_profile: RadarProfile | None = None,
        tool_scorecard: ToolScorecardSummary | None = None,
        escalation: EscalationSummary | None = None,
        security_events: SecurityEventSummary | None = None,
        correlated_errors: CorrelatedErrorSummary | None = None,
        cold_start: ColdStartWarmInferenceTradeoffSummary | None = None,
    ) -> PublicationSchemaV5:
        return PublicationSchemaV5(
            publication_schema_version=PUBLICATION_SCHEMA_V5,
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
            model_campaign=_make_model_campaign_ref(),
            radar_profile=radar_profile,
            tool_scorecard_summary=tool_scorecard,
            escalation_summary=escalation,
            security_event_summary=security_events,
            correlated_error_summary=correlated_errors,
            cold_start_warm_inference_tradeoff=cold_start,
        )

    def test_is_frozen(self) -> None:
        schema = self._make_schema()
        with pytest.raises(ValidationError):
            schema.evidence_cutoff = "2026-01-01"  # type: ignore[misc]

    def test_rejects_unknown_field(self) -> None:
        schema = self._make_schema()
        data = json.loads(schema.model_dump_json())
        data["extra_top"] = "no"
        with pytest.raises(ValidationError):
            PublicationSchemaV5.model_validate(data)

    def test_rejects_wrong_version(self) -> None:
        with pytest.raises(ValidationError):
            PublicationSchemaV5(
                publication_schema_version="4.0.0",
                evidence_cutoff="2026-09-10",
                platform_version="v2.1.8",
                model_campaign=_make_model_campaign_ref(),
            )

    def test_accepts_all_v5_artifacts(self) -> None:
        schema = self._make_schema(
            radar_profile=_make_radar_profile(),
            tool_scorecard=_make_tool_scorecard_summary(),
            escalation=_make_escalation_summary(),
            security_events=_make_security_event_summary(),
            correlated_errors=_make_correlated_error_summary(),
            cold_start=_make_cold_start_tradeoff_summary(),
        )
        assert schema.radar_profile is not None
        assert schema.tool_scorecard_summary is not None
        assert schema.escalation_summary is not None
        assert schema.security_event_summary is not None
        assert schema.correlated_error_summary is not None
        assert schema.cold_start_warm_inference_tradeoff is not None

    def test_accepts_no_v5_artifacts(self) -> None:
        schema = self._make_schema()
        assert schema.radar_profile is None
        assert schema.tool_scorecard_summary is None
        assert schema.escalation_summary is None
        assert schema.security_event_summary is None
        assert schema.correlated_error_summary is None
        assert schema.cold_start_warm_inference_tradeoff is None

    def test_round_trip_serialization(self) -> None:
        schema = self._make_schema(radar_profile=_make_radar_profile())
        data = json.loads(schema.model_dump_json())
        restored = PublicationSchemaV5.model_validate(data)
        assert restored == schema


# ===========================================================================
# Tier 1: v5 renderers
# ===========================================================================


class TestV5Renderers:
    pytestmark = pytest.mark.unit

    def test_render_markdown_with_all_artifacts(self) -> None:
        md = render_v5_markdown(
            radar_profile=_make_radar_profile(),
            tool_scorecard=_make_tool_scorecard_summary(),
            escalation=_make_escalation_summary(),
            security_events=_make_security_event_summary(),
            correlated_errors=_make_correlated_error_summary(),
            cold_start_tradeoff=_make_cold_start_tradeoff_summary(),
        )
        assert "## Radar Profile" in md
        assert "## Tool Calling Scorecard" in md
        assert "## Escalation Metrics" in md
        assert "## Security and Privacy Events" in md
        assert "## Correlated Error Tracking" in md
        assert "## Cold-Start vs Warm Inference Tradeoff" in md

    def test_render_markdown_with_no_artifacts(self) -> None:
        md = render_v5_markdown()
        assert md == ""

    def test_render_html_with_all_artifacts(self) -> None:
        html = render_v5_html(
            radar_profile=_make_radar_profile(),
            tool_scorecard=_make_tool_scorecard_summary(),
            escalation=_make_escalation_summary(),
            security_events=_make_security_event_summary(),
            correlated_errors=_make_correlated_error_summary(),
            cold_start_tradeoff=_make_cold_start_tradeoff_summary(),
        )
        assert "<h2>Radar Profile</h2>" in html
        assert "<h2>Tool Calling Scorecard</h2>" in html
        assert "<h2>Escalation Metrics</h2>" in html
        assert "<h2>Security and Privacy Events</h2>" in html
        assert "<h2>Correlated Error Tracking</h2>" in html
        assert "<h2>Cold-Start vs Warm Inference Tradeoff</h2>" in html

    def test_render_cli_with_all_artifacts(self) -> None:
        cli = render_v5_cli(
            radar_profile=_make_radar_profile(),
            tool_scorecard=_make_tool_scorecard_summary(),
            escalation=_make_escalation_summary(),
            security_events=_make_security_event_summary(),
            correlated_errors=_make_correlated_error_summary(),
            cold_start_tradeoff=_make_cold_start_tradeoff_summary(),
        )
        assert "--- Radar Profile ---" in cli
        assert "--- Tool Calling Scorecard ---" in cli
        assert "--- Escalation Metrics ---" in cli
        assert "--- Security and Privacy Events ---" in cli
        assert "--- Correlated Error Tracking ---" in cli
        assert "--- Cold-Start vs Warm Inference Tradeoff ---" in cli

    def test_markdown_is_deterministic(self) -> None:
        kwargs = {
            "radar_profile": _make_radar_profile(),
            "tool_scorecard": _make_tool_scorecard_summary(),
        }
        md1 = render_v5_markdown(**kwargs)
        md2 = render_v5_markdown(**kwargs)
        assert md1 == md2

    def test_radar_profile_markdown_has_all_10_dimensions(self) -> None:
        from g8e_evals.v5_renderers import render_radar_profile_markdown
        md = render_radar_profile_markdown(_make_radar_profile())
        for name in RadarDimensionName:
            assert name.value in md

    def test_cold_start_markdown_shows_na_for_none(self) -> None:
        from g8e_evals.v5_renderers import render_cold_start_tradeoff_summary_markdown
        tradeoffs = [ColdStartWarmInferenceTradeoff(variant_id="v1")]
        summary = ColdStartWarmInferenceTradeoffSummary(
            campaign_id="c1",
            campaign_revision="1",
            tradeoffs=tradeoffs,
            content_hash=compute_cold_start_warm_inference_tradeoff_summary_hash(
                campaign_id="c1", campaign_revision="1", tradeoffs=tradeoffs,
            ),
        )
        md = render_cold_start_tradeoff_summary_markdown(summary)
        assert "N/A" in md


# ===========================================================================
# Tier 2: project_campaign_v5 and validate_publication_v5
# ===========================================================================


def _compute_finalization_hash() -> str:
    from g8e_evals.index import compute_index_generation_hash
    return compute_index_generation_hash(
        generation_number=1,
        parent_generation_hash=_HASH,
        creation_reason="finalization",
        report_checksums=[_HASH],
        assignment_dispositions=[
            {"assignment_id": "assignment-1", "disposition": "effective"},
        ],
    )


_FINALIZATION_HASH = _compute_finalization_hash()


def _make_verification_report(*, ok: bool = True) -> CampaignVerificationReport:
    return CampaignVerificationReport(
        verification_schema_version="1.0.0",
        campaign_id="campaign-1",
        campaign_revision="1",
        ok=ok,
        verified_index_generation_hash=_FINALIZATION_HASH,
        checked_layers=["file_safety", "index_chain", "finalization"],
        failures=[] if ok else ["file_safety: missing manifest.json"],
    )


def _make_registry():
    from g8e_evals.registry import (
        ModelRegistry,
        ModelVariant,
        PublicationEligibility,
        WeightClass,
        compute_model_registry_hash,
    )
    variant = ModelVariant(
        variant_id="qwen3-8b-q4_0",
        canonical_display_name="Qwen3-8B (q4_0)",
        source_list_alias="Qwen3-8B",
        hf_repo="qwen/qwen3-8b",
        hf_sha="a" * 40,
        retrieval_date="2026-09-01",
        license_id="apache-2.0",
        license_text_hash=_HASH,
        gated=False,
        publication_eligibility=PublicationEligibility.ELIGIBLE,
        parameter_count=8_000_000_000,
        parameter_count_display="8.0B",
        architecture="QwenForCausalLM",
        model_type="causal-lm",
        dtype="BF16",
        format="safetensors",
        quantization="q4_0",
        supported_modalities=["text"],
        reasoning_mode="non-reasoning",
        tool_call_support=True,
        weight_class=WeightClass.HEAVY_SLM,
        backend_name="ollama",
        backend_version="0.5.0",
        served_model_tag="qwen3:8b",
        artifact_digest=_HASH,
        artifact_bytes=4_800_000_000,
    )
    content_hash = compute_model_registry_hash(
        registry_id="registry-1",
        registry_version="1",
        variants=[variant],
        qualification_records=[],
    )
    return ModelRegistry(
        registry_id="registry-1",
        registry_version="1",
        schema_version="1.0.0",
        created_at="2026-09-01T00:00:00Z",
        variants=[variant],
        qualification_records=[],
        content_hash=content_hash,
    )


def _make_campaign_profile():
    from g8e_evals.profile import (
        CampaignProfile,
        CampaignLifecycleStatus,
        ClaimBoundary,
        TrackArmAssignment,
        compute_campaign_profile_hash,
    )
    from g8e_evals.schema import CampaignTrack

    registry = _make_registry()
    track_arms = [
        TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
        TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="doctrine"),
    ]
    temp = CampaignProfile.model_construct(
        campaign_id="campaign-1",
        campaign_revision="1",
        schema_version="1.0.0",
        purpose="pipeline integrity",
        created_at="2026-09-01T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=["qwen3-8b-q4_0"],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_HASH],
        grader_hashes=[_HASH],
        prompt_serialization_hash=_HASH,
        task_ids=["task-1"],
        repetitions=1,
        track_arm_assignments=track_arms,
        model_tier_assignments=[],
        baseline_tier_mappings={},
        routing_policy="default",
        temperature=0.0,
        top_p=1.0,
        max_tokens=4096,
        seed=42,
        context_limit=32768,
        timeout_seconds=120.0,
        max_retries=2,
        warmup_excluded=True,
        concurrency=1,
        hardware_identity="linux/amd64/rtx-4090",
        environment_stratum="single-machine",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=registry.content_hash,
        content_hash="0" * 64,
    )
    content_hash = compute_campaign_profile_hash(temp)
    return CampaignProfile(
        campaign_id="campaign-1",
        campaign_revision="1",
        schema_version="1.0.0",
        purpose="pipeline integrity",
        created_at="2026-09-01T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=["qwen3-8b-q4_0"],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_HASH],
        grader_hashes=[_HASH],
        prompt_serialization_hash=_HASH,
        task_ids=["task-1"],
        repetitions=1,
        track_arm_assignments=track_arms,
        model_tier_assignments=[],
        temperature=0.0,
        top_p=1.0,
        max_tokens=4096,
        seed=42,
        context_limit=32768,
        timeout_seconds=120.0,
        max_retries=2,
        hardware_identity="linux/amd64/rtx-4090",
        environment_stratum="single-machine",
        primary_metrics=["ifeval_subset_verifier"],
        unit_of_analysis="task",
        claim_boundary=ClaimBoundary.DESCRIPTIVE_ONLY,
        model_registry_hash=registry.content_hash,
        content_hash=content_hash,
    )


def _make_provenance_manifest():
    from g8e_evals.provenance import SourceInclusionEntry, SourceInclusionManifest, compute_manifest_hash
    entries = [
        SourceInclusionEntry(path="src/main.py", sha256=_HASH, byte_length=100),
        SourceInclusionEntry(path="src/util.py", sha256="b" * 64, byte_length=50),
    ]
    entry_dicts = [
        {"path": e.path, "sha256": e.sha256, "byte_length": e.byte_length}
        for e in entries
    ]
    return SourceInclusionManifest(
        schema_version="1.0.0",
        entries=entries,
        manifest_hash=compute_manifest_hash(entry_dicts),
    )


def _write_report_dir(report_dir: Path) -> None:
    """Write a minimal campaign report directory with metrics and attempts."""
    from g8e_evals.constants import (
        ATTEMPTS_JSONL,
        CAMPAIGN_ASSIGNMENTS_JSONL,
        CAMPAIGN_INDEX_JSONL,
        CORRELATED_ERRORS_JSONL,
        ESCALATION_RECORDS_JSONL,
        METRICS_JSONL,
        RESOURCE_OBSERVATIONS_JSONL,
        SECURITY_EVENTS_JSONL,
        TOOL_CALL_SCORECARDS_JSONL,
    )
    report_dir.mkdir(parents=True, exist_ok=True)

    # Write attempts.jsonl
    attempt = {
        "attempt_id": "attempt-1",
        "schema_version": "1.44.0",
        "run_id": "run-1",
        "assignment_id": "assignment-1",
        "task_id": "task-1",
        "model_cohort_id": "cohort-qwen3-8b-q4_0",
        "replicate_id": "replicate-1",
        "terminal_status": "completed",
        "answer": "test answer",
        "verification_status": "verified",
    }
    (report_dir / ATTEMPTS_JSONL).write_text(json.dumps(attempt) + "\n")

    # Write metrics.jsonl with a tool_call metric so the v5 projector builds a tool scorecard summary
    metric = {
        "attempt_id": "attempt-1",
        "task_id": "task-1",
        "metric_id": "tool_call_recognition",
        "metric_version": "1.0.0",
        "value": 1.0,
        "unit": "boolean",
        "denominator_contribution": 1,
        "verification_status": "verified",
    }
    (report_dir / METRICS_JSONL).write_text(json.dumps(metric) + "\n")

    # Write campaign-assignments.jsonl
    assignment = {
        "assignment_id": "assignment-1",
        "model_cohort_id": "cohort-qwen3-8b-q4_0",
        "task_id": "task-1",
    }
    (report_dir / CAMPAIGN_ASSIGNMENTS_JSONL).write_text(json.dumps(assignment) + "\n")

    # Write campaign-index.jsonl with the finalization generation
    index_gen = {
        "generation_number": 1,
        "content_hash": _FINALIZATION_HASH,
        "creation_reason": "finalization",
        "assignment_dispositions": [
            {"assignment_id": "assignment-1", "disposition": "effective"},
        ],
    }
    (report_dir / CAMPAIGN_INDEX_JSONL).write_text(json.dumps(index_gen) + "\n")

    # Write the 5 event/resource files (empty) so the v5 projector copies them
    # to the candidate and the v5 validator's required-event-resource layer passes.
    for event_resource_file in (
        RESOURCE_OBSERVATIONS_JSONL,
        TOOL_CALL_SCORECARDS_JSONL,
        ESCALATION_RECORDS_JSONL,
        SECURITY_EVENTS_JSONL,
        CORRELATED_ERRORS_JSONL,
    ):
        (report_dir / event_resource_file).write_text("")


class TestProjectCampaignV5:
    pytestmark = pytest.mark.integration

    def test_creates_v5_candidate_directory(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)

        candidate = project_campaign_v5(
            report_dir=report_dir,
            verification=_make_verification_report(),
            candidate_dir=candidate_dir,
            campaign_profile=_make_campaign_profile(),
            model_registry=_make_registry(),
            provenance_manifest=_make_provenance_manifest(),
            caveats=["descriptive_only"],
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
        )
        assert candidate == candidate_dir
        assert (candidate_dir / "model-campaign.json").exists()
        assert (candidate_dir / "campaign-projections.jsonl").exists()
        # The radar profile and cold-start tradeoff should be present because
        # there are variant summaries and model variants.
        assert (candidate_dir / RADAR_PROFILE_JSON).exists()
        assert (candidate_dir / COLD_START_WARM_INFERENCE_TRADEOFF_JSON).exists()
        # The tool scorecard summary should be present because there is a
        # tool_call metric in the report.
        assert (candidate_dir / TOOL_SCORECARD_SUMMARY_JSON).exists()

    def test_refuses_overwrite(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)
        candidate_dir.mkdir(parents=True)

        with pytest.raises(ValueError, match="already exists"):
            project_campaign_v5(
                report_dir=report_dir,
                verification=_make_verification_report(),
                candidate_dir=candidate_dir,
                campaign_profile=_make_campaign_profile(),
                model_registry=_make_registry(),
                provenance_manifest=_make_provenance_manifest(),
                caveats=["descriptive_only"],
                evidence_cutoff="2026-09-10",
                platform_version="v2.1.8",
            )

    def test_rejects_failing_verification(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)

        with pytest.raises(ValueError, match="campaign verification failed"):
            project_campaign_v5(
                report_dir=report_dir,
                verification=_make_verification_report(ok=False),
                candidate_dir=candidate_dir,
                campaign_profile=_make_campaign_profile(),
                model_registry=_make_registry(),
                provenance_manifest=_make_provenance_manifest(),
                caveats=["descriptive_only"],
                evidence_cutoff="2026-09-10",
                platform_version="v2.1.8",
            )

    def test_model_campaign_uses_v5_version(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)

        project_campaign_v5(
            report_dir=report_dir,
            verification=_make_verification_report(),
            candidate_dir=candidate_dir,
            campaign_profile=_make_campaign_profile(),
            model_registry=_make_registry(),
            provenance_manifest=_make_provenance_manifest(),
            caveats=["descriptive_only"],
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
        )
        mc = json.loads((candidate_dir / "model-campaign.json").read_text())
        assert mc["publication_schema_version"] == PUBLICATION_SCHEMA_V5


class TestValidatePublicationV5:
    pytestmark = pytest.mark.integration

    def test_validates_clean_candidate(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)

        project_campaign_v5(
            report_dir=report_dir,
            verification=_make_verification_report(),
            candidate_dir=candidate_dir,
            campaign_profile=_make_campaign_profile(),
            model_registry=_make_registry(),
            provenance_manifest=_make_provenance_manifest(),
            caveats=["descriptive_only"],
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
        )
        result = validate_publication_v5(candidate_dir)
        assert result.ok
        assert "radar_profile" in result.checked_layers
        assert "tool_scorecard_summary" in result.checked_layers
        assert "cold_start_warm_inference_tradeoff" in result.checked_layers

    def test_passes_without_v5_artifacts(self, tmp_path: Path) -> None:
        # Project a v4 candidate (no v5 artifacts) and validate with v5.
        from g8e_evals.publication import project_campaign_v4, validate_publication_v4
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)

        project_campaign_v4(
            report_dir=report_dir,
            verification=_make_verification_report(),
            candidate_dir=candidate_dir,
            campaign_profile=_make_campaign_profile(),
            model_registry=_make_registry(),
            provenance_manifest=_make_provenance_manifest(),
            caveats=["descriptive_only"],
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
        )
        # v4 validation should pass
        v4_result = validate_publication_v4(candidate_dir)
        assert v4_result.ok
        # v5 validation should also pass (v5 artifacts are optional)
        v5_result = validate_publication_v5(candidate_dir)
        assert v5_result.ok

    def test_rejects_malformed_radar_profile(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)

        project_campaign_v5(
            report_dir=report_dir,
            verification=_make_verification_report(),
            candidate_dir=candidate_dir,
            campaign_profile=_make_campaign_profile(),
            model_registry=_make_registry(),
            provenance_manifest=_make_provenance_manifest(),
            caveats=["descriptive_only"],
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
        )
        # Corrupt the radar profile
        (candidate_dir / RADAR_PROFILE_JSON).write_text("{not valid json")
        result = validate_publication_v5(candidate_dir)
        assert not result.ok
        assert any("radar-profile.json" in f for f in result.failures)

    def test_rejects_malformed_tool_scorecard(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)

        project_campaign_v5(
            report_dir=report_dir,
            verification=_make_verification_report(),
            candidate_dir=candidate_dir,
            campaign_profile=_make_campaign_profile(),
            model_registry=_make_registry(),
            provenance_manifest=_make_provenance_manifest(),
            caveats=["descriptive_only"],
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
        )
        (candidate_dir / TOOL_SCORECARD_SUMMARY_JSON).write_text("{not valid json")
        result = validate_publication_v5(candidate_dir)
        assert not result.ok
        assert any("tool-scorecard-summary.json" in f for f in result.failures)

    def test_rejects_symlinked_radar_profile(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        candidate_dir = tmp_path / "candidate"
        _write_report_dir(report_dir)

        project_campaign_v5(
            report_dir=report_dir,
            verification=_make_verification_report(),
            candidate_dir=candidate_dir,
            campaign_profile=_make_campaign_profile(),
            model_registry=_make_registry(),
            provenance_manifest=_make_provenance_manifest(),
            caveats=["descriptive_only"],
            evidence_cutoff="2026-09-10",
            platform_version="v2.1.8",
        )
        # Replace radar profile with a symlink
        radar_path = candidate_dir / RADAR_PROFILE_JSON
        radar_path.unlink()
        target = tmp_path / "radar_target.json"
        target.write_text("{}")
        radar_path.symlink_to(target)
        result = validate_publication_v5(candidate_dir)
        assert not result.ok
        assert any("symlink" in f for f in result.failures)
