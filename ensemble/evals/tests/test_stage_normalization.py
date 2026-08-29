# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import json

import pytest

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
            payload={"event": {"type": "g8e.v1.ai.consensus.voting.pass.completed", "data": {"member": "axiom", "model": "qwen", "provider": "ollama", "input_tokens": 7, "output_tokens": 3, "finish_reason": "stop"}}},
            monotonic_received_at=20.0,
        ),
        AgentTrailEvent(
            id=2,
            event_type="g8e.v1.ai.llm.chat.iteration.text.completed",
            payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {"agent_mode": "sage", "model_calls": [{"agent_role": "sage", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 4.0, "monotonic_end": 5.0, "input_tokens": 11, "output_tokens": 5, "finish_reason": "stop"}], "token_usage": {"input_tokens": 11, "output_tokens": 5, "total_tokens": 16}}}},
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
                    {"agent_role": "auditor", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 4.0, "monotonic_end": 5.0, "input_tokens": 10, "output_tokens": 2, "retry_count": 0, "input_artifact_hash": "input-1", "output_artifact_hash": "output-1"},
                    {"agent_role": "auditor", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 6.0, "monotonic_end": 7.0, "input_tokens": 12, "output_tokens": 4, "retry_count": 1, "input_artifact_hash": "input-2", "output_artifact_hash": "output-2"},
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
                    {"agent_role": "warden_command_risk", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 4.0, "monotonic_end": 5.0, "input_tokens": 8, "output_tokens": 1, "succeeded": True},
                    {"agent_role": "warden_error", "provider": "OllamaProvider", "model": "qwen", "monotonic_start": 6.0, "monotonic_end": 7.0, "input_tokens": 12, "output_tokens": 3, "succeeded": False, "error_type": "OllamaEmptyResponseError"},
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
                payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {"agent_mode": "sage", "token_usage": {"input_tokens": 20, "output_tokens": 10}, "model_calls": [{"agent_role": "sage", "input_tokens": 18, "output_tokens": 10}]}}},
            )
        ],
    )

    normalized = normalize_attempt_evidence(evidence, run_id="run", attempt_id="attempt")

    assert normalized.usage.reconciled is False
    assert normalized.usage.input_token_delta == -2


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
