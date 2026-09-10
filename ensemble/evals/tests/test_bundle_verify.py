# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for complete fail-closed offline eval bundle verification (P2-03).

Verifies the typed deterministic verification report, centralized stable
failure codes, ordered verification layers, and cancellation behavior. The
verifier requires no originating service, runtime directory, network
access, or in-bundle trust policy.
"""

from __future__ import annotations

import hashlib
import json
import threading
from datetime import UTC, datetime
from pathlib import Path

import pytest
from pydantic import ValidationError

pytestmark = pytest.mark.unit

from g8e_evals import constants as evals_constants
from g8e_evals.analysis import canonical_model_json, compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.bundle import (
    EvalSigningKey,
    EvalTrustStore,
    EvalTrustedKey,
    produce_bundle,
    verify_bundle,
)
from g8e_evals.bundle.signing import (
    EVAL_SIGNING_ALGORITHM,
    EVAL_TRUST_SCOPE,
)
from g8e_evals.bundle.verify import (
    VerificationCancelled,
    VerificationFailure,
    VerificationFailureCode,
    VerificationLayer,
    VerificationReport,
)
from g8e_evals.schema import (
    AttemptRecord,
    MetricObservation,
    ReceiptObservation,
    StageKind,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)
from g8e.operator.v1.operator_pb2 import (
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)
from g8e_evals.arms import Arm
from g8e_evals.schema import ActionReceipt


_TS = datetime(2026, 1, 1, tzinfo=UTC)
_TS_PLUS = datetime(2026, 12, 31, tzinfo=UTC)


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


# ---------------------------------------------------------------------------
# Minimal report directory builder
# ---------------------------------------------------------------------------


def _minimal_task() -> TaskDefinition:
    return TaskDefinition(
        task_id="task-1",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        prompt_hash="0" * 64,
    )


def _minimal_attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
        started_at=_TS,
        ended_at=_TS,
    )


def _minimal_metric() -> MetricObservation:
    return MetricObservation(
        metric_id="receipt_integrity",
        metric_version="1.0.0",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        arm_id=Arm.DOCTRINE,
        value=1.0,
        unit="boolean",
        verification_status=VerificationStatus.VERIFIED,
    )


def _minimal_receipt() -> ReceiptObservation:
    receipt = ActionReceipt(
        transaction_id="tx-1",
        transaction_hash="hash-1",
    )
    receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="TEST_ACTION",
    )
    return ReceiptObservation(
        receipt_id="receipt-1",
        attempt_id="attempt-1",
        run_id="run-1",
        transaction_id="tx-1",
        action_type="TEST_ACTION",
        primary=True,
        action_receipt=receipt,
    )


def _minimal_stage() -> StageObservation:
    return StageObservation(
        stage_id="stage-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        kind=StageKind.PROTOCOL_L2,
        decision="pass",
    )


def _build_report_dir(report_dir: Path) -> None:
    """Build a minimal valid report directory with analysis artifacts."""
    report_dir.mkdir(parents=True, exist_ok=True)

    task = _minimal_task()
    attempt = _minimal_attempt()
    metric = _minimal_metric()
    receipt = _minimal_receipt()
    stage = _minimal_stage()

    # Write source records.
    (report_dir / evals_constants.MANIFEST_JSON).write_text(json.dumps({
        "run_id": "run-1",
        "suite_id": "ifeval_subset",
        "started_at": _TS.isoformat(),
        "ended_at": _TS.isoformat(),
    }, sort_keys=True))
    (report_dir / evals_constants.TASKS_JSONL).write_text(canonical_model_json(task) + "\n")
    (report_dir / evals_constants.ATTEMPTS_JSONL).write_text(canonical_model_json(attempt) + "\n")
    (report_dir / evals_constants.METRICS_JSONL).write_text(canonical_model_json(metric) + "\n")
    (report_dir / evals_constants.RECEIPTS_JSONL).write_text(canonical_model_json(receipt) + "\n")
    (report_dir / evals_constants.STAGES_JSONL).write_text(canonical_model_json(stage) + "\n")

    # Build and write analysis artifacts.
    analysis_input = AnalysisInputRecord(
        run_id="run-1",
        release_version="v2.1.8",
        tasks=[task],
        attempts=[attempt],
        metric_observations=[metric],
        receipts=[receipt],
        stages=[stage],
    )
    (report_dir / evals_constants.ANALYSIS_INPUT_JSON).write_text(
        canonical_model_json(analysis_input)
    )

    from g8e_evals.analysis import render_cli, render_html, render_markdown
    analysis = compute_canonical_analysis_from_record(analysis_input)
    (report_dir / evals_constants.ANALYSIS_JSON).write_text(canonical_model_json(analysis))
    (report_dir / evals_constants.ANALYSIS_MD).write_text(render_markdown(analysis))
    (report_dir / evals_constants.ANALYSIS_HTML).write_text(render_html(analysis))
    (report_dir / evals_constants.ANALYSIS_TXT).write_text(render_cli(analysis))


def _produce_valid_bundle(tmp_path: Path, signing_key: EvalSigningKey | None = None) -> Path:
    """Produce a valid bundle from a minimal report directory."""
    report_dir = tmp_path / "report"
    bundle_dir = tmp_path / "bundle"
    _build_report_dir(report_dir)
    produce_bundle(
        report_dir=report_dir,
        bundle_dir=bundle_dir,
        bundle_id="bundle-1",
        run_id="run-1",
        release_version="v2.1.8",
        signing_key=signing_key,
        created_at=_TS,
    )
    return bundle_dir


def _trusted_key(signing_key: EvalSigningKey) -> EvalTrustedKey:
    return EvalTrustedKey(
        key_id=signing_key.key_id,
        public_key_hex=signing_key.public_key_hex,
        algorithm=EVAL_SIGNING_ALGORITHM,
        scope=EVAL_TRUST_SCOPE,
        valid_from=_TS,
        valid_until=_TS_PLUS,
        revoked=False,
        source="protocol-owned-test-metadata",
    )


def _trust_store(signing_key: EvalSigningKey) -> EvalTrustStore:
    return EvalTrustStore(keys=[_trusted_key(signing_key)])


# ---------------------------------------------------------------------------
# VerificationReport model tests
# ---------------------------------------------------------------------------


class TestVerificationReportModel:
    """The verification report is a frozen, extra-forbid typed model."""

    def test_frozen_model_rejects_mutation(self) -> None:
        report = VerificationReport(
            verified_at=_TS,
            ok=True,
            layers=[],
            failures=[],
        )
        with pytest.raises(ValidationError):
            report.ok = False  # type: ignore[misc]

    def test_extra_field_rejected(self) -> None:
        with pytest.raises(ValidationError):
            VerificationReport(
                verified_at=_TS,
                ok=True,
                layers=[],
                failures=[],
                extra_field="bad",  # type: ignore[call-arg]
            )

    def test_failure_is_frozen(self) -> None:
        failure = VerificationFailure(
            layer=VerificationLayer.ROOTED_INVENTORY,
            code=VerificationFailureCode.ORPHAN_FILE,
            record_id="foo.txt",
            message="orphan",
        )
        with pytest.raises(ValidationError):
            failure.message = "other"  # type: ignore[misc]


# ---------------------------------------------------------------------------
# Layer 1: Rooted inventory and limits
# ---------------------------------------------------------------------------


class TestRootedInventory:
    """Layer 1: rooted inventory, orphan detection, missing files, limits."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer1 = next(lr for lr in report.layers if lr.layer == VerificationLayer.ROOTED_INVENTORY)
        assert layer1.passed, f"Layer 1 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.ROOTED_INVENTORY]}"

    def test_orphan_file_detected(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        (bundle_dir / "orphan.txt").write_text("not in manifest")
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.ROOTED_INVENTORY]
        assert VerificationFailureCode.ORPHAN_FILE in codes

    def test_missing_file_detected(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        # Remove a data file (not a metadata file).
        (bundle_dir / evals_constants.TASKS_JSONL).unlink()
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.ROOTED_INVENTORY]
        assert VerificationFailureCode.MISSING_FILE in codes

    def test_symlink_rejected(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        # Create a symlink to a real file.
        target = bundle_dir / evals_constants.TASKS_JSONL
        (bundle_dir / "evil.link").symlink_to(target)
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.ROOTED_INVENTORY]
        assert VerificationFailureCode.SYMLINK_REJECTED in codes


# ---------------------------------------------------------------------------
# Layer 2: Schemas and canonical bytes
# ---------------------------------------------------------------------------


class TestSchemasCanonical:
    """Layer 2: manifest/checksum schema validation and self-hash verification."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer2 = next(lr for lr in report.layers if lr.layer == VerificationLayer.SCHEMAS_CANONICAL)
        assert layer2.passed, f"Layer 2 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.SCHEMAS_CANONICAL]}"

    def test_manifest_not_found(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        (bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON).unlink()
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SCHEMAS_CANONICAL]
        assert VerificationFailureCode.MANIFEST_SCHEMA_INVALID in codes

    def test_manifest_invalid_json(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        (bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON).write_text("{invalid}")
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SCHEMAS_CANONICAL]
        assert VerificationFailureCode.MANIFEST_SCHEMA_INVALID in codes

    def test_manifest_self_hash_mismatch(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        # Corrupt the manifest self-hash.
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = json.loads(manifest_path.read_text())
        manifest["manifest_content_sha256"] = "a" * 64
        manifest_path.write_text(json.dumps(manifest, sort_keys=True))
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SCHEMAS_CANONICAL]
        assert VerificationFailureCode.MANIFEST_SELF_HASH_MISMATCH in codes

    def test_checksum_not_found(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        (bundle_dir / evals_constants.CHECKSUM_ROOT_JSON).unlink()
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SCHEMAS_CANONICAL]
        assert VerificationFailureCode.CHECKSUM_SCHEMA_INVALID in codes


# ---------------------------------------------------------------------------
# Layer 3: File hashes and references
# ---------------------------------------------------------------------------


class TestFileHashes:
    """Layer 3: file hash and byte length verification."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer3 = next(lr for lr in report.layers if lr.layer == VerificationLayer.FILE_HASHES)
        assert layer3.passed, f"Layer 3 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.FILE_HASHES]}"

    def test_file_hash_mismatch_detected(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        # Corrupt a data file's content (hash will mismatch).
        (bundle_dir / evals_constants.TASKS_JSONL).write_text("corrupted")
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.FILE_HASHES]
        assert VerificationFailureCode.FILE_HASH_MISMATCH in codes


# ---------------------------------------------------------------------------
# Layer 4: Signatures and trust
# ---------------------------------------------------------------------------


class TestSignaturesTrust:
    """Layer 4: manifest/checksum signatures and assessed trust."""

    def test_unsigned_bundle_without_trust_store_passes_other_layers(self, tmp_path: Path) -> None:
        """An unsigned bundle without a trust store: signature layer fails, others pass."""
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=None)
        report = verify_bundle(bundle_dir, trust_store=None)
        layer4 = next(lr for lr in report.layers if lr.layer == VerificationLayer.SIGNATURES_TRUST)
        assert not layer4.passed
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SIGNATURES_TRUST]
        assert VerificationFailureCode.SIGNATURE_MISSING in codes

    def test_signed_bundle_with_valid_trust_store_passes(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(b"k" * 32)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        layer4 = next(lr for lr in report.layers if lr.layer == VerificationLayer.SIGNATURES_TRUST)
        assert layer4.passed, f"Layer 4 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.SIGNATURES_TRUST]}"

    def test_signed_bundle_without_trust_store_fails(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(b"k" * 32)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        report = verify_bundle(bundle_dir, trust_store=None)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SIGNATURES_TRUST]
        assert VerificationFailureCode.TRUST_STORE_MISSING in codes

    def test_signed_bundle_with_empty_trust_store_fails(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(b"k" * 32)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        report = verify_bundle(bundle_dir, trust_store=EvalTrustStore(keys=[]))
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SIGNATURES_TRUST]
        assert VerificationFailureCode.SIGNATURE_UNKNOWN_KEY in codes

    def test_signed_bundle_with_revoked_key_fails(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(b"k" * 32)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        revoked_key = EvalTrustedKey(
            key_id=signing_key.key_id,
            public_key_hex=signing_key.public_key_hex,
            algorithm=EVAL_SIGNING_ALGORITHM,
            scope=EVAL_TRUST_SCOPE,
            valid_from=_TS,
            valid_until=_TS_PLUS,
            revoked=True,
            source="protocol-owned-test-metadata",
        )
        report = verify_bundle(bundle_dir, trust_store=EvalTrustStore(keys=[revoked_key]))
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SIGNATURES_TRUST]
        assert VerificationFailureCode.SIGNATURE_REVOKED in codes

    def test_signed_bundle_with_wrong_scope_fails(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(b"k" * 32)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        wrong_scope_key = EvalTrustedKey(
            key_id=signing_key.key_id,
            public_key_hex=signing_key.public_key_hex,
            algorithm=EVAL_SIGNING_ALGORITHM,
            scope="wrong-scope",
            valid_from=_TS,
            valid_until=_TS_PLUS,
            revoked=False,
            source="protocol-owned-test-metadata",
        )
        report = verify_bundle(bundle_dir, trust_store=EvalTrustStore(keys=[wrong_scope_key]))
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SIGNATURES_TRUST]
        assert VerificationFailureCode.SIGNATURE_WRONG_SCOPE in codes


# ---------------------------------------------------------------------------
# Layer 5: Record bindings
# ---------------------------------------------------------------------------


class TestRecordBindings:
    """Layer 5: run/task/attempt binding verification."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer5 = next(lr for lr in report.layers if lr.layer == VerificationLayer.RECORD_BINDINGS)
        assert layer5.passed, f"Layer 5 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.RECORD_BINDINGS]}"

    def test_run_id_mismatch_detected(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        # Corrupt the run manifest's run_id.
        run_manifest_path = bundle_dir / evals_constants.MANIFEST_JSON
        run_manifest = json.loads(run_manifest_path.read_text())
        run_manifest["run_id"] = "wrong-run"
        run_manifest_path.write_text(json.dumps(run_manifest, sort_keys=True))
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.RECORD_BINDINGS]
        assert VerificationFailureCode.RUN_ID_MISMATCH in codes


# ---------------------------------------------------------------------------
# Layer 6: Envelope/receipt correlation
# ---------------------------------------------------------------------------


class TestEnvelopeReceipt:
    """Layer 6: envelope/receipt correlation."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer6 = next(lr for lr in report.layers if lr.layer == VerificationLayer.ENVELOPE_RECEIPT)
        assert layer6.passed, f"Layer 6 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.ENVELOPE_RECEIPT]}"


# ---------------------------------------------------------------------------
# Layer 7: Chain links
# ---------------------------------------------------------------------------


class TestChainLinks:
    """Layer 7: stage chain, persistence, commitment, and audit links."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer7 = next(lr for lr in report.layers if lr.layer == VerificationLayer.CHAIN_LINKS)
        assert layer7.passed, f"Layer 7 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.CHAIN_LINKS]}"


# ---------------------------------------------------------------------------
# Layer 8: Metric producers
# ---------------------------------------------------------------------------


class TestMetricProducers:
    """Layer 8: metric evidence and producer validation."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer8 = next(lr for lr in report.layers if lr.layer == VerificationLayer.METRIC_PRODUCERS)
        assert layer8.passed, f"Layer 8 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.METRIC_PRODUCERS]}"


# ---------------------------------------------------------------------------
# Layer 9: Analysis reproduction
# ---------------------------------------------------------------------------


class TestAnalysisReproduction:
    """Layer 9: canonical analysis reproduction from analysis-input.json."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer9 = next(lr for lr in report.layers if lr.layer == VerificationLayer.ANALYSIS_REPRODUCTION)
        assert layer9.passed, f"Layer 9 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.ANALYSIS_REPRODUCTION]}"

    def test_analysis_input_missing(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        (bundle_dir / evals_constants.ANALYSIS_INPUT_JSON).unlink()
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.ANALYSIS_REPRODUCTION]
        assert VerificationFailureCode.ANALYSIS_INPUT_SCHEMA_INVALID in codes

    def test_analysis_json_mismatch(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        # Corrupt analysis.json so reproduction doesn't match.
        (bundle_dir / evals_constants.ANALYSIS_JSON).write_text('{"corrupted": true}')
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.ANALYSIS_REPRODUCTION]
        assert VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH in codes


# ---------------------------------------------------------------------------
# Layer 10: Renderer byte equality
# ---------------------------------------------------------------------------


class TestRendererEquality:
    """Layer 10: renderer byte equality."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer10 = next(lr for lr in report.layers if lr.layer == VerificationLayer.RENDERER_EQUALITY)
        assert layer10.passed, f"Layer 10 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.RENDERER_EQUALITY]}"

    def test_renderer_byte_mismatch(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        (bundle_dir / evals_constants.ANALYSIS_MD).write_text("corrupted markdown")
        report = verify_bundle(bundle_dir)
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.RENDERER_EQUALITY]
        assert VerificationFailureCode.RENDERER_BYTE_MISMATCH in codes


# ---------------------------------------------------------------------------
# Layer 11: Public/restricted separation
# ---------------------------------------------------------------------------


class TestPrivacySeparation:
    """Layer 11: public/restricted artifact separation."""

    def test_valid_bundle_passes(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        layer11 = next(lr for lr in report.layers if lr.layer == VerificationLayer.PRIVACY_SEPARATION)
        assert layer11.passed, f"Layer 11 should pass: {[f.message for f in report.failures if f.layer == VerificationLayer.PRIVACY_SEPARATION]}"


# ---------------------------------------------------------------------------
# Overall report and determinism
# ---------------------------------------------------------------------------


class TestVerificationReportOverall:
    """Overall report structure, determinism, and ok flag."""

    def test_valid_bundle_ok(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(b"k" * 32)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        assert report.ok, f"Valid bundle should be ok: {[f.message for f in report.failures]}"
        assert report.bundle_id == "bundle-1"
        assert report.run_id == "run-1"
        assert report.release_version == "v2.1.8"

    def test_invalid_bundle_not_ok(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        (bundle_dir / "orphan.txt").write_text("not in manifest")
        report = verify_bundle(bundle_dir)
        assert not report.ok

    def test_all_twelve_layers_present(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir)
        assert len(report.layers) == 12
        layer_values = [lr.layer for lr in report.layers]
        assert layer_values == list(VerificationLayer)

    def test_failures_sorted_by_layer(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        # Add orphan (layer 1) and corrupt analysis.json (layer 9).
        (bundle_dir / "orphan.txt").write_text("not in manifest")
        (bundle_dir / evals_constants.ANALYSIS_JSON).write_text('{"corrupted": true}')
        report = verify_bundle(bundle_dir)
        if len(report.failures) >= 2:
            for i in range(len(report.failures) - 1):
                assert int(report.failures[i].layer) <= int(report.failures[i + 1].layer)

    def test_report_is_deterministic(self, tmp_path: Path) -> None:
        """Two verifications of the same bundle produce identical failure sets (ignoring verified_at)."""
        signing_key = EvalSigningKey.from_seed(b"k" * 32)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        report1 = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        report2 = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        assert report1.ok == report2.ok
        assert report1.bundle_id == report2.bundle_id
        assert [f.code for f in report1.failures] == [f.code for f in report2.failures]
        assert [f.record_id for f in report1.failures] == [f.record_id for f in report2.failures]


# ---------------------------------------------------------------------------
# Cancellation tests
# ---------------------------------------------------------------------------


class TestCancellation:
    """A cancelled verification raises VerificationCancelled and does not emit a valid report."""

    def test_cancel_before_layer_1(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        cancel_event = threading.Event()
        cancel_event.set()
        with pytest.raises(VerificationCancelled):
            verify_bundle(bundle_dir, cancel_event=cancel_event)

    def test_cancel_between_layers(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        cancel_event = threading.Event()

        # Set the cancel event after a short delay so layer 1 completes.
        def _cancel_soon():
            cancel_event.set()

        timer = threading.Timer(0.001, _cancel_soon)
        timer.start()
        try:
            with pytest.raises(VerificationCancelled):
                verify_bundle(bundle_dir, cancel_event=cancel_event)
        finally:
            timer.cancel()
            timer.join()

    def test_no_cancel_event_produces_report(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        report = verify_bundle(bundle_dir, cancel_event=None)
        assert isinstance(report, VerificationReport)

    def test_unset_cancel_event_produces_report(self, tmp_path: Path) -> None:
        bundle_dir = _produce_valid_bundle(tmp_path)
        cancel_event = threading.Event()
        report = verify_bundle(bundle_dir, cancel_event=cancel_event)
        assert isinstance(report, VerificationReport)
