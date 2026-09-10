# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for source/build provenance verification layer (P2-05, layer 12).

Verifies that the bundle verifier detects missing, empty, and invalid
source/build provenance in the run manifest. Production-posture runs
require source/build provenance; non-production runs may omit it.
"""

from __future__ import annotations

import json
from datetime import UTC, datetime
from pathlib import Path

import pytest

pytestmark = pytest.mark.unit

from g8e_evals import constants as evals_constants
from g8e_evals.analysis import canonical_model_json, compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.bundle import (
    EvalSigningKey,
    EvalTrustStore,
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
    ArmManifestEntry,
    Arm,
    GovernancePosture,
    MetricObservation,
    ProviderBudget,
    ReceiptObservation,
    RunManifest,
    SamplingSettings,
    SourceBuildProvenance,
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
from g8e_evals.schema import ActionReceipt


_TS = datetime(2026, 1, 1, tzinfo=UTC)
_TS_PLUS = datetime(2026, 12, 31, tzinfo=UTC)
_VALID_SHA256 = "a" * 64
_SEED = b"0" * 32


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


def _valid_provenance() -> SourceBuildProvenance:
    return SourceBuildProvenance(
        source_revision="abc123",
        source_tree_state_hash=_VALID_SHA256,
        build_id="build-1",
        build_system="github-actions",
    )


def _build_report_dir(
    report_dir: Path,
    *,
    provenance: SourceBuildProvenance | None = None,
    budget: ProviderBudget | None = None,
    is_production: bool = False,
) -> None:
    """Build a minimal valid report directory with analysis artifacts."""
    report_dir.mkdir(parents=True, exist_ok=True)
    task = _minimal_task()
    attempt = _minimal_attempt()
    metric = _minimal_metric()
    receipt = _minimal_receipt()
    stage = _minimal_stage()

    arm_entry = ArmManifestEntry(
        arm_id=Arm.DOCTRINE,
        requested_posture=GovernancePosture.L1_DOCTRINE,
        uses_g8ee=True,
        uses_gateway=True,
        receipt_binding=True,
        is_production_posture=is_production,
    )
    manifest = RunManifest(
        run_id="run-1",
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        arms=[arm_entry],
        source_build_provenance=provenance,
        provider_budget=budget,
    )
    (report_dir / evals_constants.MANIFEST_JSON).write_text(manifest.model_dump_json())
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
    (report_dir / evals_constants.ANALYSIS_INPUT_JSON).write_text(canonical_model_json(analysis_input))
    from g8e_evals.analysis import render_cli, render_html, render_markdown
    analysis = compute_canonical_analysis_from_record(analysis_input)
    (report_dir / evals_constants.ANALYSIS_JSON).write_text(canonical_model_json(analysis))
    (report_dir / evals_constants.ANALYSIS_MD).write_text(render_markdown(analysis))
    (report_dir / evals_constants.ANALYSIS_HTML).write_text(render_html(analysis))
    (report_dir / evals_constants.ANALYSIS_TXT).write_text(render_cli(analysis))


def _produce_bundle(
    tmp_path: Path,
    *,
    provenance: SourceBuildProvenance | None = None,
    budget: ProviderBudget | None = None,
    is_production: bool = False,
    signing_key: EvalSigningKey | None = None,
) -> Path:
    report_dir = tmp_path / "report"
    bundle_dir = tmp_path / "bundle"
    _build_report_dir(
        report_dir,
        provenance=provenance,
        budget=budget,
        is_production=is_production,
    )
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


def _trusted_key(signing_key: EvalSigningKey):
    from g8e_evals.bundle import EvalTrustedKey
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


def _assert_failure(report, layer: VerificationLayer, code: VerificationFailureCode) -> None:
    """Assert the report has a failure with the given layer and code."""
    matching = [f for f in report.failures if f.layer == layer and f.code == code]
    assert matching, (
        f"Expected failure with layer={layer}, code={code}. "
        f"Got failures: {[(f.layer, f.code, f.message) for f in report.failures]}"
    )


# ---------------------------------------------------------------------------
# Layer 12: Source/build provenance
# ---------------------------------------------------------------------------


class TestSourceBuildProvenanceLayer:
    """Layer 12: source/build provenance bound into the run manifest."""

    def test_valid_provenance_passes(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_bundle(
            tmp_path,
            provenance=_valid_provenance(),
            signing_key=signing_key,
        )
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        layer12 = next(
            lr for lr in report.layers
            if lr.layer == VerificationLayer.SOURCE_BUILD_PROVENANCE
        )
        assert layer12.passed, (
            f"Layer 12 should pass: "
            f"{[f.message for f in report.failures if f.layer == VerificationLayer.SOURCE_BUILD_PROVENANCE]}"
        )

    def test_non_production_without_provenance_passes(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_bundle(
            tmp_path,
            provenance=None,
            is_production=False,
            signing_key=signing_key,
        )
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        layer12 = next(
            lr for lr in report.layers
            if lr.layer == VerificationLayer.SOURCE_BUILD_PROVENANCE
        )
        assert layer12.passed

    def test_production_without_provenance_fails(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_bundle(
            tmp_path,
            provenance=None,
            is_production=True,
            signing_key=signing_key,
        )
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(
            report,
            VerificationLayer.SOURCE_BUILD_PROVENANCE,
            VerificationFailureCode.SOURCE_BUILD_PROVENANCE_MISSING,
        )

    def test_empty_source_revision_fails(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        provenance = _valid_provenance().model_copy(update={"source_revision": ""})
        bundle_dir = _produce_bundle(
            tmp_path,
            provenance=provenance,
            signing_key=signing_key,
        )
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(
            report,
            VerificationLayer.SOURCE_BUILD_PROVENANCE,
            VerificationFailureCode.SOURCE_REVISION_MISSING,
        )

    def test_empty_source_tree_state_hash_fails(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        provenance = _valid_provenance().model_copy(update={"source_tree_state_hash": ""})
        bundle_dir = _produce_bundle(
            tmp_path,
            provenance=provenance,
            signing_key=signing_key,
        )
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(
            report,
            VerificationLayer.SOURCE_BUILD_PROVENANCE,
            VerificationFailureCode.SOURCE_TREE_STATE_HASH_MISSING,
        )

    def test_invalid_source_tree_state_hash_fails(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        provenance = _valid_provenance().model_copy(update={"source_tree_state_hash": "not-hex"})
        bundle_dir = _produce_bundle(
            tmp_path,
            provenance=provenance,
            signing_key=signing_key,
        )
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(
            report,
            VerificationLayer.SOURCE_BUILD_PROVENANCE,
            VerificationFailureCode.SOURCE_TREE_STATE_HASH_INVALID,
        )

    def test_provider_budget_bound_into_manifest(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        budget = ProviderBudget(max_usd=10.0, max_tokens=1000, max_requests=100)
        bundle_dir = _produce_bundle(
            tmp_path,
            provenance=_valid_provenance(),
            budget=budget,
            signing_key=signing_key,
        )
        # The budget is in the run manifest, not directly verified by a
        # layer, but it must be present in the manifest.json.
        manifest_path = bundle_dir / evals_constants.MANIFEST_JSON
        manifest_data = json.loads(manifest_path.read_text())
        assert "provider_budget" in manifest_data
        assert manifest_data["provider_budget"]["max_usd"] == 10.0
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        assert report.ok, f"Bundle should verify: {[(f.layer, f.code, f.message) for f in report.failures]}"
