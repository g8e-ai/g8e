// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

//go:build integration

package serve

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestOperatorDeploymentRecorderPreservesIdentityAcrossSubscriptionChanges(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	recorder, err := NewOperatorDeploymentRecorder(fileSvc, "launch")
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, recorder.Reset(ctx))
	require.NoError(t, recorder.Record(ctx, models.OperatorDeploymentState{Phase: models.OperatorDeploymentPhasePendingApproval, RequestID: "request"}))
	require.NoError(t, recorder.Record(ctx, models.OperatorDeploymentState{Phase: models.OperatorDeploymentPhaseEnrolled, OperatorSessionID: "session"}))
	for _, connected := range []bool{true, false, true} {
		require.NoError(t, recorder.CommandSubscriptionChanged(ctx, "session", connected))
		data, err := fileSvc.ReadFile(ctx, operatorDeploymentStateRelPath())
		require.NoError(t, err)
		var state models.OperatorDeploymentState
		require.NoError(t, json.Unmarshal(data, &state))
		require.Equal(t, "launch", state.LaunchID)
		require.Equal(t, "request", state.RequestID)
		require.Equal(t, "session", state.OperatorSessionID)
		require.False(t, state.UpdatedAt.IsZero())
		phase := models.OperatorDeploymentPhaseEnrolled
		if connected {
			phase = models.OperatorDeploymentPhaseReady
		}
		require.Equal(t, phase, state.Phase)
	}
	info, err := fileSvc.Stat(ctx, operatorDeploymentStateRelPath())
	require.NoError(t, err)
	require.Equal(t, constants.PermFilePrivate, info.Mode().Perm())
}

func TestOperatorDeploymentFailureSurvivesLateReadinessCallback(t *testing.T) {
	fileSvc := newTestFileSvc(t)
	recorder, err := NewOperatorDeploymentRecorder(fileSvc, "launch")
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, recorder.Record(ctx, models.OperatorDeploymentState{Phase: models.OperatorDeploymentPhaseFailed, Error: constants.ErrOperatorDeployFailed.Error()}))
	require.NoError(t, recorder.CommandSubscriptionChanged(ctx, "session", true))
	data, err := fileSvc.ReadFile(ctx, operatorDeploymentStateRelPath())
	require.NoError(t, err)
	var state models.OperatorDeploymentState
	require.NoError(t, json.Unmarshal(data, &state))
	require.Equal(t, models.OperatorDeploymentPhaseFailed, state.Phase)
	require.Empty(t, state.OperatorSessionID)
	require.NoError(t, recorder.Reset(ctx))
	exists, err := fileSvc.FileExists(ctx, operatorDeploymentStateRelPath())
	require.NoError(t, err)
	require.False(t, exists)
}
