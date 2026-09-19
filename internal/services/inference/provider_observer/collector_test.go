// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type fixedHardwareCollector struct {
	sample *evalv1.ProviderBoundaryHardwareSample
	err    error
}

func (c *fixedHardwareCollector) Collect(_ context.Context, observedAt time.Time) (*evalv1.ProviderBoundaryHardwareSample, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.sample == nil {
		return nil, nil
	}
	clone := *c.sample
	if clone.ObservedAtUnixNanos == 0 {
		clone.ObservedAtUnixNanos = uint64(observedAt.UTC().UnixNano())
	}
	return &clone, nil
}

func reportedGPUSample(device string) *evalv1.ProviderBoundaryHardwareSample {
	return &evalv1.ProviderBoundaryHardwareSample{
		DevicePseudonym:            device,
		GpuUtilizationAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		GpuUtilizationPercent:      42,
		VramBytesAvailability:      evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		VramUsedBytes:              100,
		VramTotalBytes:             200,
	}
}

func reportedRAMSample() *evalv1.ProviderBoundaryHardwareSample {
	return &evalv1.ProviderBoundaryHardwareSample{
		HostRamAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		HostRamUsedBytes:    512,
		HostRamTotalBytes:   1024,
	}
}

func TestCompositeCollector_Collect(t *testing.T) {
	t.Parallel()
	observedAt := time.Unix(1_700_000_000, 0).UTC()
	tests := []struct {
		name           string
		gpu            HardwareCollector
		ram            HardwareCollector
		wantDevice     string
		wantGPU        evalv1.ProviderHardwareMetricAvailability
		wantRAM        evalv1.ProviderHardwareMetricAvailability
		wantGPUPercent float32
		wantRAMUsed    uint64
	}{
		{
			name:           "merges gpu and ram samples",
			gpu:            &fixedHardwareCollector{sample: reportedGPUSample("gpu-a")},
			ram:            &fixedHardwareCollector{sample: reportedRAMSample()},
			wantDevice:     "gpu-a",
			wantGPU:        evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			wantRAM:        evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			wantGPUPercent: 42,
			wantRAMUsed:    512,
		},
		{
			name:       "nil gpu marks gpu metrics unavailable",
			gpu:        nil,
			ram:        &fixedHardwareCollector{sample: reportedRAMSample()},
			wantDevice: DefaultDevicePseudonym,
			wantGPU:    evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
			wantRAM:    evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			wantRAMUsed: 512,
		},
		{
			name:       "gpu collector error keeps ram metrics",
			gpu:        &fixedHardwareCollector{err: errors.New("gpu unavailable")},
			ram:        &fixedHardwareCollector{sample: reportedRAMSample()},
			wantDevice: DefaultDevicePseudonym,
			wantGPU:    evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
			wantRAM:    evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			wantRAMUsed: 512,
		},
		{
			name:       "nil ram marks host ram unavailable",
			gpu:        &fixedHardwareCollector{sample: reportedGPUSample("gpu-b")},
			ram:        nil,
			wantDevice: "gpu-b",
			wantGPU:    evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			wantRAM:    evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
			wantGPUPercent: 42,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			collector := &CompositeCollector{GPU: test.gpu, RAM: test.ram}
			sample, err := collector.Collect(context.Background(), observedAt)
			require.NoError(t, err)
			require.NotNil(t, sample)
			assert.Equal(t, uint64(observedAt.UTC().UnixNano()), sample.GetObservedAtUnixNanos())
			assert.Equal(t, test.wantDevice, sample.GetDevicePseudonym())
			assert.Equal(t, test.wantGPU, sample.GetGpuUtilizationAvailability())
			assert.Equal(t, test.wantRAM, sample.GetHostRamAvailability())
			if test.wantGPU == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
				assert.Equal(t, test.wantGPUPercent, sample.GetGpuUtilizationPercent())
			}
			if test.wantRAM == evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED {
				assert.Equal(t, test.wantRAMUsed, sample.GetHostRamUsedBytes())
			}
		})
	}
}

func TestCompositeCollector_MarksRemainingMetricsUnavailable(t *testing.T) {
	t.Parallel()
	sample, err := (&CompositeCollector{}).Collect(context.Background(), time.Unix(1, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE, sample.GetVramBytesAvailability())
	assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE, sample.GetTemperatureAvailability())
	assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE, sample.GetPowerAvailability())
	assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE, sample.GetClockAvailability())
}
