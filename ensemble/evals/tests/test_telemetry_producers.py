# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 unit tests for primary telemetry metric producers.

Verifies that telemetry ``MetricObservation`` records are produced from
verified immutable source records (``StageObservation``,
``LocalResourceObservation``, ``HumanWaitObservation``) and that invalid
inputs are rejected fail-closed. No external dependencies (no files,
network, or DB).
"""

from __future__ import annotations

from datetime import UTC, datetime

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.analysis.telemetry import (
    produce_human_wait_observations,
    produce_local_resource_observations,
    produce_provider_usage_observations,
    produce_stage_latency_observations,
)
from g8e_evals.arms import Arm
from g8e_evals.schema import (
    AttemptRecord,
    HumanWaitObservation,
    LocalResourceObservation,
    StageKind,
    StageObservation,
    TerminalStatus,
    VerificationStatus,
)


_RUN_ID = "run-tel-1"
_TASK_ID = "task-tel-1"
_ATTEMPT_ID = "attempt-tel-1"
_TS = datetime(2026, 1, 1, tzinfo=UTC)
_HASH = "a" * 64


def _attempt(attempt_id: str = _ATTEMPT_ID, arm_id: Arm = Arm.DOCTRINE) -> AttemptRecord:
    return AttemptRecord(
        attempt_id=attempt_id,
        run_id=_RUN_ID,
        task_id=_TASK_ID,
        arm_id=arm_id,
        terminal_status=TerminalStatus.COMPLETED,
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


# ---------------------------------------------------------------------------
# Stage latency
# ---------------------------------------------------------------------------


class TestStageLatencyProducer:
    def test_produces_observation_from_valid_stage(self) -> None:
        stages = [_stage(monotonic_start=100.0, monotonic_end=250.0)]
        obs = produce_stage_latency_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 1
        assert obs[0].metric_id == "stage_latency_seconds"
        assert obs[0].value == 150.0
        assert obs[0].eligible is True
        assert obs[0].denominator_contribution == 1
        assert obs[0].verification_status == VerificationStatus.VERIFIED

    def test_mean_latency_across_multiple_stages_for_one_attempt(self) -> None:
        stages = [
            _stage(stage_id="s1", monotonic_start=0.0, monotonic_end=100.0),
            _stage(stage_id="s2", monotonic_start=100.0, monotonic_end=400.0),
        ]
        obs = produce_stage_latency_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 1
        assert obs[0].value == 200.0  # mean of 100 and 300

    def test_rejects_incomplete_timing_pair(self) -> None:
        stages = [_stage(monotonic_start=None, monotonic_end=200.0)]
        obs = produce_stage_latency_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_negative_duration(self) -> None:
        stages = [_stage(monotonic_start=300.0, monotonic_end=100.0)]
        obs = produce_stage_latency_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_mismatched_clock_domains(self) -> None:
        stages = [
            _stage(stage_id="s1", monotonic_start=0.0, monotonic_end=100.0, clock_domain="monotonic"),
            _stage(stage_id="s2", monotonic_start=100.0, monotonic_end=200.0, clock_domain="wall_clock"),
        ]
        obs = produce_stage_latency_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_stages_from_different_attempts_produce_separate_observations(self) -> None:
        attempts = [_attempt("a1"), _attempt("a2")]
        stages = [
            _stage(stage_id="s1", attempt_id="a1", monotonic_start=0.0, monotonic_end=100.0),
            _stage(stage_id="s2", attempt_id="a2", monotonic_start=0.0, monotonic_end=200.0),
        ]
        obs = produce_stage_latency_observations(stages, attempts, _RUN_ID)
        assert len(obs) == 2
        by_attempt = {o.attempt_id: o for o in obs}
        assert by_attempt["a1"].value == 100.0
        assert by_attempt["a2"].value == 200.0

    def test_rejects_cross_run_stage(self) -> None:
        stages = [_stage(attempt_id=_ATTEMPT_ID)]
        stages[0] = stages[0].model_copy(update={"run_id": "other-run"})
        obs = produce_stage_latency_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_stage_for_unknown_attempt(self) -> None:
        stages = [_stage(attempt_id="unknown-attempt")]
        obs = produce_stage_latency_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_empty_stages_produces_no_observations(self) -> None:
        obs = produce_stage_latency_observations([], [_attempt()], _RUN_ID)
        assert len(obs) == 0


# ---------------------------------------------------------------------------
# Provider usage tokens
# ---------------------------------------------------------------------------


class TestProviderUsageProducer:
    def test_produces_observation_from_valid_stage(self) -> None:
        stages = [_stage(input_tokens=10, output_tokens=20, thinking_tokens=5, cache_tokens=3)]
        obs = produce_provider_usage_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 1
        assert obs[0].metric_id == "provider_usage_tokens"
        assert obs[0].value == 38.0
        assert obs[0].denominator_contribution == 1

    def test_sums_tokens_across_multiple_stages_for_one_attempt(self) -> None:
        stages = [
            _stage(stage_id="s1", input_tokens=10, output_tokens=20),
            _stage(stage_id="s2", input_tokens=5, output_tokens=15),
        ]
        obs = produce_provider_usage_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 1
        assert obs[0].value == 50.0

    def test_rejects_unreported_usage(self) -> None:
        stages = [_stage(usage_reported=False, input_tokens=10, output_tokens=20)]
        obs = produce_provider_usage_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_estimated_usage(self) -> None:
        stages = [_stage(usage_reported=True, input_tokens=10, output_tokens=20)]
        stages[0] = stages[0].model_copy(update={"usage_estimated": True})
        obs = produce_provider_usage_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_stage_with_no_token_fields(self) -> None:
        stages = [_stage(input_tokens=None, output_tokens=None, thinking_tokens=None, cache_tokens=None)]
        obs = produce_provider_usage_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_cross_run_stage(self) -> None:
        stages = [_stage()]
        stages[0] = stages[0].model_copy(update={"run_id": "other-run"})
        obs = produce_provider_usage_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_stage_for_unknown_attempt(self) -> None:
        stages = [_stage(attempt_id="unknown-attempt")]
        obs = produce_provider_usage_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_only_counts_model_inference_stages(self) -> None:
        stages = [
            _stage(stage_id="s1", kind=StageKind.MODEL_INFERENCE, input_tokens=10, output_tokens=20),
            _stage(stage_id="s2", kind=StageKind.GRADING, input_tokens=100, output_tokens=200),
        ]
        obs = produce_provider_usage_observations(stages, [_attempt()], _RUN_ID)
        assert len(obs) == 1
        assert obs[0].value == 30.0  # only the model-inference stage


# ---------------------------------------------------------------------------
# Local resource peak memory
# ---------------------------------------------------------------------------


class TestLocalResourcePeakMemoryProducer:
    def test_produces_observation_from_valid_record(self) -> None:
        records = [_local_resource(peak_memory_bytes=2048)]
        obs = produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_peak_memory_bytes")
        assert len(obs) == 1
        assert obs[0].metric_id == "local_resource_peak_memory_bytes"
        assert obs[0].value == 2048.0

    def test_rejects_unverified_record(self) -> None:
        records = [_local_resource(verification_status=VerificationStatus.PENDING)]
        obs = produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_peak_memory_bytes")
        assert len(obs) == 0

    def test_rejects_none_value(self) -> None:
        records = [_local_resource(peak_memory_bytes=None)]
        obs = produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_peak_memory_bytes")
        assert len(obs) == 0

    def test_rejects_cross_run_record(self) -> None:
        records = [_local_resource()]
        records[0] = records[0].model_copy(update={"run_id": "other-run"})
        obs = produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_peak_memory_bytes")
        assert len(obs) == 0

    def test_rejects_record_for_unknown_attempt(self) -> None:
        records = [_local_resource(attempt_id="unknown-attempt")]
        obs = produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_peak_memory_bytes")
        assert len(obs) == 0

    def test_rejects_duplicate_observation_id(self) -> None:
        records = [_local_resource(observation_id="lr-1"), _local_resource(observation_id="lr-1", peak_memory_bytes=4096)]
        with pytest.raises(ValueError, match="duplicate"):
            produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_peak_memory_bytes")


# ---------------------------------------------------------------------------
# Local resource CPU seconds
# ---------------------------------------------------------------------------


class TestLocalResourceCpuSecondsProducer:
    def test_produces_observation_from_valid_record(self) -> None:
        records = [_local_resource(cpu_seconds=1.5)]
        obs = produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_cpu_seconds")
        assert len(obs) == 1
        assert obs[0].metric_id == "local_resource_cpu_seconds"
        assert obs[0].value == 1.5

    def test_rejects_none_value(self) -> None:
        records = [_local_resource(cpu_seconds=None)]
        obs = produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_cpu_seconds")
        assert len(obs) == 0

    def test_rejects_negative_cpu_seconds(self) -> None:
        records = [_local_resource(cpu_seconds=-1.0)]
        with pytest.raises(ValueError, match="negative"):
            produce_local_resource_observations(records, [_attempt()], _RUN_ID, metric_id="local_resource_cpu_seconds")


# ---------------------------------------------------------------------------
# Human wait
# ---------------------------------------------------------------------------


class TestHumanWaitProducer:
    def test_produces_observation_from_valid_record(self) -> None:
        records = [_human_wait(human_wait_seconds=5.0)]
        obs = produce_human_wait_observations(records, [_attempt()], _RUN_ID)
        assert len(obs) == 1
        assert obs[0].metric_id == "human_wait_seconds"
        assert obs[0].value == 5.0

    def test_rejects_unverified_record(self) -> None:
        records = [_human_wait(verification_status=VerificationStatus.PENDING)]
        obs = produce_human_wait_observations(records, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_none_value(self) -> None:
        records = [_human_wait(human_wait_seconds=None)]
        obs = produce_human_wait_observations(records, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_negative_wait(self) -> None:
        records = [_human_wait(human_wait_seconds=-1.0)]
        with pytest.raises(ValueError, match="negative"):
            produce_human_wait_observations(records, [_attempt()], _RUN_ID)

    def test_rejects_cross_run_record(self) -> None:
        records = [_human_wait()]
        records[0] = records[0].model_copy(update={"run_id": "other-run"})
        obs = produce_human_wait_observations(records, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_record_for_unknown_attempt(self) -> None:
        records = [_human_wait(attempt_id="unknown-attempt")]
        obs = produce_human_wait_observations(records, [_attempt()], _RUN_ID)
        assert len(obs) == 0

    def test_rejects_duplicate_observation_id(self) -> None:
        records = [_human_wait(observation_id="hw-1"), _human_wait(observation_id="hw-1", human_wait_seconds=10.0)]
        with pytest.raises(ValueError, match="duplicate"):
            produce_human_wait_observations(records, [_attempt()], _RUN_ID)

    def test_multiple_attempts_produce_separate_observations(self) -> None:
        attempts = [_attempt("a1"), _attempt("a2")]
        records = [
            _human_wait(observation_id="hw-1", attempt_id="a1", human_wait_seconds=3.0),
            _human_wait(observation_id="hw-2", attempt_id="a2", human_wait_seconds=7.0),
        ]
        obs = produce_human_wait_observations(records, attempts, _RUN_ID)
        assert len(obs) == 2
        by_attempt = {o.attempt_id: o for o in obs}
        assert by_attempt["a1"].value == 3.0
        assert by_attempt["a2"].value == 7.0
