# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for ResourceObservation missingness declarations (D26).

The bounded live smoke stopped on a ``_build_resource_observation``
ValidationError: ``InferenceObservation`` records with ``None`` timing
fields (failed calls, unreported native durations, non-streaming
boundaries) produced ``ResourceObservation`` records whose ``None``
measurement fields had no matching ``unavailable_measurements`` entry.
The schema requires an exact bijection between ``None`` measurement
fields and typed unavailable entries.

These tests pin the producer contract:

- Every ``None`` field on the resulting ``ResourceObservation`` has a
  typed ``unavailable_measurements`` entry.
- Conditional entries are emitted for ``provider_call_latency_seconds``,
  ``time_to_first_token_seconds``, ``generation_duration_seconds``, and
  ``output_throughput_tokens_per_second`` whenever the observation
  carries ``None``.
- Measured fields — including measured zero — never get an entry.

No filesystem, process, network, database, or provider dependencies.
"""

from __future__ import annotations

import pytest

pytestmark = pytest.mark.unit

from g8e_evals.harness import InferenceObservation
from g8e_evals.index import (
    _RESOURCE_MEASUREMENT_FIELDS,
    MeasurementAvailability,
    MeasurementScope,
    ResourceObservation,
)
from g8e_evals.runner import _ORCHESTRATOR_SCOPE, _build_resource_observation


def _build(obs: InferenceObservation) -> ResourceObservation:
    return _build_resource_observation(
        obs=obs,
        campaign_id="campaign-1",
        child_id="campaign-1",
        run_id="run-1",
        assignment_id="assignment-1",
        attempt_id="attempt-1",
        task_id="task-1",
        stage_id="stage-1",
        orchestrator_scope=_ORCHESTRATOR_SCOPE,
    )


def _none_fields(ro: ResourceObservation) -> set[str]:
    return {
        name for name in _RESOURCE_MEASUREMENT_FIELDS if getattr(ro, name) is None
    }


def _unavailable_fields(ro: ResourceObservation) -> set[str]:
    return {um.field_name for um in ro.unavailable_measurements}


class TestUnavailableBijection:
    """Every None measurement field has exactly one typed unavailable entry."""

    def test_failed_call_declares_every_none_timing_field(self):
        """A failed provider call (error, all timing/usage fields None)
        produces a valid ResourceObservation with typed unavailable entries
        for latency, TTFT, generation duration, and throughput."""
        obs = InferenceObservation(
            inference_id="inf-1",
            role="primary",
            model_variant_id="qwen3:8b",
            provider="ollama",
            model="qwen3:8b",
            error="connection refused",
        )
        ro = _build(obs)

        assert ro.provider_call_latency_seconds is None
        assert ro.time_to_first_token_seconds is None
        assert ro.generation_duration_seconds is None
        assert ro.output_throughput_tokens_per_second is None
        assert ro.hidden_reasoning_throughput_tokens_per_second is None

        unavailable = _unavailable_fields(ro)
        for field_name in (
            "provider_call_latency_seconds",
            "time_to_first_token_seconds",
            "generation_duration_seconds",
            "output_throughput_tokens_per_second",
            "hidden_reasoning_throughput_tokens_per_second",
        ):
            assert field_name in unavailable, (
                f"{field_name} is None but has no unavailable_measurements entry"
            )
        # Exact bijection: no entry without a None field, no None field
        # without an entry.
        assert unavailable == _none_fields(ro)

    def test_partially_measured_observation_declares_only_none_fields(self):
        """When latency and throughput are measured but TTFT and
        generation duration are not, only the None fields get entries."""
        obs = InferenceObservation(
            inference_id="inf-1",
            role="primary",
            model_variant_id="qwen3:8b",
            provider="ollama",
            model="qwen3:8b",
            provider_call_latency_seconds=0.5,
            output_throughput_tokens_per_second=20.0,
        )
        ro = _build(obs)

        unavailable = _unavailable_fields(ro)
        assert "provider_call_latency_seconds" not in unavailable
        assert "output_throughput_tokens_per_second" not in unavailable
        assert "time_to_first_token_seconds" in unavailable
        assert "generation_duration_seconds" in unavailable
        assert unavailable == _none_fields(ro)

    def test_measured_zero_is_not_declared_unavailable(self):
        """A measured zero (0.0) is a measurement, not missingness."""
        obs = InferenceObservation(
            inference_id="inf-1",
            role="primary",
            model_variant_id="qwen3:8b",
            provider="ollama",
            model="qwen3:8b",
            provider_call_latency_seconds=0.5,
            time_to_first_token_seconds=0.0,
            generation_duration_seconds=0.0,
            output_throughput_tokens_per_second=0.0,
        )
        ro = _build(obs)

        assert ro.time_to_first_token_seconds == 0.0
        assert ro.generation_duration_seconds == 0.0
        assert ro.output_throughput_tokens_per_second == 0.0
        unavailable = _unavailable_fields(ro)
        assert "time_to_first_token_seconds" not in unavailable
        assert "generation_duration_seconds" not in unavailable
        assert "output_throughput_tokens_per_second" not in unavailable
        assert unavailable == _none_fields(ro)

    def test_fully_measured_observation_declares_only_static_unavailables(self):
        """A fully measured warm-inference observation declares only the
        structurally unavailable fields (GPU, local-process metrics)."""
        obs = InferenceObservation(
            inference_id="inf-1",
            role="primary",
            model_variant_id="qwen3:8b",
            provider="ollama",
            model="qwen3:8b",
            provider_call_latency_seconds=2.5,
            time_to_first_token_seconds=0.4,
            generation_duration_seconds=2.1,
            output_throughput_tokens_per_second=23.8,
            hidden_reasoning_throughput_tokens_per_second=8.0,
        )
        ro = _build(obs)

        unavailable = _unavailable_fields(ro)
        assert "provider_call_latency_seconds" not in unavailable
        assert "time_to_first_token_seconds" not in unavailable
        assert "generation_duration_seconds" not in unavailable
        assert "output_throughput_tokens_per_second" not in unavailable
        assert "hidden_reasoning_throughput_tokens_per_second" not in unavailable
        # GPU and local-process metrics remain declared
        for field_name in (
            "gpu_utilization_percent",
            "gpu_temperature_celsius",
            "gpu_power_draw_watts",
            "gpu_clock_mhz",
            "model_load_time_seconds",
            "peak_resident_memory_bytes",
        ):
            assert field_name in unavailable
        assert unavailable == _none_fields(ro)


class TestUnavailableEntryShape:
    """Conditional entries carry typed availability, scope, and reason."""

    def test_latency_and_ttft_entries_scope_to_orchestrator(self):
        """Latency and TTFT are orchestrator-side monotonic measurements;
        when unmeasured the unavailable entry scopes orchestrator-local."""
        obs = InferenceObservation(
            inference_id="inf-1",
            role="primary",
            model_variant_id="qwen3:8b",
            provider="ollama",
            model="qwen3:8b",
            error="timeout",
        )
        ro = _build(obs)
        entries = {um.field_name: um for um in ro.unavailable_measurements}

        for field_name in (
            "provider_call_latency_seconds",
            "time_to_first_token_seconds",
        ):
            entry = entries[field_name]
            assert entry.availability == MeasurementAvailability.UNAVAILABLE
            assert entry.scope == MeasurementScope.ORCHESTRATOR_LOCAL
            assert entry.reason != ""

    def test_duration_and_throughput_entries_scope_to_provider(self):
        """Generation duration and output throughput derive from
        provider-reported values; when unreported the entry scopes
        provider-remote."""
        obs = InferenceObservation(
            inference_id="inf-1",
            role="primary",
            model_variant_id="qwen3:8b",
            provider="ollama",
            model="qwen3:8b",
            error="timeout",
        )
        ro = _build(obs)
        entries = {um.field_name: um for um in ro.unavailable_measurements}

        for field_name in (
            "generation_duration_seconds",
            "output_throughput_tokens_per_second",
        ):
            entry = entries[field_name]
            assert entry.availability == MeasurementAvailability.UNAVAILABLE
            assert entry.scope == MeasurementScope.PROVIDER_REMOTE
            assert entry.reason != ""
