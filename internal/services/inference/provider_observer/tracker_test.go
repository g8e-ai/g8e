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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func TestNewTracker_RequiresCollector(t *testing.T) {
	t.Parallel()
	_, err := NewTracker(TrackerConfig{ObserverID: "observer-test"})
	require.ErrorIs(t, err, constants.ErrMissingRequiredField)
}

func TestTracker_BeginRejectsInvalidCommands(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	tracker, err := NewTracker(TrackerConfig{
		ObserverID:     "observer-test",
		Collector:      &stubCollector{},
		SampleInterval: time.Hour,
		Now:            func() time.Time { return now },
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		command   *evalv1.ProviderBoundaryObservationCommand
		wantSubstr string
	}{
		{name: "nil command", command: nil},
		{name: "empty attempt id", command: &evalv1.ProviderBoundaryObservationCommand{Phase: evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN}},
		{name: "wrong phase", command: &evalv1.ProviderBoundaryObservationCommand{ProviderAttemptId: "attempt-1", Phase: evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE}, wantSubstr: "invalid phase"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := tracker.Begin(ctx, test.command)
			require.Error(t, err)
			if test.wantSubstr != "" {
				assert.Contains(t, err.Error(), test.wantSubstr)
			}
		})
	}
}

func TestTracker_FinalizeWithoutBeginProducesTerminalSample(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	tracker, err := NewTracker(TrackerConfig{
		ObserverID:     "observer-test",
		Collector:      &stubCollector{},
		SampleInterval: time.Hour,
		Now:            func() time.Time { return now },
	})
	require.NoError(t, err)

	finalize := &evalv1.ProviderBoundaryObservationCommand{
		ProviderAttemptId:        "attempt-1",
		InferenceTransactionId:   "txn-1",
		Phase:                    evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs:   now.UnixMilli(),
		AttemptCompletedAtUnixMs: now.Add(2 * time.Second).UnixMilli(),
		AttemptStatus:            evalv1.ProviderBoundaryObservationAttemptStatus_PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_COMPLETED,
	}
	window, err := tracker.Finalize(ctx, finalize)
	require.NoError(t, err)
	require.NotEmpty(t, window.GetSamples())
	assert.NotEmpty(t, window.GetObservationDigest())
}

func TestTracker_BeginAndFinalize(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	tracker, err := NewTracker(TrackerConfig{
		ObserverID:     "observer-test",
		Collector:      &stubCollector{},
		SampleInterval: 1,
		Now:            func() time.Time { return now },
	})
	require.NoError(t, err)

	begin := &evalv1.ProviderBoundaryObservationCommand{
		ProviderAttemptId:      "attempt-1",
		Phase:                  evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_BEGIN,
		AttemptStartedAtUnixMs: now.UnixMilli(),
	}
	require.NoError(t, tracker.Begin(ctx, begin))

	finalize := &evalv1.ProviderBoundaryObservationCommand{
		ProviderAttemptId:        "attempt-1",
		InferenceTransactionId:   "txn-1",
		Phase:                    evalv1.ProviderBoundaryObservationPhase_PROVIDER_BOUNDARY_OBSERVATION_PHASE_FINALIZE,
		AttemptStartedAtUnixMs:   now.UnixMilli(),
		AttemptCompletedAtUnixMs: now.Add(2 * time.Second).UnixMilli(),
		AttemptStatus:            evalv1.ProviderBoundaryObservationAttemptStatus_PROVIDER_BOUNDARY_OBSERVATION_ATTEMPT_STATUS_COMPLETED,
	}
	window, err := tracker.Finalize(ctx, finalize)
	require.NoError(t, err)
	assert.Equal(t, "attempt-1", window.GetProviderAttemptId())
	assert.NotEmpty(t, window.GetObservationDigest())
	assert.NotEmpty(t, window.GetSamples())
}
