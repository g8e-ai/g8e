# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from typing import Any

from g8e_evals.schema import (
    EvidenceIndex,
    EvidenceMediaType,
    PrivacyClassification,
    StageKind,
    StageObservation,
    UsageReconciliation,
)


@dataclass(frozen=True)
class EvidenceArtifact:
    index: EvidenceIndex
    content: str


@dataclass(frozen=True)
class NormalizedAttemptEvidence:
    stages: list[StageObservation]
    usage: UsageReconciliation
    raw_evidence: EvidenceArtifact | None


def _int(value: Any) -> int:
    return value if isinstance(value, int) and not isinstance(value, bool) else 0


def _event_data(payload: dict[str, Any]) -> dict[str, Any]:
    event = payload.get("event")
    if not isinstance(event, dict):
        return {}
    data = event.get("data")
    return data if isinstance(data, dict) else {}


def _canonical_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def _raw_artifact(evidence: Any, run_id: str, attempt_id: str) -> EvidenceArtifact:
    wire = evidence.model_dump(mode="json")
    content = _canonical_json(wire)
    digest = hashlib.sha256(content.encode()).hexdigest()
    artifact_id = f"{attempt_id}:agent-trail"
    return EvidenceArtifact(
        index=EvidenceIndex(
            artifact_id=artifact_id,
            run_id=run_id,
            attempt_id=attempt_id,
            media_type=EvidenceMediaType.APPLICATION_JSON,
            schema_ref="g8e_evals.ChatEvaluationReceipt",
            byte_length=len(content.encode()),
            sha256=digest,
            producer_identity="g8e-evals",
            privacy_classification=PrivacyClassification.RESTRICTED,
            storage_location=f"evidence/{digest}.json",
        ),
        content=content,
    )


def _direct_stage(wire: dict[str, Any], run_id: str, attempt_id: str) -> StageObservation:
    return StageObservation(
        stage_id=f"{attempt_id}:direct:1",
        attempt_id=attempt_id,
        run_id=run_id,
        kind=StageKind.MODEL_INFERENCE,
        agent_role="direct",
        provider=str(wire.get("provider") or ""),
        model=str(wire.get("model") or ""),
        monotonic_start=float(wire.get("monotonic_start") or 0.0),
        monotonic_end=float(wire.get("monotonic_end") or 0.0),
        clock_domain="g8e-evals-process",
        timing_source="provider_call_monotonic",
        input_tokens=_int(wire.get("prompt_token_count")),
        output_tokens=_int(wire.get("candidates_token_count")),
        thinking_tokens=_int(wire.get("thinking_token_count")),
        finish_reason=wire.get("finish_reason") if isinstance(wire.get("finish_reason"), str) else None,
    )


def _chat_stages(evidence: Any, run_id: str, attempt_id: str) -> tuple[list[StageObservation], int]:
    stages: list[StageObservation] = []
    expected_call_count = 0
    for index, event in enumerate(evidence.agent_trail, start=1):
        data = _event_data(event.payload)
        event_type = event.event_type
        kind: StageKind | None = None
        role = ""
        deterministic_kinds = {
            "g8e.v1.operator.command.execution": StageKind.L5_EXECUTION,
            "g8e.v1.operator.notary.approval.requested": StageKind.L3_CEREMONY,
            "g8e.v1.operator.notary.transaction.expired": StageKind.L3_CEREMONY,
            "g8e.v1.operator.receipt.recorded": StageKind.RECEIPT_PERSISTENCE,
            "g8e.v1.operator.reputation.commitment.created": StageKind.COMMITMENT_APPEND,
            "g8e.v1.operator.reputation.commitment.failed": StageKind.COMMITMENT_APPEND,
            "g8e.v1.operator.reputation.commitment.verified": StageKind.COMMITMENT_APPEND,
        }
        if event_type in deterministic_kinds:
            kind = deterministic_kinds[event_type]
        elif event_type == "g8e.v1.ai.consensus.voting.pass.completed":
            kind = StageKind.TRIBUNAL_GENERATION
            role = str(data.get("member") or "tribunal_member")
        elif event_type.endswith("consensus.voting.audit.completed"):
            kind = StageKind.TRIBUNAL_AUDITOR
            role = "auditor"
        elif event_type == "g8e.v1.ai.llm.chat.iteration.text.completed":
            kind = StageKind.MODEL_INFERENCE
            role = str(data.get("agent_mode") or "primary")
            expected_call_count = max(expected_call_count, _int(data.get("model_call_count")))
        if kind is None:
            continue
        usage_value = data.get("token_usage")
        usage: dict[str, Any] = usage_value if isinstance(usage_value, dict) else data
        stages.append(
            StageObservation(
                stage_id=f"{attempt_id}:sse:{event.id or index}",
                attempt_id=attempt_id,
                run_id=run_id,
                kind=kind,
                agent_role=role,
                provider=str(data.get("provider") or ""),
                model=str(data.get("model") or ""),
                monotonic_end=event.monotonic_received_at,
                clock_domain="g8e-evals-process",
                timing_source="sse_receive_monotonic",
                cross_process_timing=True,
                input_tokens=_int(usage.get("input_tokens")),
                output_tokens=_int(usage.get("output_tokens")),
                thinking_tokens=_int(usage.get("thinking_tokens")),
                cache_tokens=_int(usage.get("cache_tokens")),
                usage_estimated=bool(usage.get("estimated", False)),
                retry_count=_int(data.get("retry_count")),
                finish_reason=data.get("finish_reason") if isinstance(data.get("finish_reason"), str) else None,
                decision=data.get("decision") if isinstance(data.get("decision"), str) else None,
            )
        )
    model_call_count = sum(
        stage.kind in {StageKind.MODEL_INFERENCE, StageKind.TRIBUNAL_GENERATION, StageKind.TRIBUNAL_AUDITOR, StageKind.GRADING}
        for stage in stages
    )
    return stages, expected_call_count or model_call_count


def normalize_attempt_evidence(
    evidence: Any,
    run_id: str,
    attempt_id: str,
) -> NormalizedAttemptEvidence:
    wire = evidence.model_dump()
    if wire.get("binding") == "direct_provider":
        stages = [_direct_stage(wire, run_id, attempt_id)]
        raw_evidence = None
        expected_call_count = 1
    else:
        stages, expected_call_count = _chat_stages(evidence, run_id, attempt_id)
        raw_evidence = _raw_artifact(evidence, run_id, attempt_id)
        stages = [
            stage.model_copy(update={"output_artifact_hash": raw_evidence.index.sha256})
            for stage in stages
        ]

    model_stages = [
        stage
        for stage in stages
        if stage.kind in {StageKind.MODEL_INFERENCE, StageKind.TRIBUNAL_GENERATION, StageKind.TRIBUNAL_AUDITOR, StageKind.GRADING}
    ]
    observed_input = sum(stage.input_tokens or 0 for stage in model_stages)
    observed_output = sum(stage.output_tokens or 0 for stage in model_stages)
    observed_thinking = sum(stage.thinking_tokens or 0 for stage in model_stages)
    usage = UsageReconciliation(
        reported_input_tokens=observed_input,
        reported_output_tokens=observed_output,
        reported_thinking_tokens=observed_thinking,
        observed_input_tokens=observed_input,
        observed_output_tokens=observed_output,
        observed_thinking_tokens=observed_thinking,
        observed_call_count=len(model_stages),
        expected_call_count=expected_call_count,
    )
    return NormalizedAttemptEvidence(stages=stages, usage=usage, raw_evidence=raw_evidence)
