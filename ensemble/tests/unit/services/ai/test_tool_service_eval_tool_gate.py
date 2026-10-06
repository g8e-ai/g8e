# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Eval tool-gate bypass: scored requests always declare the full tool set.

Production strips tools from any model the static registry does not mark
``supports_tools`` (an unregistered tag resolves to ``UNKNOWN_MODEL_CONFIG``).
A request that carries an ``evaluation_context`` must never be pre-judged that
way (INV-EVAL-CAMP-07): the full production tool set for the agent mode is
declared and the trace records that the registry was bypassed. The bypass is
keyed on the context object only, never on an environment variable.
"""

import pytest
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant

from app.constants.prompts import AgentMode
from app.models.evaluation_trace import ToolGate
from app.models.model_configs import get_model_config
from app.services.evaluation.tool_gate import resolve_tool_gate

pytestmark = [pytest.mark.unit]

# Rollout-intake tag that no registry entry names: resolves to UNKNOWN_MODEL_CONFIG.
UNREGISTERED_MODEL = "qwen3.5:4b"
REGISTERED_TOOL_MODEL = "gemma4:e4b"


def _tool_service():
    from tests.fakes.tool_helpers import create_tool_service_fake

    return create_tool_service_fake(auto_approve=True)


def _evaluation_context() -> EvaluationInferenceContext:
    return EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="tool-select-grep",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model=UNREGISTERED_MODEL, digest="a" * 64)],
        target_operator_session_id="session-1",
        evaluation_lane="model_role",
        designated_model_role="primary",
    )


def _names(groups) -> list[str]:
    return [tool.name for group in groups for tool in group.tools]


def test_fixture_model_is_unregistered_and_withheld_in_production():
    """Characterizes E1 so the eval tests below cannot pass vacuously."""
    assert get_model_config(UNREGISTERED_MODEL).supports_tools is False
    assert get_model_config(REGISTERED_TOOL_MODEL).supports_tools is True


def test_production_request_for_unregistered_model_still_withholds_tools():
    """Q14: production behavior is out of scope; only eval bypasses the gate."""
    service = _tool_service()

    assert service.get_tools(AgentMode.G8E_BOUND, UNREGISTERED_MODEL) == []


def test_evaluation_request_for_unregistered_model_declares_full_tool_set():
    service = _tool_service()

    declared = service.get_tools(
        AgentMode.G8E_BOUND,
        UNREGISTERED_MODEL,
        evaluation_context=_evaluation_context(),
    )

    full_set = service.get_tools(AgentMode.G8E_BOUND, None)
    assert _names(declared)
    assert _names(declared) == _names(full_set)


@pytest.mark.parametrize("agent_mode", [AgentMode.G8E_BOUND, AgentMode.G8E_NOT_BOUND])
def test_evaluation_tool_set_follows_agent_mode(agent_mode):
    service = _tool_service()

    declared = service.get_tools(
        agent_mode, UNREGISTERED_MODEL, evaluation_context=_evaluation_context()
    )

    assert _names(declared) == _names(service.get_tools(agent_mode, None))


def test_evaluation_tool_set_for_registered_model_is_the_production_set():
    service = _tool_service()

    declared = service.get_tools(
        AgentMode.G8E_BOUND,
        REGISTERED_TOOL_MODEL,
        evaluation_context=_evaluation_context(),
    )

    assert _names(declared) == _names(service.get_tools(AgentMode.G8E_BOUND, REGISTERED_TOOL_MODEL))


def test_tool_gate_is_bypassed_only_for_evaluation_requests():
    assert resolve_tool_gate(None) is ToolGate.REGISTRY
    assert resolve_tool_gate(_evaluation_context()) is ToolGate.BYPASSED_FOR_EVAL
    assert ToolGate.BYPASSED_FOR_EVAL.value == "bypassed_for_eval"
    assert ToolGate.REGISTRY.value == "registry"
