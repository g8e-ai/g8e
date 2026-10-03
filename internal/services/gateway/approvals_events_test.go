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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// liveTap records what the SSE stream handler would receive on a web session's
// live channel.
type liveTap struct {
	mu     sync.Mutex
	events []models.SSEPublishedEvent
}

func tapWebSession(t *testing.T, ws *GatewayWebSocketHandler, webSessionID string) *liveTap {
	t.Helper()
	tap := &liveTap{}
	unregister := ws.RegisterHandler("sse:web:"+webSessionID, func(_ string, data []byte) {
		var ev models.SSEPublishedEvent
		require.NoError(t, json.Unmarshal(data, &ev))
		tap.mu.Lock()
		defer tap.mu.Unlock()
		tap.events = append(tap.events, ev)
	})
	t.Cleanup(unregister)
	return tap
}

// subjects decodes every recorded event as an approvals.changed event and
// returns the subjects in arrival order.
func (l *liveTap) subjects(t *testing.T) []models.ApprovalsChangedSubject {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]models.ApprovalsChangedSubject, 0, len(l.events))
	for _, ev := range l.events {
		assert.Zero(t, ev.ID, "approvals.changed is ephemeral and must not carry a row id")
		var push models.SSEPushPayload
		require.NoError(t, json.Unmarshal(ev.Payload, &push))
		var envelope sseEventEnvelope
		require.NoError(t, json.Unmarshal(push.Event, &envelope))
		assert.Equal(t, string(constants.EventPlatformApprovalsChanged), envelope.Type)
		var payload models.ApprovalsChangedPayload
		require.NoError(t, json.Unmarshal(envelope.Data, &payload))
		assert.False(t, payload.Timestamp.IsZero())
		out = append(out, payload.Subject)
	}
	return out
}

// fakeSuspendedStore is an in-memory SuspendedTransactionStore. Methods the
// decorator does not call stay unimplemented and panic if reached.
type fakeSuspendedStore struct {
	storage.SuspendedTransactionStore
	txs        map[string]*models.SuspendedTransaction
	expired    []*models.SuspendedTransaction
	deleteErr  error
	cleanupCnt int64
}

func (f *fakeSuspendedStore) StoreSuspendedTransaction(_ context.Context, tx *models.SuspendedTransaction) error {
	f.txs[tx.TransactionHash] = tx
	return nil
}

func (f *fakeSuspendedStore) GetSuspendedTransaction(_ context.Context, hash string) (*models.SuspendedTransaction, bool, error) {
	tx, ok := f.txs[hash]
	return tx, ok, nil
}

func (f *fakeSuspendedStore) ApproveSuspendedTransaction(_ context.Context, hash string, _ models.ApprovalProof) error {
	f.txs[hash].Approved = true
	return nil
}

func (f *fakeSuspendedStore) DeleteSuspendedTransaction(_ context.Context, hash string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.txs, hash)
	return nil
}

func (f *fakeSuspendedStore) GetExpiredSuspendedTransactions(context.Context) ([]*models.SuspendedTransaction, error) {
	return f.expired, nil
}

func (f *fakeSuspendedStore) CleanupExpiredSuspendedTransactions(context.Context) (int64, error) {
	return f.cleanupCnt, nil
}

type approvalsFixture struct {
	store    *DocumentStoreService
	ws       *GatewayWebSocketHandler
	users    *UserService
	notifier *ApprovalsChangePublisher
}

func newApprovalsFixture(t *testing.T) approvalsFixture {
	t.Helper()
	store := newDocumentStoreService(t)
	logger := testutil.NewTestLogger()
	ws := NewGatewayWebSocketHandler(logger)
	users := NewUserService(store, logger)
	return approvalsFixture{
		store:    store,
		ws:       ws,
		users:    users,
		notifier: NewApprovalsChangePublisher(store, users, NewSSEEventPublisher(nil, ws), logger),
	}
}

func TestApprovalsChanged_TransactionsReachOnlyTheOwnersLiveSessions(t *testing.T) {
	f := newApprovalsFixture(t)
	putWebSession(t, f.store, "web-owner", "user-a", time.Hour)
	putWebSession(t, f.store, "web-expired", "user-a", -time.Minute)
	putWebSession(t, f.store, "web-other", "user-b", time.Hour)
	owner := tapWebSession(t, f.ws, "web-owner")
	expired := tapWebSession(t, f.ws, "web-expired")
	other := tapWebSession(t, f.ws, "web-other")

	f.notifier.TransactionsChanged("user-a")

	assert.Equal(t, []models.ApprovalsChangedSubject{models.ApprovalsChangedTransactions}, owner.subjects(t))
	assert.Empty(t, expired.subjects(t))
	assert.Empty(t, other.subjects(t))
}

func TestApprovalsChanged_EnrollmentsGoToThePlatformOwnerOnly(t *testing.T) {
	f := newApprovalsFixture(t)
	first, err := f.users.CreateUser()
	require.NoError(t, err)
	second, err := f.users.CreateUser()
	require.NoError(t, err)
	putWebSession(t, f.store, "web-first", first.ID, time.Hour)
	putWebSession(t, f.store, "web-second", second.ID, time.Hour)
	firstTap := tapWebSession(t, f.ws, "web-first")
	secondTap := tapWebSession(t, f.ws, "web-second")

	f.notifier.EnrollmentsChanged()

	assert.Equal(t, []models.ApprovalsChangedSubject{models.ApprovalsChangedEnrollments}, firstTap.subjects(t))
	assert.Empty(t, secondTap.subjects(t))
}

func TestNotifyingSuspendedStore_AnnouncesEveryChangeToThePendingSet(t *testing.T) {
	f := newApprovalsFixture(t)
	putWebSession(t, f.store, "web-a", "user-a", time.Hour)
	putWebSession(t, f.store, "web-b", "user-b", time.Hour)
	tapA := tapWebSession(t, f.ws, "web-a")
	tapB := tapWebSession(t, f.ws, "web-b")

	inner := &fakeSuspendedStore{txs: map[string]*models.SuspendedTransaction{}}
	store := newNotifyingSuspendedStore(inner, f.notifier)
	ctx := context.Background()

	require.NoError(t, store.StoreSuspendedTransaction(ctx, &models.SuspendedTransaction{TransactionHash: "h1", UserID: "user-a"}))
	require.NoError(t, store.ApproveSuspendedTransaction(ctx, "h1", models.ApprovalProof{}))
	require.NoError(t, store.DeleteSuspendedTransaction(ctx, "h1"))

	tx := models.ApprovalsChangedTransactions
	assert.Equal(t, []models.ApprovalsChangedSubject{tx, tx, tx}, tapA.subjects(t))
	assert.Empty(t, tapB.subjects(t))

	// The expiry sweep tells each owner of an expired row once, however many
	// of their rows expired.
	inner.expired = []*models.SuspendedTransaction{
		{TransactionHash: "e1", UserID: "user-b"},
		{TransactionHash: "e2", UserID: "user-b"},
	}
	inner.cleanupCnt = 2
	deleted, err := store.CleanupExpiredSuspendedTransactions(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)
	assert.Equal(t, []models.ApprovalsChangedSubject{tx}, tapB.subjects(t))
	assert.Len(t, tapA.subjects(t), 3)
}

func TestNotifyingSuspendedStore_FailedMutationAnnouncesNothing(t *testing.T) {
	f := newApprovalsFixture(t)
	putWebSession(t, f.store, "web-a", "user-a", time.Hour)
	tap := tapWebSession(t, f.ws, "web-a")

	boom := errors.New("disk full")
	inner := &fakeSuspendedStore{
		txs:       map[string]*models.SuspendedTransaction{"h1": {TransactionHash: "h1", UserID: "user-a"}},
		deleteErr: boom,
	}
	store := newNotifyingSuspendedStore(inner, f.notifier)

	err := store.DeleteSuspendedTransaction(context.Background(), "h1")

	require.ErrorIs(t, err, boom)
	assert.Empty(t, tap.subjects(t))
}
