// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestReceiptProjectionNeverRegressesTerminalStatus(t *testing.T) {
	store := newIntegrationAuditStore(t)
	require.NoError(t, store.CreateSession("projection-session", constants.SessionTypeOperator, "Projection", "user-1"))

	completed := sampleActionReceiptRecord(t, "projection-session")
	require.NoError(t, store.RecordActionReceipt(completed))

	replayed := sampleActionReceiptRecord(t, "projection-session")
	replayed.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING
	replayed.ActionReceipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING
	replayed.ResultSummary = ""
	require.NoError(t, store.RecordActionReceipt(replayed))

	got, err := store.GetActionReceipt(completed.TransactionID)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, got.Status)
	require.Equal(t, "completed successfully", got.ResultSummary)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, got.ActionReceipt.GetStatus())

	// The replayed stage is still evidence: both stage writes append an audit fact.
	var facts int
	require.NoError(t, store.db.QueryRow("SELECT count(*) FROM events WHERE type = ?", string(constants.EventOperatorReceiptRecorded)).Scan(&facts))
	require.Equal(t, 2, facts)
}

func TestReceiptProjectionAdvancesFromExecutingToTerminal(t *testing.T) {
	store := newIntegrationAuditStore(t)
	require.NoError(t, store.CreateSession("projection-session", constants.SessionTypeOperator, "Projection", "user-1"))

	executing := sampleActionReceiptRecord(t, "projection-session")
	executing.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING
	executing.ActionReceipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING
	require.NoError(t, store.RecordActionReceipt(executing))

	failed := sampleActionReceiptRecord(t, "projection-session")
	failed.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	failed.ActionReceipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	require.NoError(t, store.RecordActionReceipt(failed))

	got, err := store.GetActionReceipt(failed.TransactionID)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, got.Status)
}

func TestReceiptReadsRestoreL2L3Validity(t *testing.T) {
	store := newIntegrationAuditStore(t)
	require.NoError(t, store.CreateSession("projection-session", constants.SessionTypeOperator, "Projection", "user-1"))

	record := sampleActionReceiptRecord(t, "projection-session")
	record.ActionReceipt.L2Status = operatorv1.L2Status_L2_STATUS_REQUIRED_VALID
	record.ActionReceipt.L3Status = operatorv1.L3Status_L3_STATUS_NOT_REQUIRED
	require.NoError(t, store.RecordActionReceipt(record))

	got, err := store.GetActionReceipt(record.TransactionID)
	require.NoError(t, err)
	require.True(t, got.L2Valid)
	require.False(t, got.L3Valid)

	listed, err := store.ListActionReceipts(models.AuditScope{}, 10, 0)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.True(t, listed[0].L2Valid)
}

func TestReceiptReadsTolerateLegacyNullColumns(t *testing.T) {
	store := newIntegrationAuditStore(t)
	require.NoError(t, store.CreateSession("projection-session", constants.SessionTypeOperator, "Projection", "user-1"))

	record := sampleActionReceiptRecord(t, "projection-session")
	require.NoError(t, store.RecordActionReceipt(record))
	// Rows written before migrateReceiptsColumns added these columns hold NULL.
	_, err := store.db.Exec(`UPDATE receipts SET requestor_user_id = NULL, acting_app_id = NULL, event_type = NULL,
		target_resource = NULL, result_summary = NULL, state_root_before = NULL, state_root_after = NULL`)
	require.NoError(t, err)

	got, err := store.GetActionReceipt(record.TransactionID)
	require.NoError(t, err)
	require.Empty(t, got.RequestorUserID)
	require.Empty(t, got.TargetResource)

	listed, err := store.ListActionReceipts(models.AuditScope{}, 10, 0)
	require.NoError(t, err)
	require.Len(t, listed, 1)
}

func TestReceiptProjectionIgnoresReplayedUnspecifiedAfterTerminal(t *testing.T) {
	store := newIntegrationAuditStore(t)
	require.NoError(t, store.CreateSession("projection-session", constants.SessionTypeOperator, "Projection", "user-1"))

	failed := sampleActionReceiptRecord(t, "projection-session")
	failed.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	failed.ActionReceipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	require.NoError(t, store.RecordActionReceipt(failed))

	stale := sampleActionReceiptRecord(t, "projection-session")
	stale.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED
	stale.ActionReceipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_UNSPECIFIED
	require.NoError(t, store.RecordActionReceipt(stale))

	got, err := store.GetActionReceipt(failed.TransactionID)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, got.Status)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED, got.ActionReceipt.GetStatus())
}

func TestReceiptProjectionAllowsTerminalReplacement(t *testing.T) {
	store := newIntegrationAuditStore(t)
	require.NoError(t, store.CreateSession("projection-session", constants.SessionTypeOperator, "Projection", "user-1"))

	first := sampleActionReceiptRecord(t, "projection-session")
	require.NoError(t, store.RecordActionReceipt(first))

	second := sampleActionReceiptRecord(t, "projection-session")
	second.ResultSummary = "re-signed final stage"
	require.NoError(t, store.RecordActionReceipt(second))

	got, err := store.GetActionReceipt(first.TransactionID)
	require.NoError(t, err)
	require.Equal(t, "re-signed final stage", got.ResultSummary)
}

func TestReceiptProjectionKeepsTransactionsIndependent(t *testing.T) {
	store := newIntegrationAuditStore(t)
	require.NoError(t, store.CreateSession("projection-session", constants.SessionTypeOperator, "Projection", "user-1"))

	done := sampleActionReceiptRecord(t, "projection-session")
	require.NoError(t, store.RecordActionReceipt(done))

	other := sampleActionReceiptRecord(t, "projection-session")
	other.TransactionID = "tx-integration-2"
	other.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING
	other.ActionReceipt.TransactionId = other.TransactionID
	other.ActionReceipt.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING
	require.NoError(t, store.RecordActionReceipt(other))

	got, err := store.GetActionReceipt(other.TransactionID)
	require.NoError(t, err)
	require.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING, got.Status)
}

func TestReceiptExportReadsRestoreL2L3ValidityAndTolerateNulls(t *testing.T) {
	store := newIntegrationAuditStore(t)
	require.NoError(t, store.CreateSession("projection-session", constants.SessionTypeOperator, "Projection", "user-1"))

	record := sampleActionReceiptRecord(t, "projection-session")
	record.ActionReceipt.L2Status = operatorv1.L2Status_L2_STATUS_REQUIRED_VALID
	record.ActionReceipt.L3Status = operatorv1.L3Status_L3_STATUS_REQUIRED_VALID
	require.NoError(t, store.RecordActionReceipt(record))
	_, err := store.db.Exec(`UPDATE receipts SET requestor_user_id = NULL, acting_app_id = NULL, event_type = NULL,
		target_resource = NULL, result_summary = NULL, state_root_before = NULL, state_root_after = NULL`)
	require.NoError(t, err)

	got, err := store.ListActionReceiptsSince(time.Now().Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, got[0].L2Valid)
	require.True(t, got[0].L3Valid)
	require.Empty(t, got[0].StateRootBefore)
}
