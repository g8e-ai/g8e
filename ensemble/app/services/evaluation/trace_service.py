# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import logging
import os
from pathlib import Path

from g8e.eval.v1.trace_digest import compute_chat_probe_trace_digest, marshal_canonical_json

from app.constants.paths import resolve_runtime_dir
from app.errors import ValidationError
from app.models.evaluation_trace import (
    EvaluationAssignmentTrace,
    EvaluationControlledRoleAssignment,
    EvaluationGovernedActionRecord,
    EvaluationGraderCallRecord,
    EvaluationPolicyDecisionRecord,
    EvaluationProviderToolRejection,
    EvaluationRoleOutcome,
    EvaluationSeedApplication,
    EvaluationSemanticGradeRecord,
    EvaluationToolCallRecord,
    EvaluationToolDecisionRecord,
    EvaluationTraceStatus,
    ToolGate,
)
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry
from app.utils.security import resolve_safe_path_segments, validate_safe_filename
from app.utils.time_ids.timestamp import now

logger = logging.getLogger(__name__)

# 3: tool_calls carry arguments_json/command/result_json, and arguments_hash
# is now sha256(arguments_json) rather than a hash of the command or result.
# 4: adds the optional `error` field carrying the terminal stream error message
# for a failed assignment, so failure diagnosis no longer depends on ensemble
# server logs that outlive the run.
# 5: proves the opportunity was real and records the trajectory. Adds
# `tool_gate` (registry | bypassed_for_eval), `provider_tool_rejection`, per
# model call `tools_declared` (names as sent to the provider), and per tool call
# `loop_turn`, `error`, `suggestion`, and `error_analysis`.
# 6: records the seeded investigation and the deliberate eval-only divergences.
# Adds `seed_application` (counts of what the seed wrote), `user_memories_suppressed`,
# and `tool_turn_limit_reached`. The seed and workspace themselves are echoed
# whole inside `evaluation_context`.
_TRACE_SCHEMA_VERSION = "6"


def _trace_root() -> Path:
    return (resolve_runtime_dir() / "data" / "evaluation" / "traces").resolve()


def validated_trace_ids(assignment_id: str, evaluation_attempt_id: str) -> tuple[str, str]:
    """Validate evaluation trace path parameters before filesystem access."""
    try:
        return (
            validate_safe_filename(assignment_id, label="assignment_id"),
            validate_safe_filename(evaluation_attempt_id, label="evaluation_attempt_id"),
        )
    except ValueError as exc:
        raise ValidationError(str(exc), component="g8ee") from exc


def _resolve_trace_path(assignment_id: str, evaluation_attempt_id: str) -> Path:
    safe_assignment_id, safe_attempt_id = validated_trace_ids(assignment_id, evaluation_attempt_id)
    try:
        return resolve_safe_path_segments(
            _trace_root(),
            safe_assignment_id,
            f"{safe_attempt_id}.json",
        )
    except ValueError as exc:
        raise ValidationError(str(exc), component="g8ee") from exc


def compute_trace_digest(trace: EvaluationAssignmentTrace) -> str:
    payload = trace.model_copy(update={"trace_digest": ""}).model_dump(mode="json")
    return compute_chat_probe_trace_digest(payload)


class EvaluationTraceService:
    """Persist immutable evaluation assignment traces before lifecycle delivery."""

    def trace_file(self, assignment_id: str, evaluation_attempt_id: str) -> Path:
        return _resolve_trace_path(assignment_id, evaluation_attempt_id)

    def begin(
        self,
        g8e_context: G8eHttpContext,
        *,
        triage_model_call: ModelCallTelemetry | None = None,
        controlled_role_assignment: EvaluationControlledRoleAssignment | None = None,
    ) -> EvaluationAssignmentTrace:
        evaluation = g8e_context.evaluation_context
        if evaluation is None:
            raise ValidationError(
                "evaluation_context is required to begin a trace",
                field="evaluation_context",
                component="g8ee",
            )

        trace = EvaluationAssignmentTrace(
            schema_version=_TRACE_SCHEMA_VERSION,
            evaluation_context=evaluation,
            chat_execution_id=g8e_context.execution_id,
            status="running",
            triage_model_call=triage_model_call,
            controlled_role_assignment=controlled_role_assignment,
        )
        trace = trace.model_copy(update={"trace_digest": compute_trace_digest(trace)})
        self._write(trace)
        return trace

    def finalize(
        self,
        g8e_context: G8eHttpContext,
        *,
        model_calls: list[ModelCallTelemetry],
        triage_model_call: ModelCallTelemetry | None = None,
        controlled_role_assignment: EvaluationControlledRoleAssignment | None = None,
        role_outcome: EvaluationRoleOutcome | None = None,
        designated_role_output: str | None = None,
        tool_decisions: list[EvaluationToolDecisionRecord] | None = None,
        tool_calls: list[EvaluationToolCallRecord] | None = None,
        governed_actions: list[EvaluationGovernedActionRecord] | None = None,
        policy_decisions: list[EvaluationPolicyDecisionRecord] | None = None,
        semantic_grades: list[EvaluationSemanticGradeRecord] | None = None,
        grader_calls: list[EvaluationGraderCallRecord] | None = None,
        tool_gate: ToolGate | None = None,
        provider_tool_rejection: EvaluationProviderToolRejection | None = None,
        tool_turn_limit_reached: bool = False,
        user_memories_suppressed: bool = False,
        seed_application: EvaluationSeedApplication | None = None,
        finish_reason: str | None,
        status: EvaluationTraceStatus,
        error: str | None = None,
    ) -> EvaluationAssignmentTrace:
        evaluation = g8e_context.evaluation_context
        if evaluation is None:
            raise ValidationError(
                "evaluation_context is required to finalize a trace",
                field="evaluation_context",
                component="g8ee",
            )

        trace = EvaluationAssignmentTrace(
            schema_version=_TRACE_SCHEMA_VERSION,
            evaluation_context=evaluation,
            chat_execution_id=g8e_context.execution_id,
            status=status,
            triage_model_call=triage_model_call,
            controlled_role_assignment=controlled_role_assignment,
            tool_gate=tool_gate,
            provider_tool_rejection=provider_tool_rejection,
            tool_turn_limit_reached=tool_turn_limit_reached,
            user_memories_suppressed=user_memories_suppressed,
            seed_application=seed_application,
            model_calls=list(model_calls),
            role_outcome=role_outcome,
            designated_role_output=designated_role_output,
            tool_decisions=list(tool_decisions or []),
            tool_calls=list(tool_calls or []),
            governed_actions=list(governed_actions or []),
            policy_decisions=list(policy_decisions or []),
            semantic_grades=list(semantic_grades or []),
            grader_calls=list(grader_calls or []),
            finish_reason=finish_reason,
            error=error,
            completed_at=now().isoformat(),
        )
        trace = trace.model_copy(update={"trace_digest": compute_trace_digest(trace)})
        self._write(trace)
        return trace

    def finalize_crashed(
        self,
        g8e_context: G8eHttpContext,
        *,
        error: str,
        tool_gate: ToolGate | None = None,
        seed_application: EvaluationSeedApplication | None = None,
    ) -> EvaluationAssignmentTrace:
        """Close a trace left ``running`` by a pipeline crash as ``failed``.

        Without this the Go waiter polls a ``running`` trace until it times out
        instead of reading a terminal one. A trace that already reached a
        terminal status is returned unchanged, so a late crash never overwrites
        a completed result; the triage call and role assignment recorded while
        it was running are kept.
        """
        evaluation = g8e_context.evaluation_context
        if evaluation is None:
            raise ValidationError(
                "evaluation_context is required to finalize a trace",
                field="evaluation_context",
                component="g8ee",
            )
        path = self.trace_file(evaluation.assignment_id, evaluation.evaluation_attempt_id)
        running = self._read_trace(path) if path.exists() else None
        if running is not None and running.status != "running":
            return running
        return self.finalize(
            g8e_context,
            model_calls=[],
            triage_model_call=running.triage_model_call if running else None,
            controlled_role_assignment=running.controlled_role_assignment if running else None,
            tool_gate=tool_gate,
            seed_application=seed_application,
            finish_reason="error",
            status="failed",
            error=error,
        )

    def load(self, assignment_id: str, evaluation_attempt_id: str) -> EvaluationAssignmentTrace:
        path = _resolve_trace_path(assignment_id, evaluation_attempt_id)
        return self._read_trace(path)

    @staticmethod
    def _read_trace(path: Path) -> EvaluationAssignmentTrace:
        raw = path.read_text(encoding="utf-8")
        trace = EvaluationAssignmentTrace.model_validate_json(raw)
        expected = compute_trace_digest(trace.model_copy(update={"trace_digest": ""}))
        if trace.trace_digest and trace.trace_digest != expected:
            raise ValidationError(
                "evaluation trace digest mismatch",
                field="trace_digest",
                details={"expected": expected, "actual": trace.trace_digest},
                component="g8ee",
            )
        return trace

    def _write(self, trace: EvaluationAssignmentTrace) -> None:
        evaluation = trace.evaluation_context
        path = self.trace_file(evaluation.assignment_id, evaluation.evaluation_attempt_id)
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp_path = path.with_suffix(".json.tmp")
        payload = trace.model_dump(mode="json")
        tmp_path.write_bytes(marshal_canonical_json(payload))
        os.replace(tmp_path, path)
        logger.info(
            "Persisted evaluation trace assignment_id=%s attempt_id=%s status=%s",
            evaluation.assignment_id,
            evaluation.evaluation_attempt_id,
            trace.status,
        )
