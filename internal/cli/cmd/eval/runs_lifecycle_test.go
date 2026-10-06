// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	"context"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

const otherHostPID = 9999

// holdRun records a lease on a run for a process that the fake process control
// reports as alive.
func (e *runEnv) holdRun(t *testing.T, runID string) {
	t.Helper()
	_, err := e.store(t).AcquireRunLease(context.Background(), evaluation.RunLease{
		RunID:     runID,
		PID:       otherHostPID,
		Host:      testHost,
		StartedAt: e.deps.now(),
		LogPath:   path.Join(constants.EvaluationQueueLogsDirname, runID, "execution-1.txt"),
	}, nil)
	require.NoError(t, err)
	e.control.setAlive(otherHostPID, true)
}

func (e *runEnv) writeLog(t *testing.T, relPath, body string) {
	t.Helper()
	fileSvc := e.fileSvc(t)
	require.NoError(t, fileSvc.MkdirAll(context.Background(), path.Dir(relPath), constants.PermDirPrivate))
	file, err := fileSvc.OpenForAppend(context.Background(), relPath, constants.PermFilePrivate)
	require.NoError(t, err)
	_, err = file.WriteString(body)
	require.NoError(t, err)
	require.NoError(t, file.Close())
}

func waitForCancelRequest(t *testing.T, env *runEnv, runID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		lease, err := env.store(t).LoadRunLease(context.Background(), runID)
		return err == nil && lease.CancelRequested()
	}, 5*time.Second, 2*time.Millisecond, "cancel never recorded a request on the lease")
}
