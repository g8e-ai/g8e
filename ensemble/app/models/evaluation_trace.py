# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from typing import Literal

from g8e.models.internal_api import EvaluationInferenceContext

from app.models.base import Field, G8eBaseModel
from app.models.model_telemetry import ModelCallTelemetry
from app.constants import TriageComplexityClassification

EvaluationTraceStatus = Literal["running", "completed", "failed"]
DesignatedModelRole = Literal["primary", "assistant", "lite"]
NaturalModelRole = Literal["primary", "assistant"]
EvaluationRoleOutcome = Literal["invoked", "role_not_invoked"]


class EvaluationControlledRoleAssignment(G8eBaseModel):
    """Immutable controlled role binding recorded after triage."""

    designated_model_role: DesignatedModelRole
    natural_model_role: NaturalModelRole
    routing_agreement: bool
    triage_complexity: TriageComplexityClassification


class EvaluationAssignmentTrace(G8eBaseModel):
    """Immutable application-owned record of one scored chat assignment."""

    schema_version: str = Field(default="1")
    evaluation_context: EvaluationInferenceContext
    chat_execution_id: str = Field(..., min_length=1)
    status: EvaluationTraceStatus = Field(default="running")
    triage_model_call: ModelCallTelemetry | None = None
    controlled_role_assignment: EvaluationControlledRoleAssignment | None = None
    model_calls: list[ModelCallTelemetry] = Field(default_factory=list)
    role_outcome: EvaluationRoleOutcome | None = None
    finish_reason: str | None = None
    trace_digest: str = Field(default="", pattern=r"^[0-9a-f]{64}$|^$")
    completed_at: str | None = None
