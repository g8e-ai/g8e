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

package storagetest

import (
	"crypto/ed25519"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ActionReceipt Tests
// ============================================================================

func TestSQLAuditStore_RecordActionReceipt(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Record an action receipt
	record := &models.ActionReceiptRecord{
		TransactionID:     "tx-123",
		TransactionHash:   "hash-abc123",
		OperatorID:        "operator-1",
		OperatorSessionID: "session-1",
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "localhost",
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:     "completed successfully",
		StateRootBefore:   "root-before-123",
		StateRootAfter:    "root-after-456",
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "key-1",
		Signature:         "signature-xyz",
		Timestamp:         time.Now().UTC(),
	}

	err = avs.RecordActionReceipt(record)
	require.NoError(t, err)

	// Verify it was persisted
	persisted, err := avs.GetActionReceipt("tx-123")
	require.NoError(t, err)
	require.NotNil(t, persisted)

	assert.Equal(t, "tx-123", persisted.TransactionID)
	assert.Equal(t, "hash-abc123", persisted.TransactionHash)
	assert.Equal(t, "operator-1", persisted.OperatorID)
	assert.Equal(t, "session-1", persisted.OperatorSessionID)
	assert.Equal(t, constants.ActionTypeExecuteBash, persisted.ActionType)
	assert.Equal(t, "localhost", persisted.TargetResource)
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, persisted.Status)
	assert.Equal(t, "completed successfully", persisted.ResultSummary)
	assert.Equal(t, "root-before-123", persisted.StateRootBefore)
	assert.Equal(t, "root-after-456", persisted.StateRootAfter)
	assert.Equal(t, "key-1", persisted.SignerKeyID)
	assert.Equal(t, "signature-xyz", persisted.Signature)
}

func TestSQLAuditStore_RecordActionReceipt_Upsert(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Record initial receipt
	record1 := &models.ActionReceiptRecord{
		TransactionID:     "tx-upsert-123",
		TransactionHash:   "hash-initial",
		OperatorID:        "operator-1",
		OperatorSessionID: "session-1",
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "localhost",
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		ResultSummary:     "executing",
		StateRootBefore:   "root-before",
		StateRootAfter:    "root-after",
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "key-1",
		Signature:         "signature-initial",
		Timestamp:         time.Now().UTC(),
	}

	err = avs.RecordActionReceipt(record1)
	require.NoError(t, err)

	record2 := &models.ActionReceiptRecord{
		TransactionID:     "tx-upsert-123", // Same ID
		TransactionHash:   "hash-updated",
		OperatorID:        "operator-1",
		OperatorSessionID: "session-1",
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "localhost",
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:     "completed",
		StateRootBefore:   "root-before",
		StateRootAfter:    "root-after-updated",
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "key-1",
		Signature:         "signature-updated",
		Timestamp:         time.Now().UTC(),
	}

	err = avs.RecordActionReceipt(record2)
	require.NoError(t, err)

	// Verify the update was applied
	persisted, err := avs.GetActionReceipt("tx-upsert-123")
	require.NoError(t, err)
	require.NotNil(t, persisted)

	assert.Equal(t, "tx-upsert-123", persisted.TransactionID)
	assert.Equal(t, "hash-initial", persisted.TransactionHash) // NOT updated by UPSERT
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, persisted.Status) // Updated
	assert.Equal(t, "completed", persisted.ResultSummary) // Updated
	assert.Equal(t, "root-after-updated", persisted.StateRootAfter) // Updated
	assert.Equal(t, "signature-updated", persisted.Signature) // Updated
}

func TestSQLAuditStore_GetActionReceipt_NotFound(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Query non-existent receipt
	receipt, err := avs.GetActionReceipt("non-existent-tx")
	require.NoError(t, err)
	assert.Nil(t, receipt)
}

func TestSQLAuditStore_ListActionReceipts(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create receipts for two different sessions
	for i := 0; i < 5; i++ {
		record := &models.ActionReceiptRecord{
			TransactionID:     fmt.Sprintf("tx-session1-%d", i),
			TransactionHash:   fmt.Sprintf("hash-%d", i),
			OperatorID:        "operator-1",
			OperatorSessionID: "session-1",
			ActionType:        constants.ActionTypeExecuteBash,
			TargetResource:    "localhost",
			Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:     fmt.Sprintf("result %d", i),
			StateRootBefore:   "root-before",
			StateRootAfter:    "root-after",
			ExecutedAt:        time.Now().UTC(),
			SignerKeyID:       "key-1",
			Signature:         fmt.Sprintf("sig-%d", i),
			Timestamp:         time.Now().UTC(),
		}
		err = avs.RecordActionReceipt(record)
		require.NoError(t, err)
	}

	for i := 0; i < 3; i++ {
		record := &models.ActionReceiptRecord{
			TransactionID:     fmt.Sprintf("tx-session2-%d", i),
			TransactionHash:   fmt.Sprintf("hash-2-%d", i),
			OperatorID:        "operator-1",
			OperatorSessionID: "session-2",
			ActionType:        constants.ActionTypeExecuteBash,
			TargetResource:    "localhost",
			Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:     fmt.Sprintf("result 2-%d", i),
			StateRootBefore:   "root-before",
			StateRootAfter:    "root-after",
			ExecutedAt:        time.Now().UTC(),
			SignerKeyID:       "key-1",
			Signature:         fmt.Sprintf("sig-2-%d", i),
			Timestamp:         time.Now().UTC(),
		}
		err = avs.RecordActionReceipt(record)
		require.NoError(t, err)
	}

	// List all receipts for session-1
	receipts, err := avs.ListActionReceipts("session-1", 10, 0)
	require.NoError(t, err)
	assert.Len(t, receipts, 5)

	// Verify all belong to session-1
	for _, r := range receipts {
		assert.Equal(t, "session-1", r.OperatorSessionID)
	}

	// List with pagination (limit 2)
	receipts, err = avs.ListActionReceipts("session-1", 2, 0)
	require.NoError(t, err)
	assert.Len(t, receipts, 2)

	// List with offset
	receipts, err = avs.ListActionReceipts("session-1", 2, 2)
	require.NoError(t, err)
	assert.Len(t, receipts, 2)

	// List for session-2
	receipts, err = avs.ListActionReceipts("session-2", 10, 0)
	require.NoError(t, err)
	assert.Len(t, receipts, 3)
}

func TestSQLAuditStore_ListActionReceipts_AllSessions(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Create receipts for multiple sessions
	for i := 0; i < 3; i++ {
		record := &models.ActionReceiptRecord{
			TransactionID:     fmt.Sprintf("tx-%d", i),
			TransactionHash:   fmt.Sprintf("hash-%d", i),
			OperatorID:        "operator-1",
			OperatorSessionID: fmt.Sprintf("session-%d", i),
			ActionType:        constants.ActionTypeExecuteBash,
			TargetResource:    "localhost",
			Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:     fmt.Sprintf("result %d", i),
			StateRootBefore:   "root-before",
			StateRootAfter:    "root-after",
			ExecutedAt:        time.Now().UTC(),
			SignerKeyID:       "key-1",
			Signature:         fmt.Sprintf("sig-%d", i),
			Timestamp:         time.Now().UTC(),
		}
		err = avs.RecordActionReceipt(record)
		require.NoError(t, err)
	}

	// List all receipts (empty session ID)
	receipts, err := avs.ListActionReceipts("", 10, 0)
	require.NoError(t, err)
	assert.Len(t, receipts, 3)
}

func TestSQLAuditStore_ListActionReceiptsSince(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	baseTime := time.Now().UTC()

	// Create receipts at different times
	for i := 0; i < 5; i++ {
		record := &models.ActionReceiptRecord{
			TransactionID:     fmt.Sprintf("tx-time-%d", i),
			TransactionHash:   fmt.Sprintf("hash-time-%d", i),
			OperatorID:        "operator-1",
			OperatorSessionID: "session-1",
			ActionType:        constants.ActionTypeExecuteBash,
			TargetResource:    "localhost",
			Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			ResultSummary:     fmt.Sprintf("result %d", i),
			StateRootBefore:   "root-before",
			StateRootAfter:    "root-after",
			ExecutedAt:        baseTime.Add(time.Duration(i) * time.Minute),
			SignerKeyID:       "key-1",
			Signature:         fmt.Sprintf("sig-time-%d", i),
			Timestamp:         baseTime.Add(time.Duration(i) * time.Minute),
		}
		err = avs.RecordActionReceipt(record)
		require.NoError(t, err)
	}

	// List receipts since 2 minutes after base time
	since := baseTime.Add(2 * time.Minute)
	receipts, err := avs.ListActionReceiptsSince(since, 10)
	require.NoError(t, err)
	assert.Len(t, receipts, 2) // tx-time-3 and tx-time-4

	// Verify ordering (ascending by timestamp)
	assert.Equal(t, "tx-time-3", receipts[0].TransactionID)
	assert.Equal(t, "tx-time-4", receipts[1].TransactionID)
}

func TestSQLAuditStore_ListActionReceiptsSince_Empty(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// No receipts
	receipts, err := avs.ListActionReceiptsSince(time.Now().UTC(), 10)
	require.NoError(t, err)
	assert.Len(t, receipts, 0)
}

func TestSQLAuditStore_ActionReceipts_AutoSessionCreation(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	// Record a receipt without explicitly creating the session
	// (should auto-create the session row for FK satisfaction)
	record := &models.ActionReceiptRecord{
		TransactionID:     "tx-auto-session",
		TransactionHash:   "hash-auto",
		OperatorID:        "operator-1",
		OperatorSessionID: "auto-created-session",
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "localhost",
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:     "completed",
		StateRootBefore:   "root-before",
		StateRootAfter:    "root-after",
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "key-1",
		Signature:         "signature-auto",
		Timestamp:         time.Now().UTC(),
	}

	err = avs.RecordActionReceipt(record)
	require.NoError(t, err)

	// Verify receipt was recorded
	persisted, err := avs.GetActionReceipt("tx-auto-session")
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, "auto-created-session", persisted.OperatorSessionID)

	// Verify session was auto-created
	session, err := avs.GetOperatorSession("auto-created-session")
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "auto-created-session", session.ID)
}

func TestSQLAuditStore_ActionReceipts_MultipleStatuses(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := createTestVault(t, vaultDir, privKey)

	config := &TestSQLAuditStoreConfig{
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		Enabled:                   true,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger())
	require.NoError(t, err)
	defer avs.Close()

	statuses := []operatorv1.ExecutionStatus{
		operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
		operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT,
	}

	for i, status := range statuses {
		record := &models.ActionReceiptRecord{
			TransactionID:     fmt.Sprintf("tx-status-%d", i),
			TransactionHash:   fmt.Sprintf("hash-status-%d", i),
			OperatorID:        "operator-1",
			OperatorSessionID: "session-status",
			ActionType:        constants.ActionTypeExecuteBash,
			TargetResource:    "localhost",
			Status:            status,
			ResultSummary:     fmt.Sprintf("status %s", status.String()),
			StateRootBefore:   "root-before",
			StateRootAfter:    "root-after",
			ExecutedAt:        time.Now().UTC(),
			SignerKeyID:       "key-1",
			Signature:         fmt.Sprintf("sig-status-%d", i),
			Timestamp:         time.Now().UTC(),
		}
		err = avs.RecordActionReceipt(record)
		require.NoError(t, err)
	}

	// List all receipts
	receipts, err := avs.ListActionReceipts("session-status", 10, 0)
	require.NoError(t, err)
	assert.Len(t, receipts, 4)

	// Verify all statuses are present
	statusSet := make(map[operatorv1.ExecutionStatus]bool)
	for _, r := range receipts {
		statusSet[r.Status] = true
	}
	for _, status := range statuses {
		assert.True(t, statusSet[status], "Missing status: %s", status.String())
	}
}
