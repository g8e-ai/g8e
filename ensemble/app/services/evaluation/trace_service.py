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

from app.constants.env_vars import EnvVar
from app.errors import ValidationError
from app.models.evaluation_trace import (
    EvaluationAssignmentTrace,
    EvaluationControlledRoleAssignment,
    EvaluationGovernedActionRecord,
    EvaluationGraderCallRecord,
    EvaluationPolicyDecisionRecord,
    EvaluationRoleOutcome,
    EvaluationSemanticGradeRecord,
    EvaluationToolCallRecord,
    EvaluationToolDecisionRecord,
    EvaluationTraceStatus,
)
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry
from app.utils.path import resolve_project_root
from app.utils.security import resolve_safe_path_segments, validate_safe_filename
from app.utils.time_ids.timestamp import now

logger = logging.getLogger(__name__)

_TRACE_SCHEMA_VERSION = "2"


def _trace_root() -> Path:
    runtime_dir = os.environ.get(EnvVar.RUNTIME_DIR)
    base = Path(runtime_dir) if runtime_dir else resolve_project_root() / ".g8e"
    if not base.is_absolute():
        base = resolve_project_root() / base
    return (base / "data" / "evaluation" / "traces").resolve()


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
            raise ValueError("evaluation_context is required to begin a trace")

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
        finish_reason: str | None,
        status: EvaluationTraceStatus,
    ) -> EvaluationAssignmentTrace:
        evaluation = g8e_context.evaluation_context
        if evaluation is None:
            raise ValueError("evaluation_context is required to finalize a trace")

        trace = EvaluationAssignmentTrace(
            schema_version=_TRACE_SCHEMA_VERSION,
            evaluation_context=evaluation,
            chat_execution_id=g8e_context.execution_id,
            status=status,
            triage_model_call=triage_model_call,
            controlled_role_assignment=controlled_role_assignment,
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
            completed_at=now().isoformat(),
        )
        trace = trace.model_copy(update={"trace_digest": compute_trace_digest(trace)})
        self._write(trace)
        return trace

    def load(self, assignment_id: str, evaluation_attempt_id: str) -> EvaluationAssignmentTrace:
        path = _resolve_trace_path(assignment_id, evaluation_attempt_id)
        return self._read_trace(path)

    @staticmethod
    def _read_trace(path: Path) -> EvaluationAssignmentTrace:
        raw = path.read_text(encoding="utf-8")
        trace = EvaluationAssignmentTrace.model_validate_json(raw)
        expected = compute_trace_digest(trace.model_copy(update={"trace_digest": ""}))
        if trace.trace_digest and trace.trace_digest != expected:
            raise ValueError("evaluation trace digest mismatch")
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
