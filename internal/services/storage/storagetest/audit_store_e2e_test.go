// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package storagetest

import (
	"crypto/ed25519"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// End-to-End Audit Trail Flow Tests
// ============================================================================

func TestSQLAuditStore_EndToEnd_AuditTrail(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	// Simulate a complete audit trail flow
	operatorSessionID := "e2e-session-123"
	err = avs.CreateSession(operatorSessionID, "operator", "E2E Test Session", "user@example.com")
	require.NoError(t, err)

	// 1. Record user message
	exitCode := 0
	userMsgEvent := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.UserMsg,
		ContentText:       "Deploy the application",
	}
	_, err = avs.RecordEvent(userMsgEvent)
	require.NoError(t, err)

	// 2. Record AI response
	aiMsgEvent := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.AIMsg,
		ContentText:       "I'll deploy the application using the deployment script",
	}
	_, err = avs.RecordEvent(aiMsgEvent)
	require.NoError(t, err)

	// 3. Record command execution
	cmdEvent := &storage.Event{
		OperatorSessionID:   operatorSessionID,
		Timestamp:           time.Now().UTC(),
		Type:                constants.Event.Operator.Audit.Command,
		ContentText:         "Execute deployment",
		CommandRaw:          "./deploy.sh",
		CommandExitCode:     exitCode,
		CommandStdout:       "Deployment successful\nApplication deployed to production",
		CommandStderr:       "",
		ExecutionDurationMs: 5000,
	}
	cmdEventID, err := avs.RecordEvent(cmdEvent)
	require.NoError(t, err)
	assert.Positive(t, cmdEventID)

	// 4. Record file mutations
	fileMutation := &storage.FileMutationLog{
		EventID:          cmdEventID,
		Filepath:         "/app/config.yaml",
		Operation:        storage.FileMutationWrite,
		LedgerHashBefore: "hash-before-deploy",
		LedgerHashAfter:  "hash-after-deploy",
		DiffStat:         "+10 lines, -2 lines",
	}
	err = avs.RecordFileMutation(fileMutation)
	require.NoError(t, err)

	// 5. Record action receipt (governed transaction)
	receipt := &models.ActionReceiptRecord{
		TransactionID:     "tx-e2e-deploy-123",
		TransactionHash:   "hash-e2e-deploy",
		OperatorID:        "operator-1",
		OperatorSessionID: operatorSessionID,
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "production-server",
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:     "deployment completed successfully",
		StateRootBefore:   "state-root-before",
		StateRootAfter:    "state-root-after",
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "key-1",
		Signature:         "signature-e2e",
		Timestamp:         time.Now().UTC(),
	}
	err = avs.RecordActionReceipt(receipt)
	require.NoError(t, err)

	// Verify the complete audit trail
	// 1. Retrieve all events
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 3) // user msg, AI msg, command

	// 2. Verify event types
	eventTypes := make(map[constants.EventType]bool)
	for _, e := range events {
		eventTypes[e.Type] = true
	}
	assert.True(t, eventTypes[constants.Event.Operator.Audit.UserMsg])
	assert.True(t, eventTypes[constants.Event.Operator.Audit.AIMsg])
	assert.True(t, eventTypes[constants.Event.Operator.Audit.Command])

	// 3. Retrieve file mutations
	mutations, err := avs.GetFileMutations(cmdEventID)
	require.NoError(t, err)
	assert.Len(t, mutations, 1)
	assert.Equal(t, "/app/config.yaml", mutations[0].Filepath)
	assert.Equal(t, storage.FileMutationWrite, mutations[0].Operation)

	// 4. Retrieve action receipt
	persistedReceipt, err := avs.GetActionReceipt("tx-e2e-deploy-123")
	require.NoError(t, err)
	require.NotNil(t, persistedReceipt)
	assert.Equal(t, "tx-e2e-deploy-123", persistedReceipt.TransactionID)
	assert.Equal(t, operatorSessionID, persistedReceipt.OperatorSessionID)

	// 5. List all receipts for the session
	receipts, err := avs.ListActionReceipts(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, receipts, 1)
}

func TestSQLAuditStore_EndToEnd_MultipleTransactions(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "multi-tx-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Multi-TX Session", "user@example.com")
	require.NoError(t, err)

	// Simulate multiple governed transactions
	transactions := []struct {
		txID       string
		actionType constants.ActionType
		resource   string
		status     operatorv1.ExecutionStatus
	}{
		{"tx-1", constants.ActionTypeExecuteBash, "server-1", operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED},
		{"tx-2", constants.ActionTypeFileEdit, "/etc/hosts", operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED},
		{"tx-3", constants.ActionTypeExecuteBash, "server-2", operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED},
	}

	for _, tx := range transactions {
		// Record event
		exitCode := 0
		if tx.status == operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED {
			exitCode = 1
		}
		event := &storage.Event{
			OperatorSessionID: operatorSessionID,
			Timestamp:         time.Now().UTC(),
			Type:              constants.Event.Operator.Audit.Command,
			ContentText:       string(tx.actionType),
			CommandRaw:        fmt.Sprintf("action on %s", tx.resource),
			CommandExitCode:   exitCode,
		}
		eventID, err := avs.RecordEvent(event)
		require.NoError(t, err)

		// Record receipt
		receipt := &models.ActionReceiptRecord{
			TransactionID:     tx.txID,
			TransactionHash:   fmt.Sprintf("hash-%s", tx.txID),
			OperatorID:        "operator-1",
			OperatorSessionID: operatorSessionID,
			ActionType:        tx.actionType,
			TargetResource:    tx.resource,
			Status:            tx.status,
			ResultSummary:     tx.status.String(),
			StateRootBefore:   "root-before",
			StateRootAfter:    "root-after",
			ExecutedAt:        time.Now().UTC(),
			SignerKeyID:       "key-1",
			Signature:         fmt.Sprintf("sig-%s", tx.txID),
			Timestamp:         time.Now().UTC(),
		}
		err = avs.RecordActionReceipt(receipt)
		require.NoError(t, err)

		// Record file mutation for file edit
		if tx.actionType == constants.ActionTypeFileEdit {
			mutation := &storage.FileMutationLog{
				EventID:          eventID,
				Filepath:         tx.resource,
				Operation:        storage.FileMutationWrite,
				LedgerHashBefore: "before",
				LedgerHashAfter:  "after",
				DiffStat:         "+1 line",
			}
			err = avs.RecordFileMutation(mutation)
			require.NoError(t, err)
		}
	}

	// Verify all transactions are recorded
	receipts, err := avs.ListActionReceipts(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, receipts, 3)

	// Verify events
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 3)

	// Verify action types
	actionTypes := make(map[constants.ActionType]bool)
	for _, r := range receipts {
		actionTypes[r.ActionType] = true
	}
	assert.True(t, actionTypes[constants.ActionTypeExecuteBash])
	assert.True(t, actionTypes[constants.ActionTypeFileEdit])
}

func TestSQLAuditStore_Consistency_EventReceiptLinkage(t *testing.T) {
	tempDir := testutil.TempDir(t)
	vaultDir := filepath.Join(tempDir, "vault")

	// Create test vault
	_, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	testVault := CreateTestVault(t, vaultDir, privKey)
	fileSvc := NewTestFileSvc(t, tempDir)
	config := &TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           testVault,
	}

	avs, err := NewTestSQLAuditStore(config, testutil.NewTestLogger(), fileSvc)
	require.NoError(t, err)
	defer avs.Close()

	operatorSessionID := "consistency-session"
	err = avs.CreateSession(operatorSessionID, "operator", "Consistency Test", "user@example.com")
	require.NoError(t, err)

	// Record event and receipt for the same logical action
	exitCode := 0
	event := &storage.Event{
		OperatorSessionID: operatorSessionID,
		Timestamp:         time.Now().UTC(),
		Type:              constants.Event.Operator.Audit.Command,
		ContentText:       "Consistency check",
		CommandRaw:        "ls -la",
		CommandExitCode:   exitCode,
		CommandStdout:     "file1\nfile2",
	}
	_, err = avs.RecordEvent(event)
	require.NoError(t, err)

	receipt := &models.ActionReceiptRecord{
		TransactionID:     "tx-consistency-123",
		TransactionHash:   "hash-consistency",
		OperatorID:        "operator-1",
		OperatorSessionID: operatorSessionID,
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "localhost",
		Status:            operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		ResultSummary:     "completed",
		StateRootBefore:   "root-before",
		StateRootAfter:    "root-after",
		ExecutedAt:        time.Now().UTC(),
		SignerKeyID:       "key-1",
		Signature:         "signature-consistency",
		Timestamp:         time.Now().UTC(),
	}
	err = avs.RecordActionReceipt(receipt)
	require.NoError(t, err)

	// Verify both are linked to the same session
	events, err := avs.GetEvents(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, operatorSessionID, events[0].OperatorSessionID)

	receipts, err := avs.ListActionReceipts(operatorSessionID, 10, 0)
	require.NoError(t, err)
	assert.Len(t, receipts, 1)
	assert.Equal(t, operatorSessionID, receipts[0].OperatorSessionID)

	// Verify session exists and has correct metadata
	session, err := avs.GetOperatorSession(operatorSessionID)
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, operatorSessionID, session.ID)
	assert.Equal(t, "Consistency Test", session.Title)
}
