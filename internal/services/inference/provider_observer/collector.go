// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"context"
	"time"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

const (
	SchemaVersion              = "1.0.0"
	DefaultDevicePseudonym     = "gpu-0"
	DefaultObserverClockSource = "host-monotonic"
)

// HardwareCollector samples read-only provider-boundary hardware telemetry.
type HardwareCollector interface {
	Collect(ctx context.Context, observedAt time.Time) (*evalv1.ProviderBoundaryHardwareSample, error)
}

// CompositeCollector merges GPU and host RAM collectors into one sample.
type CompositeCollector struct {
	GPU HardwareCollector
	RAM HardwareCollector
}

func (c *CompositeCollector) Collect(ctx context.Context, observedAt time.Time) (*evalv1.ProviderBoundaryHardwareSample, error) {
	sample := &evalv1.ProviderBoundaryHardwareSample{
		ObservedAtUnixNanos: uint64(observedAt.UTC().UnixNano()),
		DevicePseudonym:     DefaultDevicePseudonym,
	}
	if c.GPU != nil {
		gpuSample, err := c.GPU.Collect(ctx, observedAt)
		if err == nil && gpuSample != nil {
			mergeHardwareSample(sample, gpuSample)
		}
	}
	if c.RAM != nil {
		ramSample, err := c.RAM.Collect(ctx, observedAt)
		if err == nil && ramSample != nil {
			mergeHardwareSample(sample, ramSample)
		}
	}
	markUnavailableMetrics(sample)
	return sample, nil
}

func mergeHardwareSample(target, source *evalv1.ProviderBoundaryHardwareSample) {
	if source.GetDevicePseudonym() != "" {
		target.DevicePseudonym = source.GetDevicePseudonym()
	}
	if source.GetVramBytesAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
		target.VramBytesAvailability = source.GetVramBytesAvailability()
		target.VramUsedBytes = source.GetVramUsedBytes()
		target.VramTotalBytes = source.GetVramTotalBytes()
	}
	if source.GetGpuUtilizationAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
		target.GpuUtilizationAvailability = source.GetGpuUtilizationAvailability()
		target.GpuUtilizationPercent = source.GetGpuUtilizationPercent()
	}
	if source.GetTemperatureAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
		target.TemperatureAvailability = source.GetTemperatureAvailability()
		target.TemperatureCelsius = source.GetTemperatureCelsius()
	}
	if source.GetPowerAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
		target.PowerAvailability = source.GetPowerAvailability()
		target.PowerWatts = source.GetPowerWatts()
	}
	if source.GetClockAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
		target.ClockAvailability = source.GetClockAvailability()
		target.ClockMhz = source.GetClockMhz()
	}
	if source.GetHostRamAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
		target.HostRamAvailability = source.GetHostRamAvailability()
		target.HostRamUsedBytes = source.GetHostRamUsedBytes()
		target.HostRamTotalBytes = source.GetHostRamTotalBytes()
	}
}

func markUnavailableMetrics(sample *evalv1.ProviderBoundaryHardwareSample) {
	if sample.GetVramBytesAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNSPECIFIED {
		sample.VramBytesAvailability = evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE
	}
	if sample.GetGpuUtilizationAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNSPECIFIED {
		sample.GpuUtilizationAvailability = evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE
	}
	if sample.GetTemperatureAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNSPECIFIED {
		sample.TemperatureAvailability = evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE
	}
	if sample.GetPowerAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNSPECIFIED {
		sample.PowerAvailability = evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE
	}
	if sample.GetClockAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNSPECIFIED {
		sample.ClockAvailability = evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE
	}
	if sample.GetHostRamAvailability() == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNSPECIFIED {
		sample.HostRamAvailability = evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE
	}
}
