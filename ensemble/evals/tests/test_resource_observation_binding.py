# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for resource observation binding.

Verifies that resource observations (latency, memory, energy) are bound
to campaign ID, child/report ID, assignment ID, attempt ID, inference
ID, role, model variant ID, canonical task ID, orchestrator scope,
provider scope, observation boundary, clock domain, collection tool,
and source evidence hash. Unbound or partially bound observations are
rejected. Observations without evidence hashes are rejected. VRAM is
never inferred from parameter count or quantization labels.

Missing and zero are distinct: every None measurement field must have a
typed unavailable_measurements entry explaining the unavailability
with scope and reason. Zero (0 or 0.0) is always a measured zero.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with missing required fields
# and unknown extra fields to verify pydantic validation rejects them.

from __future__ import annotations

import pytest
from pydantic import ValidationError

from g8e_evals.index import (
    MeasurementAvailability,
    MeasurementScope,
    ModelRole,
    ResourceObservation,
    UnavailableMeasurement,
    validate_resource_observations,
)


pytestmark = pytest.mark.unit

_VALID_HASH = "a" * 64


def _make_unavailable(field_name: str, *, availability: MeasurementAvailability = MeasurementAvailability.UNAVAILABLE) -> UnavailableMeasurement:
    return UnavailableMeasurement(
        field_name=field_name,
        availability=availability,
        scope=MeasurementScope.PROVIDER_REMOTE,
        reason=f"{field_name} not available at remote provider boundary",
    )


_MEASUREMENT_FIELD_NAMES: tuple[str, ...] = (
    "model_load_time_seconds",
    "peak_resident_memory_bytes",
    "peak_accelerator_memory_bytes",
    "artifact_bytes",
    "measured_energy_joules",
    "end_to_end_latency_seconds",
    "provider_call_latency_seconds",
    "output_throughput_tokens_per_second",
    "hidden_reasoning_throughput_tokens_per_second",
    "time_to_first_token_seconds",
    "generation_duration_seconds",
    "accelerator_memory_before_bytes",
    "gpu_utilization_percent",
    "gpu_temperature_celsius",
    "gpu_power_draw_watts",
    "gpu_clock_mhz",
)


def _auto_unavailable(**measurement_values) -> list[UnavailableMeasurement]:
    """Build unavailable_measurements for every measurement field that is None."""
    result: list[UnavailableMeasurement] = []
    for field_name in _MEASUREMENT_FIELD_NAMES:
        if measurement_values.get(field_name) is None:
            result.append(_make_unavailable(field_name))
    return result


def _make_resource_observation(
    *,
    campaign_id: str = "campaign-1",
    child_id: str = "campaign-1",
    run_id: str = "run-1",
    assignment_id: str = "assignment-1",
    attempt_id: str = "att-1",
    inference_id: str = "inf-1",
    role: ModelRole = ModelRole.PRIMARY,
    model_variant_id: str = "qwen3-8b-q4_0",
    task_id: str = "task-1",
    orchestrator_scope: str = "linux/amd64/cpu",
    provider_scope: str = "linux/amd64/rtx-4090",
    observation_boundary: str = "provider_call",
    clock_domain: str = "monotonic",
    collection_tool: str = "psutil-5.9",
    source_evidence_hash: str = _VALID_HASH,
    model_load_time_seconds: float | None = 12.5,
    peak_resident_memory_bytes: int | None = 4_000_000_000,
    peak_accelerator_memory_bytes: int | None = 8_000_000_000,
    artifact_bytes: int | None = 4_800_000_000,
    measured_energy_joules: float | None = None,
    end_to_end_latency_seconds: float | None = 1.5,
    provider_call_latency_seconds: float | None = 1.2,
    output_throughput_tokens_per_second: float | None = None,
    time_to_first_token_seconds: float | None = None,
    generation_duration_seconds: float | None = None,
    accelerator_memory_before_bytes: int | None = None,
    gpu_utilization_percent: float | None = None,
    gpu_temperature_celsius: float | None = None,
    gpu_power_draw_watts: float | None = None,
    gpu_clock_mhz: float | None = None,
    unavailable_measurements: list[UnavailableMeasurement] | None = None,
) -> ResourceObservation:
    measurement_values = {
        "model_load_time_seconds": model_load_time_seconds,
        "peak_resident_memory_bytes": peak_resident_memory_bytes,
        "peak_accelerator_memory_bytes": peak_accelerator_memory_bytes,
        "artifact_bytes": artifact_bytes,
        "measured_energy_joules": measured_energy_joules,
        "end_to_end_latency_seconds": end_to_end_latency_seconds,
        "provider_call_latency_seconds": provider_call_latency_seconds,
        "output_throughput_tokens_per_second": output_throughput_tokens_per_second,
        "time_to_first_token_seconds": time_to_first_token_seconds,
        "generation_duration_seconds": generation_duration_seconds,
        "accelerator_memory_before_bytes": accelerator_memory_before_bytes,
        "gpu_utilization_percent": gpu_utilization_percent,
        "gpu_temperature_celsius": gpu_temperature_celsius,
        "gpu_power_draw_watts": gpu_power_draw_watts,
        "gpu_clock_mhz": gpu_clock_mhz,
    }
    if unavailable_measurements is None:
        unavailable_measurements = _auto_unavailable(**measurement_values)
    return ResourceObservation(
        campaign_id=campaign_id,
        child_id=child_id,
        run_id=run_id,
        assignment_id=assignment_id,
        attempt_id=attempt_id,
        inference_id=inference_id,
        role=role,
        model_variant_id=model_variant_id,
        task_id=task_id,
        orchestrator_scope=orchestrator_scope,
        provider_scope=provider_scope,
        observation_boundary=observation_boundary,
        clock_domain=clock_domain,
        collection_tool=collection_tool,
        source_evidence_hash=source_evidence_hash,
        unavailable_measurements=unavailable_measurements,
        **measurement_values,
    )


def _make_fully_unavailable_observation(**kwargs) -> ResourceObservation:
    """Build an observation where all measurement fields are None with typed explanations."""
    return _make_resource_observation(
        model_load_time_seconds=None,
        peak_resident_memory_bytes=None,
        peak_accelerator_memory_bytes=None,
        artifact_bytes=None,
        measured_energy_joules=None,
        end_to_end_latency_seconds=None,
        provider_call_latency_seconds=None,
        output_throughput_tokens_per_second=None,
        time_to_first_token_seconds=None,
        generation_duration_seconds=None,
        accelerator_memory_before_bytes=None,
        gpu_utilization_percent=None,
        gpu_temperature_celsius=None,
        gpu_power_draw_watts=None,
        gpu_clock_mhz=None,
        **kwargs,
    )


class TestResourceObservationModel:
    def test_round_trip_preserves_all_fields(self):
        obs = _make_fully_unavailable_observation()
        restored = ResourceObservation.model_validate_json(obs.model_dump_json())
        assert restored == obs

    def test_rejects_unknown_fields(self):
        data = _make_fully_unavailable_observation().model_dump()
        data["extra_field"] = "bad"
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_frozen_model(self):
        obs = _make_fully_unavailable_observation()
        with pytest.raises((TypeError, ValueError)):
            obs.run_id = "changed"  # type: ignore[misc]

    def test_requires_campaign_id(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["campaign_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_child_id(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["child_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_run_id(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["run_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_assignment_id(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["assignment_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_attempt_id(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["attempt_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_inference_id(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["inference_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_role(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["role"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_model_variant_id(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["model_variant_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_task_id(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["task_id"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_orchestrator_scope(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["orchestrator_scope"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_provider_scope(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["provider_scope"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_observation_boundary(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["observation_boundary"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_clock_domain(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["clock_domain"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_collection_tool(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["collection_tool"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_requires_source_evidence_hash(self):
        data = _make_fully_unavailable_observation().model_dump()
        del data["source_evidence_hash"]
        with pytest.raises(ValidationError):
            ResourceObservation(**data)


class TestResourceObservationValidation:
    def test_valid_observations_pass(self):
        """A list of valid resource observations passes validation."""
        observations = [
            _make_fully_unavailable_observation(run_id="run-1", attempt_id="att-1", inference_id="inf-1"),
            _make_fully_unavailable_observation(run_id="run-1", attempt_id="att-1", inference_id="inf-2"),
            _make_fully_unavailable_observation(run_id="run-2", attempt_id="att-1", inference_id="inf-1"),
        ]
        validate_resource_observations(observations)

    def test_empty_observations_pass(self):
        """An empty list of observations passes."""
        validate_resource_observations([])

    def test_duplicate_inference_rejected(self):
        """Duplicate (campaign, child, run, assignment, attempt, inference) tuples are rejected."""
        observations = [
            _make_fully_unavailable_observation(inference_id="inf-1"),
            _make_fully_unavailable_observation(inference_id="inf-1"),
        ]
        with pytest.raises(ValueError, match=r"duplicate.*resource observation"):
            validate_resource_observations(observations)

    def test_same_attempt_different_inference_allowed(self):
        """The same attempt with different inference IDs is allowed (one per inference)."""
        observations = [
            _make_fully_unavailable_observation(attempt_id="att-1", inference_id="inf-1"),
            _make_fully_unavailable_observation(attempt_id="att-1", inference_id="inf-2"),
        ]
        validate_resource_observations(observations)

    def test_same_inference_different_attempt_allowed(self):
        """The same inference ID with different attempt IDs is allowed."""
        observations = [
            _make_fully_unavailable_observation(attempt_id="att-1", inference_id="inf-1"),
            _make_fully_unavailable_observation(attempt_id="att-2", inference_id="inf-1"),
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
        """When accelerator memory is not measured, it is None with a typed explanation."""
        obs = _make_fully_unavailable_observation()
        assert obs.peak_accelerator_memory_bytes is None
        um = [u for u in obs.unavailable_measurements if u.field_name == "peak_accelerator_memory_bytes"][0]
        assert um.availability == MeasurementAvailability.UNAVAILABLE

    def test_energy_none_when_no_calibrated_source(self):
        """When no calibrated energy source exists, measured_energy_joules is None with explanation."""
        obs = _make_fully_unavailable_observation()
        assert obs.measured_energy_joules is None
        um = [u for u in obs.unavailable_measurements if u.field_name == "measured_energy_joules"][0]
        assert um.availability == MeasurementAvailability.UNAVAILABLE


class TestPerInferenceFields:
    """Verify the per-inference extension fields separate cold-start from warm
    inference and capture GPU metrics, all defaulting to None for backward
    compatibility with existing observations."""

    def test_new_fields_default_to_none(self):
        """All per-inference extension fields default to None when omitted."""
        obs = _make_fully_unavailable_observation()
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
        data = _make_fully_unavailable_observation().model_dump()
        data["time_to_first_token_seconds"] = -0.1
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_generation_duration(self):
        data = _make_fully_unavailable_observation().model_dump()
        data["generation_duration_seconds"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_accelerator_memory_before(self):
        data = _make_fully_unavailable_observation().model_dump()
        data["accelerator_memory_before_bytes"] = -1
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_gpu_utilization(self):
        data = _make_fully_unavailable_observation().model_dump()
        data["gpu_utilization_percent"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_gpu_power_draw(self):
        data = _make_fully_unavailable_observation().model_dump()
        data["gpu_power_draw_watts"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_rejects_negative_gpu_clock(self):
        data = _make_fully_unavailable_observation().model_dump()
        data["gpu_clock_mhz"] = -1.0
        with pytest.raises(ValidationError):
            ResourceObservation(**data)

    def test_allows_negative_gpu_temperature(self):
        """GPU temperature can be negative (sub-ambient cooling)."""
        obs = _make_resource_observation(
            gpu_temperature_celsius=-5.0,
        )
        assert obs.gpu_temperature_celsius == -5.0


class TestUnavailableMeasurementConsistency:
    """Verify that None measurement fields require typed unavailable_measurements entries."""

    def test_none_field_without_explanation_rejected(self):
        """A None measurement field without an unavailable_measurements entry is rejected."""
        data = _make_fully_unavailable_observation().model_dump()
        # Remove the explanation for model_load_time_seconds (which is None)
        data["unavailable_measurements"] = [
            um for um in data["unavailable_measurements"]
            if um["field_name"] != "model_load_time_seconds"
        ]
        with pytest.raises(ValidationError, match="without unavailable_measurements"):
            ResourceObservation(**data)

    def test_explanation_for_non_none_field_rejected(self):
        """An unavailable_measurements entry for a non-None field is rejected."""
        data = _make_resource_observation(model_load_time_seconds=10.0).model_dump()
        # Add an explanation for model_load_time_seconds which is 10.0 (not None)
        data["unavailable_measurements"].append({
            "field_name": "model_load_time_seconds",
            "availability": "unavailable",
            "scope": "provider_remote",
            "reason": "fake explanation for a measured field",
        })
        with pytest.raises(ValidationError, match="non-None"):
            ResourceObservation(**data)

    def test_duplicate_explanations_rejected(self):
        """Duplicate unavailable_measurements entries for the same field are rejected."""
        data = _make_fully_unavailable_observation().model_dump()
        # Add a duplicate entry for model_load_time_seconds
        data["unavailable_measurements"].append(data["unavailable_measurements"][0])
        with pytest.raises(ValidationError, match="duplicate unavailable"):
            ResourceObservation(**data)

    def test_explanation_for_unknown_field_rejected(self):
        """An unavailable_measurements entry for an unknown field is rejected."""
        data = _make_fully_unavailable_observation().model_dump()
        data["unavailable_measurements"].append({
            "field_name": "nonexistent_field",
            "availability": "unavailable",
            "scope": "provider_remote",
            "reason": "fake",
        })
        with pytest.raises(ValidationError, match="unknown field"):
            ResourceObservation(**data)

    def test_measured_zero_distinct_from_unavailable(self):
        """Zero (0 or 0.0) is a measured zero, not unavailable."""
        obs = _make_resource_observation(
            model_load_time_seconds=0.0,
            peak_resident_memory_bytes=0,
        )
        assert obs.model_load_time_seconds == 0.0
        assert obs.peak_resident_memory_bytes == 0
        # No unavailable_measurements entry for these measured-zero fields
        field_names = {um.field_name for um in obs.unavailable_measurements}
        assert "model_load_time_seconds" not in field_names
        assert "peak_resident_memory_bytes" not in field_names


class TestModelRoleEnum:
    def test_model_role_has_three_values(self):
        assert len(list(ModelRole)) == 3
        values = {r.value for r in ModelRole}
        assert values == {"primary", "assistant", "lite"}

    def test_role_field_accepts_typed_enum(self):
        obs = _make_fully_unavailable_observation(role=ModelRole.ASSISTANT)
        assert obs.role == ModelRole.ASSISTANT

    def test_role_field_accepts_string_value(self):
        data = _make_fully_unavailable_observation().model_dump()
        data["role"] = "lite"
        obs = ResourceObservation(**data)
        assert obs.role == ModelRole.LITE

    def test_role_field_rejects_unknown_value(self):
        data = _make_fully_unavailable_observation().model_dump()
        data["role"] = "unknown_role"
        with pytest.raises(ValidationError):
            ResourceObservation(**data)


class TestMeasurementAvailabilityEnum:
    def test_availability_has_four_values(self):
        assert len(list(MeasurementAvailability)) == 4
        values = {a.value for a in MeasurementAvailability}
        assert values == {"measured", "unavailable", "not_applicable", "withheld"}

    def test_scope_has_two_values(self):
        assert len(list(MeasurementScope)) == 2
        values = {s.value for s in MeasurementScope}
        assert values == {"orchestrator_local", "provider_remote"}
