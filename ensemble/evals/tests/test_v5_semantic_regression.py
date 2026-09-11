# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Failing fixture tests for v5 semantic regression (EF13-TESTS).

These tests document the known semantic defects in the current v5
publication projector and validator. Each test asserts the CORRECT
behavior that Jade's Phase 2 remediation (EF13-REMEDIATION-PROJECTOR)
should implement. The tests are marked ``xfail`` so they do not break
the suite; Jade removes the markers as she satisfies each test without
weakening the assertions.

Each test targets a specific defect identified in the senior review:

1. Single-report limitation: the v5 projector accepts one report
   directory, not a typed aggregate.
2. Per-variant metric overwrite: grouping keys are incomplete so
   multiple roles/stages/repetitions overwrite each other.
3. Unweighted aggregation: macro vs micro semantics are not stated.
4. Missing-to-zero conversion: unavailable radar inputs become 0.0.
5. Empty timing summaries: cold-start tradeoff emits all-None rows.
6. Absent required event/resource files: the v5 validator does not
   enforce required event/resource files.
7. Unsupported v5 README schema: no v5 README loading path exists.
8. Undeclared files: the v5 validator does not reject undeclared files.
9. Denominator loss: per-variant aggregation loses per-task
   denominators.
10. Tampered authority links: content hash drift is not independently
    recomputed for every v5 artifact.

Additional fixtures cover:
- Measured zero, unavailable, not applicable, withheld, and
  missing-corrupt values remain distinct.
- Public-safe failure links without embedding restricted content.
"""

# pyright: reportCallIssue=false

from __future__ import annotations

import json
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.constants import (
    RADAR_PROFILE_JSON,
)
from g8e_evals.index import CampaignVerificationReport
from g8e_evals.publication import (
    validate_publication_v5,
)
from g8e_evals.radar_profile import (
    ColdStartWarmInferenceTradeoff,
    RadarDimension,
    RadarDimensionName,
    compute_radar_profile_hash,
)


_HASH = "a" * 64


# ---------------------------------------------------------------------------
# Helpers (reused from the existing v5 test infrastructure)
# ---------------------------------------------------------------------------


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
    from g8e_evals.constants import ATTEMPTS_JSONL, CAMPAIGN_ASSIGNMENTS_JSONL, CAMPAIGN_INDEX_JSONL, METRICS_JSONL
    report_dir.mkdir(parents=True, exist_ok=True)
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
    assignment = {
        "assignment_id": "assignment-1",
        "model_cohort_id": "cohort-qwen3-8b-q4_0",
        "task_id": "task-1",
    }
    (report_dir / CAMPAIGN_ASSIGNMENTS_JSONL).write_text(json.dumps(assignment) + "\n")
    index_gen = {
        "generation_number": 1,
        "content_hash": _FINALIZATION_HASH,
        "creation_reason": "finalization",
        "assignment_dispositions": [
            {"assignment_id": "assignment-1", "disposition": "effective"},
        ],
    }
    (report_dir / CAMPAIGN_INDEX_JSONL).write_text(json.dumps(index_gen) + "\n")


def _project_v5(tmp_path: Path) -> Path:
    """Project a v5 candidate and return the candidate directory."""
    from g8e_evals.publication import project_campaign_v5
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
    return candidate_dir


# ===========================================================================
# Defect 1: Single-report limitation
# ===========================================================================


class TestSingleReportLimitation:
    """The v5 projector accepts one report directory, not a typed aggregate.

    The current ``project_campaign_v5`` signature takes a single
    ``report_dir: Path`` and a single ``CampaignVerificationReport``. It
    cannot consume a typed campaign-set plan, index, or aggregate
    verification result. Jade should replace this with an
    aggregate-capable entry point.
    """

    @pytest.mark.xfail(
        reason="project_campaign_v5 accepts a single report_dir, not a typed aggregate",
        strict=True,
    )
    def test_project_campaign_v5_accepts_aggregate_inputs(self) -> None:
        from g8e_evals.publication import project_campaign_v5
        import inspect

        sig = inspect.signature(project_campaign_v5)
        params = sig.parameters
        # The correct signature should accept typed aggregate inputs.
        assert "campaign_set_plan" in params
        assert "campaign_set_index" in params
        assert "aggregate_verification_result" in params

    @pytest.mark.xfail(
        reason="project_campaign_v5 has no aggregate entry point for multiple child reports",
        strict=True,
    )
    def test_project_campaign_v5_has_aggregate_entry_point(self) -> None:
        from g8e_evals.publication import project_campaign_v5
        import inspect

        sig = inspect.signature(project_campaign_v5)
        params = sig.parameters
        # The correct API should accept a typed plan/index pair binding
        # multiple child reports. The current API accepts only a single
        # report_dir Path. When Jade adds aggregate support, either
        # the signature should accept aggregate inputs or a new
        # aggregate-capable function should exist.
        has_aggregate_param = any(
            name in params
            for name in ("campaign_set_plan", "campaign_set_index", "aggregate_verification_result")
        )
        assert has_aggregate_param, (
            "project_campaign_v5 should accept typed aggregate inputs "
            "(campaign_set_plan, campaign_set_index, or aggregate_verification_result)"
        )


# ===========================================================================
# Defect 2: Per-variant metric overwrite
# ===========================================================================


class TestPerVariantMetricOverwrite:
    """The v5 projector groups variant summaries by ``(variant_id, metric_id)``
    only, so multiple roles, stages, or repetitions overwrite each other.
    """

    @pytest.mark.xfail(
        reason="variant summary grouping key omits role; multiple roles overwrite each other",
        strict=True,
    )
    def test_variant_summary_grouping_key_includes_role(self) -> None:
        from g8e_evals.publication import _generate_variant_summaries, CampaignProjectionRow

        rows = [
            CampaignProjectionRow(
                campaign_id="c1",
                campaign_revision="1",
                variant_id="v1",
                task_id="task-1",
                metric_id="ifeval_subset_verifier",
                numerator=1,
                denominator=2,
                rate=0.5,
                unit="boolean",
                verification_status="verified",
                evidence_link="proofs/v1/task-1/rep-1.json",
                repetition=1,
            ),
            CampaignProjectionRow(
                campaign_id="c1",
                campaign_revision="1",
                variant_id="v1",
                task_id="task-1",
                metric_id="ifeval_subset_verifier",
                numerator=0,
                denominator=2,
                rate=0.0,
                unit="boolean",
                verification_status="verified",
                evidence_link="proofs/v1/task-1/rep-2.json",
                repetition=2,
            ),
        ]
        summaries = _generate_variant_summaries(rows)
        # The correct behavior: summaries should carry a role field so
        # multiple roles are not collapsed. The summary model should
        # have a role field.
        assert hasattr(summaries[0], "role")


# ===========================================================================
# Defect 3: Unweighted aggregation
# ===========================================================================


class TestUnweightedAggregation:
    """The radar profile builder uses a simple mean (unweighted) across
    variant summaries without stating macro vs micro semantics.
    """

    @pytest.mark.xfail(
        reason="radar profile uses unweighted mean without stating macro vs micro semantics",
        strict=True,
    )
    def test_radar_dimension_states_weighting_method(self) -> None:
        from g8e_evals.publication import _build_radar_profile, CampaignVariantSummaryRow

        summaries = [
            CampaignVariantSummaryRow(
                variant_id="v1",
                metric_id="ifeval_subset_verifier",
                numerator=1,
                denominator=100,
                rate=0.01,
                unit="boolean",
                task_count=100,
                repetition_count=1,
                terminal_count=100,
                superseded_count=0,
                invalid_count=0,
                missing_count=0,
            ),
        ]
        profile = _build_radar_profile(
            campaign_id="c1",
            campaign_revision="1",
            variant_summaries=summaries,
        )
        assert profile is not None
        task_accuracy_dim = next(
            d for d in profile.dimensions if d.name == RadarDimensionName.TASK_ACCURACY
        )
        # The correct behavior: the dimension should carry a weighting
        # method field stating whether it is macro or micro.
        assert hasattr(task_accuracy_dim, "weighting_method")


# ===========================================================================
# Defect 4: Missing-to-zero conversion
# ===========================================================================


class TestMissingToZeroConversion:
    """The radar profile builder converts unavailable radar inputs to 0.0
    instead of preserving them as unavailable.
    """

    @pytest.mark.xfail(
        reason="radar profile converts unavailable inputs to 0.0 instead of preserving unavailable state",
        strict=True,
    )
    def test_radar_dimension_preserves_unavailable_state(self) -> None:
        from g8e_evals.publication import _build_radar_profile, CampaignVariantSummaryRow

        summaries = [
            CampaignVariantSummaryRow(
                variant_id="v1",
                metric_id="some_unknown_metric",
                numerator=5,
                denominator=10,
                rate=0.5,
                unit="boolean",
                task_count=10,
                repetition_count=1,
                terminal_count=10,
                superseded_count=0,
                invalid_count=0,
                missing_count=0,
            ),
        ]
        profile = _build_radar_profile(
            campaign_id="c1",
            campaign_revision="1",
            variant_summaries=summaries,
        )
        assert profile is not None
        for dim in profile.dimensions:
            # The correct behavior: a dimension with no source metrics
            # should carry a typed unavailable state, not 0.0.
            assert hasattr(dim, "availability")
            assert dim.availability != "measured"


# ===========================================================================
# Defect 5: Empty timing summaries
# ===========================================================================


class TestEmptyTimingSummaries:
    """The cold-start tradeoff builder emits all-None timing values for
    every variant, producing empty timing summaries.
    """

    @pytest.mark.xfail(
        reason="cold-start tradeoff emits all-None timing values without availability reason",
        strict=True,
    )
    def test_cold_start_tradeoff_carries_availability_reason(self) -> None:
        from g8e_evals.publication import (
            _build_cold_start_tradeoff_summary,
            SafeModelVariantProjection,
        )

        variant = SafeModelVariantProjection(
            variant_id="v1",
            canonical_display_name="Variant 1",
            weight_class="heavy-slm",
            parameter_count_display="8.0B",
            backend_name="ollama",
            served_model_tag="v1:latest",
            artifact_digest=_HASH,
            quantization="q4_0",
            publication_eligibility="eligible",
        )
        summary = _build_cold_start_tradeoff_summary(
            campaign_id="c1",
            campaign_revision="1",
            model_variants=[variant],
        )
        assert summary is not None
        assert len(summary.tradeoffs) == 1
        tradeoff = summary.tradeoffs[0]
        # The correct behavior: None timing values should carry a typed
        # unavailability reason explaining why the measurement is absent.
        assert hasattr(tradeoff, "unavailability_reason")


# ===========================================================================
# Defect 6: Absent required event/resource files
# ===========================================================================


class TestAbsentRequiredEventResourceFiles:
    """The v5 validator does not enforce the presence of required
    event/resource files. It only validates files that exist.
    """

    @pytest.mark.xfail(
        reason="v5 validator does not enforce required event/resource files",
        strict=True,
    )
    def test_validator_fails_without_required_event_resource_files(self, tmp_path: Path) -> None:
        candidate_dir = _project_v5(tmp_path)
        result = validate_publication_v5(candidate_dir)
        # The correct behavior: the validator should FAIL when required
        # event/resource files are absent.
        assert result.ok is False
        assert any("resource" in f.lower() or "event" in f.lower() for f in result.failures)


# ===========================================================================
# Defect 7: Unsupported v5 README schema
# ===========================================================================


class TestUnsupportedV5ReadmeSchema:
    """No v5 README loading path exists. The README generator only
    supports v1-v4 publication schemas.
    """

    @pytest.mark.xfail(
        reason="README generator does not support v5 schema loading",
        strict=True,
    )
    def test_readme_generator_supports_v5(self) -> None:
        import importlib

        try:
            mod = importlib.import_module("scripts.generate_readme")
        except ImportError:
            pytest.fail("scripts.generate_readme not importable")

        v5_attrs = [attr for attr in dir(mod) if "v5" in attr.lower()]
        assert len(v5_attrs) > 0, "No v5-specific functions found in generate_readme"


# ===========================================================================
# Defect 8: Undeclared files
# ===========================================================================


class TestUndeclaredFiles:
    """The v5 validator does not reject undeclared files in the candidate
    directory.
    """

    @pytest.mark.xfail(
        reason="v5 validator does not reject undeclared files in candidate directory",
        strict=True,
    )
    def test_validator_fails_with_undeclared_file(self, tmp_path: Path) -> None:
        candidate_dir = _project_v5(tmp_path)
        (candidate_dir / "secret-data.json").write_text(json.dumps({"secret": "leaked"}))
        result = validate_publication_v5(candidate_dir)
        # The correct behavior: the validator should FAIL when
        # undeclared files are present.
        assert result.ok is False
        assert any("undeclared" in f.lower() for f in result.failures)


# ===========================================================================
# Defect 9: Denominator loss
# ===========================================================================


class TestDenominatorLoss:
    """The per-variant summary aggregation loses per-task denominators.
    """

    @pytest.mark.xfail(
        reason="variant summary loses per-task denominators in aggregation",
        strict=True,
    )
    def test_variant_summary_preserves_per_task_denominators(self) -> None:
        from g8e_evals.publication import _generate_variant_summaries, CampaignProjectionRow

        rows = [
            CampaignProjectionRow(
                campaign_id="c1",
                campaign_revision="1",
                variant_id="v1",
                task_id="task-1",
                metric_id="ifeval_subset_verifier",
                numerator=8,
                denominator=10,
                rate=0.8,
                unit="boolean",
                verification_status="verified",
                evidence_link="proofs/v1/task-1/rep-1.json",
                repetition=1,
            ),
            CampaignProjectionRow(
                campaign_id="c1",
                campaign_revision="1",
                variant_id="v1",
                task_id="task-2",
                metric_id="ifeval_subset_verifier",
                numerator=2,
                denominator=100,
                rate=0.02,
                unit="boolean",
                verification_status="verified",
                evidence_link="proofs/v1/task-2/rep-1.json",
                repetition=1,
            ),
        ]
        summaries = _generate_variant_summaries(rows)
        assert len(summaries) == 1
        summary = summaries[0]
        # The correct behavior: the summary should preserve per-task
        # denominator breakdowns so a reader can inspect the distribution.
        assert hasattr(summary, "per_task_denominators")


# ===========================================================================
# Defect 10: Tampered authority links
# ===========================================================================


class TestTamperedAuthorityLinks:
    """The v5 validator validates content hashes on individual artifacts
    but does not independently recompute every summary from canonical rows.
    """

    @pytest.mark.xfail(
        reason="v5 validator does not independently recompute summaries from projection rows",
        strict=True,
    )
    def test_validator_recomputes_radar_from_projections(self, tmp_path: Path) -> None:
        candidate_dir = _project_v5(tmp_path)

        radar_path = candidate_dir / RADAR_PROFILE_JSON
        radar_data = json.loads(radar_path.read_text())

        radar_data["dimensions"][0]["value"] = 0.01

        from g8e_evals.radar_profile import RadarDimension
        tampered_dims = [RadarDimension.model_validate(d) for d in radar_data["dimensions"]]
        new_hash = compute_radar_profile_hash(
            campaign_id=radar_data["campaign_id"],
            campaign_revision=radar_data["campaign_revision"],
            dimensions=tampered_dims,
        )
        radar_data["content_hash"] = new_hash
        radar_path.write_text(json.dumps(radar_data, sort_keys=True))

        result = validate_publication_v5(candidate_dir)
        # The correct behavior: the validator should FAIL because the
        # tampered radar profile does not recompute from the projection rows.
        assert result.ok is False
        assert any("recompute" in f.lower() or "radar" in f.lower() for f in result.failures)


# ===========================================================================
# Missingness state fixtures
# ===========================================================================


class TestMissingnessStatesRemainDistinct:
    """Fixtures with measured zero, unavailable, not applicable, withheld,
    and missing-corrupt values must remain distinct.

    Per invariant 7: missing and zero are distinct. Per Beacon's
    ``MeasurementAvailability`` enum: measured, unavailable,
    not_applicable, withheld. The v5 publication pipeline must
    preserve these distinct states and never convert one to another.
    """

    def test_measured_zero_is_not_unavailable(self) -> None:
        from g8e_evals.index import MeasurementAvailability
        assert MeasurementAvailability.MEASURED != MeasurementAvailability.UNAVAILABLE

    def test_unavailable_is_not_not_applicable(self) -> None:
        from g8e_evals.index import MeasurementAvailability
        assert MeasurementAvailability.UNAVAILABLE != MeasurementAvailability.NOT_APPLICABLE

    def test_withheld_is_not_unavailable(self) -> None:
        from g8e_evals.index import MeasurementAvailability
        assert MeasurementAvailability.WITHHELD != MeasurementAvailability.UNAVAILABLE

    def test_all_four_states_are_distinct(self) -> None:
        from g8e_evals.index import MeasurementAvailability
        states = [
            MeasurementAvailability.MEASURED,
            MeasurementAvailability.UNAVAILABLE,
            MeasurementAvailability.NOT_APPLICABLE,
            MeasurementAvailability.WITHHELD,
        ]
        assert len(set(states)) == 4

    @pytest.mark.xfail(
        reason="radar dimension does not carry availability state distinguishing measured zero from unavailable",
        strict=True,
    )
    def test_radar_dimension_carries_availability_state(self) -> None:
        dim = RadarDimension(
            name=RadarDimensionName.TASK_ACCURACY,
            value=0.0,
            source_metric_ids=["m1"],
        )
        # The correct behavior: a radar dimension with value 0.0 should
        # carry an availability state distinguishing "measured zero" from
        # "unavailable."
        assert hasattr(dim, "availability")

    @pytest.mark.xfail(
        reason="cold-start tradeoff does not carry unavailability reason for None timing values",
        strict=True,
    )
    def test_cold_start_tradeoff_carries_unavailability_reason(self) -> None:
        tradeoff = ColdStartWarmInferenceTradeoff(variant_id="v1")
        # The correct behavior: None timing values should carry a typed
        # unavailability reason.
        assert hasattr(tradeoff, "unavailability_reason")


# ===========================================================================
# Public-safe failure-link fixtures
# ===========================================================================


class TestPublicSafeFailureLinks:
    """Public-safe failure-link fixtures without embedding restricted
    prompts, model outputs, canaries, credentials, exact private
    endpoints, or private keys.
    """

    def test_evidence_link_is_relative_path(self) -> None:
        """Evidence links in projection rows are relative POSIX paths
        with no traversal, no absolute paths, and no embedded content."""
        from g8e_evals.publication import CampaignProjectionRow

        row = CampaignProjectionRow(
            campaign_id="c1",
            campaign_revision="1",
            variant_id="v1",
            task_id="task-1",
            metric_id="ifeval_subset_verifier",
            numerator=0,
            denominator=1,
            rate=0.0,
            unit="boolean",
            verification_status="verified",
            evidence_link="proofs/v1/task-1/rep-1.json",
            repetition=1,
        )
        assert not row.evidence_link.startswith("/")
        assert ".." not in row.evidence_link
        assert "prompt" not in row.evidence_link.lower()
        assert "output" not in row.evidence_link.lower()
        assert "key" not in row.evidence_link.lower()

    def test_evidence_link_rejects_traversal(self) -> None:
        """Evidence links with path traversal are rejected."""
        from pydantic import ValidationError
        from g8e_evals.publication import CampaignProjectionRow

        with pytest.raises(ValidationError):
            CampaignProjectionRow(
                campaign_id="c1",
                campaign_revision="1",
                variant_id="v1",
                task_id="task-1",
                metric_id="ifeval_subset_verifier",
                numerator=0,
                denominator=1,
                rate=0.0,
                unit="boolean",
                verification_status="verified",
                evidence_link="../../../etc/passwd",
                repetition=1,
            )

    def test_failure_link_does_not_contain_restricted_content(self) -> None:
        """A public-safe failure link references a proof artifact path
        without embedding the restricted prompt, output, or canary."""
        safe_link = "proofs/qwen3-8b-q4_0/task-1/rep-1.json"
        assert "prompt" not in safe_link
        assert "output" not in safe_link
        assert "canary" not in safe_link
        assert "credential" not in safe_link
        assert "key" not in safe_link
        assert "endpoint" not in safe_link
        assert "192.168" not in safe_link

    def test_candidate_directory_has_no_restricted_content(self, tmp_path: Path) -> None:
        """A projected v5 candidate directory must not contain restricted
        prompts, model outputs, canaries, credentials, or private keys."""
        candidate_dir = _project_v5(tmp_path)
        restricted_markers = [
            "raw_prompt",
            "model_output",
            "canary",
            "credential",
            "private_key",
            "api_key",
            "secret_key",
            "password",
            "192.168.1.2",
        ]
        for path in candidate_dir.rglob("*"):
            if path.is_file():
                content = path.read_text()
                for marker in restricted_markers:
                    assert marker not in content.lower(), (
                        f"restricted marker {marker!r} found in {path}"
                    )

    def test_candidate_directory_has_no_sqlite(self, tmp_path: Path) -> None:
        """A projected v5 candidate directory must not contain SQLite
        files per D12."""
        candidate_dir = _project_v5(tmp_path)
        for path in candidate_dir.rglob("*"):
            if path.is_file():
                assert not path.name.endswith(".sqlite")
                assert not path.name.endswith(".db")
                assert not path.name.endswith(".sqlite3")


# ===========================================================================
# Disclosure authority integration
# ===========================================================================


class TestDisclosureAuthorityIntegration:
    """The disclosure authority must be bindable to the v5 publication
    so that promotion consumes only disclosure-approved fields."""

    @pytest.mark.xfail(
        reason="PublicationSchemaV5 does not carry a disclosure authority reference",
        strict=True,
    )
    def test_publication_schema_v5_carries_disclosure_authority(self) -> None:
        from g8e_evals.publication import PublicationSchemaV5
        fields = PublicationSchemaV5.model_fields
        assert "disclosure_authority" in fields or "disclosure_authority_hash" in fields

    def test_disclosure_authority_is_content_addressed(self) -> None:
        """The disclosure authority is frozen and content-addressed."""
        from g8e_evals.disclosure_authority import (
            DisclosureAuthority,
            DisclosureFieldEntry,
            DisclosureOutputEntry,
            compute_disclosure_authority_hash,
        )
        fields = [
            {
                "field_name": "variant_id",
                "classification": "public",
                "public_column_name": "variant_id",
                "in_csv": True,
                "tombstone_hash_field": "",
            },
        ]
        outputs = [
            {
                "file_name": "disclosure-public.jsonl",
                "output_format": "canonical_jsonl",
                "required": True,
                "description": "test",
            },
        ]
        h = compute_disclosure_authority_hash(
            schema_version="1.0.0",
            authority_id="d1",
            authority_version="1",
            fields=fields,
            outputs=outputs,
            proof_index_description="test",
        )
        assert len(h) == 64
        authority = DisclosureAuthority(
            schema_version="1.0.0",
            authority_id="d1",
            authority_version="1",
            fields=[DisclosureFieldEntry.model_validate(f) for f in fields],
            outputs=[DisclosureOutputEntry.model_validate(o) for o in outputs],
            proof_index_description="test",
            content_hash=h,
        )
        assert authority.content_hash == h
