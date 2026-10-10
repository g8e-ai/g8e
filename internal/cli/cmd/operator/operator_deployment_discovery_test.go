// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

//go:build integration

package operatorcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestDeploymentReadsPendingRequestWithoutLogs(t *testing.T) {
	dir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	data, err := json.Marshal(models.OperatorDeploymentState{
		Phase: models.OperatorDeploymentPhasePendingApproval, RequestID: "resumed-request", UpdatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator), data, constants.PermFilePrivate))
	state, err := (deploySSH{local: true}).readDeploymentState(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "resumed-request", state.RequestID)
}

func TestDeploymentRejectsPreviousLaunchProgress(t *testing.T) {
	dir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(dir, slog.Default())
	require.NoError(t, err)
	data, err := json.Marshal(models.OperatorDeploymentState{
		LaunchID: "previous", Phase: models.OperatorDeploymentPhaseReady, OperatorSessionID: "old-session", UpdatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator), data, constants.PermFilePrivate))
	state, err := (deploySSH{local: true, launchID: "current"}).readDeploymentState(context.Background(), dir)
	require.NoError(t, err)
	require.Nil(t, state)
}

func TestDeploymentReadsSessionAndFailureWithoutLogs(t *testing.T) {
	for _, phase := range []models.OperatorDeploymentPhase{models.OperatorDeploymentPhaseEnrolled, models.OperatorDeploymentPhaseReady, models.OperatorDeploymentPhaseFailed} {
		t.Run(string(phase), func(t *testing.T) {
			dir := testutil.TempDir(t)
			fileSvc, err := fs.NewRuntimeFileService(dir, slog.Default())
			require.NoError(t, err)
			data, err := json.Marshal(models.OperatorDeploymentState{
				LaunchID: "current", Phase: phase, RequestID: "original-request", OperatorSessionID: "session", Error: "startup failed", UpdatedAt: time.Now().UTC(),
			})
			require.NoError(t, err)
			require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator), data, constants.PermFilePrivate))
			target := deploySSH{local: true, launchID: "current"}

			state, err := target.readDeploymentState(context.Background(), dir)
			require.NoError(t, err)
			require.Equal(t, phase, state.Phase)
			waitErr := explainDeployWait(target, dir, fmt.Errorf("%w: wait ended", constants.ErrOperatorDeployFailed))
			require.ErrorIs(t, waitErr, constants.ErrOperatorDeployFailed)
			if phase == models.OperatorDeploymentPhaseFailed {
				require.ErrorContains(t, waitErr, "startup failed")
			} else {
				require.NotContains(t, waitErr.Error(), "startup failed")
			}
		})
	}
}

func TestAwaitOperatorsOnlineReportsEachWorkersAnnouncedSession(t *testing.T) {
	events := newDeploymentEvents()
	ready, unannounced := events.watch("ready"), events.watch("unannounced")
	ready.setReadiness("session", constants.OperatorStatusActive)
	target := deploySSH{local: true}

	ops := []deployedOperator{{target: target, Dir: testutil.TempDir(t), watch: ready}}
	require.NoError(t, awaitOperatorsOnline(context.Background(), ops))
	require.Equal(t, "session", ops[0].SessionID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	ops = []deployedOperator{{target: target, Dir: testutil.TempDir(t), watch: unannounced}}
	require.ErrorIs(t, awaitOperatorsOnline(ctx, ops), constants.ErrOperatorDeployFailed)
}

func TestOperatorDeploymentStateCommandReadsOnlyRequestedRuntime(t *testing.T) {
	dir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(dir, slog.Default())
	require.NoError(t, err)
	cmd := operatorDeploymentStateCmdWithFactory(func(base string, _ *slog.Logger) (fs.RuntimeFileService, error) {
		require.Equal(t, dir, base)
		return fileSvc, nil
	})
	var output, errOutput bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&errOutput)
	cmd.SetArgs([]string{"--working-dir", dir})
	require.NoError(t, cmd.Execute())
	require.JSONEq(t, "null", output.String())
	require.False(t, fileExistsForTest(t, fileSvc), "read command must not create runtime files")
}

func fileExistsForTest(t *testing.T, fileSvc fs.RuntimeFileService) bool {
	t.Helper()
	exists, err := fileSvc.FileExists(context.Background(), "")
	require.NoError(t, err)
	return exists
}
