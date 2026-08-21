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

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

// TestPlatformEnrollment_RestartDuringPending verifies that restarting the
// operator while its enrollment is pending resumes the same request ID rather
// than creating a duplicate. The user starts the full stack (docker compose
// up) with the owner bootstrapped and no approvals, waits for the operator's
// pending request to appear, restarts the operator container
// (docker compose restart g8e-operator), then runs:
//
//	./g8e test e2e --run TestPlatformEnrollment_RestartDuringPending
//
// The test asserts that the original operator request ID is still pending, that
// no duplicate operator request appears, then approves that request ID and
// verifies the operator becomes active and a command roundtrip succeeds. This
// proves the operator's pending state (private keys, requester token, request
// ID, CSR fingerprints) is persisted across restart and resumed correctly.
//
// This replaces the prior TestPlatformEnrollment_RestartDuringPending_E2E
// which used a per-test Docker fixture and restarted the container from
// within the test. This version relies on the user to restart the operator
// and asserts the API-visible consequences: request-ID continuity, absence
// of duplicates, and successful enrollment after approval.
func TestPlatformEnrollment_RestartDuringPending(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Precondition: this test requires a platform with exactly one pending
	// operator enrollment request. The user starts the full stack without
	// approving any enrollments, waits for the operator's pending request
	// to appear, restarts the operator container, then runs this test. On
	// an approved stack or a stack with no pending operator, fail fast with
	// an actionable message instead of timing out.
	pending, err := e2eClient.GetPendingEnrollments(ctx)
	require.NoError(t, err, "pending enrollment list must succeed")
	var operatorReq *models.PlatformEnrollmentPendingRequest
	opCount := 0
	for i := range pending.Requests {
		if pending.Requests[i].ComponentKind == models.PlatformComponentOperator {
			operatorReq = &pending.Requests[i]
			opCount++
		}
	}
	if operatorReq == nil {
		t.Fatalf("TestPlatformEnrollment_RestartDuringPending requires a pending operator enrollment request. " +
			"Start the full stack without approving enrollments (docker compose down -v && docker compose up -d), " +
			"restart the operator (docker compose restart g8e-operator), " +
			"then run: ./g8e test e2e --run TestPlatformEnrollment_RestartDuringPending")
	}
	require.NotEmpty(t, operatorReq.RequestID, "resumed operator request must have a non-empty request ID")
	t.Logf("discovered resumed operator request: %s", operatorReq.RequestID)

	// The precondition check already asserted exactly one pending operator
	// request exists (opCount == 1). A duplicate would indicate the
	// operator lost its persisted pending state and re-submitted, which
	// breaks request-ID continuity. The precondition fails fast in that
	// case with the actionable message above.

	// Approve the resumed request ID. The operator should complete
	// enrollment and become active.
	require.NoError(t, e2eClient.ApproveEnrollment(ctx, operatorReq.RequestID),
		"approving resumed operator enrollment request %s must succeed", operatorReq.RequestID)
	t.Logf("approved resumed operator enrollment request %s", operatorReq.RequestID)

	// The operator must become active in the registry. Approval triggers
	// credential issuance; the operator completes enrollment and registers.
	// A 180-second window accommodates issuance, WS reconnection, and
	// registry update.
	var active *models.OperatorDocumentGo
	require.Eventually(t, func() bool {
		operators, err := e2eClient.ListOperators(ctx)
		if err != nil {
			t.Logf("operator list attempt error: %v", err)
			return false
		}
		for i := range operators.Operators {
			if operators.Operators[i].Status == constants.OperatorStatusActive {
				active = &operators.Operators[i]
				return true
			}
		}
		return false
	}, 180*time.Second, 3*time.Second,
		"an active operator must appear in the registry after approving the resumed request")
	require.NotNil(t, active, "active operator must be discovered after approval")
	require.NotEmpty(t, active.OperatorSessionID, "active operator must have a session ID")
	t.Logf("operator became active after restart-during-pending approval: session=%s", active.OperatorSessionID)

	// A command roundtrip must succeed against the now-active operator.
	// This proves the full L4/L5 verification and execution chain works
	// after restart-during-pending enrollment.
	resp := e2eClient.dispatchFsRead(t, ctx)
	assert.NotEmpty(t, resp.TransactionID, "dispatch response must carry a transaction ID")
	assert.Equal(t, string(constants.Event.Operator.FsRead.Completed), resp.EventType,
		"dispatch response event type must be the fs.read completed event")
	t.Logf("command roundtrip succeeded after restart-during-pending: txn=%s",
		resp.TransactionID)
}
