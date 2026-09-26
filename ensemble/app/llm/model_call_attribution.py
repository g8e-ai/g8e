# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Shared helpers for governed-provider attribution at chat model call sites."""

from __future__ import annotations

import time
from typing import Any

from app.llm.model_evidence import (
    recorded_governed_dispatch_evidence,
    recorded_model_boundary_hash,
    recorded_model_boundary_privacy,
)
from app.llm.provider import LLMProvider
from app.decision.provider import DecisionProvider
from app.models.http_context import G8eHttpContext
from app.models.model_telemetry import ModelCallTelemetry

ProviderForTelemetry = LLMProvider | DecisionProvider


def prepare_provider_call(
    provider: LLMProvider,
    *,
    g8e_context: G8eHttpContext | None = None,
    retry_count: int = 0,
) -> None:
    """Reset per-call evidence and bind request-scoped evaluation context."""
    provider.clear_input_artifact_hash()
    provider.set_g8e_context(g8e_context)
    provider.set_provider_retry_count(retry_count)


def governed_telemetry_fields(provider: LLMProvider) -> dict[str, Any]:
    """Return governed-dispatch evidence flattened for ModelCallTelemetry."""
    evidence = recorded_governed_dispatch_evidence(provider)
    if evidence is None:
        return {}
    return {
        "governed_transaction_id": evidence.transaction_id or None,
        "governed_result_digest": evidence.result_digest or None,
        "governed_receipt_status": evidence.receipt_status or None,
        "provider_attempt_id": evidence.provider_attempt_id or None,
        "requested_model": evidence.requested_model or None,
        "served_model": evidence.served_model or None,
        "model_digest": evidence.model_digest or None,
        "normalized_request_hash": evidence.normalized_request_hash or None,
        "governed_output_hash": evidence.output_hash or None,
        "campaign_id": evidence.campaign_id or None,
        "run_id": evidence.run_id or None,
        "assignment_id": evidence.assignment_id or None,
        "evaluation_attempt_id": evidence.evaluation_attempt_id or None,
        "scenario_id": evidence.scenario_id or None,
        "model_registry_digest": evidence.model_registry_digest or None,
    }


def build_model_call_telemetry(
    *,
    provider: ProviderForTelemetry,
    agent_role: str,
    model: str,
    monotonic_start: float,
    input_artifact_hash: str,
    model_role: str | None = None,
    monotonic_end: float | None = None,
    input_tokens: int = 0,
    output_tokens: int = 0,
    thinking_tokens: int | None = None,
    cache_tokens: int | None = None,
    total_tokens: int = 0,
    usage_reported: bool = False,
    finish_reason: str | None = None,
    time_to_first_token_seconds: float | None = None,
    generation_duration_seconds: float | None = None,
    prompt_eval_duration_seconds: float | None = None,
    total_duration_seconds: float | None = None,
    load_duration_seconds: float | None = None,
    retry_count: int = 0,
    succeeded: bool = True,
    error_type: str | None = None,
    output_artifact_hash: str = "",
    **extra: Any,
) -> ModelCallTelemetry:
    """Build attribution-complete telemetry for one provider invocation."""
    resolved_end = monotonic_end if monotonic_end is not None else time.monotonic()
    resolved_input_hash = recorded_model_boundary_hash(provider, input_artifact_hash)
    return ModelCallTelemetry(
        agent_role=agent_role,
        model_role=model_role,
        provider=type(provider).__name__,
        model=model,
        monotonic_start=monotonic_start,
        monotonic_end=resolved_end,
        input_tokens=input_tokens,
        output_tokens=output_tokens,
        thinking_tokens=thinking_tokens,
        cache_tokens=cache_tokens,
        total_tokens=total_tokens,
        usage_reported=usage_reported,
        finish_reason=finish_reason,
        time_to_first_token_seconds=time_to_first_token_seconds,
        generation_duration_seconds=generation_duration_seconds,
        prompt_eval_duration_seconds=prompt_eval_duration_seconds,
        total_duration_seconds=total_duration_seconds,
        load_duration_seconds=load_duration_seconds,
        retry_count=retry_count,
        succeeded=succeeded,
        error_type=error_type,
        input_artifact_hash=resolved_input_hash,
        output_artifact_hash=output_artifact_hash,
        model_boundary_privacy=recorded_model_boundary_privacy(provider),
        **governed_telemetry_fields(provider),
        **extra,
    )
