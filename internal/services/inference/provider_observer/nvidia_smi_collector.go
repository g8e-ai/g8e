// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// NvidiaSMICollector reads GPU telemetry through read-only nvidia-smi queries.
type NvidiaSMICollector struct {
	Command string
	Args    []string
}

// NewNvidiaSMICollector constructs the default provider-boundary GPU collector.
func NewNvidiaSMICollector() *NvidiaSMICollector {
	return &NvidiaSMICollector{
		Command: "nvidia-smi",
		Args: []string{
			"--query-gpu=memory.used,memory.total,utilization.gpu,temperature.gpu,power.draw,clocks.sm",
			"--format=csv,noheader,nounits",
		},
	}
}

func (c *NvidiaSMICollector) Collect(ctx context.Context, observedAt time.Time) (*evalv1.ProviderBoundaryHardwareSample, error) {
	if c == nil || c.Command == "" {
		return unavailableGPUSample(observedAt), nil
	}
	cmd := exec.CommandContext(ctx, c.Command, c.Args...)
	output, err := cmd.Output()
	if err != nil {
		return unavailableGPUSample(observedAt), nil
	}
	line := strings.TrimSpace(string(output))
	if line == "" {
		return unavailableGPUSample(observedAt), nil
	}
	fields := strings.Split(line, ",")
	if len(fields) < 6 {
		return unavailableGPUSample(observedAt), nil
	}
	vramUsedMiB, err1 := parseFloatField(fields[0])
	vramTotalMiB, err2 := parseFloatField(fields[1])
	utilization, err3 := parseFloatField(fields[2])
	temperature, err4 := parseFloatField(fields[3])
	power, err5 := parseFloatField(fields[4])
	clock, err6 := parseFloatField(fields[5])
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil || err5 != nil || err6 != nil {
		return unavailableGPUSample(observedAt), nil
	}
	return &evalv1.ProviderBoundaryHardwareSample{
		ObservedAtUnixNanos:        uint64(observedAt.UTC().UnixNano()),
		DevicePseudonym:            DefaultDevicePseudonym,
		VramBytesAvailability:      evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		VramUsedBytes:              uint64(vramUsedMiB * 1024 * 1024),
		VramTotalBytes:             uint64(vramTotalMiB * 1024 * 1024),
		GpuUtilizationAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		GpuUtilizationPercent:      float32(utilization),
		TemperatureAvailability:    evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		TemperatureCelsius:         float32(temperature),
		PowerAvailability:          evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		PowerWatts:                 float32(power),
		ClockAvailability:          evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		ClockMhz:                   uint32(clock),
	}, nil
}

func unavailableGPUSample(observedAt time.Time) *evalv1.ProviderBoundaryHardwareSample {
	sample := &evalv1.ProviderBoundaryHardwareSample{
		ObservedAtUnixNanos: uint64(observedAt.UTC().UnixNano()),
		DevicePseudonym:     DefaultDevicePseudonym,
	}
	markUnavailableMetrics(sample)
	sample.HostRamAvailability = evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNSPECIFIED
	return sample
}

func parseFloatField(value string) (float64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "[N/A]") {
		return 0, fmt.Errorf("unavailable metric")
	}
	return strconv.ParseFloat(trimmed, 64)
}
