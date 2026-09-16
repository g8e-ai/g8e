# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest

from app.constants import ReasoningAgent, TriageComplexityClassification
from app.llm.utils import ModelOverrideResolver
from app.models.model_telemetry import ModelCallTelemetry
from app.models.settings import G8eeUserSettings, LLMSettings
from app.services.evaluation.role_control import (
    apply_homogeneous_role_control,
    resolve_role_outcome,
    resolve_scored_provider_is_lite,
)
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant


def _settings() -> G8eeUserSettings:
    return G8eeUserSettings(
        llm=LLMSettings(
            primary_model="primary-default",
            assistant_model="assistant-default",
            lite_model="lite-default",
        )
    )


def _model_role_context(role: str) -> EvaluationInferenceContext:
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


@pytest.mark.parametrize(
    ("complexity", "designated", "expected_agent", "expected_model", "agreement"),
    [
        (
            TriageComplexityClassification.COMPLEX,
            "primary",
            ReasoningAgent.SAGE,
            "candidate-primary",
            True,
        ),
        (
            TriageComplexityClassification.SIMPLE,
            "assistant",
            ReasoningAgent.DASH,
            "candidate-assistant",
            True,
        ),
        (
            TriageComplexityClassification.COMPLEX,
            "assistant",
            ReasoningAgent.DASH,
            "candidate-assistant",
            False,
        ),
        (
            TriageComplexityClassification.SIMPLE,
            "lite",
            ReasoningAgent.DASH,
            "candidate-lite",
            False,
        ),
    ],
)
def test_apply_homogeneous_role_control_overrides_triage_routing(
    complexity,
    designated,
    expected_agent,
    expected_model,
    agreement,
):
    routing = apply_homogeneous_role_control(
        evaluation_context=_model_role_context(designated),
        triage_complexity=complexity,
        model_overrides=ModelOverrideResolver(
            primary_model="candidate-primary",
            assistant_model="candidate-assistant",
            lite_model="candidate-lite",
        ),
        request_settings=_settings(),
    )

    assert routing is not None
    assert routing.active_agent == expected_agent
    assert routing.model_to_use == expected_model
    assert routing.controlled_role_assignment.designated_model_role == designated
    assert routing.controlled_role_assignment.natural_model_role == (
        "primary" if complexity == TriageComplexityClassification.COMPLEX else "assistant"
    )
    assert routing.controlled_role_assignment.routing_agreement is agreement


def test_apply_homogeneous_role_control_skips_system_lane():
    context = EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="scenario-1",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="candidate", digest="a" * 64)],
        target_operator_session_id="session-1",
        evaluation_lane="system",
    )
    assert (
        apply_homogeneous_role_control(
            evaluation_context=context,
            triage_complexity=TriageComplexityClassification.COMPLEX,
            model_overrides=ModelOverrideResolver("a", "b", "c"),
            request_settings=_settings(),
        )
        is None
    )


@pytest.mark.parametrize(
    ("designated", "complexity", "expected"),
    [
        ("lite", TriageComplexityClassification.COMPLEX, True),
        ("lite", TriageComplexityClassification.SIMPLE, True),
        ("assistant", TriageComplexityClassification.SIMPLE, True),
        ("assistant", TriageComplexityClassification.COMPLEX, False),
        ("primary", TriageComplexityClassification.SIMPLE, True),
        ("primary", TriageComplexityClassification.COMPLEX, False),
        (None, TriageComplexityClassification.SIMPLE, True),
        (None, TriageComplexityClassification.COMPLEX, False),
    ],
)
def test_resolve_scored_provider_is_lite_follows_designated_role(
    designated, complexity, expected
):
    assert (
        resolve_scored_provider_is_lite(
            designated_model_role=designated,
            triage_complexity=complexity,
        )
        is expected
    )


def test_resolve_role_outcome_requires_scored_agent_call():
    designated = "assistant"
    assert (
        resolve_role_outcome(
            designated,
            [
                ModelCallTelemetry(
                    agent_role="triage",
                    model_role="lite",
                    provider="G8EProvider",
                    model="m",
                    monotonic_start=1.0,
                    monotonic_end=2.0,
                ),
                ModelCallTelemetry(
                    agent_role="dash",
                    model_role="primary",
                    provider="G8EProvider",
                    model="m",
                    monotonic_start=3.0,
                    monotonic_end=4.0,
                ),
            ],
        )
        == "role_not_invoked"
    )
    assert (
        resolve_role_outcome(
            designated,
            [
                ModelCallTelemetry(
                    agent_role="dash",
                    model_role="assistant",
                    provider="G8EProvider",
                    model="m",
                    monotonic_start=3.0,
                    monotonic_end=4.0,
                )
            ],
        )
        == "invoked"
    )
