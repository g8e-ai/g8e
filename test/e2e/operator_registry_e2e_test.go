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

// TestOperatorRegistry_ActiveOperator verifies that after platform enrollment
// approval, the gateway's operator registry records the operator as active.
// The operator is discovered through the owner-authenticated operator list
// endpoint rather than parsing a session ID from container logs or
// environment variables. On an approved stack at least one operator must be
// registered with active status.
//
// This replaces the prior test that extracted the operator session ID from
// the container environment and queried a single session. The typed list
// endpoint is the canonical discovery path and does not depend on Docker
// introspection.
func TestOperatorRegistry_ActiveOperator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err, "owner-authenticated operator list must succeed")
	require.True(t, operators.Success, "operator list response must report success")
	require.NotEmpty(t, operators.Operators,
		"at least one operator must be registered on an approved stack")

	var active *models.OperatorDocumentGo
	for i := range operators.Operators {
		if operators.Operators[i].Status == constants.OperatorStatusActive {
			active = &operators.Operators[i]
			break
		}
	}
	require.NotNil(t, active, "at least one operator must have active status")
	assert.NotEmpty(t, active.ID, "active operator must have a non-empty ID")
	assert.NotEmpty(t, active.OperatorSessionID, "active operator must have a session ID")
	assert.False(t, active.CreatedAt.IsZero(), "active operator must have a creation timestamp")
	t.Logf("active operator discovered: id=%s session=%s slot=%d",
		active.ID, active.OperatorSessionID, active.SlotNumber)

	// Cross-check: the session lookup endpoint must return the same operator
	// document with active status. This proves the registry is consistent
	// across the list and session-scoped query paths.
	single, err := e2eClient.GetOperatorBySession(ctx, active.OperatorSessionID)
	require.NoError(t, err, "session lookup for the active operator must succeed")
	require.True(t, single.Success, "session lookup response must report success")
	require.NotNil(t, single.Operator, "session lookup must return an operator document")
	assert.Equal(t, active.ID, single.Operator.ID,
		"session lookup must return the same operator ID as the list")
	assert.Equal(t, constants.OperatorStatusActive, single.Operator.Status,
		"session lookup must report active status")
}

// TestOperatorRegistry_HeartbeatTimestampSet verifies that the active
// operator's UpdatedAt timestamp is set, indicating the pub/sub heartbeat
// path has delivered at least one update to the registry. The detailed
// heartbeat advancement assertion lives in the pubsub_heartbeat test; this
// test confirms the baseline that the registry is not stale at suite start.
func TestOperatorRegistry_HeartbeatTimestampSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultClientTimeout)
	defer cancel()

	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, operators.Operators)

	var active *models.OperatorDocumentGo
	for i := range operators.Operators {
		if operators.Operators[i].Status == constants.OperatorStatusActive {
			active = &operators.Operators[i]
			break
		}
	}
	require.NotNil(t, active, "an active operator must exist")
	assert.False(t, active.UpdatedAt.IsZero(),
		"active operator UpdatedAt must be set by at least one heartbeat delivery")
	// UpdatedAt should be recent relative to suite start, proving the
	// heartbeat path is live, not a stale bootstrap timestamp.
	assert.WithinDuration(t, time.Now(), active.UpdatedAt, 5*time.Minute,
		"active operator UpdatedAt must be within 5 minutes of now — heartbeat path may be stale")
}
