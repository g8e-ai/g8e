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
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
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

func TestRunner_FinalizesCompletedAttempt(t *testing.T) {
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	attempt := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-1",
		TransactionId:     "txn-1",
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		StartedAtUnixMs:   time.Unix(1_700_000_000, 0).UnixMilli(),
		CompletedAtUnixMs: time.Unix(1_700_000_010, 0).UnixMilli(),
	}
	require.NoError(t, writeAttemptRecord(ctx, fileSvc, attempt))

	windowStore, err := NewWindowStore(fileSvc)
	require.NoError(t, err)
	now := time.Unix(1_700_000_005, 0).UTC()
	runner, err := NewRunner(RunnerConfig{
		ObserverID:     "observer-test",
		Collector:      &stubCollector{},
		WindowStore:    windowStore,
		FileSvc:        fileSvc,
		SampleInterval: 1,
		PollInterval:   1,
		Now:            func() time.Time { return now },
	})
	require.NoError(t, err)
	require.NoError(t, runner.poll(ctx))

	window, err := windowStore.Load(ctx, "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, "attempt-1", window.GetProviderAttemptId())
	assert.NotEmpty(t, window.GetObservationDigest())
	assert.NotEmpty(t, window.GetSamples())

	report, err := VerifyObservationCoverage(window, attempt)
	require.NoError(t, err)
	assert.True(t, report.Complete)
	assert.True(t, report.HostRAMReported)
}

func writeAttemptRecord(ctx context.Context, fileSvc fs.RuntimeFileService, record *operatorv1.InferenceProviderAttemptRecord) error {
	dir := filepath.Join(constants.DataDirname, constants.InferenceDirname, constants.InferenceAttemptsDirname)
	if err := fileSvc.MkdirAll(ctx, dir, constants.PermDirStandard); err != nil {
		return err
	}
	body, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(record)
	if err != nil {
		return err
	}
	return fileSvc.WriteFile(ctx, filepath.Join(dir, record.GetProviderAttemptId()+constants.FileExtJSON), body, constants.PermFilePrivate)
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
