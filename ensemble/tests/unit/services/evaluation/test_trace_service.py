# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest

from app.constants.bootstrap import BootstrapSettings, configure_bootstrap
from app.errors import ValidationError
from app.models.evaluation_trace import (
    EvaluationAssignmentTrace,
    EvaluationPlayerStep,
    EvaluationProviderToolRejection,
    EvaluationSeedApplication,
    EvaluationTextOutput,
    EvaluationToolCallRecord,
    EvaluationTriageOutput,
    ToolGate,
)
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry
from app.services.evaluation.trace_service import (
    EvaluationTraceService,
    compute_trace_digest,
    validated_trace_ids,
)
from g8e.models.internal_api import (
    EvaluationInferenceContext,
    EvaluationInvestigationSeed,
    EvaluationSeedTurn,
    InferenceModelVariant,
)


@pytest.fixture
def trace_service(tmp_path):
    configure_bootstrap(BootstrapSettings(runtime_dir=str(tmp_path)))
    return EvaluationTraceService()


def _evaluation_context() -> EvaluationInferenceContext:
    return EvaluationInferenceContext(
        campaign_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        evaluation_attempt_id="attempt-1",
        scenario_id="scenario-1",
        model_registry_digest="d" * 64,
        model_registry=[InferenceModelVariant(model="model-a", digest="a" * 64)],
        target_operator_session_id="session-1",
    )


def _context() -> G8eHttpContext:
    return G8eHttpContext(user_id="user-1", evaluation_context=_evaluation_context())


def test_trace_begin_persists_running_state_before_triage(trace_service):
    context = _context()
    trace = trace_service.begin(context)

    assert trace.status == "running"
    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.status == "running"
    assert loaded.triage_model_call is None


def test_trace_persist_finalize_and_load(trace_service):
    context = _context()
    triage_call = ModelCallTelemetry(
        agent_role="triage",
        model_role="lite",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
    )
    trace_service.begin(context, triage_model_call=triage_call)

    agent_call = ModelCallTelemetry(
        agent_role="sage",
        model_role="primary",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=3.0,
        monotonic_end=4.0,
        provider_attempt_id="attempt-1",
    )
    finalized = trace_service.finalize(
        context,
        model_calls=[agent_call],
        triage_model_call=triage_call,
        finish_reason="stop",
        status="completed",
    )

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.status == "completed"
    assert loaded.finish_reason == "stop"
    assert len(loaded.model_calls) == 1
    assert loaded.designated_role_output is None
    assert loaded.trace_digest == finalized.trace_digest
    assert compute_trace_digest(
        loaded.model_copy(update={"trace_digest": ""})
    ) == loaded.trace_digest


def test_trace_finalize_persists_the_chain_and_binds_it_into_the_digest(trace_service):
    context = _context()
    trace_service.begin(context)
    steps = [
        EvaluationPlayerStep(
            step_id="triage",
            sequence=1,
            player="triage",
            model_role="lite",
            model="model-a",
            triage=EvaluationTriageOutput(
                complexity="simple",
                complexity_confidence="high",
                intent="information",
                intent_confidence="high",
                request_posture="normal",
                posture_confidence="high",
            ),
        ),
        EvaluationPlayerStep(
            step_id="dash",
            sequence=2,
            player="dash",
            model_role="assistant",
            model="model-a",
            text=EvaluationTextOutput(text="Blue"),
        ),
    ]

    finalized = trace_service.finalize(
        context,
        model_calls=[],
        player_steps=steps,
        finish_reason="stop",
        status="completed",
    )

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.schema_version == "7"
    assert [step.player for step in loaded.player_steps] == ["triage", "dash"]
    assert loaded.player_steps[1].text is not None
    assert loaded.player_steps[1].text.text == "Blue"
    assert loaded.trace_digest == finalized.trace_digest

    tampered = loaded.model_copy(
        update={"player_steps": [loaded.player_steps[0], loaded.player_steps[1].model_copy(
            update={"text": EvaluationTextOutput(text="Red")}
        )]}
    )
    assert compute_trace_digest(tampered) != loaded.trace_digest


def test_trace_finalize_persists_designated_role_output(trace_service):
    context = _context()
    trace_service.begin(context)
    finalized = trace_service.finalize(
        context,
        model_calls=[],
        designated_role_output="READY",
        finish_reason="stop",
        status="completed",
    )
    assert finalized.designated_role_output == "READY"
    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.designated_role_output == "READY"


def test_trace_finalize_records_the_proof_that_the_opportunity_was_real(trace_service):
    """Per-call tools_declared (as sent to the provider) and tool_gate make 'tool offered
    and ignored' distinguishable from 'tool never offered' for every reader of the trace."""
    context = _context()
    trace_service.begin(context)
    agent_call = ModelCallTelemetry(
        agent_role="sage",
        model_role="primary",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=3.0,
        monotonic_end=4.0,
        tools_declared=["recursive_grep_search", "file_read_on_operator"],
    )

    finalized = trace_service.finalize(
        context,
        model_calls=[agent_call],
        tool_gate=ToolGate.BYPASSED_FOR_EVAL,
        finish_reason="stop",
        status="completed",
    )

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.trace_digest == finalized.trace_digest
    assert loaded.model_calls[0].tools_declared == ["recursive_grep_search", "file_read_on_operator"]
    assert loaded.tool_gate is ToolGate.BYPASSED_FOR_EVAL
    assert loaded.provider_tool_rejection is None


def test_trace_finalize_records_a_provider_tool_declaration_rejection(trace_service):
    context = _context()
    trace_service.begin(context)

    trace_service.finalize(
        context,
        model_calls=[],
        tool_gate=ToolGate.BYPASSED_FOR_EVAL,
        provider_tool_rejection=EvaluationProviderToolRejection(
            model="qwen3.5:4b",
            reason="Provider rejected the tool declaration: inference: requested capability unsupported",
        ),
        finish_reason="error",
        status="failed",
        error="Provider rejected the tool declaration",
    )

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.status == "failed"
    assert loaded.provider_tool_rejection is not None
    assert loaded.provider_tool_rejection.model == "qwen3.5:4b"
    assert "requested capability unsupported" in loaded.provider_tool_rejection.reason


def test_trace_distinguishes_no_tools_declared_from_not_reported(trace_service):
    """Unknown (None) must stay distinguishable from 'no tools were declared' ([])."""
    context = _context()
    trace_service.begin(context)

    def _call(role: str, tools: list[str] | None) -> ModelCallTelemetry:
        return ModelCallTelemetry(
            agent_role=role,
            provider="G8EProvider",
            model="model-a",
            monotonic_start=1.0,
            monotonic_end=2.0,
            tools_declared=tools,
        )

    trace_service.finalize(
        context,
        model_calls=[_call("codex", []), _call("sage", None)],
        finish_reason="stop",
        status="completed",
    )

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert [call.tools_declared for call in loaded.model_calls] == [[], None]
    assert loaded.tool_gate is None


def test_trace_persists_per_call_trajectory_and_guidance(trace_service):
    context = _context()
    trace_service.begin(context)

    trace_service.finalize(
        context,
        model_calls=[],
        tool_calls=[
            EvaluationToolCallRecord(
                call_id="exec-1",
                tool_name="recursive_grep_search",
                arguments_json='{"pattern":"AUTH_FAILURE"}',
                success=False,
                error_type="validation.error",
                loop_turn=1,
                error="path Field required",
                suggestion="Provide path",
            ),
            EvaluationToolCallRecord(
                call_id="exec-2",
                tool_name="recursive_grep_search",
                arguments_json='{"path":"/w","pattern":"AUTH_FAILURE"}',
                success=True,
                loop_turn=2,
            ),
        ],
        finish_reason="stop",
        status="completed",
    )

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert [(call.loop_turn, call.success) for call in loaded.tool_calls] == [(1, False), (2, True)]
    assert loaded.tool_calls[0].error == "path Field required"
    assert loaded.tool_calls[0].suggestion == "Provide path"
    assert loaded.tool_calls[1].error is None


def test_trace_schema_version_covers_the_seed_and_divergence_fields(trace_service):
    context = _context()

    assert trace_service.begin(context).schema_version == "7"


def test_trace_records_seed_application_and_the_eval_only_divergences(trace_service):
    context = _context()
    trace_service.begin(context)

    trace_service.finalize(
        context,
        model_calls=[],
        tool_turn_limit_reached=True,
        user_memories_suppressed=True,
        seed_application=EvaluationSeedApplication(turns=2, history_events=1, case_memory=True),
        finish_reason="tool_turn_limit",
        status="completed",
    )

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.tool_turn_limit_reached is True
    assert loaded.user_memories_suppressed is True
    assert loaded.seed_application == EvaluationSeedApplication(
        turns=2, history_events=1, case_memory=True
    )


def test_trace_defaults_record_no_divergence_when_none_occurred(trace_service):
    context = _context()

    trace_service.finalize(context, model_calls=[], finish_reason="stop", status="completed")

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.tool_turn_limit_reached is False
    assert loaded.user_memories_suppressed is False
    assert loaded.seed_application is None


def test_trace_digest_binds_seed_application_and_divergences():
    base = EvaluationAssignmentTrace(
        evaluation_context=_evaluation_context(), chat_execution_id="exec-1", status="completed"
    )
    seeded = base.model_copy(
        update={"seed_application": EvaluationSeedApplication(turns=1, history_events=0)}
    )
    suppressed = base.model_copy(update={"user_memories_suppressed": True})
    limited = base.model_copy(update={"tool_turn_limit_reached": True})

    digests = {compute_trace_digest(t) for t in (base, seeded, suppressed, limited)}
    assert len(digests) == 4


def test_finalize_crashed_closes_a_running_trace_as_failed_and_keeps_triage(trace_service):
    context = _context()
    triage_call = ModelCallTelemetry(
        agent_role="triage",
        model_role="lite",
        provider="G8EProvider",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
    )
    trace_service.begin(context, triage_model_call=triage_call)

    trace = trace_service.finalize_crashed(
        context,
        error="pipeline exploded",
        tool_gate=ToolGate.BYPASSED_FOR_EVAL,
        seed_application=EvaluationSeedApplication(turns=1),
    )

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.trace_digest == trace.trace_digest
    assert loaded.status == "failed"
    assert loaded.error == "pipeline exploded"
    assert loaded.finish_reason == "error"
    assert loaded.completed_at is not None
    assert loaded.triage_model_call == triage_call
    assert loaded.tool_gate is ToolGate.BYPASSED_FOR_EVAL
    assert loaded.seed_application == EvaluationSeedApplication(turns=1)


def test_finalize_crashed_without_a_running_trace_still_writes_a_failed_one(trace_service):
    context = _context()

    trace_service.finalize_crashed(context, error="crashed before begin")

    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.status == "failed"
    assert loaded.triage_model_call is None


def test_finalize_crashed_never_overwrites_a_terminal_trace(trace_service):
    context = _context()
    trace_service.begin(context)
    completed = trace_service.finalize(
        context,
        model_calls=[],
        designated_role_output="READY",
        finish_reason="stop",
        status="completed",
    )

    returned = trace_service.finalize_crashed(context, error="late crash")

    assert returned.status == "completed"
    loaded = trace_service.load("assignment-1", "attempt-1")
    assert loaded.trace_digest == completed.trace_digest
    assert loaded.designated_role_output == "READY"
    assert loaded.error is None


def test_finalize_crashed_requires_an_evaluation_context(trace_service):
    with pytest.raises(ValidationError, match="evaluation_context is required"):
        trace_service.finalize_crashed(G8eHttpContext(user_id="user-1"), error="x")


def test_trace_echoes_the_seed_whole_including_model_defaults(trace_service):
    """The echoed seed carries model defaults the harness never sent
    (``case_description`` and empty lists; unset optionals are omitted). The Go
    validator compares the seed after decoding it into its own typed form, so
    these defaults must not make an honest echo look different; this pins the
    shape."""
    seed = EvaluationInvestigationSeed(
        case_title="Checkout payment timeouts",
        turns=[EvaluationSeedTurn(sender="user", content="Checkout is failing.")],
    )
    context = G8eHttpContext(
        user_id="user-1",
        evaluation_context=_evaluation_context().model_copy(update={"seed": seed}),
    )

    trace_service.begin(context)

    echoed = trace_service.load("assignment-1", "attempt-1").model_dump(mode="json")[
        "evaluation_context"
    ]["seed"]
    assert echoed == {
        "case_title": "Checkout payment timeouts",
        "case_description": "",
        "turns": [{"sender": "user", "content": "Checkout is failing."}],
        "history_events": [],
    }


def test_trace_digest_binds_declared_tools_and_gate():
    def _call(tools: list[str]) -> ModelCallTelemetry:
        return ModelCallTelemetry(
            agent_role="sage",
            provider="G8EProvider",
            model="model-a",
            monotonic_start=1.0,
            monotonic_end=2.0,
            tools_declared=tools,
        )

    base = EvaluationAssignmentTrace(
        evaluation_context=_evaluation_context(),
        chat_execution_id="exec-1",
        status="completed",
        model_calls=[_call([])],
    )
    declared = base.model_copy(update={"model_calls": [_call(["recursive_grep_search"])]})
    gated = base.model_copy(update={"tool_gate": ToolGate.BYPASSED_FOR_EVAL})

    digests = {compute_trace_digest(t) for t in (base, declared, gated)}
    assert len(digests) == 3


def test_trace_load_rejects_path_traversal(trace_service):
    with pytest.raises(ValidationError, match="invalid assignment_id"):
        trace_service.load("../etc", "attempt-1")
    with pytest.raises(ValidationError, match="invalid evaluation_attempt_id"):
        trace_service.load("assignment-1", "../secret")


def test_validated_trace_ids_rejects_unsafe_segments():
    with pytest.raises(ValidationError, match="invalid assignment_id"):
        validated_trace_ids("../etc", "attempt-1")
    with pytest.raises(ValidationError, match="invalid evaluation_attempt_id"):
        validated_trace_ids("assignment-1", "../secret")


def test_validated_trace_ids_accepts_safe_segments():
    assert validated_trace_ids("assignment-1", "attempt-1") == ("assignment-1", "attempt-1")


def test_trace_digest_changes_when_model_calls_change():
    _context()
    base = EvaluationAssignmentTrace(
        evaluation_context=_evaluation_context(),
        chat_execution_id="exec-1",
        status="running",
    )
    with_call = base.model_copy(
        update={
            "model_calls": [
                ModelCallTelemetry(
                    agent_role="sage",
                    provider="G8EProvider",
                    model="model-a",
                    monotonic_start=1.0,
                    monotonic_end=2.0,
                )
            ]
        }
    )
    assert compute_trace_digest(base) != compute_trace_digest(with_call)
