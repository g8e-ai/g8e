# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Controlled homogeneous model-role assignment for scored chat evaluations."""

from __future__ import annotations

from typing import Literal

from app.constants import ReasoningAgent, TriageComplexityClassification
from app.llm.utils import ModelOverrideResolver, resolve_model
from app.models.base import G8eBaseModel
from app.models.evaluation_trace import EvaluationControlledRoleAssignment
from app.models.model_telemetry import ModelCallTelemetry
from app.models.settings import G8eeUserSettings
from g8e.models.internal_api import EvaluationInferenceContext

DesignatedModelRole = Literal["primary", "assistant", "lite"]
NaturalModelRole = Literal["primary", "assistant"]
EvaluationRoleOutcome = Literal["invoked", "role_not_invoked"]
_SCORED_AGENT_ROLES = frozenset({"sage", "dash"})


def resolve_scored_provider_is_lite(
    *,
    designated_model_role: str | None,
    triage_complexity: TriageComplexityClassification,
) -> bool:
    """Return whether the scored chat provider should use the lite tier.

    Homogeneous role control binds any campaign model to primary, assistant,
    or lite slots. Provider selection must follow the designated scored role,
    not triage complexity alone.
    """
    if designated_model_role == "lite":
        return True
    return triage_complexity == TriageComplexityClassification.SIMPLE


class ControlledRoleRouting(G8eBaseModel):
    """Resolved chat routing for one homogeneous model-role assignment."""

    controlled_role_assignment: EvaluationControlledRoleAssignment
    active_agent: ReasoningAgent
    model_to_use: str


def natural_model_role_from_triage(
    complexity: TriageComplexityClassification,
) -> NaturalModelRole:
    return (
        "primary"
        if complexity == TriageComplexityClassification.COMPLEX
        else "assistant"
    )


def apply_homogeneous_role_control(
    *,
    evaluation_context: EvaluationInferenceContext,
    triage_complexity: TriageComplexityClassification,
    model_overrides: ModelOverrideResolver,
    request_settings: G8eeUserSettings,
) -> ControlledRoleRouting | None:
    """Apply immutable homogeneous role control after triage is recorded."""
    if evaluation_context.evaluation_lane != "model_role":
        return None

    designated = evaluation_context.designated_model_role
    if designated is None:
        raise ValueError("designated_model_role is required for model_role evaluation lane")

    natural_model_role = natural_model_role_from_triage(triage_complexity)
    controlled_role_assignment = EvaluationControlledRoleAssignment(
        designated_model_role=designated,
        natural_model_role=natural_model_role,
        routing_agreement=designated == natural_model_role,
        triage_complexity=triage_complexity,
    )

    if designated == "primary":
        active_agent = ReasoningAgent.SAGE
        model_to_use = resolve_model(
            tier="primary",
            primary_override=model_overrides.primary_model,
            assistant_override=None,
            lite_override=None,
            settings_primary_model=request_settings.llm.resolved_primary_model,
            settings_assistant_model=request_settings.llm.resolved_assistant_model,
            settings_lite_model=request_settings.llm.resolved_lite_model,
        )
    elif designated == "assistant":
        active_agent = ReasoningAgent.DASH
        model_to_use = resolve_model(
            tier="assistant",
            primary_override=None,
            assistant_override=model_overrides.assistant_model,
            lite_override=None,
            settings_primary_model=request_settings.llm.resolved_primary_model,
            settings_assistant_model=request_settings.llm.resolved_assistant_model,
            settings_lite_model=request_settings.llm.resolved_lite_model,
        )
    else:
        active_agent = ReasoningAgent.DASH
        model_to_use = resolve_model(
            tier="lite",
            primary_override=None,
            assistant_override=None,
            lite_override=model_overrides.lite_model,
            settings_primary_model=request_settings.llm.resolved_primary_model,
            settings_assistant_model=request_settings.llm.resolved_assistant_model,
            settings_lite_model=request_settings.llm.resolved_lite_model,
        )

    if not model_to_use:
        raise ValueError(
            f"No model configured for designated homogeneous role {designated}"
        )

    return ControlledRoleRouting(
        controlled_role_assignment=controlled_role_assignment,
        active_agent=active_agent,
        model_to_use=model_to_use,
    )


def resolve_role_outcome(
    designated_model_role: DesignatedModelRole,
    model_calls: list[ModelCallTelemetry],
) -> EvaluationRoleOutcome:
    """Return invoked when the scored agent loop produced the designated role."""
    for call in model_calls:
        if call.agent_role not in _SCORED_AGENT_ROLES:
            continue
        if call.model_role == designated_model_role:
            return "invoked"
    return "role_not_invoked"
