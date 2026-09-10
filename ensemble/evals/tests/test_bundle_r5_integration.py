# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""R5.1 integration test: production-shaped campaign bundle with nested encrypted evidence.

This test creates a production-shaped campaign report directory containing
nested encrypted evidence files beneath ``evidence/<attempt_id>/``, runs the
public ``bundle`` CLI command with a signing key and trust store, verifies the
produced bundle with ``verify_bundle``, and resolves every
``EvidenceIndex.storage_location`` to an included bundle artifact or an
authenticated external reference.

The test is intentionally failing until R5.2-R5.9 land:

- R5.2: recursive inventory picks up ``evidence/<attempt_id>/provider-request.json.enc``.
- R5.3: nested evidence files are classified by normalized relative path, not
  basename; unknown files fail closed.
- R5.4: staging and atomic finalize write the nested bytes exactly once.
- R5.7: restricted evidence carries authenticated ciphertext, not blanket
  encryption metadata.
- R5.8: ``evidence-index.jsonl`` is parsed with the typed ``EvidenceIndex``
  model; malformed lines fail.
- R5.9: EvidenceIndex semantic verification resolves ``storage_location`` to
  an included artifact or external reference and checks ciphertext digest/length
  closure.
"""

from __future__ import annotations

import hashlib
import os
from datetime import UTC, datetime
from pathlib import Path

import pytest

pytestmark = pytest.mark.integration

from g8e_evals import constants as evals_constants
from g8e_evals.analysis import canonical_model_json, compute_canonical_analysis_from_record
from g8e_evals.analysis.canonical import (
    ClaimPolicy,
    ContinuousTestPolicy,
    PreregistrationConfig,
)
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.arms import Arm
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
from g8e_evals.campaign import (
    CampaignAssignment,
    CampaignManifest,
    ExecutionSchedule,
    InitialStateAssignmentManifest,
    ModelCohort,
    RetryPolicy,
    RoleModelBinding,
    SamplingSettings,
    TaskAssignmentManifest,
    compute_campaign_manifest_hash,
    compute_initial_state_hash,
    compute_model_cohort_hash,
    compute_retry_policy_hash,
    compute_schedule_hash,
    compute_task_assignment_hash,
)
from g8e_evals.constants import (
    ANALYSIS_HTML,
    ANALYSIS_INPUT_JSON,
    ANALYSIS_JSON,
    ANALYSIS_MD,
    ANALYSIS_TXT,
    ATTEMPTS_JSONL,
    CAMPAIGN_ASSIGNMENTS_JSONL,
    CAMPAIGN_COHORTS_JSONL,
    CAMPAIGN_MANIFEST_JSON,
    CAMPAIGN_RETRY_POLICY_JSON,
    CAMPAIGN_SCHEDULE_JSON,
    EVIDENCE_INDEX_JSONL,
    MANIFEST_JSON,
    METRICS_JSONL,
    RECEIPTS_JSONL,
    STAGES_JSONL,
    TASKS_JSONL,
)
from g8e_evals.schema import (
    ArmManifestEntry,
    AttemptRecord,
    ContentHash,
    EvidenceAccessControl,
    EvidenceAccessPolicy,
    EvidenceEncryption,
    EvidenceEncryptionAlgorithm,
    EvidenceIndex,
    EvidenceMediaType,
    GraderClass,
    GraderReference,
    MetricObservation,
    ModelIdentity,
    PostureObservation,
    PrivacyClassification,
    ReceiptObservation,
    RoleToModelMapping,
    RunManifest,
    StageKind,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)
from g8e_evals.schema import ActionReceipt
from g8e.operator.v1.operator_pb2 import (
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)
from g8e_evals.arms import GovernancePosture


_TS = datetime(2026, 1, 1, tzinfo=UTC)
_TS_PLUS = datetime(2026, 12, 31, tzinfo=UTC)
_SEED = b"k" * 32

_CAMPAIGN_ID = "v2.1.8-ifeval-pipeline-integrity"
_RELEASE_VERSION = "v2.1.8"
_RUN_ID = "run-r5-test"
_TASK_IDS = ["task-1001", "task-1019"]
_COHORT_IDS = ["cohort-qwen3-8b", "cohort-granite-3.3-8b"]
_ARM_IDS = ["direct", "ensemble_ungoverned"]
_REPLICATE_IDS = ["replicate-1", "replicate-2"]
_INITIAL_STATE_ID = "no-initial-state-v1"
_DATASET_HASH = "5eee4bb145007b67e3fe38899fc18a49a8b29b1d6ad844c76a160795bc9b6d37"
_NO_STATE_HASH = "0" * 64


# ---------------------------------------------------------------------------
# Campaign contract helpers
# ---------------------------------------------------------------------------


def _make_sampling() -> SamplingSettings:
    return SamplingSettings(temperature=0.0, top_p=1.0, max_tokens=4096, seed=42)


def _make_role_binding(role: str = "primary", model_id: str = "qwen3:8b") -> RoleModelBinding:
    return RoleModelBinding(
        role=role,
        model_id=model_id,
        provider="ollama",
        endpoint="http://192.168.1.2:11434",
        sampling_settings=_make_sampling(),
        timeout_seconds=120.0,
        seed_capable=True,
    )


def _make_cohort(cohort_id: str, model_id: str) -> ModelCohort:
    bindings = [_make_role_binding("primary", model_id)]
    ch = compute_model_cohort_hash(cohort_id, bindings)
    return ModelCohort(cohort_id=cohort_id, role_bindings=bindings, content_hash=ch)


def _make_task_assignment() -> TaskAssignmentManifest:
    ch = compute_task_assignment_hash("task-assignment-v1", "ifeval_subset", _DATASET_HASH, _TASK_IDS)
    return TaskAssignmentManifest(
        task_assignment_id="task-assignment-v1",
        suite_id="ifeval_subset",
        dataset_hash=_DATASET_HASH,
        task_ids=_TASK_IDS,
        content_hash=ch,
    )


def _make_initial_state() -> InitialStateAssignmentManifest:
    ch = compute_initial_state_hash(_INITIAL_STATE_ID, "no_initial_state", _NO_STATE_HASH)
    return InitialStateAssignmentManifest(
        initial_state_assignment_id=_INITIAL_STATE_ID,
        state_type="no_initial_state",
        snapshot_hash=_NO_STATE_HASH,
        content_hash=ch,
    )


def _make_preregistration() -> PreregistrationConfig:
    return PreregistrationConfig(
        config_id="test-config-1",
        config_version="1.0.0",
        baseline_arm_id=_ARM_IDS[0],
        comparison_arm_ids=_ARM_IDS[1:],
        model_cohort_ids=_COHORT_IDS,
        task_assignment_id="task-assignment-v1",
        initial_state_assignment_id=_INITIAL_STATE_ID,
        required_replicate_ids=_REPLICATE_IDS,
        required_replicate_count=len(_REPLICATE_IDS),
        primary_metric_ids=["ifeval_subset_verifier"],
        continuous_test_policy=ContinuousTestPolicy.PAIRED_T,
        bootstrap_count=10000,
        bootstrap_confidence=0.95,
        bootstrap_seed=0,
        significance_level=0.05,
        claim_policy=ClaimPolicy.DESCRIPTIVE_ONLY,
    )


def _make_campaign_assignments() -> list[CampaignAssignment]:
    """Build one CampaignAssignment per Cartesian slot."""
    assignments: list[CampaignAssignment] = []
    position = 0
    for task_id in _TASK_IDS:
        for cohort_id in _COHORT_IDS:
            for arm_id in _ARM_IDS:
                for replicate_id in _REPLICATE_IDS:
                    aid = hashlib.sha256(
                        f"{_CAMPAIGN_ID}:{task_id}:{cohort_id}:{arm_id}:{_INITIAL_STATE_ID}:{replicate_id}".encode()
                    ).hexdigest()[:16]
                    assignments.append(CampaignAssignment(
                        campaign_id=_CAMPAIGN_ID,
                        assignment_id=aid,
                        task_id=task_id,
                        model_cohort_id=cohort_id,
                        arm_id=arm_id,
                        initial_state_assignment_id=_INITIAL_STATE_ID,
                        replicate_id=replicate_id,
                        schedule_position=position,
                    ))
                    position += 1
    return assignments


def _make_schedule(assignments: list[CampaignAssignment]) -> ExecutionSchedule:
    ordered_ids = [a.assignment_id for a in assignments]
    schedule_id = hashlib.sha256(f"{_CAMPAIGN_ID}:42".encode()).hexdigest()[:16]
    ch = compute_schedule_hash(schedule_id, _CAMPAIGN_ID, 42, "fisher_yates_shuffle", "1.0.0", ordered_ids)
    return ExecutionSchedule(
        schedule_id=schedule_id,
        campaign_id=_CAMPAIGN_ID,
        randomization_seed=42,
        algorithm_id="fisher_yates_shuffle",
        algorithm_version="1.0.0",
        ordered_assignment_ids=ordered_ids,
        content_hash=ch,
    )


def _make_retry_policy() -> RetryPolicy:
    return RetryPolicy(max_retries=1, retryable_terminal_statuses=["infrastructure_failed"])


def _make_campaign_manifest(
    schedule: ExecutionSchedule,
    prereg: PreregistrationConfig,
    cohorts: list[ModelCohort],
    retry_policy: RetryPolicy,
) -> CampaignManifest:
    prereg_hash = hashlib.sha256(canonical_model_json(prereg).encode()).hexdigest()
    cohort_hashes = sorted(c.content_hash for c in cohorts)
    retry_hash = compute_retry_policy_hash(retry_policy.max_retries, retry_policy.retryable_terminal_statuses)
    metric_registry_hash = hashlib.sha256(b"default_metric_registry_v1").hexdigest()
    release_metric_set_hash = hashlib.sha256(b"release_metric_set_v1").hexdigest()
    threshold_authority_hash = hashlib.sha256(b"descriptive_only_no_threshold").hexdigest()
    missingness_authority_hash = hashlib.sha256(b"assignment_level_missingness").hexdigest()
    provider_budget_hash = hashlib.sha256(b"no_budget").hexdigest()
    source_build_provenance_hash = hashlib.sha256(b"no_provenance").hexdigest()
    claim_exclusion_hash = hashlib.sha256(b"descriptive_only_claim_exclusion").hexdigest()
    ch = compute_campaign_manifest_hash(
        _CAMPAIGN_ID,
        "1.0.0",
        _RELEASE_VERSION,
        prereg_hash,
        cohort_hashes,
        _make_task_assignment().content_hash,
        _make_initial_state().content_hash,
        schedule.content_hash,
        retry_hash,
        metric_registry_hash,
        release_metric_set_hash,
        threshold_authority_hash,
        missingness_authority_hash,
        provider_budget_hash,
        source_build_provenance_hash,
        claim_exclusion_hash,
    )
    return CampaignManifest(
        campaign_id=_CAMPAIGN_ID,
        campaign_version="1.0.0",
        release_version=_RELEASE_VERSION,
        preregistration_hash=prereg_hash,
        cohort_hashes=cohort_hashes,
        task_assignment_hash=_make_task_assignment().content_hash,
        initial_state_assignment_hash=_make_initial_state().content_hash,
        schedule_hash=schedule.content_hash,
        retry_policy_hash=retry_hash,
        metric_registry_hash=metric_registry_hash,
        release_metric_set_hash=release_metric_set_hash,
        threshold_authority_hash=threshold_authority_hash,
        missingness_authority_hash=missingness_authority_hash,
        provider_budget_hash=provider_budget_hash,
        source_build_provenance_hash=source_build_provenance_hash,
        claim_exclusion_hash=claim_exclusion_hash,
        content_hash=ch,
    )


# ---------------------------------------------------------------------------
# Report directory builder
# ---------------------------------------------------------------------------


def _make_task_def(task_id: str) -> TaskDefinition:
    return TaskDefinition(
        task_id=task_id,
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        category="instruction_following",
        prompt_hash=hashlib.sha256(f"Prompt for {task_id}".encode()).hexdigest(),
        prompt_length=len(f"Prompt for {task_id}"),
        graders=[
            GraderReference(
                grader_id="ifeval_subset_verifier",
                grader_version="1.0.0",
                grader_class=GraderClass.DETERMINISTIC,
            ),
        ],
        metadata={"instruction_id_list": ["punctuation:no_comma"]},
    )


def _make_attempt(assignment: CampaignAssignment, attempt_num: int = 0) -> AttemptRecord:
    attempt_id = f"{_RUN_ID}:{assignment.assignment_id}:{attempt_num}"
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=assignment.task_id,
        arm_id=Arm(assignment.arm_id),
        model_cohort_id=assignment.model_cohort_id,
        state_snapshot_hash=_NO_STATE_HASH,
        replicate_id=assignment.replicate_id,
        assignment_id=assignment.assignment_id,
        assignment_order=assignment.schedule_position,
        started_at=_TS,
        ended_at=_TS,
        terminal_status=TerminalStatus.COMPLETED,
        posture=PostureObservation(requested_posture=GovernancePosture.NONE),
    )


def _make_metric(attempt: AttemptRecord) -> MetricObservation:
    return MetricObservation(
        metric_id="ifeval_subset_verifier",
        metric_version="1.0.0",
        attempt_id=attempt.attempt_id,
        run_id=_RUN_ID,
        task_id=attempt.task_id,
        arm_id=attempt.arm_id,
        value=1.0,
        unit="boolean",
        verification_status=VerificationStatus.VERIFIED,
        grader_class=GraderClass.DETERMINISTIC,
    )


def _make_receipt(attempt: AttemptRecord) -> ReceiptObservation:
    receipt = ActionReceipt(
        transaction_id=f"tx-{attempt.attempt_id}",
        transaction_hash=f"hash-{attempt.attempt_id}",
    )
    receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="IFEVAL_TASK",
    )
    return ReceiptObservation(
        receipt_id=f"receipt-{attempt.attempt_id}",
        attempt_id=attempt.attempt_id,
        run_id=_RUN_ID,
        transaction_id=f"tx-{attempt.attempt_id}",
        action_type="IFEVAL_TASK",
        primary=True,
        action_receipt=receipt,
    )


def _make_stage(attempt: AttemptRecord) -> StageObservation:
    return StageObservation(
        stage_id=f"stage-{attempt.attempt_id}",
        attempt_id=attempt.attempt_id,
        run_id=_RUN_ID,
        task_id=attempt.task_id,
        kind=StageKind.PROTOCOL_L2,
        decision="pass",
    )


def _make_evidence_index(attempt: AttemptRecord, ciphertext: bytes) -> EvidenceIndex:
    """Build an EvidenceIndex for one attempt with nested encrypted evidence."""
    ct_hash = hashlib.sha256(ciphertext).hexdigest()
    return EvidenceIndex(
        artifact_id=f"evidence-{attempt.attempt_id}",
        run_id=_RUN_ID,
        attempt_id=attempt.attempt_id,
        media_type=EvidenceMediaType.APPLICATION_OCTET_STREAM,
        schema_ref="provider-request/v1",
        byte_length=len(ciphertext),
        sha256=ct_hash,
        producer_identity="ollama-adapter",
        privacy_classification=PrivacyClassification.RESTRICTED,
        storage_location=f"evidence/{attempt.attempt_id}/provider-request.json.enc",
        encryption=EvidenceEncryption(
            algorithm=EvidenceEncryptionAlgorithm.AES_256_GCM,
            key_id="evidence-key-1",
            aad_sha256=hashlib.sha256(b"aad").hexdigest(),
            ciphertext_sha256=ct_hash,
            ciphertext_byte_length=len(ciphertext),
        ),
        access_control=EvidenceAccessControl(
            policy=EvidenceAccessPolicy.NAMED_KEY_HOLDERS,
            authorization_scope="eval-release-operator",
        ),
    )


def _build_campaign_report(report_dir: Path) -> tuple[list[EvidenceIndex], list[AttemptRecord]]:
    """Build a production-shaped campaign report directory with nested encrypted evidence.

    Returns the list of EvidenceIndex records and the list of AttemptRecords
    so the test can assert against them.
    """
    report_dir.mkdir(parents=True, exist_ok=True)

    cohorts = [_make_cohort(c, m) for c, m in zip(_COHORT_IDS, ["qwen3:8b", "granite3.3:8b"], strict=True)]
    prereg = _make_preregistration()
    assignments = _make_campaign_assignments()
    schedule = _make_schedule(assignments)
    retry_policy = _make_retry_policy()
    manifest = _make_campaign_manifest(schedule, prereg, cohorts, retry_policy)

    task_defs = [_make_task_def(tid) for tid in _TASK_IDS]
    attempts = [_make_attempt(a) for a in assignments]
    metrics = [_make_metric(a) for a in attempts]
    receipts = [_make_receipt(a) for a in attempts]
    stages = [_make_stage(a) for a in attempts]

    # Build nested encrypted evidence files and EvidenceIndex records.
    evidence_indices: list[EvidenceIndex] = []
    for attempt in attempts:
        ciphertext = os.urandom(64)
        evidence_dir = report_dir / "evidence" / attempt.attempt_id
        evidence_dir.mkdir(parents=True, exist_ok=True)
        enc_path = evidence_dir / "provider-request.json.enc"
        enc_path.write_bytes(ciphertext)
        evidence_indices.append(_make_evidence_index(attempt, ciphertext))

    # Write run manifest.
    run_manifest = RunManifest(
        run_id=_RUN_ID,
        suite_id="ifeval_subset",
        suite_version="1.0.0",
        orchestrator_version=_RELEASE_VERSION,
        content_hashes=[
            ContentHash(name="dataset", sha256=_DATASET_HASH),
            ContentHash(name="prompt_bundle", sha256=hashlib.sha256(b"prompts").hexdigest()),
            ContentHash(name="grader_bundle", sha256="g" * 64),
        ],
        arms=[
            ArmManifestEntry(
                arm_id=Arm.DOCTRINE,
                requested_posture=GovernancePosture.NONE,
                uses_g8ee=False,
                uses_gateway=False,
                receipt_binding=False,
                is_production_posture=False,
            ),
            ArmManifestEntry(
                arm_id=Arm.ENSEMBLE_UNGOVERNED,
                requested_posture=GovernancePosture.NONE,
                uses_g8ee=True,
                uses_gateway=False,
                receipt_binding=False,
                is_production_posture=False,
            ),
        ],
        role_to_model=RoleToModelMapping(
            primary=ModelIdentity(
                role="primary",
                provider="ollama",
                model="qwen3:8b",
                endpoint="http://192.168.1.2:11434",
                endpoint_class="local",
            ),
        ),
    )
    (report_dir / MANIFEST_JSON).write_text(run_manifest.model_dump_json(indent=2))

    # Write task definitions.
    (report_dir / TASKS_JSONL).write_text(
        "\n".join(canonical_model_json(t) for t in task_defs) + "\n"
    )

    # Write attempts.
    (report_dir / ATTEMPTS_JSONL).write_text(
        "\n".join(canonical_model_json(a) for a in attempts) + "\n"
    )

    # Write metrics.
    (report_dir / METRICS_JSONL).write_text(
        "\n".join(canonical_model_json(m) for m in metrics) + "\n"
    )

    # Write receipts.
    (report_dir / RECEIPTS_JSONL).write_text(
        "\n".join(canonical_model_json(r) for r in receipts) + "\n"
    )

    # Write stages.
    (report_dir / STAGES_JSONL).write_text(
        "\n".join(canonical_model_json(s) for s in stages) + "\n"
    )

    # Write evidence index.
    (report_dir / EVIDENCE_INDEX_JSONL).write_text(
        "\n".join(canonical_model_json(e) for e in evidence_indices) + "\n"
    )

    # Write campaign contract files.
    (report_dir / CAMPAIGN_MANIFEST_JSON).write_text(manifest.model_dump_json(indent=2))
    (report_dir / CAMPAIGN_ASSIGNMENTS_JSONL).write_text(
        "\n".join(a.model_dump_json() for a in assignments) + "\n"
    )
    (report_dir / CAMPAIGN_COHORTS_JSONL).write_text(
        "\n".join(c.model_dump_json() for c in cohorts) + "\n"
    )
    (report_dir / CAMPAIGN_SCHEDULE_JSON).write_text(schedule.model_dump_json(indent=2))
    (report_dir / CAMPAIGN_RETRY_POLICY_JSON).write_text(retry_policy.model_dump_json(indent=2))

    # Write analysis input and analysis artifacts.
    analysis_input = AnalysisInputRecord(
        run_id=_RUN_ID,
        release_version=_RELEASE_VERSION,
        tasks=task_defs,
        attempts=attempts,
        metric_observations=metrics,
        receipts=receipts,
        stages=stages,
        preregistration=prereg,
        campaign_manifest=manifest,
        model_cohorts=cohorts,
        campaign_assignments=assignments,
        execution_schedule=schedule,
        retry_policy=retry_policy,
    )
    (report_dir / ANALYSIS_INPUT_JSON).write_text(canonical_model_json(analysis_input))

    from g8e_evals.analysis import render_cli, render_html, render_markdown
    analysis = compute_canonical_analysis_from_record(analysis_input)
    (report_dir / ANALYSIS_JSON).write_text(canonical_model_json(analysis))
    (report_dir / ANALYSIS_MD).write_text(render_markdown(analysis))
    (report_dir / ANALYSIS_HTML).write_text(render_html(analysis))
    (report_dir / ANALYSIS_TXT).write_text(render_cli(analysis))

    return evidence_indices, attempts


# ---------------------------------------------------------------------------
# Trust store helpers
# ---------------------------------------------------------------------------


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


def _write_signing_seed(tmp_path: Path) -> Path:
    """Write a 32-byte signing seed to a temp file and return its path."""
    seed_path = tmp_path / "signing-key.seed"
    seed_path.write_bytes(_SEED)
    return seed_path


def _write_trust_store(tmp_path: Path, signing_key: EvalSigningKey) -> Path:
    """Write an EvalTrustStore JSON file and return its path."""
    ts = _trust_store(signing_key)
    ts_path = tmp_path / "trust-store.json"
    ts_path.write_text(canonical_model_json(ts))
    return ts_path


# ---------------------------------------------------------------------------
# R5.1: Production-shaped campaign bundle with nested encrypted evidence
# ---------------------------------------------------------------------------


class TestProductionShapedCampaignBundle:
    """R5.1: A production-shaped campaign report with nested encrypted evidence
    passes complete offline verification and every evidence-index storage
    location resolves to an included artifact or authenticated external
    reference.

    This test fails until R5.2-R5.9 land because:

    - ``produce_bundle`` reads only flat files via ``report_dir.iterdir()``
      and does not pick up nested evidence files (R5.2).
    - Campaign artifact filenames are not in ``_ARTIFACT_TYPE_MAP`` (R5.3).
    - ``EvidenceIndex`` semantic verification does not exist (R5.9).
    """

    def test_nested_evidence_included_and_verified(self, tmp_path: Path) -> None:
        report_dir = tmp_path / "report"
        bundle_dir = tmp_path / "bundle"

        evidence_indices, _attempts = _build_campaign_report(report_dir)

        signing_key = EvalSigningKey.from_seed(_SEED)
        _write_signing_seed(tmp_path)
        _write_trust_store(tmp_path, signing_key)

        # Produce the bundle via the public produce_bundle function.
        # The CLI command calls this function; we call it directly to
        # avoid CliRunner overhead while exercising the same code path.
        manifest = produce_bundle(
            report_dir=report_dir,
            bundle_dir=bundle_dir,
            bundle_id="bundle-r5-test",
            signing_key=signing_key,
            created_at=_TS,
        )

        # Verify the bundle with the external trust store.
        trust_store = _trust_store(signing_key)
        report = verify_bundle(bundle_dir, trust_store=trust_store)

        # The bundle must pass complete verification.
        assert report.ok, (
            f"Bundle verification failed. Failures: "
            f"{[(f.layer.name, f.code.value, f.message) for f in report.failures]}"
        )

        # The bundle manifest must include the nested evidence files.
        manifest_artifact_paths = {e.path for e in manifest.artifacts}
        for ei in evidence_indices:
            assert ei.storage_location in manifest_artifact_paths, (
                f"Nested evidence file {ei.storage_location} is not in the bundle manifest. "
                f"Manifest paths: {sorted(manifest_artifact_paths)}"
            )

        # Every EvidenceIndex.storage_location must resolve to an included
        # bundle artifact (nested path beneath the bundle root) or an
        # ExternalReference in the manifest with a content-addressed SHA-256.
        external_refs = {r.reference_id: r for r in manifest.external_references}
        for ei in evidence_indices:
            loc = ei.storage_location
            if loc in manifest_artifact_paths:
                # The storage location is an included artifact. Verify the
                # artifact entry has the correct privacy class and encryption.
                entry = next(e for e in manifest.artifacts if e.path == loc)
                assert entry.privacy_class == PrivacyClass.RESTRICTED, (
                    f"Evidence artifact {loc} must be RESTRICTED, got {entry.privacy_class}"
                )
                assert entry.encryption is not None, (
                    f"RESTRICTED evidence artifact {loc} must carry encryption metadata"
                )
                assert ei.encryption is not None, (
                    f"RESTRICTED evidence index record {ei.artifact_id} must carry encryption metadata"
                )
                # Verify ciphertext digest and length closure against the
                # actual file bytes in the bundle.
                actual_bytes = (bundle_dir / loc).read_bytes()
                actual_hash = hashlib.sha256(actual_bytes).hexdigest()
                assert actual_hash == ei.encryption.ciphertext_sha256, (
                    f"Ciphertext hash mismatch for {loc}: "
                    f"evidence-index={ei.encryption.ciphertext_sha256} "
                    f"actual={actual_hash}"
                )
                assert len(actual_bytes) == ei.encryption.ciphertext_byte_length, (
                    f"Ciphertext byte length mismatch for {loc}: "
                    f"evidence-index={ei.encryption.ciphertext_byte_length} "
                    f"actual={len(actual_bytes)}"
                )
            else:
                # The storage location must be an external reference.
                assert loc in external_refs, (
                    f"Storage location {loc} is neither an included artifact "
                    f"nor an external reference"
                )
                ref = external_refs[loc]
                assert ref.content_sha256, (
                    f"External reference {loc} must be content-addressed"
                )

    def test_campaign_artifacts_classified_not_defaulted_to_diagnostic(self, tmp_path: Path) -> None:
        """Campaign artifact files must be classified by their typed report
        contract, not defaulted to DIAGNOSTIC/INTERNAL.

        Fails until R5.3 adds campaign artifact filenames to the
        ``_ARTIFACT_TYPE_MAP``.
        """
        report_dir = tmp_path / "report"
        bundle_dir = tmp_path / "bundle"

        _build_campaign_report(report_dir)

        signing_key = EvalSigningKey.from_seed(_SEED)
        manifest = produce_bundle(
            report_dir=report_dir,
            bundle_dir=bundle_dir,
            bundle_id="bundle-r5-classification",
            signing_key=signing_key,
            created_at=_TS,
        )

        from g8e_evals.bundle.manifest import ArtifactType
        artifact_by_path = {e.path: e for e in manifest.artifacts}

        # Campaign manifest must be classified as PREREGISTRATION_MANIFEST,
        # not DIAGNOSTIC/INTERNAL.
        campaign_manifest_entry = artifact_by_path.get(CAMPAIGN_MANIFEST_JSON)
        assert campaign_manifest_entry is not None, (
            f"{CAMPAIGN_MANIFEST_JSON} not in bundle manifest"
        )
        assert campaign_manifest_entry.artifact_type != ArtifactType.DIAGNOSTIC, (
            f"{CAMPAIGN_MANIFEST_JSON} must not be classified as DIAGNOSTIC"
        )

        # Campaign assignments must not be DIAGNOSTIC/INTERNAL.
        campaign_assignments_entry = artifact_by_path.get(CAMPAIGN_ASSIGNMENTS_JSONL)
        assert campaign_assignments_entry is not None, (
            f"{CAMPAIGN_ASSIGNMENTS_JSONL} not in bundle manifest"
        )
        assert campaign_assignments_entry.artifact_type != ArtifactType.DIAGNOSTIC, (
            f"{CAMPAIGN_ASSIGNMENTS_JSONL} must not be classified as DIAGNOSTIC"
        )

    def test_unknown_file_fails_closed(self, tmp_path: Path) -> None:
        """An unknown file in the report directory must fail closed, not
        be silently included as DIAGNOSTIC/INTERNAL.

        Fails until R5.3 makes unknown files fail closed.
        """
        report_dir = tmp_path / "report"
        bundle_dir = tmp_path / "bundle"

        _build_campaign_report(report_dir)

        # Add an unknown file that is not part of any report contract.
        (report_dir / "unknown-mystery-file.txt").write_text("not a known artifact")

        signing_key = EvalSigningKey.from_seed(_SEED)

        # produce_bundle must reject the unknown file, not silently include it.
        with pytest.raises((ValueError, RuntimeError), match="unknown"):
            produce_bundle(
                report_dir=report_dir,
                bundle_dir=bundle_dir,
                bundle_id="bundle-r5-unknown",
                signing_key=signing_key,
                created_at=_TS,
            )

    def test_evidence_index_storage_location_resolves(self, tmp_path: Path) -> None:
        """Every EvidenceIndex.storage_location in the bundle resolves to an
        included artifact or an authenticated external reference.

        Fails until R5.9 adds EvidenceIndex semantic verification.
        """
        report_dir = tmp_path / "report"
        bundle_dir = tmp_path / "bundle"

        evidence_indices, _ = _build_campaign_report(report_dir)

        signing_key = EvalSigningKey.from_seed(_SEED)
        produce_bundle(
            report_dir=report_dir,
            bundle_dir=bundle_dir,
            bundle_id="bundle-r5-evidence-resolve",
            signing_key=signing_key,
            created_at=_TS,
        )

        trust_store = _trust_store(signing_key)
        _report = verify_bundle(bundle_dir, trust_store=trust_store)

        # The verification report must not contain evidence-index-specific
        # failures. Until R5.9 lands, there is no evidence-index semantic
        # verification layer, so the test asserts that the report either
        # passes or fails for unrelated reasons — but the evidence index
        # storage locations must all resolve.
        manifest = BundleManifest.model_validate_json(
            (bundle_dir / evals_constants.BUNDLE_MANIFEST_JSON).read_text()
        )
        manifest_paths = {e.path for e in manifest.artifacts}
        external_refs = {r.reference_id for r in manifest.external_references}

        for ei in evidence_indices:
            loc = ei.storage_location
            assert loc in manifest_paths or loc in external_refs, (
                f"EvidenceIndex storage_location {loc} does not resolve to "
                f"an included artifact or external reference"
            )
