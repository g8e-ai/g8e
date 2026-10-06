# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Controlled homogeneous model-role assignment for scored chat evaluations."""

from __future__ import annotations

from typing import Literal

from g8e.models.internal_api import EvaluationInferenceContext

from app.constants import ReasoningAgent, TriageComplexityClassification
from app.constants.evaluation import DesignatedRoleToAgent
from app.errors import ValidationError
from app.llm.utils import ModelOverrideResolver, resolve_model_for_designated_role
from app.models.base import G8eBaseModel
from app.models.evaluation_trace import (
    DesignatedModelRole,
    EvaluationControlledRoleAssignment,
    EvaluationRoleOutcome,
    NaturalModelRole,
)
from app.models.model_telemetry import ModelCallTelemetry
from app.models.settings import G8eeUserSettings

_SCORED_AGENT_ROLES = frozenset({"sage", "dash"})


def resolve_scored_model_role(
    *,
    designated_model_role: str | None,
    active_agent: ReasoningAgent | None,
) -> Literal["primary", "assistant", "lite"]:
    """Return the model role the scored chat turn runs as.

    A campaign's designated role wins. Otherwise Dash runs as Assistant and
    every other agent as Primary. The chat pipeline takes the scored provider
    from this same role, so the provider, the role sent to governed dispatch,
    and the role's model always agree.
    """
    if designated_model_role:
        if designated_model_role in ("primary", "assistant", "lite"):
            return designated_model_role
        raise ValidationError(
            f"Unsupported designated model role: {designated_model_role}",
            field="designated_model_role",
            component="g8ee",
        )
    return "assistant" if active_agent == ReasoningAgent.DASH else "primary"


class ControlledRoleRouting(G8eBaseModel):
    """Resolved chat routing for one homogeneous model-role assignment."""

    controlled_role_assignment: EvaluationControlledRoleAssignment
    active_agent: ReasoningAgent
    model_to_use: str


def natural_model_role_from_triage(
    complexity: TriageComplexityClassification,
) -> NaturalModelRole:
    return "primary" if complexity == TriageComplexityClassification.COMPLEX else "assistant"


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
        raise ValidationError(
            "designated_model_role is required for model_role evaluation lane",
            field="designated_model_role",
            component="g8ee",
        )

    natural_model_role = natural_model_role_from_triage(triage_complexity)
    controlled_role_assignment = EvaluationControlledRoleAssignment(
        designated_model_role=designated,
        natural_model_role=natural_model_role,
        routing_agreement=designated == natural_model_role,
        triage_complexity=triage_complexity,
    )

    active_agent = DesignatedRoleToAgent[designated]
    model_to_use = resolve_model_for_designated_role(
        tier=designated,
        overrides=model_overrides,
        settings_primary_model=request_settings.llm.resolved_primary_model,
        settings_assistant_model=request_settings.llm.resolved_assistant_model,
        settings_lite_model=request_settings.llm.resolved_lite_model,
    )

    if not model_to_use:
        raise ValidationError(
            f"No model configured for designated homogeneous role {designated}",
            field="model_config",
            component="g8ee",
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
