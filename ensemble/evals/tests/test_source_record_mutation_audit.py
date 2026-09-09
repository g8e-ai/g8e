# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Phase 1 source-record mutation audit.

Verifies that mutating each source-record family changes the input
content hash or produces a typed validation failure. Also verifies
that reanalyzing identical complete inputs twice produces byte-identical
canonical JSON and all three renderer byte streams.

This test is the named verification artifact for P1-08 step 6.
"""

from __future__ import annotations

from datetime import UTC, datetime

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis import AnalysisInputRecord, compute_canonical_analysis_from_record
from g8e_evals.analysis.canonical import canonical_model_json
from g8e_evals.analysis.renderers import render_cli, render_html, render_markdown
from g8e_evals.arms import Arm
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
)
from g8e_evals.schema import (
    AttemptRecord,
    GraderClass,
    MetricObservation,
    ReceiptObservation,
    StageKind,
    StageObservation,
    TaskDefinition,
    TerminalStatus,
    VerificationStatus,
)

_RUN_ID = "run-mutation-1"
_RELEASE = "v2.1.8"
_TASK_ID = "task-mutation-1"
_ATTEMPT_ID = "attempt-mutation-1"
_TS = datetime(2026, 1, 1, tzinfo=UTC)


def _task() -> TaskDefinition:
    return TaskDefinition(
        task_id=_TASK_ID,
        suite_id="utility",
        suite_version="1.0.0",
        prompt_hash="abc123",
        prompt_length=10,
        expected_action_class="TEST_ACTION",
        compatible_arms=[Arm.DOCTRINE],
    )


def _attempt() -> AttemptRecord:
    return AttemptRecord(
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=Arm.DOCTRINE,
        terminal_status=TerminalStatus.COMPLETED,
    )


def _metric_observation() -> MetricObservation:
    return MetricObservation(
        metric_id="receipt_integrity",
        metric_version="1.0.0",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        arm_id=Arm.DOCTRINE,
        task_id=_TASK_ID,
        value=1.0,
        unit="boolean",
        eligible=True,
        verification_status=VerificationStatus.VERIFIED,
        grader_class=GraderClass.DETERMINISTIC,
    )


def _stage() -> StageObservation:
    return StageObservation(
        stage_id="stage-1",
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        kind=StageKind.DETERMINISTIC_DOCTRINE,
    )


def _receipt() -> ReceiptObservation:
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
        attempt_id=_ATTEMPT_ID,
        run_id=_RUN_ID,
        transaction_id="tx-1",
        action_type="TEST_ACTION",
        primary=True,
        verified=True,
        action_receipt=receipt,
    )


def _base_record() -> AnalysisInputRecord:
    return AnalysisInputRecord(
        run_id=_RUN_ID,
        release_version=_RELEASE,
        tasks=[_task()],
        attempts=[_attempt()],
        metric_observations=[_metric_observation()],
        receipts=[_receipt()],
        stages=[_stage()],
    )


def _hash(record: AnalysisInputRecord) -> str:
    return compute_canonical_analysis_from_record(record).input_summary.input_content_hash


class TestReanalyzeIdenticalInputs:
    """Reanalyzing identical complete inputs twice produces byte-identical output."""

    def test_identical_inputs_produce_identical_canonical_json(self) -> None:
        record = _base_record()
        analysis1 = compute_canonical_analysis_from_record(record)
        analysis2 = compute_canonical_analysis_from_record(record)
        assert canonical_model_json(analysis1) == canonical_model_json(analysis2)

    def test_identical_inputs_produce_identical_renderer_bytes(self) -> None:
        record = _base_record()
        analysis1 = compute_canonical_analysis_from_record(record)
        analysis2 = compute_canonical_analysis_from_record(record)
        assert render_markdown(analysis1) == render_markdown(analysis2)
        assert render_html(analysis1) == render_html(analysis2)
        assert render_cli(analysis1) == render_cli(analysis2)

    def test_identical_inputs_produce_identical_input_hash(self) -> None:
        record = _base_record()
        hash1 = _hash(record)
        hash2 = _hash(record)
        assert hash1 == hash2


class TestSourceRecordMutationChangesHash:
    """Mutating each source-record family changes the input content hash."""

    def test_mutating_run_id_changes_hash(self) -> None:
        base = _base_record()
        new_run = "run-mutation-2"
        mutated_attempts = [a.model_copy(update={"run_id": new_run}) for a in base.attempts]
        mutated_obs = [o.model_copy(update={"run_id": new_run}) for o in base.metric_observations]
        mutated_stages = [s.model_copy(update={"run_id": new_run}) for s in base.stages]
        mutated_receipts = [r.model_copy(update={"run_id": new_run}) for r in base.receipts]
        mutated = base.model_copy(update={
            "run_id": new_run,
            "attempts": mutated_attempts,
            "metric_observations": mutated_obs,
            "stages": mutated_stages,
            "receipts": mutated_receipts,
        })
        assert _hash(base) != _hash(mutated)

    def test_mutating_release_version_changes_hash(self) -> None:
        base = _base_record()
        mutated = base.model_copy(update={"release_version": "v2.1.9"})
        assert _hash(base) != _hash(mutated)

    def test_mutating_task_prompt_hash_changes_hash(self) -> None:
        base = _base_record()
        mutated_task = base.tasks[0].model_copy(update={"prompt_hash": "different-hash"})
        mutated = base.model_copy(update={"tasks": [mutated_task]})
        assert _hash(base) != _hash(mutated)

    def test_mutating_task_suite_id_changes_hash(self) -> None:
        base = _base_record()
        mutated_task = base.tasks[0].model_copy(update={"suite_id": "different-suite"})
        mutated = base.model_copy(update={"tasks": [mutated_task]})
        assert _hash(base) != _hash(mutated)

    def test_mutating_attempt_terminal_status_changes_hash(self) -> None:
        base = _base_record()
        mutated_attempt = base.attempts[0].model_copy(update={"terminal_status": TerminalStatus.MODEL_FAILED})
        mutated = base.model_copy(update={"attempts": [mutated_attempt]})
        assert _hash(base) != _hash(mutated)

    def test_mutating_attempt_arm_changes_hash(self) -> None:
        base = _base_record()
        mutated_attempt = base.attempts[0].model_copy(update={"arm_id": Arm.DIRECT})
        # Metric observations must match the attempt's arm
        mutated_obs = base.metric_observations[0].model_copy(update={"arm_id": Arm.DIRECT})
        mutated = base.model_copy(
            update={"attempts": [mutated_attempt], "metric_observations": [mutated_obs]}
        )
        assert _hash(base) != _hash(mutated)

    def test_mutating_metric_observation_value_changes_hash(self) -> None:
        base = _base_record()
        mutated_obs = base.metric_observations[0].model_copy(update={"value": 0.0})
        mutated = base.model_copy(update={"metric_observations": [mutated_obs]})
        assert _hash(base) != _hash(mutated)

    def test_mutating_metric_observation_verification_status_changes_hash(self) -> None:
        base = _base_record()
        mutated_obs = base.metric_observations[0].model_copy(update={"verification_status": VerificationStatus.FAILED})
        mutated = base.model_copy(update={"metric_observations": [mutated_obs]})
        assert _hash(base) != _hash(mutated)

    def test_mutating_receipt_verified_changes_hash(self) -> None:
        base = _base_record()
        mutated_receipt = base.receipts[0].model_copy(update={"verified": False})
        mutated = base.model_copy(update={"receipts": [mutated_receipt]})
        assert _hash(base) != _hash(mutated)

    def test_mutating_receipt_transaction_id_changes_hash(self) -> None:
        base = _base_record()
        new_tx_id = "tx-2"
        new_receipt_proto = ActionReceipt(
            transaction_id=new_tx_id,
            transaction_hash="hash-2",
        )
        new_receipt_proto.deterministic_stage_evidence.add(
            kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
            outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
            action_type="TEST_ACTION",
        )
        mutated_receipt = base.receipts[0].model_copy(
            update={
                "transaction_id": new_tx_id,
                "action_receipt": new_receipt_proto,
            }
        )
        mutated = base.model_copy(update={"receipts": [mutated_receipt]})
        assert _hash(base) != _hash(mutated)

    def test_mutating_stage_kind_changes_hash(self) -> None:
        base = _base_record()
        mutated_stage = base.stages[0].model_copy(update={"kind": StageKind.L4_VERIFICATION})
        mutated = base.model_copy(update={"stages": [mutated_stage]})
        assert _hash(base) != _hash(mutated)

    def test_mutating_stage_decision_changes_hash(self) -> None:
        base = _base_record()
        mutated_stage = base.stages[0].model_copy(update={"decision": "rejected"})
        mutated = base.model_copy(update={"stages": [mutated_stage]})
        assert _hash(base) != _hash(mutated)

    def test_adding_extra_metric_observation_changes_hash(self) -> None:
        base = _base_record()
        extra_obs = MetricObservation(
            metric_id="protocol_chain",
            metric_version="1.0.0",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            arm_id=Arm.DOCTRINE,
            task_id=_TASK_ID,
            value=1.0,
            unit="boolean",
            eligible=True,
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
        )
        mutated = base.model_copy(update={"metric_observations": [*base.metric_observations, extra_obs]})
        assert _hash(base) != _hash(mutated)

    def test_adding_extra_stage_changes_hash(self) -> None:
        base = _base_record()
        extra_stage = StageObservation(
            stage_id="stage-2",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            task_id=_TASK_ID,
            kind=StageKind.L4_VERIFICATION,
        )
        mutated = base.model_copy(update={"stages": [*base.stages, extra_stage]})
        assert _hash(base) != _hash(mutated)

    def test_removing_metric_observations_changes_hash(self) -> None:
        base = _base_record()
        mutated = base.model_copy(update={"metric_observations": []})
        assert _hash(base) != _hash(mutated)

    def test_removing_receipts_changes_hash(self) -> None:
        base = _base_record()
        mutated = base.model_copy(update={"receipts": []})
        assert _hash(base) != _hash(mutated)

    def test_removing_stages_changes_hash(self) -> None:
        base = _base_record()
        mutated = base.model_copy(update={"stages": []})
        assert _hash(base) != _hash(mutated)


class TestSourceRecordMutationProducesValidationFailure:
    """Mutating source records to break cross-record bindings produces typed validation failures."""

    def test_cross_run_attempt_rejected(self) -> None:
        base = _base_record()
        mutated_attempt = base.attempts[0].model_copy(update={"run_id": "different-run"})
        mutated = base.model_copy(update={"attempts": [mutated_attempt]})
        with pytest.raises(ValueError, match="run"):
            compute_canonical_analysis_from_record(mutated)

    def test_unknown_task_on_metric_observation_rejected(self) -> None:
        base = _base_record()
        mutated_obs = base.metric_observations[0].model_copy(update={"task_id": "unknown-task"})
        mutated = base.model_copy(update={"metric_observations": [mutated_obs]})
        with pytest.raises(ValueError, match="task"):
            compute_canonical_analysis_from_record(mutated)

    def test_unknown_attempt_on_metric_observation_rejected(self) -> None:
        base = _base_record()
        mutated_obs = base.metric_observations[0].model_copy(update={"attempt_id": "unknown-attempt"})
        mutated = base.model_copy(update={"metric_observations": [mutated_obs]})
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(mutated)

    def test_unknown_attempt_on_stage_rejected(self) -> None:
        base = _base_record()
        mutated_stage = base.stages[0].model_copy(update={"attempt_id": "unknown-attempt"})
        mutated = base.model_copy(update={"stages": [mutated_stage]})
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(mutated)

    def test_unknown_attempt_on_receipt_rejected(self) -> None:
        base = _base_record()
        mutated_receipt = base.receipts[0].model_copy(update={"attempt_id": "unknown-attempt"})
        mutated = base.model_copy(update={"receipts": [mutated_receipt]})
        with pytest.raises(ValueError, match="unknown attempt"):
            compute_canonical_analysis_from_record(mutated)

    def test_duplicate_metric_observation_id_rejected(self) -> None:
        base = _base_record()
        dup_obs = base.metric_observations[0].model_copy()
        mutated = base.model_copy(update={"metric_observations": [*base.metric_observations, dup_obs]})
        with pytest.raises(ValueError, match="duplicate"):
            compute_canonical_analysis_from_record(mutated)

    def test_duplicate_stage_id_rejected(self) -> None:
        base = _base_record()
        dup_stage = base.stages[0].model_copy()
        mutated = base.model_copy(update={"stages": [*base.stages, dup_stage]})
        with pytest.raises(ValueError, match="duplicate"):
            compute_canonical_analysis_from_record(mutated)
