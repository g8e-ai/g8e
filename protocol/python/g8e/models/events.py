# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from typing import Any

from pydantic import Field

from .base import G8eBaseModel, UTCDatetime
from g8e.enums import EventType

class _SSEEventBody(G8eBaseModel):
    type: EventType
    data: dict[str, Any]


class SessionEventWire(G8eBaseModel):
    web_session_id: str | None = None
    cli_session_id: str | None = None
    user_id: str
    event: _SSEEventBody

    @classmethod
    def from_session_event(
        cls,
        event_type: str,
        data: dict[str, Any],
        *,
        web_session_id: str | None = None,
        cli_session_id: str | None = None,
        user_id: str | None = None,
    ) -> "SessionEventWire":
        return cls(
            web_session_id=web_session_id,
            cli_session_id=cli_session_id,
            user_id=user_id or "",
            event=_SSEEventBody(type=event_type, data=data),
        )


class BackgroundEventWire(G8eBaseModel):
    user_id: str
    event: _SSEEventBody

    @classmethod
    def from_background_event(
        cls,
        event_type: str,
        data: dict[str, Any],
        *,
        user_id: str | None = None,
    ) -> "BackgroundEventWire":
        return cls(
            user_id=user_id or "",
            event=_SSEEventBody(type=event_type, data=data),
        )
# AI SSE event payloads (Wire shapes)
class AiProcessingStoppedPayload(G8eBaseModel):
    reason: str
    timestamp: UTCDatetime

class AIToolLifecyclePayload(G8eBaseModel):
    tool_name: str
    display_label: str | None = None
    display_icon: str | None = None
    display_detail: str | None = None
    category: str | None = None
    execution_id: str
    status: str
    query: str | None = None
    content: str | None = None
    results: list[dict[str, Any]] | None = None
    error: str | None = None
    port: str | None = None
    host: str | None = None
    is_open: bool | None = None
    timestamp: str | None = None

class ChatCitationsReadyPayload(G8eBaseModel):
    grounding_metadata: dict[str, Any]
    timestamp: str | None = None

class ChatErrorPayload(G8eBaseModel):
    error: str
    timestamp: str | None = None

class ChatProcessingStartedPayload(G8eBaseModel):
    agent_mode: str
    timestamp: str | None = None

class ChatResponseChunkPayload(G8eBaseModel):
    content: str
    timestamp: str | None = None

class ScrubbingTelemetry(G8eBaseModel):
    source: str
    enabled: bool
    was_modified: bool
    scrub_count: int
    scrub_types: list[str] = Field(default_factory=list)
    monotonic_start: float
    monotonic_end: float
    input_artifact_hash: str
    output_artifact_hash: str

class ChatResponseCompletePayload(G8eBaseModel):
    content: str
    finish_reason: str
    has_citations: bool
    grounding_metadata: dict[str, Any]
    token_usage: dict[str, Any]
    agent_mode: str
    model_calls: list[dict[str, Any]] = Field(default_factory=list)
    scrubbing_observations: list[ScrubbingTelemetry] = Field(default_factory=list)
    timestamp: str | None = None

class ChatRetryPayload(G8eBaseModel):
    attempt: int
    max_attempts: int
    timestamp: str | None = None

class ChatThinkingPayload(G8eBaseModel):
    thinking: str | None
    action_type: str
    timestamp: str | None = None

class ChatTurnCompletePayload(G8eBaseModel):
    turn: int
    timestamp: str | None = None

class TriageClarificationQuestionsPayload(G8eBaseModel):
    questions: list[str]
    complexity: str | None = None
    complexity_confidence: str | None = None
    intent: str | None = None
    intent_confidence: str | None = None
    intent_summary: str | None = None
    request_posture: str | None = None
    posture_confidence: str | None = None


# ---------------------------------------------------------------------------
# Observability dashboard event payloads (read-only frontend contract)
#
# These payloads back the four dashboard event families:
#   g8e.v1.app.agent.status.updated
#   g8e.v1.app.run.status.updated
#   g8e.v1.ai.eval.run.completed
#   g8e.v1.ai.eval.metric.recorded
#
# Producers emit these only after the corresponding state projection is
# persisted. They are distinct from governed-document events such as
# app.agent.activity.recorded. See protocol/models/observe_event_payloads.json
# for the canonical wire shapes.
# ---------------------------------------------------------------------------


class AgentStatusUpdatedPayload(G8eBaseModel):
    schema_version: str
    agent_id: str
    display_name: str
    role: str
    status: str
    run_id: str | None = None
    task_id: str | None = None
    model: str | None = None
    observed_at: UTCDatetime


class RunStatusUpdatedPayload(G8eBaseModel):
    schema_version: str
    run_id: str
    run_kind: str
    display_name: str
    status: str
    active_task_id: str | None = None
    completed_tasks: int
    total_tasks: int
    started_at: UTCDatetime | None = None
    ended_at: UTCDatetime | None = None
    observed_at: UTCDatetime


class EvalRunCompletedPayload(G8eBaseModel):
    schema_version: str
    run_id: str
    suite_id: str
    suite_version: str
    campaign_id: str
    arm_ids: list[str]
    terminal_attempts: int
    assigned_tasks: int
    receipt_count: int
    verification_status: str
    published_projection_sha256: str
    completed_at: UTCDatetime


class EvalMetricRecordedPayload(G8eBaseModel):
    schema_version: str
    run_id: str
    metric_id: str
    metric_version: str
    model_cohort_id: str
    arm_id: str
    value: float | None = None
    unit: str
    eligible: int
    denominator: int
    verification_status: str
    recorded_at: UTCDatetime


class ObservedMeasurement(G8eBaseModel):
    """Shared typed measurement shape for displayed resource and throughput values.

    Every displayed measurement uses a typed value with source and observation
    time. Resource and throughput cards remain unavailable until real
    instrumentation exists. Extended with scope, collector_version, and
    evidence_hash from the S6 typed observer.
    """

    schema_version: str
    metric_id: str
    value: float
    unit: str
    source_component: str
    observed_at: UTCDatetime
    window_seconds: float | None = None
    status: str
    scope: str | None = None
    collector_version: str | None = None
    evidence_hash: str | None = None


# ---------------------------------------------------------------------------
# Live campaign event payloads (g8e.v1.ai.eval.* family)
#
# These payloads carry source_sequence (monotonic ordering), event_id
# (duplicate suppression), and observed_at on every event. Producers persist
# the corresponding projection before emitting the SSE event. See
# protocol/models/observe_event_payloads.json for the canonical wire shapes.
# ---------------------------------------------------------------------------


class EvalCycleStartedPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    campaign_revision: str
    role_combination_id: str
    primary_variant_id: str
    assistant_variant_id: str | None = None
    lite_variant_id: str | None = None
    benchmark_population: int
    repetition_count: int
    started_at: UTCDatetime
    observed_at: UTCDatetime


class EvalCycleCompletedPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    campaign_revision: str
    role_combination_id: str
    verification_status: str
    terminal_attempts: int
    assigned_tasks: int
    completed_at: UTCDatetime
    observed_at: UTCDatetime


class EvalAssignmentStartedPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    assignment_id: str
    variant_id: str
    role: str
    task_id: str
    arm_id: str
    repetition: int
    started_at: UTCDatetime
    observed_at: UTCDatetime


class EvalAssignmentCompletedPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    assignment_id: str
    variant_id: str
    role: str
    task_id: str
    arm_id: str
    repetition: int
    terminal_status: str
    completed_at: UTCDatetime
    observed_at: UTCDatetime


class EvalModelRoleInvokedPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    assignment_id: str
    variant_id: str
    role: str
    served_model_tag: str
    backend_name: str
    quantization: str | None = None
    invoked_at: UTCDatetime
    observed_at: UTCDatetime


class EvalMetricAvailablePayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    assignment_id: str
    variant_id: str
    metric_id: str
    metric_version: str
    numerator: int
    denominator: int
    rate: float | None = None
    unit: str
    verification_status: str
    available_at: UTCDatetime
    observed_at: UTCDatetime


class EvalVerifierCompletedPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    verification_status: str
    verified_index_generation_hash: str
    layer_count: int
    failure_count: int
    completed_at: UTCDatetime
    observed_at: UTCDatetime


class EvalProofAvailablePayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    proof_root_sha256: str
    artifact_count: int
    available_at: UTCDatetime
    observed_at: UTCDatetime


class EvalPublicationCompletedPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    cycle_id: str
    campaign_id: str
    publication_schema_version: str
    published_projection_sha256: str
    completed_at: UTCDatetime
    observed_at: UTCDatetime


class EvalHeartbeatPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    source_id: str
    observed_at: UTCDatetime


class EvalStopRequestedPayload(G8eBaseModel):
    schema_version: str
    source_sequence: int
    event_id: str
    campaign_id: str
    stop_reason: str
    stop_scope: str
    requested_at: UTCDatetime
    observed_at: UTCDatetime
