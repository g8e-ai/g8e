# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 + Tier 2 tests for the strict v4 publication validator.

Verifies that ``validate_publication_v4`` rejects unknown fields,
duplicate identities, undeclared artifacts, non-finite values, path
traversal, symlinks, unsafe links, checksum drift, and aggregate or
analysis claims that cannot be recomputed.
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
from g8e_evals.publication import (
    PublicationValidatorResult,
    validate_publication_v4,
)


_HASH = "a" * 64


def _write_valid_candidate(candidate: Path) -> dict:
    """Write a complete valid v4 candidate directory. Returns the model-campaign dict."""
    candidate.mkdir(parents=True, exist_ok=True)

    model_campaign = {
        "campaign_id": "campaign-1",
        "campaign_revision": "1",
        "publication_schema_version": PUBLICATION_SCHEMA_V4,
        "campaign_profile": {
            "campaign_id": "campaign-1",
            "campaign_revision": "1",
            "content_hash": _HASH,
            "benchmark_ids": ["ifeval_subset"],
            "task_ids": ["task-1"],
            "repetitions": 1,
            "hardware_identity": "linux/amd64/rtx-4090",
            "claim_boundary": "descriptive_only",
        },
        "model_variants": [
            {
                "variant_id": "qwen3-8b-q4_0",
                "canonical_display_name": "Qwen3-8B (q4_0)",
                "weight_class": "heavy-slm",
                "parameter_count_display": "8.0B",
                "backend_name": "ollama",
                "served_model_tag": "qwen3:8b",
                "artifact_digest": _HASH,
                "quantization": "q4_0",
                "publication_eligibility": "eligible",
            }
        ],
        "model_registry_hash": _HASH,
        "verification_ok": True,
        "verified_index_generation_hash": _HASH,
        "checked_layers": ["file_safety", "index_chain"],
        "projections": [
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
                "repetition": 1,
            }
        ],
        "dispositions": [
            {
                "assignment_id": "assignment-1",
                "disposition": "effective",
                "variant_id": "qwen3-8b-q4_0",
                "task_id": "task-1",
                "reason": "",
            }
        ],
        "comparison_rows": [
            {
                "task_id": "task-1",
                "combination_id": "combo-1",
                "primary_variant_id": "qwen3-8b-q4_0",
                "assistant_variant_id": "qwen3-4b-q4_0",
                "lite_variant_id": "qwen3-0.6b-q4_0",
                "metric_id": "ifeval_subset_verifier",
                "repetition": 1,
                "numerator": 8,
                "denominator": 10,
                "rate": 0.8,
                "unit": "boolean",
            }
        ],
        "efficiency_observations": [
            {
                "variant_id": "qwen3-8b-q4_0",
                "hardware_identity": "linux/amd64/rtx-4090",
                "end_to_end_latency_seconds": 12.5,
                "output_throughput_tokens_per_second": 42.0,
                "artifact_bytes": 4800000000,
                "peak_resident_memory_bytes": 2000000000,
            }
        ],
        "statistical_analysis": {
            "method": "mcnemar",
            "independent_unit": "task",
            "population": 1,
            "correction_family": "holm_bonferroni",
            "claim_status": "descriptive_only",
            "content_hash": _HASH,
        },
        "provenance": {
            "manifest_hash": _HASH,
            "entry_count": 2,
            "schema_version": "1.0.0",
        },
        "caveats": ["descriptive_only"],
    }
    # Build sub-models from the raw dict, compute the content hash, then set it.
    from g8e_evals.publication import (
        CampaignComparisonRow,
        CampaignDispositionRow,
        CampaignEfficiencyObservation,
        CampaignProjectionRow,
        CampaignProvenanceRef,
        CampaignStatisticalAnalysisRef,
        SafeCampaignProfileProjection,
        SafeModelVariantProjection,
        compute_model_campaign_hash,
    )

    profile_proj = SafeCampaignProfileProjection.model_validate(model_campaign["campaign_profile"])
    variant_projs = [SafeModelVariantProjection.model_validate(v) for v in model_campaign["model_variants"]]
    proj_rows = [CampaignProjectionRow.model_validate(p) for p in model_campaign["projections"]]
    disp_rows = [CampaignDispositionRow.model_validate(d) for d in model_campaign["dispositions"]]
    comp_rows = [CampaignComparisonRow.model_validate(c) for c in model_campaign["comparison_rows"]]
    eff_rows = [CampaignEfficiencyObservation.model_validate(e) for e in model_campaign["efficiency_observations"]]
    stat_ref = CampaignStatisticalAnalysisRef.model_validate(model_campaign["statistical_analysis"])
    prov_ref = CampaignProvenanceRef.model_validate(model_campaign["provenance"])
    model_campaign["content_hash"] = compute_model_campaign_hash(
        campaign_id=model_campaign["campaign_id"],
        campaign_revision=model_campaign["campaign_revision"],
        publication_schema_version=model_campaign["publication_schema_version"],
        campaign_profile=profile_proj,
        model_variants=variant_projs,
        model_registry_hash=model_campaign["model_registry_hash"],
        verification_ok=model_campaign["verification_ok"],
        verified_index_generation_hash=model_campaign["verified_index_generation_hash"],
        checked_layers=model_campaign["checked_layers"],
        projections=proj_rows,
        dispositions=disp_rows,
        comparison_rows=comp_rows,
        efficiency_observations=eff_rows,
        statistical_analysis=stat_ref,
        provenance=prov_ref,
        caveats=model_campaign["caveats"],
    )
    (candidate / MODEL_CAMPAIGN_JSON).write_text(json.dumps(model_campaign, sort_keys=True))

    projections = model_campaign["projections"]
    (candidate / CAMPAIGN_PROJECTIONS_JSONL).write_text(
        "".join(json.dumps(r, sort_keys=True) + "\n" for r in projections)
    )

    (candidate / CAMPAIGN_STATISTICAL_ANALYSIS_JSON).write_text(
        json.dumps(model_campaign["statistical_analysis"], sort_keys=True)
    )

    (candidate / CAMPAIGN_PROVENANCE_JSON).write_text(
        json.dumps(model_campaign["provenance"], sort_keys=True)
    )

    verification_ref = {
        "verification_schema_version": "1.0.0",
        "campaign_id": "campaign-1",
        "campaign_revision": "1",
        "ok": True,
        "verified_index_generation_hash": _HASH,
        "checked_layers": ["file_safety", "index_chain"],
        "failures": [],
    }
    (candidate / CAMPAIGN_VERIFICATION_REF_JSON).write_text(
        json.dumps(verification_ref, sort_keys=True)
    )

    return model_campaign


def _recompute_hash(data: dict) -> str:
    """Recompute the model-campaign content hash from a raw dict using sub-models."""
    from g8e_evals.publication import (
        CampaignComparisonRow,
        CampaignDispositionRow,
        CampaignEfficiencyObservation,
        CampaignProjectionRow,
        CampaignProvenanceRef,
        CampaignStatisticalAnalysisRef,
        SafeCampaignProfileProjection,
        SafeModelVariantProjection,
        compute_model_campaign_hash,
    )

    profile_proj = SafeCampaignProfileProjection.model_validate(data["campaign_profile"])
    variant_projs = [SafeModelVariantProjection.model_validate(v) for v in data["model_variants"]]
    proj_rows = [CampaignProjectionRow.model_validate(p) for p in data["projections"]]
    disp_rows = [CampaignDispositionRow.model_validate(d) for d in data["dispositions"]]
    comp_rows = [CampaignComparisonRow.model_validate(c) for c in data["comparison_rows"]]
    eff_rows = [CampaignEfficiencyObservation.model_validate(e) for e in data["efficiency_observations"]]
    stat_ref = CampaignStatisticalAnalysisRef.model_validate(data["statistical_analysis"])
    prov_ref = CampaignProvenanceRef.model_validate(data["provenance"])
    return compute_model_campaign_hash(
        campaign_id=data["campaign_id"],
        campaign_revision=data["campaign_revision"],
        publication_schema_version=data["publication_schema_version"],
        campaign_profile=profile_proj,
        model_variants=variant_projs,
        model_registry_hash=data["model_registry_hash"],
        verification_ok=data["verification_ok"],
        verified_index_generation_hash=data["verified_index_generation_hash"],
        checked_layers=data["checked_layers"],
        projections=proj_rows,
        dispositions=disp_rows,
        comparison_rows=comp_rows,
        efficiency_observations=eff_rows,
        statistical_analysis=stat_ref,
        provenance=prov_ref,
        caveats=data["caveats"],
    )


def _validate(candidate: Path) -> PublicationValidatorResult:
    return validate_publication_v4(candidate)


# --- Valid candidate passes ---


def test_valid_candidate_passs(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    _write_valid_candidate(candidate)
    result = _validate(candidate)
    assert result.ok is True, f"expected ok, got failures: {result.failures}"


# --- Missing required artifacts ---


def test_missing_model_campaign_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    _write_valid_candidate(candidate)
    (candidate / MODEL_CAMPAIGN_JSON).unlink()
    result = _validate(candidate)
    assert result.ok is False
    assert any("model-campaign" in f for f in result.failures)


def test_missing_projections_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    _write_valid_candidate(candidate)
    (candidate / CAMPAIGN_PROJECTIONS_JSONL).unlink()
    result = _validate(candidate)
    assert result.ok is False


# --- Unknown fields ---


def test_unknown_field_in_model_campaign_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    data = _write_valid_candidate(candidate)
    data["secret_field"] = "leak"
    (candidate / MODEL_CAMPAIGN_JSON).write_text(json.dumps(data, sort_keys=True))
    result = _validate(candidate)
    assert result.ok is False


# --- Duplicate identities ---


def test_duplicate_variant_in_projections_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    data = _write_valid_candidate(candidate)
    proj = data["projections"][0]
    dup = dict(proj)
    dup["evidence_link"] = "proofs/run-2/analysis.json"
    data["projections"].append(dup)
    # Re-hash using sub-models (duplicate is valid at the model level; validator catches it).
    data["content_hash"] = _recompute_hash(data)
    (candidate / MODEL_CAMPAIGN_JSON).write_text(json.dumps(data, sort_keys=True))
    (candidate / CAMPAIGN_PROJECTIONS_JSONL).write_text(
        "".join(json.dumps(r, sort_keys=True) + "\n" for r in data["projections"])
    )
    result = _validate(candidate)
    assert result.ok is False


# --- Non-finite values ---


def test_non_finite_rate_in_projection_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    data = _write_valid_candidate(candidate)
    data["projections"][0]["rate"] = float("nan")
    (candidate / MODEL_CAMPAIGN_JSON).write_text(json.dumps(data, sort_keys=True))
    (candidate / CAMPAIGN_PROJECTIONS_JSONL).write_text(
        "".join(json.dumps(r, sort_keys=True) + "\n" for r in data["projections"])
    )
    result = _validate(candidate)
    assert result.ok is False


# --- Path traversal ---


def test_path_traversal_in_evidence_link_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    data = _write_valid_candidate(candidate)
    data["projections"][0]["evidence_link"] = "../secret/analysis.json"
    # Write directly with stale hash; validator catches the invalid projection.
    (candidate / MODEL_CAMPAIGN_JSON).write_text(json.dumps(data, sort_keys=True))
    (candidate / CAMPAIGN_PROJECTIONS_JSONL).write_text(
        "".join(json.dumps(r, sort_keys=True) + "\n" for r in data["projections"])
    )
    result = _validate(candidate)
    assert result.ok is False


# --- Symlinks ---


def test_symlink_in_candidate_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    _write_valid_candidate(candidate)
    # Replace a real file with a symlink
    target = tmp_path / "outside.txt"
    target.write_text("data")
    (candidate / CAMPAIGN_PROVENANCE_JSON).unlink()
    (candidate / CAMPAIGN_PROVENANCE_JSON).symlink_to(target)
    result = _validate(candidate)
    assert result.ok is False
    assert any("symlink" in f.lower() for f in result.failures)


# --- Checksum drift ---


def test_content_hash_drift_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    data = _write_valid_candidate(candidate)
    data["content_hash"] = "b" * 64
    (candidate / MODEL_CAMPAIGN_JSON).write_text(json.dumps(data, sort_keys=True))
    result = _validate(candidate)
    assert result.ok is False


# --- Failing verification ref ---


def test_failing_verification_ref_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    _write_valid_candidate(candidate)
    ver = json.loads((candidate / CAMPAIGN_VERIFICATION_REF_JSON).read_text())
    ver["ok"] = False
    ver["failures"] = ["file_safety: missing manifest.json"]
    (candidate / CAMPAIGN_VERIFICATION_REF_JSON).write_text(json.dumps(ver, sort_keys=True))
    result = _validate(candidate)
    assert result.ok is False


# --- Projections mismatch between JSONL and model-campaign ---


def test_projection_count_mismatch_rejected(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    _write_valid_candidate(candidate)
    # Truncate the projections JSONL to zero rows
    (candidate / CAMPAIGN_PROJECTIONS_JSONL).write_text("")
    result = _validate(candidate)
    assert result.ok is False


# --- Result is typed ---


def test_validator_returns_typed_result(tmp_path: Path) -> None:
    candidate = tmp_path / "candidate"
    _write_valid_candidate(candidate)
    result = _validate(candidate)
    assert isinstance(result, PublicationValidatorResult)
    assert result.ok is True
    assert isinstance(result.checked_layers, list)
    assert isinstance(result.failures, list)
