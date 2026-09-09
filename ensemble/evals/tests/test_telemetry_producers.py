# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for primary telemetry metric producers.

Verifies that telemetry ``MetricObservation`` records are produced from
verified immutable source records (``StageObservation``,
``LocalResourceObservation``, ``HumanWaitObservation``) and that
malformed supplied records raise ``TelemetryProducerError`` fail-closed.
Genuine absence (no source record supplied) produces no observation.
No external dependencies (no files, network, or DB).
"""

from __future__ import annotations

from datetime import UTC, datetime

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis.input import AnalysisInputRecord
from g8e_evals.analysis.telemetry import (
    DEFAULT_TELEMETRY_REGISTRY,
    DuplicateTelemetryProducerError,
    TelemetryObservationCollisionError,
    TelemetryProducerError,
    TelemetryProducerRegistry,
    produce_human_wait_observations,
    produce_local_resource_cpu_seconds_observations,
    produce_local_resource_peak_memory_observations,
    produce_provider_cost_observations,
    produce_provider_usage_observations,
    produce_stage_latency_observations,
    run_all_telemetry_producers,
)
from g8e_evals.arms import Arm
from g8e_evals.schema import (
    AttemptRecord,
    HumanWaitObservation,
    LocalResourceObservation,
    PriceTableEntry,
    StageKind,
    StageObservation,
    TerminalStatus,
    TypedPriceTable,
    VerificationStatus,
)

_RUN_ID = "run-tel-1"
_TASK_ID = "task-tel-1"
_ATTEMPT_ID = "attempt-tel-1"
_TS = datetime(2026, 1, 1, tzinfo=UTC)
_HASH = "a" * 64


# ---------------------------------------------------------------------------
# Record builders
# ---------------------------------------------------------------------------


def _attempt(
    attempt_id: str = _ATTEMPT_ID,
    arm_id: Arm = Arm.DOCTRINE,
    terminal_status: TerminalStatus = TerminalStatus.COMPLETED,
) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=arm_id,
        terminal_status=terminal_status,
    )


def _stage(
    stage_id: str = "stage-1",
    kind: StageKind = StageKind.MODEL_INFERENCE,
    attempt_id: str = _ATTEMPT_ID,
    monotonic_start: float | None = 100.0,
    monotonic_end: float | None = 200.0,
    clock_domain: str = "monotonic",
    input_tokens: int | None = 10,
    output_tokens: int | None = 20,
    thinking_tokens: int | None = None,
    cache_tokens: int | None = None,
    usage_reported: bool = True,
    usage_estimated: bool = False,
    provider: str = "test-provider",
    model: str = "test-model",
) -> StageObservation:
    return StageObservation(
        stage_id=stage_id,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        kind=kind,
        monotonic_start=monotonic_start,
        monotonic_end=monotonic_end,
        clock_domain=clock_domain,
        input_tokens=input_tokens,
        output_tokens=output_tokens,
        thinking_tokens=thinking_tokens,
        cache_tokens=cache_tokens,
        usage_reported=usage_reported,
        usage_estimated=usage_estimated,
        provider=provider,
        model=model,
        task_id=_TASK_ID,
    )


def _local_resource(
    observation_id: str = "lr-1",
    attempt_id: str = _ATTEMPT_ID,
    peak_memory_bytes: int | None = 1024,
    cpu_seconds: float | None = 0.5,
    verification_status: VerificationStatus = VerificationStatus.VERIFIED,
) -> LocalResourceObservation:
    return LocalResourceObservation(
        observation_id=observation_id,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        peak_memory_bytes=peak_memory_bytes,
        cpu_seconds=cpu_seconds,
        collected_at=_TS,
        source_evidence_refs=["evidence-1"],
        source_evidence_sha256=_HASH,
        verification_status=verification_status,
    )


def _human_wait(
    observation_id: str = "hw-1",
    attempt_id: str = _ATTEMPT_ID,
    human_wait_seconds: float | None = 3.0,
    verification_status: VerificationStatus = VerificationStatus.VERIFIED,
) -> HumanWaitObservation:
    return HumanWaitObservation(
        observation_id=observation_id,
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        human_wait_seconds=human_wait_seconds,
        human_action_type="approval",
        collected_at=_TS,
        source_evidence_refs=["evidence-2"],
        source_evidence_sha256=_HASH,
        verification_status=verification_status,
    )


def _price_entry(
    provider: str = "test-provider",
    model: str = "test-model",
    input_price: float = 0.001,
    output_price: float = 0.002,
    thinking_price: float = 0.0,
    cache_price: float = 0.0,
) -> PriceTableEntry:
    return PriceTableEntry(
        provider=provider,
        model=model,
        input_token_price_usd=input_price,
        output_token_price_usd=output_price,
        thinking_token_price_usd=thinking_price,
        cache_token_price_usd=cache_price,
        effective_at=_TS,
    )


def _price_table(entries: list[PriceTableEntry] | None = None) -> TypedPriceTable:
    return TypedPriceTable(
        price_table_id="pt-1",
        price_table_version="1.0.0",
        entries=entries if entries is not None else [_price_entry()],
    )


def _record(
    attempts: list[AttemptRecord] | None = None,
    stages: list[StageObservation] | None = None,
    local_resource_observations: list[LocalResourceObservation] | None = None,
    human_wait_observations: list[HumanWaitObservation] | None = None,
    price_table: TypedPriceTable | None = None,
) -> AnalysisInputRecord:
    return AnalysisInputRecord(
        run_id=_RUN_ID,
        release_version="v2.1.8",
        tasks=[],
        attempts=attempts if attempts is not None else [_attempt()],
        stages=stages if stages is not None else [],
        local_resource_observations=local_resource_observations if local_resource_observations is not None else [],
        human_wait_observations=human_wait_observations if human_wait_observations is not None else [],
        price_table=price_table,
    )


# ---------------------------------------------------------------------------
# Registry tests
# ---------------------------------------------------------------------------


class TestTelemetryProducerRegistry:
    def test_default_registry_has_exactly_six_producers(self) -> None:
        producers = DEFAULT_TELEMETRY_REGISTRY.all_producers()
        assert len(producers) == 6
        metric_ids = {key[0] for key, _ in producers}
        assert metric_ids == {
            "stage_latency_seconds",
            "provider_usage_tokens",
            "provider_cost_usd",
            "local_resource_peak_memory_bytes",
            "local_resource_cpu_seconds",
            "human_wait_seconds",
        }

    def test_registry_rejects_duplicate_registration(self) -> None:
        registry = TelemetryProducerRegistry()
        registry.register("stage_latency_seconds", "1.0.0", produce_stage_latency_observations)
        with pytest.raises(DuplicateTelemetryProducerError):
            registry.register("stage_latency_seconds", "1.0.0", produce_stage_latency_observations)

    def test_registry_assert_complete_rejects_missing(self) -> None:
        registry = TelemetryProducerRegistry()
        registry.register("stage_latency_seconds", "1.0.0", produce_stage_latency_observations)
        with pytest.raises(Exception, match="missing"):
            registry.assert_complete()

    def test_registry_get_rejects_unknown(self) -> None:
        registry = TelemetryProducerRegistry()
        with pytest.raises(TelemetryProducerError, match="no telemetry producer"):
            registry.get("unknown_metric", "1.0.0")


# ---------------------------------------------------------------------------
# stage_latency_seconds
# ---------------------------------------------------------------------------


class TestStageLatencyProducer:
    def test_produces_observation_from_valid_stage(self) -> None:
        record = _record(stages=[_stage(monotonic_start=100.0, monotonic_end=250.0)])
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 1
        assert obs[0].metric_id == "stage_latency_seconds"
        assert obs[0].value == 150.0
        assert obs[0].eligible is True
        assert obs[0].denominator_contribution == 1
        assert obs[0].verification_status == VerificationStatus.VERIFIED

    def test_mean_latency_across_multiple_stages_for_one_attempt(self) -> None:
        record = _record(stages=[
            _stage(stage_id="s1", monotonic_start=0.0, monotonic_end=100.0),
            _stage(stage_id="s2", monotonic_start=100.0, monotonic_end=400.0),
        ])
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 1
        assert obs[0].value == 200.0

    def test_incomplete_timing_pair_raises_typed_error(self) -> None:
        record = _record(stages=[_stage(monotonic_start=100.0, monotonic_end=None)])
        with pytest.raises(TelemetryProducerError, match="incomplete timing pair"):
            produce_stage_latency_observations(record)

    def test_negative_duration_raises_typed_error(self) -> None:
        record = _record(stages=[_stage(monotonic_start=300.0, monotonic_end=100.0)])
        with pytest.raises(TelemetryProducerError, match="negative duration"):
            produce_stage_latency_observations(record)

    def test_mixed_clock_domains_raises_typed_error(self) -> None:
        record = _record(stages=[
            _stage(stage_id="s1", monotonic_start=0.0, monotonic_end=100.0, clock_domain="monotonic"),
            _stage(stage_id="s2", monotonic_start=100.0, monotonic_end=200.0, clock_domain="wall_clock"),
        ])
        with pytest.raises(TelemetryProducerError, match="mixed clock domains"):
            produce_stage_latency_observations(record)

    def test_stages_from_different_attempts_produce_separate_observations(self) -> None:
        attempts = [_attempt("a1"), _attempt("a2")]
        stages = [
            _stage(stage_id="s1", attempt_id="a1", monotonic_start=0.0, monotonic_end=100.0),
            _stage(stage_id="s2", attempt_id="a2", monotonic_start=0.0, monotonic_end=200.0),
        ]
        record = _record(attempts=attempts, stages=stages)
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 2
        by_attempt = {o.attempt_id: o for o in obs}
        assert by_attempt["a1"].value == 100.0
        assert by_attempt["a2"].value == 200.0

    def test_no_stages_produces_no_observations(self) -> None:
        record = _record(stages=[])
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 0

    def test_stage_with_no_timing_data_is_skipped_not_failure(self) -> None:
        record = _record(stages=[
            _stage(stage_id="s1", monotonic_start=None, monotonic_end=None),
            _stage(stage_id="s2", monotonic_start=0.0, monotonic_end=100.0),
        ])
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 1
        assert obs[0].value == 100.0

    def test_all_stages_without_timing_produces_no_observations(self) -> None:
        record = _record(stages=[_stage(stage_id="s1", monotonic_start=None, monotonic_end=None)])
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 0


# ---------------------------------------------------------------------------
# provider_usage_tokens
# ---------------------------------------------------------------------------


class TestProviderUsageProducer:
    def test_produces_observation_from_valid_stage(self) -> None:
        record = _record(stages=[_stage(input_tokens=10, output_tokens=20, thinking_tokens=5, cache_tokens=3)])
        obs = produce_provider_usage_observations(record)
        assert len(obs) == 1
        assert obs[0].metric_id == "provider_usage_tokens"
        assert obs[0].value == 38.0
        assert obs[0].denominator_contribution == 1

    def test_sums_tokens_across_multiple_stages_for_one_attempt(self) -> None:
        record = _record(stages=[
            _stage(stage_id="s1", input_tokens=10, output_tokens=20),
            _stage(stage_id="s2", input_tokens=5, output_tokens=15),
        ])
        obs = produce_provider_usage_observations(record)
        assert len(obs) == 1
        assert obs[0].value == 50.0

    def test_unreported_usage_with_tokens_raises_typed_error(self) -> None:
        record = _record(stages=[_stage(usage_reported=False, input_tokens=10, output_tokens=20)])
        with pytest.raises(TelemetryProducerError, match="unreported usage"):
            produce_provider_usage_observations(record)

    def test_estimated_usage_raises_typed_error(self) -> None:
        record = _record(stages=[_stage(usage_reported=True, usage_estimated=True, input_tokens=10, output_tokens=20)])
        with pytest.raises(TelemetryProducerError, match="estimated usage"):
            produce_provider_usage_observations(record)

    def test_missing_provider_model_binding_raises_typed_error(self) -> None:
        record = _record(stages=[_stage(provider="", model="", input_tokens=10, output_tokens=20)])
        with pytest.raises(TelemetryProducerError, match="missing provider/model binding"):
            produce_provider_usage_observations(record)

    def test_stage_with_no_token_fields_produces_no_observations(self) -> None:
        record = _record(stages=[_stage(input_tokens=None, output_tokens=None, thinking_tokens=None, cache_tokens=None)])
        obs = produce_provider_usage_observations(record)
        assert len(obs) == 0

    def test_only_counts_model_inference_stages(self) -> None:
        record = _record(stages=[
            _stage(stage_id="s1", kind=StageKind.MODEL_INFERENCE, input_tokens=10, output_tokens=20),
            _stage(stage_id="s2", kind=StageKind.GRADING, input_tokens=100, output_tokens=200),
        ])
        obs = produce_provider_usage_observations(record)
        assert len(obs) == 1
        assert obs[0].value == 30.0

    def test_no_stages_produces_no_observations(self) -> None:
        record = _record(stages=[])
        obs = produce_provider_usage_observations(record)
        assert len(obs) == 0


# ---------------------------------------------------------------------------
# provider_cost_usd
# ---------------------------------------------------------------------------


class TestProviderCostProducer:
    def test_produces_observation_from_valid_stage_and_price_table(self) -> None:
        record = _record(
            stages=[_stage(input_tokens=100, output_tokens=200, thinking_tokens=50, cache_tokens=10)],
            price_table=_price_table([_price_entry(
                input_price=0.001, output_price=0.002, thinking_price=0.003, cache_price=0.0005,
            )]),
        )
        obs = produce_provider_cost_observations(record)
        assert len(obs) == 1
        assert obs[0].metric_id == "provider_cost_usd"
        # 100*0.001 + 200*0.002 + 50*0.003 + 10*0.0005 = 0.1 + 0.4 + 0.15 + 0.005 = 0.655
        assert obs[0].value == 0.655
        assert obs[0].denominator_contribution == 1

    def test_sums_cost_across_multiple_stages(self) -> None:
        record = _record(
            stages=[
                _stage(stage_id="s1", input_tokens=100, output_tokens=0),
                _stage(stage_id="s2", input_tokens=0, output_tokens=100),
            ],
            price_table=_price_table([_price_entry(input_price=0.01, output_price=0.02)]),
        )
        obs = produce_provider_cost_observations(record)
        assert len(obs) == 1
        # 100*0.01 + 100*0.02 = 1.0 + 2.0 = 3.0
        assert obs[0].value == 3.0

    def test_no_price_table_produces_no_observations(self) -> None:
        record = _record(
            stages=[_stage(input_tokens=10, output_tokens=20)],
            price_table=None,
        )
        obs = produce_provider_cost_observations(record)
        assert len(obs) == 0

    def test_missing_price_entry_raises_typed_error(self) -> None:
        record = _record(
            stages=[_stage(provider="other-provider", model="other-model", input_tokens=10, output_tokens=20)],
            price_table=_price_table([_price_entry(provider="test-provider", model="test-model")]),
        )
        with pytest.raises(TelemetryProducerError, match="no price table entry"):
            produce_provider_cost_observations(record)

    def test_duplicate_price_entries_raises_typed_error(self) -> None:
        record = _record(
            stages=[_stage(input_tokens=10, output_tokens=20)],
            price_table=_price_table([
                _price_entry(input_price=0.001),
                _price_entry(input_price=0.002),
            ]),
        )
        with pytest.raises(TelemetryProducerError, match="duplicate price table entry"):
            produce_provider_cost_observations(record)

    def test_missing_provider_model_binding_raises_typed_error(self) -> None:
        record = _record(
            stages=[_stage(provider="", model="", input_tokens=10, output_tokens=20)],
            price_table=_price_table(),
        )
        with pytest.raises(TelemetryProducerError, match="missing provider/model binding"):
            produce_provider_cost_observations(record)

    def test_estimated_usage_raises_typed_error(self) -> None:
        record = _record(
            stages=[_stage(usage_estimated=True, input_tokens=10, output_tokens=20)],
            price_table=_price_table(),
        )
        with pytest.raises(TelemetryProducerError, match="estimated usage"):
            produce_provider_cost_observations(record)

    def test_unreported_usage_with_tokens_raises_typed_error(self) -> None:
        record = _record(
            stages=[_stage(usage_reported=False, input_tokens=10, output_tokens=20)],
            price_table=_price_table(),
        )
        with pytest.raises(TelemetryProducerError, match="unreported usage"):
            produce_provider_cost_observations(record)

    def test_multiple_providers_models_in_price_table(self) -> None:
        record = _record(
            stages=[
                _stage(stage_id="s1", provider="p1", model="m1", input_tokens=100, output_tokens=0),
                _stage(stage_id="s2", provider="p2", model="m2", input_tokens=0, output_tokens=100),
            ],
            price_table=_price_table([
                _price_entry(provider="p1", model="m1", input_price=0.01, output_price=0.0),
                _price_entry(provider="p2", model="m2", input_price=0.0, output_price=0.02),
            ]),
        )
        obs = produce_provider_cost_observations(record)
        assert len(obs) == 1
        # 100*0.01 + 100*0.02 = 1.0 + 2.0 = 3.0
        assert obs[0].value == 3.0

    def test_no_stages_produces_no_observations(self) -> None:
        record = _record(stages=[], price_table=_price_table())
        obs = produce_provider_cost_observations(record)
        assert len(obs) == 0


# ---------------------------------------------------------------------------
# local_resource_peak_memory_bytes
# ---------------------------------------------------------------------------


class TestLocalResourcePeakMemoryProducer:
    def test_produces_observation_from_valid_record(self) -> None:
        record = _record(local_resource_observations=[_local_resource(peak_memory_bytes=2048)])
        obs = produce_local_resource_peak_memory_observations(record)
        assert len(obs) == 1
        assert obs[0].metric_id == "local_resource_peak_memory_bytes"
        assert obs[0].value == 2048.0

    def test_unverified_record_with_value_raises_typed_error(self) -> None:
        record = _record(local_resource_observations=[
            _local_resource(verification_status=VerificationStatus.PENDING, peak_memory_bytes=1024),
        ])
        with pytest.raises(TelemetryProducerError, match="unverified"):
            produce_local_resource_peak_memory_observations(record)

    def test_none_value_produces_no_observations(self) -> None:
        record = _record(local_resource_observations=[_local_resource(peak_memory_bytes=None)])
        obs = produce_local_resource_peak_memory_observations(record)
        assert len(obs) == 0

    def test_negative_peak_memory_raises_typed_error(self) -> None:
        record = _record(local_resource_observations=[_local_resource(peak_memory_bytes=-1)])
        with pytest.raises(TelemetryProducerError, match="negative peak memory"):
            produce_local_resource_peak_memory_observations(record)

    def test_duplicate_observation_id_raises_typed_error(self) -> None:
        record = _record(local_resource_observations=[
            _local_resource(observation_id="lr-1", peak_memory_bytes=1024),
            _local_resource(observation_id="lr-1", peak_memory_bytes=4096),
        ])
        with pytest.raises(TelemetryProducerError, match="duplicate"):
            produce_local_resource_peak_memory_observations(record)

    def test_no_records_produces_no_observations(self) -> None:
        record = _record(local_resource_observations=[])
        obs = produce_local_resource_peak_memory_observations(record)
        assert len(obs) == 0

    def test_multiple_records_per_attempt_takes_max(self) -> None:
        record = _record(local_resource_observations=[
            _local_resource(observation_id="lr-1", peak_memory_bytes=1024),
            _local_resource(observation_id="lr-2", peak_memory_bytes=4096),
        ])
        obs = produce_local_resource_peak_memory_observations(record)
        assert len(obs) == 1
        assert obs[0].value == 4096.0


# ---------------------------------------------------------------------------
# local_resource_cpu_seconds
# ---------------------------------------------------------------------------


class TestLocalResourceCpuSecondsProducer:
    def test_produces_observation_from_valid_record(self) -> None:
        record = _record(local_resource_observations=[_local_resource(cpu_seconds=1.5)])
        obs = produce_local_resource_cpu_seconds_observations(record)
        assert len(obs) == 1
        assert obs[0].metric_id == "local_resource_cpu_seconds"
        assert obs[0].value == 1.5

    def test_unverified_record_with_value_raises_typed_error(self) -> None:
        record = _record(local_resource_observations=[
            _local_resource(verification_status=VerificationStatus.PENDING, cpu_seconds=1.0),
        ])
        with pytest.raises(TelemetryProducerError, match="unverified"):
            produce_local_resource_cpu_seconds_observations(record)

    def test_none_value_produces_no_observations(self) -> None:
        record = _record(local_resource_observations=[_local_resource(cpu_seconds=None)])
        obs = produce_local_resource_cpu_seconds_observations(record)
        assert len(obs) == 0

    def test_negative_cpu_seconds_raises_typed_error(self) -> None:
        record = _record(local_resource_observations=[_local_resource(cpu_seconds=-1.0)])
        with pytest.raises(TelemetryProducerError, match="negative cpu seconds"):
            produce_local_resource_cpu_seconds_observations(record)

    def test_duplicate_observation_id_raises_typed_error(self) -> None:
        record = _record(local_resource_observations=[
            _local_resource(observation_id="lr-1", cpu_seconds=1.0),
            _local_resource(observation_id="lr-1", cpu_seconds=2.0),
        ])
        with pytest.raises(TelemetryProducerError, match="duplicate"):
            produce_local_resource_cpu_seconds_observations(record)

    def test_multiple_records_per_attempt_sums(self) -> None:
        record = _record(local_resource_observations=[
            _local_resource(observation_id="lr-1", cpu_seconds=1.0, peak_memory_bytes=None),
            _local_resource(observation_id="lr-2", cpu_seconds=2.5, peak_memory_bytes=None),
        ])
        obs = produce_local_resource_cpu_seconds_observations(record)
        assert len(obs) == 1
        assert obs[0].value == 3.5


# ---------------------------------------------------------------------------
# human_wait_seconds
# ---------------------------------------------------------------------------


class TestHumanWaitProducer:
    def test_produces_observation_from_valid_record(self) -> None:
        record = _record(human_wait_observations=[_human_wait(human_wait_seconds=5.0)])
        obs = produce_human_wait_observations(record)
        assert len(obs) == 1
        assert obs[0].metric_id == "human_wait_seconds"
        assert obs[0].value == 5.0

    def test_unverified_record_with_value_raises_typed_error(self) -> None:
        record = _record(human_wait_observations=[
            _human_wait(verification_status=VerificationStatus.PENDING, human_wait_seconds=3.0),
        ])
        with pytest.raises(TelemetryProducerError, match="unverified"):
            produce_human_wait_observations(record)

    def test_none_value_produces_no_observations(self) -> None:
        record = _record(human_wait_observations=[_human_wait(human_wait_seconds=None)])
        obs = produce_human_wait_observations(record)
        assert len(obs) == 0

    def test_negative_wait_raises_typed_error(self) -> None:
        record = _record(human_wait_observations=[_human_wait(human_wait_seconds=-1.0)])
        with pytest.raises(TelemetryProducerError, match="negative human wait"):
            produce_human_wait_observations(record)

    def test_duplicate_observation_id_raises_typed_error(self) -> None:
        record = _record(human_wait_observations=[
            _human_wait(observation_id="hw-1", human_wait_seconds=3.0),
            _human_wait(observation_id="hw-1", human_wait_seconds=10.0),
        ])
        with pytest.raises(TelemetryProducerError, match="duplicate"):
            produce_human_wait_observations(record)

    def test_multiple_attempts_produce_separate_observations(self) -> None:
        attempts = [_attempt("a1"), _attempt("a2")]
        records = [
            _human_wait(observation_id="hw-1", attempt_id="a1", human_wait_seconds=3.0),
            _human_wait(observation_id="hw-2", attempt_id="a2", human_wait_seconds=7.0),
        ]
        record = _record(attempts=attempts, human_wait_observations=records)
        obs = produce_human_wait_observations(record)
        assert len(obs) == 2
        by_attempt = {o.attempt_id: o for o in obs}
        assert by_attempt["a1"].value == 3.0
        assert by_attempt["a2"].value == 7.0

    def test_no_records_produces_no_observations(self) -> None:
        record = _record(human_wait_observations=[])
        obs = produce_human_wait_observations(record)
        assert len(obs) == 0


# ---------------------------------------------------------------------------
# Genuine absence vs malformed disposition tests
# ---------------------------------------------------------------------------


class TestGenuineAbsenceVsMalformed:
    """One test for each disposition for every telemetry family."""

    # stage_latency_seconds
    def test_stage_latency_genuine_absence_no_stages(self) -> None:
        record = _record(stages=[])
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 0

    def test_stage_latency_malformed_incomplete_timing(self) -> None:
        record = _record(stages=[_stage(monotonic_start=100.0, monotonic_end=None)])
        with pytest.raises(TelemetryProducerError, match="incomplete timing pair"):
            produce_stage_latency_observations(record)

    # provider_usage_tokens
    def test_provider_usage_genuine_absence_no_stages(self) -> None:
        record = _record(stages=[])
        obs = produce_provider_usage_observations(record)
        assert len(obs) == 0

    def test_provider_usage_malformed_estimated_usage(self) -> None:
        record = _record(stages=[_stage(usage_estimated=True, input_tokens=10)])
        with pytest.raises(TelemetryProducerError, match="estimated usage"):
            produce_provider_usage_observations(record)

    # provider_cost_usd
    def test_provider_cost_genuine_absence_no_price_table(self) -> None:
        record = _record(stages=[_stage(input_tokens=10)], price_table=None)
        obs = produce_provider_cost_observations(record)
        assert len(obs) == 0

    def test_provider_cost_malformed_missing_price_entry(self) -> None:
        record = _record(
            stages=[_stage(provider="other", model="other", input_tokens=10)],
            price_table=_price_table([_price_entry(provider="test-provider", model="test-model")]),
        )
        with pytest.raises(TelemetryProducerError, match="no price table entry"):
            produce_provider_cost_observations(record)

    # local_resource_peak_memory_bytes
    def test_peak_memory_genuine_absence_no_records(self) -> None:
        record = _record(local_resource_observations=[])
        obs = produce_local_resource_peak_memory_observations(record)
        assert len(obs) == 0

    def test_peak_memory_malformed_unverified(self) -> None:
        record = _record(local_resource_observations=[
            _local_resource(verification_status=VerificationStatus.PENDING, peak_memory_bytes=1024),
        ])
        with pytest.raises(TelemetryProducerError, match="unverified"):
            produce_local_resource_peak_memory_observations(record)

    # local_resource_cpu_seconds
    def test_cpu_seconds_genuine_absence_no_records(self) -> None:
        record = _record(local_resource_observations=[])
        obs = produce_local_resource_cpu_seconds_observations(record)
        assert len(obs) == 0

    def test_cpu_seconds_malformed_unverified(self) -> None:
        record = _record(local_resource_observations=[
            _local_resource(verification_status=VerificationStatus.PENDING, cpu_seconds=1.0),
        ])
        with pytest.raises(TelemetryProducerError, match="unverified"):
            produce_local_resource_cpu_seconds_observations(record)

    # human_wait_seconds
    def test_human_wait_genuine_absence_no_records(self) -> None:
        record = _record(human_wait_observations=[])
        obs = produce_human_wait_observations(record)
        assert len(obs) == 0

    def test_human_wait_malformed_unverified(self) -> None:
        record = _record(human_wait_observations=[
            _human_wait(verification_status=VerificationStatus.PENDING, human_wait_seconds=3.0),
        ])
        with pytest.raises(TelemetryProducerError, match="unverified"):
            produce_human_wait_observations(record)


# ---------------------------------------------------------------------------
# Mixed-attempt tests: one malformed attempt fails closed without dropping another
# ---------------------------------------------------------------------------


class TestMixedAttemptFailClosed:
    def test_malformed_stage_timing_fails_closed_identifying_attempt(self) -> None:
        attempts = [_attempt("a1"), _attempt("a2")]
        stages = [
            _stage(stage_id="s1", attempt_id="a1", monotonic_start=0.0, monotonic_end=100.0),
            _stage(stage_id="s2", attempt_id="a2", monotonic_start=100.0, monotonic_end=None),
        ]
        record = _record(attempts=attempts, stages=stages)
        with pytest.raises(TelemetryProducerError, match="s2"):
            produce_stage_latency_observations(record)

    def test_malformed_usage_fails_closed_identifying_attempt(self) -> None:
        attempts = [_attempt("a1"), _attempt("a2")]
        stages = [
            _stage(stage_id="s1", attempt_id="a1", input_tokens=10, output_tokens=20),
            _stage(stage_id="s2", attempt_id="a2", usage_estimated=True, input_tokens=5, output_tokens=5),
        ]
        record = _record(attempts=attempts, stages=stages)
        with pytest.raises(TelemetryProducerError, match="s2"):
            produce_provider_usage_observations(record)

    def test_malformed_human_wait_fails_closed_identifying_attempt(self) -> None:
        attempts = [_attempt("a1"), _attempt("a2")]
        records = [
            _human_wait(observation_id="hw-1", attempt_id="a1", human_wait_seconds=3.0),
            _human_wait(observation_id="hw-2", attempt_id="a2", human_wait_seconds=-1.0),
        ]
        record = _record(attempts=attempts, human_wait_observations=records)
        with pytest.raises(TelemetryProducerError, match="negative"):
            produce_human_wait_observations(record)

    def test_malformed_local_resource_fails_closed_identifying_attempt(self) -> None:
        attempts = [_attempt("a1"), _attempt("a2")]
        records = [
            _local_resource(observation_id="lr-1", attempt_id="a1", peak_memory_bytes=1024, cpu_seconds=None),
            _local_resource(observation_id="lr-2", attempt_id="a2", peak_memory_bytes=-1, cpu_seconds=None),
        ]
        record = _record(attempts=attempts, local_resource_observations=records)
        with pytest.raises(TelemetryProducerError, match="negative"):
            produce_local_resource_peak_memory_observations(record)


# ---------------------------------------------------------------------------
# Denominator/missingness tests for all terminal statuses
# ---------------------------------------------------------------------------


class TestDenominatorMissingnessByTerminalStatus:
    """Verify that attempts with all terminal statuses are eligible for
    telemetry metrics when source records exist, and missing when they don't.
    """

    @pytest.mark.parametrize("status", [
        TerminalStatus.COMPLETED,
        TerminalStatus.MODEL_FAILED,
        TerminalStatus.GOVERNANCE_REJECTED,
        TerminalStatus.HUMAN_DENIED,
        TerminalStatus.TIMED_OUT,
        TerminalStatus.INFRASTRUCTURE_FAILED,
        TerminalStatus.INVALID_EVIDENCE,
    ])
    def test_stage_latency_produces_observation_for_any_terminal_status(self, status: TerminalStatus) -> None:
        record = _record(
            attempts=[_attempt(terminal_status=status)],
            stages=[_stage(monotonic_start=0.0, monotonic_end=100.0)],
        )
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 1
        assert obs[0].value == 100.0

    @pytest.mark.parametrize("status", [
        TerminalStatus.COMPLETED,
        TerminalStatus.MODEL_FAILED,
        TerminalStatus.GOVERNANCE_REJECTED,
        TerminalStatus.HUMAN_DENIED,
        TerminalStatus.TIMED_OUT,
        TerminalStatus.INFRASTRUCTURE_FAILED,
        TerminalStatus.INVALID_EVIDENCE,
    ])
    def test_stage_latency_missing_when_no_stages_for_any_status(self, status: TerminalStatus) -> None:
        record = _record(
            attempts=[_attempt(terminal_status=status)],
            stages=[],
        )
        obs = produce_stage_latency_observations(record)
        assert len(obs) == 0

    @pytest.mark.parametrize("status", [
        TerminalStatus.COMPLETED,
        TerminalStatus.MODEL_FAILED,
        TerminalStatus.GOVERNANCE_REJECTED,
        TerminalStatus.HUMAN_DENIED,
        TerminalStatus.TIMED_OUT,
        TerminalStatus.INFRASTRUCTURE_FAILED,
        TerminalStatus.INVALID_EVIDENCE,
    ])
    def test_human_wait_produces_observation_for_any_terminal_status(self, status: TerminalStatus) -> None:
        record = _record(
            attempts=[_attempt(terminal_status=status)],
            human_wait_observations=[_human_wait(human_wait_seconds=5.0)],
        )
        obs = produce_human_wait_observations(record)
        assert len(obs) == 1
        assert obs[0].value == 5.0

    @pytest.mark.parametrize("status", [
        TerminalStatus.COMPLETED,
        TerminalStatus.MODEL_FAILED,
        TerminalStatus.GOVERNANCE_REJECTED,
        TerminalStatus.HUMAN_DENIED,
        TerminalStatus.TIMED_OUT,
        TerminalStatus.INFRASTRUCTURE_FAILED,
        TerminalStatus.INVALID_EVIDENCE,
    ])
    def test_human_wait_missing_when_no_records_for_any_status(self, status: TerminalStatus) -> None:
        record = _record(
            attempts=[_attempt(terminal_status=status)],
            human_wait_observations=[],
        )
        obs = produce_human_wait_observations(record)
        assert len(obs) == 0


# ---------------------------------------------------------------------------
# Wiring tests: run_all_telemetry_producers and collision rejection
# ---------------------------------------------------------------------------


class TestRunAllTelemetryProducers:
    def test_produces_observations_for_all_six_metrics_when_evidence_exists(self) -> None:
        record = _record(
            stages=[_stage(input_tokens=100, output_tokens=200, monotonic_start=0.0, monotonic_end=10.0)],
            local_resource_observations=[_local_resource(peak_memory_bytes=1024, cpu_seconds=1.5)],
            human_wait_observations=[_human_wait(human_wait_seconds=5.0)],
            price_table=_price_table(),
        )
        produced = run_all_telemetry_producers(record, [])
        metric_ids = {obs.metric_id for obs in produced}
        assert metric_ids == {
            "stage_latency_seconds",
            "provider_usage_tokens",
            "provider_cost_usd",
            "local_resource_peak_memory_bytes",
            "local_resource_cpu_seconds",
            "human_wait_seconds",
        }

    def test_merges_produced_with_caller_supplied(self) -> None:
        from g8e_evals.schema import GraderClass, MetricObservation

        record = _record(
            stages=[_stage(input_tokens=100, output_tokens=200, monotonic_start=0.0, monotonic_end=10.0)],
            price_table=_price_table(),
        )
        # Caller supplies a non-telemetry observation
        caller_obs = MetricObservation(
            metric_id="receipt_integrity",
            metric_version="1.0.0",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            arm_id=Arm.DOCTRINE,
            task_id=_TASK_ID,
            value=1.0,
            unit="pass/fail",
            eligible=True,
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
        )
        merged = run_all_telemetry_producers(record, [caller_obs])
        metric_ids = {obs.metric_id for obs in merged}
        assert "receipt_integrity" in metric_ids
        assert "stage_latency_seconds" in metric_ids

    def test_collision_between_produced_and_caller_supplied_raises(self) -> None:
        from g8e_evals.schema import GraderClass, MetricObservation

        record = _record(
            stages=[_stage(input_tokens=100, output_tokens=200, monotonic_start=0.0, monotonic_end=10.0)],
            price_table=_price_table(),
        )
        # Caller supplies a telemetry observation that the producer will also produce
        caller_obs = MetricObservation(
            metric_id="stage_latency_seconds",
            metric_version="1.0.0",
            attempt_id=_ATTEMPT_ID,
            run_id=_RUN_ID,
            arm_id=Arm.DOCTRINE,
            task_id=_TASK_ID,
            value=999.0,
            unit="seconds",
            eligible=True,
            verification_status=VerificationStatus.VERIFIED,
            grader_class=GraderClass.DETERMINISTIC,
        )
        with pytest.raises(TelemetryObservationCollisionError, match="collision"):
            run_all_telemetry_producers(record, [caller_obs])

    def test_no_evidence_produces_no_observations(self) -> None:
        record = _record()
        produced = run_all_telemetry_producers(record, [])
        assert len(produced) == 0
