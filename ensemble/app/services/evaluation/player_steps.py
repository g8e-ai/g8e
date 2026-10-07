# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Per-player step records for a scored chat turn.

A scored turn runs the real g8ee chain. These builders turn what each player
actually produced into ``EvaluationPlayerStep`` records so a grader can judge a
player on its own job and a reader can see every player's output.

Nothing here calls a model or changes how the chain runs. Tribunal-chain steps
are recorded as the Tribunal finishes each stage, by observing the events and
results the Tribunal itself produces; the other players' steps are built after
the turn from the objects the pipeline already holds.
"""

from __future__ import annotations

import logging

from app.constants import ConsensusMember, EventType, RiskLevel
from app.models.agents.triage import TriageResult
from app.models.agents.tribunal import (
    TribunalAuditorCompletedPayload,
    TribunalAuditorFailedPayload,
    TribunalConsensusFailedPayload,
    TribunalPassCompletedPayload,
    TribunalVotingCompletedPayload,
    VoteBreakdown,
)
from app.models.base import G8eBaseModel
from app.models.evaluation_trace import (
    DesignatedModelRole,
    EvaluationAuditOutput,
    EvaluationCandidateOutput,
    EvaluationPlayer,
    EvaluationPlayerStep,
    EvaluationRiskOutput,
    EvaluationTextOutput,
    EvaluationToolCallRecord,
    EvaluationTriageOutput,
    EvaluationVoteOutput,
)
from app.models.memory import InvestigationMemory
from app.models.model_telemetry import ModelCallTelemetry
from app.models.tool_results import CommandRiskAnalysis

logger = logging.getLogger(__name__)

# A step's text output is bounded so one runaway answer cannot bloat the trace.
PLAYER_TEXT_LIMIT = 8000

_SEAT_PLAYERS: dict[ConsensusMember, EvaluationPlayer] = {
    ConsensusMember.AXIOM: "axiom",
    ConsensusMember.CONCORD: "concord",
    ConsensusMember.VARIANCE: "variance",
    ConsensusMember.PRAGMA: "pragma",
    ConsensusMember.NEMESIS: "nemesis",
}


def _tier(
    telemetry: ModelCallTelemetry | None, default: DesignatedModelRole
) -> DesignatedModelRole:
    """The tier a call resolved from, as the call itself reported it."""
    role = telemetry.model_role if telemetry is not None else None
    return role if role is not None else default


def _bounded(text: str) -> str:
    return text if len(text) <= PLAYER_TEXT_LIMIT else text[:PLAYER_TEXT_LIMIT]


class PlayerStepRecorder:
    """Collects one Tribunal run's steps as the Tribunal reports them.

    Implements ``TribunalObserver``. A recorder belongs to one
    ``run_commands_with_operator`` call: the Tribunal runs once per call, in at
    most two rounds, and the recorder follows the round so a seat's candidate
    and the vote that judged it carry the same ``round``.
    """

    def __init__(self) -> None:
        self._steps: list[EvaluationPlayerStep] = []
        self._round = 1

    @property
    def steps(self) -> list[EvaluationPlayerStep]:
        return list(self._steps)

    def observe(self, event_type: EventType, payload: G8eBaseModel) -> None:
        if event_type == EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED and isinstance(
            payload, TribunalPassCompletedPayload
        ):
            self._record_pass(payload)
        elif event_type == EventType.AI_CONSENSUS_VOTING_CONSENSUS_REACHED and isinstance(
            payload, TribunalVotingCompletedPayload
        ):
            self._record_vote(
                reached=True,
                winner=payload.vote_winner,
                vote_score=payload.vote_score,
                breakdown=payload.vote_breakdown,
            )
        elif event_type in (
            EventType.AI_CONSENSUS_VOTING_CONSENSUS_NOT_REACHED,
            EventType.AI_CONSENSUS_VOTING_CONSENSUS_FAILED,
        ) and isinstance(payload, TribunalConsensusFailedPayload):
            self._record_vote(
                reached=False,
                winner=None,
                vote_score=0.0,
                breakdown=payload.vote_breakdown,
            )
            if event_type == EventType.AI_CONSENSUS_VOTING_CONSENSUS_NOT_REACHED:
                self._round = 2
        elif event_type == EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED and isinstance(
            payload, TribunalAuditorCompletedPayload
        ):
            self._record_audit_completed(payload)
        elif event_type == EventType.AI_CONSENSUS_SESSION_AUDITOR_FAILED and isinstance(
            payload, TribunalAuditorFailedPayload
        ):
            self._record_audit_failed(payload)

    def observe_marshal_risk(self, command: str, analysis: CommandRiskAnalysis | None) -> None:
        """Record Marshal's classification of the command the Tribunal chose."""
        call = analysis.model_call if analysis is not None else None
        if analysis is None:
            self._append(
                "marshal_command",
                model_role="lite",
                succeeded=False,
                error_type="NoRiskAnalysis",
                error="Marshal returned no risk classification",
            )
            return
        self._append(
            "marshal_command",
            model_role=_tier(call, "lite"),
            model=call.model if call is not None else "",
            succeeded=call.succeeded if call is not None else True,
            error_type=call.error_type if call is not None else None,
            risk=EvaluationRiskOutput(
                risk_level=str(analysis.risk_level),
                command=command,
                blocked=analysis.risk_level == RiskLevel.HIGH,
            ),
        )

    def _record_pass(self, payload: TribunalPassCompletedPayload) -> None:
        call = payload.model_calls[0] if payload.model_calls else None
        self._append(
            _SEAT_PLAYERS[payload.member],
            model_role=_tier(call, "lite"),
            model=payload.model,
            tribunal_round=self._round,
            succeeded=payload.success,
            error_type=payload.error_type,
            error=payload.error,
            candidate=EvaluationCandidateOutput(command=payload.candidate),
        )

    def _record_vote(
        self,
        *,
        reached: bool,
        winner: str | None,
        vote_score: float,
        breakdown: VoteBreakdown,
    ) -> None:
        self._append(
            "tribunal",
            tribunal_round=self._round,
            vote=EvaluationVoteOutput(
                reached=reached,
                winner=winner,
                vote_score=vote_score,
                consensus_strength=breakdown.consensus_strength,
                tie_broken=breakdown.tie_broken,
                candidates_by_member=dict(breakdown.candidates_by_member),
            ),
        )

    def _record_audit_completed(self, payload: TribunalAuditorCompletedPayload) -> None:
        call = payload.model_calls[-1] if payload.model_calls else None
        self._append(
            "auditor",
            model_role=_tier(call, "primary"),
            model=call.model if call is not None else "",
            audit=EvaluationAuditOutput(
                passed=payload.passed,
                reason=str(payload.reason),
                revision=payload.revision,
                swap_to_member=payload.swap_to_member,
            ),
        )

    def _record_audit_failed(self, payload: TribunalAuditorFailedPayload) -> None:
        call = payload.model_calls[-1] if payload.model_calls else None
        self._append(
            "auditor",
            model_role=_tier(call, "primary"),
            model=call.model if call is not None else "",
            succeeded=False,
            error_type=str(payload.reason),
            error=payload.error,
        )

    def _append(
        self,
        player: EvaluationPlayer,
        *,
        model_role: DesignatedModelRole | None = None,
        model: str = "",
        tribunal_round: int | None = None,
        succeeded: bool = True,
        error_type: str | None = None,
        error: str | None = None,
        candidate: EvaluationCandidateOutput | None = None,
        vote: EvaluationVoteOutput | None = None,
        risk: EvaluationRiskOutput | None = None,
        audit: EvaluationAuditOutput | None = None,
    ) -> None:
        number = len(self._steps) + 1
        self._steps.append(
            EvaluationPlayerStep(
                step_id=f"tribunal-{number}",
                sequence=number,
                player=player,
                model_role=model_role,
                model=model,
                round=tribunal_round,
                succeeded=succeeded,
                error_type=error_type,
                error=error,
                candidate=candidate,
                vote=vote,
                risk=risk,
                audit=audit,
            )
        )


def attach_to_call(
    steps: list[EvaluationPlayerStep], parent_call_id: str, first_sequence: int
) -> list[EvaluationPlayerStep]:
    """Tie a Tribunal run's steps to the tool call that triggered it and renumber them.

    ``first_sequence`` is the next free position in the turn's chain, so steps
    from successive tool calls stay in the order the chain ran.
    """
    return [
        step.model_copy(
            update={
                "step_id": f"{parent_call_id}:{step.step_id}",
                "parent_call_id": parent_call_id,
                "sequence": first_sequence + index,
            }
        )
        for index, step in enumerate(steps)
    ]


def triage_step(triage: TriageResult, sequence: int) -> EvaluationPlayerStep:
    """Triage's classification, with the tier and model its call ran on."""
    call = triage.model_call
    failed = (call is not None and not call.succeeded) or triage.error_code is not None
    return EvaluationPlayerStep(
        step_id="triage",
        sequence=sequence,
        player="triage",
        model_role=_tier(call, "lite"),
        model=call.model if call is not None else "",
        succeeded=not failed,
        error_type=(call.error_type if call is not None else None) or triage.error_code,
        error=triage.error_message,
        triage=EvaluationTriageOutput(
            complexity=str(triage.complexity),
            complexity_confidence=str(triage.complexity_confidence),
            intent=str(triage.intent),
            intent_confidence=str(triage.intent_confidence),
            request_posture=str(triage.request_posture),
            posture_confidence=str(triage.posture_confidence),
            intent_summary=triage.intent_summary,
            error_code=triage.error_code,
        ),
    )


def reasoning_step(
    *,
    player: EvaluationPlayer,
    model_role: DesignatedModelRole,
    model: str,
    text: str,
    succeeded: bool,
    error: str | None,
    sequence: int,
) -> EvaluationPlayerStep:
    """Sage's or Dash's final answer for the turn."""
    return EvaluationPlayerStep(
        step_id=player,
        sequence=sequence,
        player=player,
        model_role=model_role,
        model=model,
        succeeded=succeeded,
        error=error,
        text=EvaluationTextOutput(text=_bounded(text)),
    )


def error_analysis_steps(
    tool_calls: list[EvaluationToolCallRecord], first_sequence: int
) -> list[EvaluationPlayerStep]:
    """Marshal's error analysis for each failed operator command that carried one."""
    steps: list[EvaluationPlayerStep] = []
    for call in tool_calls:
        if call.error_analysis is None:
            continue
        steps.append(
            EvaluationPlayerStep(
                step_id=f"{call.call_id}:marshal_error",
                sequence=first_sequence + len(steps),
                player="marshal_error",
                model_role="lite",
                parent_call_id=call.call_id,
                error_analysis=call.error_analysis,
            )
        )
    return steps


def memory_text(memory: InvestigationMemory) -> str:
    """Everything Codex wrote into a memory record, as one block of text.

    Codex must redact hostnames, addresses and credentials from all of it, so
    the whole record is what a grader reads.
    """
    parts = (
        memory.investigation_summary,
        memory.communication_preferences,
        memory.technical_background,
        memory.response_style,
        memory.problem_solving_approach,
        memory.interaction_style,
    )
    return "\n".join(part for part in parts if part)


def assemble_player_steps(
    *,
    triage: TriageResult | None,
    tribunal_steps: list[EvaluationPlayerStep],
    tool_calls: list[EvaluationToolCallRecord],
    reasoning: EvaluationPlayerStep | None,
    codex: EvaluationPlayerStep | None,
) -> list[EvaluationPlayerStep]:
    """Order the turn's chain the way it ran and number it.

    Triage first; then, for each tool call in order, the Tribunal-chain steps it
    triggered followed by Marshal's analysis of its failure; then the reasoning
    player's answer; then Codex. A step is never dropped: one whose tool call
    is not recorded is kept after the calls it could not be placed against.
    """
    ordered: list[EvaluationPlayerStep] = []
    if triage is not None:
        ordered.append(triage_step(triage, 1))
    placed: set[str] = set()
    for call in tool_calls:
        for step in tribunal_steps:
            if step.parent_call_id == call.call_id:
                ordered.append(step)
                placed.add(step.step_id)
        ordered.extend(error_analysis_steps([call], len(ordered) + 1))
    ordered.extend(step for step in tribunal_steps if step.step_id not in placed)
    if reasoning is not None:
        ordered.append(reasoning)
    if codex is not None:
        ordered.append(codex)
    return [step.model_copy(update={"sequence": index}) for index, step in enumerate(ordered, 1)]


def memory_step(
    *, model_call: ModelCallTelemetry | None, summary: str | None, sequence: int
) -> EvaluationPlayerStep:
    """Codex's post-turn memory update: the scrubbed summary it wrote."""
    succeeded = summary is not None and (model_call is None or model_call.succeeded)
    return EvaluationPlayerStep(
        step_id="codex",
        sequence=sequence,
        player="codex",
        model_role=_tier(model_call, "lite"),
        model=model_call.model if model_call is not None else "",
        succeeded=succeeded,
        error_type=model_call.error_type if model_call is not None else None,
        text=EvaluationTextOutput(text=_bounded(summary)) if summary is not None else None,
    )
