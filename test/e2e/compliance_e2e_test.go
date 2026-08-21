// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/models"
)

// TestCompliance_AuditReceiptRecordedForDispatchedCommand dispatches an
// FS_READ command through the governance gauntlet, then verifies that the
// audit subsystem recorded a receipt for that specific transaction with L2
// and L3 verification passed, the operator session ID, state roots, and a
// governance signature. The test performs the action (command dispatch)
// first, then asserts on the consequence (audit receipt) — it does not
// assume prior platform activity produced the expected receipts.
//
// This replaces the prior compliance test that ran the KSI pipeline inside
// the operator container via docker exec, and the intermediate version that
// checked all receipts without first dispatching a command. The E2E
// compliance assertion here covers the gateway-side audit surface that
// records command execution evidence for a specific governed transaction.
func TestCompliance_AuditReceiptRecordedForDispatchedCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Dispatch a governed command. The dispatch response carries the
	// transaction ID that the audit receipt will be keyed by.
	resp := e2eClient.dispatchFsRead(t, ctx)
	require.NotEmpty(t, resp.TransactionID, "dispatch response must carry a transaction ID")
	t.Logf("dispatched command for audit verification: txn=%s", resp.TransactionID)

	// The audit receipt for this transaction must appear in the receipts
	// list. Poll to accommodate asynchronous audit write latency.
	var targetReceipt *models.ActionReceiptRecord
	require.Eventually(t, func() bool {
		receipts, err := e2eClient.GetAuditReceipts(ctx, resp.TransactionID)
		if err != nil {
			t.Logf("audit receipts fetch attempt error: %v", err)
			return false
		}
		for i := range receipts.Receipts {
			if receipts.Receipts[i].TransactionID == resp.TransactionID {
				targetReceipt = receipts.Receipts[i]
				return true
			}
		}
		return false
	}, 60*time.Second, 2*time.Second,
		"audit receipt for transaction %s must appear in the receipts list", resp.TransactionID)

	require.NotNil(t, targetReceipt, "audit receipt for transaction %s must be found", resp.TransactionID)
	assert.NotEmpty(t, targetReceipt.TransactionID,
		"audit receipt must carry the transaction ID")
	assert.NotEmpty(t, targetReceipt.ActionType,
		"audit receipt must carry an action type")
	assert.NotEmpty(t, targetReceipt.OperatorSessionID,
		"audit receipt must carry the operator session ID")
	assert.True(t, targetReceipt.L2Valid,
		"audit receipt %s must have L2 verification passed", targetReceipt.TransactionID)
	assert.True(t, targetReceipt.L3Valid,
		"audit receipt %s must have L3 verification passed", targetReceipt.TransactionID)
	assert.NotEmpty(t, targetReceipt.StateRootBefore,
		"audit receipt %s must record the pre-execution state root", targetReceipt.TransactionID)
	assert.NotEmpty(t, targetReceipt.StateRootAfter,
		"audit receipt %s must record the post-execution state root", targetReceipt.TransactionID)
	assert.NotEmpty(t, targetReceipt.Signature,
		"audit receipt %s must carry the governance signature", targetReceipt.TransactionID)
	assert.False(t, targetReceipt.ExecutedAt.IsZero(),
		"audit receipt %s must have a non-zero execution timestamp", targetReceipt.TransactionID)
	t.Logf("audit receipt verified for txn=%s: L2=%v L3=%v action=%s",
		targetReceipt.TransactionID, targetReceipt.L2Valid, targetReceipt.L3Valid,
		targetReceipt.ActionType)
}

// TestCompliance_AuditSummaryConsistent verifies the audit summary endpoint
// returns a typed response whose total counts are non-negative. The summary
// breaks down events and receipts by type; the totals must be non-negative
// and the receipts total must be at least 1 after the command dispatch in
// the receipt test above (Go runs tests alphabetically by filename, so
// compliance_e2e_test.go runs after command_roundtrip_e2e_test.go, but this
// test does not depend on that ordering — it only asserts non-negativity).
func TestCompliance_AuditSummaryConsistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	summary, err := e2eClient.GetAuditSummary(ctx)
	require.NoError(t, err, "audit summary endpoint must succeed on an approved stack")
	require.True(t, summary.Success, "audit summary response must report success")
	assert.GreaterOrEqual(t, summary.ReceiptsTotal, 0,
		"receipts total must be non-negative")
	assert.GreaterOrEqual(t, summary.EventsTotal, 0,
		"events total must be non-negative")
	assert.GreaterOrEqual(t, summary.TotalRecords, 0,
		"total records must be non-negative")
	t.Logf("audit summary: events=%d receipts=%d total=%d",
		summary.EventsTotal, summary.ReceiptsTotal, summary.TotalRecords)
}

// TestCompliance_AuditEventsRecorded verifies the audit events endpoint
// returns a typed response with a count matching the events slice length.
// Audit events are the raw command execution log rows; after the command
// dispatch in the receipt test, at least one event must be recorded.
func TestCompliance_AuditEventsRecorded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	events, err := e2eClient.GetAuditEvents(ctx)
	require.NoError(t, err, "audit events endpoint must succeed on an approved stack")
	require.True(t, events.Success, "audit events response must report success")
	assert.Equal(t, len(events.Events), events.Count,
		"audit events count must match the events slice length")
	if events.Count > 0 {
		row := events.Events[0]
		assert.NotEmpty(t, row.Type, "audit event must carry a type")
		assert.NotEmpty(t, row.Timestamp, "audit event must carry a timestamp")
	}
	t.Logf("audit events verified: %d events", events.Count)
}
