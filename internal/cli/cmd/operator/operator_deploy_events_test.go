// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// scriptedDeploymentEvents stands in for the Gateway: every watched launch is
// announced immediately, numbered in registration order (req-1, session-1, ...).
// Deployments in tests run one worker at a time, so the numbering follows the
// directory order.
type scriptedDeploymentEvents struct {
	events *deploymentEvents
	staged bool
	ready  constants.OperatorStatus
	mu     sync.Mutex
	n      int
}

func (s *scriptedDeploymentEvents) watch(launchID string) *deploymentWatch {
	w := s.events.watch(launchID)
	s.mu.Lock()
	s.n++
	n := s.n
	s.mu.Unlock()
	if s.staged {
		w.setRequest(fmt.Sprintf("req-%d", n))
	}
	if s.ready != "" {
		w.setReadiness(fmt.Sprintf("session-%d", n), s.ready)
	}
	return w
}

// scriptedConnector connects every deploy to events that announce staging and
// readiness as the given options say.
func scriptedConnector(staged bool, ready constants.OperatorStatus) deploymentEventsConnector {
	return func(context.Context, fs.RuntimeFileService, *config.Config) (deploymentWatcher, func(), error) {
		return &scriptedDeploymentEvents{events: newDeploymentEvents(), staged: staged, ready: ready}, func() {}, nil
	}
}

func pushFrame(t *testing.T, eventType string, data any) string {
	t.Helper()
	body, err := json.Marshal(data)
	require.NoError(t, err)
	event, err := json.Marshal(deploymentEventEnvelope{Type: eventType, Data: body})
	require.NoError(t, err)
	frame, err := json.Marshal(models.SSEPushPayload{UserID: "user-001", CliSessionID: "cli-1", Event: event})
	require.NoError(t, err)
	return string(frame)
}

func TestDeploymentEventsRouteByLaunchID(t *testing.T) {
	events := newDeploymentEvents()
	mine := events.watch("launch-mine")
	other := events.watch("launch-other")

	events.handle("", pushFrame(t, string(constants.EventPlatformApprovalsChanged),
		models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedEnrollments, DeploymentID: "launch-mine", RequestID: "req-mine"}))
	events.handle("", pushFrame(t, string(constants.EventOperatorStatusUpdatedActive),
		models.OperatorStatusUpdatedPayload{OperatorID: "op-1", Status: constants.OperatorStatusActive, DeploymentID: "launch-mine", OperatorSessionID: "session-mine"}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	requestID, err := mine.awaitStaged(ctx)
	require.NoError(t, err)
	assert.Equal(t, "req-mine", requestID)
	sessionID, err := mine.awaitReady(ctx)
	require.NoError(t, err)
	assert.Equal(t, "session-mine", sessionID)

	short, shortCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer shortCancel()
	_, err = other.awaitStaged(short)
	assert.ErrorIs(t, err, constants.ErrOperatorDeployFailed, "an event for another launch must not stage this one")
}

func TestDeploymentEventsIgnoreAnnouncementsWithoutALaunchID(t *testing.T) {
	events := newDeploymentEvents()
	w := events.watch("launch-1")

	events.handle("", pushFrame(t, string(constants.EventPlatformApprovalsChanged),
		models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedEnrollments}))
	events.handle("", pushFrame(t, string(constants.EventOperatorStatusUpdatedActive),
		models.OperatorStatusUpdatedPayload{OperatorID: "op-1", Status: constants.OperatorStatusActive}))
	events.handle("", "not json")

	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := w.awaitReady(short)
	assert.ErrorIs(t, err, constants.ErrOperatorDeployFailed)
}

func TestDeploymentWatchReadyBeforeRequestMeansAlreadyEnrolled(t *testing.T) {
	events := newDeploymentEvents()
	w := events.watch("launch-1")
	events.handle("", pushFrame(t, string(constants.EventOperatorStatusUpdatedActive),
		models.OperatorStatusUpdatedPayload{OperatorID: "op-1", Status: constants.OperatorStatusActive, DeploymentID: "launch-1", OperatorSessionID: "session-1"}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	requestID, err := w.awaitStaged(ctx)
	require.NoError(t, err)
	assert.Empty(t, requestID)
}

func TestDeploymentWatchNonActiveSubscriptionIsNotReadiness(t *testing.T) {
	events := newDeploymentEvents()
	w := events.watch("launch-1")
	events.handle("", pushFrame(t, string(constants.EventOperatorStatusUpdatedStopped),
		models.OperatorStatusUpdatedPayload{OperatorID: "op-1", Status: constants.OperatorStatusStopped, DeploymentID: "launch-1", OperatorSessionID: "session-1"}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := w.awaitReady(ctx)
	require.ErrorIs(t, err, constants.ErrOperatorDeployFailed)
	assert.Contains(t, err.Error(), "stopped")
}

func TestDeploymentWatchActiveIsNotRegressedByALaterAnnouncement(t *testing.T) {
	events := newDeploymentEvents()
	w := events.watch("launch-1")
	for _, status := range []constants.OperatorStatus{constants.OperatorStatusActive, constants.OperatorStatusStale} {
		events.handle("", pushFrame(t, "g8e.v1.operator.status.updated."+string(status),
			models.OperatorStatusUpdatedPayload{OperatorID: "op-1", Status: status, DeploymentID: "launch-1", OperatorSessionID: "session-1"}))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sessionID, err := w.awaitReady(ctx)
	require.NoError(t, err)
	assert.Equal(t, "session-1", sessionID)
}

func TestExplainDeployWaitNamesTheWorkersOwnFailure(t *testing.T) {
	dir := testutil.TempDir(t)
	fileSvc, err := fs.NewRuntimeFileService(dir, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	data, err := json.Marshal(models.OperatorDeploymentState{
		LaunchID: "launch-1", Phase: models.OperatorDeploymentPhaseFailed, Error: "HTTP 429", UpdatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.DeploymentDirname, constants.DeploymentStateFileOperator), data, constants.PermFilePrivate))

	err = explainDeployWait(deploySSH{local: true, launchID: "launch-1"}, dir, fmt.Errorf("%w: wait ended", constants.ErrOperatorDeployFailed))
	require.ErrorIs(t, err, constants.ErrOperatorDeployFailed)
	assert.Contains(t, err.Error(), "HTTP 429")
}

func TestExplainDeployWaitKeepsTheWaitErrorWhenTheWorkerRecordedNothing(t *testing.T) {
	dir := testutil.TempDir(t)
	waitErr := fmt.Errorf("%w: wait ended", constants.ErrOperatorDeployFailed)

	err := explainDeployWait(deploySSH{local: true, launchID: "launch-1"}, dir, waitErr)
	assert.Equal(t, waitErr, err)
}
