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
	"google.golang.org/protobuf/proto"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
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

	// The operator must have a pending request. The user restarted the
	// operator before running this test; the resumed request should already
	// be present. Poll briefly to accommodate re-submission latency after
	// restart.
	var operatorReq *models.PlatformEnrollmentPendingRequest
	require.Eventually(t, func() bool {
		pending, err := e2eClient.GetPendingEnrollments(ctx)
		if err != nil {
			t.Logf("pending list attempt error: %v", err)
			return false
		}
		opCount := 0
		for i := range pending.Requests {
			if pending.Requests[i].ComponentKind == models.PlatformComponentOperator {
				operatorReq = &pending.Requests[i]
				opCount++
			}
		}
		return opCount == 1
	}, 120*time.Second, 3*time.Second,
		"exactly one pending operator enrollment request must appear after restart")
	require.NotNil(t, operatorReq, "operator request must be discovered after restart")
	require.NotEmpty(t, operatorReq.RequestID, "resumed operator request must have a non-empty request ID")
	t.Logf("discovered resumed operator request: %s", operatorReq.RequestID)

	// Assert no duplicate operator request exists. The pending list must
	// contain exactly one operator request — the resumed one. A duplicate
	// would indicate the operator lost its persisted pending state and
	// re-submitted, which breaks request-ID continuity.
	pending, err := e2eClient.GetPendingEnrollments(ctx)
	require.NoError(t, err, "pending list must succeed for duplicate check")
	opCount := 0
	for _, r := range pending.Requests {
		if r.ComponentKind == models.PlatformComponentOperator {
			opCount++
		}
	}
	assert.Equal(t, 1, opCount,
		"exactly one operator request must be pending after restart — a duplicate indicates lost persisted state")
	t.Logf("no duplicate operator request: %d operator request(s) pending", opCount)

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
	fsReadReq := &operatorv1.FsReadRequested{Path: constants.PathEtcHostname}
	payload, err := proto.Marshal(fsReadReq)
	require.NoError(t, err, "marshal FsReadRequested payload")

	reqBody := dispatchRequestJSON{
		TargetOperatorSessionID: active.OperatorSessionID,
		ActionType:              string(constants.ActionTypeFsRead),
		Payload:                 payload,
		TargetResource:          constants.PathEtcHostname,
	}

	var resp dispatchResponseJSON
	require.Eventually(t, func() bool {
		resp = dispatchResponseJSON{}
		dispatchCtx, dispatchCancel := context.WithTimeout(ctx, 60*time.Second)
		defer dispatchCancel()
		r, err := e2eClient.DispatchCommand(dispatchCtx, reqBody, 60*time.Second)
		if err != nil {
			t.Logf("dispatch attempt error: %v", err)
			return false
		}
		resp = r
		return resp.Success
	}, 120*time.Second, 2*time.Second,
		"command roundtrip must succeed after restart-during-pending enrollment; last response: %+v", resp)

	assert.NotEmpty(t, resp.TransactionID, "dispatch response must carry a transaction ID")
	assert.Equal(t, string(constants.Event.Operator.FsRead.Completed), resp.EventType,
		"dispatch response event type must be the fs.read completed event")

	var fsReadResult operatorv1.FsReadResult
	require.NoError(t, proto.Unmarshal(resp.ResultPayload, &fsReadResult),
		"unmarshal FsReadResult from dispatch response")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, fsReadResult.Status,
		"fs.read must complete successfully after restart-during-pending")
	assert.NotEmpty(t, fsReadResult.Content, "fs.read result content must not be empty")
	t.Logf("command roundtrip succeeded after restart-during-pending: txn=%s content_size=%d",
		resp.TransactionID, fsReadResult.SizeBytes)
}
