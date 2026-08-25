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

// TestApprovedRestart_IdentityPersists verifies that an operator's identity and
// session remain valid on an approved stack. The user starts the full stack,
// approves all enrollments, waits for the operator to become active, then runs:
//
//	./g8e test e2e --run TestApprovedRestart
//
// The test asserts that the operator is active with a non-empty session ID
// (persisted identity), that the heartbeat UpdatedAt timestamp advances
// (pub/sub liveness), and that a command roundtrip succeeds (command
// delivery). When run after a user-initiated operator restart
// (docker compose restart g8e-operator), this proves the operator's persisted
// credentials survive restart and the gateway re-establishes the pub/sub and
// command channels without re-enrollment. On a fresh approved stack without
// restart, the same assertions verify baseline operator liveness.
//
// This replaces the prior auth_e2e_test.go restart-persistence coverage that
// restarted the gateway container from within the test and parsed log lines
// for the "Authentication successful" marker. This version asserts the
// API-visible consequences: registry state, heartbeat advancement, and
// command delivery.
func TestApprovedRestart_IdentityPersists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// The operator must be active. Poll to accommodate reconnection latency
	// after a restart, or initial registration on a fresh approved stack.
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
		"an active operator must appear in the registry on an approved stack")
	require.NotNil(t, active, "active operator must be discovered")
	require.NotEmpty(t, active.OperatorSessionID,
		"active operator must have a non-empty session ID — persisted identity must survive")
	require.NotEmpty(t, active.ID, "active operator must have a non-empty ID")
	t.Logf("operator active: id=%s session=%s", active.ID, active.OperatorSessionID)

	// Cross-check: the session lookup must return the same operator with
	// active status. This proves the registry is consistent across the
	// list and session-scoped query paths.
	single, err := e2eClient.GetOperatorBySession(ctx, active.OperatorSessionID)
	require.NoError(t, err, "session lookup must succeed for the active operator")
	require.True(t, single.Success, "session lookup response must report success")
	require.NotNil(t, single.Operator, "session lookup must return an operator document")
	assert.Equal(t, active.ID, single.Operator.ID,
		"session lookup must return the same operator ID")
	assert.Equal(t, constants.OperatorStatusActive, single.Operator.Status,
		"session lookup must report active status")
	t.Logf("session lookup consistent: id=%s status=%s",
		single.Operator.ID, single.Operator.Status)

	// Heartbeat must advance. The first observation captures the current
	// UpdatedAt; the second must be strictly later, proving the pub/sub
	// heartbeat path is live. A 90-second window accommodates heartbeat
	// interval jitter and pub/sub delivery latency.
	firstUpdatedAt := active.UpdatedAt
	require.False(t, firstUpdatedAt.IsZero(),
		"first observation: operator UpdatedAt must be set")
	t.Logf("first heartbeat observation: updated_at=%s",
		firstUpdatedAt.UTC().Format(time.RFC3339Nano))

	var second *models.OperatorDocumentGo
	require.Eventually(t, func() bool {
		second = activeOperator(t, ctx)
		return second.UpdatedAt.After(firstUpdatedAt)
	}, 90*time.Second, 3*time.Second,
		"heartbeat UpdatedAt did not advance past %s within 90s — pub/sub heartbeat path may have failed",
		firstUpdatedAt.UTC().Format(time.RFC3339Nano))
	assert.True(t, second.UpdatedAt.After(firstUpdatedAt),
		"second heartbeat observation must be strictly later than the first")
	assert.Equal(t, constants.OperatorStatusActive, second.Status,
		"operator must remain active across heartbeat observations")
	t.Logf("heartbeat advanced: updated_at=%s (advanced by %s)",
		second.UpdatedAt.UTC().Format(time.RFC3339Nano),
		second.UpdatedAt.Sub(firstUpdatedAt).Round(time.Second))

	// Command roundtrip must succeed. This proves the full command delivery
	// chain (gateway pub/sub, operator WS subscription, L4/L5 verification,
	// execution, result publication) is operational.
	resp := e2eClient.dispatchFsRead(t, ctx)
	assert.NotEmpty(t, resp.TransactionID, "dispatch response must carry a transaction ID")
	assert.Equal(t, string(constants.Event.Operator.FsRead.Completed), resp.EventType,
		"dispatch response event type must be the fs.read completed event")
	t.Logf("command roundtrip succeeded: txn=%s", resp.TransactionID)
}
