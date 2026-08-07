// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration

package gateway

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

type stubSuspendedTransactionStore struct {
	txs     []*models.SuspendedTransaction
	getTx   *models.SuspendedTransaction
	found   bool
	getErr  error
	listErr error
}

func (s *stubSuspendedTransactionStore) StoreSuspendedTransaction(_ context.Context, _ *models.SuspendedTransaction) error {
	return nil
}

func (s *stubSuspendedTransactionStore) GetSuspendedTransaction(_ context.Context, _ string) (*models.SuspendedTransaction, bool, error) {
	return s.getTx, s.found, s.getErr
}

func (s *stubSuspendedTransactionStore) ListSuspendedTransactions(_ context.Context, _ string) ([]*models.SuspendedTransaction, error) {
	return s.txs, s.listErr
}

func (s *stubSuspendedTransactionStore) ApproveSuspendedTransaction(_ context.Context, _ string, _ models.ApprovalProof) error {
	return nil
}

func (s *stubSuspendedTransactionStore) DeleteSuspendedTransaction(_ context.Context, _ string) error {
	return nil
}

func (s *stubSuspendedTransactionStore) CleanupExpiredSuspendedTransactions(_ context.Context) (int64, error) {
	return 0, nil
}

func (s *stubSuspendedTransactionStore) GetExpiredSuspendedTransactions(_ context.Context) ([]*models.SuspendedTransaction, error) {
	return nil, nil
}

func TestPasskeyOrchestrator_GetSuspendedTransaction(t *testing.T) {
	t.Run("delegates to MCPServiceProvider", func(t *testing.T) {
		expectedTx := &models.SuspendedTransaction{TransactionHash: "tx-123", UserID: "u-1"}
		mock := &mockMCPServiceProvider{suspendedTx: expectedTx, found: true}
		o := NewPasskeyOrchestrator(mock, nil, nil, nil, testutil.NewTestLogger())

		tx, found, err := o.GetSuspendedTransaction(context.Background(), "tx-123")
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, expectedTx, tx)
	})

	t.Run("returns not-found when MCPServiceProvider returns not found", func(t *testing.T) {
		mock := &mockMCPServiceProvider{suspendedTx: nil, found: false}
		o := NewPasskeyOrchestrator(mock, nil, nil, nil, testutil.NewTestLogger())

		tx, found, err := o.GetSuspendedTransaction(context.Background(), "missing")
		require.NoError(t, err)
		assert.False(t, found)
		assert.Nil(t, tx)
	})
}

func TestPasskeyOrchestrator_ResumeWithL3Proof(t *testing.T) {
	t.Run("delegates to MCPServiceProvider", func(t *testing.T) {
		expectedReceipt := &operatorv1.ActionReceipt{TransactionHash: "tx-123"}
		mock := &mockMCPServiceProvider{receipt: expectedReceipt}
		o := NewPasskeyOrchestrator(mock, nil, nil, nil, testutil.NewTestLogger())

		proof := &commonv1.L3Proof{CredentialId: "cred-1"}
		receipt, err := o.ResumeWithL3Proof(context.Background(), "tx-123", "u-1", proof)
		require.NoError(t, err)
		assert.Equal(t, expectedReceipt, receipt)
	})

	t.Run("returns error from MCPServiceProvider", func(t *testing.T) {
		mock := &mockMCPServiceProvider{err: assert.AnError}
		o := NewPasskeyOrchestrator(mock, nil, nil, nil, testutil.NewTestLogger())

		proof := &commonv1.L3Proof{CredentialId: "cred-1"}
		_, err := o.ResumeWithL3Proof(context.Background(), "tx-123", "u-1", proof)
		assert.Error(t, err)
	})
}

func TestPasskeyOrchestrator_ListSuspendedTransactions(t *testing.T) {
	t.Run("delegates to SuspendedTransactionStore", func(t *testing.T) {
		expectedTxs := []*models.SuspendedTransaction{
			{TransactionHash: "tx-1", UserID: "u-1"},
			{TransactionHash: "tx-2", UserID: "u-1"},
		}
		store := &stubSuspendedTransactionStore{txs: expectedTxs}
		o := NewPasskeyOrchestrator(nil, store, nil, nil, testutil.NewTestLogger())

		txs, err := o.ListSuspendedTransactions(context.Background(), "u-1")
		require.NoError(t, err)
		assert.Equal(t, expectedTxs, txs)
	})

	t.Run("returns error from store", func(t *testing.T) {
		store := &stubSuspendedTransactionStore{listErr: assert.AnError}
		o := NewPasskeyOrchestrator(nil, store, nil, nil, testutil.NewTestLogger())

		_, err := o.ListSuspendedTransactions(context.Background(), "u-1")
		assert.Error(t, err)
	})

	t.Run("returns empty list when no transactions", func(t *testing.T) {
		store := &stubSuspendedTransactionStore{txs: nil}
		o := NewPasskeyOrchestrator(nil, store, nil, nil, testutil.NewTestLogger())

		txs, err := o.ListSuspendedTransactions(context.Background(), "u-empty")
		require.NoError(t, err)
		assert.Empty(t, txs)
	})
}

func TestPasskeyOrchestrator_EmitApprovalCompletedSSE_NoOpGuards(t *testing.T) {
	t.Run("no-ops when sseStore is nil", func(t *testing.T) {
		_, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })
		sseStore := NewSSEEventService(stores.DB, logger)
		o := NewPasskeyOrchestrator(nil, nil, nil, pubsub, logger)

		o.EmitApprovalCompletedSSE("u-1", "cli-1", "tx-1")

		events, err := sseStore.SSEEventsListSince(SSERoute{UserID: "u-1", CLISessionID: "cli-1"}, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, events)
	})

	t.Run("no-ops when pubsub is nil", func(t *testing.T) {
		_, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		sseStore := NewSSEEventService(stores.DB, logger)
		o := NewPasskeyOrchestrator(nil, nil, sseStore, nil, logger)

		o.EmitApprovalCompletedSSE("u-1", "cli-1", "tx-1")

		events, err := sseStore.SSEEventsListSince(SSERoute{UserID: "u-1", CLISessionID: "cli-1"}, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, events)
	})

	t.Run("no-ops when userID is empty", func(t *testing.T) {
		_, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		sseStore := NewSSEEventService(stores.DB, logger)
		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })
		o := NewPasskeyOrchestrator(nil, nil, sseStore, pubsub, logger)

		o.EmitApprovalCompletedSSE("", "cli-1", "tx-1")

		events, err := sseStore.SSEEventsListSince(SSERoute{UserID: "u-1", CLISessionID: "cli-1"}, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, events)
	})
}

func TestPasskeyOrchestrator_EmitPasskeyRegisteredSSE_NoOpGuards(t *testing.T) {
	t.Run("no-ops when sseStore is nil", func(t *testing.T) {
		_, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		pubsub := NewGatewayWebSocketHandler(logger)
		t.Cleanup(func() { pubsub.Close() })
		sseStore := NewSSEEventService(stores.DB, logger)
		o := NewPasskeyOrchestrator(nil, nil, nil, pubsub, logger)

		o.EmitPasskeyRegisteredSSE("u-1", "cli-1")

		events, err := sseStore.SSEEventsListSince(SSERoute{UserID: "u-1", CLISessionID: "cli-1"}, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, events)
	})

	t.Run("no-ops when pubsub is nil", func(t *testing.T) {
		_, stores := newTestDB(t)
		logger := testutil.NewTestLogger()
		sseStore := NewSSEEventService(stores.DB, logger)
		o := NewPasskeyOrchestrator(nil, nil, sseStore, nil, logger)

		o.EmitPasskeyRegisteredSSE("u-1", "cli-1")

		events, err := sseStore.SSEEventsListSince(SSERoute{UserID: "u-1", CLISessionID: "cli-1"}, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, events)
	})
}
