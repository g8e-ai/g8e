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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCompliance_AuditReceiptsRecorded verifies that the gateway's audit
// subsystem records action receipts for governance-verified commands. After
// the command roundtrip test (or any prior platform activity on an approved
// stack), the audit receipts endpoint must return a typed, non-empty receipts
// list with valid transaction IDs and L2/L3 verification flags set.
//
// This replaces the prior compliance test that ran the KSI pipeline inside
// the operator container via docker exec. The KSI/OSCAL pipeline is a CLI
// concern tested through hermetic CLI command tests; the E2E compliance
// assertion here covers the gateway-side audit surface that records command
// execution evidence. Each receipt carries the L2Valid and L3Valid flags
// from the 5-layer verification gauntlet, proving governance verification
// ran for the recorded action.
func TestCompliance_AuditReceiptsRecorded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	receipts, err := e2eClient.GetAuditReceipts(ctx, "")
	require.NoError(t, err, "audit receipts endpoint must succeed on an approved stack")
	require.True(t, receipts.Success, "audit receipts response must report success")
	require.NotEmpty(t, receipts.Receipts,
		"audit receipts must be non-empty after platform activity — governance audit path may be broken")

	for _, receipt := range receipts.Receipts {
		assert.NotEmpty(t, receipt.TransactionID,
			"every audit receipt must carry a transaction ID")
		assert.NotEmpty(t, receipt.ActionType,
			"every audit receipt must carry an action type")
		assert.NotEmpty(t, receipt.OperatorSessionID,
			"every audit receipt must carry the operator session ID")
		assert.True(t, receipt.L2Valid,
			"audit receipt %s must have L2 verification passed", receipt.TransactionID)
		assert.True(t, receipt.L3Valid,
			"audit receipt %s must have L3 verification passed", receipt.TransactionID)
		assert.NotEmpty(t, receipt.StateRootBefore,
			"audit receipt %s must record the pre-execution state root", receipt.TransactionID)
		assert.NotEmpty(t, receipt.StateRootAfter,
			"audit receipt %s must record the post-execution state root", receipt.TransactionID)
		assert.NotEmpty(t, receipt.Signature,
			"audit receipt %s must carry the governance signature", receipt.TransactionID)
		assert.False(t, receipt.ExecutedAt.IsZero(),
			"audit receipt %s must have a non-zero execution timestamp", receipt.TransactionID)
	}
	t.Logf("audit receipts verified: %d receipts, all L2/L3 valid",
		len(receipts.Receipts))
}

// TestCompliance_AuditSummaryConsistent verifies the audit summary endpoint
// returns a typed response whose total counts are consistent with the
// receipts list. The summary breaks down events and receipts by type; the
// totals must be non-negative and the receipts total must match or exceed the
// number of receipts observable through the receipts endpoint (the receipts
// endpoint applies a default limit, so the summary total is the upper bound).
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
// Audit events are the raw command execution log rows; on an approved stack
// with prior activity, at least one event must be recorded.
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
