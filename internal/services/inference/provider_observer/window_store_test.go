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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestAttemptRecordFromObservationWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		window     *evalv1.ProviderBoundaryObservationWindow
		wantNil    bool
		wantStatus operatorv1.InferenceProviderAttemptStatus
	}{
		{name: "nil window", window: nil, wantNil: true},
		{name: "empty attempt id", window: &evalv1.ProviderBoundaryObservationWindow{}, wantNil: true},
		{
			name: "completed attempt",
			window: &evalv1.ProviderBoundaryObservationWindow{
				ProviderAttemptId:        "attempt-1",
				AttemptStartedAtUnixMs:   1,
				AttemptCompletedAtUnixMs: 2,
			},
			wantStatus: operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		},
		{
			name: "in progress attempt",
			window: &evalv1.ProviderBoundaryObservationWindow{
				ProviderAttemptId:      "attempt-1",
				AttemptStartedAtUnixMs: 1,
			},
			wantStatus: operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_IN_PROGRESS,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			record := AttemptRecordFromObservationWindow(test.window)
			if test.wantNil {
				assert.Nil(t, record)
				return
			}
			require.NotNil(t, record)
			assert.Equal(t, test.window.GetProviderAttemptId(), record.GetProviderAttemptId())
			assert.Equal(t, test.wantStatus, record.GetStatus())
		})
	}
}

func TestValidateObservationWindow_RejectsInvalidBindings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		mutate     func(*evalv1.ProviderBoundaryObservationWindow)
		wantSubstr string
	}{
		{
			name:       "nil window",
			mutate:     func(_ *evalv1.ProviderBoundaryObservationWindow) {},
			wantSubstr: constants.ErrMissingRequiredField.Error(),
		},
		{
			name: "unsupported schema version",
			mutate: func(window *evalv1.ProviderBoundaryObservationWindow) {
				window.SchemaVersion = "9.9.9"
			},
			wantSubstr: "unsupported schema version",
		},
		{
			name: "digest mismatch",
			mutate: func(window *evalv1.ProviderBoundaryObservationWindow) {
				window.ObservationDigest = "bad-digest"
			},
			wantSubstr: "digest mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var window *evalv1.ProviderBoundaryObservationWindow
			if test.name == "nil window" {
				err := ValidateObservationWindow(nil)
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantSubstr)
				return
			}
			window = &evalv1.ProviderBoundaryObservationWindow{
				SchemaVersion:              SchemaVersion,
				ProviderAttemptId:          "attempt-1",
				ObserverId:                 "observer-test",
				WindowStartedAtUnixNanos:   1,
				WindowCompletedAtUnixNanos: 2,
				Samples: []*evalv1.ProviderBoundaryHardwareSample{
					{ObservedAtUnixNanos: 1, HostRamAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED},
				},
			}
			digest, err := ComputeObservationDigest(window)
			require.NoError(t, err)
			window.ObservationDigest = digest
			test.mutate(window)
			err = ValidateObservationWindow(window)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantSubstr)
		})
	}
}

func TestWindowStore_SaveLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	store, err := NewWindowStore(fileSvc)
	require.NoError(t, err)

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
	require.NoError(t, store.Save(ctx, window))

	loaded, err := store.Load(ctx, "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, window.GetProviderAttemptId(), loaded.GetProviderAttemptId())
	assert.Equal(t, window.GetObservationDigest(), loaded.GetObservationDigest())
}

func TestProcMeminfoCollector_CollectFromFixture(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	meminfoPath := filepath.Join(dir, "meminfo")
	require.NoError(t, os.WriteFile(meminfoPath, []byte("MemTotal:       8192 kB\nMemAvailable:    4096 kB\n"), 0o644))

	sample, err := (&ProcMeminfoCollector{Path: meminfoPath}).Collect(context.Background(), time.Unix(1, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED, sample.GetHostRamAvailability())
	assert.Equal(t, uint64(4096*1024), sample.GetHostRamUsedBytes())
	assert.Equal(t, uint64(8192*1024), sample.GetHostRamTotalBytes())
}

func TestProcMeminfoCollector_ReturnsUnavailableForMissingTotal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	meminfoPath := filepath.Join(dir, "meminfo")
	require.NoError(t, os.WriteFile(meminfoPath, []byte("MemAvailable: 4096 kB\n"), 0o644))

	sample, err := (&ProcMeminfoCollector{Path: meminfoPath}).Collect(context.Background(), time.Unix(1, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE, sample.GetHostRamAvailability())
}

func TestParseMeminfoKB(t *testing.T) {
	t.Parallel()
	tests := []struct {
		line string
		want uint64
	}{
		{line: "MemTotal:       16384 kB", want: 16384},
		{line: "MemTotal:", want: 0},
		{line: "MemTotal: abc kB", want: 0},
	}
	for _, test := range tests {
		t.Run(test.line, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, parseMeminfoKB(test.line))
		})
	}
}
