# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

import pytest
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant
from pydantic import ValidationError as PydanticValidationError

from app.errors import ValidationError
from app.llm.model_call_attribution import (
    build_model_call_telemetry,
    governed_telemetry_fields,
    prepare_provider_call,
)
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import GovernedDispatchEvidence, ModelCallTelemetry


class _RecordingProvider:
    def __init__(self):
        self.last_context: G8eHttpContext | None = None
        self.last_retry_count = -1
        self.input_artifact_hash = ""
        self.model_boundary_privacy = None
        self.declared_tool_names: list[str] | None = ["stale_tool_from_prior_call"]
        self._governed_dispatch_evidence = GovernedDispatchEvidence(
            transaction_id="tx-1",
            result_digest="digest-1",
            receipt_status="EXECUTION_STATUS_COMPLETED",
            provider_attempt_id="attempt-1",
            requested_model="model-a",
            served_model="model-a",
            model_digest="a" * 64,
            normalized_request_hash="b" * 64,
            output_hash="c" * 64,
            campaign_id="campaign-1",
            run_id="run-1",
            assignment_id="assignment-1",
            evaluation_attempt_id="attempt-1",
            scenario_id="scenario-1",
            model_registry_digest="d" * 64,
        )

    @property
    def governed_dispatch_evidence(self) -> GovernedDispatchEvidence:
        return self._governed_dispatch_evidence

    def clear_input_artifact_hash(self) -> None:
        self.input_artifact_hash = ""

    def clear_declared_tools(self) -> None:
        self.declared_tool_names = None

    def set_g8e_context(self, context: G8eHttpContext | None) -> None:
        self.last_context = context

    def set_provider_retry_count(self, retry_count: int) -> None:
        self.last_retry_count = retry_count


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


def test_prepare_provider_call_binds_context_and_retry():
    provider = _RecordingProvider()
    context = G8eHttpContext(
        user_id="user-1",
        evaluation_context=_evaluation_context(),
    )

    prepare_provider_call(provider, g8e_context=context, retry_count=2)

    assert provider.last_context == context
    assert provider.last_retry_count == 2


def test_prepare_provider_call_discards_tool_declarations_from_the_prior_call():
    """A call that reports nothing must never inherit the previous call's tool set."""
    provider = _RecordingProvider()
    assert provider.declared_tool_names == ["stale_tool_from_prior_call"]

    prepare_provider_call(provider)

    assert provider.declared_tool_names is None


def _telemetry(provider):
    return build_model_call_telemetry(
        provider=provider,
        agent_role="sage",
        model_role="primary",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
        input_artifact_hash="e" * 64,
    )


def test_build_model_call_telemetry_records_the_tools_sent_on_that_call():
    provider = _RecordingProvider()
    provider.declared_tool_names = ["recursive_grep_search", "file_read_on_operator"]

    assert _telemetry(provider).tools_declared == ["recursive_grep_search", "file_read_on_operator"]


def test_build_model_call_telemetry_distinguishes_no_tools_from_not_reported():
    provider = _RecordingProvider()

    provider.declared_tool_names = []
    assert _telemetry(provider).tools_declared == []

    provider.declared_tool_names = None
    assert _telemetry(provider).tools_declared is None


def test_build_model_call_telemetry_ignores_a_provider_without_the_capture():
    class _NoCapture:
        input_artifact_hash = ""
        model_boundary_privacy = None
        governed_dispatch_evidence = None

    assert _telemetry(_NoCapture()).tools_declared is None


@pytest.mark.parametrize(
    ("agent_role", "classification"),
    [("sage", "scored_chain"), ("codex", "post_turn"), ("judge", "grader")],
)
def test_build_model_call_telemetry_states_the_chain_classification(agent_role, classification):
    telemetry = build_model_call_telemetry(
        provider=_RecordingProvider(),
        agent_role=agent_role,
        model_role="lite",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
        input_artifact_hash="e" * 64,
    )

    assert telemetry.classification == classification


def test_build_model_call_telemetry_rejects_an_unregistered_agent_role():
    with pytest.raises(ValidationError):
        build_model_call_telemetry(
            provider=_RecordingProvider(),
            agent_role="unknown",
            model_role="lite",
            model="model-a",
            monotonic_start=1.0,
            monotonic_end=2.0,
            input_artifact_hash="e" * 64,
        )


def test_build_model_call_telemetry_includes_governed_fields():
    provider = _RecordingProvider()
    telemetry = build_model_call_telemetry(
        provider=provider,
        agent_role="triage",
        model_role="lite",
        model="model-a",
        monotonic_start=1.0,
        monotonic_end=2.0,
        input_artifact_hash="e" * 64,
    )

    fields = governed_telemetry_fields(provider)
    assert telemetry.provider_attempt_id == fields["provider_attempt_id"]
    assert telemetry.assignment_id == "assignment-1"
    assert telemetry.model_registry_digest == "d" * 64


def test_model_call_telemetry_requires_classification():
    with pytest.raises(PydanticValidationError, match="classification"):
        ModelCallTelemetry(
            agent_role="sage",
            provider="fake",
            model="model-a",
            monotonic_start=1.0,
            monotonic_end=2.0,
        )
