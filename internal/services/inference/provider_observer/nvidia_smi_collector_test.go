// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestNvidiaSMICollector_Collect(t *testing.T) {
	t.Parallel()
	observedAt := time.Unix(1, 0).UTC()
	tests := []struct {
		name         string
		collector    *NvidiaSMICollector
		wantReported bool
	}{
		{
			name: "parses valid csv output",
			collector: &NvidiaSMICollector{
				Command: "sh",
				Args:    []string{"-c", `echo "100,200,50,65,120,1500"`},
			},
			wantReported: true,
		},
		{
			name:         "missing command returns unavailable gpu metrics",
			collector:    &NvidiaSMICollector{Command: ""},
			wantReported: false,
		},
		{
			name: "command failure returns unavailable gpu metrics",
			collector: &NvidiaSMICollector{
				Command: "/nonexistent-binary",
			},
			wantReported: false,
		},
		{
			name: "too few fields returns unavailable gpu metrics",
			collector: &NvidiaSMICollector{
				Command: "sh",
				Args:    []string{"-c", `echo "1,2,3"`},
			},
			wantReported: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sample, err := test.collector.Collect(context.Background(), observedAt)
			require.NoError(t, err)
			require.NotNil(t, sample)
			if test.wantReported {
				assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED, sample.GetGpuUtilizationAvailability())
				assert.Equal(t, uint64(100*1024*1024), sample.GetVramUsedBytes())
				assert.Equal(t, float32(50), sample.GetGpuUtilizationPercent())
				return
			}
			assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE, sample.GetGpuUtilizationAvailability())
		})
	}
}

func TestParseFloatField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{in: " 42.5 ", want: 42.5},
		{in: "[N/A]", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			t.Parallel()
			got, err := parseFloatField(test.in)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, test.want, got, 0.0001)
		})
	}
}
