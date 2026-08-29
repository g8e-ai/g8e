# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json

import pytest

from app.models.model_telemetry import ModelCallTelemetry
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
    DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
)
from g8e_evals.schema import StageKind
from g8e_evals.stages import normalize_attempt_evidence
from g8e_evals.sut.direct_provider import DirectCallEvidence, _DirectEvidenceWrapper
from g8e_evals.sut.g8ee_chat import AgentTrailEvent, ChatEvaluationReceipt

pytestmark = pytest.mark.unit


def test_direct_provider_call_normalizes_to_one_model_stage():
    evidence = _DirectEvidenceWrapper(
        DirectCallEvidence(
            provider="ollama",
            model="qwen",
            finish_reason="stop",
            usage_reported=True,
            prompt_token_count=12,
            candidates_token_count=4,
            total_token_count=16,
            thinking_token_count=1,
            monotonic_start=10.0,
            monotonic_end=11.5,
        )
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert len(normalized.stages) == 1
    stage = normalized.stages[0]
    assert stage.kind == StageKind.MODEL_INFERENCE
    assert stage.agent_role == "direct"
    assert stage.input_tokens == 12
    assert stage.output_tokens == 4
    assert stage.thinking_tokens == 1
    assert stage.monotonic_start == 10.0
    assert stage.monotonic_end == 11.5
    assert normalized.usage.reconciled is True


def test_chat_trail_normalizes_primary_and_tribunal_model_stages():
    trail = [
        AgentTrailEvent(
            id=1,
            event_type="g8e.v1.ai.consensus.voting.pass.completed",
            payload={"event": {"type": "g8e.v1.ai.consensus.voting.pass.completed", "data": {"member": "axiom", "model": "qwen", "provider": "ollama", "input_tokens": 7, "output_tokens": 3, "usage_reported": True, "finish_reason": "stop"}}},
            monotonic_received_at=20.0,
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
            payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {"agent_mode": "sage", "model_calls": [{"agent_role": "sage", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 4.0, "monotonic_end": 5.0, "input_tokens": 11, "output_tokens": 5, "usage_reported": True, "finish_reason": "stop"}], "token_usage": {"input_tokens": 11, "output_tokens": 5, "total_tokens": 16, "usage_reported": True}}}},
            monotonic_received_at=22.0,
        ),
    ]
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=2,
        event_counts_by_type={},
        agent_trail=trail,
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert [(stage.kind, stage.agent_role) for stage in normalized.stages] == [
        (StageKind.TRIBUNAL_GENERATION, "axiom"),
        (StageKind.MODEL_INFERENCE, "sage"),
    ]
    assert normalized.usage.observed_call_count == 2
    assert normalized.usage.observed_input_tokens == 18
    assert normalized.raw_evidence is not None
    assert json.loads(normalized.raw_evidence.content)["investigation_id"] == "investigation"
    assert normalized.raw_evidence.index.sha256


def test_chat_trail_normalizes_failed_tribunal_generation_call():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.consensus.voting.pass.completed",
        answer_chars=0,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.consensus.voting.pass.completed",
                payload={"event": {"type": "g8e.v1.ai.consensus.voting.pass.completed", "data": {"member": "axiom", "model": "qwen", "provider": "ollama", "input_tokens": 7, "output_tokens": 0, "succeeded": False, "error_type": "RuntimeError"}}},
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert len(normalized.stages) == 1
    assert normalized.stages[0].kind == StageKind.TRIBUNAL_GENERATION
    assert normalized.stages[0].decision == "failed"
    assert normalized.usage.observed_call_count == 1


def test_chat_trail_normalizes_each_auditor_retry_as_a_model_stage():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.consensus.voting.audit.completed",
                payload={"event": {"type": "g8e.v1.ai.consensus.voting.audit.completed", "data": {"model_calls": [
                    {"agent_role": "auditor", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 4.0, "monotonic_end": 5.0, "input_tokens": 10, "output_tokens": 2, "usage_reported": True, "retry_count": 0, "input_artifact_hash": "input-1", "output_artifact_hash": "output-1"},
                    {"agent_role": "auditor", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 6.0, "monotonic_end": 7.0, "input_tokens": 12, "output_tokens": 4, "usage_reported": True, "retry_count": 1, "input_artifact_hash": "input-2", "output_artifact_hash": "output-2"},
                ]}}},
                monotonic_received_at=20.0,
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert len(normalized.stages) == 2
    assert all(stage.kind == StageKind.TRIBUNAL_AUDITOR for stage in normalized.stages)
    assert [stage.retry_count for stage in normalized.stages] == [0, 1]
    assert [stage.monotonic_start for stage in normalized.stages] == [4.0, 6.0]
    assert all(stage.clock_domain == "g8ee-process" for stage in normalized.stages)
    assert all(stage.cross_process_timing is False for stage in normalized.stages)
    assert normalized.usage.observed_call_count == 2
    assert normalized.usage.expected_call_count == 2
    assert normalized.usage.reconciled is True


def test_chat_trail_normalizes_failed_auditor_call():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.consensus.session.auditor.failed",
        answer_chars=0,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.consensus.session.auditor.failed",
                payload={"event": {"type": "g8e.v1.ai.consensus.session.auditor.failed", "data": {"model_calls": [{"agent_role": "auditor", "model": "qwen", "monotonic_start": 4.0, "monotonic_end": 5.0, "retry_count": 0, "succeeded": False, "error_type": "TimeoutError", "input_artifact_hash": "input-1"}]}}},
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert len(normalized.stages) == 1
    assert normalized.stages[0].kind == StageKind.TRIBUNAL_AUDITOR
    assert normalized.stages[0].decision == "failed"
    assert normalized.usage.observed_call_count == 1


def test_chat_trail_normalizes_each_warden_call_as_a_grading_stage():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.consensus.session.warden.blocked",
        answer_chars=0,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.consensus.session.warden.blocked",
                payload={"event": {"type": "g8e.v1.ai.consensus.session.warden.blocked", "data": {"model_calls": [
                    {"agent_role": "warden_command_risk", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 4.0, "monotonic_end": 5.0, "input_tokens": 8, "output_tokens": 1, "usage_reported": True, "succeeded": True},
                    {"agent_role": "warden_error", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 6.0, "monotonic_end": 7.0, "input_tokens": 12, "output_tokens": 3, "usage_reported": True, "succeeded": False, "error_type": "OllamaEmptyResponseError"},
                ]}}},
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert len(normalized.stages) == 2
    assert all(stage.kind == StageKind.GRADING for stage in normalized.stages)
    assert [stage.agent_role for stage in normalized.stages] == ["warden_command_risk", "warden_error"]
    assert [stage.decision for stage in normalized.stages] == ["completed", "failed"]
    assert normalized.usage.observed_call_count == 2
    assert normalized.usage.expected_call_count == 2
    assert normalized.usage.reconciled is True


def test_chat_trail_normalizes_authoritative_scrubbing_observations():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
                payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {"agent_mode": "sage", "scrubbing_observations": [{"source": "user_chat", "enabled": True, "was_modified": True, "scrub_count": 1, "scrub_types": ["email"], "monotonic_start": 2.0, "monotonic_end": 2.5, "input_artifact_hash": "input-hash", "output_artifact_hash": "output-hash"}]}}},
                monotonic_received_at=10.0,
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    scrubbing_stages = [stage for stage in normalized.stages if stage.kind == StageKind.SCRUBBING]
    assert len(scrubbing_stages) == 1
    stage = scrubbing_stages[0]
    assert stage.agent_role == "sentinel"
    assert stage.monotonic_start == 2.0
    assert stage.monotonic_end == 2.5
    assert stage.clock_domain == "g8ee-process"
    assert stage.timing_source == "scrubber_monotonic"
    assert stage.cross_process_timing is False
    assert stage.decision == "modified"
    assert stage.input_artifact_hash == "input-hash"
    assert stage.output_artifact_hash == "output-hash"
    assert stage.scrub_count == 1
    assert stage.scrub_types == ["email"]
    assert stage.source == "user_chat"


def test_signed_receipt_normalizes_authoritative_deterministic_stage_evidence():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=0,
        event_counts_by_type={},
        agent_trail=[],
    )
    receipt = ActionReceipt(transaction_id="tx-1", transaction_hash="hash-1")
    receipt.deterministic_stage_evidence.add(
        stage_id="tx-1:l4",
        kind=DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
        monotonic_start_ns=2_000_000_000,
        monotonic_end_ns=2_500_000_000,
        clock_domain="g8e-operator-process",
        timing_source="go_monotonic",
        outcome=DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
        transaction_id="tx-1",
        transaction_hash="hash-1",
        action_type="FILE_EDIT",
        operator_id="operator-1",
        operator_session_id="session-1",
        case_id="case-1",
        investigation_id="investigation-1",
        task_id="task-1",
        state_root_before="root-before",
        l2_signature_digest="l2-digest",
        doctrine_bundle_hash="doctrine-hash",
        doctrine_bundle_version="doctrine-v1",
        parent_stage_id="tx-1:l5",
    )
    receipt.deterministic_stage_evidence.add(
        stage_id="tx-1:commitment",
        kind=DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
        monotonic_start_ns=3_000_000_000,
        monotonic_end_ns=3_250_000_000,
        clock_domain="g8e-operator-process",
        timing_source="go_monotonic",
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        transaction_id="tx-1",
        transaction_hash="hash-1",
        commitment_hash="commitment-hash",
        prior_commitment_hash="prior-hash",
        signer_key_id="auditor-key",
        parent_stage_id="tx-1:l5",
    )
    receipt.deterministic_stage_evidence.add(
        stage_id="tx-1:l5",
        kind=DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
        monotonic_start_ns=3_500_000_000,
        monotonic_end_ns=4_000_000_000,
        clock_domain="g8e-operator-process",
        timing_source="go_monotonic",
        outcome=DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
        transaction_id="tx-1",
        transaction_hash="hash-1",
    )
    receipt.final_persistence_attestation.transaction_id = "tx-1"
    receipt.final_persistence_attestation.receipt_signature_digest = "receipt-digest"
    receipt.final_persistence_attestation.persisted_at_unix_ms = 1_777_777_777_456
    receipt.final_persistence_attestation.audit_record_id = "tx-1"
    receipt.final_persistence_attestation.signer_key_id = "warden-key"
    receipt.final_persistence_attestation.signature = "attestation-signature"

    normalized = normalize_attempt_evidence(
        evidence,
        run_id="run",
        attempt_id="attempt",
        action_receipt=receipt,
        receipt_verified=True,
    )

    assert [stage.kind for stage in normalized.stages] == [
        StageKind.L4_VERIFICATION,
        StageKind.COMMITMENT_APPEND,
        StageKind.L5_EXECUTION,
        StageKind.RECEIPT_PERSISTENCE,
    ]
    l4_stage, commitment_stage, execution_stage, final_persistence_stage = normalized.stages
    assert l4_stage.monotonic_start == 2.0
    assert l4_stage.monotonic_end == 2.5
    assert l4_stage.clock_domain == "g8e-operator-process"
    assert l4_stage.cross_process_timing is False
    assert l4_stage.transaction_id == "tx-1"
    assert l4_stage.operator_session_id == "session-1"
    assert l4_stage.state_root_before == "root-before"
    assert l4_stage.l2_signature_digest == "l2-digest"
    assert l4_stage.doctrine_bundle_hash == "doctrine-hash"
    assert l4_stage.doctrine_bundle_version == "doctrine-v1"
    assert commitment_stage.decision == "completed"
    assert commitment_stage.commitment_hash == "commitment-hash"
    assert commitment_stage.prior_commitment_hash == "prior-hash"
    assert commitment_stage.signer_key_id == "auditor-key"
    assert l4_stage.parent_stage_id == execution_stage.stage_id
    assert commitment_stage.parent_stage_id == execution_stage.stage_id
    assert final_persistence_stage.stage_id == "attempt:receipt:persistence:final"
    assert final_persistence_stage.decision == "verified"
    assert final_persistence_stage.transaction_id == "tx-1"
    assert final_persistence_stage.receipt_signature_digest == "receipt-digest"
    assert final_persistence_stage.audit_record_id == "tx-1"
    assert final_persistence_stage.signer_key_id == "warden-key"
    assert final_persistence_stage.persisted_at_unix_ms == 1_777_777_777_456
    assert final_persistence_stage.parent_stage_id == execution_stage.stage_id
    assert execution_stage.child_stage_ids == [
        l4_stage.stage_id,
        commitment_stage.stage_id,
        final_persistence_stage.stage_id,
    ]


@pytest.mark.parametrize(
    ("kind", "outcome", "message"),
    [
        (999, DETERMINISTIC_STAGE_OUTCOME_VERIFIED, "unknown deterministic stage kind"),
        (DETERMINISTIC_STAGE_KIND_L4_VERIFICATION, 999, "unknown deterministic stage outcome"),
    ],
)
def test_receipt_normalization_rejects_unknown_deterministic_stage_enums(kind, outcome, message):
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=0,
        event_counts_by_type={},
        agent_trail=[],
    )
    receipt = ActionReceipt(transaction_id="tx-1", transaction_hash="hash-1")
    receipt.deterministic_stage_evidence.add(
        stage_id="tx-1:stage",
        kind=kind,
        outcome=outcome,
        transaction_id="tx-1",
    )

    with pytest.raises(ValueError, match=message):
        normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt", action_receipt=receipt)


def test_receipt_normalization_rejects_unknown_parent_stage():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=0,
        event_counts_by_type={},
        agent_trail=[],
    )
    receipt = ActionReceipt(transaction_id="tx-1", transaction_hash="hash-1")
    receipt.deterministic_stage_evidence.add(
        stage_id="tx-1:l4",
        kind=DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
        outcome=DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
        transaction_id="tx-1",
        parent_stage_id="tx-1:missing",
    )

    with pytest.raises(ValueError, match="unknown parent deterministic stage"):
        normalize_attempt_evidence(
            evidence,
            run_id="run",
            attempt_id="attempt",
            action_receipt=receipt,
        )


def test_eval_judge_calls_are_attached_to_stages_and_reconciliation():
    evidence = _DirectEvidenceWrapper(
        DirectCallEvidence(
            provider="ollama",
            model="qwen",
            usage_reported=True,
            prompt_token_count=10,
            candidates_token_count=4,
            thinking_token_count=1,
            monotonic_start=1.0,
            monotonic_end=2.0,
        )
    )
    judge_calls = [
        ModelCallTelemetry(
            agent_role="judge",
            provider="OllamaProvider",
            model="qwen-judge",
            monotonic_start=3.0,
            monotonic_end=4.0,
            input_tokens=12,
            output_tokens=3,
            total_tokens=15,
            usage_reported=True,
            input_artifact_hash="judge-input",
            output_artifact_hash="judge-output",
        )
    ]

    normalized = normalize_attempt_evidence(
        evidence,
        run_id="run",
        attempt_id="attempt",
        grading_model_calls=judge_calls,
    )

    assert [(stage.kind, stage.agent_role) for stage in normalized.stages] == [
        (StageKind.MODEL_INFERENCE, "direct"),
        (StageKind.GRADING, "judge"),
    ]
    judge_stage = normalized.stages[1]
    assert judge_stage.clock_domain == "g8e-evals-process"
    assert judge_stage.input_artifact_hash == "judge-input"
    assert judge_stage.output_artifact_hash == "judge-output"
    assert normalized.usage.reported_input_tokens == 22
    assert normalized.usage.reported_output_tokens == 7
    assert normalized.usage.reported_thinking_tokens == 1
    assert normalized.usage.expected_call_count == 2
    assert normalized.usage.observed_call_count == 2
    assert normalized.usage.reconciled is True


def test_chat_usage_reconciliation_flags_token_total_mismatch():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
                payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {"agent_mode": "sage", "token_usage": {"input_tokens": 20, "output_tokens": 10, "usage_reported": True}, "model_calls": [{"agent_role": "sage", "input_tokens": 18, "output_tokens": 10, "usage_reported": True}]}}},
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert normalized.usage.reconciled is False
    assert normalized.usage.input_token_delta == -2


def test_direct_provider_missing_usage_is_not_treated_as_reported_zero():
    evidence = _DirectEvidenceWrapper(
        DirectCallEvidence(
            provider="ollama",
            model="qwen",
            usage_reported=False,
            monotonic_start=1.0,
            monotonic_end=2.0,
        )
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    stage = normalized.stages[0]
    assert stage.input_tokens is None
    assert stage.output_tokens is None
    assert stage.cache_tokens is None
    assert stage.usage_reported is False
    assert normalized.usage.missing_provider_usage_call_count == 1
    assert normalized.usage.exact_reconciliation_possible is False
    assert normalized.usage.reconciled is False


def test_direct_provider_reported_zero_usage_remains_exact():
    evidence = _DirectEvidenceWrapper(
        DirectCallEvidence(
            provider="ollama",
            model="qwen",
            usage_reported=True,
            prompt_token_count=0,
            candidates_token_count=0,
            cache_token_count=0,
            monotonic_start=1.0,
            monotonic_end=2.0,
        )
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    stage = normalized.stages[0]
    assert stage.input_tokens == 0
    assert stage.output_tokens == 0
    assert stage.cache_tokens == 0
    assert stage.usage_reported is True
    assert normalized.usage.missing_provider_usage_call_count == 0
    assert normalized.usage.exact_reconciliation_possible is True
    assert normalized.usage.reconciled is True


def test_chat_usage_reconciliation_accounts_for_cache_tokens():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
                payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {"agent_mode": "sage", "token_usage": {"input_tokens": 20, "output_tokens": 10, "cache_tokens": 7, "usage_reported": True}, "model_calls": [{"agent_role": "sage", "input_tokens": 20, "output_tokens": 10, "cache_tokens": 7, "usage_reported": True}]}}},
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert normalized.stages[0].cache_tokens == 7
    assert normalized.usage.reported_cache_tokens == 7
    assert normalized.usage.observed_cache_tokens == 7
    assert normalized.usage.cache_token_delta == 0
    assert normalized.usage.reconciled is True


def test_chat_usage_reconciliation_flags_uninstrumented_calls():
    evidence = ChatEvaluationReceipt(
        case_id="case",
        investigation_id="investigation",
        terminal_event="g8e.v1.ai.llm.chat.iteration.text.completed",
        answer_chars=5,
        event_count=1,
        event_counts_by_type={},
        agent_trail=[
            AgentTrailEvent(
                id=1,
                event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
                payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {"agent_mode": "sage", "token_usage": {"input_tokens": 20, "output_tokens": 10}, "model_call_count": 2}}},
                monotonic_received_at=30.0,
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert normalized.usage.reconciled is False
    assert normalized.usage.expected_call_count == 2
    assert normalized.usage.observed_call_count == 1
