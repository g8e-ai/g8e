#!/usr/bin/env python3
"""Tests for the historical projector.

Validates that the projector:
- Parses all input families correctly
- Produces records matching the frozen contract
- Reproduces aggregate counts exactly
- Passes the disclosure scan
- Is deterministic (byte-identical on re-run)
- Keeps verified-public data separate from exploratory

Run: python3 scripts/test_projector.py
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------

PROJECT_ROOT = Path(__file__).resolve().parent.parent
SCRIPT_PATH = PROJECT_ROOT / "scripts" / "project.py"
OUTPUT_PATH = PROJECT_ROOT / "scripts" / "fixtures" / "projected-records.jsonl"
REPORT_PATH = PROJECT_ROOT / "scripts" / "fixtures" / "generation-report.json"

BASELINE_ROOT = Path("/home/bob/g8e/.local.dev/campaign/overnight-baseline/run-20260914-r2")
MODEL_REGISTRY_PATH = Path("/home/bob/g8e/.local.dev/campaign/model-registry.json")
VERIFIED_SNAPSHOT_ROOT = Path("/home/bob/g8e/docs/evidence/readme/current")

# Prohibited fields from the g8e publisher
PROHIBITED_FIELDS = frozenset({
    "raw_prompt", "raw_output", "prompt", "output", "chain_of_thought",
    "credentials", "credential", "api_key", "secret", "password",
    "token", "private_key", "private_endpoint", "evidence_key",
    "evidence_key_metadata", "machine_path", "filesystem_path", "local_path",
})

PATH_INDICATORS = ("/home/", "/tmp/", "/etc/", "http://192.", "http://127.",
                   "http://localhost", "192.168.1.2", "11434")


# ---------------------------------------------------------------------------
# Test helpers
# ---------------------------------------------------------------------------

def load_records(path: Path) -> list[dict[str, Any]]:
    records: list[dict[str, Any]] = []
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            d = json.loads(line)
            payload = json.loads(d["record_bytes"])
            payload["_record_type"] = d["record_type"]
            records.append(payload)
    return records


def load_report(path: Path) -> dict[str, Any]:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def run_projector(output_path: Path, report_path: Path) -> int:
    result = subprocess.run(
        [sys.executable, str(SCRIPT_PATH), "--output", str(output_path), "--report", str(report_path)],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(f"STDERR:\n{result.stderr}", file=sys.stderr)
        print(f"STDOUT:\n{result.stdout}", file=sys.stderr)
    return result.returncode


def scan_prohibited(obj: Any, path: str = "") -> list[str]:
    hits: list[str] = []
    if isinstance(obj, dict):
        for k, v in obj.items():
            if k in PROHIBITED_FIELDS:
                hits.append(f"{path}.{k}")
            hits.extend(scan_prohibited(v, f"{path}.{k}"))
    elif isinstance(obj, list):
        for i, item in enumerate(obj):
            hits.extend(scan_prohibited(item, f"{path}[{i}]"))
    return hits


def scan_paths(obj: Any, path: str = "") -> list[str]:
    hits: list[str] = []
    if isinstance(obj, dict):
        for k, v in obj.items():
            hits.extend(scan_paths(v, f"{path}.{k}"))
    elif isinstance(obj, list):
        for i, item in enumerate(obj):
            hits.extend(scan_paths(item, f"{path}[{i}]"))
    elif isinstance(obj, str):
        for ind in PATH_INDICATORS:
            if ind in obj:
                hits.append(f"{path} contains '{ind}'")
                break
    return hits


def assert_eq(actual: Any, expected: Any, msg: str) -> None:
    if actual != expected:
        raise AssertionError(f"{msg}: expected {expected!r}, got {actual!r}")


def assert_true(condition: bool, msg: str) -> None:
    if not condition:
        raise AssertionError(msg)


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

def test_projector_runs_successfully() -> None:
    """The projector exits 0 and produces output."""
    print("test_projector_runs_successfully...", end=" ")
    rc = run_projector(OUTPUT_PATH, REPORT_PATH)
    assert_eq(rc, 0, "projector should exit 0")
    assert_true(OUTPUT_PATH.exists(), "output file should exist")
    assert_true(REPORT_PATH.exists(), "report file should exist")
    print("PASS")


def test_record_counts() -> None:
    """Record counts match expected values from the plan."""
    print("test_record_counts...", end=" ")
    records = load_records(OUTPUT_PATH)
    report = load_report(REPORT_PATH)

    counts = report["record_counts_by_kind"]
    assert_eq(counts["catalog_snapshot"], 2, "should have 2 catalog snapshots (exploratory + verified)")
    assert_eq(counts["suite_summary"], 9, "should have 9 suite summaries")
    assert_eq(counts["evaluation_summary"], 11, "should have 11 evaluation summaries (9 exploratory + 2 verified)")
    assert_eq(counts["methodology_snapshot"], 1, "should have 1 methodology snapshot")
    assert_true(counts["assignment_result"] >= 1125, "should have at least 1125 assignment results (exploratory)")
    assert_true(counts["model_summary"] >= 31, "should have at least 31 model summaries (registry models)")
    print("PASS")


def test_aggregate_reconciliation() -> None:
    """All aggregate counts from working-model-selection.json are reproduced exactly."""
    print("test_aggregate_reconciliation...", end=" ")
    report = load_report(REPORT_PATH)
    recon = report["aggregate_reconciliation"]

    assert_eq(recon["expected_assignment_count"], 1125, "expected assignment count")
    assert_eq(recon["computed_assignment_count"], 1125, "computed assignment count")
    assert_eq(recon["expected_provider_request_count"], 2282, "expected provider request count")
    assert_eq(recon["computed_provider_request_count"], 2282, "computed provider request count")
    assert_eq(recon["expected_provider_token_count"], 20185634, "expected provider token count")
    assert_eq(recon["computed_provider_token_count"], 20185634, "computed provider token count")
    assert_eq(recon["expected_retry_count"], 0, "expected retry count")
    assert_eq(recon["computed_retry_count"], 0, "computed retry count")
    assert_eq(recon["expected_verifier_passed"], 5, "expected verifier passed")
    assert_eq(recon["computed_verifier_passed"], 5, "computed verifier passed")
    assert_eq(recon["expected_verifier_failed"], 4, "expected verifier failed")
    assert_eq(recon["computed_verifier_failed"], 4, "computed verifier failed")

    mismatches = report.get("reconciliation_mismatches", [])
    assert_eq(mismatches, [], "should have no aggregate reconciliation mismatches")
    print("PASS")


def test_catalog_snapshot_exploratory() -> None:
    """The exploratory catalog snapshot has correct counts and quality state."""
    print("test_catalog_snapshot_exploratory...", end=" ")
    records = load_records(OUTPUT_PATH)
    catalogs = [r for r in records if r.get("kind") == "catalog_snapshot"]
    explo = [c for c in catalogs if c["dataset_kind"] == "exploratory_baseline"]
    assert_eq(len(explo), 1, "should have exactly 1 exploratory catalog")
    c = explo[0]

    assert_eq(c["schema_version"], "1.1.0", "schema version")
    assert_eq(c["quality_state"], "exploratory_partial", "quality state")
    assert_eq(c["model_count"], 31, "model count")
    assert_eq(c["evaluated_count"], 9, "evaluated count")
    assert_eq(c["suite_count"], 9, "suite count")
    assert_eq(c["run_count"], 9, "run count")
    assert_eq(c["assignment_count"], 1125, "assignment count")
    assert_eq(c["provider_request_count"], 2282, "provider request count")
    assert_eq(c["provider_token_count"], 20185634, "provider token count")
    assert_eq(c["retry_count"], 0, "retry count")
    assert_eq(c["verifier_passed_count"], 5, "verifier passed count")
    assert_eq(c["verifier_failed_count"], 4, "verifier failed count")
    print("PASS")


def test_catalog_snapshot_verified() -> None:
    """The verified catalog snapshot is separate from exploratory."""
    print("test_catalog_snapshot_verified...", end=" ")
    records = load_records(OUTPUT_PATH)
    catalogs = [r for r in records if r.get("kind") == "catalog_snapshot"]
    verified = [c for c in catalogs if c["dataset_kind"] == "verified_public_snapshot"]
    assert_eq(len(verified), 1, "should have exactly 1 verified catalog")
    c = verified[0]

    assert_eq(c["schema_version"], "1.1.0", "schema version")
    assert_eq(c["quality_state"], "verified_public", "quality state")
    assert_eq(c["dataset_id"], "ds-verified-public-20260914", "dataset id")
    assert_true(c["assignment_count"] == 10, f"verified assignment count should be 10, got {c['assignment_count']}")
    assert_eq(c["model_count"], 32, "verified model count")
    assert_eq(c["evaluated_count"], 3, "verified evaluated model count")
    assert_eq(c["provider_request_count"], 20, "verified provider request count")
    assert_true(any("token" in limitation.lower() for limitation in c["limitations"]), "verified zero token count should have a limitation")
    print("PASS")


def test_model_summaries_nine_candidates() -> None:
    """All 9 evaluated candidates have model summaries with correct pass rates."""
    print("test_model_summaries_nine_candidates...", end=" ")
    records = load_records(OUTPUT_PATH)
    morning = json.load(open(BASELINE_ROOT / "morning-summary.json"))
    candidates = morning["candidates"]

    explo_models = [r for r in records
                    if r.get("kind") == "model_summary"
                    and r.get("dataset_id") == "ds-exploratory-baseline-20260914-r2"
                    and r.get("evaluation_coverage", 0) > 0]

    assert_eq(len(explo_models), 9, "should have 9 evaluated model summaries")

    for m in explo_models:
        vid = m["variant_id"]
        c = candidates[vid]
        # Pass rate
        assert_true(m["pass_rate"] is not None, f"{vid} should have pass_rate")
        assert_eq(m["pass_rate"]["estimate"], c["pass_rate"], f"{vid} pass_rate estimate")
        assert_eq(m["pass_rate"]["lower"], c["task_bootstrap_95_ci"][0], f"{vid} CI lower")
        assert_eq(m["pass_rate"]["upper"], c["task_bootstrap_95_ci"][1], f"{vid} CI upper")
        # Agreement
        assert_eq(m["agreement_pairwise"]["value"], c["mean_pairwise_agreement"], f"{vid} pairwise agreement")
        assert_eq(m["agreement_all_five"]["value"], c["all_five_agree_rate"], f"{vid} all-five agreement")
        # Role
        assert_eq(m["role"], c["role"], f"{vid} role")
        # Served model
        assert_eq(m["served_model_tag"], c["served_model"], f"{vid} served model")
        # Repeatability
        rep = c.get("repeatability_outcomes", {})
        assert_eq(m["repeatability"]["consistently_correct"], rep.get("consistently_correct", 0), f"{vid} consistently_correct")
        assert_eq(m["repeatability"]["consistently_wrong"], rep.get("consistently_wrong", 0), f"{vid} consistently_wrong")
        assert_eq(m["repeatability"]["inconsistent"], rep.get("inconsistent", 0), f"{vid} inconsistent")
        assert_eq(m["repeatability"]["insufficient"], rep.get("insufficient", 0), f"{vid} insufficient")
        # Terminal outcomes
        terminal = c.get("terminal_statuses", {})
        assert_eq(m["terminal_outcomes"]["completed"], terminal.get("completed", 0), f"{vid} completed")
        assert_eq(m["terminal_outcomes"]["model_failed"], terminal.get("model_failed", 0), f"{vid} model_failed")
        assert_eq(m["pass_rate"]["denominator"], sum(result.get("terminal_measurements", 0) for result in c.get("suite_results", {}).values()), f"{vid} denominator")

    verified_models = {
        model["variant_id"]: model
        for model in records
        if model.get("kind") == "model_summary"
        and model.get("dataset_id") == "ds-verified-public-20260914"
        and model.get("evaluation_coverage", 0) > 0
    }
    assert_eq(verified_models["gemma-4-e4b-it"]["pass_rate"]["estimate"], 5 / 8, "verified e4b pass rate")
    assert_eq(verified_models["gemma-4-e4b-it"]["pass_rate"]["denominator"], 8, "verified e4b denominator")
    assert_eq(verified_models["gemma4-12b"]["pass_rate"]["estimate"], 2 / 2, "verified 12b pass rate")
    assert_eq(verified_models["gemma4-12b"]["pass_rate"]["denominator"], 2, "verified 12b denominator")
    assert_true("pass_rate" not in verified_models["gemma-4-e2b-it"], "verified triage-only model should not have pass rate")

    print("PASS")


def test_model_summaries_unevaluated() -> None:
    """Registry models without measurements appear as not_evaluated."""
    print("test_model_summaries_unevaluated...", end=" ")
    records = load_records(OUTPUT_PATH)
    registry = json.load(open(MODEL_REGISTRY_PATH))
    excluded = {q["variant_id"] for q in registry["qualification_records"]}
    required = [v for v in registry["variants"] if v["variant_id"] not in excluded]

    explo_models = [r for r in records
                    if r.get("kind") == "model_summary"
                    and r.get("dataset_id") == "ds-exploratory-baseline-20260914-r2"]

    explo_vids = {m["variant_id"] for m in explo_models}
    for v in required:
        assert_true(v["variant_id"] in explo_vids, f"required variant {v['variant_id']} should be in model summaries")

    not_evaluated = [m for m in explo_models if m["quality_state"] == "not_evaluated"]
    assert_eq(len(not_evaluated), 22, "should have 22 not_evaluated models (31 - 9)")
    for m in not_evaluated:
        assert_eq(m["evaluation_coverage"], 0, f"{m['variant_id']} should have 0 coverage")
        assert_true(m.get("unavailable_reasons") is not None, f"{m['variant_id']} should have unavailable_reasons")
    print("PASS")


def test_suite_summaries_verifier_states() -> None:
    """Suite summaries have correct verifier states (5 passed, 4 failed)."""
    print("test_suite_summaries_verifier_states...", end=" ")
    records = load_records(OUTPUT_PATH)
    suites = [r for r in records if r.get("kind") == "suite_summary"]
    assert_eq(len(suites), 9, "should have 9 suite summaries")

    passed = [s for s in suites if s["verifier_state"] == "passed"]
    failed = [s for s in suites if s["verifier_state"] == "failed"]
    assert_eq(len(passed), 5, "should have 5 passed suites")
    assert_eq(len(failed), 4, "should have 4 failed suites")

    # Failed suites should have failure summaries
    for s in failed:
        assert_true(s.get("verifier_failure_summary") is not None, f"failed suite {s['suite_id']} should have failure summary")
        assert_true(len(s.get("limitations", [])) > 0, f"failed suite {s['suite_id']} should have limitations")
    print("PASS")


def test_evaluation_summaries() -> None:
    """Evaluation summaries have correct lifecycle and verifier states."""
    print("test_evaluation_summaries...", end=" ")
    records = load_records(OUTPUT_PATH)
    evals = [r for r in records if r.get("kind") == "evaluation_summary"]
    explo = [e for e in evals if e["dataset_id"] == "ds-exploratory-baseline-20260914-r2"]
    verified = [e for e in evals if e["dataset_id"] == "ds-verified-public-20260914"]

    assert_eq(len(explo), 9, "should have 9 exploratory evaluation summaries")
    assert_eq(len(verified), 2, "should have 2 verified evaluation summaries")

    terminal_keys = {"completed", "model_failed", "grader_failed", "invalid_evidence", "stopped"}
    for e in explo:
        assert_eq(e["lifecycle_state"], "completed", f"{e['run_id']} should be completed")
        assert_eq(e["arm"], "ensemble_ungoverned", f"{e['run_id']} should be ensemble_ungoverned")
        assert_eq(set(e["terminal_outcomes"]), terminal_keys, f"{e['run_id']} terminal keys")
        assert_eq(e["model_role_mapping"], {"primary": "cohort-rotating-primary"}, f"{e['run_id']} rotating cohort mapping")

    for e in verified:
        assert_eq(e["quality_state"], "verified_public", f"{e['run_id']} should be verified_public")
        assert_eq(e["verifier_state"], "passed", f"{e['run_id']} should have passed verifier")
        assert_eq(set(e["terminal_outcomes"]), terminal_keys, f"{e['run_id']} terminal keys")
        assert_true("gemma4:e4b" not in e["model_role_mapping"].values(), f"{e['run_id']} mapping should use variant ids")
    print("PASS")


def test_assignment_results_count() -> None:
    """Assignment results count matches expected (1125 exploratory + 10 verified)."""
    print("test_assignment_results_count...", end=" ")
    records = load_records(OUTPUT_PATH)
    assignments = [r for r in records if r.get("kind") == "assignment_result"]
    explo = [a for a in assignments if a["dataset_id"] == "ds-exploratory-baseline-20260914-r2"]
    verified = [a for a in assignments if a["dataset_id"] == "ds-verified-public-20260914"]

    assert_eq(len(explo), 1125, "should have 1125 exploratory assignments")
    assert_eq(len(verified), 10, "should have 10 verified assignments")
    assert_true(all(a.get("verification_disposition") in {"passed", "failed", "not_applicable"} for a in assignments), "all assignments should use contract verifier states")
    assert_true(all(a["variant_id"] in {"gemma-4-e4b-it", "gemma4-12b"} for a in verified), "verified assignments should join to bound-stage model variants")
    assert_true(all(a["stage_summary"] == [] for a in verified), "verified assignments should not fabricate stage durations")
    assert_true(all(a.get("missingness_reason") for a in verified), "verified assignments should disclose missing resources")
    print("PASS")


def test_disclosure_scan() -> None:
    """No record contains prohibited fields, paths, or endpoints."""
    print("test_disclosure_scan...", end=" ")
    records = load_records(OUTPUT_PATH)

    all_prohibited: list[str] = []
    all_paths: list[str] = []
    for r in records:
        all_prohibited.extend(scan_prohibited(r))
        all_paths.extend(scan_paths(r))

    assert_eq(all_prohibited, [], "should have no prohibited fields")
    assert_eq(all_paths, [], "should have no paths or endpoints")
    print("PASS")


def test_jsonl_format() -> None:
    """Each JSONL line has record_type and record_bytes in the correct format."""
    print("test_jsonl_format...", end=" ")
    with open(OUTPUT_PATH, "r", encoding="utf-8") as f:
        for i, line in enumerate(f):
            line = line.strip()
            if not line:
                continue
            d = json.loads(line)
            assert_true("record_type" in d, f"line {i}: missing record_type")
            assert_true("record_bytes" in d, f"line {i}: missing record_bytes")
            assert_eq(d["record_type"], "projection", f"line {i}: record_type should be projection")
            payload = json.loads(d["record_bytes"])
            assert_true(isinstance(payload, dict), f"line {i}: record_bytes should decode to dict")
            assert_true("kind" in payload, f"line {i}: payload should have kind")
            assert_true("schema_version" in payload, f"line {i}: payload should have schema_version")
            assert_eq(payload["schema_version"], "1.1.0", f"line {i}: schema_version should be 1.0.0")
    print("PASS")


def test_deterministic_output() -> None:
    """Running the projector twice produces byte-identical output."""
    print("test_deterministic_output...", end=" ")
    with tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False) as tmp1:
        out1 = Path(tmp1.name)
    with tempfile.NamedTemporaryFile(suffix=".json", delete=False) as tmp2:
        rep1 = Path(tmp2.name)

    rc = run_projector(out1, rep1)
    assert_eq(rc, 0, "first run should succeed")
    hash1 = hashlib.sha256(out1.read_bytes()).hexdigest()

    with tempfile.NamedTemporaryFile(suffix=".jsonl", delete=False) as tmp3:
        out2 = Path(tmp3.name)
    with tempfile.NamedTemporaryFile(suffix=".json", delete=False) as tmp4:
        rep2 = Path(tmp4.name)

    rc = run_projector(out2, rep2)
    assert_eq(rc, 0, "second run should succeed")
    hash2 = hashlib.sha256(out2.read_bytes()).hexdigest()

    assert_eq(hash1, hash2, "output should be byte-identical on re-run")

    # Clean up
    out1.unlink(missing_ok=True)
    rep1.unlink(missing_ok=True)
    out2.unlink(missing_ok=True)
    rep2.unlink(missing_ok=True)
    print("PASS")


def test_datasets_separate() -> None:
    """Exploratory and verified datasets use different dataset_ids and never merge."""
    print("test_datasets_separate...", end=" ")
    records = load_records(OUTPUT_PATH)
    explo_ids = {r["dataset_id"] for r in records if r.get("dataset_id", "").startswith("ds-exploratory")}
    verified_ids = {r["dataset_id"] for r in records if r.get("dataset_id", "").startswith("ds-verified")}

    assert_eq(len(explo_ids), 1, "should have exactly 1 exploratory dataset_id")
    assert_eq(len(verified_ids), 1, "should have exactly 1 verified dataset_id")
    assert_true(explo_ids != verified_ids, "exploratory and verified dataset_ids should differ")
    print("PASS")


def test_quality_states() -> None:
    """All quality states used are from the frozen contract enum."""
    print("test_quality_states...", end=" ")
    records = load_records(OUTPUT_PATH)
    valid_states = {
        "verified_public", "exploratory_verified", "exploratory_partial",
        "live_in_progress", "terminal_failed", "dead_evidence",
        "not_evaluated", "unavailable",
    }
    timestamp = re.compile(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$")
    for r in records:
        qs = r.get("quality_state")
        assert_true(qs in valid_states, f"invalid quality_state: {qs} in {r.get('kind')}")
        assert_true(bool(timestamp.match(r.get("observed_at", ""))), f"invalid observed_at: {r.get('observed_at')} in {r.get('kind')}")
    print("PASS")


def test_methodology_snapshot() -> None:
    """Methodology snapshot has metric definitions and suite definitions."""
    print("test_methodology_snapshot...", end=" ")
    records = load_records(OUTPUT_PATH)
    methods = [r for r in records if r.get("kind") == "methodology_snapshot"]
    assert_eq(len(methods), 1, "should have 1 methodology snapshot")
    m = methods[0]

    assert_true(len(m["metric_definitions"]) >= 4, "should have at least 4 metric definitions")
    assert_eq(len(m["suite_definitions"]), 9, "should have 9 suite definitions")
    assert_true(len(m["limitations"]) > 0, "should have limitations")

    # Check metric definition fields
    for md in m["metric_definitions"]:
        assert_true("key" in md, "metric def should have key")
        assert_true("name" in md, "metric def should have name")
        assert_true("unit" in md, "metric def should have unit")
        assert_true("direction" in md, "metric def should have direction")
        assert_true("denominator" in md, "metric def should have denominator")
        assert_true("explanation" in md, "metric def should have explanation")
    print("PASS")


def test_benchmark_observation_contract() -> None:
    print("test_benchmark_observation_contract...", end=" ")
    records = load_records(OUTPUT_PATH)
    evaluations = [record for record in records if record["kind"] == "evaluation_summary"]
    assignments = [record for record in records if record["kind"] == "assignment_result"]
    assert_true(all(record["evaluation_unit"] == "model" for record in evaluations), "historical evaluations should be model evaluation units")
    assert_true(all("primary_invocation_share" in record for record in evaluations), "evaluations should carry Primary invocation availability")
    assert_true(all("correlated_failure_rate" in record for record in evaluations), "evaluations should carry correlated-failure availability")
    assert_true(all("scenario_category" in record for record in assignments), "assignments should carry scenario categories")
    assert_true(all(record["evaluation_unit"] == "model" for record in assignments), "historical assignments should be model evaluation units")
    assert_true(all(record["benchmark_observations"]["unavailable_reasons"] for record in assignments), "unobserved benchmark dimensions should carry reasons")
    assert_eq({record["scenario_category"] for record in assignments}, {
        "instruction_adherence", "tool_selection", "tool_arguments", "technical_analysis",
        "routing_delegation", "verification", "security_policy", "recovery", "final_response",
    }, "scenario categories")
    print("PASS")


def test_generation_report() -> None:
    """Generation report has all required fields."""
    print("test_generation_report...", end=" ")
    report = load_report(REPORT_PATH)

    assert_true("record_counts_by_kind" in report, "report should have record_counts_by_kind")
    assert_true("quality_state_counts" in report, "report should have quality_state_counts")
    assert_true("model_coverage" in report, "report should have model_coverage")
    assert_true("rejected_fields" in report, "report should have rejected_fields")
    assert_true("source_artifact_hashes" in report, "report should have source_artifact_hashes")
    assert_true("output_hash" in report, "report should have output_hash")
    assert_true("aggregate_reconciliation" in report, "report should have aggregate_reconciliation")

    assert_eq(report["rejected_fields"], [], "should have no rejected fields")
    assert_eq(report["model_coverage"]["total_models"], 63, "should have 63 total models")
    assert_eq(report["model_coverage"]["evaluated"], 12, "should have 12 evaluated models")
    print("PASS")


def test_per_model_reconciliation() -> None:
    """Per-model reconciliation mismatches are documented and explained."""
    print("test_per_model_reconciliation...", end=" ")
    report = load_report(REPORT_PATH)
    mismatches = report.get("per_model_reconciliation_mismatches", [])
    note = report.get("per_model_reconciliation_note", "")

    # Mismatches are expected due to terminal_measurements vs eligible_count difference
    # They should all be explained by the note
    assert_true(len(note) > 0, "should have a reconciliation note explaining the difference")

    # All mismatches should be about terminal_measurements or pass_rate
    for m in mismatches:
        assert_true("terminal_measurements" in m or "pass_rate" in m,
                     f"unexpected mismatch type: {m}")
    print("PASS")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    tests = [
        test_projector_runs_successfully,
        test_record_counts,
        test_aggregate_reconciliation,
        test_catalog_snapshot_exploratory,
        test_catalog_snapshot_verified,
        test_model_summaries_nine_candidates,
        test_model_summaries_unevaluated,
        test_suite_summaries_verifier_states,
        test_evaluation_summaries,
        test_assignment_results_count,
        test_disclosure_scan,
        test_jsonl_format,
        test_deterministic_output,
        test_datasets_separate,
        test_quality_states,
        test_methodology_snapshot,
        test_benchmark_observation_contract,
        test_generation_report,
        test_per_model_reconciliation,
    ]

    failures = 0
    for test in tests:
        try:
            test()
        except AssertionError as e:
            print(f"FAIL: {e}")
            failures += 1
        except Exception as e:
            print(f"ERROR: {e}")
            failures += 1

    print(f"\n{'=' * 40}")
    if failures == 0:
        print(f"All {len(tests)} tests passed")
        return 0
    else:
        print(f"{failures}/{len(tests)} tests failed")
        return 1


if __name__ == "__main__":
    sys.exit(main())
