# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""Tier 1 tests for the GPU metrics collector.

Verifies that ``GpuMetricsCollector`` lazily imports ``pynvml``, reports
``is_available() == False`` and ``collection_tool() == "none"``
gracefully when no NVIDIA GPU is present (or ``pynvml`` is not
installed), and captures GPU metrics when a GPU is present (simulated
via a fake ``pynvml`` module injected into ``sys.modules``).

Also verifies that ``GpuSnapshot`` is frozen, forbids extra fields, and
validates the same constraints as the ``ResourceObservation`` GPU
fields.
"""

# pyright: reportCallIssue=false
# This file intentionally constructs models with extra fields and
# negative values to verify pydantic validation rejects them.

from __future__ import annotations

import sys

import pytest
from pydantic import ValidationError

from g8e_evals.gpu import GpuMetricsCollector, GpuSnapshot
from g8e_evals.index import MeasurementAvailability, MeasurementScope, UnavailableMeasurement


pytestmark = pytest.mark.unit


class _FakeMemoryInfo:
    def __init__(self, used: int) -> None:
        self.used = used


class _FakeUtilizationRates:
    def __init__(self, gpu: int) -> None:
        self.gpu = gpu


class _FakePynvml:
    """Minimal fake of the ``pynvml`` module for unit testing."""

    NVML_TEMPERATURE_GPU = 0
    NVML_CLOCK_SM = 0

    def __init__(
        self,
        *,
        version: str = "12.0.0",
        memory_used: int = 2_000_000_000,
        utilization_gpu: int = 87,
        temperature: int = 71,
        power_mw: int = 280_000,
        clock_mhz: int = 2520,
    ) -> None:
        self._version = version
        self._memory_info = _FakeMemoryInfo(memory_used)
        self._utilization = _FakeUtilizationRates(utilization_gpu)
        self._temperature = temperature
        self._power_mw = power_mw
        self._clock_mhz = clock_mhz
        self.init_called = False
        self.shutdown_called = False

    def nvmlInit(self) -> None:
        self.init_called = True

    def nvmlShutdown(self) -> None:
        self.shutdown_called = True

    def nvmlSystemGetNVMLVersion(self) -> str:
        return self._version

    def nvmlDeviceGetHandleByIndex(self, index: int):
        return object()

    def nvmlDeviceGetMemoryInfo(self, handle):
        return self._memory_info

    def nvmlDeviceGetUtilizationRates(self, handle):
        return self._utilization

    def nvmlDeviceGetTemperature(self, handle, sensor):
        return self._temperature

    def nvmlDeviceGetPowerUsage(self, handle):
        return self._power_mw

    def nvmlDeviceGetClockInfo(self, handle, clock_type):
        return self._clock_mhz


class _FailingPynvml:
    """Fake pynvml that fails on nvmlInit to simulate no NVIDIA driver."""

    def nvmlInit(self) -> None:
        raise OSError("no NVIDIA driver")

    def nvmlShutdown(self) -> None:
        pass


class TestGpuSnapshotModel:
    def test_all_fields_default_to_none(self):
        snap = GpuSnapshot()
        assert snap.accelerator_memory_before_bytes is None
        assert snap.gpu_utilization_percent is None
        assert snap.gpu_temperature_celsius is None
        assert snap.gpu_power_draw_watts is None
        assert snap.gpu_clock_mhz is None

    def test_round_trip_preserves_all_fields(self):
        snap = GpuSnapshot(
            accelerator_memory_before_bytes=2_000_000_000,
            gpu_utilization_percent=87.5,
            gpu_temperature_celsius=71.0,
            gpu_power_draw_watts=280.0,
            gpu_clock_mhz=2520.0,
        )
        restored = GpuSnapshot.model_validate_json(snap.model_dump_json())
        assert restored == snap

    def test_frozen_model(self):
        snap = GpuSnapshot()
        with pytest.raises(ValidationError):
            snap.gpu_utilization_percent = 50.0  # type: ignore[misc]

    def test_rejects_unknown_fields(self):
        with pytest.raises(ValidationError):
            GpuSnapshot(extra_field="bad")

    def test_rejects_negative_accelerator_memory_before(self):
        with pytest.raises(ValidationError):
            GpuSnapshot(accelerator_memory_before_bytes=-1)

    def test_rejects_negative_gpu_utilization(self):
        with pytest.raises(ValidationError):
            GpuSnapshot(gpu_utilization_percent=-1.0)

    def test_rejects_negative_gpu_power_draw(self):
        with pytest.raises(ValidationError):
            GpuSnapshot(gpu_power_draw_watts=-1.0)

    def test_rejects_negative_gpu_clock(self):
        with pytest.raises(ValidationError):
            GpuSnapshot(gpu_clock_mhz=-1.0)

    def test_allows_negative_gpu_temperature(self):
        snap = GpuSnapshot(gpu_temperature_celsius=-5.0)
        assert snap.gpu_temperature_celsius == -5.0


class TestGpuMetricsCollectorNoGpu:
    def test_is_available_false_when_pynvml_not_installed(self):
        collector = GpuMetricsCollector()
        assert collector.is_available() is False

    def test_collection_tool_none_when_no_gpu(self):
        collector = GpuMetricsCollector()
        assert collector.collection_tool() == "none"

    def test_snapshot_all_none_when_no_gpu(self):
        collector = GpuMetricsCollector()
        snap = collector.snapshot()
        assert snap.accelerator_memory_before_bytes is None
        assert snap.gpu_utilization_percent is None
        assert snap.gpu_temperature_celsius is None
        assert snap.gpu_power_draw_watts is None
        assert snap.gpu_clock_mhz is None

    def test_memory_before_bytes_none_when_no_gpu(self):
        collector = GpuMetricsCollector()
        assert collector.memory_before_bytes() is None

    def test_close_resets_state(self):
        collector = GpuMetricsCollector()
        collector.is_available()
        collector.close()
        assert collector.is_available() is False

    def test_context_manager_closes(self):
        with GpuMetricsCollector() as collector:
            assert collector.is_available() is False
        assert collector.is_available() is False


class TestGpuMetricsCollectorNoDriver:
    """When pynvml is installed but nvmlInit fails (no driver), the collector
    reports unavailable without raising."""

    def test_is_available_false_when_init_fails(self, monkeypatch):
        fake = _FailingPynvml()
        monkeypatch.setitem(sys.modules, "pynvml", fake)
        collector = GpuMetricsCollector()
        assert collector.is_available() is False
        assert collector.collection_tool() == "none"
        assert collector.snapshot().gpu_utilization_percent is None


class TestGpuMetricsCollectorWithGpu:
    """When pynvml is installed and an NVIDIA GPU is present, the collector
    captures GPU metrics. A fake pynvml module is injected via sys.modules."""

    def _install_fake_pynvml(self, monkeypatch, **kwargs) -> _FakePynvml:
        fake = _FakePynvml(**kwargs)
        monkeypatch.setitem(sys.modules, "pynvml", fake)
        return fake

    def test_is_available_true_when_gpu_present(self, monkeypatch):
        self._install_fake_pynvml(monkeypatch)
        collector = GpuMetricsCollector()
        assert collector.is_available() is True

    def test_collection_tool_includes_pynvml_version(self, monkeypatch):
        self._install_fake_pynvml(monkeypatch, version="12.535.54")
        collector = GpuMetricsCollector()
        assert collector.collection_tool() == "pynvml-12.535.54"

    def test_snapshot_captures_all_gpu_metrics(self, monkeypatch):
        self._install_fake_pynvml(
            monkeypatch,
            memory_used=2_000_000_000,
            utilization_gpu=87,
            temperature=71,
            power_mw=280_000,
            clock_mhz=2520,
        )
        collector = GpuMetricsCollector()
        snap = collector.snapshot()
        assert snap.accelerator_memory_before_bytes == 2_000_000_000
        assert snap.gpu_utilization_percent == 87.0
        assert snap.gpu_temperature_celsius == 71.0
        assert snap.gpu_power_draw_watts == 280.0
        assert snap.gpu_clock_mhz == 2520.0

    def test_memory_before_bytes_returns_used_memory(self, monkeypatch):
        self._install_fake_pynvml(monkeypatch, memory_used=4_500_000_000)
        collector = GpuMetricsCollector()
        assert collector.memory_before_bytes() == 4_500_000_000

    def test_power_converted_from_milliwatts_to_watts(self, monkeypatch):
        self._install_fake_pynvml(monkeypatch, power_mw=150_000)
        collector = GpuMetricsCollector()
        snap = collector.snapshot()
        assert snap.gpu_power_draw_watts == 150.0

    def test_snapshot_none_fields_when_individual_reads_fail(self, monkeypatch):
        """A partial GPU read (some metrics fail) returns None for the failed
        metrics rather than raising."""

        class _PartialPynvml(_FakePynvml):
            def nvmlDeviceGetTemperature(self, handle, sensor):
                raise OSError("sensor unavailable")

            def nvmlDeviceGetClockInfo(self, handle, clock_type):
                raise OSError("clock unavailable")

        monkeypatch.setitem(sys.modules, "pynvml", _PartialPynvml())
        collector = GpuMetricsCollector()
        snap = collector.snapshot()
        assert snap.gpu_utilization_percent == 87.0
        assert snap.gpu_temperature_celsius is None
        assert snap.gpu_clock_mhz is None
        assert snap.gpu_power_draw_watts == 280.0

    def test_close_calls_nvml_shutdown(self, monkeypatch):
        fake = self._install_fake_pynvml(monkeypatch)
        collector = GpuMetricsCollector()
        collector.is_available()
        assert fake.shutdown_called is False
        collector.close()
        assert fake.shutdown_called is True
        assert collector.is_available() is False

    def test_context_manager_calls_shutdown(self, monkeypatch):
        fake = self._install_fake_pynvml(monkeypatch)
        with GpuMetricsCollector() as collector:
            assert collector.is_available() is True
        assert fake.shutdown_called is True

    def test_collection_tool_none_after_close(self, monkeypatch):
        self._install_fake_pynvml(monkeypatch)
        collector = GpuMetricsCollector()
        assert collector.collection_tool().startswith("pynvml-")
        collector.close()
        assert collector.collection_tool() == "none"


class TestGpuMetricsCollectorIntegrationWithObservation:
    """The GPU collector output maps directly onto ResourceObservation GPU fields."""

    def test_gpu_snapshot_fields_match_resource_observation(self, monkeypatch):
        from g8e_evals.index import ResourceObservation

        _FakePynvml()
        monkeypatch.setitem(sys.modules, "pynvml", _FakePynvml(
            memory_used=2_000_000_000,
            utilization_gpu=87,
            temperature=71,
            power_mw=280_000,
            clock_mhz=2520,
        ))
        collector = GpuMetricsCollector()
        snap = collector.snapshot()

        obs = ResourceObservation(
            campaign_id="campaign-1",
            child_id="campaign-1",
            run_id="run-1",
            assignment_id="assignment-1",
            attempt_id="att-1",
            inference_id="inf-1",
            stage_id="att-1:direct:1",
            role="primary",
            model_variant_id="v1",
            task_id="task-1",
            orchestrator_scope="linux/amd64/cpu",
            provider_scope="linux/amd64/rtx-4090",
            observation_boundary="provider_call",
            clock_domain="monotonic",
            collection_tool=f"psutil-5.9+{collector.collection_tool()}",
            source_evidence_refs=["evidence/test.json"],
            source_evidence_sha256="a" * 64,
            verification_status="verified",
            model_load_time_seconds=10.0,
            peak_resident_memory_bytes=4_000_000_000,
            artifact_bytes=4_800_000_000,
            end_to_end_latency_seconds=1.5,
            provider_call_latency_seconds=1.2,
            accelerator_memory_before_bytes=snap.accelerator_memory_before_bytes,
            gpu_utilization_percent=snap.gpu_utilization_percent,
            gpu_temperature_celsius=snap.gpu_temperature_celsius,
            gpu_power_draw_watts=snap.gpu_power_draw_watts,
            gpu_clock_mhz=snap.gpu_clock_mhz,
            unavailable_measurements=[
                UnavailableMeasurement(
                    field_name="peak_accelerator_memory_bytes",
                    availability=MeasurementAvailability.UNAVAILABLE,
                    scope=MeasurementScope.PROVIDER_REMOTE,
                    reason="not measured",
                ),
                UnavailableMeasurement(
                    field_name="measured_energy_joules",
                    availability=MeasurementAvailability.UNAVAILABLE,
                    scope=MeasurementScope.PROVIDER_REMOTE,
                    reason="not measured",
                ),
                UnavailableMeasurement(
                    field_name="output_throughput_tokens_per_second",
                    availability=MeasurementAvailability.UNAVAILABLE,
                    scope=MeasurementScope.PROVIDER_REMOTE,
                    reason="not measured",
                ),
                UnavailableMeasurement(
                    field_name="hidden_reasoning_throughput_tokens_per_second",
                    availability=MeasurementAvailability.UNAVAILABLE,
                    scope=MeasurementScope.PROVIDER_REMOTE,
                    reason="not measured",
                ),
                UnavailableMeasurement(
                    field_name="time_to_first_token_seconds",
                    availability=MeasurementAvailability.UNAVAILABLE,
                    scope=MeasurementScope.PROVIDER_REMOTE,
                    reason="not measured",
                ),
                UnavailableMeasurement(
                    field_name="generation_duration_seconds",
                    availability=MeasurementAvailability.UNAVAILABLE,
                    scope=MeasurementScope.PROVIDER_REMOTE,
                    reason="not measured",
                ),
            ],
        )
        assert obs.accelerator_memory_before_bytes == 2_000_000_000
        assert obs.gpu_utilization_percent == 87.0
        assert obs.gpu_temperature_celsius == 71.0
        assert obs.gpu_power_draw_watts == 280.0
        assert obs.gpu_clock_mhz == 2520.0
        assert obs.collection_tool == "psutil-5.9+pynvml-12.0.0"
        collector.close()
