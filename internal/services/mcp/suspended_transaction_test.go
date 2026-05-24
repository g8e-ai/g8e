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

package mcp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
)

// TestSuspendedTransactionStore_BasicOperations tests the core suspended transaction store operations
// which are the foundation of the OOB approval flow.
func TestSuspendedTransactionStore_BasicOperations(t *testing.T) {
	t.Parallel()

	// Setup: Create a suspended transaction store
	suspendedStore := &fakeSuspendedStore{txs: make(map[string]*models.SuspendedTransaction)}

	// Test: Store a suspended transaction
	tx := &models.SuspendedTransaction{
		TransactionHash: "test-tx-123",
		Envelope:        json.RawMessage(`{"id":"test-tx-123","action":"file_edit"}`),
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(1 * time.Hour),
		ToolName:        "test-tool",
		ToolArguments:   json.RawMessage(`{"command":"rm -rf /"}`),
		UserID:          "user-123",
		OperatorID:      "op-456",
	}
	err := suspendedStore.StoreSuspendedTransaction(tx)
	require.NoError(t, err)

	// Test: Retrieve the stored transaction
	retrievedTx, found := suspendedStore.GetSuspendedTransaction("test-tx-123")
	require.True(t, found)
	require.Equal(t, "test-tx-123", retrievedTx.TransactionHash)
	require.Equal(t, "test-tool", retrievedTx.ToolName)
	require.NotEmpty(t, retrievedTx.Envelope)
	require.NotEmpty(t, retrievedTx.ToolArguments)

	// Test: Delete the transaction (simulating approval and execution)
	err = suspendedStore.DeleteSuspendedTransaction("test-tx-123")
	require.NoError(t, err)

	// Test: Verify transaction is deleted
	_, found = suspendedStore.GetSuspendedTransaction("test-tx-123")
	require.False(t, found, "Transaction should be deleted after execution")
}

// TestSuspendedTransactionStore_ConcurrentSuspensions tests that multiple concurrent suspensions are handled correctly
func TestSuspendedTransactionStore_ConcurrentSuspensions(t *testing.T) {
	t.Parallel()

	// Setup: Create a suspended transaction store
	suspendedStore := &fakeSuspendedStore{txs: make(map[string]*models.SuspendedTransaction)}

	// Test: Store multiple concurrent transactions
	numConcurrent := 5
	txHashes := make([]string, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		txHash := "concurrent-tx-" + string(rune('0'+i))
		tx := &models.SuspendedTransaction{
			TransactionHash: txHash,
			Envelope:        json.RawMessage(`{"id":"` + txHash + `"}`),
			CreatedAt:       time.Now(),
			ExpiresAt:       time.Now().Add(1 * time.Hour),
			ToolName:        "test-tool",
			ToolArguments:   json.RawMessage(`{"command":"test ` + string(rune('a'+i)) + `"}`),
			UserID:          "user-123",
			OperatorID:      "op-456",
		}
		err := suspendedStore.StoreSuspendedTransaction(tx)
		require.NoError(t, err)
		txHashes[i] = txHash
	}

	// Verify: All transactions were stored
	require.Len(t, suspendedStore.txs, numConcurrent)

	// Verify: Each transaction can be retrieved
	for _, txHash := range txHashes {
		_, found := suspendedStore.GetSuspendedTransaction(txHash)
		require.True(t, found, "Transaction "+txHash+" should be retrievable")
	}

	// Verify: Each transaction has a unique hash
	uniqueHashes := make(map[string]bool)
	for _, hash := range txHashes {
		uniqueHashes[hash] = true
	}
	require.Len(t, uniqueHashes, numConcurrent, "All transaction hashes should be unique")
}

// TestSuspendedTransactionStore_EnvelopePersistence tests that the full envelope is persisted correctly
func TestSuspendedTransactionStore_EnvelopePersistence(t *testing.T) {
	t.Parallel()

	// Setup: Create a suspended transaction store
	suspendedStore := &fakeSuspendedStore{txs: make(map[string]*models.SuspendedTransaction)}

	// Test: Store a transaction with a full envelope
	envelope := &commonv1.GovernanceEnvelope{
		Id:        "test-envelope-123",
		Timestamp: timestamppb.Now(),
	}
	envelopeJSON, err := protojson.Marshal(envelope)
	require.NoError(t, err)

	tx := &models.SuspendedTransaction{
		TransactionHash: "test-tx-123",
		Envelope:        json.RawMessage(envelopeJSON),
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(1 * time.Hour),
		ToolName:        "test-tool",
		ToolArguments:   json.RawMessage(`{"command":"test"}`),
		UserID:          "user-123",
		OperatorID:      "op-456",
	}
	err = suspendedStore.StoreSuspendedTransaction(tx)
	require.NoError(t, err)

	// Test: Retrieve and verify envelope persistence
	retrievedTx, found := suspendedStore.GetSuspendedTransaction("test-tx-123")
	require.True(t, found)
	require.NotEmpty(t, retrievedTx.Envelope)

	// Test: Envelope can be parsed back into a GovernanceEnvelope
	var parsedEnvelope commonv1.GovernanceEnvelope
	err = protojson.Unmarshal(retrievedTx.Envelope, &parsedEnvelope)
	require.NoError(t, err, "Persisted envelope should be valid protojson")
	require.Equal(t, "test-envelope-123", parsedEnvelope.Id)
}

// TestSuspendedTransactionStore_Expiry tests that expired transactions can be identified
func TestSuspendedTransactionStore_Expiry(t *testing.T) {
	t.Parallel()

	// Setup: Create a suspended transaction store
	suspendedStore := &fakeSuspendedStore{txs: make(map[string]*models.SuspendedTransaction)}

	// Test: Store an expired transaction
	expiredTx := &models.SuspendedTransaction{
		TransactionHash: "expired-tx-123",
		Envelope:        json.RawMessage(`{"id":"expired-tx-123"}`),
		CreatedAt:       time.Now().Add(-2 * time.Hour),
		ExpiresAt:       time.Now().Add(-1 * time.Hour),
		ToolName:        "test-tool",
		ToolArguments:   json.RawMessage(`{"command":"test"}`),
		UserID:          "user-123",
		OperatorID:      "op-456",
	}
	err := suspendedStore.StoreSuspendedTransaction(expiredTx)
	require.NoError(t, err)

	// Test: Store a valid transaction
	validTx := &models.SuspendedTransaction{
		TransactionHash: "valid-tx-456",
		Envelope:        json.RawMessage(`{"id":"valid-tx-456"}`),
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(1 * time.Hour),
		ToolName:        "test-tool",
		ToolArguments:   json.RawMessage(`{"command":"test"}`),
		UserID:          "user-123",
		OperatorID:      "op-456",
	}
	err = suspendedStore.StoreSuspendedTransaction(validTx)
	require.NoError(t, err)

	// Test: Both transactions can be retrieved
	_, found := suspendedStore.GetSuspendedTransaction("expired-tx-123")
	require.True(t, found, "Expired transaction should still be in store")

	_, found = suspendedStore.GetSuspendedTransaction("valid-tx-456")
	require.True(t, found, "Valid transaction should be in store")

	// Test: Verify expiry timestamps
	retrievedExpiredTx, _ := suspendedStore.GetSuspendedTransaction("expired-tx-123")
	require.True(t, retrievedExpiredTx.ExpiresAt.Before(time.Now()), "Expired transaction should have past expiry time")

	retrievedValidTx, _ := suspendedStore.GetSuspendedTransaction("valid-tx-456")
	require.True(t, retrievedValidTx.ExpiresAt.After(time.Now()), "Valid transaction should have future expiry time")
}
