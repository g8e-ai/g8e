# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from app.llm.model_call_attribution import (
    build_model_call_telemetry,
    governed_telemetry_fields,
    prepare_provider_call,
)
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import GovernedDispatchEvidence
from g8e.models.internal_api import EvaluationInferenceContext, InferenceModelVariant


class _RecordingProvider:
    def __init__(self):
        self.last_context: G8eHttpContext | None = None
        self.last_retry_count = -1
        self.input_artifact_hash = ""
        self.model_boundary_privacy = None
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
