# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the canonical analytical data model.

Verifies schema validation, extra-field rejection, hash property
resolution, and the logical key structure of attempt records.
"""

from __future__ import annotations

from datetime import UTC, datetime

import pytest
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)
from pydantic import ValidationError

from g8e_evals.arms import Arm, GovernancePosture
from g8e_evals.schema import (
    SCHEMA_VERSION,
    AttemptRecord,
    ArmManifestEntry,
    ArtifactCollectionBoundary,
    ArtifactLeakageAssertion,
    ArtifactLeakageObservation,
    CanaryScrubbingAssertion,
    ContentHash,
    EvidenceIndex,
    EvidenceMediaType,
    ExfiltrationAttemptAssertion,
    ExfiltrationAttemptObservation,
    IdentityBinding,
    IdentityMismatchAssertion,
    IdentityMismatchObservation,
    FinalStateAssertion,
    FinalStateObservation,
    MetricObservation,
    ModelBoundaryPrivacyAttestation,
    ModelIdentity,
    PolicyOutcome,
    PostureObservation,
    PayloadTamperingAssertion,
    PayloadTamperingObservation,
    ReceiptObservation,
    RehydrationAssertion,
    RehydrationBoundary,
    RehydrationObservation,
    RejectionLayer,
    ReplayAttemptAssertion,
    ReplayAttemptObservation,
    RunManifest,
    SecretDetectionAssertion,
    SecretDetectionObservation,
    SensitiveArtifactContentType,
    SignedField,
    SignedFieldTamperingAssertion,
    SignedFieldTamperingObservation,
    StaleStateRootAssertion,
    StaleStateRootObservation,
    StateAssertion,
    StateAssertionPredicate,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateFixtureDefinition,
    StateObservation,
    StateValue,
    StageKind,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    TokenStorePersistenceAssertion,
    TokenStorePersistenceObservation,
    TokenTTLExpiryAssertion,
    TokenTTLExpiryObservation,
    TokenPersistenceFailureAssertion,
    TokenPersistenceFailureObservation,
    TokenPersistenceFailureOutcome,
    UsageReconciliation,
    VerificationStatus,
)

pytestmark = pytest.mark.unit


def test_schema_version_is_pinned():
    assert SCHEMA_VERSION == "1.26.0"


def test_rehydration_assertion_and_observation_round_trip():
    assertion = RehydrationAssertion(
        assertion_id="rehydration-fixture-1",
        source="assistant_response",
        input_artifact_sha256="a" * 64,
        expected_output_artifact_sha256="b" * 64,
        expected_token_count=2,
        expected_sensitive_types=["email", "api_key"],
    )
    observation = RehydrationObservation(
        observation_id="rehydration-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        source=assertion.source,
        input_artifact_sha256=assertion.input_artifact_sha256,
        output_artifact_sha256=assertion.expected_output_artifact_sha256,
        rehydrator_version="sentinel-rehydrator@1.0.0",
        execution_boundary=RehydrationBoundary.LOCAL_RUNTIME,
        collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
        restored_token_count=2,
        unresolved_token_count=0,
        restored_sensitive_types=["email", "api_key"],
        source_evidence_refs=["restricted-rehydration-evidence"],
        source_evidence_sha256="c" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored = RehydrationObservation.model_validate_json(observation.model_dump_json())

    assert restored == observation


@pytest.mark.parametrize(
    "update",
    [
        {"expected_sensitive_types": ["email", "email"]},
        {"expected_token_count": 0},
    ],
)
def test_rehydration_assertion_rejects_malformed_ground_truth(update: dict[str, object]):
    values: dict[str, object] = {
        "assertion_id": "rehydration-fixture-1",
        "source": "assistant_response",
        "input_artifact_sha256": "a" * 64,
        "expected_output_artifact_sha256": "b" * 64,
        "expected_token_count": 1,
        "expected_sensitive_types": ["email"],
    }
    values.update(update)

    with pytest.raises(ValidationError):
        RehydrationAssertion.model_validate(values)


def test_verified_rehydration_observation_requires_source_evidence():
    with pytest.raises(ValidationError):
        RehydrationObservation(
            observation_id="rehydration-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="rehydration-fixture-1",
            source="assistant_response",
            input_artifact_sha256="a" * 64,
            output_artifact_sha256="b" * 64,
            rehydrator_version="sentinel-rehydrator@1.0.0",
            execution_boundary=RehydrationBoundary.LOCAL_RUNTIME,
            collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
            restored_token_count=1,
            unresolved_token_count=0,
            restored_sensitive_types=["email"],
            verification_status=VerificationStatus.VERIFIED,
        )


def test_rehydration_observation_rejects_duplicate_source_evidence_references():
    with pytest.raises(ValidationError, match="rehydration source evidence references must be unique"):
        RehydrationObservation(
            observation_id="rehydration-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="rehydration-fixture-1",
            source="assistant_response",
            input_artifact_sha256="a" * 64,
            output_artifact_sha256="b" * 64,
            rehydrator_version="sentinel-rehydrator@1.0.0",
            execution_boundary=RehydrationBoundary.LOCAL_RUNTIME,
            collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
            restored_token_count=1,
            unresolved_token_count=0,
            restored_sensitive_types=["email"],
            source_evidence_refs=["restricted-evidence", "restricted-evidence"],
            source_evidence_sha256="c" * 64,
            verification_status=VerificationStatus.VERIFIED,
        )


def test_secret_detection_assertion_and_observation_round_trip():
    assertion = SecretDetectionAssertion(
        assertion_id="scanner-fixture-1",
        source="conversation_history:user",
        input_artifact_sha256="a" * 64,
        expected_sensitive_occurrences=2,
        expected_benign_occurrences=1,
        expected_sensitive_types=["email", "api_key"],
    )
    observation = SecretDetectionObservation(
        observation_id="scanner-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        source=assertion.source,
        input_artifact_sha256=assertion.input_artifact_sha256,
        scanner_version="sentinel-regex@1.0.0",
        collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
        true_positive_count=1,
        false_positive_count=1,
        false_negative_count=1,
        true_negative_count=0,
        detected_sensitive_types=["email"],
        missed_sensitive_types=["api_key"],
        source_evidence_refs=["restricted-evidence-1"],
        source_evidence_sha256="b" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored = SecretDetectionObservation.model_validate_json(observation.model_dump_json())

    assert restored == observation


@pytest.mark.parametrize(
    "update",
    [
        {"expected_sensitive_types": ["email", "email"]},
        {"expected_sensitive_occurrences": 0},
    ],
)
def test_secret_detection_assertion_rejects_malformed_ground_truth(update: dict[str, object]):
    values: dict[str, object] = {
        "assertion_id": "scanner-fixture-1",
        "source": "conversation_history:user",
        "input_artifact_sha256": "a" * 64,
        "expected_sensitive_occurrences": 1,
        "expected_benign_occurrences": 1,
        "expected_sensitive_types": ["email"],
    }
    values.update(update)
    with pytest.raises(ValidationError):
        SecretDetectionAssertion.model_validate(values)


def test_verified_secret_detection_observation_requires_source_evidence():
    with pytest.raises(ValidationError):
        SecretDetectionObservation(
            observation_id="scanner-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="scanner-fixture-1",
            source="conversation_history:user",
            input_artifact_sha256="a" * 64,
            scanner_version="sentinel-regex@1.0.0",
            collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
            true_positive_count=1,
            false_positive_count=0,
            false_negative_count=0,
            true_negative_count=0,
            detected_sensitive_types=["email"],
            verification_status=VerificationStatus.VERIFIED,
        )


def test_model_boundary_privacy_attestation_rejects_unbound_payload_hash():
    with pytest.raises(ValidationError):
        ModelBoundaryPrivacyAttestation(
            scanner_version="sentinel-regex@1.0.0",
            input_artifact_hash="not-a-sha256",
            raw_sensitive_occurrences=0,
        )


def test_usage_reconciliation_flags_mismatched_stage_totals():
    reconciliation = UsageReconciliation(
        reported_input_tokens=20,
        reported_output_tokens=10,
        observed_input_tokens=18,
        observed_output_tokens=10,
        observed_call_count=2,
        expected_call_count=2,
    )

    assert reconciliation.reconciled is False
    assert reconciliation.input_token_delta == -2
    assert reconciliation.output_token_delta == 0


def test_run_manifest_resolves_dataset_hash():
    manifest = RunManifest(
        run_id="r1",
        suite_id="ifeval_subset",
        suite_version="1.0",
        content_hashes=[
            ContentHash(name="dataset", sha256="abc123"),
            ContentHash(name="grader_bundle", sha256="def456"),
            ContentHash(name="prompt_bundle", sha256="ghi789"),
        ],
    )
    assert manifest.dataset_hash == "abc123"
    assert manifest.grader_bundle_hash == "def456"
    assert manifest.prompt_bundle_hash == "ghi789"


def test_run_manifest_hash_properties_return_none_when_absent():
    manifest = RunManifest(run_id="r1", suite_id="s", suite_version="1.0")
    assert manifest.dataset_hash is None
    assert manifest.grader_bundle_hash is None
    assert manifest.prompt_bundle_hash is None


def test_run_manifest_rejects_extra_fields():
    with pytest.raises(ValidationError):
        RunManifest(run_id="r1", suite_id="s", suite_version="1.0", extra_field="bad")  # type: ignore[call-arg]


def test_task_definition_rejects_extra_fields():
    with pytest.raises(ValidationError):
        TaskDefinition(
            task_id="t1",
            suite_id="s",
            suite_version="1.0",
            prompt_hash="h",
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_attempt_record_rejects_extra_fields():
    with pytest.raises(ValidationError):
        AttemptRecord(
            attempt_id="a1",
            run_id="r1",
            task_id="t1",
            arm_id=Arm.DIRECT,
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_final_state_assertion_and_observation_round_trip():
    assertion = FinalStateAssertion(
        assertion_id="file-edit-state-root",
        predicate=StateAssertionPredicate.STATE_ROOT_CHANGED,
        action_type="FILE_EDIT",
    )
    observation = FinalStateObservation(
        observation_id="attempt-1:final-state:file-edit-state-root",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        action_type=assertion.action_type,
        state_root_before="root-before",
        state_root_after="root-after",
        source_receipt_id="receipt-1",
        verification_status=VerificationStatus.VERIFIED,
    )

    restored = FinalStateObservation.model_validate_json(observation.model_dump_json())

    assert restored.assertion_id == assertion.assertion_id
    assert restored.state_root_before == "root-before"
    assert restored.state_root_after == "root-after"
    assert restored.source_receipt_id == "receipt-1"


@pytest.mark.parametrize(
    ("kind", "expected"),
    [
        (
            StateEvidenceKind.FILE,
            StateValue(
                kind=StateEvidenceKind.FILE,
                exists=True,
                content_sha256="a" * 64,
                byte_length=12,
                mode="0640",
            ),
        ),
        (
            StateEvidenceKind.DOCUMENT,
            StateValue(
                kind=StateEvidenceKind.DOCUMENT,
                exists=True,
                content_sha256="b" * 64,
                version="7",
            ),
        ),
        (
            StateEvidenceKind.WORKLOAD_SIDE_EFFECT,
            StateValue(
                kind=StateEvidenceKind.WORKLOAD_SIDE_EFFECT,
                exists=True,
                content_sha256="c" * 64,
                byte_length=4,
            ),
        ),
        (
            StateEvidenceKind.LEDGER_CONSISTENCY,
            StateValue(
                kind=StateEvidenceKind.LEDGER_CONSISTENCY,
                consistent=True,
                entry_count=3,
                head_sha256="d" * 64,
            ),
        ),
    ],
)
def test_state_fixture_and_observation_round_trip_with_typed_values(
    kind: StateEvidenceKind,
    expected: StateValue,
):
    fixture = StateFixtureDefinition(
        fixture_id=f"fixture-{kind.value}",
        fixture_sha256="e" * 64,
        assertions=[StateAssertion(
            assertion_id=f"assertion-{kind.value}",
            action_type="FILE_EDIT",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            target=f"target-{kind.value}",
            expected=expected,
        )],
    )
    observation = StateObservation(
        observation_id=f"observation-{kind.value}",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=fixture.assertions[0].assertion_id,
        action_type="FILE_EDIT",
        fixture_sha256=fixture.fixture_sha256,
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        target=f"target-{kind.value}",
        observed=expected,
        collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
        source_evidence_refs=["evidence-1"],
        source_evidence_sha256="f" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored = StateObservation.model_validate_json(observation.model_dump_json())

    assert restored.observed.kind == kind
    assert restored.fixture_sha256 == fixture.fixture_sha256
    assert restored.collection_boundary == StateCollectionBoundary.OPERATOR_WORKLOAD


def test_state_fixture_rejects_malformed_content_hash():
    with pytest.raises(ValidationError):
        StateFixtureDefinition(
            fixture_id="fixture-1",
            fixture_sha256="not-a-sha256",
            assertions=[StateAssertion(
                assertion_id="assertion-1",
                action_type="FILE_EDIT",
                collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
                target="target-1",
                expected=StateValue(kind=StateEvidenceKind.FILE, exists=True),
            )],
        )


def test_verified_state_observation_requires_source_evidence_binding():
    with pytest.raises(ValidationError, match="verified state observation requires source evidence"):
        StateObservation(
            observation_id="observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="assertion-1",
            action_type="FILE_EDIT",
            fixture_sha256="e" * 64,
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            target="target-1",
            observed=StateValue(kind=StateEvidenceKind.FILE, exists=True),
            collected_at=datetime(2026, 8, 31, 12, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_state_fixture_hash_mismatch():
    fixture = StateFixtureDefinition(
        fixture_id="fixture-1",
        fixture_sha256="e" * 64,
        assertions=[StateAssertion(
            assertion_id="assertion-1",
            action_type="FILE_EDIT",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            target="target-1",
            expected=StateValue(kind=StateEvidenceKind.FILE, exists=True),
        )],
    )

    with pytest.raises(ValidationError, match="state fixture hash does not match"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            initial_state_fixture_hash="a" * 64,
            state_fixture=fixture,
        )


def test_canary_scrubbing_assertion_round_trips_without_raw_canary():
    assertion = CanaryScrubbingAssertion(
        assertion_id="email-canary",
        canary_sha256="a" * 64,
        source="conversation_history:user",
        input_artifact_sha256="b" * 64,
        expected_output_artifact_sha256="c" * 64,
        expected_scrub_type="email",
        expected_occurrences=1,
    )

    restored = CanaryScrubbingAssertion.model_validate_json(assertion.model_dump_json())

    assert restored == assertion
    assert "raw_canary" not in assertion.model_dump()


def test_task_definition_rejects_duplicate_canary_assertion_ids():
    assertion = CanaryScrubbingAssertion(
        assertion_id="duplicate",
        canary_sha256="a" * 64,
        source="conversation_history:user",
        input_artifact_sha256="b" * 64,
        expected_output_artifact_sha256="c" * 64,
        expected_scrub_type="email",
        expected_occurrences=1,
    )

    with pytest.raises(ValidationError, match="canary assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            sensitive_canary_annotations=[assertion, assertion],
        )


def test_task_definition_rejects_duplicate_rehydration_assertion_ids():
    assertion = RehydrationAssertion(
        assertion_id="duplicate",
        source="assistant_response",
        input_artifact_sha256="a" * 64,
        expected_output_artifact_sha256="b" * 64,
        expected_token_count=1,
        expected_sensitive_types=["email"],
    )

    with pytest.raises(ValidationError, match="rehydration assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            rehydration_assertions=[assertion, assertion],
        )


def test_task_definition_rejects_duplicate_final_state_assertion_ids():
    assertion = FinalStateAssertion(
        assertion_id="duplicate",
        predicate=StateAssertionPredicate.STATE_ROOT_CHANGED,
        action_type="FILE_EDIT",
    )

    with pytest.raises(ValidationError, match="final-state assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            expected_final_state_assertions=[assertion, assertion],
        )


@pytest.mark.parametrize(
    ("expected_outcome", "expected_rejection_layer"),
    [
        (PolicyOutcome.ALLOW, RejectionLayer.L1_DOCTRINE),
        (PolicyOutcome.BLOCK, None),
    ],
)
def test_task_definition_rejects_inconsistent_policy_expectation(
    expected_outcome: PolicyOutcome,
    expected_rejection_layer: RejectionLayer | None,
):
    with pytest.raises(ValidationError, match="expected rejection layer"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            expected_action_class="FILE_EDIT",
            expected_allow_block_outcome=expected_outcome,
            expected_rejection_layer=expected_rejection_layer,
        )


def test_attempt_record_links_all_final_state_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        final_state_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.final_state_observation_refs == ["observation-1", "observation-2"]


def test_attempt_record_logical_key_fields():
    ar = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        model_cohort_id="cohort-1",
        state_snapshot_hash="snap-1",
        replicate_id="1",
    )
    assert ar.arm_id == Arm.DOCTRINE
    assert ar.model_cohort_id == "cohort-1"
    assert ar.state_snapshot_hash == "snap-1"
    assert ar.replicate_id == "1"
    assert ar.parent_attempt_id is None


def test_attempt_record_default_posture_is_none():
    ar = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DIRECT,
    )
    assert ar.posture.requested_posture == GovernancePosture.NONE
    assert ar.posture.observed_posture is None


def test_receipt_observation_round_trips_canonical_action_receipt():
    receipt = ActionReceipt(
        transaction_id="tx-1",
        transaction_hash="hash-1",
    )
    receipt.deterministic_stage_evidence.add(
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        action_type="FILE_EDIT",
    )
    observation = ReceiptObservation(
        receipt_id="attempt:receipt:tx-1",
        attempt_id="attempt",
        run_id="run",
        transaction_id="tx-1",
        action_type="FILE_EDIT",
        primary=True,
        verified=True,
        action_receipt=receipt,
    )

    restored = ReceiptObservation.model_validate_json(observation.model_dump_json())

    assert restored.action_receipt.transaction_id == "tx-1"
    assert restored.action_receipt.deterministic_stage_evidence[0].action_type == "FILE_EDIT"
    assert restored.primary is True
    assert restored.verified is True


def test_stage_observation_rejects_extra_fields():
    with pytest.raises(ValidationError):
        StageObservation(
            stage_id="s1",
            attempt_id="a1",
            run_id="r1",
            kind=StageKind.MODEL_INFERENCE,
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_metric_obsation_rejects_extra_fields():
    with pytest.raises(ValidationError):
        MetricObservation(
            metric_id="m1",
            attempt_id="a1",
            run_id="r1",
            arm_id=Arm.DIRECT,
            task_id="t1",
            value=1.0,
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_failed_metric_observation_has_no_fabricated_value():
    metric = MetricObservation(
        metric_id="eval_judge",
        attempt_id="a1",
        run_id="r1",
        arm_id=Arm.DIRECT,
        task_id="t1",
        value=None,
        eligible=False,
        denominator_contribution=0,
    )

    assert metric.value is None


def test_evidence_index_rejects_extra_fields():
    with pytest.raises(ValidationError):
        EvidenceIndex(
            artifact_id="e1",
            run_id="r1",
            media_type=EvidenceMediaType.APPLICATION_JSON,
            sha256="abc",
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_model_identity_rejects_extra_fields():
    with pytest.raises(ValidationError):
        ModelIdentity(
            role="primary",
            provider="ollama",
            model="test",
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_posture_observation_records_requested_and_observed():
    po = PostureObservation(
        requested_posture=GovernancePosture.L2_CONSENSUS,
        observed_posture=GovernancePosture.L2_CONSENSUS,
        observation_source="gateway",
        posture_match=True,
    )
    assert po.requested_posture == GovernancePosture.L2_CONSENSUS
    assert po.observed_posture == GovernancePosture.L2_CONSENSUS
    assert po.posture_match is True


def test_arm_manifest_entry_rejects_extra_fields():
    with pytest.raises(ValidationError):
        ArmManifestEntry(
            arm_id=Arm.DIRECT,
            requested_posture=GovernancePosture.NONE,
            uses_g8ee=False,
            uses_gateway=False,
            receipt_binding=False,
            is_production_posture=False,
            extra_field="bad",  # type: ignore[call-arg]
        )


def test_terminal_status_has_seven_values():
    assert set(TerminalStatus) == {
        TerminalStatus.COMPLETED,
        TerminalStatus.MODEL_FAILED,
        TerminalStatus.GOVERNANCE_REJECTED,
        TerminalStatus.HUMAN_DENIED,
        TerminalStatus.TIMED_OUT,
        TerminalStatus.INFRASTRUCTURE_FAILED,
        TerminalStatus.INVALID_EVIDENCE,
    }


def test_stage_kind_covers_all_canonical_kinds():
    expected = {
        StageKind.MODEL_INFERENCE,
        StageKind.DETERMINISTIC_DOCTRINE,
        StageKind.TRIBUNAL_GENERATION,
        StageKind.TRIBUNAL_AUDITOR,
        StageKind.PROTOCOL_L2,
        StageKind.L3_CEREMONY,
        StageKind.L4_VERIFICATION,
        StageKind.L5_EXECUTION,
        StageKind.SCRUBBING,
        StageKind.REHYDRATION,
        StageKind.GRADING,
        StageKind.RECEIPT_PERSISTENCE,
        StageKind.COMMITMENT_APPEND,
    }
    assert set(StageKind) == expected


def test_run_manifest_serializes_to_json():
    manifest = RunManifest(
        run_id="r1",
        suite_id="s",
        suite_version="1.0",
        arms=[ArmManifestEntry(
            arm_id=Arm.DOCTRINE,
            requested_posture=GovernancePosture.L1_DOCTRINE,
            uses_g8ee=True,
            uses_gateway=True,
            receipt_binding=True,
            is_production_posture=True,
        )],
    )
    json_str = manifest.model_dump_json()
    assert '"run_id":"r1"' in json_str
    assert '"arm_id":"doctrine"' in json_str


def test_attempt_record_with_infrastructure_retry_linkage():
    parent = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.INFRASTRUCTURE_FAILED,
    )
    child = AttemptRecord(
        attempt_id="a2",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        parent_attempt_id="a1",
        assignment_order=1,
    )
    assert child.parent_attempt_id == "a1"
    assert parent.terminal_status == TerminalStatus.INFRASTRUCTURE_FAILED
    assert parent.parent_attempt_id is None


def test_token_store_persistence_assertion_and_observation_round_trip():
    assertion = TokenStorePersistenceAssertion(
        assertion_id="token-store-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_encryption_at_rest=True,
        expected_fail_closed_on_lock=True,
        expected_persistence_across_restart=True,
        expected_ttl_seconds=86400,
        expected_restored_token_count=3,
    )
    observation = TokenStorePersistenceObservation(
        observation_id="token-store-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        vault_algorithm="aes-256-gcm",
        stored_ciphertext_sha256="a" * 64,
        plaintext_in_store=False,
        vault_locked_write_refused=True,
        vault_locked_read_refused=True,
        restored_token_count=3,
        expired_token_invisible=True,
        collected_at=datetime(2026, 9, 3, 12, tzinfo=UTC),
        source_evidence_refs=["restricted-token-store-evidence"],
        source_evidence_sha256="b" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = TokenStorePersistenceAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = TokenStorePersistenceObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_token_store_persistence_assertion_rejects_wrong_collection_boundary():
    with pytest.raises(ValidationError, match="encrypted-token-store collection boundary"):
        TokenStorePersistenceAssertion(
            assertion_id="token-store-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            expected_ttl_seconds=86400,
            expected_restored_token_count=1,
        )


def test_token_store_persistence_assertion_rejects_zero_ttl():
    with pytest.raises(ValidationError):
        TokenStorePersistenceAssertion(
            assertion_id="token-store-1",
            collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
            expected_ttl_seconds=0,
            expected_restored_token_count=1,
        )


def test_verified_token_store_persistence_observation_requires_source_evidence():
    with pytest.raises(ValidationError, match="verified token-store persistence observation requires source evidence"):
        TokenStorePersistenceObservation(
            observation_id="token-store-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="token-store-1",
            collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
            vault_algorithm="aes-256-gcm",
            stored_ciphertext_sha256="a" * 64,
            plaintext_in_store=False,
            vault_locked_write_refused=True,
            vault_locked_read_refused=True,
            restored_token_count=1,
            expired_token_invisible=True,
            collected_at=datetime(2026, 9, 3, 12, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_token_store_persistence_observation_rejects_invalid_ciphertext_hash():
    with pytest.raises(ValidationError):
        TokenStorePersistenceObservation(
            observation_id="token-store-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="token-store-1",
            collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
            vault_algorithm="aes-256-gcm",
            stored_ciphertext_sha256="not-a-sha256",
            plaintext_in_store=False,
            vault_locked_write_refused=True,
            vault_locked_read_refused=True,
            restored_token_count=1,
            expired_token_invisible=True,
            collected_at=datetime(2026, 9, 3, 12, tzinfo=UTC),
        )


def test_task_definition_rejects_duplicate_token_store_persistence_assertion_ids():
    assertion = TokenStorePersistenceAssertion(
        assertion_id="duplicate",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_ttl_seconds=86400,
        expected_restored_token_count=1,
    )
    with pytest.raises(ValidationError, match="token-store persistence assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            token_store_persistence_assertions=[assertion, assertion],
        )


def test_attempt_record_links_token_store_persistence_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        token_store_persistence_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.token_store_persistence_observation_refs == ["observation-1", "observation-2"]


def test_token_ttl_expiry_assertion_and_observation_round_trip():
    assertion = TokenTTLExpiryAssertion(
        assertion_id="token-ttl-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_ttl_seconds=3600,
        expected_visible_before_expiry=True,
        expected_invisible_after_expiry=True,
        expected_expiry_tolerance_seconds=5,
    )
    observation = TokenTTLExpiryObservation(
        observation_id="token-ttl-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        token_visible_before_expiry=True,
        token_invisible_after_expiry=True,
        measured_ttl_seconds=3602,
        pre_expiry_collection_time=datetime(2026, 9, 3, 12, tzinfo=UTC),
        post_expiry_collection_time=datetime(2026, 9, 3, 13, 5, tzinfo=UTC),
        measured_expiry_timestamp=datetime(2026, 9, 3, 13, tzinfo=UTC),
        collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
        source_evidence_refs=["restricted-ttl-evidence"],
        source_evidence_sha256="c" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = TokenTTLExpiryAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = TokenTTLExpiryObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_token_ttl_expiry_assertion_rejects_wrong_collection_boundary():
    with pytest.raises(ValidationError, match="encrypted-token-store collection boundary"):
        TokenTTLExpiryAssertion(
            assertion_id="token-ttl-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            expected_ttl_seconds=3600,
        )


def test_token_ttl_expiry_assertion_rejects_zero_ttl():
    with pytest.raises(ValidationError):
        TokenTTLExpiryAssertion(
            assertion_id="token-ttl-1",
            collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
            expected_ttl_seconds=0,
        )


def test_verified_token_ttl_expiry_observation_requires_source_evidence():
    with pytest.raises(ValidationError, match="verified token TTL expiry observation requires source evidence"):
        TokenTTLExpiryObservation(
            observation_id="token-ttl-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="token-ttl-1",
            collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
            token_visible_before_expiry=True,
            token_invisible_after_expiry=True,
            measured_ttl_seconds=3600,
            pre_expiry_collection_time=datetime(2026, 9, 3, 12, tzinfo=UTC),
            post_expiry_collection_time=datetime(2026, 9, 3, 13, tzinfo=UTC),
            measured_expiry_timestamp=datetime(2026, 9, 3, 13, tzinfo=UTC),
            collected_at=datetime(2026, 9, 3, 13, 1, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_token_ttl_expiry_observation_rejects_unordered_collection_times():
    with pytest.raises(ValidationError, match="post-expiry collection time must be after pre-expiry collection time"):
        TokenTTLExpiryObservation(
            observation_id="token-ttl-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="token-ttl-1",
            collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
            token_visible_before_expiry=True,
            token_invisible_after_expiry=True,
            measured_ttl_seconds=3600,
            pre_expiry_collection_time=datetime(2026, 9, 3, 13, tzinfo=UTC),
            post_expiry_collection_time=datetime(2026, 9, 3, 12, tzinfo=UTC),
            measured_expiry_timestamp=datetime(2026, 9, 3, 13, tzinfo=UTC),
            collected_at=datetime(2026, 9, 3, 13, 1, tzinfo=UTC),
        )


def test_task_definition_rejects_duplicate_token_ttl_expiry_assertion_ids():
    assertion = TokenTTLExpiryAssertion(
        assertion_id="duplicate",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_ttl_seconds=3600,
    )
    with pytest.raises(ValidationError, match="token TTL expiry assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            token_ttl_expiry_assertions=[assertion, assertion],
        )


def test_attempt_record_links_token_ttl_expiry_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        token_ttl_expiry_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.token_ttl_expiry_observation_refs == ["observation-1", "observation-2"]


# ---------------------------------------------------------------------------
# TokenPersistenceFailureAssertion / TokenPersistenceFailureObservation
# ---------------------------------------------------------------------------


def test_token_persistence_failure_assertion_and_observation_round_trip():
    assertion = TokenPersistenceFailureAssertion(
        assertion_id="token-persist-fail-1",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        expected_fail_closed=True,
        expected_rollback=True,
        expected_no_sensitive_leak=True,
        expected_no_unsafe_continuation=True,
        expected_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
    )
    observation = TokenPersistenceFailureObservation(
        observation_id="token-persist-fail-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
        storage_failure_injected=True,
        operation_refused=True,
        in_memory_token_rolled_back=True,
        sensitive_value_leaked=False,
        unsafe_continuation_detected=False,
        measured_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
        collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
        source_evidence_refs=["restricted-persist-fail-evidence"],
        source_evidence_sha256="d" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = TokenPersistenceFailureAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = TokenPersistenceFailureObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_token_persistence_failure_assertion_rejects_wrong_collection_boundary():
    with pytest.raises(ValidationError, match="encrypted-token-store collection boundary"):
        TokenPersistenceFailureAssertion(
            assertion_id="token-persist-fail-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        )


def test_verified_token_persistence_failure_observation_requires_source_evidence():
    with pytest.raises(
        ValidationError,
        match="verified token persistence failure observation requires source evidence",
    ):
        TokenPersistenceFailureObservation(
            observation_id="token-persist-fail-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="token-persist-fail-1",
            collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
            storage_failure_injected=True,
            operation_refused=True,
            in_memory_token_rolled_back=True,
            sensitive_value_leaked=False,
            unsafe_continuation_detected=False,
            measured_failure_outcome=TokenPersistenceFailureOutcome.STORAGE_FAILURE,
            collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_duplicate_token_persistence_failure_assertion_ids():
    assertion = TokenPersistenceFailureAssertion(
        assertion_id="duplicate",
        collection_boundary=StateCollectionBoundary.ENCRYPTED_TOKEN_STORE,
    )
    with pytest.raises(ValidationError, match="token persistence failure assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            token_persistence_failure_assertions=[assertion, assertion],
        )


def test_attempt_record_links_token_persistence_failure_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        token_persistence_failure_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.token_persistence_failure_observation_refs == ["observation-1", "observation-2"]


# ---------------------------------------------------------------------------
# ExfiltrationAttemptAssertion / ExfiltrationAttemptObservation
# ---------------------------------------------------------------------------


def test_exfiltration_attempt_assertion_and_observation_round_trip():
    assertion = ExfiltrationAttemptAssertion(
        assertion_id="exfil-1",
        action_type="DATA_EXFIL",
        source="conversation_history:user",
        destination="model_boundary:provider",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=False),
    )
    observation = ExfiltrationAttemptObservation(
        observation_id="exfil-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        action_type="DATA_EXFIL",
        source="conversation_history:user",
        destination="model_boundary:provider",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        observed=StateValue(kind=StateEvidenceKind.FILE, exists=False),
        collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
        source_evidence_refs=["restricted-exfil-evidence"],
        source_evidence_sha256="e" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = ExfiltrationAttemptAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = ExfiltrationAttemptObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_exfiltration_attempt_assertion_rejects_present_expected_state():
    with pytest.raises(ValidationError, match="exfiltration expected absence requires exists=False"):
        ExfiltrationAttemptAssertion(
            assertion_id="exfil-1",
            action_type="DATA_EXFIL",
            source="conversation_history:user",
            destination="model_boundary:provider",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=True),
        )


def test_exfiltration_attempt_assertion_rejects_consistent_ledger_expected_state():
    with pytest.raises(ValidationError, match="exfiltration expected absence requires consistent=False for ledger state"):
        ExfiltrationAttemptAssertion(
            assertion_id="exfil-1",
            action_type="DATA_EXFIL",
            source="conversation_history:user",
            destination="ledger://restricted",
            collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=True),
        )


def test_verified_exfiltration_attempt_observation_requires_source_evidence():
    with pytest.raises(
        ValidationError,
        match="verified exfiltration observation requires source evidence",
    ):
        ExfiltrationAttemptObservation(
            observation_id="exfil-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="exfil-1",
            action_type="DATA_EXFIL",
            source="conversation_history:user",
            destination="model_boundary:provider",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            observed=StateValue(kind=StateEvidenceKind.FILE, exists=False),
            collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_duplicate_exfiltration_attempt_assertion_ids():
    assertion = ExfiltrationAttemptAssertion(
        assertion_id="duplicate",
        action_type="DATA_EXFIL",
        source="conversation_history:user",
        destination="model_boundary:provider",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=False),
    )
    with pytest.raises(ValidationError, match="exfiltration attempt assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            exfiltration_attempt_assertions=[assertion, assertion],
        )


def test_attempt_record_links_exfiltration_attempt_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        exfiltration_attempt_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.exfiltration_attempt_observation_refs == ["observation-1", "observation-2"]


# ---------------------------------------------------------------------------
# ArtifactLeakageAssertion / ArtifactLeakageObservation
# ---------------------------------------------------------------------------


def test_artifact_leakage_assertion_and_observation_round_trip():
    assertion = ArtifactLeakageAssertion(
        assertion_id="leak-1",
        artifact_class="summary_json",
        collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
        expected_absent_sensitive_types=[
            SensitiveArtifactContentType.RAW_CANARY,
            SensitiveArtifactContentType.CREDENTIAL,
        ],
    )
    observation = ArtifactLeakageObservation(
        observation_id="leak-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        artifact_class="summary_json",
        collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
        artifact_present=True,
        artifact_sha256="a" * 64,
        artifact_byte_length=1024,
        scanner_version="sentinel-leakage@1.0.0",
        sensitive_occurrences=0,
        sensitive_types_found=[],
        collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
        source_evidence_refs=["restricted-leak-evidence"],
        source_evidence_sha256="e" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = ArtifactLeakageAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = ArtifactLeakageObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_artifact_leakage_assertion_rejects_duplicate_sensitive_types():
    with pytest.raises(ValidationError, match="expected absent sensitive types must be unique"):
        ArtifactLeakageAssertion(
            assertion_id="leak-1",
            artifact_class="summary_json",
            collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
            expected_absent_sensitive_types=[
                SensitiveArtifactContentType.RAW_CANARY,
                SensitiveArtifactContentType.RAW_CANARY,
            ],
        )


def test_artifact_leakage_assertion_rejects_empty_sensitive_types():
    with pytest.raises(ValidationError):
        ArtifactLeakageAssertion(
            assertion_id="leak-1",
            artifact_class="summary_json",
            collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
            expected_absent_sensitive_types=[],
        )


def test_artifact_leakage_observation_rejects_present_artifact_without_hash():
    with pytest.raises(ValidationError, match="present artifact requires a content hash and non-zero byte length"):
        ArtifactLeakageObservation(
            observation_id="leak-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="leak-1",
            artifact_class="summary_json",
            collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
            artifact_present=True,
            artifact_sha256=None,
            artifact_byte_length=0,
            scanner_version="sentinel-leakage@1.0.0",
            sensitive_occurrences=0,
            sensitive_types_found=[],
            collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256="e" * 64,
            verification_status=VerificationStatus.VERIFIED,
        )


def test_artifact_leakage_observation_rejects_absent_artifact_with_hash():
    with pytest.raises(ValidationError, match="absent artifact must not declare a content hash or byte length"):
        ArtifactLeakageObservation(
            observation_id="leak-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="leak-1",
            artifact_class="summary_json",
            collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
            artifact_present=False,
            artifact_sha256="a" * 64,
            artifact_byte_length=1024,
            scanner_version="sentinel-leakage@1.0.0",
            sensitive_occurrences=0,
            sensitive_types_found=[],
            collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256="e" * 64,
            verification_status=VerificationStatus.VERIFIED,
        )


def test_artifact_leakage_observation_rejects_sensitive_occurrence_type_mismatch():
    with pytest.raises(ValidationError, match="sensitive occurrence count and sensitive types found must agree"):
        ArtifactLeakageObservation(
            observation_id="leak-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="leak-1",
            artifact_class="summary_json",
            collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
            artifact_present=True,
            artifact_sha256="a" * 64,
            artifact_byte_length=1024,
            scanner_version="sentinel-leakage@1.0.0",
            sensitive_occurrences=2,
            sensitive_types_found=[SensitiveArtifactContentType.RAW_CANARY],
            collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256="e" * 64,
            verification_status=VerificationStatus.VERIFIED,
        )


def test_verified_artifact_leakage_observation_requires_source_evidence():
    with pytest.raises(
        ValidationError,
        match="verified artifact-leakage observation requires source evidence",
    ):
        ArtifactLeakageObservation(
            observation_id="leak-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="leak-1",
            artifact_class="summary_json",
            collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
            artifact_present=True,
            artifact_sha256="a" * 64,
            artifact_byte_length=1024,
            scanner_version="sentinel-leakage@1.0.0",
            sensitive_occurrences=0,
            sensitive_types_found=[],
            collected_at=datetime(2026, 9, 3, 13, 6, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_duplicate_artifact_leakage_assertion_ids():
    assertion = ArtifactLeakageAssertion(
        assertion_id="duplicate",
        artifact_class="summary_json",
        collection_boundary=ArtifactCollectionBoundary.REPORT_DIRECTORY,
        expected_absent_sensitive_types=[SensitiveArtifactContentType.RAW_CANARY],
    )
    with pytest.raises(ValidationError, match="artifact leakage assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            artifact_leakage_assertions=[assertion, assertion],
        )


def test_attempt_record_links_artifact_leakage_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        artifact_leakage_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.artifact_leakage_observation_refs == ["observation-1", "observation-2"]


# ---------------------------------------------------------------------------
# ReplayAttemptAssertion / ReplayAttemptObservation
# ---------------------------------------------------------------------------


def test_replay_attempt_assertion_and_observation_round_trip():
    assertion = ReplayAttemptAssertion(
        assertion_id="replay-1",
        action_type="FILE_EDIT",
        replayed_transaction_id="original-tx-1",
        replayed_transaction_hash="original-hash-1",
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
    )
    observation = ReplayAttemptObservation(
        observation_id="replay-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        action_type="FILE_EDIT",
        replayed_transaction_id="original-tx-1",
        replayed_transaction_hash="original-hash-1",
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        observed=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
        collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
        source_evidence_refs=["restricted-replay-evidence"],
        source_evidence_sha256="e" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = ReplayAttemptAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = ReplayAttemptObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_replay_attempt_assertion_rejects_present_expected_state():
    with pytest.raises(ValidationError, match="replay expected absence requires exists=False"):
        ReplayAttemptAssertion(
            assertion_id="replay-1",
            action_type="FILE_EDIT",
            replayed_transaction_id="original-tx-1",
            replayed_transaction_hash="original-hash-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=True),
        )


def test_replay_attempt_assertion_rejects_consistent_ledger_expected_state():
    with pytest.raises(ValidationError, match="replay expected absence requires consistent=False for ledger state"):
        ReplayAttemptAssertion(
            assertion_id="replay-1",
            action_type="FILE_EDIT",
            replayed_transaction_id="original-tx-1",
            replayed_transaction_hash="original-hash-1",
            collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=True),
        )


def test_verified_replay_attempt_observation_requires_source_evidence():
    with pytest.raises(
        ValidationError,
        match="verified replay observation requires source evidence",
    ):
        ReplayAttemptObservation(
            observation_id="replay-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="replay-1",
            action_type="FILE_EDIT",
            replayed_transaction_id="original-tx-1",
            replayed_transaction_hash="original-hash-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            observed=StateValue(kind=StateEvidenceKind.FILE, exists=False),
            collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_duplicate_replay_attempt_assertion_ids():
    assertion = ReplayAttemptAssertion(
        assertion_id="duplicate",
        action_type="FILE_EDIT",
        replayed_transaction_id="original-tx-1",
        replayed_transaction_hash="original-hash-1",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=False),
    )
    with pytest.raises(ValidationError, match="replay attempt assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            replay_attempt_assertions=[assertion, assertion],
        )


def test_attempt_record_links_replay_attempt_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        replay_attempt_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.replay_attempt_observation_refs == ["observation-1", "observation-2"]


# ---------------------------------------------------------------------------
# SignedFieldTamperingAssertion / SignedFieldTamperingObservation
# ---------------------------------------------------------------------------


def test_signed_field_tampering_assertion_and_observation_round_trip():
    assertion = SignedFieldTamperingAssertion(
        assertion_id="tamper-1",
        action_type="FILE_EDIT",
        tampered_field=SignedField.TRANSACTION_HASH,
        original_value="original-hash-1",
        tampered_value="tampered-hash-1",
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
    )
    observation = SignedFieldTamperingObservation(
        observation_id="tamper-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        action_type="FILE_EDIT",
        tampered_field=SignedField.TRANSACTION_HASH,
        tampered_value="tampered-hash-1",
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        observed=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
        collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
        source_evidence_refs=["restricted-tamper-evidence"],
        source_evidence_sha256="e" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = SignedFieldTamperingAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = SignedFieldTamperingObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_signed_field_tampering_assertion_rejects_present_expected_state():
    with pytest.raises(ValidationError, match="signed-field tampering expected absence requires exists=False"):
        SignedFieldTamperingAssertion(
            assertion_id="tamper-1",
            action_type="FILE_EDIT",
            tampered_field=SignedField.TRANSACTION_HASH,
            original_value="original-hash-1",
            tampered_value="tampered-hash-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=True),
        )


def test_signed_field_tampering_assertion_rejects_consistent_ledger_expected_state():
    with pytest.raises(
        ValidationError,
        match="signed-field tampering expected absence requires consistent=False for ledger state",
    ):
        SignedFieldTamperingAssertion(
            assertion_id="tamper-1",
            action_type="FILE_EDIT",
            tampered_field=SignedField.TRANSACTION_HASH,
            original_value="original-hash-1",
            tampered_value="tampered-hash-1",
            collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=True),
        )


def test_verified_signed_field_tampering_observation_requires_source_evidence():
    with pytest.raises(
        ValidationError,
        match="verified signed-field tampering observation requires source evidence",
    ):
        SignedFieldTamperingObservation(
            observation_id="tamper-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="tamper-1",
            action_type="FILE_EDIT",
            tampered_field=SignedField.TRANSACTION_HASH,
            tampered_value="tampered-hash-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            observed=StateValue(kind=StateEvidenceKind.FILE, exists=False),
            collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_duplicate_signed_field_tampering_assertion_ids():
    assertion = SignedFieldTamperingAssertion(
        assertion_id="duplicate",
        action_type="FILE_EDIT",
        tampered_field=SignedField.TRANSACTION_HASH,
        original_value="original-hash-1",
        tampered_value="tampered-hash-1",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=False),
    )
    with pytest.raises(ValidationError, match="signed-field tampering assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            signed_field_tampering_assertions=[assertion, assertion],
        )


def test_attempt_record_links_signed_field_tampering_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        signed_field_tampering_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.signed_field_tampering_observation_refs == ["observation-1", "observation-2"]


# ---------------------------------------------------------------------------
# PayloadTamperingAssertion / PayloadTamperingObservation
# ---------------------------------------------------------------------------


def test_payload_tampering_assertion_and_observation_round_trip():
    assertion = PayloadTamperingAssertion(
        assertion_id="payload-tamper-1",
        action_type="FILE_EDIT",
        original_payload_hash="a" * 64,
        tampered_payload_hash="b" * 64,
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
    )
    observation = PayloadTamperingObservation(
        observation_id="payload-tamper-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        action_type="FILE_EDIT",
        tampered_payload_hash="b" * 64,
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        observed=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
        collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
        source_evidence_refs=["restricted-payload-tamper-evidence"],
        source_evidence_sha256="e" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = PayloadTamperingAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = PayloadTamperingObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_payload_tampering_assertion_rejects_present_expected_state():
    with pytest.raises(ValidationError, match="payload tampering expected absence requires exists=False"):
        PayloadTamperingAssertion(
            assertion_id="payload-tamper-1",
            action_type="FILE_EDIT",
            original_payload_hash="a" * 64,
            tampered_payload_hash="b" * 64,
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=True),
        )


def test_payload_tampering_assertion_rejects_consistent_ledger_expected_state():
    with pytest.raises(
        ValidationError,
        match="payload tampering expected absence requires consistent=False for ledger state",
    ):
        PayloadTamperingAssertion(
            assertion_id="payload-tamper-1",
            action_type="FILE_EDIT",
            original_payload_hash="a" * 64,
            tampered_payload_hash="b" * 64,
            collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=True),
        )


def test_verified_payload_tampering_observation_requires_source_evidence():
    with pytest.raises(
        ValidationError,
        match="verified payload tampering observation requires source evidence",
    ):
        PayloadTamperingObservation(
            observation_id="payload-tamper-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="payload-tamper-1",
            action_type="FILE_EDIT",
            tampered_payload_hash="b" * 64,
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            observed=StateValue(kind=StateEvidenceKind.FILE, exists=False),
            collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_duplicate_payload_tampering_assertion_ids():
    assertion = PayloadTamperingAssertion(
        assertion_id="duplicate",
        action_type="FILE_EDIT",
        original_payload_hash="a" * 64,
        tampered_payload_hash="b" * 64,
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=False),
    )
    with pytest.raises(ValidationError, match="payload tampering assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            payload_tampering_assertions=[assertion, assertion],
        )


def test_attempt_record_links_payload_tampering_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        payload_tampering_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.payload_tampering_observation_refs == ["observation-1", "observation-2"]


# ---------------------------------------------------------------------------
# StaleStateRootAssertion / StaleStateRootObservation
# ---------------------------------------------------------------------------


def test_stale_state_root_assertion_and_observation_round_trip():
    assertion = StaleStateRootAssertion(
        assertion_id="stale-1",
        action_type="FILE_EDIT",
        declared_current_root="current-root-1",
        stale_root_replayed="stale-root-1",
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
    )
    observation = StaleStateRootObservation(
        observation_id="stale-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        action_type="FILE_EDIT",
        declared_current_root="current-root-1",
        stale_root_replayed="stale-root-1",
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        observed=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
        collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
        source_evidence_refs=["restricted-stale-evidence"],
        source_evidence_sha256="e" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = StaleStateRootAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = StaleStateRootObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_stale_state_root_assertion_rejects_present_expected_state():
    with pytest.raises(ValidationError, match="stale-state-root expected absence requires exists=False"):
        StaleStateRootAssertion(
            assertion_id="stale-1",
            action_type="FILE_EDIT",
            declared_current_root="current-root-1",
            stale_root_replayed="stale-root-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=True),
        )


def test_stale_state_root_assertion_rejects_consistent_ledger_expected_state():
    with pytest.raises(ValidationError, match="stale-state-root expected absence requires consistent=False for ledger state"):
        StaleStateRootAssertion(
            assertion_id="stale-1",
            action_type="FILE_EDIT",
            declared_current_root="current-root-1",
            stale_root_replayed="stale-root-1",
            collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=True),
        )


def test_verified_stale_state_root_observation_requires_source_evidence():
    with pytest.raises(
        ValidationError,
        match="verified stale-state-root observation requires source evidence",
    ):
        StaleStateRootObservation(
            observation_id="stale-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="stale-1",
            action_type="FILE_EDIT",
            declared_current_root="current-root-1",
            stale_root_replayed="stale-root-1",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            observed=StateValue(kind=StateEvidenceKind.FILE, exists=False),
            collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_duplicate_stale_state_root_assertion_ids():
    assertion = StaleStateRootAssertion(
        assertion_id="duplicate",
        action_type="FILE_EDIT",
        declared_current_root="current-root-1",
        stale_root_replayed="stale-root-1",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=False),
    )
    with pytest.raises(ValidationError, match="stale-state-root assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            stale_state_root_assertions=[assertion, assertion],
        )


def test_attempt_record_links_stale_state_root_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        stale_state_root_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.stale_state_root_observation_refs == ["observation-1", "observation-2"]


# ---------------------------------------------------------------------------
# IdentityMismatchAssertion / IdentityMismatchObservation
# ---------------------------------------------------------------------------


def test_identity_mismatch_assertion_and_observation_round_trip():
    assertion = IdentityMismatchAssertion(
        assertion_id="identity-1",
        action_type="FILE_EDIT",
        identity_binding=IdentityBinding.OPERATOR,
        expected_identity="operator-alpha",
        mismatched_identity="operator-beta",
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
    )
    observation = IdentityMismatchObservation(
        observation_id="identity-observation-1",
        attempt_id="attempt-1",
        run_id="run-1",
        task_id="task-1",
        assertion_id=assertion.assertion_id,
        action_type="FILE_EDIT",
        identity_binding=IdentityBinding.OPERATOR,
        expected_identity="operator-alpha",
        mismatched_identity="operator-beta",
        collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
        observed=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=False),
        collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
        source_evidence_refs=["restricted-identity-evidence"],
        source_evidence_sha256="e" * 64,
        verification_status=VerificationStatus.VERIFIED,
    )

    restored_assertion = IdentityMismatchAssertion.model_validate_json(assertion.model_dump_json())
    restored_observation = IdentityMismatchObservation.model_validate_json(observation.model_dump_json())

    assert restored_assertion == assertion
    assert restored_observation == observation


def test_identity_mismatch_assertion_rejects_present_expected_state():
    with pytest.raises(ValidationError, match="identity-mismatch expected absence requires exists=False"):
        IdentityMismatchAssertion(
            assertion_id="identity-1",
            action_type="FILE_EDIT",
            identity_binding=IdentityBinding.OPERATOR,
            expected_identity="operator-alpha",
            mismatched_identity="operator-beta",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=True),
        )


def test_identity_mismatch_assertion_rejects_consistent_ledger_expected_state():
    with pytest.raises(ValidationError, match="identity-mismatch expected absence requires consistent=False for ledger state"):
        IdentityMismatchAssertion(
            assertion_id="identity-1",
            action_type="FILE_EDIT",
            identity_binding=IdentityBinding.OPERATOR,
            expected_identity="operator-alpha",
            mismatched_identity="operator-beta",
            collection_boundary=StateCollectionBoundary.GOVERNANCE_LEDGER,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            expected_absence=StateValue(kind=StateEvidenceKind.LEDGER_CONSISTENCY, consistent=True),
        )


def test_verified_identity_mismatch_observation_requires_source_evidence():
    with pytest.raises(
        ValidationError,
        match="verified identity-mismatch observation requires source evidence",
    ):
        IdentityMismatchObservation(
            observation_id="identity-observation-1",
            attempt_id="attempt-1",
            run_id="run-1",
            task_id="task-1",
            assertion_id="identity-1",
            action_type="FILE_EDIT",
            identity_binding=IdentityBinding.OPERATOR,
            expected_identity="operator-alpha",
            mismatched_identity="operator-beta",
            collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
            observed=StateValue(kind=StateEvidenceKind.FILE, exists=False),
            collected_at=datetime(2026, 9, 4, 12, 0, tzinfo=UTC),
            verification_status=VerificationStatus.VERIFIED,
        )


def test_task_definition_rejects_duplicate_identity_mismatch_assertion_ids():
    assertion = IdentityMismatchAssertion(
        assertion_id="duplicate",
        action_type="FILE_EDIT",
        identity_binding=IdentityBinding.OPERATOR,
        expected_identity="operator-alpha",
        mismatched_identity="operator-beta",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=False),
    )
    with pytest.raises(ValidationError, match="identity-mismatch assertion IDs must be unique"):
        TaskDefinition(
            task_id="task-1",
            suite_id="suite-1",
            suite_version="1.0.0",
            prompt_hash="prompt-hash",
            identity_mismatch_assertions=[assertion, assertion],
        )


def test_attempt_record_links_identity_mismatch_observations():
    attempt = AttemptRecord(
        attempt_id="a1",
        run_id="r1",
        task_id="t1",
        arm_id=Arm.DOCTRINE,
        identity_mismatch_observation_refs=["observation-1", "observation-2"],
    )

    assert attempt.identity_mismatch_observation_refs == ["observation-1", "observation-2"]
