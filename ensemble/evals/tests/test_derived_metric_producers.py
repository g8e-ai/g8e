# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for derived analysis metric producers.

Verifies that all 32 ``GraderClass.ANALYSIS`` metrics have exactly one
authoritative producer registered in ``DEFAULT_DERIVED_REGISTRY``, that
each producer emits canonical ``MetricObservation`` records from
verified immutable source records, and that malformed supplied records
raise ``DerivedProducerError`` fail-closed. Genuine absence (no source
record supplied) produces no observation. No external dependencies (no
files, network, or DB).
"""

from __future__ import annotations

from datetime import UTC, datetime

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis.derived import (
    DEFAULT_DERIVED_REGISTRY,
    DerivedProducerError,
    DerivedProducerRegistry,
    produce_allow_block_confusion_matrix_observations,
    produce_attack_success_rate_observations,
    produce_audit_linkage_observations,
    produce_balanced_accuracy_observations,
    produce_commitment_linkage_observations,
    produce_envelope_linkage_observations,
    produce_evidence_validity_observations,
    produce_expected_layer_detection_observations,
    produce_harm_weighted_loss_observations,
    produce_l2_proof_property_observations,
    produce_l3_proof_property_observations,
    produce_l4_proof_property_observations,
    produce_l5_proof_property_observations,
    produce_matthews_correlation_coefficient_observations,
    produce_persistence_linkage_observations,
    produce_receipt_linkage_observations,
    produce_state_linkage_observations,
    run_all_derived_producers,
)
from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.arms import Arm
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY, GraderClass
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)
from g8e_evals.schema import (
    AttemptRecord,
    AuditLinkRecord,
    AttackSeverity,
    AttackType,
    CommitmentAttestation,
    FinalStateObservation,
    GovernanceEnvelopeRecord,
    GraderClass as SchemaGraderClass,
    MetricObservation,
    PersistenceAttestation,
    PolicyAttackAssertion,
    PolicyAttackObservation,
    PolicyOutcome,
    ReceiptObservation,
    RejectionLayer,
    ReliabilityAssertion,
    ReliabilityObservation,
    ReliabilityScenarioType,
    ReliabilityExpectedBehavior,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateValue,
    StageKind,
    StageObservation,
    TaskDefinition,
    VerificationStatus,
)

_RUN_ID = "run-der-1"
_TASK_ID = "task-der-1"
_ATTEMPT_ID = "attempt-der-1"
_TS = datetime(2026, 1, 1, tzinfo=UTC)
_HASH = "a" * 64
_HASH_B = "b" * 64


# ---------------------------------------------------------------------------
# Record builders
# ---------------------------------------------------------------------------


def _attempt(
    attempt_id: str = _ATTEMPT_ID,
    arm_id: Arm = Arm.DOCTRINE,
    task_id: str = _TASK_ID,
) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=task_id,
        arm_id=arm_id,
    )


def _task(
    task_id: str = _TASK_ID,
    expected_allow_block: PolicyOutcome | None = None,
    expected_rejection_layer: RejectionLayer | None = None,
    policy_attack_assertions: list[PolicyAttackAssertion] | None = None,
    reliability_assertions: list[ReliabilityAssertion] | None = None,
) -> TaskDefinition:
    return TaskDefinition(
        task_id=task_id,
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash=_HASH,
        prompt_length=10,
        expected_action_class="TEST_ACTION",
        compatible_arms=[Arm.DOCTRINE],
        expected_allow_block_outcome=expected_allow_block,
        expected_rejection_layer=expected_rejection_layer,
        policy_attack_assertions=policy_attack_assertions or [],
        reliability_assertions=reliability_assertions or [],
    )


def _policy_attack_assertion(
    assertion_id: str = "pa-1",
    expected_outcome: PolicyOutcome = PolicyOutcome.BLOCK,
    expected_rejection_layer: RejectionLayer = RejectionLayer.L2_CONSENSUS,
    severity: AttackSeverity = AttackSeverity.HIGH,
) -> PolicyAttackAssertion:
    return PolicyAttackAssertion(
        assertion_id=assertion_id,
        attack_type=AttackType.PROMPT_INJECTION,
        action_type="TEST_ACTION",
        expected_outcome=expected_outcome,
        expected_rejection_layer=expected_rejection_layer if expected_outcome == PolicyOutcome.BLOCK else None,
        severity=severity,
        prohibited_terminal_state="leaked secret",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        expected_absence=StateValue(kind=StateEvidenceKind.FILE, exists=False),
    )


def _policy_attack_observation(
    observation_id: str = "pao-1",
    attempt_id: str = _ATTEMPT_ID,
    assertion_id: str = "pa-1",
    observed_exists: bool = False,
    verification_status: VerificationStatus = VerificationStatus.VERIFIED,
) -> PolicyAttackObservation:
    return PolicyAttackObservation(
        observation_id=observation_id,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        assertion_id=assertion_id,
        attack_type=AttackType.PROMPT_INJECTION,
        action_type="TEST_ACTION",
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        observed=StateValue(kind=StateEvidenceKind.FILE, exists=observed_exists),
        collected_at=_TS,
        source_evidence_refs=["ev-1"],
        source_evidence_sha256=_HASH,
        verification_status=verification_status,
    )


def _stage(
    stage_id: str = "stage-1",
    kind: StageKind = StageKind.PROTOCOL_L2,
    attempt_id: str = _ATTEMPT_ID,
    task_id: str = _TASK_ID,
    decision: str | None = None,
    l2_signature_digest: str = "",
    l3_signature_digest: str = "",
    commitment_hash: str = "",
) -> StageObservation:
    return StageObservation(
        stage_id=stage_id,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        kind=kind,
        task_id=task_id,
        decision=decision,
        l2_signature_digest=l2_signature_digest,
        l3_signature_digest=l3_signature_digest,
        commitment_hash=commitment_hash,
    )


def _metric_obs(
    metric_id: str = "policy_outcome",
    attempt_id: str = _ATTEMPT_ID,
    task_id: str = _TASK_ID,
    arm_id: Arm = Arm.DOCTRINE,
    value: float | None = 1.0,
) -> MetricObservation:
    return MetricObservation(
        metric_id=metric_id,
        metric_version="1.0.0",
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        arm_id=arm_id,
        task_id=task_id,
        value=value,
        unit="proportion",
        eligible=True,
        denominator_contribution=1,
        verification_status=VerificationStatus.VERIFIED,
        grader_class=SchemaGraderClass.DETERMINISTIC,
    )


def _receipt(
    receipt_id: str = "receipt-1",
    attempt_id: str = _ATTEMPT_ID,
    primary: bool = True,
    verified: bool = True,
) -> ReceiptObservation:
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
        receipt_id=receipt_id,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        transaction_id="tx-1",
        action_type="TEST_ACTION",
        primary=primary,
        verified=verified,
        action_receipt=receipt,
    )


def _envelope(
    envelope_id: str = "env-1",
    attempt_id: str = _ATTEMPT_ID,
    envelope_sha256: str = _HASH,
    run_id: str = _RUN_ID,
    task_id: str = _TASK_ID,
) -> GovernanceEnvelopeRecord:
    return GovernanceEnvelopeRecord(
        envelope_id=envelope_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
        envelope_sha256=envelope_sha256,
        layer_disposition="L1_ALLOW",
        signer_key_id="key-1",
        created_at=_TS,
    )


def _final_state_obs(
    observation_id: str = "fs-1",
    attempt_id: str = _ATTEMPT_ID,
    verification_status: VerificationStatus = VerificationStatus.VERIFIED,
) -> FinalStateObservation:
    return FinalStateObservation(
        observation_id=observation_id,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        assertion_id="fsa-1",
        action_type="TEST_ACTION",
        verification_status=verification_status,
    )


def _persistence_attestation(
    attestation_id: str = "pa-att-1",
    attempt_id: str = _ATTEMPT_ID,
    content_sha256: str = _HASH,
    run_id: str = _RUN_ID,
    task_id: str = _TASK_ID,
) -> PersistenceAttestation:
    return PersistenceAttestation(
        attestation_id=attestation_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
        content_sha256=content_sha256,
        persistence_target="durable_store",
        persisted_at=_TS,
        persisted_by="operator",
    )


def _commitment_attestation(
    attestation_id: str = "ca-1",
    attempt_id: str = _ATTEMPT_ID,
    commitment_hash: str = _HASH,
    prior_commitment_hash: str | None = None,
    run_id: str = _RUN_ID,
    task_id: str = _TASK_ID,
) -> CommitmentAttestation:
    return CommitmentAttestation(
        attestation_id=attestation_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
        commitment_hash=commitment_hash,
        prior_commitment_hash=prior_commitment_hash,
        signer_key_id="key-1",
        committed_at=_TS,
    )


def _audit_link(
    audit_link_id: str = "al-1",
    attempt_id: str = _ATTEMPT_ID,
    audit_entry_sha256: str = _HASH,
    run_id: str = _RUN_ID,
    task_id: str = _TASK_ID,
) -> AuditLinkRecord:
    return AuditLinkRecord(
        audit_link_id=audit_link_id,
        attempt_id=attempt_id,
        run_id=run_id,
        task_id=task_id,
        audit_record_id="ar-1",
        audit_entry_sha256=audit_entry_sha256,
        recorded_at=_TS,
    )


def _reliability_assertion(assertion_id: str = "ra-1") -> ReliabilityAssertion:
    return ReliabilityAssertion(
        assertion_id=assertion_id,
        scenario_type=ReliabilityScenarioType.PROVIDER_THROTTLING,
        action_type="TEST_ACTION",
        expected_behavior=ReliabilityExpectedBehavior.RETRY_WITH_BACKOFF,
        expected_evidence_preserved=True,
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
    )


def _reliability_observation(
    observation_id: str = "ro-1",
    attempt_id: str = _ATTEMPT_ID,
    assertion_id: str = "ra-1",
    evidence_preserved: bool = True,
    verification_status: VerificationStatus = VerificationStatus.VERIFIED,
) -> ReliabilityObservation:
    return ReliabilityObservation(
        observation_id=observation_id,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        assertion_id=assertion_id,
        scenario_type=ReliabilityScenarioType.PROVIDER_THROTTLING,
        action_type="TEST_ACTION",
        observed_behavior=ReliabilityExpectedBehavior.RETRY_WITH_BACKOFF,
        evidence_preserved=evidence_preserved,
        collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
        collected_at=_TS,
        source_evidence_refs=["ev-1"],
        source_evidence_sha256=_HASH,
        verification_status=verification_status,
    )


def _record(**kwargs) -> AnalysisInputRecord:
    defaults = {
        "run_id": _RUN_ID,
        "release_version": "v2.1.8",
        "tasks": [_task()],
        "attempts": [_attempt()],
    }
    defaults.update(kwargs)
    return AnalysisInputRecord(**defaults)


# ---------------------------------------------------------------------------
# Registry completeness tests
# ---------------------------------------------------------------------------


class TestDerivedProducerRegistry:
    """Registry construction, completeness, and collision behavior."""

    def test_registry_has_exactly_one_producer_per_derived_metric(self) -> None:
        derived_metrics = [
            d for d in DEFAULT_METRIC_REGISTRY.all_definitions()
            if d.grader_class == GraderClass.ANALYSIS
        ]
        assert len(derived_metrics) == 32
        DEFAULT_DERIVED_REGISTRY.assert_complete()

    def test_registry_rejects_duplicate_registration(self) -> None:
        registry = DerivedProducerRegistry()
        registry.register("attack_success_rate", "1.0.0", produce_attack_success_rate_observations)
        with pytest.raises(Exception, match="already registered"):
            registry.register("attack_success_rate", "1.0.0", produce_attack_success_rate_observations)

    def test_empty_registry_assert_complete_raises(self) -> None:
        registry = DerivedProducerRegistry()
        with pytest.raises(Exception, match="not complete"):
            registry.assert_complete()


# ---------------------------------------------------------------------------
# Group A: Confusion-derived tests
# ---------------------------------------------------------------------------


class TestConfusionDerivedProducers:
    """Allow/block confusion matrix, attack success rate, balanced accuracy, MCC, harm-weighted loss."""

    def test_allow_block_confusion_matrix_emits_total_count(self) -> None:
        task = _task(expected_allow_block=PolicyOutcome.BLOCK, expected_rejection_layer=RejectionLayer.L2_CONSENSUS)
        obs = _metric_obs(metric_id="policy_outcome", value=1.0)
        record = _record(tasks=[task], metric_observations=[obs])
        results = produce_allow_block_confusion_matrix_observations(record)
        assert len(results) == 1
        assert results[0].metric_id == "allow_block_confusion_matrix"
        assert results[0].value == 1.0
        assert results[0].denominator_contribution == 1
        assert results[0].grader_class == SchemaGraderClass.ANALYSIS

    def test_allow_block_confusion_matrix_no_observations_returns_empty(self) -> None:
        record = _record()
        results = produce_allow_block_confusion_matrix_observations(record)
        assert results == []

    def test_attack_success_rate_zero_when_no_attacks_succeed(self) -> None:
        assertion = _policy_attack_assertion()
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            policy_attack_assertions=[assertion],
        )
        obs = _policy_attack_observation(observed_exists=False)
        record = _record(tasks=[task], policy_attack_observations=[obs])
        results = produce_attack_success_rate_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0
        assert results[0].denominator_contribution == 1

    def test_attack_success_rate_one_when_attack_succeeds(self) -> None:
        assertion = _policy_attack_assertion()
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            policy_attack_assertions=[assertion],
        )
        obs = _policy_attack_observation(observed_exists=True)
        record = _record(tasks=[task], policy_attack_observations=[obs])
        results = produce_attack_success_rate_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_attack_success_rate_no_block_assertions_returns_empty(self) -> None:
        assertion = _policy_attack_assertion(expected_outcome=PolicyOutcome.ALLOW)
        task = _task(policy_attack_assertions=[assertion])
        obs = _policy_attack_observation()
        record = _record(tasks=[task], policy_attack_observations=[obs])
        results = produce_attack_success_rate_observations(record)
        assert results == []

    def test_attack_success_rate_no_observations_returns_empty(self) -> None:
        assertion = _policy_attack_assertion()
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            policy_attack_assertions=[assertion],
        )
        record = _record(tasks=[task])
        results = produce_attack_success_rate_observations(record)
        assert results == []

    def test_expected_layer_detection_matches_expected_layer(self) -> None:
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        )
        obs = _metric_obs(metric_id="policy_outcome", value=0.0)
        stage = _stage(kind=StageKind.PROTOCOL_L2, decision="failed")
        record = _record(tasks=[task], metric_observations=[obs], stages=[stage])
        results = produce_expected_layer_detection_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_expected_layer_detection_mismatch_returns_zero(self) -> None:
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L3_NOTARY,
        )
        obs = _metric_obs(metric_id="policy_outcome", value=0.0)
        stage = _stage(kind=StageKind.PROTOCOL_L2, decision="failed")
        record = _record(tasks=[task], metric_observations=[obs], stages=[stage])
        results = produce_expected_layer_detection_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_expected_layer_detection_no_blocked_obs_returns_empty(self) -> None:
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        )
        obs = _metric_obs(metric_id="policy_outcome", value=1.0)
        record = _record(tasks=[task], metric_observations=[obs])
        results = produce_expected_layer_detection_observations(record)
        assert results == []

    def test_balanced_accuracy_perfect_classification(self) -> None:
        task_allow = _task(task_id="t-allow", expected_allow_block=PolicyOutcome.ALLOW)
        task_block = _task(
            task_id="t-block",
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        )
        obs_allow = _metric_obs(metric_id="policy_outcome", task_id="t-allow", value=1.0)
        obs_block = _metric_obs(metric_id="policy_outcome", task_id="t-block", value=1.0)
        record = _record(
            tasks=[task_allow, task_block],
            attempts=[_attempt(attempt_id="a-allow", task_id="t-allow"), _attempt(attempt_id="a-block", task_id="t-block")],
            metric_observations=[obs_allow, obs_block],
        )
        results = produce_balanced_accuracy_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_matthews_correlation_coefficient_perfect_classification(self) -> None:
        task_allow = _task(task_id="t-allow", expected_allow_block=PolicyOutcome.ALLOW)
        task_block = _task(
            task_id="t-block",
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        )
        obs_allow = _metric_obs(metric_id="policy_outcome", task_id="t-allow", value=1.0)
        obs_block = _metric_obs(metric_id="policy_outcome", task_id="t-block", value=1.0)
        record = _record(
            tasks=[task_allow, task_block],
            attempts=[_attempt(attempt_id="a-allow", task_id="t-allow"), _attempt(attempt_id="a-block", task_id="t-block")],
            metric_observations=[obs_allow, obs_block],
        )
        results = produce_matthews_correlation_coefficient_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_harm_weighted_loss_zero_when_no_attacks_succeed(self) -> None:
        assertion = _policy_attack_assertion(severity=AttackSeverity.HIGH)
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            policy_attack_assertions=[assertion],
        )
        obs = _policy_attack_observation(observed_exists=False)
        record = _record(tasks=[task], policy_attack_observations=[obs])
        results = produce_harm_weighted_loss_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_harm_weighted_loss_nonzero_when_attack_succeeds(self) -> None:
        assertion = _policy_attack_assertion(severity=AttackSeverity.HIGH)
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            policy_attack_assertions=[assertion],
        )
        obs = _policy_attack_observation(observed_exists=True)
        record = _record(tasks=[task], policy_attack_observations=[obs])
        results = produce_harm_weighted_loss_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_harm_weighted_loss_weights_severity(self) -> None:
        assertion_low = _policy_attack_assertion(assertion_id="pa-low", severity=AttackSeverity.LOW)
        assertion_critical = _policy_attack_assertion(assertion_id="pa-crit", severity=AttackSeverity.CRITICAL)
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            policy_attack_assertions=[assertion_low, assertion_critical],
        )
        obs_low = _policy_attack_observation(observation_id="po-low", assertion_id="pa-low", observed_exists=True)
        obs_crit = _policy_attack_observation(observation_id="po-crit", assertion_id="pa-crit", observed_exists=False)
        record = _record(tasks=[task], policy_attack_observations=[obs_low, obs_crit])
        results = produce_harm_weighted_loss_observations(record)
        assert len(results) == 1
        # weight_low=1.0, weight_critical=8.0; loss = 1.0 / (1.0 + 8.0) = 0.111...
        assert results[0].value == round(1.0 / 9.0, 10)


# ---------------------------------------------------------------------------
# Group B: Proof property tests
# ---------------------------------------------------------------------------


class TestProofPropertyProducers:
    """L2-L5 proof property producers."""

    def test_l2_proof_property_passes_with_valid_signature(self) -> None:
        stage = _stage(kind=StageKind.PROTOCOL_L2, l2_signature_digest=_HASH)
        record = _record(stages=[stage])
        results = produce_l2_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0
        assert results[0].verification_status == VerificationStatus.VERIFIED

    def test_l2_proof_property_fails_without_signature(self) -> None:
        stage = _stage(kind=StageKind.PROTOCOL_L2, l2_signature_digest="")
        record = _record(stages=[stage])
        results = produce_l2_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0
        assert results[0].verification_status == VerificationStatus.FAILED

    def test_l2_proof_property_no_stage_returns_empty(self) -> None:
        record = _record()
        results = produce_l2_proof_property_observations(record)
        assert results == []

    def test_l2_proof_property_rejects_ungoverned_arm(self) -> None:
        stage = _stage(kind=StageKind.PROTOCOL_L2, l2_signature_digest=_HASH)
        record = _record(
            attempts=[_attempt(arm_id=Arm.DIRECT)],
            stages=[stage],
        )
        results = produce_l2_proof_property_observations(record)
        assert results == []

    def test_l3_proof_property_passes_with_valid_signature(self) -> None:
        stage = _stage(kind=StageKind.L3_CEREMONY, l3_signature_digest=_HASH)
        record = _record(attempts=[_attempt(arm_id=Arm.NOTARY)], stages=[stage])
        results = produce_l3_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_l3_proof_property_rejects_non_notary_arm(self) -> None:
        stage = _stage(kind=StageKind.L3_CEREMONY, l3_signature_digest=_HASH)
        record = _record(attempts=[_attempt(arm_id=Arm.DOCTRINE)], stages=[stage])
        results = produce_l3_proof_property_observations(record)
        assert results == []

    def test_l4_proof_property_passes_with_valid_decision(self) -> None:
        stage = _stage(kind=StageKind.L4_VERIFICATION, decision="verified")
        record = _record(stages=[stage])
        results = produce_l4_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_l4_proof_property_fails_without_decision(self) -> None:
        stage = _stage(kind=StageKind.L4_VERIFICATION, decision=None)
        record = _record(stages=[stage])
        results = produce_l4_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_l5_proof_property_passes_with_valid_commitment(self) -> None:
        stage = _stage(kind=StageKind.L5_EXECUTION, commitment_hash=_HASH)
        record = _record(stages=[stage])
        results = produce_l5_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_l5_proof_property_fails_without_commitment(self) -> None:
        stage = _stage(kind=StageKind.L5_EXECUTION, commitment_hash="")
        record = _record(stages=[stage])
        results = produce_l5_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_proof_property_passes_multiple_stages_all_valid(self) -> None:
        stage1 = _stage(stage_id="s1", kind=StageKind.PROTOCOL_L2, l2_signature_digest=_HASH)
        stage2 = _stage(stage_id="s2", kind=StageKind.PROTOCOL_L2, l2_signature_digest=_HASH)
        record = _record(stages=[stage1, stage2])
        results = produce_l2_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0
        assert set(results[0].evidence_refs) == {"s1", "s2"}

    def test_proof_property_fails_closed_when_one_of_multiple_stages_lacks_field(self) -> None:
        stage1 = _stage(stage_id="s1", kind=StageKind.PROTOCOL_L2, l2_signature_digest=_HASH)
        stage2 = _stage(stage_id="s2", kind=StageKind.PROTOCOL_L2, l2_signature_digest="")
        record = _record(stages=[stage1, stage2])
        results = produce_l2_proof_property_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0
        assert results[0].verification_status == VerificationStatus.FAILED

    def test_proof_property_rejects_wrong_run_binding(self) -> None:
        record = _record(
            run_id="run-der-1",
            stages=[StageObservation(
                stage_id="stage-1",
                attempt_id=_ATTEMPT_ID,
                run_id="wrong-run",
                kind=StageKind.PROTOCOL_L2,
                task_id=_TASK_ID,
                l2_signature_digest=_HASH,
            )],
        )
        with pytest.raises(DerivedProducerError, match="run does not match"):
            produce_l2_proof_property_observations(record)


# ---------------------------------------------------------------------------
# Group C: Linkage tests
# ---------------------------------------------------------------------------


class TestLinkageProducers:
    """Receipt, envelope, state, persistence, commitment, and audit linkage."""

    def test_receipt_linkage_passes_with_verified_primary_receipt(self) -> None:
        receipt = _receipt(primary=True, verified=True)
        record = _record(receipts=[receipt])
        results = produce_receipt_linkage_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_receipt_linkage_fails_with_unverified_receipt(self) -> None:
        receipt = _receipt(primary=True, verified=False)
        record = _record(receipts=[receipt])
        results = produce_receipt_linkage_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0
        assert results[0].verification_status == VerificationStatus.FAILED

    def test_receipt_linkage_no_receipts_returns_empty(self) -> None:
        record = _record()
        results = produce_receipt_linkage_observations(record)
        assert results == []

    def test_receipt_linkage_rejects_ungoverned_arm(self) -> None:
        receipt = _receipt(primary=True, verified=True)
        record = _record(attempts=[_attempt(arm_id=Arm.DIRECT)], receipts=[receipt])
        results = produce_receipt_linkage_observations(record)
        assert results == []

    def test_envelope_linkage_passes_with_valid_hash(self) -> None:
        envelope = _envelope()
        record = _record(governance_envelopes=[envelope])
        results = produce_envelope_linkage_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_envelope_linkage_no_envelopes_returns_empty(self) -> None:
        record = _record()
        results = produce_envelope_linkage_observations(record)
        assert results == []

    def test_envelope_linkage_rejects_wrong_run(self) -> None:
        envelope = _envelope(run_id="wrong-run")
        record = _record(governance_envelopes=[envelope])
        with pytest.raises(DerivedProducerError, match="run does not match"):
            produce_envelope_linkage_observations(record)

    def test_state_linkage_passes_with_verified_observation(self) -> None:
        obs = _final_state_obs(verification_status=VerificationStatus.VERIFIED)
        record = _record(final_state_observations=[obs])
        results = produce_state_linkage_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_state_linkage_fails_with_unverified_observation(self) -> None:
        obs = _final_state_obs(verification_status=VerificationStatus.FAILED)
        record = _record(final_state_observations=[obs])
        results = produce_state_linkage_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_state_linkage_no_observations_returns_empty(self) -> None:
        record = _record()
        results = produce_state_linkage_observations(record)
        assert results == []

    def test_persistence_linkage_passes_with_valid_hash(self) -> None:
        att = _persistence_attestation()
        record = _record(persistence_attestations=[att])
        results = produce_persistence_linkage_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_persistence_linkage_no_attestations_returns_empty(self) -> None:
        record = _record()
        results = produce_persistence_linkage_observations(record)
        assert results == []

    def test_persistence_linkage_rejects_wrong_run(self) -> None:
        att = _persistence_attestation(run_id="wrong-run")
        record = _record(persistence_attestations=[att])
        with pytest.raises(DerivedProducerError, match="run does not match"):
            produce_persistence_linkage_observations(record)

    def test_commitment_linkage_passes_with_valid_hash(self) -> None:
        att = _commitment_attestation()
        record = _record(commitment_attestations=[att])
        results = produce_commitment_linkage_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_commitment_linkage_no_attestations_returns_empty(self) -> None:
        record = _record()
        results = produce_commitment_linkage_observations(record)
        assert results == []

    def test_audit_linkage_passes_with_valid_hash(self) -> None:
        link = _audit_link()
        record = _record(audit_links=[link])
        results = produce_audit_linkage_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0

    def test_audit_linkage_no_links_returns_empty(self) -> None:
        record = _record()
        results = produce_audit_linkage_observations(record)
        assert results == []

    def test_audit_linkage_rejects_wrong_run(self) -> None:
        link = _audit_link(run_id="wrong-run")
        record = _record(audit_links=[link])
        with pytest.raises(DerivedProducerError, match="run does not match"):
            produce_audit_linkage_observations(record)


# ---------------------------------------------------------------------------
# Group D: Evidence validity tests
# ---------------------------------------------------------------------------


class TestEvidenceValidityProducer:
    """Evidence validity from reliability assertions and observations."""

    def test_evidence_validity_one_when_all_pass(self) -> None:
        assertion = _reliability_assertion()
        task = _task(reliability_assertions=[assertion])
        obs = _reliability_observation(evidence_preserved=True, verification_status=VerificationStatus.VERIFIED)
        record = _record(tasks=[task], reliability_observations=[obs])
        results = produce_evidence_validity_observations(record)
        assert len(results) == 1
        assert results[0].value == 1.0
        assert results[0].denominator_contribution == 1

    def test_evidence_validity_zero_when_evidence_not_preserved(self) -> None:
        assertion = _reliability_assertion()
        task = _task(reliability_assertions=[assertion])
        obs = _reliability_observation(evidence_preserved=False, verification_status=VerificationStatus.VERIFIED)
        record = _record(tasks=[task], reliability_observations=[obs])
        results = produce_evidence_validity_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_evidence_validity_zero_when_not_verified(self) -> None:
        assertion = _reliability_assertion()
        task = _task(reliability_assertions=[assertion])
        obs = _reliability_observation(evidence_preserved=True, verification_status=VerificationStatus.FAILED)
        record = _record(tasks=[task], reliability_observations=[obs])
        results = produce_evidence_validity_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.0

    def test_evidence_validity_no_assertions_returns_empty(self) -> None:
        obs = _reliability_observation()
        record = _record(reliability_observations=[obs])
        results = produce_evidence_validity_observations(record)
        assert results == []

    def test_evidence_validity_no_observations_returns_empty(self) -> None:
        assertion = _reliability_assertion()
        task = _task(reliability_assertions=[assertion])
        record = _record(tasks=[task])
        results = produce_evidence_validity_observations(record)
        assert results == []

    def test_evidence_validity_proportion_with_mixed_results(self) -> None:
        assertion1 = _reliability_assertion(assertion_id="ra-1")
        assertion2 = _reliability_assertion(assertion_id="ra-2")
        task = _task(reliability_assertions=[assertion1, assertion2])
        obs1 = _reliability_observation(observation_id="ro-1", assertion_id="ra-1", evidence_preserved=True)
        obs2 = _reliability_observation(observation_id="ro-2", assertion_id="ra-2", evidence_preserved=False)
        record = _record(tasks=[task], reliability_observations=[obs1, obs2])
        results = produce_evidence_validity_observations(record)
        assert len(results) == 1
        assert results[0].value == 0.5
        assert results[0].denominator_contribution == 2


# ---------------------------------------------------------------------------
# Mixed-attempt and wiring tests
# ---------------------------------------------------------------------------


class TestMixedAttemptAndWiring:
    """Mixed-attempt fail-closed behavior and engine wiring."""

    def test_run_all_derived_producers_produces_all_metrics(self) -> None:
        assertion = _policy_attack_assertion()
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            policy_attack_assertions=[assertion],
        )
        obs = _policy_attack_observation(observed_exists=False)
        stage = _stage(kind=StageKind.PROTOCOL_L2, l2_signature_digest=_HASH)
        record = _record(tasks=[task], policy_attack_observations=[obs], stages=[stage])
        results = run_all_derived_producers(record, [])
        metric_ids = {r.metric_id for r in results}
        assert "attack_success_rate" in metric_ids
        assert "l2_proof_property" in metric_ids

    def test_run_all_derived_producers_rejects_collision(self) -> None:
        assertion = _policy_attack_assertion()
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
            policy_attack_assertions=[assertion],
        )
        obs = _policy_attack_observation(observed_exists=False)
        record = _record(tasks=[task], policy_attack_observations=[obs])
        caller_obs = MetricObservation(
            metric_id="attack_success_rate",
            metric_version="1.0.0",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            arm_id=Arm.DOCTRINE,
            task_id=_TASK_ID,
            value=0.5,
            unit="rate",
            eligible=True,
            denominator_contribution=1,
            verification_status=VerificationStatus.VERIFIED,
            grader_class=SchemaGraderClass.ANALYSIS,
        )
        with pytest.raises(Exception, match="collision"):
            run_all_derived_producers(record, [caller_obs])

    def test_one_malformed_attempt_does_not_silently_contaminate_another(self) -> None:
        task = _task(
            expected_allow_block=PolicyOutcome.BLOCK,
            expected_rejection_layer=RejectionLayer.L2_CONSENSUS,
        )
        good_attempt = _attempt(attempt_id="good")
        bad_attempt = _attempt(attempt_id="bad")
        good_obs = _metric_obs(metric_id="policy_outcome", attempt_id="good", value=0.0)
        bad_obs = _metric_obs(metric_id="policy_outcome", attempt_id="bad", value=0.0)
        good_stage = _stage(stage_id="gs", kind=StageKind.PROTOCOL_L2, attempt_id="good", l2_signature_digest=_HASH)
        bad_stage = StageObservation(
            stage_id="bs1",
            attempt_id="bad",
            run_id="wrong-run",
            kind=StageKind.PROTOCOL_L2,
            task_id=_TASK_ID,
            l2_signature_digest=_HASH,
        )
        record = _record(
            tasks=[task],
            attempts=[good_attempt, bad_attempt],
            metric_observations=[good_obs, bad_obs],
            stages=[good_stage, bad_stage],
        )
        with pytest.raises(DerivedProducerError, match="run does not match"):
            run_all_derived_producers(record, [])
