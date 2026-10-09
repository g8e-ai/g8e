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
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestSQLAuditStore_ReceiptProjectionNeverRegressesTerminalStatus(t *testing.T) {
	store := newReceiptProjectionTestStore(t)

	completed := receiptProjectionRecord("tx-terminal", operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED)
	completed.ResultSummary = "completed successfully"
	require.NoError(t, store.RecordActionReceipt(completed))

	replayed := receiptProjectionRecord("tx-terminal", operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING)
	replayed.ResultSummary = "still executing"
	require.NoError(t, store.RecordActionReceipt(replayed))

	got, err := store.GetActionReceipt(completed.TransactionID)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, got.Status)
	require.Equal(t, "completed successfully", got.ResultSummary)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, got.ActionReceipt.GetStatus())

	var facts int
	require.NoError(t, store.db.QueryRow(
		"SELECT count(*) FROM events WHERE type = ?",
		string(constants.EventOperatorReceiptRecorded),
	).Scan(&facts))
	require.Equal(t, 2, facts, "the replayed stage remains audit evidence")
}

func TestSQLAuditStore_ReceiptReadsRestoreValidityAndTolerateLegacyNulls(t *testing.T) {
	store := newReceiptProjectionTestStore(t)

	record := receiptProjectionRecord("tx-legacy", operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED)
	record.ActionReceipt.L2Status = operatorv1.L2Status_L2_STATUS_REQUIRED_VALID
	record.ActionReceipt.L3Status = operatorv1.L3Status_L3_STATUS_REQUIRED_VALID
	require.NoError(t, store.RecordActionReceipt(record))

	_, err := store.db.Exec(`UPDATE receipts SET requestor_user_id = NULL, acting_app_id = NULL, event_type = NULL,
		target_resource = NULL, result_summary = NULL, state_root_before = NULL, state_root_after = NULL`)
	require.NoError(t, err)

	got, err := store.GetActionReceipt(record.TransactionID)
	require.NoError(t, err)
	require.True(t, got.L2Valid)
	require.True(t, got.L3Valid)
	require.Empty(t, got.RequestorUserID)
	require.Empty(t, got.TargetResource)

	listed, err := store.ListActionReceipts(record.OperatorSessionID, 10, 0)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.True(t, listed[0].L2Valid)
	require.True(t, listed[0].L3Valid)
	require.Empty(t, listed[0].StateRootBefore)

	since, err := store.ListActionReceiptsSince(record.Timestamp.Add(-time.Second), 10)
	require.NoError(t, err)
	require.Len(t, since, 1)
	require.True(t, since[0].L2Valid)
	require.True(t, since[0].L3Valid)
	require.Empty(t, since[0].ResultSummary)
}

func newReceiptProjectionTestStore(t *testing.T) *TestSQLAuditStore {
	t.Helper()

	tempDir := testutil.TempDir(t)
	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	store, err := NewTestSQLAuditStore(&TestSQLAuditStoreConfig{
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             7,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
		EncryptionVault:           CreateTestVault(t, filepath.Join(tempDir, "vault"), privateKey),
	}, testutil.NewTestLogger(), NewTestFileSvc(t, tempDir))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return store
}

func receiptProjectionRecord(transactionID string, status operatorv1.ExecutionStatus) *models.ActionReceiptRecord {
	now := time.Now().UTC()
	return &models.ActionReceiptRecord{
		TransactionID:     transactionID,
		TransactionHash:   "hash-" + transactionID,
		OperatorID:        "operator-1",
		OperatorSessionID: "session-1",
		RequestorUserID:   "user-1",
		ActingAppID:       "app-1",
		EventType:         constants.EventOperatorReceiptRecorded,
		ActionType:        constants.ActionTypeExecuteBash,
		TargetResource:    "localhost",
		Status:            status,
		ResultSummary:     "result",
		StateRootBefore:   "before",
		StateRootAfter:    "after",
		ExecutedAt:        now,
		SignerKeyID:       "key-1",
		Signature:         "signature",
		Timestamp:         now,
		ActionReceipt: &operatorv1.ActionReceipt{
			TransactionId:   transactionID,
			TransactionHash: "hash-" + transactionID,
			Status:          status,
		},
	}
}
