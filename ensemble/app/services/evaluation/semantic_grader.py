# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import logging

from app.llm import get_llm_provider
from app.models.evaluation_trace import (
    EvaluationGraderCallRecord,
    EvaluationSemanticGradeRecord,
    EvaluationToolCallRecord,
)
from app.models.http_context import G8eHttpContext
from app.models.settings import G8eeUserSettings
from app.services.ai.eval_judge import EvalJudge, EvalJudgeError
from g8e.models.internal_api import EvaluationGoldSummary, EvaluationInferenceContext

logger = logging.getLogger(__name__)


def _build_interaction_trace(
    designated_role_output: str | None,
    tool_calls: list[EvaluationToolCallRecord],
) -> str:
    sections: list[str] = []
    if designated_role_output:
        sections.append(f"Designated role output:\n{designated_role_output}")
    if tool_calls:
        tool_lines = [
            f"- {call.tool_name} success={call.success} execution_id={call.execution_id or ''}"
            for call in tool_calls
        ]
        sections.append("Tool calls:\n" + "\n".join(tool_lines))
    return "\n\n".join(sections) if sections else "(no interaction evidence recorded)"


async def grade_campaign_assignment_semantically(
    *,
    evaluation_context: EvaluationInferenceContext,
    g8e_context: G8eHttpContext,
    request_settings: G8eeUserSettings,
    gold_summary: EvaluationGoldSummary,
    designated_role_output: str | None,
    tool_calls: list[EvaluationToolCallRecord],
) -> tuple[list[EvaluationSemanticGradeRecord], list[EvaluationGraderCallRecord]]:
    grade_id = f"{evaluation_context.assignment_id}:semantic-judge"
    provider = get_llm_provider(request_settings.llm)
    judge = EvalJudge(
        provider=provider,
        settings=request_settings.eval_judge,
        g8e_context=g8e_context,
    )
    try:
        grade = await judge.grade_turn(
            user_query=gold_summary.user_prompt,
            interaction_trace=_build_interaction_trace(designated_role_output, tool_calls),
            expected_behavior=gold_summary.expected_behavior,
            required_concepts=list(gold_summary.required_concepts),
            expected_tools=list(gold_summary.expected_tools),
            forbidden_tools=list(gold_summary.forbidden_tools),
        )
    except EvalJudgeError as exc:
        logger.warning(
            "Semantic judge failed for assignment %s: %s",
            evaluation_context.assignment_id,
            exc,
        )
        return (
            [
                EvaluationSemanticGradeRecord(
                    grade_id=grade_id,
                    status="unavailable",
                    judge_variant_id=request_settings.eval_judge.model or "",
                    detail=str(exc),
                )
            ],
            [],
        )

    semantic_grade = EvaluationSemanticGradeRecord(
        grade_id=grade_id,
        status="pass" if grade.passed else "fail",
        judge_variant_id=request_settings.eval_judge.model or "",
        detail=grade.reasoning,
        score=grade.score,
    )
    grader_calls = [
        EvaluationGraderCallRecord(
            grader_call_id=f"{grade_id}:call-{index + 1}",
            judge_variant_id=call.model or request_settings.eval_judge.model or "",
            provider_attempt_id=call.provider_attempt_id or "",
        )
        for index, call in enumerate(grade.model_calls)
    ]
    return [semantic_grade], grader_calls
