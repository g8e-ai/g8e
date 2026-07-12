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

package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
)

// setupTestSuspendedTransactionStore creates a real SuspendedTransactionService with a temporary database.
func setupTestSuspendedTransactionStore(t *testing.T) *SuspendedTransactionService {
	t.Helper()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_suspended_transactions.db")

	config := &SuspendedTransactionConfig{
		DBPath:               dbPath,
		MaxDBSizeMB:          256,
		RetentionDays:        7,
		PruneIntervalMinutes: 30,
	}

	logger := testutil.NewTestLogger()
	sts, err := NewSuspendedTransactionService(config, logger)
	require.NoError(t, err)
	require.NotNil(t, sts)

	t.Cleanup(func() {
		sts.Close()
	})

	return sts
}

func TestDefaultSuspendedTransactionConfig(t *testing.T) {
	t.Parallel()

	config := DefaultSuspendedTransactionConfig()

	require.NotNil(t, config)
	assert.Equal(t, constants.SuspendedTransactionDBPath, config.DBPath)
	assert.Equal(t, int64(256), config.MaxDBSizeMB)
	assert.Equal(t, 7, config.RetentionDays)
	assert.Equal(t, 30, config.PruneIntervalMinutes)
}

func TestNewSuspendedTransactionService_NilConfig(t *testing.T) {
	t.Parallel()

	logger := testutil.NewTestLogger()
	sts, err := NewSuspendedTransactionService(nil, logger)

	require.NoError(t, err)
	require.NotNil(t, sts)
	assert.NotNil(t, sts.config)
}

func TestNewSuspendedTransactionService_InvalidDBPath(t *testing.T) {
	t.Parallel()

	// Create a file to use as parent directory — os.MkdirAll will fail because
	// the path component is a file, not a directory. This is cross-platform.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0644))

	config := &SuspendedTransactionConfig{
		DBPath: filepath.Join(blocker, "suspended.db"),
	}

	logger := testutil.NewTestLogger()
	sts, err := NewSuspendedTransactionService(config, logger)

	require.Error(t, err)
	assert.Nil(t, sts)
	assert.Contains(t, err.Error(), "failed to initialize database")
}

func TestStoreSuspendedTransaction_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService
	ctx := context.Background()
	tx := &models.SuspendedTransaction{}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestStoreSuspendedTransaction_NilDB(t *testing.T) {
	t.Parallel()

	sts := &SuspendedTransactionService{
		db: nil,
	}
	ctx := context.Background()
	tx := &models.SuspendedTransaction{}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestStoreSuspendedTransaction_Success(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	tx := &models.SuspendedTransaction{
		TransactionHash:         "test-hash-123",
		Envelope:                []byte("test-envelope"),
		CreatedAt:               time.Now().UTC(),
		ExpiresAt:               time.Now().UTC().Add(time.Hour),
		ToolName:                "test_tool",
		ToolArguments:           []byte(`{"arg": "value"}`),
		UserID:                  "user-123",
		OperatorID:              "operator-456",
		Approved:                false,
		ExpectedCertFingerprint: "cert-fingerprint",
	}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)
}

func TestStoreSuspendedTransaction_UpdateExisting(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	tx := &models.SuspendedTransaction{
		TransactionHash:         "test-hash-update",
		Envelope:                []byte("original-envelope"),
		CreatedAt:               time.Now().UTC(),
		ExpiresAt:               time.Now().UTC().Add(time.Hour),
		ToolName:                "test_tool",
		ToolArguments:           []byte(`{"arg": "value"}`),
		UserID:                  "user-123",
		OperatorID:              "operator-456",
		Approved:                false,
		ExpectedCertFingerprint: "cert-fingerprint",
	}

	// Store initial transaction
	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Update the transaction
	tx.Envelope = []byte("updated-envelope")
	tx.ExpectedCertFingerprint = "updated-fingerprint"
	err = sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Verify the update
	retrieved, found, err := sts.GetSuspendedTransaction(ctx, "test-hash-update")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "updated-envelope", string(retrieved.Envelope))
	assert.Equal(t, "updated-fingerprint", retrieved.ExpectedCertFingerprint)
}

func TestGetSuspendedTransaction_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService
	ctx := context.Background()

	tx, found, err := sts.GetSuspendedTransaction(ctx, "test-hash")
	require.Error(t, err)
	assert.False(t, found)
	assert.Nil(t, tx)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestGetSuspendedTransaction_NilDB(t *testing.T) {
	t.Parallel()

	sts := &SuspendedTransactionService{
		db: nil,
	}
	ctx := context.Background()

	tx, found, err := sts.GetSuspendedTransaction(ctx, "test-hash")
	require.Error(t, err)
	assert.False(t, found)
	assert.Nil(t, tx)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestGetSuspendedTransaction_NotFound(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	tx, found, err := sts.GetSuspendedTransaction(ctx, "nonexistent-hash")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, tx)
}

func TestGetSuspendedTransaction_Expired(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	tx := &models.SuspendedTransaction{
		TransactionHash: "expired-hash",
		Envelope:        []byte("test-envelope"),
		CreatedAt:       time.Now().UTC(),
		ExpiresAt:       time.Now().UTC().Add(-time.Hour), // Already expired
		ToolName:        "test_tool",
	}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Try to retrieve expired transaction
	retrieved, found, err := sts.GetSuspendedTransaction(ctx, "expired-hash")
	require.NoError(t, err)
	assert.False(t, found, "expired transaction should not be found")
	assert.Nil(t, retrieved)
}

func TestGetSuspendedTransaction_Success(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	approvedAt := now.Add(5 * time.Minute)

	tx := &models.SuspendedTransaction{
		TransactionHash:         "get-test-hash",
		Envelope:                []byte("test-envelope"),
		CreatedAt:               now,
		ExpiresAt:               now.Add(time.Hour),
		ToolName:                "test_tool",
		ToolArguments:           []byte(`{"arg": "value"}`),
		UserID:                  "user-123",
		OperatorID:              "operator-456",
		Approved:                true,
		ApprovedAt:              &approvedAt,
		ApprovedBy:              "approver-789",
		ApprovalSignature:       "signature-abc",
		ExpectedCertFingerprint: "cert-fingerprint",
	}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Retrieve the transaction
	retrieved, found, err := sts.GetSuspendedTransaction(ctx, "get-test-hash")
	require.NoError(t, err)
	assert.True(t, found)
	require.NotNil(t, retrieved)

	assert.Equal(t, "get-test-hash", retrieved.TransactionHash)
	assert.Equal(t, "test-envelope", string(retrieved.Envelope))
	assert.Equal(t, "test_tool", retrieved.ToolName)
	assert.Equal(t, `{"arg": "value"}`, string(retrieved.ToolArguments))
	assert.Equal(t, "user-123", retrieved.UserID)
	assert.Equal(t, "operator-456", retrieved.OperatorID)
	assert.True(t, retrieved.Approved)
	assert.Equal(t, "approver-789", retrieved.ApprovedBy)
	assert.Equal(t, "signature-abc", retrieved.ApprovalSignature)
	assert.Equal(t, "cert-fingerprint", retrieved.ExpectedCertFingerprint)
	assert.NotNil(t, retrieved.ApprovedAt)
}

func TestListSuspendedTransactions_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService
	ctx := context.Background()

	txs, err := sts.ListSuspendedTransactions(ctx, "user-123")
	require.Error(t, err)
	assert.Nil(t, txs)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestListSuspendedTransactions_NilDB(t *testing.T) {
	t.Parallel()

	sts := &SuspendedTransactionService{
		db: nil,
	}
	ctx := context.Background()

	txs, err := sts.ListSuspendedTransactions(ctx, "user-123")
	require.Error(t, err)
	assert.Nil(t, txs)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestListSuspendedTransactions_Empty(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	txs, err := sts.ListSuspendedTransactions(ctx, "")
	require.NoError(t, err)
	// Accept either nil or empty slice
	if txs != nil {
		assert.Empty(t, txs)
	}
}

func TestListSuspendedTransactions_All(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create multiple transactions for different users
	transactions := []*models.SuspendedTransaction{
		{
			TransactionHash: "hash-1",
			Envelope:        []byte("env-1"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "hash-2",
			Envelope:        []byte("env-2"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-2",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "hash-3",
			Envelope:        []byte("env-3"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-2",
		},
	}

	for _, tx := range transactions {
		err := sts.StoreSuspendedTransaction(ctx, tx)
		require.NoError(t, err)
	}

	// List all transactions
	txs, err := sts.ListSuspendedTransactions(ctx, "")
	require.NoError(t, err)
	assert.Len(t, txs, 3)
}

func TestListSuspendedTransactions_ByUser(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create transactions for different users
	transactions := []*models.SuspendedTransaction{
		{
			TransactionHash: "hash-user-1",
			Envelope:        []byte("env-1"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "hash-user-2",
			Envelope:        []byte("env-2"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-2",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "hash-user-3",
			Envelope:        []byte("env-3"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-2",
		},
	}

	for _, tx := range transactions {
		err := sts.StoreSuspendedTransaction(ctx, tx)
		require.NoError(t, err)
	}

	// List transactions for user-1 only
	txs, err := sts.ListSuspendedTransactions(ctx, "user-1")
	require.NoError(t, err)
	assert.Len(t, txs, 2)

	// Verify all returned transactions belong to user-1
	for _, tx := range txs {
		assert.Equal(t, "user-1", tx.UserID)
	}
}

func TestListSuspendedTransactions_ExcludesExpired(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create active and expired transactions
	transactions := []*models.SuspendedTransaction{
		{
			TransactionHash: "active-hash",
			Envelope:        []byte("active-env"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "expired-hash",
			Envelope:        []byte("expired-env"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(-time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
	}

	for _, tx := range transactions {
		err := sts.StoreSuspendedTransaction(ctx, tx)
		require.NoError(t, err)
	}

	// List should only return active transactions
	txs, err := sts.ListSuspendedTransactions(ctx, "")
	require.NoError(t, err)
	assert.Len(t, txs, 1)
	assert.Equal(t, "active-hash", txs[0].TransactionHash)
}

func TestApproveSuspendedTransaction_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService
	ctx := context.Background()

	err := sts.ApproveSuspendedTransaction(ctx, "hash", models.ApprovalProof{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestApproveSuspendedTransaction_NilDB(t *testing.T) {
	t.Parallel()

	sts := &SuspendedTransactionService{
		db: nil,
	}
	ctx := context.Background()

	err := sts.ApproveSuspendedTransaction(ctx, "hash", models.ApprovalProof{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestApproveSuspendedTransaction_NotFound(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	err := sts.ApproveSuspendedTransaction(ctx, "nonexistent-hash", models.ApprovalProof{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction not found or expired")
}

func TestApproveSuspendedTransaction_Expired(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	tx := &models.SuspendedTransaction{
		TransactionHash: "expired-approval-hash",
		Envelope:        []byte("env"),
		CreatedAt:       now,
		ExpiresAt:       now.Add(-time.Hour),
		UserID:          "user-1",
		OperatorID:      "op-1",
	}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Try to approve expired transaction
	err = sts.ApproveSuspendedTransaction(ctx, "expired-approval-hash", models.ApprovalProof{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction not found or expired")
}

func TestApproveSuspendedTransaction_Success(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	tx := &models.SuspendedTransaction{
		TransactionHash: "approval-test-hash",
		Envelope:        []byte("env"),
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Hour),
		UserID:          "user-1",
		OperatorID:      "op-1",
		Approved:        false,
	}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Approve the transaction
	proof := models.ApprovalProof{
		ApprovedBy:        "approver-123",
		CliSignature:      "signature-abc",
		CertFingerprint:   "fingerprint-xyz",
		ApprovalPublicKey: "pubkey-abc",
		CredentialID:      "cred-id-1",
		ClientDataJSON:    "client-data",
		AuthenticatorData: "auth-data",
		Signature:         "passkey-sig",
	}
	err = sts.ApproveSuspendedTransaction(ctx, "approval-test-hash", proof)
	require.NoError(t, err)

	// Verify the approval
	retrieved, found, err := sts.GetSuspendedTransaction(ctx, "approval-test-hash")
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, retrieved.Approved)
	assert.Equal(t, "approver-123", retrieved.ApprovedBy)
	assert.Equal(t, "signature-abc", retrieved.ApprovalSignature)
	assert.Equal(t, "fingerprint-xyz", retrieved.ExpectedCertFingerprint)
	assert.Equal(t, "pubkey-abc", retrieved.ApprovalPublicKey)
	assert.Equal(t, "cred-id-1", retrieved.PasskeyCredentialID)
	assert.Equal(t, "client-data", retrieved.PasskeyClientDataJSON)
	assert.Equal(t, "auth-data", retrieved.PasskeyAuthenticatorData)
	assert.Equal(t, "passkey-sig", retrieved.PasskeySignature)
	assert.NotNil(t, retrieved.ApprovedAt)
}

func TestDeleteSuspendedTransaction_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService
	ctx := context.Background()

	err := sts.DeleteSuspendedTransaction(ctx, "hash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestDeleteSuspendedTransaction_NilDB(t *testing.T) {
	t.Parallel()

	sts := &SuspendedTransactionService{
		db: nil,
	}
	ctx := context.Background()

	err := sts.DeleteSuspendedTransaction(ctx, "hash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestDeleteSuspendedTransaction_Success(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	tx := &models.SuspendedTransaction{
		TransactionHash: "delete-test-hash",
		Envelope:        []byte("env"),
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Hour),
		UserID:          "user-1",
		OperatorID:      "op-1",
	}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Verify it exists
	_, found, _ := sts.GetSuspendedTransaction(ctx, "delete-test-hash")
	assert.True(t, found)

	// Delete the transaction
	err = sts.DeleteSuspendedTransaction(ctx, "delete-test-hash")
	require.NoError(t, err)

	// Verify it's gone
	_, found, _ = sts.GetSuspendedTransaction(ctx, "delete-test-hash")
	assert.False(t, found)
}

func TestDeleteSuspendedTransaction_NonExistent(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	// Delete non-existent transaction (should not error)
	err := sts.DeleteSuspendedTransaction(ctx, "nonexistent-hash")
	require.NoError(t, err)
}

func TestCleanupExpiredSuspendedTransactions_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService
	ctx := context.Background()

	count, err := sts.CleanupExpiredSuspendedTransactions(ctx)
	require.Error(t, err)
	assert.Zero(t, count)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestCleanupExpiredSuspendedTransactions_NilDB(t *testing.T) {
	t.Parallel()

	sts := &SuspendedTransactionService{
		db: nil,
	}
	ctx := context.Background()

	count, err := sts.CleanupExpiredSuspendedTransactions(ctx)
	require.Error(t, err)
	assert.Zero(t, count)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestCleanupExpiredSuspendedTransactions_Success(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create active and expired transactions
	transactions := []*models.SuspendedTransaction{
		{
			TransactionHash: "active-cleanup",
			Envelope:        []byte("active"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "expired-cleanup-1",
			Envelope:        []byte("expired1"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(-time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "expired-cleanup-2",
			Envelope:        []byte("expired2"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(-2 * time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
	}

	for _, tx := range transactions {
		err := sts.StoreSuspendedTransaction(ctx, tx)
		require.NoError(t, err)
	}

	// Cleanup expired transactions
	count, err := sts.CleanupExpiredSuspendedTransactions(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Verify only active transaction remains
	txs, err := sts.ListSuspendedTransactions(ctx, "")
	require.NoError(t, err)
	assert.Len(t, txs, 1)
	assert.Equal(t, "active-cleanup", txs[0].TransactionHash)
}

func TestCleanupExpiredSuspendedTransactions_NoExpired(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	tx := &models.SuspendedTransaction{
		TransactionHash: "active-only",
		Envelope:        []byte("active"),
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Hour),
		UserID:          "user-1",
		OperatorID:      "op-1",
	}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Cleanup should remove nothing
	count, err := sts.CleanupExpiredSuspendedTransactions(ctx)
	require.NoError(t, err)
	assert.Zero(t, count)
}

func TestGetExpiredSuspendedTransactions_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService
	ctx := context.Background()

	txs, err := sts.GetExpiredSuspendedTransactions(ctx)
	require.Error(t, err)
	assert.Nil(t, txs)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestGetExpiredSuspendedTransactions_NilDB(t *testing.T) {
	t.Parallel()

	sts := &SuspendedTransactionService{
		db: nil,
	}
	ctx := context.Background()

	txs, err := sts.GetExpiredSuspendedTransactions(ctx)
	require.Error(t, err)
	assert.Nil(t, txs)
	assert.Contains(t, err.Error(), "store not initialized")
}

func TestGetExpiredSuspendedTransactions_Empty(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	txs, err := sts.GetExpiredSuspendedTransactions(ctx)
	require.NoError(t, err)
	// Accept either nil or empty slice
	if txs != nil {
		assert.Empty(t, txs)
	}
}

func TestGetExpiredSuspendedTransactions_Success(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create active and expired transactions
	transactions := []*models.SuspendedTransaction{
		{
			TransactionHash: "active-expired",
			Envelope:        []byte("active"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "expired-get-1",
			Envelope:        []byte("expired1"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(-time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
		{
			TransactionHash: "expired-get-2",
			Envelope:        []byte("expired2"),
			CreatedAt:       now,
			ExpiresAt:       now.Add(-2 * time.Hour),
			UserID:          "user-1",
			OperatorID:      "op-1",
		},
	}

	for _, tx := range transactions {
		err := sts.StoreSuspendedTransaction(ctx, tx)
		require.NoError(t, err)
	}

	// Get expired transactions
	txs, err := sts.GetExpiredSuspendedTransactions(ctx)
	require.NoError(t, err)
	assert.Len(t, txs, 2)

	// Verify all are expired
	for _, tx := range txs {
		assert.True(t, tx.ExpiresAt.Before(now))
	}
}

func TestClose_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService

	err := sts.Close()
	require.NoError(t, err)
}

func TestClose_Success(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)

	err := sts.Close()
	require.NoError(t, err)
}

func TestWait_NilStore(t *testing.T) {
	t.Parallel()

	var sts *SuspendedTransactionService

	// Wait() on nil store will panic - this is expected behavior
	// The test verifies this behavior is consistent
	assert.Panics(t, func() {
		sts.Wait()
	})
}

func TestWait_Success(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)

	// Should not panic
	sts.Wait()
}

func TestSuspendedTransactionStore_FullWorkflow(t *testing.T) {
	t.Parallel()

	sts := setupTestSuspendedTransactionStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Step 1: Store a suspended transaction
	tx := &models.SuspendedTransaction{
		TransactionHash: "workflow-hash",
		Envelope:        []byte("workflow-envelope"),
		CreatedAt:       now,
		ExpiresAt:       now.Add(time.Hour),
		ToolName:        "workflow_tool",
		ToolArguments:   []byte(`{"arg": "value"}`),
		UserID:          "workflow-user",
		OperatorID:      "workflow-operator",
		Approved:        false,
	}

	err := sts.StoreSuspendedTransaction(ctx, tx)
	require.NoError(t, err)

	// Step 2: Retrieve the transaction
	retrieved, found, err := sts.GetSuspendedTransaction(ctx, "workflow-hash")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "workflow-hash", retrieved.TransactionHash)

	// Step 3: List transactions for the user
	txs, err := sts.ListSuspendedTransactions(ctx, "workflow-user")
	require.NoError(t, err)
	assert.Len(t, txs, 1)

	// Step 4: Approve the transaction
	err = sts.ApproveSuspendedTransaction(ctx, "workflow-hash", models.ApprovalProof{
		ApprovedBy:      "workflow-approver",
		CliSignature:    "workflow-sig",
		CertFingerprint: "workflow-fingerprint",
	})
	require.NoError(t, err)

	// Step 5: Verify approval
	retrieved, found, err = sts.GetSuspendedTransaction(ctx, "workflow-hash")
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, retrieved.Approved)
	assert.Equal(t, "workflow-approver", retrieved.ApprovedBy)

	// Step 6: Delete the transaction
	err = sts.DeleteSuspendedTransaction(ctx, "workflow-hash")
	require.NoError(t, err)

	// Step 7: Verify deletion
	_, found, _ = sts.GetSuspendedTransaction(ctx, "workflow-hash")
	assert.False(t, found)
}
