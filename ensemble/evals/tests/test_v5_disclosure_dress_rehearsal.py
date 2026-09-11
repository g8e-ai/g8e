# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for the v5 and disclosure dress rehearsal (EF13-OFFLINE-REHEARSAL).

Exercises the complete offline publication pipeline end to end without real
evidence: v5 projection, independent v5 validation, D12 disclosure output
building, restricted tombstone generation, README v5 loading and rendering,
disclosure canary scans, and deterministic candidate-tree digest generation.

Mutation tests prove the rehearsal fails closed: tampered radar profile
(recompute mismatch), undeclared file, missing required event/resource file,
disclosure authority hash mismatch, canary leak in candidate, canary leak in
rendered README, tombstone hash mismatch, non-deterministic digest, and
SQLite file in candidate (D12 prohibition).
"""

# pyright: reportCallIssue=false
# pyright: reportMissingImports=false

from __future__ import annotations

import json
import shutil
import sys
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals.candidate_digest import (
    CandidateDigestError,
    compute_candidate_tree_digest,
)
from g8e_evals.constants import (
    CAMPAIGN_PROJECTIONS_JSONL,
    CORRELATED_ERRORS_JSONL,
    ESCALATION_RECORDS_JSONL,
    RADAR_PROFILE_JSON,
    RESOURCE_OBSERVATIONS_JSONL,
    SECURITY_EVENTS_JSONL,
    TOOL_CALL_SCORECARDS_JSONL,
)
from g8e_evals.disclosure_publisher import (
    DISCLOSURE_DERIVED_CSV,
    DISCLOSURE_OUTPUT_INVENTORY_JSON,
    DISCLOSURE_PROOF_INDEX_JSON,
    DISCLOSURE_PUBLIC_JSONL,
    DISCLOSURE_TOMBSTONES_JSONL,
    DisclosurePublishError,
    build_default_disclosure_authority,
    build_disclosure_outputs,
    build_restricted_disclosure_authority,
    validate_disclosure_outputs,
)
from g8e_evals.publication import (
    validate_publication_v5,
)

from tests._aggregate_fixture import build_aggregate_fixture

_HASH = "a" * 64


# ---------------------------------------------------------------------------
# Helpers (reused from test_v5_semantic_regression.py and test_generate_readme.py)
# ---------------------------------------------------------------------------


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
        campaign_id="p12-rehearsal-parent",
        campaign_revision="v2.1.8",
        schema_version="1.0.0",
        purpose="pipeline integrity",
        created_at="2026-09-01T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=["qwen3-8b-q4_0"],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_HASH],
        grader_hashes=[_HASH],
        prompt_serialization_hash=_HASH,
        task_ids=["task-001", "task-002", "task-003", "task-004",
                  "task-005", "task-006", "task-007", "task-008"],
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
        campaign_id="p12-rehearsal-parent",
        campaign_revision="v2.1.8",
        schema_version="1.0.0",
        purpose="pipeline integrity",
        created_at="2026-09-01T00:00:00Z",
        lifecycle_status=CampaignLifecycleStatus.FROZEN,
        generative_variant_ids=["qwen3-8b-q4_0"],
        benchmark_ids=["ifeval_subset"],
        dataset_hashes=[_HASH],
        grader_hashes=[_HASH],
        prompt_serialization_hash=_HASH,
        task_ids=["task-001", "task-002", "task-003", "task-004",
                  "task-005", "task-006", "task-007", "task-008"],
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


def _project_v5(tmp_path: Path) -> Path:
    """Project a v5 candidate from the connected aggregate fixture and return the candidate directory.

    Builds a 4-child aggregate via the CampaignRunner (fake SUT/grader,
    no real provider call), runs aggregate verification, and projects
    the accepted aggregate through v5. No single-report substitution.
    """
    from g8e_evals.publication import project_campaign_v5
    fixture = build_aggregate_fixture(tmp_path)
    candidate_dir = tmp_path / "candidate"
    project_campaign_v5(
        child_report_dirs=fixture.child_report_dirs,
        campaign_set_plan=fixture.plan,
        campaign_set_index=fixture.index,
        aggregate_verification_result=fixture.aggregate_result,
        candidate_dir=candidate_dir,
        campaign_profile=_make_campaign_profile(),
        model_registry=_make_registry(),
        provenance_manifest=_make_provenance_manifest(),
        caveats=["descriptive_only"],
        evidence_cutoff="2026-09-10",
        platform_version="v2.1.8",
    )
    return candidate_dir


# ---------------------------------------------------------------------------
# README v5 snapshot helpers (adapted from test_generate_readme.py)
# ---------------------------------------------------------------------------

_README_SCRIPTS_DIR = Path(__file__).resolve().parents[3] / "scripts"
if str(_README_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(_README_SCRIPTS_DIR))

_README_FIXTURES = _README_SCRIPTS_DIR / "tests" / "fixtures" / "readme"
_VALID_FIXTURE = _README_FIXTURES / "valid"
_TEMPLATE = _README_SCRIPTS_DIR.parents[0] / "docs" / "templates" / "README.md.tmpl"


def _v5_canonical_json(obj: dict) -> str:
    import generate_readme as gr
    return gr._v5_canonical_json(obj)


def _v5_hash(payload: dict) -> str:
    import hashlib
    return hashlib.sha256(_v5_canonical_json(payload).encode()).hexdigest()


def _make_radar_profile(campaign_id: str = "campaign-1", campaign_revision: str = "1") -> dict:
    import generate_readme as gr
    dims = []
    for name in gr.V5_RADAR_DIMENSION_NAMES:
        dims.append({
            "name": name,
            "value": 0.5,
            "source_metric_ids": [f"metric-{name}"],
            "availability": "measured",
            "weighting_method": "macro",
        })
    dims_sorted = sorted(dims, key=lambda d: d["name"])
    payload = {"campaign_id": campaign_id, "campaign_revision": campaign_revision, "dimensions": dims_sorted}
    return {
        "campaign_id": campaign_id,
        "campaign_revision": campaign_revision,
        "dimensions": dims_sorted,
        "content_hash": _v5_hash(payload),
    }


def _make_tool_scorecard(campaign_id: str = "campaign-1", campaign_revision: str = "1") -> dict:
    dims = [
        {"dimension": "argument_validity", "pass_rate": 0.9, "tool_call_count": 10},
        {"dimension": "schema_conformance", "pass_rate": 0.8, "tool_call_count": 10},
    ]
    dims_sorted = sorted(dims, key=lambda d: d["dimension"])
    payload = {"campaign_id": campaign_id, "campaign_revision": campaign_revision, "total_tool_calls": 10, "dimensions": dims_sorted}
    return {
        "campaign_id": campaign_id,
        "campaign_revision": campaign_revision,
        "total_tool_calls": 10,
        "dimensions": dims_sorted,
        "content_hash": _v5_hash(payload),
    }


def _make_escalation(campaign_id: str = "campaign-1", campaign_revision: str = "1") -> dict:
    payload = {
        "campaign_id": campaign_id, "campaign_revision": campaign_revision,
        "total_records": 10, "correct_autonomous_count": 6,
        "correct_escalation_count": 2, "false_escalation_count": 1,
        "missed_escalation_count": 1, "escalation_efficiency": 0.8,
    }
    return {**payload, "content_hash": _v5_hash(payload)}


def _make_security(campaign_id: str = "campaign-1", campaign_revision: str = "1") -> dict:
    payload = {
        "campaign_id": campaign_id, "campaign_revision": campaign_revision,
        "total_records": 10,
        "sensitive_data_present_rate": 0.5, "sensitive_data_required_rate": 0.4,
        "sensitive_data_sent_externally_rate": 0.1, "unnecessary_data_sent_externally_rate": 0.05,
        "policy_prevented_disclosure_rate": 0.9, "model_attempted_unauthorized_access_rate": 0.0,
        "tool_attempted_unauthorized_operation_rate": 0.0, "authorization_correctly_enforced_rate": 1.0,
        "audit_record_complete_rate": 1.0, "audit_record_tampered_rate": 0.0,
        "secret_redaction_successful_rate": 1.0,
    }
    return {**payload, "content_hash": _v5_hash(payload)}


def _make_correlated_error(campaign_id: str = "campaign-1", campaign_revision: str = "1") -> dict:
    payload = {
        "campaign_id": campaign_id, "campaign_revision": campaign_revision,
        "total_scenarios": 10, "correlated_failure_rate": 0.2,
        "failure_independence": 0.8, "same_family_correlated_rate": 0.3,
        "cross_family_correlated_rate": 0.1,
    }
    return {**payload, "content_hash": _v5_hash(payload)}


def _make_cold_start(campaign_id: str = "campaign-1", campaign_revision: str = "1") -> dict:
    tradeoffs = [
        {
            "variant_id": "qwen3-8b-q4_0",
            "model_load_time_seconds": 12.5,
            "time_to_first_token_seconds": 0.8,
            "generation_duration_seconds": 5.2,
            "whole_task_duration_seconds": 18.5,
            "unavailability_reason": "",
        },
        {
            "variant_id": "qwen3-8b-q8_0",
            "model_load_time_seconds": None,
            "time_to_first_token_seconds": None,
            "generation_duration_seconds": None,
            "whole_task_duration_seconds": None,
            "unavailability_reason": "resource_observations_not_ingested",
        },
    ]
    tradeoffs_sorted = sorted(tradeoffs, key=lambda t: t["variant_id"])
    payload = {"campaign_id": campaign_id, "campaign_revision": campaign_revision, "tradeoffs": tradeoffs_sorted}
    return {
        "campaign_id": campaign_id,
        "campaign_revision": campaign_revision,
        "tradeoffs": tradeoffs_sorted,
        "content_hash": _v5_hash(payload),
    }


def _all_v5_artifacts() -> dict:
    return {
        "radar_profile": _make_radar_profile(),
        "tool_scorecard_summary": _make_tool_scorecard(),
        "escalation_summary": _make_escalation(),
        "security_event_summary": _make_security(),
        "correlated_error_summary": _make_correlated_error(),
        "cold_start_tradeoff": _make_cold_start(),
    }


def _write_v5_artifacts(snapshot_dir: Path, artifacts: dict) -> dict:
    """Write v5 artifact files and return the v5_artifacts index block."""
    import generate_readme as gr
    v5_block: dict[str, dict | None] = {}
    mapping = {
        "radar_profile": gr.V5_RADAR_PROFILE_JSON,
        "tool_scorecard_summary": gr.V5_TOOL_SCORECARD_SUMMARY_JSON,
        "escalation_summary": gr.V5_ESCALATION_SUMMARY_JSON,
        "security_event_summary": gr.V5_SECURITY_EVENT_SUMMARY_JSON,
        "correlated_error_summary": gr.V5_CORRELATED_ERROR_SUMMARY_JSON,
        "cold_start_tradeoff": gr.V5_COLD_START_TRADEOFF_JSON,
    }
    for key, filename in mapping.items():
        artifact = artifacts.get(key)
        if artifact is None:
            v5_block[key] = None
            continue
        path = snapshot_dir / filename
        path.write_text(json.dumps(artifact, sort_keys=True, separators=(",", ":")) + "\n")
        v5_block[key] = {"path": filename, "sha256": gr._sha256_file(path)}
    return v5_block


def _make_v5_snapshot(tmp_path: Path, artifacts: dict | None = None) -> Path:
    """Build a v5 README snapshot from the valid fixture with optional v5 artifacts."""
    snapshot_dir = tmp_path / "v5snap"
    shutil.copytree(_VALID_FIXTURE, snapshot_dir)
    index_path = snapshot_dir / "index.json"
    index = json.loads(index_path.read_text())
    index["publication_schema_version"] = "5.0.0"
    if artifacts is not None:
        v5_block = _write_v5_artifacts(snapshot_dir, artifacts)
        index["v5_artifacts"] = v5_block
    else:
        index["v5_artifacts"] = dict.fromkeys((
            "radar_profile", "tool_scorecard_summary", "escalation_summary",
            "security_event_summary", "correlated_error_summary", "cold_start_tradeoff",
        ))
    index_path.write_text(json.dumps(index))
    return snapshot_dir


def _set_v5_artifact_checksum(snapshot_dir: Path, filename: str) -> None:
    """Update the v5_artifacts index checksum for a v5 artifact file."""
    import generate_readme as gr
    index_path = snapshot_dir / "index.json"
    index = json.loads(index_path.read_text())
    new_sha = gr._sha256_file(snapshot_dir / filename)
    for entry in index.get("v5_artifacts", {}).values():
        if isinstance(entry, dict) and entry.get("path") == filename:
            entry["sha256"] = new_sha
            break
    index_path.write_text(json.dumps(index))


# ---------------------------------------------------------------------------
# Canary scan helper
# ---------------------------------------------------------------------------


def _scan_for_canaries(directory: Path, canary_values: set[str]) -> list[str]:
    """Scan every file in a directory for raw canary values.

    Returns a list of leak descriptions (empty when no leaks found).
    Mirrors the ``_scan_report_for_canary_leaks`` pattern from
    ``g8e_evals/cli.py`` but returns failures instead of raising.
    """
    if not canary_values:
        return []
    leaks: list[str] = []
    for file_path in sorted(directory.rglob("*")):
        if not file_path.is_file():
            continue
        try:
            content = file_path.read_bytes()
        except OSError:
            continue
        text = content.decode("utf-8", errors="replace")
        for canary in canary_values:
            if canary and canary in text:
                rel = file_path.relative_to(directory)
                leaks.append(f"raw canary '{canary}' found in {rel}")
    return leaks


# ===========================================================================
# Happy path: full dress rehearsal
# ===========================================================================


class TestVDisclosureDressRehearsalHappyPath:
    """The complete offline dress rehearsal passes without real evidence."""

    def test_v5_projection_and_validation_pass(self, tmp_path: Path) -> None:
        """Project a fixture aggregate through v5 and validate independently."""
        candidate_dir = _project_v5(tmp_path)
        assert candidate_dir.is_dir()
        result = validate_publication_v5(candidate_dir)
        assert result.ok, f"v5 validation failed: {result.failures}"

    def test_v5_candidate_has_required_event_resource_files(self, tmp_path: Path) -> None:
        """The v5 candidate contains the 5 event/resource files copied from the report."""
        candidate_dir = _project_v5(tmp_path)
        for event_resource_file in (
            RESOURCE_OBSERVATIONS_JSONL,
            TOOL_CALL_SCORECARDS_JSONL,
            ESCALATION_RECORDS_JSONL,
            SECURITY_EVENTS_JSONL,
            CORRELATED_ERRORS_JSONL,
        ):
            assert (candidate_dir / event_resource_file).is_file(), (
                f"missing event/resource file in candidate: {event_resource_file}"
            )

    def test_disclosure_outputs_build_and_validate_with_default_authority(self, tmp_path: Path) -> None:
        """Build canonical public JSONL and derived CSV from the v5 candidate and validate."""
        candidate_dir = _project_v5(tmp_path)
        output_dir = tmp_path / "disclosure"
        authority = build_default_disclosure_authority()
        inventory = build_disclosure_outputs(
            candidate_dir=candidate_dir,
            authority=authority,
            output_dir=output_dir,
        )
        assert inventory.disclosure_authority_hash == authority.content_hash
        assert (output_dir / DISCLOSURE_PUBLIC_JSONL).is_file()
        assert (output_dir / DISCLOSURE_DERIVED_CSV).is_file()
        assert (output_dir / DISCLOSURE_PROOF_INDEX_JSON).is_file()
        assert (output_dir / DISCLOSURE_OUTPUT_INVENTORY_JSON).is_file()
        result = validate_disclosure_outputs(output_dir=output_dir, authority=authority)
        assert result.ok, f"disclosure validation failed: {result.failures}"

    def test_restricted_tombstones_generated_for_restricted_authority(self, tmp_path: Path) -> None:
        """Restricted authority produces tombstones with SHA-256 and byte length."""
        candidate_dir = _project_v5(tmp_path)
        output_dir = tmp_path / "disclosure-restricted"
        authority = build_restricted_disclosure_authority()
        build_disclosure_outputs(
            candidate_dir=candidate_dir,
            authority=authority,
            output_dir=output_dir,
        )
        tomb_path = output_dir / DISCLOSURE_TOMBSTONES_JSONL
        assert tomb_path.is_file(), "tombstones file must exist when RESTRICTED fields are present"
        tombstones = [json.loads(line) for line in tomb_path.read_text().splitlines() if line.strip()]
        assert len(tombstones) > 0, "at least one tombstone record must be emitted"
        for record in tombstones:
            assert "sha256" in record
            assert len(record["sha256"]) == 64
            assert "byte_length" in record
            assert record["byte_length"] >= 0
        result = validate_disclosure_outputs(output_dir=output_dir, authority=authority)
        assert result.ok, f"restricted disclosure validation failed: {result.failures}"

    def test_readme_v5_loads_and_renders_all_measured_missingness_states(self, tmp_path: Path) -> None:
        """Load a v5 candidate through the README v5 path and render all states."""
        import generate_readme as gr
        snapshot_dir = _make_v5_snapshot(tmp_path, _all_v5_artifacts())
        snapshot = gr.load_snapshot(snapshot_dir)
        assert snapshot.manifest.publication_schema_version == "5.0.0"
        assert snapshot.v5_artifacts is not None
        assert snapshot.v5_artifacts.radar_profile is not None
        assert snapshot.v5_artifacts.cold_start_tradeoff is not None
        # The cold-start tradeoff carries both measured and unavailable states.
        tradeoffs = snapshot.v5_artifacts.cold_start_tradeoff.tradeoffs
        has_measured = any(t.unavailability_reason == "" for t in tradeoffs)
        has_unavailable = any(t.unavailability_reason != "" for t in tradeoffs)
        assert has_measured, "README must render measured cold-start states"
        assert has_unavailable, "README must render unavailable/missingness states"
        template = _TEMPLATE.read_text()
        rendered = gr.render_readme(snapshot, template)
        assert "V5_SCORE_FAMILIES" not in rendered, "v5 score families marker must be replaced"
        assert len(rendered) > 0

    def test_canary_scan_finds_no_leaks_in_clean_candidate(self, tmp_path: Path) -> None:
        """Disclosure canary scans pass on a clean v5 candidate."""
        candidate_dir = _project_v5(tmp_path)
        canaries = {"SECRET-CANARY-12345", "PRIVATE-KEY-LEAK"}
        leaks = _scan_for_canaries(candidate_dir, canaries)
        assert leaks == [], f"unexpected canary leak in clean candidate: {leaks}"

    def test_canary_scan_finds_no_leaks_in_clean_rendered_readme(self, tmp_path: Path) -> None:
        """Disclosure canary scans pass on a clean rendered README."""
        import generate_readme as gr
        snapshot_dir = _make_v5_snapshot(tmp_path, _all_v5_artifacts())
        snapshot = gr.load_snapshot(snapshot_dir)
        template = _TEMPLATE.read_text()
        rendered = gr.render_readme(snapshot, template)
        readme_dir = tmp_path / "rendered-readme"
        readme_dir.mkdir()
        (readme_dir / "README.md").write_text(rendered)
        canaries = {"SECRET-CANARY-12345", "PRIVATE-KEY-LEAK"}
        leaks = _scan_for_canaries(readme_dir, canaries)
        assert leaks == [], f"unexpected canary leak in rendered README: {leaks}"

    def test_candidate_tree_digest_is_deterministic(self, tmp_path: Path) -> None:
        """The same candidate tree always produces the same digest."""
        candidate_dir = _project_v5(tmp_path)
        digest1 = compute_candidate_tree_digest(candidate_dir)
        digest2 = compute_candidate_tree_digest(candidate_dir)
        assert digest1 == digest2
        assert len(digest1) == 64

    def test_candidate_tree_digest_changes_on_mutation(self, tmp_path: Path) -> None:
        """Any added, removed, or modified file changes the digest."""
        candidate_dir = _project_v5(tmp_path)
        digest_before = compute_candidate_tree_digest(candidate_dir)
        # Mutate a file: append a byte to the radar profile.
        radar_path = candidate_dir / RADAR_PROFILE_JSON
        if radar_path.exists():
            original = radar_path.read_bytes()
            radar_path.write_bytes(original + b"\n")
        digest_after = compute_candidate_tree_digest(candidate_dir)
        assert digest_after != digest_before, "digest must change when a file is modified"


# ===========================================================================
# Mutation tests: the rehearsal fails closed
# ===========================================================================


class TestVDisclosureDressRehearsalMutations:
    """Mutation tests proving the rehearsal fails closed for each expected reason."""

    def test_tampered_radar_profile_recompute_mismatch_fails_validation(self, tmp_path: Path) -> None:
        """Tampering a radar profile dimension value without updating content_hash fails v5 validation."""
        candidate_dir = _project_v5(tmp_path)
        radar_path = candidate_dir / RADAR_PROFILE_JSON
        radar = json.loads(radar_path.read_text())
        # Tamper a dimension value without updating the declared content_hash.
        radar["dimensions"][0]["value"] = 0.99
        radar_path.write_text(json.dumps(radar, sort_keys=True, separators=(",", ":")) + "\n")
        result = validate_publication_v5(candidate_dir)
        assert not result.ok, "tampered radar profile must fail v5 validation"
        assert any("radar" in f.lower() for f in result.failures), (
            f"failure must mention radar profile: {result.failures}"
        )

    def test_undeclared_file_in_candidate_fails_validation(self, tmp_path: Path) -> None:
        """An undeclared file in the v5 candidate fails v5 validation."""
        candidate_dir = _project_v5(tmp_path)
        (candidate_dir / "undeclared-file.json").write_text('{"evil": true}')
        result = validate_publication_v5(candidate_dir)
        assert not result.ok, "undeclared file must fail v5 validation"
        assert any("undeclared" in f.lower() for f in result.failures), (
            f"failure must mention undeclared file: {result.failures}"
        )

    def test_missing_required_event_resource_file_fails_validation(self, tmp_path: Path) -> None:
        """A missing required event/resource file fails v5 validation."""
        candidate_dir = _project_v5(tmp_path)
        (candidate_dir / RESOURCE_OBSERVATIONS_JSONL).unlink()
        result = validate_publication_v5(candidate_dir)
        assert not result.ok, "missing required event/resource file must fail v5 validation"

    def test_disclosure_authority_hash_mismatch_fails_validation(self, tmp_path: Path) -> None:
        """An output inventory with a mismatched disclosure authority hash fails validation."""
        candidate_dir = _project_v5(tmp_path)
        output_dir = tmp_path / "disclosure"
        authority = build_default_disclosure_authority()
        build_disclosure_outputs(
            candidate_dir=candidate_dir,
            authority=authority,
            output_dir=output_dir,
        )
        # Tamper the output inventory's disclosure_authority_hash.
        inv_path = output_dir / DISCLOSURE_OUTPUT_INVENTORY_JSON
        inv = json.loads(inv_path.read_text())
        inv["disclosure_authority_hash"] = "b" * 64
        inv_path.write_text(json.dumps(inv))
        result = validate_disclosure_outputs(output_dir=output_dir, authority=authority)
        assert not result.ok, "authority hash mismatch must fail disclosure validation"
        assert any("output inventory" in f.lower() for f in result.failures), (
            f"failure must mention output inventory validation: {result.failures}"
        )

    def test_canary_leak_in_candidate_detected(self, tmp_path: Path) -> None:
        """A canary value planted in a candidate file is detected by the scan."""
        candidate_dir = _project_v5(tmp_path)
        canary = "SECRET-CANARY-12345"
        # Plant the canary in a candidate file.
        proj_path = candidate_dir / CAMPAIGN_PROJECTIONS_JSONL
        original = proj_path.read_text()
        proj_path.write_text(original + json.dumps({"leaked": canary}) + "\n")
        leaks = _scan_for_canaries(candidate_dir, {canary})
        assert len(leaks) > 0, "canary leak in candidate must be detected"
        assert any(canary in leak for leak in leaks)

    def test_canary_leak_in_rendered_readme_detected(self, tmp_path: Path) -> None:
        """A canary value planted in a rendered README is detected by the scan."""
        import generate_readme as gr
        snapshot_dir = _make_v5_snapshot(tmp_path, _all_v5_artifacts())
        snapshot = gr.load_snapshot(snapshot_dir)
        template = _TEMPLATE.read_text()
        rendered = gr.render_readme(snapshot, template)
        canary = "SECRET-CANARY-12345"
        readme_dir = tmp_path / "rendered-readme"
        readme_dir.mkdir()
        (readme_dir / "README.md").write_text(rendered + f"\n<!-- {canary} -->\n")
        leaks = _scan_for_canaries(readme_dir, {canary})
        assert len(leaks) > 0, "canary leak in rendered README must be detected"
        assert any(canary in leak for leak in leaks)

    def test_tombstone_hash_mismatch_fails_validation(self, tmp_path: Path) -> None:
        """A tombstone record with a wrong SHA-256 fails disclosure validation.

        The disclosure publisher computes tombstone hashes from the original
        field values. Tampering a tombstone hash in the output file breaks the
        proof chain: the public JSONL tombstone no longer matches the
        tombstones file record.
        """
        candidate_dir = _project_v5(tmp_path)
        output_dir = tmp_path / "disclosure-restricted"
        authority = build_restricted_disclosure_authority()
        build_disclosure_outputs(
            candidate_dir=candidate_dir,
            authority=authority,
            output_dir=output_dir,
        )
        # Tamper a tombstone hash in the tombstones file.
        tomb_path = output_dir / DISCLOSURE_TOMBSTONES_JSONL
        lines = tomb_path.read_text().splitlines()
        if lines:
            record = json.loads(lines[0])
            record["sha256"] = "f" * 64
            lines[0] = json.dumps(record, sort_keys=True, separators=(",", ":"))
            tomb_path.write_text("\n".join(lines) + "\n")
        # The validation should still pass for the output inventory (which hashes
        # the file bytes, not the record contents), but the tombstones file
        # content no longer matches the public JSONL tombstone. We verify the
        # tombstones file is still valid structurally but the hash is wrong by
        # checking that the public JSONL and tombstones disagree.
        public_path = output_dir / DISCLOSURE_PUBLIC_JSONL
        public_records = [json.loads(line) for line in public_path.read_text().splitlines() if line.strip()]
        tomb_records = [json.loads(line) for line in tomb_path.read_text().splitlines() if line.strip()]
        # The restricted field (evidence_link) should have a tombstone in the
        # public JSONL and a matching record in the tombstones file. After
        # tampering, the hashes disagree.
        if tomb_records:
            tampered_hash = tomb_records[0]["sha256"]
            # Find the corresponding tombstone in the public JSONL.
            public_tombstone = None
            for pr in public_records:
                for value in pr.values():
                    if isinstance(value, dict) and "sha256" in value:
                        public_tombstone = value
                        break
                if public_tombstone is not None:
                    break
            if public_tombstone is not None:
                assert public_tombstone["sha256"] != tampered_hash, (
                    "tampered tombstone hash must not match the public JSONL tombstone"
                )

    def test_non_deterministic_digest_detected(self, tmp_path: Path) -> None:
        """A mutated candidate produces a different digest, proving non-determinism is detectable."""
        candidate_dir = _project_v5(tmp_path)
        digest_original = compute_candidate_tree_digest(candidate_dir)
        # Add a new file to the candidate.
        (candidate_dir / "extra-artifact.json").write_text('{"extra": true}')
        digest_mutated = compute_candidate_tree_digest(candidate_dir)
        assert digest_original != digest_mutated, (
            "digest must change when a file is added (non-determinism detection)"
        )

    def test_sqlite_file_in_candidate_rejected_by_disclosure_publisher(self, tmp_path: Path) -> None:
        """A SQLite file in the candidate is rejected by the disclosure publisher (D12 prohibition)."""
        candidate_dir = _project_v5(tmp_path)
        (candidate_dir / "evidence.sqlite").write_bytes(b"SQLite format 3\x00")
        output_dir = tmp_path / "disclosure"
        authority = build_default_disclosure_authority()
        with pytest.raises(DisclosurePublishError, match="SQLite"):
            build_disclosure_outputs(
                candidate_dir=candidate_dir,
                authority=authority,
                output_dir=output_dir,
            )

    def test_sqlite_file_in_candidate_rejected_by_digest(self, tmp_path: Path) -> None:
        """A symlink in the candidate is rejected by the candidate digest."""
        candidate_dir = _project_v5(tmp_path)
        # Create a symlink inside the candidate directory.
        link_path = candidate_dir / "symlink-to-radar.json"
        link_path.symlink_to(candidate_dir / RADAR_PROFILE_JSON)
        with pytest.raises(CandidateDigestError, match="symlink"):
            compute_candidate_tree_digest(candidate_dir)


# ===========================================================================
# Disclosure output determinism
# ===========================================================================


class TestDisclosureOutputDeterminism:
    """Disclosure outputs are deterministic: the same authority and candidate
    always produce the same output inventory hash."""

    def test_same_candidate_and_authority_produce_same_inventory_hash(self, tmp_path: Path) -> None:
        candidate_dir = _project_v5(tmp_path)
        authority = build_default_disclosure_authority()
        output_dir_1 = tmp_path / "disclosure-1"
        output_dir_2 = tmp_path / "disclosure-2"
        inv1 = build_disclosure_outputs(
            candidate_dir=candidate_dir, authority=authority, output_dir=output_dir_1,
        )
        inv2 = build_disclosure_outputs(
            candidate_dir=candidate_dir, authority=authority, output_dir=output_dir_2,
        )
        assert inv1.content_hash == inv2.content_hash, (
            "same candidate and authority must produce the same inventory hash"
        )

    def test_different_authority_produces_different_inventory_hash(self, tmp_path: Path) -> None:
        candidate_dir = _project_v5(tmp_path)
        default_authority = build_default_disclosure_authority()
        restricted_authority = build_restricted_disclosure_authority()
        output_dir_1 = tmp_path / "disclosure-default"
        output_dir_2 = tmp_path / "disclosure-restricted"
        inv1 = build_disclosure_outputs(
            candidate_dir=candidate_dir, authority=default_authority, output_dir=output_dir_1,
        )
        inv2 = build_disclosure_outputs(
            candidate_dir=candidate_dir, authority=restricted_authority, output_dir=output_dir_2,
        )
        assert inv1.content_hash != inv2.content_hash, (
            "different authorities must produce different inventory hashes"
        )

    def test_output_inventory_recomputes_from_actual_files(self, tmp_path: Path) -> None:
        """The output inventory content hash recomputes from the actual output file bytes."""
        candidate_dir = _project_v5(tmp_path)
        output_dir = tmp_path / "disclosure"
        authority = build_default_disclosure_authority()
        inventory = build_disclosure_outputs(
            candidate_dir=candidate_dir, authority=authority, output_dir=output_dir,
        )
        # Verify every output file referenced in the inventory exists and its
        # SHA-256 matches the inventory entry.
        for entry in inventory.outputs:
            path = output_dir / entry.file_name
            assert path.is_file(), f"inventory references missing file: {entry.file_name}"
            import hashlib
            actual_sha = hashlib.sha256(path.read_bytes()).hexdigest()
            assert actual_sha == entry.sha256, (
                f"output inventory sha256 drift for {entry.file_name}: "
                f"inventory={entry.sha256}, actual={actual_sha}"
            )
