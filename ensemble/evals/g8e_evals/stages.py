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

from app.models.model_telemetry import ModelCallTelemetry
from g8e.operator.v1.operator_pb2 import (
    ActionReceipt,
    DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND,
    DETERMINISTIC_STAGE_KIND_L1_DOCTRINE,
    DETERMINISTIC_STAGE_KIND_L3_NOTARY,
    DETERMINISTIC_STAGE_KIND_L4_VERIFICATION,
    DETERMINISTIC_STAGE_KIND_L5_EXECUTION,
    DETERMINISTIC_STAGE_KIND_PROTOCOL_L2,
    DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE,
    DETERMINISTIC_STAGE_OUTCOME_COMPLETED,
    DETERMINISTIC_STAGE_OUTCOME_FAILED,
    DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED,
    DETERMINISTIC_STAGE_OUTCOME_VERIFIED,
)
from g8e_evals.harness import ReceiptEvidence
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
    usage_reported = wire.get("usage_reported") is True
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
        input_tokens=_int(wire.get("prompt_token_count")) if usage_reported else None,
        output_tokens=_int(wire.get("candidates_token_count")) if usage_reported else None,
        thinking_tokens=_int(wire.get("thinking_token_count")) if usage_reported else None,
        cache_tokens=_int(wire.get("cache_token_count")) if usage_reported else None,
        usage_reported=usage_reported,
        finish_reason=wire.get("finish_reason") if isinstance(wire.get("finish_reason"), str) else None,
        input_artifact_hash=str(wire.get("input_artifact_hash") or "") or None,
        output_artifact_hash=str(wire.get("output_artifact_hash") or "") or None,
    )


def _chat_stages(
    evidence: Any, run_id: str, attempt_id: str
) -> tuple[list[StageObservation], int, tuple[int, int, int, int]]:
    stages: list[StageObservation] = []
    declared_primary_calls = 0
    reported_input = 0
    reported_output = 0
    reported_thinking = 0
    reported_cache = 0
    for index, event in enumerate(evidence.agent_trail, start=1):
        data = _event_data(event.payload)
        event_type = event.event_type
        scrubbing_observations = data.get("scrubbing_observations")
        if isinstance(scrubbing_observations, list):
            for scrub_index, observation in enumerate(scrubbing_observations, start=1):
                if not isinstance(observation, dict):
                    continue
                enabled = bool(observation.get("enabled", False))
                modified = bool(observation.get("was_modified", False))
                stages.append(StageObservation(
                    stage_id=f"{attempt_id}:sse:{event.id or index}:scrub:{scrub_index}",
                    attempt_id=attempt_id,
                    run_id=run_id,
                    kind=StageKind.SCRUBBING,
                    agent_role="sentinel",
                    monotonic_start=float(observation.get("monotonic_start") or 0.0),
                    monotonic_end=float(observation.get("monotonic_end") or 0.0),
                    clock_domain="g8ee-process",
                    timing_source="scrubber_monotonic",
                    decision="modified" if modified else "unchanged" if enabled else "disabled",
                    input_artifact_hash=str(observation.get("input_artifact_hash") or "") or None,
                    output_artifact_hash=str(observation.get("output_artifact_hash") or "") or None,
                    source=str(observation.get("source") or ""),
                    scrub_count=_int(observation.get("scrub_count")),
                    scrub_types=[value for value in observation.get("scrub_types", []) if isinstance(value, str)]
                    if isinstance(observation.get("scrub_types"), list)
                    else [],
                ))
        model_calls = data.get("model_calls")
        if event_type == "g8e.v1.ai.llm.chat.iteration.text.completed" and isinstance(model_calls, list) and model_calls:
            declared_primary_calls = max(declared_primary_calls, len(model_calls))
            aggregate_usage = data.get("token_usage")
            if isinstance(aggregate_usage, dict):
                reported_input += _int(aggregate_usage.get("input_tokens"))
                reported_output += _int(aggregate_usage.get("output_tokens"))
                reported_thinking += _int(aggregate_usage.get("thinking_tokens"))
                reported_cache += _int(aggregate_usage.get("cache_tokens"))
            for call_index, call in enumerate(model_calls, start=1):
                if not isinstance(call, dict):
                    continue
                usage_reported = call.get("usage_reported") is True
                stages.append(StageObservation(
                    stage_id=f"{attempt_id}:sse:{event.id or index}:call:{call_index}",
                    attempt_id=attempt_id,
                    run_id=run_id,
                    kind=StageKind.MODEL_INFERENCE,
                    agent_role=str(call.get("agent_role") or data.get("agent_mode") or "primary"),
                    provider=str(call.get("provider") or ""),
                    model=str(call.get("model") or ""),
                    monotonic_start=float(call.get("monotonic_start") or 0.0),
                    monotonic_end=float(call.get("monotonic_end") or 0.0),
                    clock_domain="g8ee-process",
                    timing_source="provider_call_monotonic",
                    input_tokens=_int(call.get("input_tokens")) if usage_reported else None,
                    output_tokens=_int(call.get("output_tokens")) if usage_reported else None,
                    thinking_tokens=_int(call.get("thinking_tokens")) if usage_reported else None,
                    cache_tokens=_int(call.get("cache_tokens")) if usage_reported else None,
                    usage_reported=usage_reported,
                    retry_count=_int(call.get("retry_count")),
                    finish_reason=call.get("finish_reason") if isinstance(call.get("finish_reason"), str) else None,
                    input_artifact_hash=str(call.get("input_artifact_hash") or "") or None,
                    output_artifact_hash=str(call.get("output_artifact_hash") or "") or None,
                ))
            continue
        model_call_event_kinds = {
            "g8e.v1.ai.consensus.voting.audit.completed": StageKind.TRIBUNAL_AUDITOR,
            "g8e.v1.ai.consensus.session.auditor.failed": StageKind.TRIBUNAL_AUDITOR,
            "g8e.v1.ai.consensus.session.completed": StageKind.GRADING,
            "g8e.v1.ai.consensus.session.warden.blocked": StageKind.GRADING,
            "g8e.v1.ai.agent.conflict.detected": StageKind.GRADING,
        }
        if event_type in model_call_event_kinds and isinstance(model_calls, list):
            for call_index, call in enumerate(model_calls, start=1):
                if not isinstance(call, dict):
                    continue
                usage_reported = call.get("usage_reported") is True
                input_tokens = _int(call.get("input_tokens"))
                output_tokens = _int(call.get("output_tokens"))
                thinking_tokens = _int(call.get("thinking_tokens"))
                cache_tokens = _int(call.get("cache_tokens"))
                reported_input += input_tokens
                reported_output += output_tokens
                reported_thinking += thinking_tokens
                reported_cache += cache_tokens
                stages.append(StageObservation(
                    stage_id=f"{attempt_id}:sse:{event.id or index}:call:{call_index}",
                    attempt_id=attempt_id,
                    run_id=run_id,
                    kind=model_call_event_kinds[event_type],
                    agent_role=str(call.get("agent_role") or "auditor"),
                    provider=str(call.get("provider") or ""),
                    model=str(call.get("model") or ""),
                    monotonic_start=float(call.get("monotonic_start") or 0.0),
                    monotonic_end=float(call.get("monotonic_end") or 0.0),
                    clock_domain="g8ee-process",
                    timing_source="provider_call_monotonic",
                    input_tokens=input_tokens if usage_reported else None,
                    output_tokens=output_tokens if usage_reported else None,
                    thinking_tokens=thinking_tokens if usage_reported else None,
                    cache_tokens=cache_tokens if usage_reported else None,
                    usage_reported=usage_reported,
                    retry_count=_int(call.get("retry_count")),
                    finish_reason=call.get("finish_reason") if isinstance(call.get("finish_reason"), str) else None,
                    input_artifact_hash=str(call.get("input_artifact_hash") or "") or None,
                    output_artifact_hash=str(call.get("output_artifact_hash") or "") or None,
                    decision="completed" if call.get("succeeded", True) else "failed",
                ))
            continue
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
            declared_primary_calls = max(declared_primary_calls, _int(data.get("model_call_count")))
        if kind is None:
            continue
        usage_value = data.get("token_usage")
        usage: dict[str, Any] = usage_value if isinstance(usage_value, dict) else data
        usage_reported = usage.get("usage_reported") is True
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
                input_tokens=_int(usage.get("input_tokens")) if usage_reported else None,
                output_tokens=_int(usage.get("output_tokens")) if usage_reported else None,
                thinking_tokens=_int(usage.get("thinking_tokens")) if usage_reported else None,
                cache_tokens=_int(usage.get("cache_tokens")) if usage_reported else None,
                usage_reported=usage_reported,
                usage_estimated=bool(usage.get("estimated", False)),
                retry_count=_int(data.get("retry_count")),
                finish_reason=data.get("finish_reason") if isinstance(data.get("finish_reason"), str) else None,
                decision=(
                    data.get("decision")
                    if isinstance(data.get("decision"), str)
                    else "completed" if data.get("succeeded", True) else "failed"
                ),
                input_artifact_hash=str(data.get("input_artifact_hash") or "") or None,
                output_artifact_hash=str(data.get("output_artifact_hash") or "") or None,
            )
        )
        if kind in {StageKind.MODEL_INFERENCE, StageKind.TRIBUNAL_GENERATION, StageKind.TRIBUNAL_AUDITOR, StageKind.GRADING}:
            reported_input += _int(usage.get("input_tokens"))
            reported_output += _int(usage.get("output_tokens"))
            reported_thinking += _int(usage.get("thinking_tokens"))
            reported_cache += _int(usage.get("cache_tokens"))
    model_call_count = sum(
        stage.kind in {StageKind.MODEL_INFERENCE, StageKind.TRIBUNAL_GENERATION, StageKind.TRIBUNAL_AUDITOR, StageKind.GRADING}
        for stage in stages
    )
    observed_primary_calls = sum(stage.kind == StageKind.MODEL_INFERENCE for stage in stages)
    expected_call_count = (declared_primary_calls or observed_primary_calls) + model_call_count - observed_primary_calls
    return stages, expected_call_count, (
        reported_input,
        reported_output,
        reported_thinking,
        reported_cache,
    )


def _link_stage_relationships(stages: list[StageObservation]) -> list[StageObservation]:
    stages_by_id: dict[str, StageObservation] = {}
    child_ids: dict[str, list[str]] = {}
    for stage in stages:
        if stage.stage_id in stages_by_id:
            raise ValueError(f"duplicate deterministic stage id: {stage.stage_id}")
        stages_by_id[stage.stage_id] = stage
        child_ids[stage.stage_id] = []
    for stage in stages:
        if not stage.parent_stage_id:
            continue
        if stage.parent_stage_id not in stages_by_id:
            raise ValueError(
                f"unknown parent deterministic stage: {stage.parent_stage_id}"
            )
        child_ids[stage.parent_stage_id].append(stage.stage_id)
    for stage in stages:
        visited = {stage.stage_id}
        parent_id = stage.parent_stage_id
        while parent_id:
            if parent_id in visited:
                raise ValueError(f"cyclic deterministic stage relationship: {parent_id}")
            visited.add(parent_id)
            parent_id = stages_by_id[parent_id].parent_stage_id
    return [
        stage.model_copy(update={"child_stage_ids": child_ids[stage.stage_id]})
        for stage in stages
    ]


def _receipt_stages(
    receipt: ActionReceipt,
    run_id: str,
    attempt_id: str,
    receipt_verified: bool,
) -> list[StageObservation]:
    kinds = {
        DETERMINISTIC_STAGE_KIND_L1_DOCTRINE: StageKind.DETERMINISTIC_DOCTRINE,
        DETERMINISTIC_STAGE_KIND_PROTOCOL_L2: StageKind.PROTOCOL_L2,
        DETERMINISTIC_STAGE_KIND_L3_NOTARY: StageKind.L3_CEREMONY,
        DETERMINISTIC_STAGE_KIND_L4_VERIFICATION: StageKind.L4_VERIFICATION,
        DETERMINISTIC_STAGE_KIND_RECEIPT_PERSISTENCE: StageKind.RECEIPT_PERSISTENCE,
        DETERMINISTIC_STAGE_KIND_COMMITMENT_APPEND: StageKind.COMMITMENT_APPEND,
        DETERMINISTIC_STAGE_KIND_L5_EXECUTION: StageKind.L5_EXECUTION,
    }
    outcomes = {
        DETERMINISTIC_STAGE_OUTCOME_VERIFIED: "verified",
        DETERMINISTIC_STAGE_OUTCOME_NOT_REQUIRED: "not_required",
        DETERMINISTIC_STAGE_OUTCOME_COMPLETED: "completed",
        DETERMINISTIC_STAGE_OUTCOME_FAILED: "failed",
    }
    stages: list[StageObservation] = []
    for index, observation in enumerate(receipt.deterministic_stage_evidence, start=1):
        kind = kinds.get(observation.kind)
        if kind is None:
            raise ValueError(f"unknown deterministic stage kind: {observation.kind}")
        outcome = outcomes.get(observation.outcome)
        if outcome is None:
            raise ValueError(f"unknown deterministic stage outcome: {observation.outcome}")
        stages.append(StageObservation(
            stage_id=observation.stage_id or f"{attempt_id}:receipt:{index}",
            attempt_id=attempt_id,
            run_id=run_id,
            kind=kind,
            monotonic_start=observation.monotonic_start_ns / 1_000_000_000,
            monotonic_end=observation.monotonic_end_ns / 1_000_000_000,
            clock_domain=observation.clock_domain,
            timing_source=observation.timing_source,
            decision=outcome,
            transaction_id=observation.transaction_id,
            transaction_hash=observation.transaction_hash,
            action_type=observation.action_type,
            operator_id=observation.operator_id,
            operator_session_id=observation.operator_session_id,
            requestor_user_id=observation.requestor_user_id,
            acting_app_id=observation.acting_app_id,
            case_id=observation.case_id,
            investigation_id=observation.investigation_id,
            task_id=observation.task_id,
            state_root_before=observation.state_root_before,
            state_root_after=observation.state_root_after,
            signer_key_id=observation.signer_key_id,
            receipt_signature_digest=observation.receipt_signature_digest,
            commitment_hash=observation.commitment_hash,
            prior_commitment_hash=observation.prior_commitment_hash,
            l2_signature_digest=observation.l2_signature_digest,
            l3_signature_digest=observation.l3_signature_digest,
            audit_record_id=observation.audit_record_id,
            doctrine_bundle_hash=observation.doctrine_bundle_hash,
            doctrine_bundle_version=observation.doctrine_bundle_version,
            parent_stage_id=observation.parent_stage_id or None,
        ))
    if receipt.HasField("final_persistence_attestation"):
        attestation = receipt.final_persistence_attestation
        stages.append(StageObservation(
            stage_id=f"{attempt_id}:receipt:{receipt.transaction_id}:persistence:final",
            attempt_id=attempt_id,
            run_id=run_id,
            kind=StageKind.RECEIPT_PERSISTENCE,
            timing_source="signed_persistence_attestation",
            decision="verified" if receipt_verified else "unverified",
            transaction_id=attestation.transaction_id,
            signer_key_id=attestation.signer_key_id,
            receipt_signature_digest=attestation.receipt_signature_digest,
            audit_record_id=attestation.audit_record_id,
            persisted_at_unix_ms=attestation.persisted_at_unix_ms,
            parent_stage_id=stages[-1].stage_id if stages else None,
        ))
    return _link_stage_relationships(stages)


def _grading_stages(
    model_calls: list[ModelCallTelemetry], run_id: str, attempt_id: str
) -> list[StageObservation]:
    return [
        StageObservation(
            stage_id=f"{attempt_id}:grading:{index}",
            attempt_id=attempt_id,
            run_id=run_id,
            kind=StageKind.GRADING,
            agent_role=call.agent_role,
            provider=call.provider,
            model=call.model,
            monotonic_start=call.monotonic_start,
            monotonic_end=call.monotonic_end,
            clock_domain="g8e-evals-process",
            timing_source="provider_call_monotonic",
            input_tokens=call.input_tokens if call.usage_reported else None,
            output_tokens=call.output_tokens if call.usage_reported else None,
            thinking_tokens=call.thinking_tokens if call.usage_reported else None,
            cache_tokens=call.cache_tokens if call.usage_reported else None,
            usage_reported=call.usage_reported,
            retry_count=call.retry_count,
            finish_reason=call.finish_reason,
            input_artifact_hash=call.input_artifact_hash or None,
            output_artifact_hash=call.output_artifact_hash or None,
            decision="completed" if call.succeeded else "failed",
        )
        for index, call in enumerate(model_calls, start=1)
    ]


def normalize_attempt_evidence(
    evidence: Any,
    run_id: str,
    attempt_id: str,
    receipts: list[ReceiptEvidence] | None = None,
    grading_model_calls: list[ModelCallTelemetry] | None = None,
) -> NormalizedAttemptEvidence:
    wire = evidence.model_dump()
    if wire.get("binding") == "direct_provider":
        stages = [_direct_stage(wire, run_id, attempt_id)]
        raw_evidence = None
        expected_call_count = 1
        reported_tokens = (
            _int(wire.get("prompt_token_count")),
            _int(wire.get("candidates_token_count")),
            _int(wire.get("thinking_token_count")),
            _int(wire.get("cache_token_count")),
        )
    else:
        stages, expected_call_count, reported_tokens = _chat_stages(evidence, run_id, attempt_id)
        raw_evidence = _raw_artifact(evidence, run_id, attempt_id)
        stages = [
            stage.model_copy(update={"output_artifact_hash": stage.output_artifact_hash or raw_evidence.index.sha256})
            for stage in stages
        ]
    for receipt in receipts or []:
        stages.extend(
            _receipt_stages(
                receipt.action_receipt,
                run_id,
                attempt_id,
                receipt.verified,
            )
        )
    grading_stages = _grading_stages(grading_model_calls or [], run_id, attempt_id)
    stages.extend(grading_stages)
    expected_call_count += len(grading_stages)
    reported_tokens = (
        reported_tokens[0] + sum(stage.input_tokens or 0 for stage in grading_stages),
        reported_tokens[1] + sum(stage.output_tokens or 0 for stage in grading_stages),
        reported_tokens[2] + sum(stage.thinking_tokens or 0 for stage in grading_stages),
        reported_tokens[3] + sum(stage.cache_tokens or 0 for stage in grading_stages),
    )

    model_stages = [
        stage
        for stage in stages
        if stage.kind in {StageKind.MODEL_INFERENCE, StageKind.TRIBUNAL_GENERATION, StageKind.TRIBUNAL_AUDITOR, StageKind.GRADING}
    ]
    observed_input = sum(stage.input_tokens or 0 for stage in model_stages)
    observed_output = sum(stage.output_tokens or 0 for stage in model_stages)
    observed_thinking = sum(stage.thinking_tokens or 0 for stage in model_stages)
    observed_cache = sum(stage.cache_tokens or 0 for stage in model_stages)
    usage = UsageReconciliation(
        reported_input_tokens=reported_tokens[0],
        reported_output_tokens=reported_tokens[1],
        reported_thinking_tokens=reported_tokens[2],
        reported_cache_tokens=reported_tokens[3],
        observed_input_tokens=observed_input,
        observed_output_tokens=observed_output,
        observed_thinking_tokens=observed_thinking,
        observed_cache_tokens=observed_cache,
        observed_call_count=len(model_stages),
        expected_call_count=expected_call_count,
        missing_provider_usage_call_count=sum(not stage.usage_reported for stage in model_stages),
    )
    return NormalizedAttemptEvidence(stages=stages, usage=usage, raw_evidence=raw_evidence)
