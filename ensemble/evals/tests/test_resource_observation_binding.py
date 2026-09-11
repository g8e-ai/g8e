# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for resource observation binding.

Verifies that resource observations (latency, memory, energy) are bound
to run ID, model variant ID, task block, hardware identity, collection
tool/version, and source evidence hash. Unbound or partially bound
observations are rejected. Observations without evidence hashes are
rejected. VRAM is never inferred from parameter count or quantization
labels.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with missing required fields
# and unknown extra fields to verify pydantic validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.index import ResourceObservation, validate_resource_observations


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _make_resource_observation(
    *,
    run_id: str = "run-1",
    model_variant_id: str = "qwen3-8b-q4_0",
    task_block: str = "task-1",
    hardware_identity: str = "linux/amd64/rtx-4090",
    collection_tool: str = "psutil-5.9",
    source_evidence_hash: str = _VALID_HASH,
    model_load_time_seconds: float = 12.5,
    peak_resident_memory_bytes: int = 4_000_000_000,
    peak_accelerator_memory_bytes: int | None = 8_000_000_000,
    artifact_bytes: int = 4_800_000_000,
    measured_energy_joules: float | None = None,
    end_to_end_latency_seconds: float = 1.5,
    provider_call_latency_seconds: float = 1.2,
    output_throughput_tokens_per_second: float | None = None,
    time_to_first_token_seconds: float | None = None,
    generation_duration_seconds: float | None = None,
    accelerator_memory_before_bytes: int | None = None,
    gpu_utilization_percent: float | None = None,
    gpu_temperature_celsius: float | None = None,
    gpu_power_draw_watts: float | None = None,
    gpu_clock_mhz: float | None = None,
) -> ResourceObservation:
    return ResourceObservation(
        run_id=run_id,
        model_variant_id=model_variant_id,
        task_block=task_block,
        hardware_identity=hardware_identity,
        collection_tool=collection_tool,
        source_evidence_hash=source_evidence_hash,
        model_load_time_seconds=model_load_time_seconds,
        peak_resident_memory_bytes=peak_resident_memory_bytes,
        peak_accelerator_memory_bytes=peak_accelerator_memory_bytes,
        artifact_bytes=artifact_bytes,
        measured_energy_joules=measured_energy_joules,
        end_to_end_latency_seconds=end_to_end_latency_seconds,
        provider_call_latency_seconds=provider_call_latency_seconds,
        output_throughput_tokens_per_second=output_throughput_tokens_per_second,
        time_to_first_token_seconds=time_to_first_token_seconds,
        generation_duration_seconds=generation_duration_seconds,
        accelerator_memory_before_bytes=accelerator_memory_before_bytes,
        gpu_utilization_percent=gpu_utilization_percent,
        gpu_temperature_celsius=gpu_temperature_celsius,
        gpu_power_draw_watts=gpu_power_draw_watts,
        gpu_clock_mhz=gpu_clock_mhz,
    )


class TestResourceObservationModel:
    def test_round_trip_preserves_all_fields(self):
        obs = _make_resource_observation()
        restored = ResourceObservation.model_validate_json(obs.model_dump_json())
        assert restored == obs

    def test_rejects_unknown_fields(self):
        data = _make_resource_observation().model_dump()
        data["extra_field"] = "bad"
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_frozen_model(self):
        obs = _make_resource_observation()
        with pytest.raises(ValidationError):
            obs.run_id = "changed"  # type: ignore[misc]

    def test_requires_run_id(self):
        data = _make_resource_observation().model_dump()
        del data["run_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_model_variant_id(self):
        data = _make_resource_observation().model_dump()
        del data["model_variant_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_task_block(self):
        data = _make_resource_observation().model_dump()
        del data["task_block"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_hardware_identity(self):
        data = _make_resource_observation().model_dump()
        del data["hardware_identity"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_collection_tool(self):
        data = _make_resource_observation().model_dump()
        del data["collection_tool"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_source_evidence_hash(self):
        data = _make_resource_observation().model_dump()
        del data["source_evidence_hash"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_latency(self):
        data = _make_resource_observation().model_dump()
        data["end_to_end_latency_seconds"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_memory(self):
        data = _make_resource_observation().model_dump()
        data["peak_resident_memory_bytes"] = -1
        with pytest.raises(ValidationError):
            ResourceObservation(**data)


class TestResourceObservationValidation:
    def test_valid_observations_pass(self):
        """A list of valid resource observations passes validation."""
        observations = [
            _make_resource_observation(run_id="run-1", task_block="task-1"),
            _make_resource_observation(run_id="run-1", task_block="task-2"),
            _make_resource_observation(run_id="run-2", task_block="task-1"),
        ]
        validate_resource_observations(observations)

    def test_empty_observations_pass(self):
        """An empty list of observations passes."""
        validate_resource_observations([])

    def test_duplicate_run_task_variant_rejected(self):
        """Duplicate (run_id, task_block, model_variant_id) tuples are rejected."""
        observations = [
            _make_resource_observation(run_id="run-1", task_block="task-1", model_variant_id="v1"),
            _make_resource_observation(run_id="run-1", task_block="task-1", model_variant_id="v1"),
        ]
        with pytest.raises(ValueError, match=r"duplicate.*resource observation"):
            validate_resource_observations(observations)

    def test_same_run_task_different_variant_allowed(self):
        """The same run and task with different variants is allowed."""
        observations = [
            _make_resource_observation(run_id="run-1", task_block="task-1", model_variant_id="v1"),
            _make_resource_observation(run_id="run-1", task_block="task-1", model_variant_id="v2"),
        ]
        validate_resource_observations(observations)


class TestNoVramInference:
    def test_accelerator_memory_is_measured_not_inferred(self):
        """peak_accelerator_memory_bytes is a measured value, not inferred from
        parameter_count or quantization labels."""
        obs = _make_resource_observation(
            peak_accelerator_memory_bytes=8_000_000_000,
        )
        assert obs.peak_accelerator_memory_bytes == 8_000_000_000

    def test_accelerator_memory_none_when_unmeasured(self):
        """When accelerator memory is not measured, it is None, not inferred."""
        obs = _make_resource_observation(peak_accelerator_memory_bytes=None)
        assert obs.peak_accelerator_memory_bytes is None

    def test_energy_none_when_no_calibrated_source(self):
        """When no calibrated energy source exists, measured_energy_joules is None."""
        obs = _make_resource_observation(measured_energy_joules=None)
        assert obs.measured_energy_joules is None


class TestPerInferenceFields:
    """Verify the per-inference extension fields separate cold-start from warm
    inference and capture GPU metrics, all defaulting to None for backward
    compatibility."""

    def test_new_fields_default_to_none(self):
        """All per-inference extension fields default to None when omitted."""
        obs = _make_resource_observation()
        assert obs.time_to_first_token_seconds is None
        assert obs.generation_duration_seconds is None
        assert obs.accelerator_memory_before_bytes is None
        assert obs.gpu_utilization_percent is None
        assert obs.gpu_temperature_celsius is None
        assert obs.gpu_power_draw_watts is None
        assert obs.gpu_clock_mhz is None

    def test_round_trip_preserves_per_inference_fields(self):
        """Round-trip serialization preserves all per-inference fields."""
        obs = _make_resource_observation(
            time_to_first_token_seconds=0.45,
            generation_duration_seconds=1.1,
            accelerator_memory_before_bytes=2_000_000_000,
            gpu_utilization_percent=87.5,
            gpu_temperature_celsius=71.0,
            gpu_power_draw_watts=280.0,
            gpu_clock_mhz=2520.0,
        )
        restored = ResourceObservation.model_validate_json(obs.model_dump_json())
        assert restored == obs
        assert restored.time_to_first_token_seconds == 0.45
        assert restored.generation_duration_seconds == 1.1
        assert restored.accelerator_memory_before_bytes == 2_000_000_000
        assert restored.gpu_utilization_percent == 87.5
        assert restored.gpu_temperature_celsius == 71.0
        assert restored.gpu_power_draw_watts == 280.0
        assert restored.gpu_clock_mhz == 2520.0

    def test_rejects_negative_time_to_first_token(self):
        data = _make_resource_observation().model_dump()
        data["time_to_first_token_seconds"] = -0.1
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_generation_duration(self):
        data = _make_resource_observation().model_dump()
        data["generation_duration_seconds"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_accelerator_memory_before(self):
        data = _make_resource_observation().model_dump()
        data["accelerator_memory_before_bytes"] = -1
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_gpu_utilization(self):
        data = _make_resource_observation().model_dump()
        data["gpu_utilization_percent"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_gpu_power_draw(self):
        data = _make_resource_observation().model_dump()
        data["gpu_power_draw_watts"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_gpu_clock(self):
        data = _make_resource_observation().model_dump()
        data["gpu_clock_mhz"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_allows_negative_gpu_temperature(self):
        """GPU temperature can be negative (sub-ambient cooling)."""
        obs = _make_resource_observation(gpu_temperature_celsius=-5.0)
        assert obs.gpu_temperature_celsius == -5.0


class TestBackwardCompatibility:
    """Existing observations without the new per-inference fields still validate."""

    def test_observation_without_new_fields_validates(self):
        """An observation constructed with only the original fields validates."""
        obs = ResourceObservation(
            run_id="run-1",
            model_variant_id="v1",
            task_block="task-1",
            hardware_identity="linux/amd64/rtx-4090",
            collection_tool="psutil-5.9",
            source_evidence_hash=_VALID_HASH,
            model_load_time_seconds=10.0,
            peak_resident_memory_bytes=4_000_000_000,
            artifact_bytes=4_800_000_000,
            end_to_end_latency_seconds=1.5,
            provider_call_latency_seconds=1.2,
        )
        assert obs.time_to_first_token_seconds is None
        assert obs.gpu_utilization_percent is None

    def test_legacy_json_round_trip_preserves_absence(self):
        """JSON without the new fields round-trips with None for the new fields."""
        legacy_json = (
            '{"run_id":"run-1","model_variant_id":"v1","task_block":"task-1",'
            '"hardware_identity":"linux/amd64/rtx-4090","collection_tool":"psutil-5.9",'
            '"source_evidence_hash":"' + _VALID_HASH + '",'
            '"model_load_time_seconds":10.0,"peak_resident_memory_bytes":4000000000,'
            '"artifact_bytes":4800000000,"end_to_end_latency_seconds":1.5,'
            '"provider_call_latency_seconds":1.2}'
        )
        obs = ResourceObservation.model_validate_json(legacy_json)
        assert obs.time_to_first_token_seconds is None
        assert obs.generation_duration_seconds is None
        assert obs.accelerator_memory_before_bytes is None
        assert obs.gpu_utilization_percent is None
        assert obs.gpu_temperature_celsius is None
        assert obs.gpu_power_draw_watts is None
        assert obs.gpu_clock_mhz is None
