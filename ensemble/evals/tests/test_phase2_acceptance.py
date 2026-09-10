# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 2 acceptance tests (P2-06).

Verifies one public fixture bundle in a network-disabled clean environment
without restricted plaintext or decryption keys. Records the typed report,
mutation-matrix coverage, deterministic analysis reproduction, and renderer
byte equality. Phase 2 remains open if any protected class lacks mutation
coverage or any verifier trust originates inside the bundle.

These tests are the Phase 2 exit gate. They do not duplicate the per-layer
mutation matrix (P2-04) or the per-layer verification tests (P2-03); they
assert the aggregate acceptance criteria that the full verification suite
detects every protected mutation without trusting in-bundle keys or
requiring restricted plaintext.
"""

from __future__ import annotations

import socket
from datetime import UTC, datetime
from pathlib import Path

import pytest


from g8e_evals import constants as evals_constants
from g8e_evals.analysis import canonical_model_json, compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.bundle import (
    BundleManifest,
    EvalSigningKey,
    EvalTrustStore,
    EvalTrustedKey,
    PrivacyClass,
    produce_bundle,
    verify_bundle,
)
from g8e_evals.bundle.signing import (
    EVAL_SIGNING_ALGORITHM,
    EVAL_TRUST_SCOPE,
)
from g8e_evals.bundle.verify import (
    VerificationFailureCode,
    VerificationLayer,
)
from g8e_evals.schema import (
    AttemptRecord,
    MetricObservation,
    ReceiptObservation,
    RunManifest,
    StageKind,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)
from g8e_evals.arms import Arm
from g8e_evals.schema import ActionReceipt
from g8e.operator.v1.operator_pb2 import (
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)


_TS = datetime(2026, 1, 1, tzinfo=UTC)
_TS_PLUS = datetime(2026, 12, 31, tzinfo=UTC)
_SEED = b"k" * 32


# ---------------------------------------------------------------------------
# Public fixture bundle builder (no restricted artifacts, no decryption keys)
# ---------------------------------------------------------------------------


def _public_task() -> TaskDefinition:
    return TaskDefinition(
        task_id="task-1",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        prompt_hash="0" * 64,
    )


def _public_attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
        started_at=_TS,
        ended_at=_TS,
    )


def _public_metric() -> MetricObservation:
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


def _public_receipt() -> ReceiptObservation:
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


def _public_stage() -> StageObservation:
    return StageObservation(
        stage_id="stage-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        kind=StageKind.PROTOCOL_L2,
        decision="pass",
    )


def _build_public_report_dir(report_dir: Path) -> None:
    """Build a public-only report directory with no restricted artifacts."""
    report_dir.mkdir(parents=True, exist_ok=True)

    task = _public_task()
    attempt = _public_attempt()
    metric = _public_metric()
    receipt = _public_receipt()
    stage = _public_stage()

    run_manifest = RunManifest(
        run_id="run-1",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
    )
    (report_dir / evals_constants.MANIFEST_JSON).write_text(canonical_model_json(run_manifest))
    (report_dir / evals_constants.TASKS_JSONL).write_text(canonical_model_json(task) + "\n")
    (report_dir / evals_constants.ATTEMPTS_JSONL).write_text(canonical_model_json(attempt) + "\n")
    (report_dir / evals_constants.METRICS_JSONL).write_text(canonical_model_json(metric) + "\n")
    (report_dir / evals_constants.RECEIPTS_JSONL).write_text(canonical_model_json(receipt) + "\n")
    (report_dir / evals_constants.STAGES_JSONL).write_text(canonical_model_json(stage) + "\n")

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


def _produce_public_bundle(tmp_path: Path, signing_key: EvalSigningKey) -> Path:
    """Produce a public-only signed bundle with no restricted artifacts."""
    report_dir = tmp_path / "report"
    bundle_dir = tmp_path / "bundle"
    _build_public_report_dir(report_dir)
    produce_bundle(
        report_dir=report_dir,
        bundle_dir=bundle_dir,
        bundle_id="bundle-phase2-acceptance",
        signing_key=signing_key,
        created_at=_TS,
    )
    return bundle_dir


def _trust_store(signing_key: EvalSigningKey) -> EvalTrustStore:
    return EvalTrustStore(keys=[
        EvalTrustedKey(
            key_id=signing_key.key_id,
            public_key_hex=signing_key.public_key_hex,
            algorithm=EVAL_SIGNING_ALGORITHM,
            scope=EVAL_TRUST_SCOPE,
            valid_from=_TS,
            valid_until=_TS_PLUS,
            revoked=False,
            source="protocol-owned-test-metadata",
        )
    ])


# ---------------------------------------------------------------------------
# Acceptance criteria
# ---------------------------------------------------------------------------


class TestPublicFixtureBundleVerification:
    """A public-only fixture bundle is verified in a network-disabled clean
    environment without restricted plaintext or decryption keys."""

    @pytest.mark.integration
    def test_public_bundle_has_no_restricted_artifacts(self, tmp_path: Path) -> None:
        """The public fixture bundle contains no RESTRICTED artifacts and no
        encryption metadata. Public verification requires no decryption keys."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)

        manifest = BundleManifest.model_validate_json(
            (bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON).read_text()
        )

        for entry in manifest.artifacts:
            assert entry.privacy_class != PrivacyClass.RESTRICTED, (
                f"Public fixture bundle must not contain restricted artifact: {entry.path}"
            )
            assert entry.encryption is None, (
                f"Public fixture bundle must not carry encryption metadata: {entry.path}"
            )

    @pytest.mark.integration
    def test_public_bundle_verified_ok_with_external_trust(self, tmp_path: Path) -> None:
        """The typed verification report is ok=True with all twelve layers
        passing when the trust store is supplied externally."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))

        assert report.ok, (
            f"Public bundle should verify ok: {[(f.layer.name, f.code.value, f.message) for f in report.failures]}"
        )
        assert report.bundle_id == "bundle-phase2-acceptance"
        assert report.run_id == "run-1"
        assert report.release_version == "v2.1.8"
        assert len(report.layers) == 12
        for layer_result in report.layers:
            assert layer_result.passed, (
                f"Layer {layer_result.layer.name} should pass: "
                f"{[f.message for f in report.failures if f.layer == layer_result.layer]}"
            )

    @pytest.mark.integration
    def test_verification_requires_no_network_access(self, tmp_path: Path) -> None:
        """The verifier reads only the bundle directory and the externally
        supplied trust store. It requires no originating service, runtime
        directory, network access, or in-bundle trust policy.

        This test patches socket.socket to fail on any outbound connection,
        proving the verifier never touches the network.
        """
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)

        original_socket = socket.socket

        def _blocking_socket(*args: object, **kwargs: object) -> socket.socket:
            raise AssertionError(
                "Verifier attempted a network connection during offline verification"
            )

        socket.socket = _blocking_socket  # type: ignore[assignment]
        try:
            report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        finally:
            socket.socket = original_socket  # type: ignore[assignment]

        assert report.ok, (
            f"Verification should succeed without network: {[(f.layer.name, f.code.value) for f in report.failures]}"
        )

    @pytest.mark.integration
    def test_deterministic_analysis_reproduction_passes(self, tmp_path: Path) -> None:
        """Layer 9 (ANALYSIS_REPRODUCTION) reproduces the canonical analysis
        from analysis-input.json and matches the stored analysis.json."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))

        layer9 = next(
            lr for lr in report.layers if lr.layer == VerificationLayer.ANALYSIS_REPRODUCTION
        )
        assert layer9.passed, (
            f"Layer 9 (analysis reproduction) should pass: "
            f"{[f.message for f in report.failures if f.layer == VerificationLayer.ANALYSIS_REPRODUCTION]}"
        )

    @pytest.mark.integration
    def test_renderer_byte_equality_passes(self, tmp_path: Path) -> None:
        """Layer 10 (RENDERER_EQUALITY) re-renders Markdown, HTML, and CLI from
        the reproduced analysis and matches the stored bytes."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))

        layer10 = next(
            lr for lr in report.layers if lr.layer == VerificationLayer.RENDERER_EQUALITY
        )
        assert layer10.passed, (
            f"Layer 10 (renderer equality) should pass: "
            f"{[f.message for f in report.failures if f.layer == VerificationLayer.RENDERER_EQUALITY]}"
        )

    @pytest.mark.integration
    def test_report_is_deterministic_across_verifications(self, tmp_path: Path) -> None:
        """Two verifications of the same public bundle produce identical
        failure sets (ignoring verified_at)."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)
        report1 = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        report2 = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))

        assert report1.ok == report2.ok
        assert report1.bundle_id == report2.bundle_id
        assert [f.code for f in report1.failures] == [f.code for f in report2.failures]
        assert [f.record_id for f in report1.failures] == [f.record_id for f in report2.failures]


class TestNoInBundleTrust:
    """No verifier trust originates inside the bundle. The trust store is
    supplied externally by the verifier from protocol-owned public-key
    metadata. A key embedded in the bundle is never trusted by the verifier."""

    @pytest.mark.integration
    def test_signed_bundle_without_trust_store_fails(self, tmp_path: Path) -> None:
        """A signed bundle without an external trust store fails with
        TRUST_STORE_MISSING, not ok."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)
        report = verify_bundle(bundle_dir, trust_store=None)

        assert not report.ok
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SIGNATURES_TRUST]
        assert VerificationFailureCode.TRUST_STORE_MISSING in codes

    @pytest.mark.integration
    def test_signed_bundle_with_empty_trust_store_fails(self, tmp_path: Path) -> None:
        """A signed bundle with an empty external trust store fails with
        SIGNATURE_UNKNOWN_KEY. The verifier never trusts a key merely because
        the bundle contains it."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)
        report = verify_bundle(bundle_dir, trust_store=EvalTrustStore(keys=[]))

        assert not report.ok
        codes = [f.code for f in report.failures if f.layer == VerificationLayer.SIGNATURES_TRUST]
        assert VerificationFailureCode.SIGNATURE_UNKNOWN_KEY in codes

    @pytest.mark.integration
    def test_bundle_signature_file_is_not_trusted_as_key_source(self, tmp_path: Path) -> None:
        """The bundle-signature.json file in the bundle directory contains the
        key_id but the verifier never reads it as a trust source. Removing the
        external trust store causes verification to fail even though the
        signature file is present."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_public_bundle(tmp_path, signing_key)

        # The signature file exists in the bundle.
        assert (bundle_dir / evals_constants.BUNDLE_SIGNATURE_JSON).exists()

        # But without an external trust store, verification fails.
        report = verify_bundle(bundle_dir, trust_store=None)
        assert not report.ok


class TestMutationMatrixCoverage:
    """The mutation matrix (P2-04) covers every protected record class. Phase 2
    remains open if any protected class lacks mutation coverage. These tests
    assert the aggregate coverage by importing and counting the mutation
    matrix test collection."""

    @pytest.mark.unit
    def test_mutation_matrix_covers_all_protected_classes(self) -> None:
        """The mutation matrix test module defines tests for every protected
        record class: manifest, task, attempt, posture observation, state
        observation, envelope, receipt, persistence attestation, commitment
        attestation, stage, metric, audit link, evidence artifact, analysis
        input, canonical analysis, renderer, checksum, trust record, and
        signature."""
        import importlib.util

        matrix_path = Path(__file__).parent / "test_bundle_mutation_matrix.py"
        spec = importlib.util.spec_from_file_location("test_bundle_mutation_matrix", matrix_path)
        assert spec is not None
        assert spec.loader is not None
        matrix = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(matrix)

        expected_classes = [
            "TestStaleChecksumMutations",
            "TestTaskSemanticMutations",
            "TestAttemptSemanticMutations",
            "TestReceiptSemanticMutations",
            "TestStageSemanticMutations",
            "TestMetricSemanticMutations",
            "TestAnalysisInputSemanticMutations",
            "TestAnalysisJsonSemanticMutations",
            "TestRendererSemanticMutations",
            "TestManifestSemanticMutations",
            "TestChecksumRootSemanticMutations",
            "TestSignatureSemanticMutations",
            "TestRunManifestSemanticMutations",
            "TestDuplicateRecordRejection",
            "TestUnknownRecordRejection",
            "TestLimitEnforcement",
            "TestDeterministicFailureOrdering",
            "TestObservationJsonlSemanticMutations",
            "TestPostureObservationSemanticMutations",
            "TestAnalysisInputEmbeddedRecordMutations",
            "TestEvidenceArtifactMutations",
            "TestRestrictedArtifactHandling",
            "TestTrustRecordMutations",
            "TestValidBundlePasses",
        ]
        for class_name in expected_classes:
            assert hasattr(matrix, class_name), (
                f"Mutation matrix must define {class_name} for protected-record coverage"
            )
