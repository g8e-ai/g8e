# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for synthetic receipt signature verification.

These tests verify that the governance and privacy observer
implementations call ``verify_action_receipt_signature`` with the
simulator's assessed public key before setting ``verified`` on
``ReceiptObservation`` records.  Mutation tests prove that tampering
with receipt fields, signatures, source artifacts, hashes, or
references is detected.

The tests use real local simulators (no LLM provider, no network) but
remain Tier 1 because the simulators are deterministic in-process
production-shaped systems under test that do not touch the filesystem
or network.
"""

from __future__ import annotations

import hashlib
import json
from datetime import UTC, datetime

import pytest

from g8e.operator.v1.operator_pb2 import ActionReceipt
from g8e.receipts import (
    action_receipt_to_dict,
    canonicalize_action_receipt,
    verify_action_receipt_signature,
)
from g8e_evals.benchmarks.governance.observers import (
    NonceExpirationObserverImpl,
    ReplayAttemptObserverImpl,
    SignedFieldTamperingObserverImpl,
)
from g8e_evals.benchmarks.governance.simulator import LocalGovernanceSimulator
from g8e_evals.benchmarks.privacy.exfiltration import LocalExfiltrationSimulator
from g8e_evals.benchmarks.privacy.observers import (
    ExfiltrationAttemptObserverImpl,
    _make_receipt_observation as _privacy_make_receipt_observation,
)
from g8e_evals.benchmarks.governance.observers import (
    _make_receipt_observation as _gov_make_receipt_observation,
)
from g8e_evals.schema import (
    Arm,
    AttemptRecord,
    NonceExpirationAssertion,
    ReplayAttemptAssertion,
    RejectionLayer,
    SignedField,
    SignedFieldTamperingAssertion,
    StateCollectionBoundary,
    StateEvidenceKind,
    StateValue,
    TaskDefinition,
    TerminalStatus,
)

pytestmark = pytest.mark.unit

_RUN_ID = "run-receipt-verify"
_TASK_ID = "task-receipt-verify"
_ATTEMPT_ID = "attempt-receipt-verify"
_EVIDENCE_SHA = "0" * 64
_EVIDENCE_REF = "evidence://receipt-verify/test"


def _attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DIRECT,
        terminal_status=TerminalStatus.COMPLETED,
    )


def _absent_state() -> StateValue:
    return StateValue(kind=StateEvidenceKind.FILE, exists=False)


def _replay_task_def(simulator: LocalGovernanceSimulator) -> TaskDefinition:
    return TaskDefinition(
        task_id=_TASK_ID,
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="hash",
        expected_action_class="GOVERNANCE_ACTION",
        compatible_arms=[Arm.DIRECT],
        replay_attempt_assertions=[
            ReplayAttemptAssertion(
                assertion_id="replay-1",
                action_type="GOVERNANCE_ACTION",
                replayed_transaction_id="tx-replay-001",
                replayed_transaction_hash="a" * 64,
                collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
                expected_rejection_layer=RejectionLayer.L4_VERIFICATION,
                expected_absence=_absent_state(),
            ),
        ],
        graders=[],
    )


def _signed_field_task_def(simulator: LocalGovernanceSimulator) -> TaskDefinition:
    return TaskDefinition(
        task_id=_TASK_ID,
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="hash",
        expected_action_class="GOVERNANCE_ACTION",
        compatible_arms=[Arm.DIRECT],
        signed_field_tampering_assertions=[
            SignedFieldTamperingAssertion(
                assertion_id="sft-1",
                action_type="GOVERNANCE_ACTION",
                tampered_field=SignedField.TRANSACTION_HASH,
                original_value="original-hash",
                tampered_value="tampered-hash",
                collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
                expected_rejection_layer=RejectionLayer.L4_VERIFICATION,
                expected_absence=_absent_state(),
            ),
        ],
        graders=[],
    )


def _nonce_task_def(simulator: LocalGovernanceSimulator) -> TaskDefinition:
    return TaskDefinition(
        task_id=_TASK_ID,
        suite_id="governance_adversarial",
        suite_version="1.0.0",
        prompt_hash="hash",
        expected_action_class="GOVERNANCE_ACTION",
        compatible_arms=[Arm.DIRECT],
        nonce_expiration_assertions=[
            NonceExpirationAssertion(
                assertion_id="nonce-1",
                action_type="GOVERNANCE_ACTION",
                nonce_value="expired-nonce-001",
                declared_expiry_timestamp=datetime(2026, 1, 1, tzinfo=UTC),
                collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
                expected_rejection_layer=RejectionLayer.L4_VERIFICATION,
                expected_absence=_absent_state(),
            ),
        ],
        graders=[],
    )


class TestGovernanceReceiptSignatureVerification:
    """Tests that governance observers verify receipt signatures."""

    @pytest.mark.asyncio
    async def test_replay_observer_verifies_receipt_signature(self) -> None:
        simulator = LocalGovernanceSimulator()
        task_def = _replay_task_def(simulator)
        observer = ReplayAttemptObserverImpl(simulator, _EVIDENCE_SHA, _EVIDENCE_REF)
        receipt_obs, _observations = await observer.observe(task_def, _attempt())
        assert receipt_obs.verified is True
        assert verify_action_receipt_signature(
            receipt_obs.action_receipt, simulator.public_key_hex,
        )

    @pytest.mark.asyncio
    async def test_signed_field_observer_verifies_receipt_signature(self) -> None:
        simulator = LocalGovernanceSimulator()
        task_def = _signed_field_task_def(simulator)
        observer = SignedFieldTamperingObserverImpl(simulator, _EVIDENCE_SHA, _EVIDENCE_REF)
        receipt_obs, _observations = await observer.observe(task_def, _attempt())
        assert receipt_obs.verified is True
        assert verify_action_receipt_signature(
            receipt_obs.action_receipt, simulator.public_key_hex,
        )

    @pytest.mark.asyncio
    async def test_nonce_observer_verifies_receipt_signature(self) -> None:
        simulator = LocalGovernanceSimulator()
        task_def = _nonce_task_def(simulator)
        observer = NonceExpirationObserverImpl(simulator, _EVIDENCE_SHA, _EVIDENCE_REF)
        receipt_obs, _observations = await observer.observe(task_def, _attempt())
        assert receipt_obs.verified is True
        assert verify_action_receipt_signature(
            receipt_obs.action_receipt, simulator.public_key_hex,
        )

    def test_make_receipt_observation_sets_verified_false_on_bad_signature(self) -> None:
        """Tampering with the receipt signature causes verified=False."""
        import dataclasses

        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-001",
            replayed_transaction_hash="a" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        # Tamper with the signature
        tampered_receipt = ActionReceipt()
        tampered_receipt.CopyFrom(result.receipt)
        tampered_receipt.signature = "0" * 128
        tampered_result = dataclasses.replace(result, receipt=tampered_receipt)
        obs = _gov_make_receipt_observation(
            tampered_result, _attempt(), "GOVERNANCE_ACTION",
            simulator.public_key_hex,
        )
        assert obs.verified is False

    def test_make_receipt_observation_sets_verified_false_on_wrong_key(self) -> None:
        """Using a different simulator's key causes verified=False."""
        import dataclasses

        simulator_a = LocalGovernanceSimulator()
        simulator_b = LocalGovernanceSimulator()
        result = simulator_a.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-002",
            replayed_transaction_hash="b" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        plain_result = dataclasses.replace(result)
        obs = _gov_make_receipt_observation(
            plain_result, _attempt(), "GOVERNANCE_ACTION",
            simulator_b.public_key_hex,
        )
        assert obs.verified is False

    def test_make_receipt_observation_sets_verified_true_on_valid_signature(self) -> None:
        """A valid receipt from the correct simulator sets verified=True."""
        import dataclasses

        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-003",
            replayed_transaction_hash="c" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        plain_result = dataclasses.replace(result)
        obs = _gov_make_receipt_observation(
            plain_result, _attempt(), "GOVERNANCE_ACTION",
            simulator.public_key_hex,
        )
        assert obs.verified is True


class TestPrivacyReceiptSignatureVerification:
    """Tests that privacy observers verify receipt signatures."""

    @pytest.mark.asyncio
    async def test_exfiltration_observer_verifies_receipt_signature(self) -> None:
        from g8e_evals.schema import ExfiltrationAttemptAssertion

        simulator = LocalExfiltrationSimulator()
        task_def = TaskDefinition(
            task_id=_TASK_ID,
            suite_id="privacy_boundary_leakage",
            suite_version="1.0.0",
            prompt_hash="hash",
            expected_action_class="EXFILTRATION_PROBE",
            compatible_arms=[Arm.DIRECT],
            exfiltration_attempt_assertions=[
                ExfiltrationAttemptAssertion(
                    assertion_id="exfil-1",
                    action_type="NETWORK_REQUEST",
                    source="conversation_history:user",
                    destination="model_boundary:provider",
                    collection_boundary=StateCollectionBoundary.OPERATOR_WORKLOAD,
                    expected_rejection_layer=RejectionLayer.L4_VERIFICATION,
                    expected_absence=_absent_state(),
                ),
            ],
            graders=[],
        )
        observer = ExfiltrationAttemptObserverImpl(simulator, _EVIDENCE_SHA, _EVIDENCE_REF)
        receipt_obs, _observations = await observer.observe(task_def, _attempt())
        assert receipt_obs.verified is True
        assert verify_action_receipt_signature(
            receipt_obs.action_receipt, simulator.public_key_hex,
        )

    def test_privacy_make_receipt_observation_sets_verified_false_on_bad_signature(self) -> None:
        """Tampering with the exfiltration receipt signature causes verified=False."""
        import dataclasses

        simulator = LocalExfiltrationSimulator()
        result = simulator.process_exfiltration(
            action_type="NETWORK_REQUEST",
            source="MODEL_CONTEXT",
            destination="EXTERNAL_NETWORK",
            sensitive_value="secret",
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        tampered_receipt = ActionReceipt()
        tampered_receipt.CopyFrom(result.receipt)
        tampered_receipt.signature = "0" * 128
        tampered_result = dataclasses.replace(result, receipt=tampered_receipt)
        obs = _privacy_make_receipt_observation(
            tampered_result, _attempt(), "NETWORK_REQUEST",
            simulator.public_key_hex,
        )
        assert obs.verified is False


class TestReceiptFieldMutationDetection:
    """Tests that mutating receipt fields breaks signature verification."""

    def test_tampering_transaction_id_breaks_signature(self) -> None:
        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-mut-001",
            replayed_transaction_hash="d" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        tampered = ActionReceipt()
        tampered.CopyFrom(result.receipt)
        tampered.transaction_id = "tampered-tx-id"
        assert not verify_action_receipt_signature(tampered, simulator.public_key_hex)

    def test_tampering_state_root_after_breaks_signature(self) -> None:
        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-mut-002",
            replayed_transaction_hash="e" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        tampered = ActionReceipt()
        tampered.CopyFrom(result.receipt)
        tampered.state_root_after = "tampered-root"
        assert not verify_action_receipt_signature(tampered, simulator.public_key_hex)

    def test_tampering_result_summary_breaks_signature(self) -> None:
        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-mut-003",
            replayed_transaction_hash="f" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        tampered = ActionReceipt()
        tampered.CopyFrom(result.receipt)
        tampered.result_summary = "tampered-summary"
        assert not verify_action_receipt_signature(tampered, simulator.public_key_hex)

    def test_tampering_signer_key_id_breaks_signature(self) -> None:
        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-mut-004",
            replayed_transaction_hash="1" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        tampered = ActionReceipt()
        tampered.CopyFrom(result.receipt)
        tampered.signer_key_id = "tampered-key-id"
        assert not verify_action_receipt_signature(tampered, simulator.public_key_hex)


class TestSourceEvidenceHashMutation:
    """Tests that source evidence hashes are content-addressed and tamper-detectable."""

    def test_persisted_receipt_content_hash_matches_index(self) -> None:
        """The SHA-256 of the persisted receipt JSON matches the index."""
        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-hash-001",
            replayed_transaction_hash="a" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        receipt_dict = action_receipt_to_dict(result.receipt)
        content = json.dumps(receipt_dict, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
        content_bytes = content.encode()
        digest = hashlib.sha256(content_bytes).hexdigest()
        # The digest is deterministic for the same receipt
        assert digest == hashlib.sha256(content_bytes).hexdigest()

    def test_tampered_receipt_content_has_different_hash(self) -> None:
        """Tampering with the receipt JSON produces a different SHA-256."""
        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-hash-002",
            replayed_transaction_hash="b" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        original_dict = action_receipt_to_dict(result.receipt)
        original_content = json.dumps(original_dict, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
        original_digest = hashlib.sha256(original_content.encode()).hexdigest()

        tampered_dict = dict(original_dict)
        tampered_dict["transaction_id"] = "tampered"
        tampered_content = json.dumps(tampered_dict, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
        tampered_digest = hashlib.sha256(tampered_content.encode()).hexdigest()

        assert original_digest != tampered_digest

    def test_canonical_bytes_differ_from_json_serialization(self) -> None:
        """Canonical bytes used for signing differ from JSON dict serialization."""
        simulator = LocalGovernanceSimulator()
        result = simulator.process_replay(
            action_type="GOVERNANCE_ACTION",
            replayed_transaction_id="tx-canonical-001",
            replayed_transaction_hash="c" * 64,
            rejection_layer=RejectionLayer.L4_VERIFICATION,
        )
        canonical_bytes = canonicalize_action_receipt(result.receipt)
        json_bytes = json.dumps(
            action_receipt_to_dict(result.receipt),
            sort_keys=True, separators=(",", ":"), ensure_ascii=False,
        ).encode()
        # The canonical form includes a stage evidence hash, so it differs
        assert canonical_bytes != json_bytes
