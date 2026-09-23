// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package evaluation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/g8e-ai/g8e/v2/internal/services/inference/provider_observer"
	"github.com/g8e-ai/g8e/v2/internal/services/storage/storagetest"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type recordingProviderObservationRemote struct {
	loadCalls []string
	window    *evalv1.ProviderBoundaryObservationWindow
	attempt   *operatorv1.InferenceProviderAttemptRecord
}

func (s *recordingProviderObservationRemote) Load(_ context.Context, providerAttemptID string) (*evalv1.ProviderBoundaryObservationWindow, *operatorv1.InferenceProviderAttemptRecord, error) {
	s.loadCalls = append(s.loadCalls, providerAttemptID)
	if s.window == nil || s.attempt == nil || s.window.GetProviderAttemptId() != providerAttemptID {
		return nil, nil, constants.ErrNotFound
	}
	return s.window, s.attempt, nil
}

func TestFallbackProviderObservationStores_DelegateLocalWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	localWindows, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	localAttempts, err := inference.NewAttemptStore(fileSvc)
	require.NoError(t, err)
	remote := &recordingProviderObservationRemote{}
	windows := &fallbackProviderObservationWindows{local: localWindows, remote: remote}
	attempts := &fallbackProviderObservationAttempts{local: localAttempts, remote: remote}

	attempt := &operatorv1.InferenceProviderAttemptRecord{
		ProviderAttemptId: "attempt-1",
		TransactionId:     "tx-1",
		Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_IN_PROGRESS,
		StartedAtUnixMs:   time.Unix(1_700_000_000, 0).UnixMilli(),
	}
	require.NoError(t, attempts.Begin(ctx, attempt))
	require.NoError(t, attempts.Complete(ctx, "attempt-1", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))

	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:       provider_observer.SchemaVersion,
		ProviderAttemptId:   "attempt-1",
		ObserverId:          "observer-test",
		ObserverClockSource: provider_observer.DefaultObserverClockSource,
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	require.NoError(t, windows.Save(ctx, window))

	loadedAttempt, err := attempts.Get(ctx, "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, "attempt-1", loadedAttempt.GetProviderAttemptId())
	loadedWindow, err := windows.Load(ctx, "attempt-1")
	require.NoError(t, err)
	assert.Equal(t, "attempt-1", loadedWindow.GetProviderAttemptId())
}

func TestFallbackProviderObservationStores_LoadRemoteOnLocalMiss(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	localWindows, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	localAttempts, err := inference.NewAttemptStore(fileSvc)
	require.NoError(t, err)
	remote := &recordingProviderObservationRemote{
		window: &evalv1.ProviderBoundaryObservationWindow{
			SchemaVersion:       provider_observer.SchemaVersion,
			ProviderAttemptId:   "attempt-remote",
			ObserverId:          "observer-test",
			ObserverClockSource: provider_observer.DefaultObserverClockSource,
		},
		attempt: &operatorv1.InferenceProviderAttemptRecord{
			ProviderAttemptId: "attempt-remote",
			Status:            operatorv1.InferenceProviderAttemptStatus_INFERENCE_PROVIDER_ATTEMPT_STATUS_COMPLETED,
		},
	}
	digest, err := provider_observer.ComputeObservationDigest(remote.window)
	require.NoError(t, err)
	remote.window.ObservationDigest = digest

	windows := &fallbackProviderObservationWindows{local: localWindows, remote: remote}
	attempts := &fallbackProviderObservationAttempts{local: localAttempts, remote: remote}

	loadedWindow, err := windows.Load(ctx, "attempt-remote")
	require.NoError(t, err)
	assert.Equal(t, "attempt-remote", loadedWindow.GetProviderAttemptId())
	loadedAttempt, err := attempts.Get(ctx, "attempt-remote")
	require.NoError(t, err)
	assert.Equal(t, "attempt-remote", loadedAttempt.GetProviderAttemptId())
	assert.Equal(t, []string{"attempt-remote", "attempt-remote"}, remote.loadCalls)
}

func TestBindProviderBoundaryObservationRefs_AttachesComplianceRef(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fileSvc := storagetest.NewTestFileSvc(t, t.TempDir())
	windowStore, err := provider_observer.NewWindowStore(fileSvc)
	require.NoError(t, err)
	window := &evalv1.ProviderBoundaryObservationWindow{
		SchemaVersion:              provider_observer.SchemaVersion,
		ProviderAttemptId:          "attempt-1",
		ObserverId:                 "observer-test",
		ObserverClockSource:        provider_observer.DefaultObserverClockSource,
		WindowStartedAtUnixNanos:   uint64(time.Unix(1_700_000_000, 0).UnixNano()),
		WindowCompletedAtUnixNanos: uint64(time.Unix(1_700_000_010, 0).UnixNano()),
	}
	digest, err := provider_observer.ComputeObservationDigest(window)
	require.NoError(t, err)
	window.ObservationDigest = digest
	require.NoError(t, windowStore.Save(ctx, window))

	reader, err := NewCampaignProviderObservationReader(fileSvc)
	require.NoError(t, err)
	result := &evalv1.EvaluationAssignmentResult{
		ModelInferences: []*evalv1.ModelInferenceRecord{{
			InferenceRecordId: "inference-1",
			ProviderAttemptId: "attempt-1",
		}},
	}
	require.NoError(t, reader.BindProviderBoundaryObservationRefs(ctx, result))
	ref := result.GetModelInferences()[0].GetProviderBoundaryObservationRef()
	require.NotNil(t, ref)
	assert.Equal(t, "attempt-1", ref.GetArtifactId())
	assert.Equal(t, digest, ref.GetSha256())
}

func TestToolScorecardMetricFromGrade_UsesClosedUnavailableReason(t *testing.T) {
	t.Parallel()
	metric := toolScorecardMetricFromGrade(&evalv1.DeterministicGrade{
		Status: evalv1.EvaluationVerdictStatus_EVALUATION_VERDICT_STATUS_UNAVAILABLE,
	})
	require.NotNil(t, metric)
	assert.Equal(t, "source_unavailable", metric.UnavailableReason)
}

func TestProviderBoundaryObservationRef_ReturnsNilForMissingWindow(t *testing.T) {
	t.Parallel()
	assert.Nil(t, providerBoundaryObservationRef(nil))
	assert.Nil(t, providerBoundaryObservationRef(&evalv1.ProviderBoundaryObservationWindow{}))
}
