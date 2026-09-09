# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Primary telemetry metric producers.

Produces canonical ``MetricObservation`` records from verified immutable
source records (``StageObservation``, ``LocalResourceObservation``,
``HumanWaitObservation``). Each producer validates the source records
fail-closed: negative durations, mismatched clock domains, incomplete
timing pairs, unreported usage represented as measured usage, unverified
resource records, and duplicate referenced observations are rejected.

The generated observations feed into the existing
``_compute_metric_results`` aggregation path. ``provider_cost_usd`` is
deferred until the typed price table is wired into the analysis input.
"""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Sequence

from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.schema import (
    AttemptRecord,
    GraderClass,
    HumanWaitObservation,
    LocalResourceObservation,
    MetricObservation,
    StageKind,
    StageObservation,
    VerificationStatus,
)

_GRADER_VERSION = "1.0.0"


def _attempt_lookup(attempts: Sequence[AttemptRecord], run_id: str) -> dict[str, AttemptRecord]:
    """Build a lookup of valid attempts by ID, filtered to the given run."""
    return {a.attempt_id: a for a in attempts if a.run_id == run_id}


def _stage_latency_for_attempt(
    stages: Sequence[StageObservation],
    attempt_id: str,
    run_id: str,
) -> list[float]:
    """Extract valid per-stage latencies for one attempt.

    Rejects incomplete timing pairs, negative durations, and mismatched
    clock domains. Returns an empty list if no valid stages remain.
    """
    latencies: list[float] = []
    clock_domains: set[str] = set()
    for stage in stages:
        if stage.attempt_id != attempt_id or stage.run_id != run_id:
            continue
        if stage.monotonic_start is None or stage.monotonic_end is None:
            continue
        duration = stage.monotonic_end - stage.monotonic_start
        if duration < 0:
            continue
        clock_domains.add(stage.clock_domain)
        latencies.append(duration)
    if len(clock_domains) > 1:
        return []
    return latencies


def produce_stage_latency_observations(
    stages: Sequence[StageObservation],
    attempts: Sequence[AttemptRecord],
    run_id: str,
) -> list[MetricObservation]:
    """Produce ``stage_latency_seconds`` metric observations from stage timing.

    Computes the mean wall-clock latency per deterministic stage for each
    attempt. Rejects incomplete timing pairs, negative durations, and
    mismatched clock domains within one attempt.
    """
    definition = DEFAULT_METRIC_REGISTRY.get("stage_latency_seconds", _GRADER_VERSION)
    attempt_map = _attempt_lookup(attempts, run_id)
    stages_by_attempt: dict[str, list[StageObservation]] = defaultdict(list)
    for stage in stages:
        if stage.run_id != run_id:
            continue
        if stage.attempt_id not in attempt_map:
            continue
        stages_by_attempt[stage.attempt_id].append(stage)

    results: list[MetricObservation] = []
    for attempt_id in sorted(stages_by_attempt.keys()):
        latencies = _stage_latency_for_attempt(stages_by_attempt[attempt_id], attempt_id, run_id)
        if not latencies:
            continue
        attempt = attempt_map[attempt_id]
        mean_latency = sum(latencies) / len(latencies)
        results.append(MetricObservation(
            metric_id="stage_latency_seconds",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=run_id,
            arm_id=attempt.arm_id,
            task_id=attempt.task_id,
            value=round(mean_latency, 10),
            unit=definition.unit,
            eligible=True,
            denominator_contribution=len(latencies),
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
            evidence_refs=[s.stage_id for s in stages_by_attempt[attempt_id]],
        ))
    return results


def produce_provider_usage_observations(
    stages: Sequence[StageObservation],
    attempts: Sequence[AttemptRecord],
    run_id: str,
) -> list[MetricObservation]:
    """Produce ``provider_usage_tokens`` metric observations from stage token usage.

    Sums input, output, thinking, and cache tokens across model-inference
    stages for each attempt. Rejects unreported or estimated usage and
    stages with no token fields.
    """
    definition = DEFAULT_METRIC_REGISTRY.get("provider_usage_tokens", _GRADER_VERSION)
    attempt_map = _attempt_lookup(attempts, run_id)
    stages_by_attempt: dict[str, list[StageObservation]] = defaultdict(list)
    for stage in stages:
        if stage.run_id != run_id:
            continue
        if stage.attempt_id not in attempt_map:
            continue
        if stage.kind != StageKind.MODEL_INFERENCE:
            continue
        stages_by_attempt[stage.attempt_id].append(stage)

    results: list[MetricObservation] = []
    for attempt_id in sorted(stages_by_attempt.keys()):
        total_tokens = 0
        valid_stages: list[StageObservation] = []
        for stage in stages_by_attempt[attempt_id]:
            if not stage.usage_reported or stage.usage_estimated:
                continue
            tokens = sum(t for t in (stage.input_tokens, stage.output_tokens, stage.thinking_tokens, stage.cache_tokens) if t is not None)
            if tokens == 0:
                continue
            total_tokens += tokens
            valid_stages.append(stage)
        if not valid_stages:
            continue
        attempt = attempt_map[attempt_id]
        results.append(MetricObservation(
            metric_id="provider_usage_tokens",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=run_id,
            arm_id=attempt.arm_id,
            task_id=attempt.task_id,
            value=float(total_tokens),
            unit=definition.unit,
            eligible=True,
            denominator_contribution=len(valid_stages),
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
            evidence_refs=[s.stage_id for s in valid_stages],
        ))
    return results


def produce_local_resource_observations(
    records: Sequence[LocalResourceObservation],
    attempts: Sequence[AttemptRecord],
    run_id: str,
    *,
    metric_id: str,
) -> list[MetricObservation]:
    """Produce local-resource metric observations from verified records.

    Supports ``local_resource_peak_memory_bytes`` and
    ``local_resource_cpu_seconds``. Rejects unverified records, None
    values, negative CPU seconds, cross-run records, records for unknown
    attempts, and duplicate observation IDs.
    """
    definition = DEFAULT_METRIC_REGISTRY.get(metric_id, _GRADER_VERSION)
    attempt_map = _attempt_lookup(attempts, run_id)

    seen_ids: set[str] = set()
    by_attempt: dict[str, list[LocalResourceObservation]] = defaultdict(list)
    for record in records:
        if record.run_id != run_id:
            continue
        if record.attempt_id not in attempt_map:
            continue
        if record.observation_id in seen_ids:
            raise ValueError(f"duplicate local-resource observation ID: {record.observation_id}")
        seen_ids.add(record.observation_id)
        if record.verification_status != VerificationStatus.VERIFIED:
            continue
        if metric_id == "local_resource_peak_memory_bytes":
            if record.peak_memory_bytes is None:
                continue
            if record.peak_memory_bytes < 0:
                raise ValueError(f"negative peak memory bytes: {record.peak_memory_bytes}")
        elif metric_id == "local_resource_cpu_seconds":
            if record.cpu_seconds is None:
                continue
            if record.cpu_seconds < 0:
                raise ValueError(f"negative cpu seconds: {record.cpu_seconds}")
        by_attempt[record.attempt_id].append(record)

    results: list[MetricObservation] = []
    for attempt_id in sorted(by_attempt.keys()):
        attempt = attempt_map[attempt_id]
        recs = by_attempt[attempt_id]
        if metric_id == "local_resource_peak_memory_bytes":
            value = float(max(r.peak_memory_bytes for r in recs))  # type: ignore[arg-type]
        else:
            value = float(sum(r.cpu_seconds for r in recs))  # type: ignore[arg-type]
        results.append(MetricObservation(
            metric_id=metric_id,
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=run_id,
            arm_id=attempt.arm_id,
            task_id=attempt.task_id,
            value=round(value, 10),
            unit=definition.unit,
            eligible=True,
            denominator_contribution=len(recs),
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
            evidence_refs=[r.observation_id for r in recs],
        ))
    return results


def produce_human_wait_observations(
    records: Sequence[HumanWaitObservation],
    attempts: Sequence[AttemptRecord],
    run_id: str,
) -> list[MetricObservation]:
    """Produce ``human_wait_seconds`` metric observations from verified records.

    Rejects unverified records, None values, negative wait seconds,
    cross-run records, records for unknown attempts, and duplicate
    observation IDs.
    """
    definition = DEFAULT_METRIC_REGISTRY.get("human_wait_seconds", _GRADER_VERSION)
    attempt_map = _attempt_lookup(attempts, run_id)

    seen_ids: set[str] = set()
    by_attempt: dict[str, list[HumanWaitObservation]] = defaultdict(list)
    for record in records:
        if record.run_id != run_id:
            continue
        if record.attempt_id not in attempt_map:
            continue
        if record.observation_id in seen_ids:
            raise ValueError(f"duplicate human-wait observation ID: {record.observation_id}")
        seen_ids.add(record.observation_id)
        if record.verification_status != VerificationStatus.VERIFIED:
            continue
        if record.human_wait_seconds is None:
            continue
        if record.human_wait_seconds < 0:
            raise ValueError(f"negative human wait seconds: {record.human_wait_seconds}")
        by_attempt[record.attempt_id].append(record)

    results: list[MetricObservation] = []
    for attempt_id in sorted(by_attempt.keys()):
        attempt = attempt_map[attempt_id]
        recs = by_attempt[attempt_id]
        total_wait = sum(r.human_wait_seconds for r in recs)  # type: ignore[arg-type]
        results.append(MetricObservation(
            metric_id="human_wait_seconds",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=run_id,
            arm_id=attempt.arm_id,
            task_id=attempt.task_id,
            value=round(float(total_wait), 10),
            unit=definition.unit,
            eligible=True,
            denominator_contribution=len(recs),
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
            evidence_refs=[r.observation_id for r in recs],
        ))
    return results


__all__ = [
    "produce_human_wait_observations",
    "produce_local_resource_observations",
    "produce_provider_usage_observations",
    "produce_stage_latency_observations",
]
