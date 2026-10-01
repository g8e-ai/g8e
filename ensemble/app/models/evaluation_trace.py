# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

from enum import StrEnum
from typing import Literal

from g8e.models.internal_api import EvaluationInferenceContext

from app.models.base import Field, G8eBaseModel
from app.models.model_telemetry import ModelCallTelemetry
from app.constants import TriageComplexityClassification


class ToolGate(StrEnum):
    """Which authority decided the tool set declared to the provider.

    ``REGISTRY`` consults the static per-model ``supports_tools`` table (the
    production path). ``BYPASSED_FOR_EVAL`` declares the full production tool
    set for the agent mode without consulting that table; it is keyed on the
    request's ``evaluation_context`` and is the one deliberate divergence
    between a scored request and production chat (INV-EVAL-CAMP-07).
    """

    REGISTRY = "registry"
    BYPASSED_FOR_EVAL = "bypassed_for_eval"


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


class EvaluationErrorAnalysisSummary(G8eBaseModel):
    """Bounded summary of the LLM error analysis shown to the model after a
    failed operator command (``suggested_fix`` / ``suggested_command`` /
    ``should_escalate``), so grading can tell guidance followed from guidance
    ignored."""

    error_category: str = Field(default="")
    root_cause: str = Field(default="")
    suggested_fix: str | None = None
    suggested_command: str | None = None
    should_escalate: bool = False


class EvaluationToolCallRecord(G8eBaseModel):
    """One executed tool call: the model's exact arguments, the resolved command,
    and the tool's returned result.

    arguments_json and result_json are canonical JSON text (not nested objects)
    so the cross-language trace digest never depends on float formatting, and
    arguments_hash is sha256(arguments_json) so any reader can verify the
    arguments shown against the hash bound into the trace.

    ``loop_turn`` is the tool-loop turn whose model response issued the call,
    and ``error`` / ``error_type`` / ``suggestion`` / ``error_analysis`` are the
    guidance the model was shown for this result, so a reader can order the
    trajectory and see what the model did after each correction.
    """

    call_id: str = Field(..., min_length=1)
    tool_name: str = Field(..., min_length=1)
    arguments_json: str = Field(default="")
    arguments_hash: str = Field(default="", pattern=r"^[0-9a-f]{64}$|^$")
    command: str = Field(default="")
    result_json: str = Field(default="")
    success: bool = False
    is_operator_tool: bool = False
    execution_id: str | None = None
    error_type: str | None = None
    loop_turn: int | None = Field(default=None, ge=1)
    error: str | None = None
    suggestion: str | None = None
    error_analysis: EvaluationErrorAnalysisSummary | None = None


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


class EvaluationProviderToolRejection(G8eBaseModel):
    """The provider itself rejected the tool declaration for this model.

    Recorded as its own explicit reason (``PROVIDER_REJECTED_TOOL_DECLARATION``)
    so a model that cannot accept tools is a plain FAIL on tool scenarios, never
    an infrastructure error and never a capability label that gates the result.
    ``reason`` is the gateway's public-safe rejection text.
    """

    model: str = Field(..., min_length=1)
    reason: str = Field(default="")


class EvaluationSeedApplication(G8eBaseModel):
    """What the investigation seed actually wrote before the scored turn.

    The seed itself is echoed whole in ``evaluation_context.seed``; these
    counts prove it was applied (not just received) so a reader can tell a
    seeded investigation from a cold one without trusting the request.
    """

    turns: int = Field(default=0, ge=0)
    history_events: int = Field(default=0, ge=0)
    case_memory: bool = False


class EvaluationAssignmentTrace(G8eBaseModel):
    """Immutable application-owned record of one scored chat assignment.

    The tool names actually sent to the provider are recorded per call, as
    ``ModelCallTelemetry.tools_declared`` on each entry of ``model_calls``
    (captured at the provider boundary, never recomputed from the registry).
    ``tool_gate`` records which authority decided that set.

    The remaining fields record the deliberate divergences between a scored
    request and production chat, each keyed on ``evaluation_context``:
    ``user_memories_suppressed`` (user-wide memories are not read, because they
    are artifacts of other assignments), and ``tool_turn_limit_reached`` (the
    continue-approval at ``AGENT_MAX_TOOL_TURNS`` is denied immediately instead
    of waiting for a human who is not there). ``seed_application`` records what
    the investigation seed wrote.
    """

    schema_version: str = Field(default="1")
    evaluation_context: EvaluationInferenceContext
    chat_execution_id: str = Field(..., min_length=1)
    status: EvaluationTraceStatus = Field(default="running")
    triage_model_call: ModelCallTelemetry | None = None
    controlled_role_assignment: EvaluationControlledRoleAssignment | None = None
    tool_gate: ToolGate | None = None
    provider_tool_rejection: EvaluationProviderToolRejection | None = None
    tool_turn_limit_reached: bool = False
    user_memories_suppressed: bool = False
    seed_application: EvaluationSeedApplication | None = None
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
    error: str | None = None
    trace_digest: str = Field(default="", pattern=r"^[0-9a-f]{64}$|^$")
    completed_at: str | None = None
