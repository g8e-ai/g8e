// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	pubsubv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/pubsub/v1"
)

// putCLISession persists an unexpired CLI session of userID.
func putCLISession(t *testing.T, store *DocumentStoreService, id, userID string) {
	t.Helper()
	body, err := json.Marshal(models.CLISession{
		ID: id, UserID: userID, IsActive: true,
		CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, store.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), id, body))
}

// connectCLIStream simulates the open SSE stream of CLI session id, which is
// what makes the session a recipient of live status events.
func connectCLIStream(t *testing.T, ls *GatewayModeService, id string) {
	t.Helper()
	t.Cleanup(ls.GetGatewayWebSocketHandler().RegisterHandler(sseCLIChannelPrefix+id, func(string, []byte) {}))
}

// cliSessionEvents returns the operator status events recorded for one CLI session.
func cliSessionEvents(t *testing.T, ls *GatewayModeService, userID, cliSessionID string) []recordedOperatorEvent {
	t.Helper()
	rows, err := ls.sseStore.SSEEventsListSince(SSERoute{UserID: userID, CLISessionID: cliSessionID}, 0, 100)
	require.NoError(t, err)
	events := make([]recordedOperatorEvent, 0, len(rows))
	for _, row := range rows {
		var push models.SSEPushPayload
		require.NoError(t, json.Unmarshal([]byte(row.Payload), &push))
		var envelope sseEventEnvelope
		require.NoError(t, json.Unmarshal(push.Event, &envelope))
		var payload models.OperatorStatusUpdatedPayload
		require.NoError(t, json.Unmarshal(envelope.Data, &payload))
		events = append(events, recordedOperatorEvent{eventType: envelope.Type, payload: payload})
	}
	return events
}

func putSubscribableOperator(t *testing.T, store *DocumentStoreService, status constants.OperatorStatus) {
	t.Helper()
	op := remoteOperator(status)
	op.UserId = statusEventOwner
	op.Name = "edge-1"
	op.OperatorSessionId = "sess-1"
	op.LastHeartbeatAt = timeAgo(0)
	putOperator(t, store, "op-1", op, time.Hour)
}

func TestOperatorCommandSubscribed_AnnouncesReadinessToOwnerCLIAndWebSessions(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putCLISession(t, store, "cli-owner", statusEventOwner)
	putCLISession(t, store, "cli-other", statusEventOther)
	connectCLIStream(t, ls, "cli-owner")
	connectCLIStream(t, ls, "cli-other")
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)
	putSubscribableOperator(t, store, constants.OperatorStatusActive)

	require.NoError(t, store.OperatorCommandSubscribed("op-1", "sess-1", "launch-1"))

	for name, events := range map[string][]recordedOperatorEvent{
		"owner CLI session": cliSessionEvents(t, ls, statusEventOwner, "cli-owner"),
		"owner web session": webSessionEvents(t, ls, statusEventOwner, "web-owner"),
	} {
		require.Len(t, events, 1, name)
		assert.Equal(t, string(constants.EventOperatorStatusUpdatedActive), events[0].eventType, name)
		assert.Equal(t, "op-1", events[0].payload.OperatorID, name)
		assert.Equal(t, "launch-1", events[0].payload.DeploymentID, name)
		assert.Equal(t, "sess-1", events[0].payload.OperatorSessionID, name)
	}
	assert.Empty(t, cliSessionEvents(t, ls, statusEventOther, "cli-other"), "another user's CLI must not hear it")
}

func TestOperatorCommandSubscribed_ReportsTheDocumentsStatusNotReadiness(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putCLISession(t, store, "cli-owner", statusEventOwner)
	connectCLIStream(t, ls, "cli-owner")
	putSubscribableOperator(t, store, constants.OperatorStatusStopped)

	require.NoError(t, store.OperatorCommandSubscribed("op-1", "sess-1", "launch-1"))

	events := cliSessionEvents(t, ls, statusEventOwner, "cli-owner")
	require.Len(t, events, 1)
	assert.Equal(t, string(constants.EventOperatorStatusUpdatedStopped), events[0].eventType)
	assert.Equal(t, constants.OperatorStatusStopped, events[0].payload.Status)
}

func TestOperatorCommandSubscribed_StaleSessionAndUnknownOperatorAnnounceNothing(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putCLISession(t, store, "cli-owner", statusEventOwner)
	connectCLIStream(t, ls, "cli-owner")
	putSubscribableOperator(t, store, constants.OperatorStatusActive)

	require.NoError(t, store.OperatorCommandSubscribed("op-1", "sess-superseded", "launch-1"))
	assert.Empty(t, cliSessionEvents(t, ls, statusEventOwner, "cli-owner"))

	require.ErrorIs(t, store.OperatorCommandSubscribed("op-missing", "sess-1", "launch-1"), constants.ErrNotFound)
}

func TestOperatorStatusEvents_EnrollmentActiveAnnouncementReachesOwnerCLISessionWithoutLaunchKey(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putCLISession(t, store, "cli-owner", statusEventOwner)
	putCLISession(t, store, "cli-other", statusEventOther)
	connectCLIStream(t, ls, "cli-owner")
	connectCLIStream(t, ls, "cli-other")
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)

	store.NotifyOperatorEnrolled("op-1", statusEventOwner, "edge-1")

	require.Len(t, webSessionEvents(t, ls, statusEventOwner, "web-owner"), 1)
	events := cliSessionEvents(t, ls, statusEventOwner, "cli-owner")
	require.Len(t, events, 1, "the TUI re-lists Operators on any status transition")
	assert.Empty(t, events[0].payload.DeploymentID)
	assert.Empty(t, cliSessionEvents(t, ls, statusEventOther, "cli-other"), "another user's CLI must not hear it")
}

// Enrollment creates a CLI session per worker and none of them opens a stream.
// A status event must cost one durable row for the one connected consumer, not
// one per session on record.
func TestOperatorStatusEvents_WorkerCLISessionsWithoutAStreamGetNoRows(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putCLISession(t, store, "cli-owner", statusEventOwner)
	connectCLIStream(t, ls, "cli-owner")
	const workers = 50
	for i := range workers {
		putCLISession(t, store, fmt.Sprintf("cli-worker-%d", i), statusEventOwner)
	}

	store.NotifyOperatorEnrolled("op-1", statusEventOwner, "edge-1")

	require.Len(t, cliSessionEvents(t, ls, statusEventOwner, "cli-owner"), 1)
	for i := range workers {
		assert.Empty(t, cliSessionEvents(t, ls, statusEventOwner, fmt.Sprintf("cli-worker-%d", i)))
	}
}

func TestApprovalsChanged_EnrollmentRequestedCarriesTheLaunchToOwnerCLISession(t *testing.T) {
	f := newApprovalsFixture(t)
	owner, err := f.users.CreateUser()
	require.NoError(t, err)
	putCLISession(t, f.store, "cli-owner", owner.ID)
	tap := &liveTap{}
	unregister := f.ws.RegisterHandler("sse:cli:cli-owner", func(_ string, data []byte) {
		var ev models.SSEPublishedEvent
		require.NoError(t, json.Unmarshal(data, &ev))
		tap.mu.Lock()
		defer tap.mu.Unlock()
		tap.events = append(tap.events, ev)
	})
	t.Cleanup(unregister)

	f.notifier.EnrollmentRequested("req-1", "launch-1")

	tap.mu.Lock()
	defer tap.mu.Unlock()
	require.Len(t, tap.events, 1)
	var push models.SSEPushPayload
	require.NoError(t, json.Unmarshal(tap.events[0].Payload, &push))
	var envelope sseEventEnvelope
	require.NoError(t, json.Unmarshal(push.Event, &envelope))
	assert.Equal(t, string(constants.EventPlatformApprovalsChanged), envelope.Type)
	var payload models.ApprovalsChangedPayload
	require.NoError(t, json.Unmarshal(envelope.Data, &payload))
	assert.Equal(t, models.ApprovalsChangedEnrollments, payload.Subject)
	assert.Equal(t, "launch-1", payload.DeploymentID)
	assert.Equal(t, "req-1", payload.RequestID)
}

func TestPubSubCommandSubscription_AnnouncesOnlyDeployLaunchedWorkers(t *testing.T) {
	type announced struct{ operatorID, sessionID, deploymentID string }
	var got []announced
	broker := NewGatewayWebSocketHandler(testutil.NewTestLogger())
	broker.SetCommandSubscribedHandler(func(operatorID, sessionID, deploymentID string) {
		got = append(got, announced{operatorID, sessionID, deploymentID})
	})
	newHandler := func(deploymentID string) *pubSubSessionHandler {
		return &pubSubSessionHandler{broker: broker, sub: &wsSubscriber{
			buf:              newDropOldestBuf(10),
			done:             make(chan struct{}),
			identitySPIFFEID: "spiffe://g8e.local/operator/op-1",
			operatorID:       "op-1",
			deploymentID:     deploymentID,
		}}
	}
	subscribe := func(h *pubSubSessionHandler, channel string) {
		h.handleAction(&pubsubv1.PubSubMessage{Action: constants.PubSubActionSubscribe, Channel: channel})
	}

	subscribe(newHandler(""), "cmd:op-1:sess-1")
	subscribe(newHandler("launch-1"), "results:op-1:sess-1")
	subscribe(newHandler("launch-1"), "cmd:op-1:sess-1")

	assert.Equal(t, []announced{{"op-1", "sess-1", "launch-1"}}, got,
		"only a cmd: subscription from a connection that presented a launch ID is announced")
}

func operatorCreateRequest(t *testing.T, deploymentID string) models.PlatformEnrollmentCreateRequest {
	t.Helper()
	operatorCSR, _, cliCSR, _ := generateOperatorCSRsAndKeys(t)
	return models.PlatformEnrollmentCreateRequest{
		ComponentKind:     models.PlatformComponentOperator,
		InstanceID:        "operator-1",
		Hostname:          "operator.local",
		SystemFingerprint: "host-fingerprint",
		DeploymentID:      deploymentID,
		Operator:          &models.PlatformOperatorCSRPayload{OperatorCSRPEM: operatorCSR, CLICSRPEM: cliCSR},
	}
}

func TestPlatformEnrollmentCreateRequest_AnnouncesTheLaunchToTheOwnersCLISessionOnCreateAndOnDeduplication(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	putCLISession(t, env.docStore, "cli-owner", env.ownerID)
	var mu sync.Mutex
	var payloads []models.ApprovalsChangedPayload
	unregister := env.svc.GetGatewayWebSocketHandler().RegisterHandler("sse:cli:cli-owner", func(_ string, data []byte) {
		var ev models.SSEPublishedEvent
		require.NoError(t, json.Unmarshal(data, &ev))
		var push models.SSEPushPayload
		require.NoError(t, json.Unmarshal(ev.Payload, &push))
		var envelope sseEventEnvelope
		require.NoError(t, json.Unmarshal(push.Event, &envelope))
		var payload models.ApprovalsChangedPayload
		require.NoError(t, json.Unmarshal(envelope.Data, &payload))
		mu.Lock()
		defer mu.Unlock()
		payloads = append(payloads, payload)
	})
	t.Cleanup(unregister)
	const launchID = "35fe96f6-cb3c-4e7e-a392-ed72e84ac9ad"

	req := operatorCreateRequest(t, launchID)
	created, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)
	// The same worker restarting under a new launch finds its live request and
	// must still be announced under the new launch ID.
	const restartedLaunchID = "7c0f2f4e-1c6b-4a52-9c52-6d7a6d1d6a11"
	req.DeploymentID = restartedLaunchID
	deduplicated, err := env.enrollSvc.CreateRequest(context.Background(), req, "https://gateway.local/console")
	require.NoError(t, err)
	require.Equal(t, created.RequestID, deduplicated.RequestID)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, payloads, 2)
	assert.Equal(t, launchID, payloads[0].DeploymentID)
	assert.Equal(t, created.RequestID, payloads[0].RequestID)
	assert.Equal(t, restartedLaunchID, payloads[1].DeploymentID)
	assert.Equal(t, created.RequestID, payloads[1].RequestID)
}

func TestPlatformEnrollmentCreateRequest_RejectsAMalformedLaunchID(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)

	_, err := env.enrollSvc.CreateRequest(context.Background(), operatorCreateRequest(t, "not-a-uuid"), "https://gateway.local/console")

	require.ErrorIs(t, err, constants.ErrPlatformEnrollmentInvalidDeploymentID)
}
