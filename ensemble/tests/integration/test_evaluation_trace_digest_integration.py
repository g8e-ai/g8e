# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
from pathlib import Path

import pytest

from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry
from app.services.evaluation.trace_service import EvaluationTraceService, compute_trace_digest
from g8e.eval.v1.trace_digest import compute_chat_probe_trace_digest
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant

PROTOCOL_ROOT = Path(__file__).resolve().parents[3] / "protocol"
CHAT_PROBE_TRACE_VECTOR_PATH = PROTOCOL_ROOT / "vectors" / "eval" / "chat_probe_trace.json"


@pytest.mark.integration
def test_chat_probe_trace_digest_matches_protocol_vector():
    vector = json.loads(CHAT_PROBE_TRACE_VECTOR_PATH.read_text())
    assert vector["message_type"] == "ChatProbeTrace"
    assert compute_chat_probe_trace_digest(vector["trace"]) == vector["trace_digest"]


@pytest.fixture
def trace_service(tmp_path, monkeypatch):
    monkeypatch.setenv("G8E_RUNTIME_DIR", str(tmp_path))
    return EvaluationTraceService()


def _evaluation_context() -> EvaluationInferenceContext:
    return EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="scenario-1",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="model-a", digest="a" * 64)],
        target_operator_session_id="session-1",
    )


@pytest.mark.integration
def test_evaluation_trace_service_persists_digest_binding(trace_service):
    context = G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())
    agent_call = ModelCallTelemetry(
        agent_role="sage",
        model_role="primary",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
        provider_attempt_id="attempt-1",
    )
    trace_service.begin(context)
    finalized = trace_service.finalize(
        context,
        model_calls=[agent_call],
        finish_reason="stop",
        status="completed",
    )
    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.trace_digest == finalized.trace_digest
    assert compute_trace_digest(loaded.model_copy(update={"trace_digest": ""})) == loaded.trace_digest
    assert trace_service.trace_file("assignment-1", "attempt-1").exists()
