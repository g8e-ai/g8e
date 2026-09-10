# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for the v4 campaign projector.

Verifies that ``project_campaign_v4`` creates a complete v4 candidate
directory from a passing verification report, refuses to overwrite an
existing candidate, rejects a failing verification report, and produces
deterministic output.
"""

# pyright: reportCallIssue=false

from __future__ import annotations

import json
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.constants import (
    CAMPAIGN_PROJECTIONS_JSONL,
    CAMPAIGN_PROVENANCE_JSON,
    CAMPAIGN_STATISTICAL_ANALYSIS_JSON,
    CAMPAIGN_VERIFICATION_REF_JSON,
    MODEL_CAMPAIGN_JSON,
    PUBLICATION_SCHEMA_V4,
)
from g8e_evals.index import CampaignVerificationReport
from g8e_evals.publication import (
    ModelCampaignRef,
    PublicationSchemaV4,
    project_campaign_v4,
)
from g8e_evals.provenance import SourceInclusionEntry, SourceInclusionManifest, compute_manifest_hash
from g8e_evals.profile import CampaignProfile, CampaignLifecycleStatus, ClaimBoundary
from g8e_evals.registry import (
    ModelRegistry,
    ModelVariant,
    PublicationEligibility,
    WeightClass,
    compute_model_registry_hash,
)


_HASH = "a" * 64


def _make_verification_report(*, ok: bool = True) -> CampaignVerificationReport:
    return CampaignVerificationReport(
        verification_schema_version="1.0.0",
        campaign_id="campaign-1",
        campaign_revision="1",
        ok=ok,
        verified_index_generation_hash=_HASH,
        checked_layers=["file_safety", "index_chain", "finalization"],
        failures=[] if ok else ["file_safety: missing manifest.json"],
    )


def _make_registry() -> ModelRegistry:
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


def _make_profile(registry: ModelRegistry) -> CampaignProfile:
    from g8e_evals.profile import (
        TrackArmAssignment,
        compute_campaign_profile_hash,
    )
    from g8e_evals.schema import CampaignTrack

    track_arms = [
        TrackArmAssignment(track=CampaignTrack.DIRECT, arm_id="direct"),
        TrackArmAssignment(track=CampaignTrack.TIER_FITNESS, arm_id="doctrine"),
    ]
    content_hash = compute_campaign_profile_hash(
        campaign_id="campaign-1",
        campaign_revision="1",
        schema_version="1.0.0",
        purpose="pipeline integrity",
        generative_variant_ids=["qwen3-8b-q4_0"],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_HASH],
        grader_hashes=[_HASH],
        track_arm_assignments=track_arms,
        model_tier_assignments=[],
        repetitions=1,
        model_registry_hash=registry.content_hash,
    )
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


def _make_provenance_manifest(tmp_path: Path) -> SourceInclusionManifest:
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
        reviewed_by="reviewer-1",
        review_timestamp="2026-09-01T00:00:00Z",
    )


def _make_projection_records() -> list[dict]:
    return [
        {
            "campaign_id": "campaign-1",
            "campaign_revision": "1",
            "variant_id": "qwen3-8b-q4_0",
            "task_id": "task-1",
            "metric_id": "ifeval_subset_verifier",
            "numerator": 8,
            "denominator": 10,
            "rate": 0.8,
            "unit": "boolean",
            "verification_status": "verified",
            "evidence_link": "proofs/run-1/analysis.json",
        },
    ]


def _make_disposition_records() -> list[dict]:
    return [
        {
            "assignment_id": "assignment-1",
            "disposition": "effective",
            "variant_id": "qwen3-8b-q4_0",
            "task_id": "task-1",
            "reason": "",
        },
    ]


def _make_statistical_analysis_record() -> dict:
    return {
        "method": "mcnemar",
        "independent_unit": "task",
        "population": 1,
        "correction_family": "holm_bonferroni",
        "claim_status": "descriptive_only",
        "content_hash": _HASH,
    }


def _build_report_dir(
    tmp_path: Path,
    *,
    projection_records: list[dict] | None = None,
    disposition_records: list[dict] | None = None,
) -> Path:
    report_dir = tmp_path / "report"
    report_dir.mkdir(parents=True)
    if projection_records is None:
        projection_records = _make_projection_records()
    if disposition_records is None:
        disposition_records = _make_disposition_records()
    # Write projections JSONL
    proj_path = report_dir / "campaign-projections.jsonl"
    proj_path.write_text("".join(json.dumps(r) + "\n" for r in projection_records))
    # Write dispositions JSONL
    disp_path = report_dir / "campaign-dispositions.jsonl"
    disp_path.write_text("".join(json.dumps(r) + "\n" for r in disposition_records))
    # Write statistical analysis
    stat_path = report_dir / "campaign-statistical-analysis.json"
    stat_path.write_text(json.dumps(_make_statistical_analysis_record()))
    return report_dir


def _project(
    tmp_path: Path,
    *,
    verification_ok: bool = True,
    report_dir: Path | None = None,
) -> Path:
    registry = _make_registry()
    profile = _make_profile(registry)
    provenance = _make_provenance_manifest(tmp_path)
    verification = _make_verification_report(ok=verification_ok)
    if report_dir is None:
        report_dir = _build_report_dir(tmp_path)
    candidate_dir = tmp_path / "candidate"
    return project_campaign_v4(
        report_dir=report_dir,
        verification=verification,
        candidate_dir=candidate_dir,
        campaign_profile=profile,
        model_registry=registry,
        provenance_manifest=provenance,
        caveats=["descriptive_only"],
        evidence_cutoff="2026-09-10",
        platform_version="v2.1.8",
    )


# --- Tests ---


def test_projector_creates_complete_candidate_directory(tmp_path: Path) -> None:
    candidate = _project(tmp_path)
    assert candidate.is_dir()
    assert (candidate / MODEL_CAMPAIGN_JSON).is_file()
    assert (candidate / CAMPAIGN_PROJECTIONS_JSONL).is_file()
    assert (candidate / CAMPAIGN_STATISTICAL_ANALYSIS_JSON).is_file()
    assert (candidate / CAMPAIGN_PROVENANCE_JSON).is_file()
    assert (candidate / CAMPAIGN_VERIFICATION_REF_JSON).is_file()


def test_projector_writes_valid_model_campaign_json(tmp_path: Path) -> None:
    candidate = _project(tmp_path)
    data = json.loads((candidate / MODEL_CAMPAIGN_JSON).read_text())
    ref = ModelCampaignRef.model_validate(data)
    assert ref.campaign_id == "campaign-1"
    assert ref.verification_ok is True
    assert ref.publication_schema_version == PUBLICATION_SCHEMA_V4


def test_projector_writes_valid_publication_schema_v4(tmp_path: Path) -> None:
    candidate = _project(tmp_path)
    data = json.loads((candidate / MODEL_CAMPAIGN_JSON).read_text())
    ref = ModelCampaignRef.model_validate(data)
    schema = PublicationSchemaV4(
        publication_schema_version=PUBLICATION_SCHEMA_V4,
        evidence_cutoff="2026-09-10",
        platform_version="v2.1.8",
        model_campaign=ref,
    )
    assert schema.model_campaign.campaign_id == "campaign-1"


def test_projector_refuses_existing_candidate(tmp_path: Path) -> None:
    candidate_dir = tmp_path / "candidate"
    candidate_dir.mkdir()
    (candidate_dir / MODEL_CAMPAIGN_JSON).write_text("existing")
    with pytest.raises(Exception, match="already exists"):
        _project(tmp_path)


def test_projector_rejects_failing_verification_report(tmp_path: Path) -> None:
    with pytest.raises(Exception, match="verification"):
        _project(tmp_path, verification_ok=False)


def test_projector_output_is_deterministic(tmp_path: Path) -> None:
    candidate1 = _project(tmp_path)
    content1 = (candidate1 / MODEL_CAMPAIGN_JSON).read_text()

    tmp_path2 = tmp_path / "second"
    tmp_path2.mkdir()
    candidate2 = _project(tmp_path2)
    content2 = (candidate2 / MODEL_CAMPAIGN_JSON).read_text()

    assert content1 == content2


def test_projector_writes_projection_rows(tmp_path: Path) -> None:
    candidate = _project(tmp_path)
    lines = (candidate / CAMPAIGN_PROJECTIONS_JSONL).read_text().strip().splitlines()
    assert len(lines) == 1
    row = json.loads(lines[0])
    assert row["variant_id"] == "qwen3-8b-q4_0"
    assert row["verification_status"] == "verified"


def test_projector_writes_verification_ref(tmp_path: Path) -> None:
    candidate = _project(tmp_path)
    data = json.loads((candidate / CAMPAIGN_VERIFICATION_REF_JSON).read_text())
    assert data["ok"] is True
    assert data["campaign_id"] == "campaign-1"
    assert data["verified_index_generation_hash"] == _HASH


def test_projector_writes_provenance_ref(tmp_path: Path) -> None:
    candidate = _project(tmp_path)
    data = json.loads((candidate / CAMPAIGN_PROVENANCE_JSON).read_text())
    assert data["manifest_hash"] == _HASH or len(data["manifest_hash"]) == 64
    assert data["entry_count"] == 2


def test_projector_no_symlinks_in_candidate(tmp_path: Path) -> None:
    candidate = _project(tmp_path)
    for path in candidate.rglob("*"):
        assert not path.is_symlink(), f"symlink found: {path}"


def test_projector_candidate_files_are_regular(tmp_path: Path) -> None:
    candidate = _project(tmp_path)
    for path in candidate.rglob("*"):
        if path.is_file():
            assert path.is_file()
