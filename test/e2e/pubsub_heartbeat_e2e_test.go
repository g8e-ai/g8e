// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// TestDockerGateway_PubSubWebSocketConnectedAndHeartbeat verifies that:
// 1. The operator logs contain the explicit "operator pub/sub WebSocket connected" marker.
// 2. The gateway registry records recent heartbeat updates for the operator slot over pub/sub.
func TestDockerGateway_PubSubWebSocketConnectedAndHeartbeat(t *testing.T) {
	if sharedFixture == nil {
		t.Skip("Docker E2E fixture not available")
	}
	f := sharedFixture

	// Ensure operator container is running and has authenticated
	f.CheckOperatorContainer(t)

	// Verify the operator container logs contain the WebSocket connected marker
	startedAt := f.OperatorStartedAt(t)
	require.Eventually(t, func() bool {
		logs := f.OperatorLogsSince(t, startedAt)
		return strings.Contains(logs, "operator pub/sub WebSocket connected")
	}, 60*time.Second, 2*time.Second, "Operator logs do not contain 'operator pub/sub WebSocket connected' marker")

	// Verify heartbeat delivery by checking operator slot liveness in gateway registry
	sessionID := f.GetOperatorSessionID(t)
	require.NotEmpty(t, sessionID, "Operator session ID should not be empty")

	// Wait up to ~35s for at least one heartbeat delivery cycle to update UpdatedAt
	require.Eventually(t, func() bool {
		op := f.GetOperatorBySession(t, sessionID)
		if op == nil {
			return false
		}
		if op.Status != constants.OperatorStatusActive {
			return false
		}
		return !op.UpdatedAt.IsZero()
	}, 45*time.Second, 3*time.Second, "Operator slot in gateway registry did not record recent heartbeat update")

	op := f.GetOperatorBySession(t, sessionID)
	assert.Equal(t, constants.OperatorStatusActive, op.Status)
	assert.False(t, op.UpdatedAt.IsZero(), "Operator UpdatedAt timestamp should be set by heartbeat")
}
