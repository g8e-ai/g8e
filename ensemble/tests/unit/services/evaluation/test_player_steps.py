# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import json
from pathlib import Path

import pytest

from app.constants import (
    AuditorReason,
    ConsensusMember,
    EventType,
    RiskLevel,
    TriageComplexityClassification,
    TriageConfidence,
    TriageIntentClassification,
    TriageRequestPosture,
)
from app.models.agents.triage import TriageResult
from app.models.agents.tribunal import (
    TribunalAuditorCompletedPayload,
    TribunalAuditorFailedPayload,
    TribunalConsensusFailedPayload,
    TribunalPassCompletedPayload,
    TribunalVotingCompletedPayload,
    VoteBreakdown,
)
from app.models.evaluation_trace import (
    EvaluationErrorAnalysisSummary,
    EvaluationPlayerStep,
    EvaluationTextOutput,
    EvaluationToolCallRecord,
    EvaluationTriageOutput,
)
from app.models.memory import InvestigationMemory
from app.models.model_telemetry import ModelCallTelemetry
from app.models.tool_results import CommandRiskAnalysis
from app.services.ai.tribunal.emitter import TribunalEmitter
from app.services.evaluation.player_steps import (
    PLAYER_TEXT_LIMIT,
    PlayerStepRecorder,
    assemble_player_steps,
    attach_to_call,
    error_analysis_steps,
    memory_step,
    memory_text,
    reasoning_step,
    triage_step,
)


def _call(
    agent_role: str, model_role: str, *, succeeded: bool = True, error_type: str | None = None
):
    return ModelCallTelemetry(
        agent_role=agent_role,
        model_role=model_role,
        provider="OllamaProvider",
        model="m1",
        monotonic_start=1.0,
        monotonic_end=2.0,
        succeeded=succeeded,
        error_type=error_type,
    )


def _pass(member: ConsensusMember, candidate: str | None, *, error: str | None = None):
    return TribunalPassCompletedPayload(
        pass_index=0,
        member=member,
        candidate=candidate,
        success=error is None,
        error=error,
        error_type="EmptyResponseError" if error else None,
        model="m1",
        model_calls=[_call("tribunal", "lite")],
    )


def _breakdown(winner: str | None, strength: float) -> VoteBreakdown:
    return VoteBreakdown(
        candidates_by_member={"axiom": "ls /a", "concord": "ls /a", "variance": "ls /b"},
        candidates_by_command={"ls /a": ["axiom", "concord"], "ls /b": ["variance"]},
        winner=winner,
        winner_supporters=["axiom", "concord"] if winner else [],
        consensus_strength=strength,
    )


async def _emit(recorder: PlayerStepRecorder, event_type: EventType, payload) -> None:
    # The emitter calls the observer before it checks for an event service, so
    # a bare emitter is enough to drive the recorder exactly as production does.
    await TribunalEmitter(None, None, observer=recorder).emit(event_type, payload)


@pytest.mark.asyncio
async def test_a_one_round_tribunal_is_recorded_as_seats_then_vote_then_marshal_then_auditor():
    recorder = PlayerStepRecorder()
    for member, command in (
        (ConsensusMember.AXIOM, "ls /a"),
        (ConsensusMember.CONCORD, "ls /a"),
        (ConsensusMember.VARIANCE, "ls /b"),
    ):
        await _emit(recorder, EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED, _pass(member, command))
    await _emit(
        recorder,
        EventType.AI_CONSENSUS_VOTING_CONSENSUS_REACHED,
        TribunalVotingCompletedPayload(
            vote_winner="ls /a",
            vote_score=0.66,
            num_candidates=3,
            request="list a",
            vote_breakdown=_breakdown("ls /a", 0.66),
        ),
    )
    recorder.observe_marshal_risk(
        "ls /a",
        CommandRiskAnalysis(risk_level=RiskLevel.LOW, model_call=_call("marshal_command", "lite")),
    )
    await _emit(
        recorder,
        EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED,
        TribunalAuditorCompletedPayload(
            passed=True,
            reason=AuditorReason.OK,
            model_calls=[_call("auditor", "primary")],
        ),
    )

    steps = recorder.steps
    assert [step.player for step in steps] == [
        "axiom",
        "concord",
        "variance",
        "tribunal",
        "marshal_command",
        "auditor",
    ]
    assert [step.sequence for step in steps] == [1, 2, 3, 4, 5, 6]
    assert {step.round for step in steps[:4]} == {1}
    assert steps[0].candidate is not None
    assert steps[0].candidate.command == "ls /a"
    assert steps[0].model_role == "lite"
    assert steps[3].vote is not None
    assert steps[3].vote.reached
    assert steps[3].vote.winner == "ls /a"
    assert steps[3].vote.candidates_by_member["variance"] == "ls /b"
    assert steps[4].risk is not None
    assert steps[4].risk.risk_level == "LOW"
    assert not steps[4].risk.blocked
    assert steps[5].model_role == "primary"
    assert steps[5].audit is not None
    assert steps[5].audit.passed
    assert steps[5].audit.reason == "ok"


@pytest.mark.asyncio
async def test_a_failed_seat_keeps_its_error_instead_of_a_candidate():
    recorder = PlayerStepRecorder()
    await _emit(
        recorder,
        EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED,
        _pass(ConsensusMember.NEMESIS, None, error="Pass 4 (nemesis): empty response"),
    )

    (step,) = recorder.steps
    assert step.player == "nemesis"
    assert not step.succeeded
    assert step.error_type == "EmptyResponseError"
    assert step.error == "Pass 4 (nemesis): empty response"
    assert step.candidate is not None
    assert step.candidate.command is None


@pytest.mark.asyncio
async def test_no_consensus_in_round_one_moves_later_seats_and_the_vote_to_round_two():
    recorder = PlayerStepRecorder()
    await _emit(
        recorder,
        EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED,
        _pass(ConsensusMember.AXIOM, "ls /a"),
    )
    await _emit(
        recorder,
        EventType.AI_CONSENSUS_VOTING_CONSENSUS_NOT_REACHED,
        TribunalConsensusFailedPayload(request="r", vote_breakdown=_breakdown(None, 0.2)),
    )
    await _emit(
        recorder,
        EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED,
        _pass(ConsensusMember.AXIOM, "ls /a"),
    )
    await _emit(
        recorder,
        EventType.AI_CONSENSUS_VOTING_CONSENSUS_FAILED,
        TribunalConsensusFailedPayload(request="r", vote_breakdown=_breakdown(None, 0.2)),
    )

    assert [(step.player, step.round) for step in recorder.steps] == [
        ("axiom", 1),
        ("tribunal", 1),
        ("axiom", 2),
        ("tribunal", 2),
    ]
    assert all(
        step.vote is not None and not step.vote.reached and step.vote.winner is None
        for step in recorder.steps
        if step.player == "tribunal"
    )


@pytest.mark.asyncio
async def test_an_auditor_failure_is_a_failed_auditor_step():
    recorder = PlayerStepRecorder()
    await _emit(
        recorder,
        EventType.AI_CONSENSUS_SESSION_AUDITOR_FAILED,
        TribunalAuditorFailedPayload(
            request="r",
            reason=AuditorReason.NO_VALID_REVISION,
            error="Empty revision",
            candidate_command="ls /a",
            model_calls=[_call("auditor", "primary")],
        ),
    )

    (step,) = recorder.steps
    assert step.player == "auditor"
    assert not step.succeeded
    assert step.error_type == "no_valid_revision"
    assert step.error == "Empty revision"
    assert step.audit is None


def test_marshal_with_no_classification_is_a_failed_step():
    recorder = PlayerStepRecorder()
    recorder.observe_marshal_risk("rm -rf /tmp/x", None)

    (step,) = recorder.steps
    assert step.player == "marshal_command"
    assert not step.succeeded
    assert step.error_type == "NoRiskAnalysis"


def test_a_high_risk_classification_is_recorded_as_blocked():
    recorder = PlayerStepRecorder()
    recorder.observe_marshal_risk("rm -rf /", CommandRiskAnalysis(risk_level=RiskLevel.HIGH))

    (step,) = recorder.steps
    assert step.risk is not None
    assert step.risk.risk_level == "HIGH"
    assert step.risk.blocked


@pytest.mark.asyncio
async def test_a_failing_observer_never_stops_the_tribunal_event():
    class Exploding:
        def observe(self, event_type, payload):
            raise RuntimeError("observer bug")

        def observe_marshal_risk(self, command, analysis):
            raise RuntimeError("observer bug")

    emitter = TribunalEmitter(None, None, observer=Exploding())
    await emitter.emit(
        EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED, _pass(ConsensusMember.AXIOM, "ls")
    )
    emitter.observe_marshal_risk("ls", None)


def test_attaching_a_run_to_its_tool_call_links_and_renumbers_it():
    recorder = PlayerStepRecorder()
    recorder.observe_marshal_risk("ls", CommandRiskAnalysis(risk_level=RiskLevel.LOW))

    (step,) = attach_to_call(recorder.steps, "exec-9", first_sequence=4)

    assert step.parent_call_id == "exec-9"
    assert step.step_id == "exec-9:tribunal-1"
    assert step.sequence == 4


def _triage(**overrides) -> TriageResult:
    fields = {
        "complexity": TriageComplexityClassification.SIMPLE,
        "complexity_confidence": TriageConfidence.HIGH,
        "intent": TriageIntentClassification.INFORMATION,
        "intent_confidence": TriageConfidence.HIGH,
        "intent_summary": "wants the colour of the sky",
        "request_posture": TriageRequestPosture.NORMAL,
        "posture_confidence": TriageConfidence.HIGH,
        "model_call": _call("triage", "lite"),
    }
    fields.update(overrides)
    return TriageResult(**fields)


def test_triage_step_carries_the_whole_classification_and_its_tier():
    step = triage_step(_triage(), 1)

    assert step.player == "triage"
    assert step.model_role == "lite"
    assert step.model == "m1"
    assert step.succeeded
    assert step.triage == EvaluationTriageOutput(
        complexity="simple",
        complexity_confidence="high",
        intent="information",
        intent_confidence="high",
        request_posture="normal",
        posture_confidence="high",
        intent_summary="wants the colour of the sky",
        error_code=None,
    )


def test_a_triage_that_fell_back_is_a_failed_step_with_its_error_code():
    step = triage_step(
        _triage(
            complexity=TriageComplexityClassification.COMPLEX,
            error_code="PARSE_FAILURE",
            error_message="bad json",
            model_call=_call("triage", "lite", succeeded=False, error_type="ValueError"),
        ),
        1,
    )

    assert not step.succeeded
    assert step.error_type == "ValueError"
    assert step.error == "bad json"
    assert step.triage is not None
    assert step.triage.error_code == "PARSE_FAILURE"


def test_reasoning_text_is_bounded():
    step = reasoning_step(
        player="sage",
        model_role="primary",
        model="m1",
        text="x" * (PLAYER_TEXT_LIMIT + 50),
        succeeded=True,
        error=None,
        sequence=1,
    )

    assert step.text is not None
    assert len(step.text.text) == PLAYER_TEXT_LIMIT


def test_memory_text_joins_every_field_codex_wrote():
    memory = InvestigationMemory(
        case_id="c",
        investigation_id="i",
        user_id="u",
        status="Open",
        case_title="t",
        investigation_summary="a Linux host with a failing service",
        response_style="short",
    )

    assert memory_text(memory) == "a Linux host with a failing service\nshort"


def test_a_codex_step_without_text_is_failed():
    assert not memory_step(model_call=None, summary=None, sequence=1).succeeded
    assert memory_step(model_call=_call("codex", "lite"), summary="s", sequence=1).succeeded
    failed = memory_step(
        model_call=_call("codex", "lite", succeeded=False, error_type="Timeout"),
        summary="s",
        sequence=1,
    )
    assert not failed.succeeded
    assert failed.error_type == "Timeout"


def _tool_call(call_id: str, *, analysis: bool = False) -> EvaluationToolCallRecord:
    return EvaluationToolCallRecord(
        call_id=call_id,
        tool_name="run_commands_with_operator",
        error_analysis=EvaluationErrorAnalysisSummary(
            error_category="auto_fixable", root_cause="missing file"
        )
        if analysis
        else None,
    )


def test_error_analysis_steps_only_cover_calls_that_carried_one():
    steps = error_analysis_steps([_tool_call("a"), _tool_call("b", analysis=True)], 3)

    assert len(steps) == 1
    assert steps[0].player == "marshal_error"
    assert steps[0].parent_call_id == "b"
    assert steps[0].error_analysis is not None
    assert steps[0].error_analysis.root_cause == "missing file"


def test_the_chain_runs_triage_then_each_call_then_the_answer_then_codex():
    first = PlayerStepRecorder()
    first.observe_marshal_risk("ls /a", CommandRiskAnalysis(risk_level=RiskLevel.LOW))
    second = PlayerStepRecorder()
    second.observe_marshal_risk("cat /b", CommandRiskAnalysis(risk_level=RiskLevel.LOW))
    tribunal = attach_to_call(first.steps, "call-1", 1) + attach_to_call(second.steps, "call-2", 2)

    chain = assemble_player_steps(
        triage=_triage(),
        tribunal_steps=tribunal,
        tool_calls=[_tool_call("call-1"), _tool_call("call-2", analysis=True)],
        reasoning=reasoning_step(
            player="dash",
            model_role="assistant",
            model="m1",
            text="done",
            succeeded=True,
            error=None,
            sequence=1,
        ),
        codex=memory_step(model_call=_call("codex", "lite"), summary="s", sequence=1),
    )

    assert [(step.player, step.parent_call_id) for step in chain] == [
        ("triage", None),
        ("marshal_command", "call-1"),
        ("marshal_command", "call-2"),
        ("marshal_error", "call-2"),
        ("dash", None),
        ("codex", None),
    ]
    assert [step.sequence for step in chain] == [1, 2, 3, 4, 5, 6]
    assert len({step.step_id for step in chain}) == len(chain)


def test_a_tribunal_step_whose_call_was_not_recorded_is_kept_not_dropped():
    recorder = PlayerStepRecorder()
    recorder.observe_marshal_risk("ls", CommandRiskAnalysis(risk_level=RiskLevel.LOW))

    chain = assemble_player_steps(
        triage=None,
        tribunal_steps=attach_to_call(recorder.steps, "ghost", 1),
        tool_calls=[],
        reasoning=None,
        codex=None,
    )

    assert [step.parent_call_id for step in chain] == ["ghost"]


PLAYER_STEPS_VECTOR_PATH = (
    Path(__file__).resolve().parents[5] / "protocol" / "vectors" / "eval" / "player_steps.json"
)


def golden_chain() -> list[EvaluationPlayerStep]:
    """The chain of one host-command turn, built from the objects g8ee itself produces.

    ``protocol/vectors/eval/player_steps.json`` is this chain's wire form; the Go
    grader decodes the same file, so the two sides cannot drift apart silently.
    """
    recorder = PlayerStepRecorder()
    candidates = {
        ConsensusMember.AXIOM: "ls /srv/ws/logs",
        ConsensusMember.CONCORD: "ls /srv/ws/logs",
        ConsensusMember.VARIANCE: "ls -la /srv/ws/logs",
        ConsensusMember.PRAGMA: "ls /srv/ws/logs",
        ConsensusMember.NEMESIS: "ls /srv/ws",
    }
    for member, command in candidates.items():
        recorder.observe(EventType.AI_CONSENSUS_VOTING_PASS_COMPLETED, _pass(member, command))
    recorder.observe(
        EventType.AI_CONSENSUS_VOTING_CONSENSUS_REACHED,
        TribunalVotingCompletedPayload(
            vote_winner="ls /srv/ws/logs",
            vote_score=0.6,
            num_candidates=5,
            request="list the log files",
            vote_breakdown=VoteBreakdown(
                candidates_by_member={m.value: c for m, c in candidates.items()},
                candidates_by_command={
                    "ls /srv/ws/logs": ["axiom", "concord", "pragma"],
                    "ls -la /srv/ws/logs": ["variance"],
                    "ls /srv/ws": ["nemesis"],
                },
                winner="ls /srv/ws/logs",
                winner_supporters=["axiom", "concord", "pragma"],
                consensus_strength=0.6,
            ),
        ),
    )
    recorder.observe_marshal_risk(
        "ls /srv/ws/logs",
        CommandRiskAnalysis(risk_level=RiskLevel.LOW, model_call=_call("marshal_command", "lite")),
    )
    recorder.observe(
        EventType.AI_CONSENSUS_VOTING_AUDIT_COMPLETED,
        TribunalAuditorCompletedPayload(
            passed=True, reason=AuditorReason.OK, model_calls=[_call("auditor", "primary")]
        ),
    )
    return assemble_player_steps(
        triage=_triage(
            complexity=TriageComplexityClassification.COMPLEX,
            intent=TriageIntentClassification.ACTION,
            intent_summary="list the log files in the workspace",
        ),
        tribunal_steps=attach_to_call(recorder.steps, "exec-1", 1),
        tool_calls=[_tool_call("exec-1")],
        reasoning=reasoning_step(
            player="sage",
            model_role="primary",
            model="m1",
            text="The logs directory holds app.log and auth.log.",
            succeeded=True,
            error=None,
            sequence=1,
        ),
        codex=memory_step(
            model_call=_call("codex", "lite"),
            summary="Listed a directory of log files on a Linux host.",
            sequence=1,
        ),
    )


def test_the_chain_matches_the_protocol_vector_the_go_grader_decodes():
    vector = json.loads(PLAYER_STEPS_VECTOR_PATH.read_text())

    assert vector["message_type"] == "EvaluationPlayerSteps"
    assert [step.model_dump(mode="json") for step in golden_chain()] == vector["player_steps"]


def test_a_player_step_carries_at_most_one_output():
    with pytest.raises(ValueError, match="at most one output"):
        EvaluationPlayerStep(
            step_id="x",
            sequence=1,
            player="sage",
            text=EvaluationTextOutput(text="a"),
            triage=EvaluationTriageOutput(
                complexity="simple",
                complexity_confidence="high",
                intent="information",
                intent_confidence="high",
                request_posture="normal",
                posture_confidence="high",
            ),
        )
