# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Table-driven mutation matrix for complete protected-record coverage (P2-04).

Every protected manifest, task, attempt, receipt, stage, metric, analysis
input, canonical analysis, renderer, checksum, trust record, and signature
field is mutated in two variants:

1. **Stale checksum**: mutate the file content without updating the manifest
   hash, checksum root, or signature. The verifier detects this through
   ``FILE_HASH_MISMATCH`` (layer 3).

2. **Semantic substitution**: mutate the file content AND recompute the
   manifest entry hash, checksum root, manifest self-hash, and signature
   with the assessed test key. The verifier detects this through a semantic
   check (source-record cross-check, analysis reproduction mismatch, record
   binding validation, or metric producer validation) rather than a stale
   checksum.

Every mutation fails for the semantic reason, not merely because a stale
checksum was left behind. Duplicate and unknown record rejection, explicit
limits, and deterministic failure ordering are also covered.
"""

from __future__ import annotations

import hashlib
import json
from datetime import UTC, datetime
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals import constants as evals_constants
from g8e_evals.analysis import canonical_model_json, compute_canonical_analysis_from_record
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.bundle import (
    BundleManifest,
    BundleSignature,
    ChecksumRoot,
    EvalSigningKey,
    EvalTrustStore,
    EvalTrustedKey,
    PrivacyClass,
    produce_bundle,
    verify_bundle,
)
from g8e_evals.bundle.canonical import (
    compute_checksum_root_hash,
    compute_manifest_hash,
)
from g8e_evals.bundle.signing import (
    EVAL_SIGNING_ALGORITHM,
    EVAL_TRUST_SCOPE,
    sign_bundle,
)
from g8e_evals.bundle.verify import (
    VerificationFailureCode,
    VerificationLayer,
    VerificationReport,
)
from g8e_evals.schema import (
    AttemptRecord,
    AuditLinkRecord,
    CommitmentAttestation,
    EvidenceEncryption,
    EvidenceEncryptionAlgorithm,
    EvidenceIndex,
    EvidenceMediaType,
    FinalStateObservation,
    GovernanceEnvelopeRecord,
    MetricObservation,
    PersistenceAttestation,
    PrivacyClassification,
    ReceiptObservation,
    SecretDetectionObservation,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateObservation,
    StateValue,
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
from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.schema import ActionReceipt


_TS = datetime(2026, 1, 1, tzinfo=UTC)
_TS_PLUS = datetime(2026, 12, 31, tzinfo=UTC)
_SEED = b"k" * 32


def _sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


# ---------------------------------------------------------------------------
# Minimal report directory builder (shared with test_bundle_verify.py shape)
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


def _minimal_final_state_observation() -> FinalStateObservation:
    return FinalStateObservation(
        observation_id="fso-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id="assert-fs-1",
        action_type="TEST_ACTION",
        verification_status=VerificationStatus.VERIFIED,
    )


def _minimal_state_observation() -> StateObservation:
    return StateObservation(
        observation_id="so-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id="assert-s-1",
        action_type="STATE_CHECK",
        fixture_sha256="0" * 64,
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        target="/tmp/test",
        observed=StateValue(kind=StateEvidenceKind.FILE, exists=True),
        collected_at=_TS,
    )


def _minimal_secret_detection_observation() -> SecretDetectionObservation:
    return SecretDetectionObservation(
        observation_id="sdo-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id="assert-sd-1",
        source="provider",
        input_artifact_sha256="0" * 64,
        scanner_version="1.0.0",
        collected_at=_TS,
        true_positive_count=0,
        false_positive_count=0,
        false_negative_count=0,
        true_negative_count=0,
    )


def _minimal_evidence_index() -> EvidenceIndex:
    return EvidenceIndex(
        artifact_id="evidence-1",
        run_id="run-1",
        attempt_id="attempt-1",
        media_type=EvidenceMediaType.APPLICATION_JSON,
        sha256="0" * 64,
        privacy_classification=PrivacyClassification.RESTRICTED,
        encryption=EvidenceEncryption(
            algorithm=EvidenceEncryptionAlgorithm.AES_256_GCM,
            key_id="key-1",
            aad_sha256="0" * 64,
            ciphertext_sha256="1" * 64,
            ciphertext_byte_length=42,
        ),
    )


def _minimal_governance_envelope() -> GovernanceEnvelopeRecord:
    return GovernanceEnvelopeRecord(
        envelope_id="env-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        envelope_sha256="0" * 64,
        layer_disposition="accepted",
        signer_key_id="signer-1",
        created_at=_TS,
    )


def _minimal_persistence_attestation() -> PersistenceAttestation:
    return PersistenceAttestation(
        attestation_id="pa-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        content_sha256="0" * 64,
        persistence_target="durable-store",
        persisted_at=_TS,
        persisted_by="operator-1",
    )


def _minimal_commitment_attestation() -> CommitmentAttestation:
    return CommitmentAttestation(
        attestation_id="ca-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        commitment_hash="0" * 64,
        signer_key_id="signer-1",
        committed_at=_TS,
    )


def _minimal_audit_link() -> AuditLinkRecord:
    return AuditLinkRecord(
        audit_link_id="al-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        audit_record_id="ar-1",
        audit_entry_sha256="0" * 64,
        recorded_at=_TS,
    )


def _build_rich_report_dir(report_dir: Path) -> None:
    """Build a report directory with observation JSONL files, attestations, audit links, and evidence index.

    Includes all protected record classes that the minimal builder omits so
    that mutation tests can target each class independently.
    """
    report_dir.mkdir(parents=True, exist_ok=True)

    task = _minimal_task()
    attempt = _minimal_attempt()
    metric = _minimal_metric()
    receipt = _minimal_receipt()
    stage = _minimal_stage()

    final_state_obs = _minimal_final_state_observation()
    state_obs = _minimal_state_observation()
    secret_detection_obs = _minimal_secret_detection_observation()
    evidence_idx = _minimal_evidence_index()
    envelope = _minimal_governance_envelope()
    persistence = _minimal_persistence_attestation()
    commitment = _minimal_commitment_attestation()
    audit_link = _minimal_audit_link()

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

    (report_dir / evals_constants.FINAL_STATE_OBSERVATIONS_JSONL).write_text(
        canonical_model_json(final_state_obs) + "\n"
    )
    (report_dir / evals_constants.STATE_OBSERVATIONS_JSONL).write_text(
        canonical_model_json(state_obs) + "\n"
    )
    (report_dir / evals_constants.SECRET_DETECTION_OBSERVATIONS_JSONL).write_text(
        canonical_model_json(secret_detection_obs) + "\n"
    )
    (report_dir / evals_constants.EVIDENCE_INDEX_JSONL).write_text(
        canonical_model_json(evidence_idx) + "\n"
    )

    analysis_input = AnalysisInputRecord(
        run_id="run-1",
        release_version="v2.1.8",
        tasks=[task],
        attempts=[attempt],
        metric_observations=[metric],
        receipts=[receipt],
        stages=[stage],
        final_state_observations=[final_state_obs],
        state_observations=[state_obs],
        secret_detection_observations=[secret_detection_obs],
        governance_envelopes=[envelope],
        persistence_attestations=[persistence],
        commitment_attestations=[commitment],
        audit_links=[audit_link],
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


def _produce_valid_rich_bundle(tmp_path: Path, signing_key: EvalSigningKey | None = None) -> Path:
    """Produce a valid signed bundle from a rich report directory with all protected record classes."""
    report_dir = tmp_path / "report"
    bundle_dir = tmp_path / "bundle"
    _build_rich_report_dir(report_dir)
    produce_bundle(
        report_dir=report_dir,
        bundle_dir=bundle_dir,
        bundle_id="bundle-1",
        run_id="run-1",
        release_version="v2.1.8",
        signing_key=signing_key,
        created_at=_TS,
        restricted_encryption=EvidenceEncryption(
            algorithm=EvidenceEncryptionAlgorithm.AES_256_GCM,
            key_id="key-1",
            aad_sha256="0" * 64,
            ciphertext_sha256="1" * 64,
            ciphertext_byte_length=42,
        ),
    )
    return bundle_dir


def _produce_valid_bundle(tmp_path: Path, signing_key: EvalSigningKey | None = None) -> Path:
    """Produce a valid signed bundle from a minimal report directory."""
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
# Semantic substitution helper: recompute hashes and signature after mutation
# ---------------------------------------------------------------------------


def _recompute_hashes_and_signature(
    bundle_dir: Path,
    signing_key: EvalSigningKey,
    mutated_filename: str | None = None,
) -> None:
    """Recompute manifest entry hash, checksum root, manifest self-hash, and signature.

    If ``mutated_filename`` is given, only that artifact's hash is updated.
    Otherwise all artifact hashes are recomputed from file contents.
    """
    manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
    checksum_path = bundle_dir / evals_constants.CHECKSUM_ROOT_JSON
    signature_path = bundle_dir / evals_constants.BUNDLE_SIGNATURE_JSON

    manifest = BundleManifest.model_validate_json(manifest_path.read_text())
    checksum_root = ChecksumRoot.model_validate_json(checksum_path.read_text())

    # Recompute artifact hashes.
    new_entries = []
    for entry in manifest.artifacts:
        if mutated_filename is not None and entry.path != mutated_filename:
            new_entries.append(entry)
            continue
        file_path = bundle_dir / entry.path
        content = file_path.read_bytes()
        new_entries.append(entry.model_copy(update={
            "sha256": _sha256(content),
            "byte_length": len(content),
        }))
    manifest = manifest.model_copy(update={"artifacts": new_entries})

    # Rebuild checksum root.
    new_checksum_entries = [
        type(checksum_root.entries[0])(path=e.path, sha256=e.sha256)
        for e in new_entries
    ]
    checksum_root = ChecksumRoot(
        schema_version=checksum_root.schema_version,
        entries=new_checksum_entries,
        checksum_root_sha256="",
    )
    checksum_hash = compute_checksum_root_hash(checksum_root)
    checksum_root = checksum_root.model_copy(update={"checksum_root_sha256": checksum_hash})

    # Set checksum root hash on manifest, compute manifest hash.
    manifest = manifest.model_copy(update={"checksum_root_sha256": checksum_hash})
    manifest_hash = compute_manifest_hash(manifest)
    manifest = manifest.model_copy(update={"manifest_content_sha256": manifest_hash})

    # Write updated manifest and checksum root.
    manifest_path.write_text(canonical_model_json(manifest))
    checksum_path.write_text(canonical_model_json(checksum_root))

    # Recompute signature.
    signature = sign_bundle(manifest, checksum_root, signing_key, _TS)
    signature_path.write_text(canonical_model_json(signature))


def _read_jsonl_line(path: Path, line_index: int = 0) -> dict:
    """Read one line from a JSONL file as a dict."""
    lines = path.read_text().splitlines()
    return json.loads(lines[line_index])


def _write_jsonl_line(path: Path, record: dict, line_index: int = 0) -> None:
    """Write one record to a JSONL file, replacing the given line."""
    lines = path.read_text().splitlines()
    lines[line_index] = json.dumps(record, sort_keys=True)
    path.write_text("\n".join(lines) + "\n")


def _read_json(path: Path) -> dict:
    return json.loads(path.read_text())


def _write_json(path: Path, obj: dict) -> None:
    path.write_text(json.dumps(obj, sort_keys=True))


def _assert_failure(
    report: VerificationReport,
    layer: VerificationLayer,
    code: VerificationFailureCode,
) -> None:
    """Assert that the report contains a failure with the given layer and code."""
    matching = [
        f for f in report.failures
        if f.layer == layer and f.code == code
    ]
    assert matching, (
        f"expected failure layer={layer.name} code={code.value} "
        f"but got: {[(f.layer.name, f.code.value, f.message) for f in report.failures]}"
    )


# ---------------------------------------------------------------------------
# Mutation matrix: stale checksum variants (layer 3 FILE_HASH_MISMATCH)
# ---------------------------------------------------------------------------


class TestStaleChecksumMutations:
    """Mutating any protected file without updating hashes is detected by layer 3."""

    @pytest.mark.parametrize("filename", [
        evals_constants.MANIFEST_JSON,
        evals_constants.TASKS_JSONL,
        evals_constants.ATTEMPTS_JSONL,
        evals_constants.METRICS_JSONL,
        evals_constants.RECEIPTS_JSONL,
        evals_constants.STAGES_JSONL,
        evals_constants.ANALYSIS_INPUT_JSON,
        evals_constants.ANALYSIS_JSON,
        evals_constants.ANALYSIS_MD,
        evals_constants.ANALYSIS_HTML,
        evals_constants.ANALYSIS_TXT,
    ])
    def test_stale_checksum_detected(self, tmp_path: Path, filename: str) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        # Mutate the file content.
        path = bundle_dir / filename
        original = path.read_bytes()
        mutated = original + b"X" if original else b"X"
        path.write_bytes(mutated)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.FILE_HASHES, VerificationFailureCode.FILE_HASH_MISMATCH)


# ---------------------------------------------------------------------------
# Mutation matrix: semantic substitution variants for JSONL source records
# ---------------------------------------------------------------------------


class TestTaskSemanticMutations:
    """Mutating task records with recomputed hashes is detected semantically."""

    def test_task_id_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        # Mutate task_id in tasks.jsonl only (not analysis-input.json).
        path = bundle_dir / evals_constants.TASKS_JSONL
        record = _read_jsonl_line(path)
        record["task_id"] = "task-mutated"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.TASKS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)

    def test_task_prompt_hash_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.TASKS_JSONL
        record = _read_jsonl_line(path)
        record["prompt_hash"] = "a" * 64
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.TASKS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)


class TestAttemptSemanticMutations:
    """Mutating attempt records with recomputed hashes is detected semantically."""

    def test_attempt_run_id_mutation_detected_by_binding(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ATTEMPTS_JSONL
        record = _read_jsonl_line(path)
        record["run_id"] = "wrong-run"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ATTEMPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RECORD_BINDINGS, VerificationFailureCode.RUN_ID_MISMATCH)

    def test_attempt_task_id_mutation_detected_by_unknown_task(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ATTEMPTS_JSONL
        record = _read_jsonl_line(path)
        record["task_id"] = "nonexistent-task"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ATTEMPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RECORD_BINDINGS, VerificationFailureCode.UNKNOWN_TASK)

    def test_attempt_id_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ATTEMPTS_JSONL
        record = _read_jsonl_line(path)
        record["attempt_id"] = "attempt-mutated"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ATTEMPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)


class TestReceiptSemanticMutations:
    """Mutating receipt records with recomputed hashes is detected semantically."""

    def test_receipt_run_id_mutation_detected_by_cross_run(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.RECEIPTS_JSONL
        record = _read_jsonl_line(path)
        record["run_id"] = "wrong-run"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.RECEIPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RECORD_BINDINGS, VerificationFailureCode.CROSS_RUN_RECORD)

    def test_receipt_attempt_id_mutation_detected_by_unknown_attempt(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.RECEIPTS_JSONL
        record = _read_jsonl_line(path)
        record["attempt_id"] = "nonexistent-attempt"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.RECEIPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RECORD_BINDINGS, VerificationFailureCode.UNKNOWN_ATTEMPT)

    def test_receipt_verified_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.RECEIPTS_JSONL
        record = _read_jsonl_line(path)
        record["verified"] = True  # Mutate from False to True.
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.RECEIPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)


class TestStageSemanticMutations:
    """Mutating stage records with recomputed hashes is detected semantically."""

    def test_stage_run_id_mutation_detected_by_run_mismatch(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.STAGES_JSONL
        record = _read_jsonl_line(path)
        record["run_id"] = "wrong-run"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.STAGES_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.CHAIN_LINKS, VerificationFailureCode.STAGE_RUN_MISMATCH)

    def test_stage_task_id_mutation_detected_by_task_mismatch(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.STAGES_JSONL
        record = _read_jsonl_line(path)
        record["task_id"] = "wrong-task"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.STAGES_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.CHAIN_LINKS, VerificationFailureCode.STAGE_TASK_MISMATCH)

    def test_stage_attempt_id_mutation_detected_by_unknown_attempt(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.STAGES_JSONL
        record = _read_jsonl_line(path)
        record["attempt_id"] = "nonexistent-attempt"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.STAGES_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.CHAIN_LINKS, VerificationFailureCode.STAGE_UNKNOWN_ATTEMPT)

    def test_stage_id_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.STAGES_JSONL
        record = _read_jsonl_line(path)
        record["stage_id"] = "stage-mutated"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.STAGES_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)


class TestMetricSemanticMutations:
    """Mutating metric records with recomputed hashes is detected semantically."""

    def test_metric_attempt_id_mutation_detected_by_unknown_attempt(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.METRICS_JSONL
        record = _read_jsonl_line(path)
        record["attempt_id"] = "nonexistent-attempt"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.METRICS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.METRIC_PRODUCERS, VerificationFailureCode.METRIC_UNKNOWN_ATTEMPT)

    def test_metric_value_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.METRICS_JSONL
        record = _read_jsonl_line(path)
        record["value"] = 0.0
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.METRICS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)

    def test_metric_verification_status_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.METRICS_JSONL
        record = _read_jsonl_line(path)
        record["verification_status"] = "failed"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.METRICS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)


# ---------------------------------------------------------------------------
# Mutation matrix: semantic substitution variants for analysis-input.json
# ---------------------------------------------------------------------------


class TestAnalysisInputSemanticMutations:
    """Mutating analysis-input.json with recomputed hashes is detected by analysis reproduction mismatch."""

    def test_run_id_mutation_detected_by_reproduction_failure(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["run_id"] = "wrong-run"
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_FAILED)

    def test_release_version_mutation_detected_by_reproduction_mismatch(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["release_version"] = "v9.9.9"
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH)

    def test_metric_value_mutation_detected_by_reproduction_mismatch(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["metric_observations"][0]["value"] = 0.0
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH)


# ---------------------------------------------------------------------------
# Mutation matrix: canonical analysis and renderer mutations
# ---------------------------------------------------------------------------


class TestAnalysisJsonSemanticMutations:
    """Mutating analysis.json with recomputed hashes is detected by reproduction mismatch."""

    def test_analysis_json_content_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_JSON
        record = _read_json(path)
        # Mutate a field that doesn't affect the schema.
        if "input_summary" in record:
            record["input_summary"]["run_id"] = "wrong-run"
        else:
            record["run_id"] = "wrong-run"
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH)


class TestRendererSemanticMutations:
    """Mutating renderer files with recomputed hashes is detected by renderer byte mismatch."""

    @pytest.mark.parametrize("filename", [
        evals_constants.ANALYSIS_MD,
        evals_constants.ANALYSIS_HTML,
        evals_constants.ANALYSIS_TXT,
    ])
    def test_renderer_mutation_detected(self, tmp_path: Path, filename: str) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / filename
        path.write_text("MUTATED RENDERER CONTENT")
        _recompute_hashes_and_signature(bundle_dir, signing_key, filename)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RENDERER_EQUALITY, VerificationFailureCode.RENDERER_BYTE_MISMATCH)


# ---------------------------------------------------------------------------
# Mutation matrix: manifest, checksum, and signature mutations
# ---------------------------------------------------------------------------


class TestManifestSemanticMutations:
    """Mutating manifest fields with recomputed self-hash is detected by self-hash mismatch or binding."""

    def test_bundle_id_mutation_detected_by_signature_substitution(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = BundleManifest.model_validate_json(manifest_path.read_text())
        manifest = manifest.model_copy(update={"bundle_id": "wrong-bundle"})
        # Recompute manifest self-hash.
        manifest_hash = compute_manifest_hash(manifest)
        manifest = manifest.model_copy(update={"manifest_content_sha256": manifest_hash})
        manifest_path.write_text(canonical_model_json(manifest))
        # Recompute checksum root (unchanged, but self-hash may change due to manifest hash).
        # The signature is NOT recomputed, so the bound bundle_id mismatch is detected.
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_SUBSTITUTED)

    def test_run_id_mutation_detected_by_signature_substitution(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = BundleManifest.model_validate_json(manifest_path.read_text())
        manifest = manifest.model_copy(update={"run_id": "wrong-run"})
        manifest_hash = compute_manifest_hash(manifest)
        manifest = manifest.model_copy(update={"manifest_content_sha256": manifest_hash})
        manifest_path.write_text(canonical_model_json(manifest))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_SUBSTITUTED)

    def test_release_version_mutation_detected_by_signature_substitution(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = BundleManifest.model_validate_json(manifest_path.read_text())
        manifest = manifest.model_copy(update={"release_version": "v9.9.9"})
        manifest_hash = compute_manifest_hash(manifest)
        manifest = manifest.model_copy(update={"manifest_content_sha256": manifest_hash})
        manifest_path.write_text(canonical_model_json(manifest))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_SUBSTITUTED)


class TestChecksumRootSemanticMutations:
    """Mutating checksum root with recomputed self-hash is detected by checksum-manifest mismatch."""

    def test_checksum_entry_hash_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        checksum_path = bundle_dir / evals_constants.CHECKSUM_ROOT_JSON
        checksum_root = ChecksumRoot.model_validate_json(checksum_path.read_text())
        # Mutate one entry hash.
        mutated_entries = [
            checksum_root.entries[0].model_copy(update={"sha256": "b" * 64}),
            *checksum_root.entries[1:],
        ]
        checksum_root = checksum_root.model_copy(update={"entries": mutated_entries})
        # Recompute self-hash.
        checksum_hash = compute_checksum_root_hash(checksum_root)
        checksum_root = checksum_root.model_copy(update={"checksum_root_sha256": checksum_hash})
        checksum_path.write_text(canonical_model_json(checksum_root))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.SCHEMAS_CANONICAL, VerificationFailureCode.CHECKSUM_MANIFEST_MISMATCH)


class TestSignatureSemanticMutations:
    """Mutating signature fields is detected by signature verification."""

    def test_signature_key_id_mutation_detected_as_unknown_key(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        sig_path = bundle_dir / evals_constants.BUNDLE_SIGNATURE_JSON
        sig = BundleSignature.model_validate_json(sig_path.read_text())
        # Mutate the key_id to a different key.
        other_key = EvalSigningKey.from_seed(b"j" * 32)
        sig = sig.model_copy(update={
            "manifest_signature": sig.manifest_signature.model_copy(update={
                "key_id": other_key.key_id,
            }),
        })
        sig_path.write_text(canonical_model_json(sig))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_UNKNOWN_KEY)

    def test_signature_bundle_id_mutation_detected_as_substituted(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        sig_path = bundle_dir / evals_constants.BUNDLE_SIGNATURE_JSON
        sig = BundleSignature.model_validate_json(sig_path.read_text())
        sig = sig.model_copy(update={
            "manifest_signature": sig.manifest_signature.model_copy(update={
                "bundle_id": "wrong-bundle",
            }),
        })
        sig_path.write_text(canonical_model_json(sig))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_SUBSTITUTED)

    def test_signature_signature_bytes_mutation_detected_as_malformed(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        sig_path = bundle_dir / evals_constants.BUNDLE_SIGNATURE_JSON
        sig = BundleSignature.model_validate_json(sig_path.read_text())
        # Mutate the signature bytes (keep valid hex length).
        new_sig = "0" * 128
        sig = sig.model_copy(update={
            "manifest_signature": sig.manifest_signature.model_copy(update={
                "signature": new_sig,
            }),
        })
        sig_path.write_text(canonical_model_json(sig))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_MALFORMED)


# ---------------------------------------------------------------------------
# Mutation matrix: run manifest mutations
# ---------------------------------------------------------------------------


class TestRunManifestSemanticMutations:
    """Mutating run manifest with recomputed hashes is detected by run_id mismatch."""

    def test_run_manifest_run_id_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.MANIFEST_JSON
        record = _read_json(path)
        record["run_id"] = "wrong-run"
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.MANIFEST_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RECORD_BINDINGS, VerificationFailureCode.RUN_ID_MISMATCH)


# ---------------------------------------------------------------------------
# Duplicate and unknown record rejection
# ---------------------------------------------------------------------------


class TestDuplicateRecordRejection:
    """Duplicate records are rejected with DUPLICATE_IDENTITY."""

    def test_duplicate_task_id_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.TASKS_JSONL
        # Append a duplicate task line.
        original = path.read_text()
        path.write_text(original + original)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.TASKS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RECORD_BINDINGS, VerificationFailureCode.DUPLICATE_IDENTITY)

    def test_duplicate_attempt_id_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ATTEMPTS_JSONL
        original = path.read_text()
        path.write_text(original + original)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ATTEMPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RECORD_BINDINGS, VerificationFailureCode.DUPLICATE_IDENTITY)

    def test_duplicate_stage_id_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.STAGES_JSONL
        original = path.read_text()
        path.write_text(original + original)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.STAGES_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.CHAIN_LINKS, VerificationFailureCode.DUPLICATE_IDENTITY)

    def test_duplicate_metric_observation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.METRICS_JSONL
        original = path.read_text()
        path.write_text(original + original)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.METRICS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.METRIC_PRODUCERS, VerificationFailureCode.METRIC_DUPLICATE_IDENTITY)


class TestUnknownRecordRejection:
    """Records referencing unknown parents are rejected."""

    def test_receipt_unknown_attempt_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.RECEIPTS_JSONL
        record = _read_jsonl_line(path)
        record["attempt_id"] = "nonexistent-attempt"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.RECEIPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.RECORD_BINDINGS, VerificationFailureCode.UNKNOWN_ATTEMPT)

    def test_stage_unknown_attempt_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.STAGES_JSONL
        record = _read_jsonl_line(path)
        record["attempt_id"] = "nonexistent-attempt"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.STAGES_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.CHAIN_LINKS, VerificationFailureCode.STAGE_UNKNOWN_ATTEMPT)

    def test_metric_unknown_attempt_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.METRICS_JSONL
        record = _read_jsonl_line(path)
        record["attempt_id"] = "nonexistent-attempt"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.METRICS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.METRIC_PRODUCERS, VerificationFailureCode.METRIC_UNKNOWN_ATTEMPT)


# ---------------------------------------------------------------------------
# Limit tests
# ---------------------------------------------------------------------------


class TestLimitEnforcement:
    """Exceeding explicit limits is detected with the correct failure code."""

    def test_orphan_file_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        (bundle_dir / "orphan.txt").write_text("not in manifest")
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ROOTED_INVENTORY, VerificationFailureCode.ORPHAN_FILE)

    def test_missing_file_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        (bundle_dir / evals_constants.TASKS_JSONL).unlink()
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ROOTED_INVENTORY, VerificationFailureCode.MISSING_FILE)

    def test_symlink_rejected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        (bundle_dir / "evil.link").symlink_to(bundle_dir / evals_constants.TASKS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ROOTED_INVENTORY, VerificationFailureCode.SYMLINK_REJECTED)

    def test_path_traversal_in_manifest_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = BundleManifest.model_validate_json(manifest_path.read_text())
        # Add a traversal path entry.
        from g8e_evals.bundle.manifest import ArtifactType, BundleArtifactEntry, PrivacyClass
        evil_entry = BundleArtifactEntry(
            path="../evil.txt",
            media_type="text/plain",
            privacy_class=PrivacyClass.PUBLIC,
            sha256="0" * 64,
            byte_length=0,
            artifact_type=ArtifactType.DIAGNOSTIC,
        )
        manifest = manifest.model_copy(update={
            "artifacts": [*manifest.artifacts, evil_entry],
        })
        # Do NOT recompute the manifest self-hash: compute_manifest_hash calls
        # validate_bundle_path which rejects traversal paths. The verifier
        # detects the traversal in layer 1 regardless of the self-hash.
        manifest_path.write_text(canonical_model_json(manifest))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ROOTED_INVENTORY, VerificationFailureCode.PATH_TRAVERSAL)

    def test_duplicate_path_in_manifest_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = BundleManifest.model_validate_json(manifest_path.read_text())
        # Duplicate the first entry.
        manifest = manifest.model_copy(update={
            "artifacts": [manifest.artifacts[0], *manifest.artifacts],
        })
        manifest_hash = compute_manifest_hash(manifest)
        manifest = manifest.model_copy(update={"manifest_content_sha256": manifest_hash})
        manifest_path.write_text(canonical_model_json(manifest))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ROOTED_INVENTORY, VerificationFailureCode.DUPLICATE_PATH)


# ---------------------------------------------------------------------------
# Deterministic failure ordering
# ---------------------------------------------------------------------------


class TestDeterministicFailureOrdering:
    """Failures are sorted by layer order, then record ID, then code."""

    def test_failures_sorted_by_layer(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        # Add orphan (layer 1) and corrupt analysis.json (layer 9).
        (bundle_dir / "orphan.txt").write_text("not in manifest")
        (bundle_dir / evals_constants.ANALYSIS_JSON).write_text('{"corrupted": true}')
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        if len(report.failures) >= 2:
            for i in range(len(report.failures) - 1):
                assert int(report.failures[i].layer) <= int(report.failures[i + 1].layer)

    def test_report_is_deterministic_across_verifications(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        (bundle_dir / "orphan.txt").write_text("not in manifest")
        report1 = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        report2 = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        assert [f.code for f in report1.failures] == [f.code for f in report2.failures]
        assert [f.record_id for f in report1.failures] == [f.record_id for f in report2.failures]
        assert [int(f.layer) for f in report1.failures] == [int(f.layer) for f in report2.failures]

    def test_multiple_failures_in_same_layer_sorted_by_record_id(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        # Add two orphan files with different names.
        (bundle_dir / "zzz_orphan.txt").write_text("not in manifest")
        (bundle_dir / "aaa_orphan.txt").write_text("not in manifest")
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        layer1_failures = [f for f in report.failures if f.layer == VerificationLayer.ROOTED_INVENTORY]
        orphan_failures = [f for f in layer1_failures if f.code == VerificationFailureCode.ORPHAN_FILE]
        if len(orphan_failures) >= 2:
            for i in range(len(orphan_failures) - 1):
                assert orphan_failures[i].record_id <= orphan_failures[i + 1].record_id


# ---------------------------------------------------------------------------
# Mutation matrix: observation JSONL semantic substitutions (layer 9)
# ---------------------------------------------------------------------------


class TestObservationJsonlSemanticMutations:
    """Mutating observation JSONL files with recomputed hashes is detected by source-record cross-check."""

    @pytest.mark.parametrize("filename", [
        evals_constants.FINAL_STATE_OBSERVATIONS_JSONL,
        evals_constants.STATE_OBSERVATIONS_JSONL,
        evals_constants.SECRET_DETECTION_OBSERVATIONS_JSONL,
    ])
    def test_observation_run_id_mutation_detected_by_source_cross_check(self, tmp_path: Path, filename: str) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / filename
        record = _read_jsonl_line(path)
        record["run_id"] = "wrong-run"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, filename)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)

    @pytest.mark.parametrize("filename", [
        evals_constants.FINAL_STATE_OBSERVATIONS_JSONL,
        evals_constants.STATE_OBSERVATIONS_JSONL,
        evals_constants.SECRET_DETECTION_OBSERVATIONS_JSONL,
    ])
    def test_observation_attempt_id_mutation_detected_by_source_cross_check(self, tmp_path: Path, filename: str) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / filename
        record = _read_jsonl_line(path)
        record["attempt_id"] = "wrong-attempt"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, filename)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)

    def test_state_observation_target_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.STATE_OBSERVATIONS_JSONL
        record = _read_jsonl_line(path)
        record["target"] = "/different/path"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.STATE_OBSERVATIONS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)

    def test_final_state_observation_assertion_id_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.FINAL_STATE_OBSERVATIONS_JSONL
        record = _read_jsonl_line(path)
        record["assertion_id"] = "wrong-assertion"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.FINAL_STATE_OBSERVATIONS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)

    def test_secret_detection_observation_scanner_version_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.SECRET_DETECTION_OBSERVATIONS_JSONL
        record = _read_jsonl_line(path)
        record["scanner_version"] = "9.9.9"
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.SECRET_DETECTION_OBSERVATIONS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)


# ---------------------------------------------------------------------------
# Mutation matrix: posture observations (attempt-embedded, layer 9)
# ---------------------------------------------------------------------------


class TestPostureObservationSemanticMutations:
    """Mutating the posture field on an attempt is detected by source-record cross-check.

    PostureObservation is embedded in AttemptRecord, not a separate JSONL
    file. Mutating the posture in attempts.jsonl while leaving
    analysis-input.json unchanged produces a source-record mismatch.
    """

    def test_posture_requested_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ATTEMPTS_JSONL
        record = _read_jsonl_line(path)
        record["posture"]["requested_posture"] = GovernancePosture.L3_NOTARY.value
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ATTEMPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)

    def test_posture_observed_mutation_detected_by_source_cross_check(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ATTEMPTS_JSONL
        record = _read_jsonl_line(path)
        record["posture"]["observed_posture"] = GovernancePosture.L2_CONSENSUS.value
        _write_jsonl_line(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ATTEMPTS_JSONL)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.SOURCE_RECORD_MISMATCH)


# ---------------------------------------------------------------------------
# Mutation matrix: analysis-input-level semantic substitutions (layer 9)
# ---------------------------------------------------------------------------


class TestAnalysisInputEmbeddedRecordMutations:
    """Mutating records embedded only in analysis-input.json (envelopes, attestations, audit links) is detected by analysis reproduction mismatch.

    These record classes do not have their own JSONL files; they are
    embedded in analysis-input.json. Mutating them with recomputed hashes
    produces a reproduced analysis that does not match the stored
    analysis.json.
    """

    def test_governance_envelope_run_id_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["governance_envelopes"][0]["run_id"] = "wrong-run"
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_FAILED)

    def test_governance_envelope_envelope_hash_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["governance_envelopes"][0]["envelope_sha256"] = "f" * 64
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH)

    def test_persistence_attestation_content_hash_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["persistence_attestations"][0]["content_sha256"] = "f" * 64
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH)

    def test_persistence_attestation_run_id_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["persistence_attestations"][0]["run_id"] = "wrong-run"
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_FAILED)

    def test_commitment_attestation_hash_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["commitment_attestations"][0]["commitment_hash"] = "f" * 64
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH)

    def test_commitment_attestation_run_id_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["commitment_attestations"][0]["run_id"] = "wrong-run"
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_FAILED)

    def test_audit_link_entry_hash_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["audit_links"][0]["audit_entry_sha256"] = "f" * 64
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_MISMATCH)

    def test_audit_link_run_id_mutation_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.ANALYSIS_INPUT_JSON
        record = _read_json(path)
        record["audit_links"][0]["run_id"] = "wrong-run"
        _write_json(path, record)
        _recompute_hashes_and_signature(bundle_dir, signing_key, evals_constants.ANALYSIS_INPUT_JSON)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.ANALYSIS_REPRODUCTION, VerificationFailureCode.ANALYSIS_REPRODUCTION_FAILED)


# ---------------------------------------------------------------------------
# Mutation matrix: evidence artifacts (evidence-index.jsonl, restricted)
# ---------------------------------------------------------------------------


class TestEvidenceArtifactMutations:
    """Mutating evidence-index.jsonl is detected by stale checksum (layer 3).

    The evidence index is a RESTRICTED artifact. It is not in the
    source-record cross-check map because it is not a field on
    AnalysisInputRecord. Stale checksum mutations are detected by layer 3.
    Semantic substitutions (recomputed hashes) are not detected by a
    semantic check; this is a known limitation documented in the P2-04
    checkpoint. The restricted artifact handling tests below verify that
    the privacy separation layer enforces encryption metadata.
    """

    def test_evidence_index_stale_checksum_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.EVIDENCE_INDEX_JSONL
        original = path.read_bytes()
        mutated = original + b"X" if original else b"X"
        path.write_bytes(mutated)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.FILE_HASHES, VerificationFailureCode.FILE_HASH_MISMATCH)

    def test_evidence_index_artifact_id_mutation_detected_by_stale_checksum(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        path = bundle_dir / evals_constants.EVIDENCE_INDEX_JSONL
        record = _read_jsonl_line(path)
        record["artifact_id"] = "wrong-artifact"
        _write_jsonl_line(path, record)
        # Do NOT recompute hashes: stale checksum detection.
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.FILE_HASHES, VerificationFailureCode.FILE_HASH_MISMATCH)


# ---------------------------------------------------------------------------
# Mutation matrix: restricted artifact handling without plaintext or keys
# ---------------------------------------------------------------------------


class TestRestrictedArtifactHandling:
    """Restricted artifacts must carry encryption metadata; public artifacts must not.

    The privacy separation layer (layer 11) enforces that restricted
    artifacts carry encryption metadata and public artifacts do not.
    Restricted plaintext or decryption keys are never exposed in the
    manifest; only ciphertext hashes and key IDs are present.
    """

    def test_restricted_artifact_without_encryption_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = BundleManifest.model_validate_json(manifest_path.read_text())
        # Find the evidence-index entry (restricted) and strip its encryption.
        new_entries = []
        for entry in manifest.artifacts:
            if entry.path == evals_constants.EVIDENCE_INDEX_JSONL:
                new_entries.append(entry.model_copy(update={"encryption": None}))
            else:
                new_entries.append(entry)
        manifest = manifest.model_copy(update={"artifacts": new_entries})
        manifest_hash = compute_manifest_hash(manifest)
        manifest = manifest.model_copy(update={"manifest_content_sha256": manifest_hash})
        manifest_path.write_text(canonical_model_json(manifest))
        # Recompute signature so only the privacy separation layer fails.
        checksum_path = bundle_dir / evals_constants.CHECKSUM_ROOT_JSON
        checksum_root = ChecksumRoot.model_validate_json(checksum_path.read_text())
        signature = sign_bundle(manifest, checksum_root, signing_key, _TS)
        (bundle_dir / evals_constants.BUNDLE_SIGNATURE_JSON).write_text(canonical_model_json(signature))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.PRIVACY_SEPARATION, VerificationFailureCode.RESTRICTED_WITHOUT_ENCRYPTION)

    def test_public_artifact_with_encryption_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = BundleManifest.model_validate_json(manifest_path.read_text())
        # Find a public artifact and add encryption metadata.
        encryption = EvidenceEncryption(
            algorithm=EvidenceEncryptionAlgorithm.AES_256_GCM,
            key_id="key-1",
            aad_sha256="0" * 64,
            ciphertext_sha256="1" * 64,
            ciphertext_byte_length=42,
        )
        new_entries = []
        for entry in manifest.artifacts:
            if entry.path == evals_constants.TASKS_JSONL:
                new_entries.append(entry.model_copy(update={"encryption": encryption}))
            else:
                new_entries.append(entry)
        manifest = manifest.model_copy(update={"artifacts": new_entries})
        manifest_hash = compute_manifest_hash(manifest)
        manifest = manifest.model_copy(update={"manifest_content_sha256": manifest_hash})
        manifest_path.write_text(canonical_model_json(manifest))
        # Recompute signature.
        checksum_path = bundle_dir / evals_constants.CHECKSUM_ROOT_JSON
        checksum_root = ChecksumRoot.model_validate_json(checksum_path.read_text())
        signature = sign_bundle(manifest, checksum_root, signing_key, _TS)
        (bundle_dir / evals_constants.BUNDLE_SIGNATURE_JSON).write_text(canonical_model_json(signature))
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        _assert_failure(report, VerificationLayer.PRIVACY_SEPARATION, VerificationFailureCode.RESTRICTED_PLAINTEXT_EXPOSED)

    def test_restricted_manifest_exposes_only_ciphertext_hashes(self, tmp_path: Path) -> None:
        """The manifest for a restricted artifact carries ciphertext hashes, not plaintext or decryption keys."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        manifest_path = bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON
        manifest = BundleManifest.model_validate_json(manifest_path.read_text())
        restricted_entries = [e for e in manifest.artifacts if e.privacy_class == PrivacyClass.RESTRICTED]
        assert restricted_entries, "expected at least one restricted artifact"
        for entry in restricted_entries:
            assert entry.encryption is not None, f"restricted artifact {entry.path} must have encryption"
            # The encryption metadata exposes ciphertext hash and key ID, never plaintext or the key itself.
            assert entry.encryption.ciphertext_sha256 != ""
            assert entry.encryption.key_id != ""
            assert entry.encryption.algorithm == EvidenceEncryptionAlgorithm.AES_256_GCM
            # The artifact hash is the hash of the file content (ciphertext), not plaintext.
            assert entry.sha256 != ""


# ---------------------------------------------------------------------------
# Mutation matrix: trust record mutations (layer 4)
# ---------------------------------------------------------------------------


class TestTrustRecordMutations:
    """Mutating trust store entries is detected by signature verification (layer 4).

    The trust store is supplied externally by the verifier and is never
    read from inside the bundle. These tests verify that trust record
    mutations (revoked, expired, wrong scope, wrong algorithm, substituted
    public key) are detected with the correct typed failure code.
    """

    def test_revoked_trust_key_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        # Supply a trust store with the correct key marked as revoked.
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
        trust_store = EvalTrustStore(keys=[revoked_key])
        report = verify_bundle(bundle_dir, trust_store=trust_store)
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_REVOKED)

    def test_expired_trust_key_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        # Supply a trust store with a key whose validity window has expired.
        expired_key = EvalTrustedKey(
            key_id=signing_key.key_id,
            public_key_hex=signing_key.public_key_hex,
            algorithm=EVAL_SIGNING_ALGORITHM,
            scope=EVAL_TRUST_SCOPE,
            valid_from=datetime(2020, 1, 1, tzinfo=UTC),
            valid_until=datetime(2020, 12, 31, tzinfo=UTC),
            revoked=False,
            source="protocol-owned-test-metadata",
        )
        trust_store = EvalTrustStore(keys=[expired_key])
        report = verify_bundle(bundle_dir, trust_store=trust_store)
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_EXPIRED)

    def test_wrong_scope_trust_key_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
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
        trust_store = EvalTrustStore(keys=[wrong_scope_key])
        report = verify_bundle(bundle_dir, trust_store=trust_store)
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_WRONG_SCOPE)

    def test_wrong_algorithm_trust_key_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        # The trust store key declares a different algorithm than the signature.
        # model_copy bypasses the field validator since the trust store is an
        # in-memory object supplied directly to the verifier (never re-parsed).
        wrong_algo_key = _trusted_key(signing_key).model_copy(update={"algorithm": "ed448"})
        trust_store = EvalTrustStore(keys=[wrong_algo_key])
        report = verify_bundle(bundle_dir, trust_store=trust_store)
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_WRONG_ALGORITHM)

    def test_substituted_public_key_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        other_key = EvalSigningKey.from_seed(b"j" * 32)
        substituted_key = EvalTrustedKey(
            key_id=signing_key.key_id,
            public_key_hex=other_key.public_key_hex,
            algorithm=EVAL_SIGNING_ALGORITHM,
            scope=EVAL_TRUST_SCOPE,
            valid_from=_TS,
            valid_until=_TS_PLUS,
            revoked=False,
            source="protocol-owned-test-metadata",
        )
        trust_store = EvalTrustStore(keys=[substituted_key])
        report = verify_bundle(bundle_dir, trust_store=trust_store)
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_SUBSTITUTED)

    def test_empty_trust_store_detected(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        empty_trust_store = EvalTrustStore(keys=[])
        report = verify_bundle(bundle_dir, trust_store=empty_trust_store)
        _assert_failure(report, VerificationLayer.SIGNATURES_TRUST, VerificationFailureCode.SIGNATURE_UNKNOWN_KEY)


# ---------------------------------------------------------------------------
# Valid bundle passes all layers (control)
# ---------------------------------------------------------------------------


class TestValidBundlePasses:
    """A valid signed bundle passes all twelve layers."""

    def test_valid_bundle_ok(self, tmp_path: Path) -> None:
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_bundle(tmp_path, signing_key=signing_key)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        assert report.ok, f"Valid bundle should be ok: {[(f.layer.name, f.code.value, f.message) for f in report.failures]}"
        assert len(report.layers) == 12
        for layer_result in report.layers:
            assert layer_result.passed, f"Layer {layer_result.layer.name} should pass"

    def test_valid_rich_bundle_ok(self, tmp_path: Path) -> None:
        """A valid signed bundle with all protected record classes passes all twelve layers."""
        signing_key = EvalSigningKey.from_seed(_SEED)
        bundle_dir = _produce_valid_rich_bundle(tmp_path, signing_key=signing_key)
        report = verify_bundle(bundle_dir, trust_store=_trust_store(signing_key))
        assert report.ok, f"Valid rich bundle should be ok: {[(f.layer.name, f.code.value, f.message) for f in report.failures]}"
        assert len(report.layers) == 12
        for layer_result in report.layers:
            assert layer_result.passed, f"Layer {layer_result.layer.name} should pass"
