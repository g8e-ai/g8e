// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

type stubCollector struct {
	samples []*evalv1.ProviderBoundaryHardwareSample
	index   int
}

func (s *stubCollector) Collect(_ context.Context, observedAt time.Time) (*evalv1.ProviderBoundaryHardwareSample, error) {
	if len(s.samples) == 0 {
		return &evalv1.ProviderBoundaryHardwareSample{
			ObservedAtUnixNanos: uint64(observedAt.UTC().UnixNano()),
			HostRamAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
			HostRamUsedBytes:    1,
			HostRamTotalBytes:   2,
		}, nil
	}
	if s.index >= len(s.samples) {
		s.index = len(s.samples) - 1
	}
	sample := s.samples[s.index]
	s.index++
	return sample, nil
}

func TestComputeObservationDigest_IsStable(t *testing.T) {
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              SchemaVersion,
		ProviderAttemptId:          "attempt-1",
		ObserverId:                 "observer-test",
		ObserverClockSource:        DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   1,
		WindowCompletedAtUnixNanos: 2,
		AttemptStartedAtUnixMs:     3,
		AttemptCompletedAtUnixMs:   4,
		Samples: []*evalv1.ProviderBoundaryHardwareSample{
			{
				ObservedAtUnixNanos: 1,
				HostRamAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
				HostRamUsedBytes:    10,
				HostRamTotalBytes:   20,
			},
		},
	}
	digest, err := ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	require.NoError(t, ValidateObservationWindow(window))
}

func TestHostRAMCollector_ReadsHostRAM(t *testing.T) {
	t.Parallel()
	sample, err := NewHostRAMCollector().Collect(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, sample)
	assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED, sample.GetHostRamAvailability())
	assert.Greater(t, sample.GetHostRamTotalBytes(), uint64(0))
}

func TestProcMeminfoCollector_ReadsHostRAM(t *testing.T) {
	if _, err := os.Stat("/proc/meminfo"); err != nil {
		t.Skip("proc meminfo unavailable")
	}
	sample, err := NewProcMeminfoCollector().Collect(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED, sample.GetHostRamAvailability())
	assert.Greater(t, sample.GetHostRamTotalBytes(), uint64(0))
}
