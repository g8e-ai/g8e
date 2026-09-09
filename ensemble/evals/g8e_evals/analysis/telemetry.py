# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Primary telemetry metric producers.

Produces canonical ``MetricObservation`` records from verified immutable
source records (``StageObservation``, ``LocalResourceObservation``,
``HumanWaitObservation``). Each producer validates source content
fail-closed: negative durations, mismatched clock domains, incomplete
timing pairs, unreported usage represented as measured usage, missing
provider/model binding, unverified resource records, and duplicate
referenced observations raise ``TelemetryProducerError``. Genuine
absence (no source record supplied for an attempt) is preserved as
missing evidence: no observation is produced and the attempt remains
in the denominator as missing.

All six declared telemetry metrics are registered in a typed producer
registry keyed by ``(metric_id, metric_version)``. The registry asserts
exactly one producer for each required metric at import time. The
canonical analysis engine calls ``run_all_telemetry_producers`` before
``_compute_metric_results`` and rejects collisions between produced and
caller-supplied observations.
"""

from __future__ import annotations

from collections import defaultdict
from collections.abc import Callable, Sequence

from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.metrics import DEFAULT_METRIC_REGISTRY
from g8e_evals.schema import (
    AttemptRecord,
    GraderClass,
    HumanWaitObservation,
    LocalResourceObservation,
    MetricObservation,
    PriceTableEntry,
    StageKind,
    StageObservation,
    TypedPriceTable,
    VerificationStatus,
)

_GRADER_VERSION = "1.0.0"


# ---------------------------------------------------------------------------
# Typed errors
# ---------------------------------------------------------------------------


class TelemetryProducerError(ValueError):
    """Typed failure from a telemetry producer.

    Raised when a supplied source record is malformed: incomplete timing
    pairs, negative durations, mixed clock domains, estimated usage
    presented as measured, missing provider/model binding, unverified
    resource records, or duplicate referenced observations. Genuine
    absence (no source record supplied) does not raise; it produces no
    observation.
    """


class DuplicateTelemetryProducerError(TelemetryProducerError):
    """Duplicate producer registration in the telemetry registry."""


class TelemetryRegistryIncompleteError(TelemetryProducerError):
    """Registry does not have exactly one producer for each required metric."""


class TelemetryObservationCollisionError(TelemetryProducerError):
    """Produced observation collides with a caller-supplied observation."""


# ---------------------------------------------------------------------------
# Producer type and registry
# ---------------------------------------------------------------------------

TelemetryProducer = Callable[[AnalysisInputRecord], list[MetricObservation]]

_REQUIRED_TELEMETRY_METRICS: list[tuple[str, str]] = [
    ("stage_latency_seconds", _GRADER_VERSION),
    ("provider_usage_tokens", _GRADER_VERSION),
    ("provider_cost_usd", _GRADER_VERSION),
    ("local_resource_peak_memory_bytes", _GRADER_VERSION),
    ("local_resource_cpu_seconds", _GRADER_VERSION),
    ("human_wait_seconds", _GRADER_VERSION),
]


class TelemetryProducerRegistry:
    """Immutable registry of telemetry producers keyed by (metric_id, metric_version).

    Registration rejects duplicates. After construction, ``assert_complete``
    verifies that exactly one producer exists for each required telemetry
    metric. The registry runner ``run_all`` calls every producer with the
    same ``AnalysisInputRecord`` and returns the combined observations.
    """

    def __init__(self) -> None:
        self._producers: dict[tuple[str, str], TelemetryProducer] = {}

    def register(self, metric_id: str, metric_version: str, producer: TelemetryProducer) -> None:
        key = (metric_id, metric_version)
        if key in self._producers:
            raise DuplicateTelemetryProducerError(
                f"telemetry producer already registered: {metric_id}@{metric_version}"
            )
        self._producers[key] = producer

    def get(self, metric_id: str, metric_version: str) -> TelemetryProducer:
        key = (metric_id, metric_version)
        producer = self._producers.get(key)
        if producer is None:
            raise TelemetryProducerError(
                f"no telemetry producer registered for {metric_id}@{metric_version}"
            )
        return producer

    def all_producers(self) -> list[tuple[tuple[str, str], TelemetryProducer]]:
        return sorted(self._producers.items(), key=lambda kv: kv[0])

    def assert_complete(self) -> None:
        registered = set(self._producers.keys())
        required = set(_REQUIRED_TELEMETRY_METRICS)
        missing = required - registered
        extra = registered - required
        if missing or extra:
            parts: list[str] = []
            if missing:
                parts.append(f"missing: {sorted(missing)}")
            if extra:
                parts.append(f"extra: {sorted(extra)}")
            raise TelemetryRegistryIncompleteError(
                "telemetry producer registry is not complete: " + "; ".join(parts)
            )

    def run_all(self, record: AnalysisInputRecord) -> list[MetricObservation]:
        results: list[MetricObservation] = []
        for _key, producer in self.all_producers():
            results.extend(producer(record))
        return results


# ---------------------------------------------------------------------------
# Shared helpers
# ---------------------------------------------------------------------------


def _attempt_lookup(record: AnalysisInputRecord) -> dict[str, AttemptRecord]:
    return {a.attempt_id: a for a in record.attempts}


# ---------------------------------------------------------------------------
# stage_latency_seconds
# ---------------------------------------------------------------------------


def produce_stage_latency_observations(record: AnalysisInputRecord) -> list[MetricObservation]:
    """Produce ``stage_latency_seconds`` metric observations from stage timing.

    Computes the mean wall-clock latency per stage for each attempt.
    Raises ``TelemetryProducerError`` for incomplete timing pairs,
    negative durations, and mixed clock domains. Attempts with no stage
    records produce no observation (missing evidence).
    """
    definition = DEFAULT_METRIC_REGISTRY.get("stage_latency_seconds", _GRADER_VERSION)
    attempt_map = _attempt_lookup(record)

    stages_by_attempt: dict[str, list[StageObservation]] = defaultdict(list)
    for stage in record.stages:
        stages_by_attempt[stage.attempt_id].append(stage)

    results: list[MetricObservation] = []
    for attempt_id in sorted(stages_by_attempt.keys()):
        attempt_stages = stages_by_attempt[attempt_id]
        attempt = attempt_map.get(attempt_id)
        if attempt is None:
            continue

        latencies: list[float] = []
        clock_domains: set[str] = set()
        timed_stage_ids: list[str] = []
        for stage in attempt_stages:
            has_start = stage.monotonic_start is not None
            has_end = stage.monotonic_end is not None
            if not has_start and not has_end:
                continue
            if not has_start or not has_end:
                raise TelemetryProducerError(
                    f"incomplete timing pair for stage {stage.stage_id}: "
                    f"monotonic_start={stage.monotonic_start}, "
                    f"monotonic_end={stage.monotonic_end}"
                )
            duration = stage.monotonic_end - stage.monotonic_start  # type: ignore[arg-type]
            if duration < 0:
                raise TelemetryProducerError(
                    f"negative duration for stage {stage.stage_id}: {duration}"
                )
            clock_domains.add(stage.clock_domain)
            latencies.append(duration)
            timed_stage_ids.append(stage.stage_id)

        if not latencies:
            continue
        if len(clock_domains) > 1:
            raise TelemetryProducerError(
                f"mixed clock domains for attempt {attempt_id}: {sorted(clock_domains)}"
            )

        mean_latency = sum(latencies) / len(latencies)
        results.append(MetricObservation(
            metric_id="stage_latency_seconds",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=record.run_id,
            arm_id=attempt.arm_id,
            task_id=attempt.task_id,
            value=round(mean_latency, 10),
            unit=definition.unit,
            eligible=True,
            denominator_contribution=len(latencies),
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
            evidence_refs=timed_stage_ids,
        ))
    return results


# ---------------------------------------------------------------------------
# provider_usage_tokens
# ---------------------------------------------------------------------------


def produce_provider_usage_observations(record: AnalysisInputRecord) -> list[MetricObservation]:
    """Produce ``provider_usage_tokens`` metric observations from stage token usage.

    Sums input, output, thinking, and cache tokens across model-inference
    stages for each attempt. Raises ``TelemetryProducerError`` for estimated
    usage presented as measured usage and missing provider/model binding on
    model-inference stages with token data. Attempts with no model-inference
    stages with reported usage produce no observation (missing evidence).
    """
    definition = DEFAULT_METRIC_REGISTRY.get("provider_usage_tokens", _GRADER_VERSION)
    attempt_map = _attempt_lookup(record)

    stages_by_attempt: dict[str, list[StageObservation]] = defaultdict(list)
    for stage in record.stages:
        if stage.kind != StageKind.MODEL_INFERENCE:
            continue
        stages_by_attempt[stage.attempt_id].append(stage)

    results: list[MetricObservation] = []
    for attempt_id in sorted(stages_by_attempt.keys()):
        attempt_stages = stages_by_attempt[attempt_id]
        attempt = attempt_map.get(attempt_id)
        if attempt is None:
            continue

        total_tokens = 0
        valid_stages: list[StageObservation] = []
        for stage in attempt_stages:
            has_tokens = any(
                t is not None
                for t in (stage.input_tokens, stage.output_tokens, stage.thinking_tokens, stage.cache_tokens)
            )
            if not has_tokens:
                continue
            if not stage.usage_reported:
                raise TelemetryProducerError(
                    f"unreported usage for model-inference stage {stage.stage_id} "
                    f"with token data"
                )
            if stage.usage_estimated:
                raise TelemetryProducerError(
                    f"estimated usage presented as measured for stage {stage.stage_id}"
                )
            if not stage.provider or not stage.model:
                raise TelemetryProducerError(
                    f"missing provider/model binding for model-inference stage {stage.stage_id}"
                )
            tokens = sum(
                t for t in (stage.input_tokens, stage.output_tokens, stage.thinking_tokens, stage.cache_tokens)
                if t is not None
            )
            total_tokens += tokens
            valid_stages.append(stage)

        if not valid_stages:
            continue

        results.append(MetricObservation(
            metric_id="provider_usage_tokens",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=record.run_id,
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


# ---------------------------------------------------------------------------
# provider_cost_usd
# ---------------------------------------------------------------------------


def _build_price_lookup(
    price_table: TypedPriceTable,
) -> dict[tuple[str, str], PriceTableEntry]:
    """Build a lookup from (provider, model) to price entry.

    Raises ``TelemetryProducerError`` for duplicate entries with the same
    (provider, model) pair.
    """
    lookup: dict[tuple[str, str], PriceTableEntry] = {}
    for entry in price_table.entries:
        key = (entry.provider, entry.model)
        if key in lookup:
            raise TelemetryProducerError(
                f"duplicate price table entry for provider={entry.provider}, "
                f"model={entry.model}"
            )
        lookup[key] = entry
    return lookup


def produce_provider_cost_observations(record: AnalysisInputRecord) -> list[MetricObservation]:
    """Produce ``provider_cost_usd`` metric observations from token usage and a price table.

    Binds each model-inference stage to an exact provider and model, selects
    exactly one effective price entry, and calculates input/output/thinking/cache
    costs separately. Raises ``TelemetryProducerError`` for missing provider/model
    binding, missing price entry, duplicate price entries, estimated usage
    presented as measured, and unreported usage with token data.

    If no price table is supplied, no observations are produced (missing
    evidence for all attempts).
    """
    definition = DEFAULT_METRIC_REGISTRY.get("provider_cost_usd", _GRADER_VERSION)
    attempt_map = _attempt_lookup(record)

    if record.price_table is None:
        return []

    price_lookup = _build_price_lookup(record.price_table)

    stages_by_attempt: dict[str, list[StageObservation]] = defaultdict(list)
    for stage in record.stages:
        if stage.kind != StageKind.MODEL_INFERENCE:
            continue
        stages_by_attempt[stage.attempt_id].append(stage)

    results: list[MetricObservation] = []
    for attempt_id in sorted(stages_by_attempt.keys()):
        attempt_stages = stages_by_attempt[attempt_id]
        attempt = attempt_map.get(attempt_id)
        if attempt is None:
            continue

        total_cost = 0.0
        valid_stages: list[StageObservation] = []
        for stage in attempt_stages:
            has_tokens = any(
                t is not None
                for t in (stage.input_tokens, stage.output_tokens, stage.thinking_tokens, stage.cache_tokens)
            )
            if not has_tokens:
                continue
            if not stage.usage_reported:
                raise TelemetryProducerError(
                    f"unreported usage for model-inference stage {stage.stage_id} "
                    f"with token data"
                )
            if stage.usage_estimated:
                raise TelemetryProducerError(
                    f"estimated usage presented as measured for stage {stage.stage_id}"
                )
            if not stage.provider or not stage.model:
                raise TelemetryProducerError(
                    f"missing provider/model binding for model-inference stage {stage.stage_id}"
                )
            price_entry = price_lookup.get((stage.provider, stage.model))
            if price_entry is None:
                raise TelemetryProducerError(
                    f"no price table entry for provider={stage.provider}, "
                    f"model={stage.model} (stage {stage.stage_id})"
                )

            input_tokens = stage.input_tokens or 0
            output_tokens = stage.output_tokens or 0
            thinking_tokens = stage.thinking_tokens or 0
            cache_tokens = stage.cache_tokens or 0

            stage_cost = (
                input_tokens * price_entry.input_token_price_usd
                + output_tokens * price_entry.output_token_price_usd
                + thinking_tokens * price_entry.thinking_token_price_usd
                + cache_tokens * price_entry.cache_token_price_usd
            )
            total_cost += stage_cost
            valid_stages.append(stage)

        if not valid_stages:
            continue

        results.append(MetricObservation(
            metric_id="provider_cost_usd",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=record.run_id,
            arm_id=attempt.arm_id,
            task_id=attempt.task_id,
            value=round(total_cost, 10),
            unit=definition.unit,
            eligible=True,
            denominator_contribution=len(valid_stages),
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
            evidence_refs=[s.stage_id for s in valid_stages],
        ))
    return results


# ---------------------------------------------------------------------------
# local_resource_peak_memory_bytes
# ---------------------------------------------------------------------------


def produce_local_resource_peak_memory_observations(
    record: AnalysisInputRecord,
) -> list[MetricObservation]:
    """Produce ``local_resource_peak_memory_bytes`` metric observations from verified records.

    Raises ``TelemetryProducerError`` for unverified records with a peak
    memory value, negative peak memory bytes, and duplicate observation IDs.
    Attempts with no local-resource records produce no observation (missing
    evidence).
    """
    definition = DEFAULT_METRIC_REGISTRY.get("local_resource_peak_memory_bytes", _GRADER_VERSION)
    attempt_map = _attempt_lookup(record)

    seen_ids: set[str] = set()
    by_attempt: dict[str, list[LocalResourceObservation]] = defaultdict(list)
    for rec in record.local_resource_observations:
        if rec.observation_id in seen_ids:
            raise TelemetryProducerError(
                f"duplicate local-resource observation ID: {rec.observation_id}"
            )
        seen_ids.add(rec.observation_id)
        if rec.peak_memory_bytes is None:
            continue
        if rec.verification_status != VerificationStatus.VERIFIED:
            raise TelemetryProducerError(
                f"unverified local-resource record with peak_memory_bytes: "
                f"{rec.observation_id}"
            )
        if rec.peak_memory_bytes < 0:
            raise TelemetryProducerError(
                f"negative peak memory bytes: {rec.peak_memory_bytes}"
            )
        by_attempt[rec.attempt_id].append(rec)

    results: list[MetricObservation] = []
    for attempt_id in sorted(by_attempt.keys()):
        attempt = attempt_map.get(attempt_id)
        if attempt is None:
            continue
        recs = by_attempt[attempt_id]
        value = float(max(r.peak_memory_bytes for r in recs))  # type: ignore[arg-type]
        results.append(MetricObservation(
            metric_id="local_resource_peak_memory_bytes",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=record.run_id,
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


# ---------------------------------------------------------------------------
# local_resource_cpu_seconds
# ---------------------------------------------------------------------------


def produce_local_resource_cpu_seconds_observations(
    record: AnalysisInputRecord,
) -> list[MetricObservation]:
    """Produce ``local_resource_cpu_seconds`` metric observations from verified records.

    Raises ``TelemetryProducerError`` for unverified records with a CPU
    seconds value, negative CPU seconds, and duplicate observation IDs.
    Attempts with no local-resource records produce no observation (missing
    evidence).
    """
    definition = DEFAULT_METRIC_REGISTRY.get("local_resource_cpu_seconds", _GRADER_VERSION)
    attempt_map = _attempt_lookup(record)

    seen_ids: set[str] = set()
    by_attempt: dict[str, list[LocalResourceObservation]] = defaultdict(list)
    for rec in record.local_resource_observations:
        if rec.observation_id in seen_ids:
            raise TelemetryProducerError(
                f"duplicate local-resource observation ID: {rec.observation_id}"
            )
        seen_ids.add(rec.observation_id)
        if rec.cpu_seconds is None:
            continue
        if rec.verification_status != VerificationStatus.VERIFIED:
            raise TelemetryProducerError(
                f"unverified local-resource record with cpu_seconds: "
                f"{rec.observation_id}"
            )
        if rec.cpu_seconds < 0:
            raise TelemetryProducerError(
                f"negative cpu seconds: {rec.cpu_seconds}"
            )
        by_attempt[rec.attempt_id].append(rec)

    results: list[MetricObservation] = []
    for attempt_id in sorted(by_attempt.keys()):
        attempt = attempt_map.get(attempt_id)
        if attempt is None:
            continue
        recs = by_attempt[attempt_id]
        value = float(sum(r.cpu_seconds for r in recs))  # type: ignore[arg-type]
        results.append(MetricObservation(
            metric_id="local_resource_cpu_seconds",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=record.run_id,
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


# ---------------------------------------------------------------------------
# human_wait_seconds
# ---------------------------------------------------------------------------


def produce_human_wait_observations(record: AnalysisInputRecord) -> list[MetricObservation]:
    """Produce ``human_wait_seconds`` metric observations from verified records.

    Raises ``TelemetryProducerError`` for unverified records with a
    human_wait_seconds value, negative wait seconds, and duplicate
    observation IDs. Attempts with no human-wait records produce no
    observation (missing evidence).
    """
    definition = DEFAULT_METRIC_REGISTRY.get("human_wait_seconds", _GRADER_VERSION)
    attempt_map = _attempt_lookup(record)

    seen_ids: set[str] = set()
    by_attempt: dict[str, list[HumanWaitObservation]] = defaultdict(list)
    for rec in record.human_wait_observations:
        if rec.observation_id in seen_ids:
            raise TelemetryProducerError(
                f"duplicate human-wait observation ID: {rec.observation_id}"
            )
        seen_ids.add(rec.observation_id)
        if rec.human_wait_seconds is None:
            continue
        if rec.verification_status != VerificationStatus.VERIFIED:
            raise TelemetryProducerError(
                f"unverified human-wait record with human_wait_seconds: "
                f"{rec.observation_id}"
            )
        if rec.human_wait_seconds < 0:
            raise TelemetryProducerError(
                f"negative human wait seconds: {rec.human_wait_seconds}"
            )
        by_attempt[rec.attempt_id].append(rec)

    results: list[MetricObservation] = []
    for attempt_id in sorted(by_attempt.keys()):
        attempt = attempt_map.get(attempt_id)
        if attempt is None:
            continue
        recs = by_attempt[attempt_id]
        total_wait = sum(r.human_wait_seconds for r in recs)  # type: ignore[arg-type]
        results.append(MetricObservation(
            metric_id="human_wait_seconds",
            metric_version=_GRADER_VERSION,
            attempt_id=attempt_id,
            run_id=record.run_id,
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


# ---------------------------------------------------------------------------
# Registry construction and runner
# ---------------------------------------------------------------------------


def _build_default_telemetry_registry() -> TelemetryProducerRegistry:
    registry = TelemetryProducerRegistry()
    registry.register("stage_latency_seconds", _GRADER_VERSION, produce_stage_latency_observations)
    registry.register("provider_usage_tokens", _GRADER_VERSION, produce_provider_usage_observations)
    registry.register("provider_cost_usd", _GRADER_VERSION, produce_provider_cost_observations)
    registry.register("local_resource_peak_memory_bytes", _GRADER_VERSION, produce_local_resource_peak_memory_observations)
    registry.register("local_resource_cpu_seconds", _GRADER_VERSION, produce_local_resource_cpu_seconds_observations)
    registry.register("human_wait_seconds", _GRADER_VERSION, produce_human_wait_observations)
    registry.assert_complete()
    return registry


DEFAULT_TELEMETRY_REGISTRY = _build_default_telemetry_registry()


def run_all_telemetry_producers(
    record: AnalysisInputRecord,
    caller_supplied: Sequence[MetricObservation],
) -> list[MetricObservation]:
    """Run all registered telemetry producers and merge with caller-supplied observations.

    Produced observations are appended to the caller-supplied list. Collisions
    (same metric_id, metric_version, and attempt_id) between produced and
    caller-supplied observations raise ``TelemetryObservationCollisionError``.
    """
    caller_keys: set[tuple[str, str, str]] = set()
    for obs in caller_supplied:
        caller_keys.add((obs.metric_id, obs.metric_version, obs.attempt_id))

    produced = DEFAULT_TELEMETRY_REGISTRY.run_all(record)

    for obs in produced:
        key = (obs.metric_id, obs.metric_version, obs.attempt_id)
        if key in caller_keys:
            raise TelemetryObservationCollisionError(
                f"telemetry producer collision for {obs.metric_id}@{obs.metric_version} "
                f"attempt {obs.attempt_id}"
            )
        caller_keys.add(key)

    return list(caller_supplied) + produced


__all__ = [
    "DEFAULT_TELEMETRY_REGISTRY",
    "DuplicateTelemetryProducerError",
    "TelemetryObservationCollisionError",
    "TelemetryProducerError",
    "TelemetryProducerRegistry",
    "TelemetryRegistryIncompleteError",
    "produce_human_wait_observations",
    "produce_local_resource_cpu_seconds_observations",
    "produce_local_resource_peak_memory_observations",
    "produce_provider_cost_observations",
    "produce_provider_usage_observations",
    "produce_stage_latency_observations",
    "run_all_telemetry_producers",
]
