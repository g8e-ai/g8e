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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// TestPubSub_HeartbeatAdvances proves heartbeat delivery over the pub/sub
// path by observing the active operator's UpdatedAt timestamp advance between
// two typed observations. The prior test checked that UpdatedAt was nonzero
// and matched a log line; this test requires a strictly later timestamp on
// the second observation, which can only be satisfied by a live heartbeat
// delivery — not a stale bootstrap value.
//
// The operator is discovered through the owner-authenticated operator list
// endpoint. No container logs, no Docker exec, no session ID from
// environment variables.
func TestPubSub_HeartbeatAdvances(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	first := activeOperator(t, ctx)
	firstUpdatedAt := first.UpdatedAt
	require.False(t, firstUpdatedAt.IsZero(),
		"first observation: active operator UpdatedAt must be set by at least one heartbeat")
	t.Logf("first heartbeat observation: updated_at=%s", firstUpdatedAt.UTC().Format(time.RFC3339Nano))

	// Poll until UpdatedAt advances past the first observation. The E2E
	// stack uses a short configurable heartbeat interval so a dead path
	// fails quickly.
	var second *models.OperatorDocumentGo
	require.Eventually(t, func() bool {
		operators, err := e2eClient.ListOperators(ctx)
		if err != nil {
			return false
		}
		second = findActiveRemoteOperator(operators.Operators)
		return second != nil && second.UpdatedAt.After(firstUpdatedAt)
	}, 45*time.Second, 500*time.Millisecond,
		"heartbeat UpdatedAt did not advance past %s within 45s — pub/sub heartbeat path may be dead",
		firstUpdatedAt.UTC().Format(time.RFC3339Nano))

	assert.True(t, second.UpdatedAt.After(firstUpdatedAt),
		"second heartbeat observation must be strictly later than the first")
	assert.Equal(t, constants.OperatorStatusActive, second.Status,
		"operator must remain active across heartbeat observations")
	assert.NotEmpty(t, second.CurrentHostname,
		"active remote operator CurrentHostname must be populated from heartbeat telemetry")
	assert.NotEmpty(t, second.LatestHeartbeat,
		"active remote operator LatestHeartbeat must be populated from heartbeat telemetry")
	t.Logf("second heartbeat observation: updated_at=%s (advanced by %s), hostname=%s",
		second.UpdatedAt.UTC().Format(time.RFC3339Nano),
		second.UpdatedAt.Sub(firstUpdatedAt).Round(time.Second),
		second.CurrentHostname)
}

// activeOperator fetches the operator list and returns a pointer to the first
// active remote operator. It fails the test if no active remote operator is found. The
// caller owns the context; this helper does not call require.Eventually or
// introduce its own polling — it is a single typed observation.
func activeOperator(t *testing.T, ctx context.Context) *models.OperatorDocumentGo {
	t.Helper()
	operators, err := e2eClient.ListOperators(ctx)
	require.NoError(t, err, "operator list must succeed for heartbeat observation")
	require.True(t, operators.Success, "operator list response must report success")
	require.NotEmpty(t, operators.Operators, "at least one operator must be registered")
	operator := findActiveRemoteOperator(operators.Operators)
	require.NotNil(t, operator, "no active remote operator found in registry — heartbeat test requires an approved stack with a live operator")
	return operator
}

func findActiveRemoteOperator(operators []models.OperatorDocumentGo) *models.OperatorDocumentGo {
	for i := range operators {
		if operators[i].Status == constants.OperatorStatusActive && operators[i].OperatorType == constants.OperatorTypeRemote {
			return &operators[i]
		}
	}
	return nil
}
