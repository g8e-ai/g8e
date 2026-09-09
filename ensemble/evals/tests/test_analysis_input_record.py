# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for the complete immutable analysis input record.

Verifies the ``AnalysisInputRecord`` model structure (frozen,
extra-forbid), field presence, default empty lists, deterministic
serialization, and that the record can carry every observation class
referenced by ``AttemptRecord`` plus envelopes, attestations, audit
links, and a typed price table. No external dependencies (no files,
network, or DB).
"""

from __future__ import annotations

from datetime import UTC, datetime

import pytest
from pydantic import ValidationError

pytestmark = pytest.mark.unit

from g8e_evals.analysis import AnalysisInputRecord
from g8e_evals.analysis import compute_canonical_analysis_from_record
from g8e_evals.analysis.canonical import canonical_model_json
from g8e_evals.arms import Arm
from g8e_evals.schema import (
    AttemptRecord,
    AuditLinkRecord,
    CommitmentAttestation,
    GovernanceEnvelopeRecord,
    HumanWaitObservation,
    LocalResourceObservation,
    PersistenceAttestation,
    PriceTableEntry,
    TaskDefinition,
    TerminalStatus,
    TypedPriceTable,
    VerificationStatus,
)


_RUN_ID = "run-input-1"
_RELEASE = "v2.1.8"
_TASK_ID = "task-input-1"
_ATTEMPT_ID = "attempt-input-1"
_HASH = "a" * 64
_TS = datetime(2026, 1, 1, tzinfo=UTC)


def _minimal_task() -> TaskDefinition:
    return TaskDefinition(
        task_id=_TASK_ID,
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="abc123",
        prompt_length=10,
        expected_action_class="TEST_ACTION",
        compatible_arms=[Arm.DOCTRINE],
    )


def _minimal_attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
    )


def _local_resource_obs() -> LocalResourceObservation:
    return LocalResourceObservation(
        observation_id="lr-1",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        peak_memory_bytes=1024,
        cpu_seconds=0.5,
        collected_at=_TS,
        source_evidence_refs=["evidence-1"],
        source_evidence_sha256=_HASH,
        verification_status=VerificationStatus.VERIFIED,
    )


def _human_wait_obs() -> HumanWaitObservation:
    return HumanWaitObservation(
        observation_id="hw-1",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        human_wait_seconds=3.0,
        human_action_type="approval",
        collected_at=_TS,
        source_evidence_refs=["evidence-2"],
        source_evidence_sha256=_HASH,
        verification_status=VerificationStatus.VERIFIED,
    )


def _envelope() -> GovernanceEnvelopeRecord:
    return GovernanceEnvelopeRecord(
        envelope_id="env-1",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        envelope_sha256=_HASH,
        layer_disposition="L1_ALLOW",
        signer_key_id="key-1",
        created_at=_TS,
    )


def _persistence() -> PersistenceAttestation:
    return PersistenceAttestation(
        attestation_id="pa-1",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        content_sha256=_HASH,
        persistence_target="sqlite",
        persisted_at=_TS,
        persisted_by="gateway",
    )


def _commitment() -> CommitmentAttestation:
    return CommitmentAttestation(
        attestation_id="ca-1",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        commitment_hash=_HASH,
        signer_key_id="key-1",
        committed_at=_TS,
    )


def _audit_link() -> AuditLinkRecord:
    return AuditLinkRecord(
        audit_link_id="al-1",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        audit_record_id="ar-1",
        audit_entry_sha256=_HASH,
        recorded_at=_TS,
    )


def _price_table() -> TypedPriceTable:
    return TypedPriceTable(
        price_table_id="pt-1",
        price_table_version="1.0.0",
        entries=[
            PriceTableEntry(
                provider="test-provider",
                model="test-model",
                input_token_price_usd=0.001,
                output_token_price_usd=0.002,
                effective_at=_TS,
            ),
        ],
    )


class TestAnalysisInputRecordStructure:
    """Verify the model is frozen, extra-forbid, and has all required fields."""

    def test_model_is_frozen(self) -> None:
        record = AnalysisInputRecord(run_id=_RUN_ID, release_version=_RELEASE)
        with pytest.raises(ValidationError):
            record.run_id = "other"  # type: ignore[misc]

    def test_model_rejects_extra_fields(self) -> None:
        with pytest.raises(ValidationError, match="unexpected"):
            AnalysisInputRecord(run_id=_RUN_ID, release_version=_RELEASE, unexpected="value")  # type: ignore[call-arg]

    def test_minimal_record_has_only_required_fields(self) -> None:
        record = AnalysisInputRecord(run_id=_RUN_ID, release_version=_RELEASE)
        assert record.run_id == _RUN_ID
        assert record.release_version == _RELEASE

    def test_run_id_rejects_empty_string(self) -> None:
        with pytest.raises(ValidationError, match="String should have at least 1 character"):
            AnalysisInputRecord(run_id="", release_version=_RELEASE)

    def test_release_version_rejects_empty_string(self) -> None:
        with pytest.raises(ValidationError, match="String should have at least 1 character"):
            AnalysisInputRecord(run_id=_RUN_ID, release_version="")


class TestAnalysisInputRecordDefaults:
    """Verify every optional field defaults to an empty list or None."""

    def test_all_sequence_fields_default_to_empty_list(self) -> None:
        record = AnalysisInputRecord(run_id=_RUN_ID, release_version=_RELEASE)
        assert record.tasks == []
        assert record.attempts == []
        assert record.metric_observations == []
        assert record.receipts == []
        assert record.stages == []
        assert record.final_state_observations == []
        assert record.state_observations == []
        assert record.rehydration_observations == []
        assert record.secret_detection_observations == []
        assert record.unauthorized_mutation_observations == []
        assert record.token_store_persistence_observations == []
        assert record.token_ttl_expiry_observations == []
        assert record.token_persistence_failure_observations == []
        assert record.exfiltration_attempt_observations == []
        assert record.artifact_leakage_observations == []
        assert record.replay_attempt_observations == []
        assert record.signed_field_tampering_observations == []
        assert record.payload_tampering_observations == []
        assert record.stale_state_root_observations == []
        assert record.identity_mismatch_observations == []
        assert record.nonce_expiration_observations == []
        assert record.signer_defect_observations == []
        assert record.l3_proof_transplant_observations == []
        assert record.revoked_credential_observations == []
        assert record.evidence_preservation_observations == []
        assert record.policy_attack_observations == []
        assert record.tool_sequence_observations == []
        assert record.factual_qa_observations == []
        assert record.citation_backed_observations == []
        assert record.partial_milestone_observations == []
        assert record.reliability_observations == []
        assert record.economics_performance_observations == []
        assert record.local_resource_observations == []
        assert record.human_wait_observations == []
        assert record.governance_envelopes == []
        assert record.persistence_attestations == []
        assert record.commitment_attestations == []
        assert record.audit_links == []

    def test_price_table_defaults_to_none(self) -> None:
        record = AnalysisInputRecord(run_id=_RUN_ID, release_version=_RELEASE)
        assert record.price_table is None

    def test_default_lists_are_independent_instances(self) -> None:
        r1 = AnalysisInputRecord(run_id=_RUN_ID, release_version=_RELEASE)
        r2 = AnalysisInputRecord(run_id=_RUN_ID, release_version=_RELEASE)
        assert r1.tasks is not r2.tasks
        assert r1.attempts is not r2.attempts


class TestAnalysisInputRecordFieldPresence:
    """Verify the record can carry every observation class and record type."""

    def test_carries_tasks_and_attempts(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
        )
        assert len(record.tasks) == 1
        assert record.tasks[0].task_id == _TASK_ID
        assert len(record.attempts) == 1
        assert record.attempts[0].attempt_id == _ATTEMPT_ID

    def test_carries_local_resource_observations(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            local_resource_observations=[_local_resource_obs()],
        )
        assert len(record.local_resource_observations) == 1
        assert record.local_resource_observations[0].peak_memory_bytes == 1024

    def test_carries_human_wait_observations(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            human_wait_observations=[_human_wait_obs()],
        )
        assert len(record.human_wait_observations) == 1
        assert record.human_wait_observations[0].human_wait_seconds == 3.0

    def test_carries_governance_envelopes(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            governance_envelopes=[_envelope()],
        )
        assert len(record.governance_envelopes) == 1
        assert record.governance_envelopes[0].envelope_id == "env-1"

    def test_carries_persistence_attestations(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            persistence_attestations=[_persistence()],
        )
        assert len(record.persistence_attestations) == 1

    def test_carries_commitment_attestations(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            commitment_attestations=[_commitment()],
        )
        assert len(record.commitment_attestations) == 1

    def test_carries_audit_links(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            audit_links=[_audit_link()],
        )
        assert len(record.audit_links) == 1

    def test_carries_typed_price_table(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            price_table=_price_table(),
        )
        assert record.price_table is not None
        assert record.price_table.price_table_id == "pt-1"
        assert len(record.price_table.entries) == 1


class TestAnalysisInputRecordSerialization:
    """Verify deterministic serialization via canonical JSON."""

    def test_identical_inputs_produce_identical_json(self) -> None:
        r1 = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[_local_resource_obs()],
            governance_envelopes=[_envelope()],
            price_table=_price_table(),
        )
        r2 = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[_local_resource_obs()],
            governance_envelopes=[_envelope()],
            price_table=_price_table(),
        )
        assert canonical_model_json(r1) == canonical_model_json(r2)

    def test_different_run_ids_produce_different_json(self) -> None:
        r1 = AnalysisInputRecord(run_id="run-a", release_version=_RELEASE)
        r2 = AnalysisInputRecord(run_id="run-b", release_version=_RELEASE)
        assert canonical_model_json(r1) != canonical_model_json(r2)

    def test_serialization_roundtrip_preserves_data(self) -> None:
        original = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            human_wait_observations=[_human_wait_obs()],
            audit_links=[_audit_link()],
        )
        json_str = canonical_model_json(original)
        restored = AnalysisInputRecord.model_validate_json(json_str)
        assert restored.run_id == original.run_id
        assert restored.release_version == original.release_version
        assert len(restored.tasks) == 1
        assert len(restored.attempts) == 1
        assert len(restored.human_wait_observations) == 1
        assert len(restored.audit_links) == 1


class TestComputeFromRecord:
    """Verify the record-based entry point delegates to the engine."""

    def test_compute_from_record_produces_analysis(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.run_id == _RUN_ID
        assert analysis.release_version == _RELEASE
        assert analysis.input_summary.task_count == 1
        assert analysis.input_summary.attempt_count == 1

    def test_compute_from_record_matches_compute_canonical_analysis(self) -> None:
        from g8e_evals.analysis import compute_canonical_analysis

        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
        )
        from_record = compute_canonical_analysis_from_record(record)
        from_args = compute_canonical_analysis(
            tasks=record.tasks,
            attempts=record.attempts,
            metric_observations=record.metric_observations,
            receipts=record.receipts,
            stages=record.stages,
            run_id=record.run_id,
            release_version=record.release_version,
        )
        assert canonical_model_json(from_record) == canonical_model_json(from_args)


class TestInputContentHashProtectsAllRecords:
    """Prove that input_content_hash covers every supplied record class, not just the legacy five."""

    def _base_record(self, **kwargs: object) -> AnalysisInputRecord:
        return AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            **kwargs,  # type: ignore[arg-type]
        )

    def test_local_resource_observations_affect_hash(self) -> None:
        without = self._base_record()
        with_obs = self._base_record(local_resource_observations=[_local_resource_obs()])
        a1 = compute_canonical_analysis_from_record(without)
        a2 = compute_canonical_analysis_from_record(with_obs)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash

    def test_human_wait_observations_affect_hash(self) -> None:
        without = self._base_record()
        with_obs = self._base_record(human_wait_observations=[_human_wait_obs()])
        a1 = compute_canonical_analysis_from_record(without)
        a2 = compute_canonical_analysis_from_record(with_obs)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash

    def test_governance_envelopes_affect_hash(self) -> None:
        without = self._base_record()
        with_obs = self._base_record(governance_envelopes=[_envelope()])
        a1 = compute_canonical_analysis_from_record(without)
        a2 = compute_canonical_analysis_from_record(with_obs)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash

    def test_persistence_attestations_affect_hash(self) -> None:
        without = self._base_record()
        with_obs = self._base_record(persistence_attestations=[_persistence()])
        a1 = compute_canonical_analysis_from_record(without)
        a2 = compute_canonical_analysis_from_record(with_obs)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash

    def test_commitment_attestations_affect_hash(self) -> None:
        without = self._base_record()
        with_obs = self._base_record(commitment_attestations=[_commitment()])
        a1 = compute_canonical_analysis_from_record(without)
        a2 = compute_canonical_analysis_from_record(with_obs)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash

    def test_audit_links_affect_hash(self) -> None:
        without = self._base_record()
        with_obs = self._base_record(audit_links=[_audit_link()])
        a1 = compute_canonical_analysis_from_record(without)
        a2 = compute_canonical_analysis_from_record(with_obs)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash

    def test_price_table_affects_hash(self) -> None:
        without = self._base_record()
        with_obs = self._base_record(price_table=_price_table())
        a1 = compute_canonical_analysis_from_record(without)
        a2 = compute_canonical_analysis_from_record(with_obs)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash

    def test_run_id_affects_hash(self) -> None:
        r1 = AnalysisInputRecord(run_id="run-a", release_version=_RELEASE, tasks=[_minimal_task()], attempts=[_minimal_attempt()])
        r2 = AnalysisInputRecord(run_id="run-b", release_version=_RELEASE, tasks=[_minimal_task()], attempts=[_minimal_attempt()])
        a1 = compute_canonical_analysis_from_record(r1)
        a2 = compute_canonical_analysis_from_record(r2)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash

    def test_release_version_affects_hash(self) -> None:
        r1 = AnalysisInputRecord(run_id=_RUN_ID, release_version="v2.1.7", tasks=[_minimal_task()], attempts=[_minimal_attempt()])
        r2 = AnalysisInputRecord(run_id=_RUN_ID, release_version="v2.1.8", tasks=[_minimal_task()], attempts=[_minimal_attempt()])
        a1 = compute_canonical_analysis_from_record(r1)
        a2 = compute_canonical_analysis_from_record(r2)
        assert a1.input_summary.input_content_hash != a2.input_summary.input_content_hash


class TestObservationCountFromSuppliedRecords:
    """Prove that observation_count reflects supplied observation records, not reference strings."""

    def test_observation_count_includes_local_resource_observations(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[_local_resource_obs()],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.input_summary.observation_count >= 1

    def test_observation_count_includes_human_wait_observations(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            human_wait_observations=[_human_wait_obs()],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.input_summary.observation_count >= 1

    def test_observation_count_includes_governance_envelopes(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            governance_envelopes=[_envelope()],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.input_summary.observation_count >= 1

    def test_observation_count_includes_persistence_attestations(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            persistence_attestations=[_persistence()],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.input_summary.observation_count >= 1

    def test_observation_count_includes_commitment_attestations(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            commitment_attestations=[_commitment()],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.input_summary.observation_count >= 1

    def test_observation_count_includes_audit_links(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            audit_links=[_audit_link()],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.input_summary.observation_count >= 1

    def test_observation_count_sums_all_supplied_records(self) -> None:
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[_local_resource_obs()],
            human_wait_observations=[_human_wait_obs()],
            governance_envelopes=[_envelope()],
            persistence_attestations=[_persistence()],
            commitment_attestations=[_commitment()],
            audit_links=[_audit_link()],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.input_summary.observation_count == 6

    def test_observation_count_excludes_reference_only_attempts(self) -> None:
        """An attempt with reference strings but no supplied observation records has observation_count 0."""
        attempt_with_refs = AttemptRecord(
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            arm_id=Arm.DOCTRINE,
            terminal_status=TerminalStatus.COMPLETED,
            local_resource_observation_refs=["fake-ref-1"],
            human_wait_observation_refs=["fake-ref-2"],
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[attempt_with_refs],
        )
        analysis = compute_canonical_analysis_from_record(record)
        assert analysis.input_summary.observation_count == 0


class TestCrossRecordBindingValidation:
    """Table-driven validation tests for every cross-record binding on AnalysisInputRecord."""

    def test_duplicate_local_resource_observation_id_rejected(self) -> None:
        obs = _local_resource_obs()
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[obs, obs],
        )
        with pytest.raises(ValueError, match="duplicate"):
            compute_canonical_analysis_from_record(record)

    def test_local_resource_observation_unknown_attempt_rejected(self) -> None:
        obs = LocalResourceObservation(
            observation_id="lr-1",
            attempt_id="unknown-attempt",
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            peak_memory_bytes=1024,
            cpu_seconds=0.5,
            collected_at=_TS,
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256=_HASH,
            verification_status=VerificationStatus.VERIFIED,
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[obs],
        )
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(record)

    def test_local_resource_observation_cross_run_rejected(self) -> None:
        obs = LocalResourceObservation(
            observation_id="lr-1",
            attempt_id=_ATTEMPT_ID,
            run_id="other-run",
            task_id=_TASK_ID,
            peak_memory_bytes=1024,
            cpu_seconds=0.5,
            collected_at=_TS,
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256=_HASH,
            verification_status=VerificationStatus.VERIFIED,
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[obs],
        )
        with pytest.raises(ValueError, match="run"):
            compute_canonical_analysis_from_record(record)

    def test_local_resource_observation_wrong_task_rejected(self) -> None:
        obs = LocalResourceObservation(
            observation_id="lr-1",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id="wrong-task",
            peak_memory_bytes=1024,
            cpu_seconds=0.5,
            collected_at=_TS,
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256=_HASH,
            verification_status=VerificationStatus.VERIFIED,
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[obs],
        )
        with pytest.raises(ValueError, match="task"):
            compute_canonical_analysis_from_record(record)

    def test_human_wait_observation_unknown_attempt_rejected(self) -> None:
        obs = HumanWaitObservation(
            observation_id="hw-1",
            attempt_id="unknown-attempt",
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            human_wait_seconds=3.0,
            human_action_type="approval",
            collected_at=_TS,
            source_evidence_refs=["evidence-2"],
            source_evidence_sha256=_HASH,
            verification_status=VerificationStatus.VERIFIED,
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            human_wait_observations=[obs],
        )
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(record)

    def test_governance_envelope_unknown_attempt_rejected(self) -> None:
        env = GovernanceEnvelopeRecord(
            envelope_id="env-1",
            attempt_id="unknown-attempt",
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            envelope_sha256=_HASH,
            layer_disposition="L1_ALLOW",
            signer_key_id="key-1",
            created_at=_TS,
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            governance_envelopes=[env],
        )
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(record)

    def test_persistence_attestation_unknown_attempt_rejected(self) -> None:
        att = PersistenceAttestation(
            attestation_id="pa-1",
            attempt_id="unknown-attempt",
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            content_sha256=_HASH,
            persistence_target="sqlite",
            persisted_at=_TS,
            persisted_by="gateway",
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            persistence_attestations=[att],
        )
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(record)

    def test_commitment_attestation_unknown_attempt_rejected(self) -> None:
        att = CommitmentAttestation(
            attestation_id="ca-1",
            attempt_id="unknown-attempt",
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            commitment_hash=_HASH,
            signer_key_id="key-1",
            committed_at=_TS,
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            commitment_attestations=[att],
        )
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(record)

    def test_audit_link_unknown_attempt_rejected(self) -> None:
        link = AuditLinkRecord(
            audit_link_id="al-1",
            attempt_id="unknown-attempt",
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            audit_record_id="ar-1",
            audit_entry_sha256=_HASH,
            recorded_at=_TS,
        )
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            audit_links=[link],
        )
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(record)

    def test_duplicate_governance_envelope_id_rejected(self) -> None:
        env = _envelope()
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            governance_envelopes=[env, env],
        )
        with pytest.raises(ValueError, match="duplicate"):
            compute_canonical_analysis_from_record(record)

    def test_duplicate_persistence_attestation_id_rejected(self) -> None:
        att = _persistence()
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            persistence_attestations=[att, att],
        )
        with pytest.raises(ValueError, match="duplicate"):
            compute_canonical_analysis_from_record(record)

    def test_duplicate_audit_link_id_rejected(self) -> None:
        link = _audit_link()
        record = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            audit_links=[link, link],
        )
        with pytest.raises(ValueError, match="duplicate"):
            compute_canonical_analysis_from_record(record)


class TestRecordImmutability:
    """Verify that analytical evidence records are genuinely frozen and reject mutation."""

    def test_local_resource_observation_is_frozen(self) -> None:
        obs = _local_resource_obs()
        with pytest.raises(ValidationError):
            obs.peak_memory_bytes = 2048  # type: ignore[misc]

    def test_human_wait_observation_is_frozen(self) -> None:
        obs = _human_wait_obs()
        with pytest.raises(ValidationError):
            obs.human_wait_seconds = 99.0  # type: ignore[misc]

    def test_governance_envelope_record_is_frozen(self) -> None:
        env = _envelope()
        with pytest.raises(ValidationError):
            env.layer_disposition = "L2_BLOCK"  # type: ignore[misc]

    def test_persistence_attestation_is_frozen(self) -> None:
        att = _persistence()
        with pytest.raises(ValidationError):
            att.persistence_target = "other"  # type: ignore[misc]

    def test_commitment_attestation_is_frozen(self) -> None:
        att = _commitment()
        with pytest.raises(ValidationError):
            att.signer_key_id = "other"  # type: ignore[misc]

    def test_audit_link_record_is_frozen(self) -> None:
        link = _audit_link()
        with pytest.raises(ValidationError):
            link.audit_record_id = "other"  # type: ignore[misc]

    def test_price_table_entry_is_frozen(self) -> None:
        entry = PriceTableEntry(
            provider="test-provider",
            model="test-model",
            input_token_price_usd=0.001,
            output_token_price_usd=0.002,
            effective_at=_TS,
        )
        with pytest.raises(ValidationError):
            entry.provider = "other"  # type: ignore[misc]

    def test_typed_price_table_is_frozen(self) -> None:
        table = _price_table()
        with pytest.raises(ValidationError):
            table.price_table_id = "other"  # type: ignore[misc]


class TestDeterminismFromRecord:
    """Permuting input sequence order produces identical canonical bytes."""

    def test_permuted_observation_order_produces_identical_hash(self) -> None:
        obs1 = LocalResourceObservation(
            observation_id="lr-1",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            peak_memory_bytes=1024,
            cpu_seconds=0.5,
            collected_at=_TS,
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256=_HASH,
            verification_status=VerificationStatus.VERIFIED,
        )
        obs2 = LocalResourceObservation(
            observation_id="lr-2",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            peak_memory_bytes=2048,
            cpu_seconds=1.0,
            collected_at=_TS,
            source_evidence_refs=["evidence-2"],
            source_evidence_sha256=_HASH,
            verification_status=VerificationStatus.VERIFIED,
        )
        r1 = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[obs1, obs2],
        )
        r2 = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[obs2, obs1],
        )
        a1 = compute_canonical_analysis_from_record(r1)
        a2 = compute_canonical_analysis_from_record(r2)
        assert a1.input_summary.input_content_hash == a2.input_summary.input_content_hash
        assert a1.canonical_json() == a2.canonical_json()

    def test_permuted_envelope_order_produces_identical_hash(self) -> None:
        env1 = GovernanceEnvelopeRecord(
            envelope_id="env-1",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            envelope_sha256=_HASH,
            layer_disposition="L1_ALLOW",
            signer_key_id="key-1",
            created_at=_TS,
        )
        env2 = GovernanceEnvelopeRecord(
            envelope_id="env-2",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            envelope_sha256="b" * 64,
            layer_disposition="L2_CONSENSUS",
            signer_key_id="key-2",
            created_at=_TS,
        )
        r1 = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            governance_envelopes=[env1, env2],
        )
        r2 = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            governance_envelopes=[env2, env1],
        )
        a1 = compute_canonical_analysis_from_record(r1)
        a2 = compute_canonical_analysis_from_record(r2)
        assert a1.input_summary.input_content_hash == a2.input_summary.input_content_hash

    def test_changing_any_protected_field_changes_hash(self) -> None:
        base = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[_local_resource_obs()],
        )
        base_analysis = compute_canonical_analysis_from_record(base)
        changed_obs = LocalResourceObservation(
            observation_id="lr-1",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            peak_memory_bytes=9999,
            cpu_seconds=0.5,
            collected_at=_TS,
            source_evidence_refs=["evidence-1"],
            source_evidence_sha256=_HASH,
            verification_status=VerificationStatus.VERIFIED,
        )
        changed = AnalysisInputRecord(
            run_id=_RUN_ID,
            release_version=_RELEASE,
            tasks=[_minimal_task()],
            attempts=[_minimal_attempt()],
            local_resource_observations=[changed_obs],
        )
        changed_analysis = compute_canonical_analysis_from_record(changed)
        assert base_analysis.input_summary.input_content_hash != changed_analysis.input_summary.input_content_hash
