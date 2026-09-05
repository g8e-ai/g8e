# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 2 integration tests for fail-closed synthetic report round trips.

These tests exercise the full ``_run_synthetic_suite`` path end-to-end
against real local simulators and a real report directory on disk.  They
verify that:

  - ``evidence-index.jsonl`` is written with correct content-addressed
    artifacts (every index entry's SHA-256 and byte length match the
    persisted file bytes).
  - ``attempt.receipt_refs`` are populated for every attempt that
    produces a receipt.
  - ``source_evidence_sha256`` on every verified observation matches the
    SHA-256 of the persisted artifact referenced by
    ``source_evidence_refs``.
  - Tampered or missing evidence artifacts are detected by digest
    checking, proving the pipeline is fail-closed.
  - An empty metric set raises ``EvaluationRunError`` (invalid evidence)
    rather than passing silently.
  - A task with no applicable grader raises ``EvaluationRunError``.

The tests use the real immutable gold-set fixtures and real local
simulators (``LocalGovernanceSimulator``, ``LocalExfiltrationSimulator``,
``LocalEncryptedTokenStore``) but no LLM provider, g8ee, operator, or
network.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pytest

from g8e_evals import cli
from g8e_evals.schema import (
    AttemptRecord,
    EvidenceIndex,
    MetricObservation,
    ReceiptObservation,
    VerificationStatus,
)

pytestmark = pytest.mark.integration

_GOLD_SETS_DIR = Path(__file__).resolve().parent.parent / "gold_sets"


def _read_jsonl(path: Path, model_cls=None) -> list:
    records: list = []
    if not path.is_file():
        return records
    with open(path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            if model_cls is not None:
                records.append(model_cls.model_validate_json(line))
            else:
                records.append(json.loads(line))
    return records


def _find_report_dir(output_dir: Path) -> Path:
    report_dirs = sorted(p for p in output_dir.iterdir() if p.is_dir())
    assert len(report_dirs) >= 1, f"expected at least one report dir, got {report_dirs}"
    return report_dirs[-1]


async def _run_suite(suite: str, output_dir: Path) -> Path:
    gold_set = _GOLD_SETS_DIR / suite / "input_data.jsonl"
    assert gold_set.is_file(), f"gold set not found: {gold_set}"
    await cli._run_synthetic_suite(suite, gold_set, output_dir, limit=None)
    return _find_report_dir(output_dir)


class TestSyntheticReportRoundTrip:
    """End-to-end synthetic report round trip tests."""

    @pytest.mark.asyncio
    async def test_governance_adversarial_report_has_all_expected_files(self, tmp_path: Path) -> None:
        """The governance_adversarial suite produces a complete report directory."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)

        assert (report_dir / "manifest.json").is_file()
        assert (report_dir / "tasks.jsonl").is_file()
        assert (report_dir / "attempts.jsonl").is_file()
        assert (report_dir / "metrics.jsonl").is_file()
        assert (report_dir / "evidence-index.jsonl").is_file()
        assert (report_dir / "governance-receipts.jsonl").is_file()

    @pytest.mark.asyncio
    async def test_privacy_boundary_leakage_report_has_all_expected_files(self, tmp_path: Path) -> None:
        """The privacy_boundary_leakage suite produces a complete report directory."""
        report_dir = await _run_suite("privacy_boundary_leakage", tmp_path)

        assert (report_dir / "manifest.json").is_file()
        assert (report_dir / "tasks.jsonl").is_file()
        assert (report_dir / "attempts.jsonl").is_file()
        assert (report_dir / "metrics.jsonl").is_file()
        assert (report_dir / "evidence-index.jsonl").is_file()

    @pytest.mark.asyncio
    async def test_privacy_token_lifecycle_report_has_all_expected_files(self, tmp_path: Path) -> None:
        """The privacy_token_lifecycle suite produces a complete report directory."""
        report_dir = await _run_suite("privacy_token_lifecycle", tmp_path)

        assert (report_dir / "manifest.json").is_file()
        assert (report_dir / "tasks.jsonl").is_file()
        assert (report_dir / "attempts.jsonl").is_file()
        assert (report_dir / "metrics.jsonl").is_file()
        assert (report_dir / "evidence-index.jsonl").is_file()

    @pytest.mark.asyncio
    async def test_all_metrics_pass_for_governance_adversarial(self, tmp_path: Path) -> None:
        """Every metric in the governance_adversarial report has value 1.0 and verified status."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        metrics = _read_jsonl(report_dir / "metrics.jsonl", MetricObservation)
        assert len(metrics) > 0
        for metric in metrics:
            assert metric.value == 1.0, f"metric {metric.metric_id} for task {metric.task_id} has value {metric.value}"
            assert metric.verification_status == VerificationStatus.VERIFIED
            assert metric.eligible is True

    @pytest.mark.asyncio
    async def test_all_metrics_pass_for_privacy_boundary_leakage(self, tmp_path: Path) -> None:
        """Every metric in the privacy_boundary_leakage report has value 1.0 and verified status."""
        report_dir = await _run_suite("privacy_boundary_leakage", tmp_path)
        metrics = _read_jsonl(report_dir / "metrics.jsonl", MetricObservation)
        assert len(metrics) > 0
        for metric in metrics:
            assert metric.value == 1.0, f"metric {metric.metric_id} for task {metric.task_id} has value {metric.value}"
            assert metric.verification_status == VerificationStatus.VERIFIED
            assert metric.eligible is True


class TestEvidenceIndexIntegrity:
    """Verify evidence-index.jsonl entries match persisted artifact bytes."""

    @pytest.mark.asyncio
    async def test_evidence_index_digests_match_persisted_artifacts_governance(self, tmp_path: Path) -> None:
        """Every evidence-index entry for governance_adversarial has a SHA-256 and byte length
        that match the actual persisted artifact file."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        assert len(indices) > 0

        for index in indices:
            artifact_path = report_dir / index.storage_location
            assert artifact_path.is_file(), (
                f"evidence artifact missing at {index.storage_location} for artifact_id={index.artifact_id}"
            )
            content_bytes = artifact_path.read_bytes()
            actual_digest = hashlib.sha256(content_bytes).hexdigest()
            assert actual_digest == index.sha256, (
                f"evidence index sha256 mismatch for artifact_id={index.artifact_id}: "
                f"index={index.sha256}, actual={actual_digest}"
            )
            assert len(content_bytes) == index.byte_length, (
                f"evidence index byte_length mismatch for artifact_id={index.artifact_id}: "
                f"index={index.byte_length}, actual={len(content_bytes)}"
            )

    @pytest.mark.asyncio
    async def test_evidence_index_digests_match_persisted_artifacts_privacy(self, tmp_path: Path) -> None:
        """Every evidence-index entry for privacy_boundary_leakage has a SHA-256 and byte length
        that match the actual persisted artifact file."""
        report_dir = await _run_suite("privacy_boundary_leakage", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        assert len(indices) > 0

        for index in indices:
            artifact_path = report_dir / index.storage_location
            assert artifact_path.is_file(), (
                f"evidence artifact missing at {index.storage_location} for artifact_id={index.artifact_id}"
            )
            content_bytes = artifact_path.read_bytes()
            actual_digest = hashlib.sha256(content_bytes).hexdigest()
            assert actual_digest == index.sha256, (
                f"evidence index sha256 mismatch for artifact_id={index.artifact_id}: "
                f"index={index.sha256}, actual={actual_digest}"
            )
            assert len(content_bytes) == index.byte_length, (
                f"evidence index byte_length mismatch for artifact_id={index.artifact_id}: "
                f"index={index.byte_length}, actual={len(content_bytes)}"
            )

    @pytest.mark.asyncio
    async def test_evidence_index_storage_locations_are_content_addressed(self, tmp_path: Path) -> None:
        """Every evidence artifact is stored at evidence/<sha256>.json and the filename
        matches the declared SHA-256."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)

        for index in indices:
            expected_location = f"evidence/{index.sha256}.json"
            assert index.storage_location == expected_location, (
                f"storage_location={index.storage_location} does not match expected={expected_location} "
                f"for artifact_id={index.artifact_id}"
            )


class TestAttemptReceiptRefs:
    """Verify attempt receipt_refs are populated and consistent."""

    @pytest.mark.asyncio
    async def test_governance_attempts_have_receipt_refs(self, tmp_path: Path) -> None:
        """Every attempt in the governance_adversarial report has non-empty receipt_refs
        matching the receipt IDs in governance-receipts.jsonl."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        attempts = _read_jsonl(report_dir / "attempts.jsonl", AttemptRecord)
        receipts = _read_jsonl(report_dir / "governance-receipts.jsonl", ReceiptObservation)

        assert len(attempts) > 0
        assert len(receipts) > 0

        receipt_ids = {r.receipt_id for r in receipts}
        for attempt in attempts:
            assert len(attempt.receipt_refs) > 0, (
                f"attempt {attempt.attempt_id} has empty receipt_refs"
            )
            for ref in attempt.receipt_refs:
                assert ref in receipt_ids, (
                    f"attempt {attempt.attempt_id} references receipt {ref} not in governance-receipts.jsonl"
                )

    @pytest.mark.asyncio
    async def test_privacy_attempts_have_receipt_refs_for_exfiltration(self, tmp_path: Path) -> None:
        """Attempts with exfiltration assertions have receipt_refs matching privacy-receipts.jsonl."""
        report_dir = await _run_suite("privacy_boundary_leakage", tmp_path)
        attempts = _read_jsonl(report_dir / "attempts.jsonl", AttemptRecord)
        privacy_receipts = _read_jsonl(report_dir / "privacy-receipts.jsonl", ReceiptObservation)

        if not privacy_receipts:
            pytest.skip("no privacy receipts in this suite run")

        receipt_ids = {r.receipt_id for r in privacy_receipts}
        attempts_with_receipts = [a for a in attempts if a.receipt_refs]
        assert len(attempts_with_receipts) > 0, "no attempts have receipt_refs"
        for attempt in attempts_with_receipts:
            for ref in attempt.receipt_refs:
                assert ref in receipt_ids, (
                    f"attempt {attempt.attempt_id} references receipt {ref} not in privacy-receipts.jsonl"
                )


class TestSourceEvidenceDigestConsistency:
    """Verify source_evidence_sha256 on observations matches persisted artifact bytes."""

    @pytest.mark.asyncio
    async def test_governance_observation_source_evidence_matches_persisted_bytes(self, tmp_path: Path) -> None:
        """Every verified governance observation's source_evidence_sha256 matches the SHA-256
        of the persisted artifact referenced by source_evidence_refs."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        index_by_artifact_id = {idx.artifact_id: idx for idx in indices}

        observation_files = [
            "replay-attempt-observations.jsonl",
            "signed-field-tampering-observations.jsonl",
            "nonce-expiration-observations.jsonl",
            "stale-state-root-observations.jsonl",
            "signer-defect-observations.jsonl",
            "l3-proof-transplant-observations.jsonl",
            "revoked-credential-observations.jsonl",
            "policy-attack-observations.jsonl",
        ]

        checked = 0
        for obs_file in observation_files:
            obs_path = report_dir / obs_file
            if not obs_path.is_file():
                continue
            for line in obs_path.read_text().splitlines():
                line = line.strip()
                if not line:
                    continue
                obs = json.loads(line)
                if obs.get("verification_status") != "verified":
                    continue
                refs = obs.get("source_evidence_refs", [])
                declared_sha = obs.get("source_evidence_sha256")
                assert refs, f"verified observation {obs.get('observation_id')} has no source_evidence_refs"
                assert declared_sha is not None, (
                    f"verified observation {obs.get('observation_id')} has no source_evidence_sha256"
                )
                for ref in refs:
                    idx = index_by_artifact_id.get(ref)
                    assert idx is not None, (
                        f"observation {obs.get('observation_id')} references artifact {ref} "
                        f"not in evidence-index.jsonl"
                    )
                    assert idx.sha256 == declared_sha, (
                        f"observation {obs.get('observation_id')} source_evidence_sha256={declared_sha} "
                        f"does not match index sha256={idx.sha256} for artifact {ref}"
                    )
                    artifact_path = report_dir / idx.storage_location
                    assert artifact_path.is_file(), f"artifact file missing at {idx.storage_location}"
                    actual_sha = hashlib.sha256(artifact_path.read_bytes()).hexdigest()
                    assert actual_sha == declared_sha, (
                        f"observation {obs.get('observation_id')} source_evidence_sha256={declared_sha} "
                        f"does not match actual file sha256={actual_sha} for artifact {ref}"
                    )
                    checked += 1

        assert checked > 0, "no verified governance observations were checked"

    @pytest.mark.asyncio
    async def test_privacy_observation_source_evidence_matches_persisted_bytes(self, tmp_path: Path) -> None:
        """Every verified privacy observation's source_evidence_sha256 matches the SHA-256
        of the persisted artifact referenced by source_evidence_refs."""
        report_dir = await _run_suite("privacy_boundary_leakage", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        index_by_artifact_id = {idx.artifact_id: idx for idx in indices}

        observation_files = [
            "exfiltration-attempt-observations.jsonl",
            "artifact-leakage-observations.jsonl",
            "rehydration-observations.jsonl",
        ]

        checked = 0
        for obs_file in observation_files:
            obs_path = report_dir / obs_file
            if not obs_path.is_file():
                continue
            for line in obs_path.read_text().splitlines():
                line = line.strip()
                if not line:
                    continue
                obs = json.loads(line)
                if obs.get("verification_status") != "verified":
                    continue
                refs = obs.get("source_evidence_refs", [])
                declared_sha = obs.get("source_evidence_sha256")
                assert refs, f"verified observation {obs.get('observation_id')} has no source_evidence_refs"
                assert declared_sha is not None, (
                    f"verified observation {obs.get('observation_id')} has no source_evidence_sha256"
                )
                for ref in refs:
                    idx = index_by_artifact_id.get(ref)
                    assert idx is not None, (
                        f"observation {obs.get('observation_id')} references artifact {ref} "
                        f"not in evidence-index.jsonl"
                    )
                    assert idx.sha256 == declared_sha, (
                        f"observation {obs.get('observation_id')} source_evidence_sha256={declared_sha} "
                        f"does not match index sha256={idx.sha256} for artifact {ref}"
                    )
                    artifact_path = report_dir / idx.storage_location
                    assert artifact_path.is_file(), f"artifact file missing at {idx.storage_location}"
                    actual_sha = hashlib.sha256(artifact_path.read_bytes()).hexdigest()
                    assert actual_sha == declared_sha, (
                        f"observation {obs.get('observation_id')} source_evidence_sha256={declared_sha} "
                        f"does not match actual file sha256={actual_sha} for artifact {ref}"
                    )
                    checked += 1

        assert checked > 0, "no verified privacy observations were checked"


class TestFailClosedTampering:
    """Verify that tampered or missing evidence is detected by digest checking."""

    @pytest.mark.asyncio
    async def test_tampered_evidence_artifact_detected_by_digest_check(self, tmp_path: Path) -> None:
        """Tampering with a persisted evidence artifact file causes digest verification to fail."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        assert len(indices) > 0

        first_index = indices[0]
        artifact_path = report_dir / first_index.storage_location
        assert artifact_path.is_file()

        original_bytes = artifact_path.read_bytes()
        tampered_bytes = original_bytes + b"\nTAMPERED"
        artifact_path.write_bytes(tampered_bytes)

        actual_digest = hashlib.sha256(artifact_path.read_bytes()).hexdigest()
        assert actual_digest != first_index.sha256, (
            "tampered artifact digest should differ from index sha256"
        )

        evidence_index_map = {first_index.artifact_id: first_index}
        result = cli._resolve_and_digest_check(
            evidence_index_map,
            [first_index.artifact_id],
            first_index.sha256,
            report_dir,
        )
        assert result is False, "digest check should fail for tampered artifact"

    @pytest.mark.asyncio
    async def test_missing_evidence_artifact_detected_by_digest_check(self, tmp_path: Path) -> None:
        """Deleting a persisted evidence artifact file causes digest verification to fail."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        assert len(indices) > 0

        first_index = indices[0]
        artifact_path = report_dir / first_index.storage_location
        assert artifact_path.is_file()
        artifact_path.unlink()
        assert not artifact_path.is_file()

        evidence_index_map = {first_index.artifact_id: first_index}
        result = cli._resolve_and_digest_check(
            evidence_index_map,
            [first_index.artifact_id],
            first_index.sha256,
            report_dir,
        )
        assert result is False, "digest check should fail for missing artifact"

    @pytest.mark.asyncio
    async def test_valid_evidence_artifact_passes_digest_check(self, tmp_path: Path) -> None:
        """A valid, untampered evidence artifact passes digest verification."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        assert len(indices) > 0

        first_index = indices[0]
        artifact_path = report_dir / first_index.storage_location
        assert artifact_path.is_file()

        evidence_index_map = {first_index.artifact_id: first_index}
        result = cli._resolve_and_digest_check(
            evidence_index_map,
            [first_index.artifact_id],
            first_index.sha256,
            report_dir,
        )
        assert result is True, "digest check should pass for valid artifact"

    @pytest.mark.asyncio
    async def test_digest_mismatch_between_index_and_observation_detected(self, tmp_path: Path) -> None:
        """When the evidence index sha256 does not match the observation's declared sha256,
        digest verification fails."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        assert len(indices) > 0

        first_index = indices[0]
        wrong_sha = "0" * 64
        evidence_index_map = {first_index.artifact_id: first_index}
        result = cli._resolve_and_digest_check(
            evidence_index_map,
            [first_index.artifact_id],
            wrong_sha,
            report_dir,
        )
        assert result is False, "digest check should fail when expected_sha256 does not match index"

    @pytest.mark.asyncio
    async def test_unresolved_evidence_reference_detected(self, tmp_path: Path) -> None:
        """An evidence reference that does not resolve to any index entry fails digest verification."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        assert len(indices) > 0

        first_index = indices[0]
        evidence_index_map = {first_index.artifact_id: first_index}
        result = cli._resolve_and_digest_check(
            evidence_index_map,
            ["nonexistent-artifact-id"],
            first_index.sha256,
            report_dir,
        )
        assert result is False, "digest check should fail for unresolved reference"

    @pytest.mark.asyncio
    async def test_empty_refs_fail_digest_check(self, tmp_path: Path) -> None:
        """Empty source_evidence_refs fails digest verification (no evidence is not valid evidence)."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        indices = _read_jsonl(report_dir / "evidence-index.jsonl", EvidenceIndex)
        assert len(indices) > 0

        first_index = indices[0]
        evidence_index_map = {first_index.artifact_id: first_index}
        result = cli._resolve_and_digest_check(
            evidence_index_map,
            [],
            first_index.sha256,
            report_dir,
        )
        assert result is False, "digest check should fail for empty refs"


class TestFailClosedEdgeCases:
    """Verify fail-closed behavior for invalid-evidence edge cases in _run_synthetic_suite."""

    @pytest.mark.asyncio
    async def test_empty_metric_set_raises_invalid_evidence(self, tmp_path: Path, monkeypatch) -> None:
        """A task that produces no graded metrics raises EvaluationRunError rather than passing."""
        dataset = tmp_path / "input_data.jsonl"
        provenance = tmp_path / "provenance.json"

        dataset.write_text(
            json.dumps({
                "key": "no-grader-task-001",
                "description": "A task with no typed assertions and therefore no applicable grader.",
                "category": "utility",
                "expected_action_class": "NO_OP",
                "scenario_params": {"graders": []},
            }) + "\n"
        )

        provenance.write_text(json.dumps({
            "schema_version": 1,
            "benchmark": "tool_sequence",
            "source": {
                "repository": "https://example.com/repo",
                "revision": "test",
                "license_spdx": "Apache-2.0",
                "code_path": "g8e_evals/benchmarks/utility/loader.py",
                "code_sha256": "0" * 64,
            },
            "output": {
                "path": "input_data.jsonl",
                "rows": 1,
                "sha256": hashlib.sha256(dataset.read_bytes()).hexdigest(),
            },
            "partition": "development",
            "domain_strata": ["utility"],
        }))

        monkeypatch.setattr(
            "g8e_evals.benchmarks.utility.loader.validate_provenance",
            lambda _provenance, **_kwargs: None,
        )

        with pytest.raises(cli.EvaluationRunError, match="no applicable grader"):
            await cli._run_synthetic_suite("tool_sequence", dataset, tmp_path, limit=None)

    @pytest.mark.asyncio
    async def test_receipt_signature_verification_in_report(self, tmp_path: Path) -> None:
        """Every receipt in the governance report passes signature verification with the
        simulator's public key."""
        report_dir = await _run_suite("governance_adversarial", tmp_path)
        receipts = _read_jsonl(report_dir / "governance-receipts.jsonl", ReceiptObservation)
        assert len(receipts) > 0

        for receipt_obs in receipts:
            assert receipt_obs.verified is True, (
                f"receipt {receipt_obs.receipt_id} is not verified"
            )
            assert receipt_obs.primary is True, (
                f"receipt {receipt_obs.receipt_id} is not primary"
            )

    @pytest.mark.asyncio
    async def test_receipt_signature_verification_in_privacy_report(self, tmp_path: Path) -> None:
        """Every receipt in the privacy report passes signature verification."""
        report_dir = await _run_suite("privacy_boundary_leakage", tmp_path)
        receipts = _read_jsonl(report_dir / "privacy-receipts.jsonl", ReceiptObservation)

        if not receipts:
            pytest.skip("no privacy receipts in this suite run")

        for receipt_obs in receipts:
            assert receipt_obs.verified is True, (
                f"receipt {receipt_obs.receipt_id} is not verified"
            )
