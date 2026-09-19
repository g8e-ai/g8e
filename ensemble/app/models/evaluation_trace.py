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
EvaluationToolOutcome = Literal["pass", "fail", "unavailable"]
EvaluationPolicyOutcome = Literal["allow", "deny", "refused"]
EvaluationSemanticOutcome = Literal["pass", "fail", "unavailable"]


class EvaluationToolDecisionRecord(G8eBaseModel):
    """One model-originated tool selection recorded for campaign grading."""

    decision_id: str = Field(..., min_length=1)
    tool_name: str = Field(..., min_length=1)
    recognized: bool = True
    selected: bool = True
    permission_compliant: bool = True
    unnecessary: bool = False
    outcome: EvaluationToolOutcome = "pass"


class EvaluationToolCallRecord(G8eBaseModel):
    """One executed tool call with hashed arguments and governed binding metadata."""

    call_id: str = Field(..., min_length=1)
    tool_name: str = Field(..., min_length=1)
    arguments_hash: str = Field(default="", pattern=r"^[0-9a-f]{64}$|^$")
    success: bool = False
    is_operator_tool: bool = False
    execution_id: str | None = None
    error_type: str | None = None


class EvaluationGovernedActionRecord(G8eBaseModel):
    """Governed operator execution binding for one tool call."""

    binding_id: str = Field(..., min_length=1)
    transaction_id: str = Field(default="")
    operator_id: str = Field(default="")
    operator_session_id: str = Field(default="")
    receipt_status: str = Field(default="")
    policy_decision: EvaluationPolicyOutcome = "allow"


class EvaluationPolicyDecisionRecord(G8eBaseModel):
    """Policy outcome for one refused or rejected tool attempt."""

    decision_id: str = Field(..., min_length=1)
    tool_name: str = Field(default="")
    outcome: EvaluationPolicyOutcome
    detail: str = Field(default="")


class EvaluationSemanticGradeRecord(G8eBaseModel):
    """Semantic judge grade for one campaign assignment."""

    grade_id: str = Field(..., min_length=1)
    criterion_id: str = Field(default="semantic-judge")
    status: EvaluationSemanticOutcome = "unavailable"
    judge_variant_id: str = Field(default="")
    detail: str = Field(default="")
    score: int | None = Field(default=None, ge=1, le=5)


class EvaluationGraderCallRecord(G8eBaseModel):
    """Judge model call evidence for one semantic grade."""

    grader_call_id: str = Field(..., min_length=1)
    judge_variant_id: str = Field(default="")
    provider_attempt_id: str = Field(default="")


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
    designated_role_output: str | None = None
    tool_decisions: list[EvaluationToolDecisionRecord] = Field(default_factory=list)
    tool_calls: list[EvaluationToolCallRecord] = Field(default_factory=list)
    governed_actions: list[EvaluationGovernedActionRecord] = Field(default_factory=list)
    policy_decisions: list[EvaluationPolicyDecisionRecord] = Field(default_factory=list)
    semantic_grades: list[EvaluationSemanticGradeRecord] = Field(default_factory=list)
    grader_calls: list[EvaluationGraderCallRecord] = Field(default_factory=list)
    finish_reason: str | None = None
    trace_digest: str = Field(default="", pattern=r"^[0-9a-f]{64}$|^$")
    completed_at: str | None = None
