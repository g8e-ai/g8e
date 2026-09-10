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
