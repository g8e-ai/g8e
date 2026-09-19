# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from unittest.mock import MagicMock

import pytest

from app.constants import (
    ReasoningAgent,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
)
from app.llm.utils import ModelOverrideResolver
from app.models.agent import AgentInputs, AgentStreamState
from app.models.agents.triage import TriageResult
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry
from app.models.settings import G8eeUserSettings
from app.services.ai.chat_pipeline import ChatPipelineService
from app.services.evaluation.role_control import apply_homogeneous_role_control
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant


def _evaluation_context(role: str) -> EvaluationInferenceContext:
    return EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="scenario-1",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="candidate", digest="a" * 64)],
        target_operator_session_id="session-1",
        evaluation_lane="model_role",
        designated_model_role=role,
    )


@pytest.mark.asyncio
async def test_finalize_evaluation_assignment_records_role_not_invoked():
    trace_service = MagicMock()
    pipeline = ChatPipelineService(
        event_service=MagicMock(),
        investigation_service=MagicMock(),
        request_builder=MagicMock(),
        g8e_agent=MagicMock(),
        memory_service=MagicMock(),
        memory_generation_service=MagicMock(),
        agent_activity_data_service=MagicMock(),
        evaluation_trace_service=trace_service,
    )

    evaluation_context = _evaluation_context("assistant")
    g8e_context = G8eHttpContext(user_id="user-1", evaluation_context=evaluation_context)
    triage_result = TriageResult(
        complexity=TriageComplexityClassification.COMPLEX,
        complexity_confidence=TriageConfidence.HIGH,
        intent=TriageIntentClassification.ACTION,
        intent_confidence=TriageConfidence.HIGH,
        intent_summary="analyze",
        request_posture=TriageRequestPosture.NORMAL,
        posture_confidence=TriageConfidence.HIGH,
        model_call=ModelCallTelemetry(
            agent_role="triage",
            model_role="lite",
            provider="G8EProvider",
            model="candidate",
            monotonic_start=1.0,
            monotonic_end=2.0,
        ),
    )
    controlled = apply_homogeneous_role_control(
        evaluation_context=evaluation_context,
        triage_complexity=triage_result.complexity,
        model_overrides=ModelOverrideResolver("candidate", "candidate", "candidate"),
        request_settings=G8eeUserSettings(),
    )
    inputs = AgentInputs.model_construct(
        case_id="case-1",
        investigation_id="inv-1",
        user_id="user-1",
        g8e_context=g8e_context,
        task_id="chat",
        agent_mode="g8e_bound",
        active_agent=ReasoningAgent.DASH,
        operator_bound=True,
        model_to_use="candidate",
        max_tokens=1024,
        conversation_history=[],
        system_instructions="",
        contents=[],
        triage_result=triage_result,
        controlled_role_assignment=controlled.controlled_role_assignment,
    )
    state = AgentStreamState(
        model_calls=[
            ModelCallTelemetry(
                agent_role="sage",
                model_role="primary",
                provider="G8EProvider",
                model="candidate",
                monotonic_start=3.0,
                monotonic_end=4.0,
            )
        ]
    )

    await pipeline._finalize_evaluation_assignment(
        g8e_context=g8e_context,
        inputs=inputs,
        state=state,
        memory_holder=None,
    )

    kwargs = trace_service.finalize.call_args.kwargs
    assert kwargs["role_outcome"] == "role_not_invoked"
    assert kwargs["controlled_role_assignment"].designated_model_role == "assistant"
