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

// TestPlatformEnrollment_Denial verifies that denying an operator enrollment
// request moves it to terminal state, removes it from the pending list, leaves
// the gateway healthy, and produces no active operator. The user starts the
// full stack (docker compose up) with the owner bootstrapped and no approvals,
// then runs:
//
//	./g8e test e2e --run TestPlatformEnrollment_Denial
//
// The test discovers the pending operator request, denies it by exact request
// ID, and asserts the consequences through typed gateway APIs. It does not
// restart or inspect containers; the denial is terminal and the operator
// remains unable to enroll, which is observable as the absence of an active
// operator in the registry.
//
// This replaces the prior TestPlatformEnrollment_Denial_E2E which used a
// per-test Docker fixture and asserted container healthcheck transitions.
// This version asserts the same security invariant (denied enrollment never
// produces an active operator) through the owner-authenticated operator list.
func TestPlatformEnrollment_Denial(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Discover the pending operator request. The operator submits its
	// enrollment request on startup; it may not be present immediately.
	var operatorReq *models.PlatformEnrollmentPendingRequest
	require.Eventually(t, func() bool {
		pending, err := e2eClient.GetPendingEnrollments(ctx)
		if err != nil {
			t.Logf("pending list attempt error: %v", err)
			return false
		}
		for i := range pending.Requests {
			if pending.Requests[i].ComponentKind == models.PlatformComponentOperator {
				operatorReq = &pending.Requests[i]
				return true
			}
		}
		return false
	}, 120*time.Second, 3*time.Second,
		"a pending operator enrollment request must appear in the pending list")
	require.NotNil(t, operatorReq, "operator request must be discovered before denial")
	require.NotEmpty(t, operatorReq.RequestID, "operator request must have a non-empty request ID")
	t.Logf("discovered pending operator request: %s (instance: %s, hostname: %s)",
		operatorReq.RequestID, operatorReq.InstanceID, operatorReq.Hostname)

	// Deny the exact request ID. The decision endpoint is owner-authenticated
	// and records the denial as a terminal state transition.
	require.NoError(t, e2eClient.DenyEnrollment(ctx, operatorReq.RequestID),
		"denying operator enrollment request %s must succeed", operatorReq.RequestID)
	t.Logf("denied operator enrollment request %s", operatorReq.RequestID)

	// The denied request must leave the pending list. It is now in the
	// denied terminal state, not pending. Poll until the operator request
	// is absent from the pending list.
	require.Eventually(t, func() bool {
		pending, err := e2eClient.GetPendingEnrollments(ctx)
		if err != nil {
			t.Logf("pending list check after denial error: %v", err)
			return false
		}
		for _, r := range pending.Requests {
			if r.RequestID == operatorReq.RequestID {
				return false
			}
		}
		return true
	}, 60*time.Second, 3*time.Second,
		"denied operator request %s must leave the pending list", operatorReq.RequestID)
	t.Logf("denied operator request %s is no longer pending", operatorReq.RequestID)

	// The gateway must remain healthy after the denial. A denial is a
	// routine owner decision; it must not destabilize the gateway.
	health, err := e2eClient.GetHealth(ctx, e2eCfg.gatewayHTTPURL)
	require.NoError(t, err, "gateway health must succeed after denial")
	assert.Equal(t, constants.GatewayModeStatusOK, health.Status,
		"gateway must remain healthy after denying an enrollment request")
	t.Logf("gateway remains healthy after denial: status=%s", health.Status)

	// No active operator must appear in the registry. A denied enrollment
	// never issues operator credentials, so the operator cannot register.
	// The operator list may be empty or contain only non-active entries
	// (e.g. from a prior approval on a reused stack); the assertion is that
	// no operator has active status.
	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err, "operator list must succeed after denial")
	for _, op := range operators.Operators {
		assert.NotEqual(t, constants.OperatorStatusActive, op.Status,
			"no operator must be active after its enrollment was denied (session=%s)",
			op.OperatorSessionID)
	}
	t.Logf("no active operator registered after denial (%d total operators)",
		len(operators.Operators))
}
