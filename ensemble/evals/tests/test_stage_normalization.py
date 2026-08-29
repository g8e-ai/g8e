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
            payload={"event": {"type": "g8e.v1.ai.llm.chat.iteration.text.completed", "data": {"agent_mode": "sage", "finish_reason": "stop", "token_usage": {"input_tokens": 11, "output_tokens": 5, "total_tokens": 16}}}},
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
