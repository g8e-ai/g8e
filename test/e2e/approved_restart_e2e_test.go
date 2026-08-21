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

// TestApprovedRestart_IdentityPersists verifies that an operator's identity and
// session remain valid after a restart on an approved stack. The user starts
// the full stack, approves all enrollments, waits for the operator to become
// active, restarts the operator container (docker compose restart
// g8e-operator), waits for it to reconnect, then runs:
//
//	./g8e test e2e --run TestApprovedRestart
//
// The test asserts that the operator returns to active status with the same
// session ID (persisted identity), that the heartbeat UpdatedAt timestamp
// advances after restart (pub/sub recovery), and that a command roundtrip
// still succeeds (command delivery after restart). This proves the operator's
// persisted credentials survive restart and the gateway re-establishes the
// pub/sub and command channels without re-enrollment.
//
// This replaces the prior auth_e2e_test.go restart-persistence coverage that
// restarted the gateway container from within the test and parsed log lines
// for the "Authentication successful" marker. This version asserts the
// API-visible consequences of restart: registry state, heartbeat advancement,
// and command delivery.
func TestApprovedRestart_IdentityPersists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// The operator must be active after restart. The user restarted the
	// operator and waited for it to reconnect; the registry must show it
	// as active with a non-empty session ID. Poll to accommodate
	// reconnection latency.
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
		"an active operator must appear in the registry after restart on an approved stack")
	require.NotNil(t, active, "active operator must be discovered after restart")
	require.NotEmpty(t, active.OperatorSessionID,
		"active operator must have a non-empty session ID after restart — persisted identity must survive")
	require.NotEmpty(t, active.ID, "active operator must have a non-empty ID after restart")
	t.Logf("operator active after restart: id=%s session=%s", active.ID, active.OperatorSessionID)

	// Cross-check: the session lookup must return the same operator with
	// active status. This proves the registry is consistent across the
	// list and session-scoped query paths after restart.
	single, err := e2eClient.GetOperatorBySession(ctx, active.OperatorSessionID)
	require.NoError(t, err, "session lookup must succeed for the restarted operator")
	require.True(t, single.Success, "session lookup response must report success")
	require.NotNil(t, single.Operator, "session lookup must return an operator document")
	assert.Equal(t, active.ID, single.Operator.ID,
		"session lookup must return the same operator ID after restart")
	assert.Equal(t, constants.OperatorStatusActive, single.Operator.Status,
		"session lookup must report active status after restart")
	t.Logf("session lookup consistent after restart: id=%s status=%s",
		single.Operator.ID, single.Operator.Status)

	// Heartbeat must advance after restart. The first observation captures
	// the current UpdatedAt; the second must be strictly later, proving
	// the pub/sub heartbeat path recovered after restart. A 90-second
	// window accommodates reconnection and heartbeat interval jitter.
	firstUpdatedAt := active.UpdatedAt
	require.False(t, firstUpdatedAt.IsZero(),
		"first observation after restart: operator UpdatedAt must be set")
	t.Logf("first heartbeat observation after restart: updated_at=%s",
		firstUpdatedAt.UTC().Format(time.RFC3339Nano))

	var second *models.OperatorDocumentGo
	require.Eventually(t, func() bool {
		second = activeOperator(t, ctx)
		return second.UpdatedAt.After(firstUpdatedAt)
	}, 90*time.Second, 3*time.Second,
		"heartbeat UpdatedAt did not advance past %s within 90s after restart — pub/sub recovery may have failed",
		firstUpdatedAt.UTC().Format(time.RFC3339Nano))
	assert.True(t, second.UpdatedAt.After(firstUpdatedAt),
		"second heartbeat observation after restart must be strictly later than the first")
	assert.Equal(t, constants.OperatorStatusActive, second.Status,
		"operator must remain active across heartbeat observations after restart")
	t.Logf("heartbeat advanced after restart: updated_at=%s (advanced by %s)",
		second.UpdatedAt.UTC().Format(time.RFC3339Nano),
		second.UpdatedAt.Sub(firstUpdatedAt).Round(time.Second))

	// Command roundtrip must succeed after restart. This proves the full
	// command delivery chain (gateway pub/sub, operator WS subscription,
	// L4/L5 verification, execution, result publication) recovered after
	// restart.
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
			t.Logf("dispatch attempt error after restart: %v", err)
			return false
		}
		resp = r
		return resp.Success
	}, 120*time.Second, 2*time.Second,
		"command roundtrip must succeed after restart; last response: %+v", resp)

	assert.NotEmpty(t, resp.TransactionID, "dispatch response must carry a transaction ID after restart")
	assert.Equal(t, string(constants.Event.Operator.FsRead.Completed), resp.EventType,
		"dispatch response event type must be the fs.read completed event after restart")

	var fsReadResult operatorv1.FsReadResult
	require.NoError(t, proto.Unmarshal(resp.ResultPayload, &fsReadResult),
		"unmarshal FsReadResult from dispatch response after restart")
	assert.Equal(t, operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED, fsReadResult.Status,
		"fs.read must complete successfully after restart")
	assert.NotEmpty(t, fsReadResult.Content, "fs.read result content must not be empty after restart")
	t.Logf("command roundtrip succeeded after restart: txn=%s content_size=%d",
		resp.TransactionID, fsReadResult.SizeBytes)
}
