# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

"""GPU metrics collection via ``pynvml`` for per-inference resource observations.

Captures GPU utilization, temperature, power draw, clock, and VRAM
baseline when an NVIDIA GPU is present. When ``pynvml`` is not installed
or no NVIDIA GPU is available, every metric is ``None`` and
``collection_tool`` reports ``"none"`` so the caller can record the
absence cleanly without branching on platform.

The collector is designed for the cold-start vs warm-inference
separation in ``ResourceObservation``: ``memory_before_bytes`` records
the VRAM baseline before an inference call, and ``snapshot`` records
the live GPU state during inference. Both are ``None`` when no GPU is
present.

``pynvml`` is imported lazily so this module imports cleanly on
machines without the library or without an NVIDIA driver. The
collection tool and version (e.g. ``pynvml-12.0.0``) are surfaced via
``collection_tool`` so they can be recorded in the
``ResourceObservation.collection_tool`` field alongside the process
resource tool (e.g. ``psutil``).
"""

from __future__ import annotations

from pydantic import BaseModel, ConfigDict, Field


class GpuSnapshot(BaseModel):
    """Frozen GPU metrics snapshot bound to a single inference event.

    Every field is ``None`` when no NVIDIA GPU is present or the metric
    is not available. The model is frozen and forbids extra fields.
    """

    model_config = ConfigDict(extra="forbid", frozen=True)

    accelerator_memory_before_bytes: int | None = Field(
        default=None, ge=0,
        description="VRAM baseline before inference in bytes. None when not on GPU or not measured.",
    )
    gpu_utilization_percent: float | None = Field(
        default=None, ge=0.0,
        description="GPU utilization in percent. None when not on GPU or not measured.",
    )
    gpu_temperature_celsius: float | None = Field(
        default=None,
        description="GPU temperature in degrees Celsius. None when not on GPU or not measured.",
    )
    gpu_power_draw_watts: float | None = Field(
        default=None, ge=0.0,
        description="GPU power draw in watts. None when not on GPU or not measured.",
    )
    gpu_clock_mhz: float | None = Field(
        default=None, ge=0.0,
        description="GPU clock in MHz. None when not on GPU or not measured.",
    )


class GpuMetricsCollector:
    """Collects GPU metrics via ``pynvml`` when an NVIDIA GPU is present.

    ``pynvml`` is imported lazily on first use. When the library is not
    installed, no NVIDIA driver is present, or NVML initialization fails,
    the collector reports ``is_available() == False`` and every
    ``snapshot`` / ``memory_before_bytes`` call returns all-``None`
    values. The collector is safe to construct and use on any platform.

    The collector holds an NVML handle for GPU index 0 (the single-GPU
    desktop target). Call ``close`` to release the NVML handle, or use
    the collector as a context manager.
    """

    def __init__(self) -> None:
        self._pynvml = None
        self._handle = None
        self._version: str | None = None
        self._available = False
        self._initialized = False

    def _ensure_initialized(self) -> None:
        if self._initialized:
            return
        self._initialized = True
        try:
            import pynvml  # type: ignore[import-untyped]
        except ImportError:
            return
        try:
            pynvml.nvmlInit()
            self._pynvml = pynvml
            self._handle = pynvml.nvmlDeviceGetHandleByIndex(0)
            try:
                self._version = pynvml.nvmlSystemGetNVMLVersion()
            except Exception:
                self._version = "unknown"
            self._available = True
        except Exception:
            self._pynvml = None
            self._handle = None
            self._version = None
            self._available = False

    def is_available(self) -> bool:
        """Return ``True`` when an NVIDIA GPU is present and NVML initialized."""
        self._ensure_initialized()
        return self._available

    def collection_tool(self) -> str:
        """Return the collection tool and version for the ``collection_tool`` field.

        Returns ``"pynvml-<version>"`` when an NVIDIA GPU is present and
        ``"none"`` when no GPU is available or ``pynvml`` is not installed.
        """
        self._ensure_initialized()
        if not self._available or self._version is None:
            return "none"
        return f"pynvml-{self._version}"

    def memory_before_bytes(self) -> int | None:
        """Return the VRAM baseline (used bytes) before an inference call.

        ``None`` when no GPU is present. Uses ``nvmlDeviceGetMemoryInfo``
        which reports used memory in bytes.
        """
        self._ensure_initialized()
        if not self._available or self._pynvml is None or self._handle is None:
            return None
        try:
            info = self._pynvml.nvmlDeviceGetMemoryInfo(self._handle)
            return int(info.used)
        except Exception:
            return None

    def snapshot(self) -> GpuSnapshot:
        """Capture the current GPU metrics as a frozen ``GpuSnapshot``.

        Every field is ``None`` when no GPU is present. Individual metrics
        that fail to read are ``None`` rather than raising, so a partial
        GPU read does not abort the inference record.
        """
        self._ensure_initialized()
        if not self._available or self._pynvml is None or self._handle is None:
            return GpuSnapshot()

        utilization = self._read_utilization()
        temperature = self._read_temperature()
        power = self._read_power()
        clock = self._read_clock()
        memory_before = self.memory_before_bytes()

        return GpuSnapshot(
            accelerator_memory_before_bytes=memory_before,
            gpu_utilization_percent=utilization,
            gpu_temperature_celsius=temperature,
            gpu_power_draw_watts=power,
            gpu_clock_mhz=clock,
        )

    def _read_utilization(self) -> float | None:
        assert self._pynvml is not None
        assert self._handle is not None
        try:
            rates = self._pynvml.nvmlDeviceGetUtilizationRates(self._handle)
            return float(rates.gpu)
        except Exception:
            return None

    def _read_temperature(self) -> float | None:
        assert self._pynvml is not None
        assert self._handle is not None
        try:
            return float(self._pynvml.nvmlDeviceGetTemperature(self._handle, self._pynvml.NVML_TEMPERATURE_GPU))
        except Exception:
            return None

    def _read_power(self) -> float | None:
        assert self._pynvml is not None
        assert self._handle is not None
        try:
            power_mw = self._pynvml.nvmlDeviceGetPowerUsage(self._handle)
            return float(power_mw) / 1000.0
        except Exception:
            return None

    def _read_clock(self) -> float | None:
        assert self._pynvml is not None
        assert self._handle is not None
        try:
            clock_info = self._pynvml.nvmlDeviceGetClockInfo(self._handle, self._pynvml.NVML_CLOCK_SM)
            return float(clock_info)
        except Exception:
            return None

    def close(self) -> None:
        """Release the NVML handle. Safe to call when no GPU is present.

        After close the collector reports unavailable; ``is_available`` does
        not re-initialize. Construct a new collector to re-probe the GPU.
        """
        if self._pynvml is not None and self._available:
            try:
                self._pynvml.nvmlShutdown()
            except Exception:
                pass
        self._pynvml = None
        self._handle = None
        self._version = None
        self._available = False
        self._initialized = True

    def __enter__(self) -> GpuMetricsCollector:
        self._ensure_initialized()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        self.close()


__all__ = [
    "GpuMetricsCollector",
    "GpuSnapshot",
]
