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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/marshaler"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

const (
	statusEventOwner = "user-owner"
	statusEventOther = "user-other"
)

// recordedOperatorEvent is one operator status event decoded from the SSE
// event store.
type recordedOperatorEvent struct {
	eventType string
	payload   models.OperatorStatusUpdatedPayload
}

// transitionRecorder is an OperatorStatusObserver that keeps what it is told.
type transitionRecorder struct {
	mu          sync.Mutex
	transitions []OperatorStatusTransition
}

func (r *transitionRecorder) OperatorStatusChanged(t OperatorStatusTransition) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transitions = append(r.transitions, t)
}

func (r *transitionRecorder) recorded() []OperatorStatusTransition {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]OperatorStatusTransition(nil), r.transitions...)
}

// putWebSession persists a web session for userID that expires expiresIn from
// now (negative for an already expired session) and returns its id.
func putWebSession(t *testing.T, store *DocumentStoreService, id, userID string, expiresIn time.Duration) {
	t.Helper()
	body, err := json.Marshal(models.WebSession{
		ID:              id,
		UserID:          userID,
		CreatedAtUnixMs: time.Now().Add(-time.Hour).UnixMilli(),
		ExpiresAtUnixMs: time.Now().Add(expiresIn).UnixMilli(),
	})
	require.NoError(t, err)
	require.NoError(t, store.DocSet(marshaler.CollectionName(constants.CollectionWebSessions), id, body))
}

// putSilentOperator persists an active remote Operator owned by userID whose
// last heartbeat is long past the stale window.
func putSilentOperator(t *testing.T, store *DocumentStoreService, _, _, _ string) {
	t.Helper()
	op := remoteOperator(constants.OperatorStatusActive)
	op.UserId = statusEventOwner
	op.Name = "edge-1"
	op.LastHeartbeatAt = timeAgo(constants.OperatorHeartbeatStaleAfter * 2)
	putOperator(t, store, "op-silent", op, time.Hour)
}

// webSessionEvents returns the operator status events delivered to one web
// session, oldest first.
func webSessionEvents(t *testing.T, ls *GatewayModeService, userID, webSessionID string) []recordedOperatorEvent {
	t.Helper()
	rows, err := ls.sseStore.SSEEventsListSince(SSERoute{UserID: userID, WebSessionID: webSessionID}, 0, 100)
	require.NoError(t, err)

	events := make([]recordedOperatorEvent, 0, len(rows))
	for _, row := range rows {
		var push models.SSEPushPayload
		require.NoError(t, json.Unmarshal([]byte(row.Payload), &push))
		var envelope sseEventEnvelope
		require.NoError(t, json.Unmarshal(push.Event, &envelope))
		assert.Equal(t, row.EventType, envelope.Type)

		var payload models.OperatorStatusUpdatedPayload
		require.NoError(t, json.Unmarshal(envelope.Data, &payload))
		events = append(events, recordedOperatorEvent{eventType: envelope.Type, payload: payload})
	}
	return events
}

func TestOperatorStatusEvents_StaleTransitionReachesOnlyOwnerLiveWebSessions(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()

	putWebSession(t, store, "web-owner-1", statusEventOwner, time.Hour)
	putWebSession(t, store, "web-owner-2", statusEventOwner, time.Hour)
	putWebSession(t, store, "web-owner-expired", statusEventOwner, -time.Minute)
	putWebSession(t, store, "web-other", statusEventOther, time.Hour)
	putSilentOperator(t, store, "op-silent", statusEventOwner, "edge-1")

	_, err := store.DocQuery(operatorsCollection, nil, "", 0)
	require.NoError(t, err)

	for _, sessionID := range []string{"web-owner-1", "web-owner-2"} {
		events := webSessionEvents(t, ls, statusEventOwner, sessionID)
		require.Len(t, events, 1, sessionID)
		assert.Equal(t, string(constants.EventOperatorStatusUpdatedStale), events[0].eventType)
		assert.Equal(t, "op-silent", events[0].payload.OperatorID)
		assert.Equal(t, constants.OperatorStatusStale, events[0].payload.Status)
		assert.Equal(t, "edge-1", events[0].payload.Name)
		assert.False(t, events[0].payload.Timestamp.IsZero())
	}
	assert.Empty(t, webSessionEvents(t, ls, statusEventOwner, "web-owner-expired"))
	assert.Empty(t, webSessionEvents(t, ls, statusEventOther, "web-other"))
}

func TestOperatorStatusEvents_ReaderThatMarksOperatorStaleReportsTheTransition(t *testing.T) {
	store := newDocumentStoreService(t)
	recorder := &transitionRecorder{}
	require.NoError(t, store.BindOperatorStatusObserver(recorder))
	putSilentOperator(t, store, "op-silent", statusEventOwner, "edge-1")

	_, err := store.DocGet(operatorsCollection, "op-silent")
	require.NoError(t, err)

	assert.Equal(t, []OperatorStatusTransition{{
		OperatorID: "op-silent",
		UserID:     statusEventOwner,
		Name:       "edge-1",
		Status:     constants.OperatorStatusStale,
	}}, recorder.recorded())
}

func TestOperatorStatusEvents_AlreadyStaleOperatorIsNotReportedAgain(t *testing.T) {
	store := newDocumentStoreService(t)
	recorder := &transitionRecorder{}
	require.NoError(t, store.BindOperatorStatusObserver(recorder))
	putSilentOperator(t, store, "op-silent", statusEventOwner, "edge-1")

	_, err := store.DocQuery(operatorsCollection, nil, "", 0)
	require.NoError(t, err)
	_, err = store.DocQuery(operatorsCollection, nil, "", 0)
	require.NoError(t, err)
	_, err = store.DocGet(operatorsCollection, "op-silent")
	require.NoError(t, err)

	assert.Len(t, recorder.recorded(), 1)
}

func TestOperatorStatusEvents_StopAndTerminateReachOwnerWebSession(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)

	for _, id := range []string{"op-stopped", "op-terminated"} {
		op := remoteOperator(constants.OperatorStatusActive)
		op.UserId = statusEventOwner
		op.Name = id
		putOperator(t, store, id, op, time.Second)
	}

	require.NoError(t, ls.reg.MarkOperatorStopped("op-stopped", statusEventOwner, "maintenance"))
	require.NoError(t, ls.reg.TerminateOperator("op-terminated", statusEventOwner, "decommissioned"))

	events := webSessionEvents(t, ls, statusEventOwner, "web-owner")
	require.Len(t, events, 2)
	assert.Equal(t, string(constants.EventOperatorStatusUpdatedStopped), events[0].eventType)
	assert.Equal(t, "op-stopped", events[0].payload.OperatorID)
	assert.Equal(t, constants.OperatorStatusStopped, events[0].payload.Status)
	assert.Equal(t, string(constants.EventOperatorStatusUpdatedTerminated), events[1].eventType)
	assert.Equal(t, "op-terminated", events[1].payload.OperatorID)
	assert.Equal(t, constants.OperatorStatusTerminated, events[1].payload.Status)
}

func TestOperatorStatusEvents_StopByNonOwnerIsNotPushed(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)
	op := remoteOperator(constants.OperatorStatusActive)
	op.UserId = statusEventOwner
	putOperator(t, store, "op-1", op, time.Second)

	require.ErrorIs(t, ls.reg.MarkOperatorStopped("op-1", statusEventOther, ""), constants.ErrRegistrationOperatorNotBelongToUser)

	assert.Empty(t, webSessionEvents(t, ls, statusEventOwner, "web-owner"))
}

func TestOperatorStatusEvents_HeartbeatRecoveryReachesOwnerWebSession(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)

	stale := remoteOperator(constants.OperatorStatusStale)
	stale.UserId = statusEventOwner
	stale.Name = "edge-1"
	stale.LastHeartbeatAt = timeAgo(constants.OperatorHeartbeatStaleAfter * 10)
	putOperator(t, store, "op-stale", stale, time.Hour)

	publishTestHeartbeat(t, ls, "op-stale")

	events := webSessionEvents(t, ls, statusEventOwner, "web-owner")
	require.Len(t, events, 1)
	assert.Equal(t, string(constants.EventOperatorStatusUpdatedActive), events[0].eventType)
	assert.Equal(t, "op-stale", events[0].payload.OperatorID)
	assert.Equal(t, constants.OperatorStatusActive, events[0].payload.Status)
}

func assertActiveAnnouncement(t *testing.T, ls *GatewayModeService, ownerID, operatorID, name string) {
	t.Helper()
	events := webSessionEvents(t, ls, ownerID, "web-owner")
	require.Len(t, events, 1)
	assert.Equal(t, string(constants.EventOperatorStatusUpdatedActive), events[0].eventType)
	assert.Equal(t, operatorID, events[0].payload.OperatorID)
	assert.Equal(t, constants.OperatorStatusActive, events[0].payload.Status)
	assert.Equal(t, name, events[0].payload.Name)
	assert.False(t, events[0].payload.Timestamp.IsZero())
	assert.Empty(t, webSessionEvents(t, ls, ownerID, "web-owner-expired"))
	assert.Empty(t, webSessionEvents(t, ls, statusEventOther, "web-other"))
}

func TestOperatorStatusEvents_EnrollmentAnnouncesActiveToOwner(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	operatorCSR, operatorKey, cliCSR, cliKey := generateOperatorCSRsAndKeys(t)
	_, token, approved := createAndApproveRequest(t, env,
		models.PlatformComponentOperator, "operator-1", "operator.local", "", operatorCSR, cliCSR)
	putWebSession(t, env.docStore, "web-owner", env.ownerID, time.Hour)
	putWebSession(t, env.docStore, "web-owner-expired", env.ownerID, -time.Minute)
	putWebSession(t, env.docStore, "web-other", statusEventOther, time.Hour)
	require.Empty(t, webSessionEvents(t, env.svc, env.ownerID, "web-owner"))

	resp, err := env.enrollSvc.Complete(context.Background(), token, models.PlatformEnrollmentProofs{
		Operator: signCompletionTranscript(t, approved, operatorKey),
		CLI:      signCompletionTranscript(t, approved, cliKey),
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Operator)
	assertActiveAnnouncement(t, env.svc, env.ownerID, resp.Operator.OperatorID, "operator.local")
}

func TestOperatorStatusEvents_SlotClaimAnnouncesActiveToOwner(t *testing.T) {
	env := setupPlatformEnrollmentEnv(t, true)
	owner, err := env.userSvc.GetByID(env.ownerID)
	require.NoError(t, err)
	slot, err := env.svc.reg.createSlot(env.ownerID, owner.OrganizationID)
	require.NoError(t, err)
	putWebSession(t, env.docStore, "web-owner", env.ownerID, time.Hour)
	putWebSession(t, env.docStore, "web-owner-expired", env.ownerID, -time.Minute)
	putWebSession(t, env.docStore, "web-other", statusEventOther, time.Hour)
	require.Empty(t, webSessionEvents(t, env.svc, env.ownerID, "web-owner"))
	operatorCSR, _, _, _ := generateOperatorCSRsAndKeys(t)

	resp, err := env.svc.reg.RegisterDeviceCSR(env.ownerID, owner.OrganizationID, models.OperatorRegistrationRequest{
		SystemFingerprint: "slot-claim-fingerprint",
		Hostname:          "slot-host",
		CSR:               operatorCSR,
	})
	require.NoError(t, err)
	require.True(t, resp.Success)
	require.Equal(t, slot.Id, resp.OperatorID, "claim the pre-created offline slot")
	assertActiveAnnouncement(t, env.svc, env.ownerID, slot.Id, slot.Name)
}

func TestOperatorStatusEvents_HeartbeatFromHealthyOperatorIsNotPushed(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)
	op := remoteOperator(constants.OperatorStatusActive)
	op.UserId = statusEventOwner
	putOperator(t, store, "op-healthy", op, time.Second)

	publishTestHeartbeat(t, ls, "op-healthy")

	assert.Empty(t, webSessionEvents(t, ls, statusEventOwner, "web-owner"))
}

func TestOperatorStatusEvents_DeadlineTimerPushesStaleOperatorWithoutAReader(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)
	putSilentOperator(t, store, "op-silent", statusEventOwner, "edge-1")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, store.WatchOperatorStaleness(ctx))

	// The Operator is already past its deadline, so its timer fires at once.
	require.Eventually(t, func() bool {
		return len(webSessionEvents(t, ls, statusEventOwner, "web-owner")) == 1
	}, 5*time.Second, 10*time.Millisecond)

	// The stale transition cancelled the timer, so nothing more is pushed.
	store.staleness.mu.Lock()
	armed := len(store.staleness.timers)
	store.staleness.mu.Unlock()
	assert.Zero(t, armed)
}

func TestOperatorStatusEvents_HeartbeatKeepsDeadlineTimerPendingAndOperatorActive(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)
	op := remoteOperator(constants.OperatorStatusActive)
	op.UserId = statusEventOwner
	op.LastHeartbeatAt = timeAgo(constants.OperatorHeartbeatStaleAfter * 2)
	putOperator(t, store, "op-beating", op, time.Hour)
	publishTestHeartbeat(t, ls, "op-beating")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, store.WatchOperatorStaleness(ctx))

	store.staleness.mu.Lock()
	timer := store.staleness.timers["op-beating"]
	store.staleness.mu.Unlock()
	assert.NotNil(t, timer, "a fresh heartbeat leaves a pending deadline timer")
	assert.Empty(t, webSessionEvents(t, ls, statusEventOwner, "web-owner"))
}

func TestOperatorStatusEvents_FiredTimerAfterLateHeartbeatKeepsOperatorActiveAndRearms(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	store := ls.GetDocStore()
	putWebSession(t, store, "web-owner", statusEventOwner, time.Hour)
	op := remoteOperator(constants.OperatorStatusActive)
	op.UserId = statusEventOwner
	op.LastHeartbeatAt = timeAgo(time.Second)
	putOperator(t, store, "op-beating", op, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, store.WatchOperatorStaleness(ctx))

	// The timer fires although the persisted deadline has not passed, as when a
	// heartbeat lands just before it fires.
	store.operatorWentSilent("op-beating")

	doc, err := store.docGet(operatorsCollection, "op-beating")
	require.NoError(t, err)
	opDoc, err := models.OperatorDocumentFromStore(doc)
	require.NoError(t, err)
	assert.Equal(t, string(constants.OperatorStatusActive), opDoc.GetStatus())
	assert.Empty(t, webSessionEvents(t, ls, statusEventOwner, "web-owner"))

	store.staleness.mu.Lock()
	timer := store.staleness.timers["op-beating"]
	store.staleness.mu.Unlock()
	assert.NotNil(t, timer, "an Operator still active keeps a pending deadline timer")
}

func TestOperatorStatusPublisher_RejectsStatusWithoutGatewayEvent(t *testing.T) {
	ls := newTestGatewayService(t, testGatewayOpts{})
	publisher := NewOperatorStatusPublisher(ls.GetDocStore(), NewSSEEventPublisher(ls.sseStore, ls.pubsub), ls.logger)

	for _, status := range []constants.OperatorStatus{
		constants.OperatorStatusBound,
		constants.OperatorStatusOffline,
		constants.OperatorStatusAvailable,
		constants.OperatorStatusUnavailable,
	} {
		err := publisher.Publish(OperatorStatusTransition{OperatorID: "op-1", UserID: statusEventOwner, Status: status})
		assert.ErrorIs(t, err, constants.ErrOperatorStatusEventUnsupported, status)
	}
}

func TestDocumentStore_BindOperatorStatusObserver_RejectsNilAndSecondBinding(t *testing.T) {
	store := newDocumentStoreService(t)

	require.ErrorIs(t, store.BindOperatorStatusObserver(nil), constants.ErrOperatorStatusObserverNil)
	require.NoError(t, store.BindOperatorStatusObserver(&transitionRecorder{}))
	assert.ErrorIs(t, store.BindOperatorStatusObserver(&transitionRecorder{}), constants.ErrOperatorStatusObserverBound)
}
