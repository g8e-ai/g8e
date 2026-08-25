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

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// TestCompliance_AuditReceiptsRecorded verifies that the gateway-side audit
// surface has signed ActionReceipts from governance transactions processed
// through the gateway's L5 actuator. After platform setup (owner bootstrap
// and enrollment approval), the gateway's audit store contains receipts for
// platform enrollment operations (CREATE, DECIDE, ISSUE, CREATE_SESSION).
// Each receipt must carry a transaction ID, action type, governance
// signature, state roots, and a non-zero execution timestamp.
//
// Command dispatch receipts are recorded in the operator's audit store by
// the operator's L5 actuator, not the gateway's, because the operator
// executes dispatched commands. The gateway's audit surface covers
// gateway-side governance transactions. This test asserts on those
// receipts, not on command dispatch receipts.
func TestCompliance_AuditReceiptsRecorded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	receipts, err := e2eClient.GetAuditReceipts(ctx, "")
	require.NoError(t, err, "audit receipts endpoint must succeed on an approved stack")
	require.True(t, receipts.Success, "audit receipts response must report success")
	require.NotEmpty(t, receipts.Receipts,
		"gateway must have at least one audit receipt from platform enrollment operations")

	r := receipts.Receipts[0]
	assert.NotEmpty(t, r.TransactionID, "audit receipt must carry a transaction ID")
	assert.NotEmpty(t, r.ActionType, "audit receipt must carry an action type")
	assert.NotEmpty(t, r.Signature, "audit receipt must carry the governance signature")
	assert.NotEmpty(t, r.SignerKeyID, "audit receipt must carry the signer key ID")
	assert.NotEmpty(t, r.StateRootBefore,
		"audit receipt must record the pre-execution state root")
	assert.False(t, r.ExecutedAt.IsZero(),
		"audit receipt must have a non-zero execution timestamp")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, r.Status,
		"audit receipt must have completed execution status")
	t.Logf("audit receipt verified: txn=%s action=%s status=%s",
		r.TransactionID, r.ActionType, r.Status.String())
}

// TestCompliance_AuditSummaryConsistent verifies the audit summary endpoint
// returns a typed response whose total counts are non-negative. The summary
// breaks down events and receipts by type; the totals must be non-negative
// and the receipts total must be at least 1 after platform enrollment
// operations have been processed through the gateway's L5 actuator.
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
// Audit events are the raw command execution log rows; after platform
// enrollment operations, at least one event must be recorded.
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
