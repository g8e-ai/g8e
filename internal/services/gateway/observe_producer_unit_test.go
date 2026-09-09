// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestIsValidAgentTransition_EmptyCurrentAcceptsAnyTarget(t *testing.T) {
	for _, status := range []models.AgentLifecycleStatus{
		models.AgentLifecycleStatusIdle,
		models.AgentLifecycleStatusQueued,
		models.AgentLifecycleStatusRunning,
		models.AgentLifecycleStatusWaiting,
		models.AgentLifecycleStatusCompleted,
		models.AgentLifecycleStatusFailed,
		models.AgentLifecycleStatusOffline,
	} {
		assert.True(t, isValidAgentTransition("", status),
			"empty current should accept any target status %s", status)
	}
}

func TestIsValidAgentTransition_SameStatusIsIdempotent(t *testing.T) {
	for _, status := range []models.AgentLifecycleStatus{
		models.AgentLifecycleStatusIdle,
		models.AgentLifecycleStatusQueued,
		models.AgentLifecycleStatusRunning,
		models.AgentLifecycleStatusWaiting,
		models.AgentLifecycleStatusCompleted,
		models.AgentLifecycleStatusFailed,
		models.AgentLifecycleStatusOffline,
	} {
		assert.True(t, isValidAgentTransition(status, status),
			"same-status transition should be idempotent for %s", status)
	}
}

func TestIsValidAgentTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  models.AgentLifecycleStatus
		to    models.AgentLifecycleStatus
		valid bool
	}{
		{"idle to queued", models.AgentLifecycleStatusIdle, models.AgentLifecycleStatusQueued, true},
		{"idle to running", models.AgentLifecycleStatusIdle, models.AgentLifecycleStatusRunning, true},
		{"idle to offline", models.AgentLifecycleStatusIdle, models.AgentLifecycleStatusOffline, true},
		{"queued to running", models.AgentLifecycleStatusQueued, models.AgentLifecycleStatusRunning, true},
		{"queued to failed", models.AgentLifecycleStatusQueued, models.AgentLifecycleStatusFailed, true},
		{"running to waiting", models.AgentLifecycleStatusRunning, models.AgentLifecycleStatusWaiting, true},
		{"running to completed", models.AgentLifecycleStatusRunning, models.AgentLifecycleStatusCompleted, true},
		{"running to failed", models.AgentLifecycleStatusRunning, models.AgentLifecycleStatusFailed, true},
		{"waiting to running", models.AgentLifecycleStatusWaiting, models.AgentLifecycleStatusRunning, true},
		{"waiting to completed", models.AgentLifecycleStatusWaiting, models.AgentLifecycleStatusCompleted, true},
		{"completed to idle (reset)", models.AgentLifecycleStatusCompleted, models.AgentLifecycleStatusIdle, true},
		{"completed to offline", models.AgentLifecycleStatusCompleted, models.AgentLifecycleStatusOffline, true},
		{"failed to idle (reset)", models.AgentLifecycleStatusFailed, models.AgentLifecycleStatusIdle, true},
		{"offline to idle (recovery)", models.AgentLifecycleStatusOffline, models.AgentLifecycleStatusIdle, true},
		{"offline to running (recovery)", models.AgentLifecycleStatusOffline, models.AgentLifecycleStatusRunning, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.valid, isValidAgentTransition(tc.from, tc.to))
		})
	}
}

func TestIsValidAgentTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  models.AgentLifecycleStatus
		to    models.AgentLifecycleStatus
		valid bool
	}{
		{"completed to running (terminal regression)", models.AgentLifecycleStatusCompleted, models.AgentLifecycleStatusRunning, false},
		{"completed to queued (terminal regression)", models.AgentLifecycleStatusCompleted, models.AgentLifecycleStatusQueued, false},
		{"completed to waiting (terminal regression)", models.AgentLifecycleStatusCompleted, models.AgentLifecycleStatusWaiting, false},
		{"failed to running (terminal regression)", models.AgentLifecycleStatusFailed, models.AgentLifecycleStatusRunning, false},
		{"failed to queued (terminal regression)", models.AgentLifecycleStatusFailed, models.AgentLifecycleStatusQueued, false},
		{"offline to completed (invalid recovery)", models.AgentLifecycleStatusOffline, models.AgentLifecycleStatusCompleted, false},
		{"offline to failed (invalid recovery)", models.AgentLifecycleStatusOffline, models.AgentLifecycleStatusFailed, false},
		{"idle to invalid status", models.AgentLifecycleStatusIdle, models.AgentLifecycleStatus("bogus"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.valid, isValidAgentTransition(tc.from, tc.to))
		})
	}
}

func TestIsValidRunTransition_EmptyCurrentAcceptsAnyTarget(t *testing.T) {
	for _, status := range []models.RunLifecycleStatus{
		models.RunLifecycleStatusQueued,
		models.RunLifecycleStatusRunning,
		models.RunLifecycleStatusWaiting,
		models.RunLifecycleStatusCompleted,
		models.RunLifecycleStatusFailed,
		models.RunLifecycleStatusCancelled,
	} {
		assert.True(t, isValidRunTransition("", status),
			"empty current should accept any target status %s", status)
	}
}

func TestIsValidRunTransition_SameStatusIsIdempotent(t *testing.T) {
	for _, status := range []models.RunLifecycleStatus{
		models.RunLifecycleStatusQueued,
		models.RunLifecycleStatusRunning,
		models.RunLifecycleStatusWaiting,
		models.RunLifecycleStatusCompleted,
		models.RunLifecycleStatusFailed,
		models.RunLifecycleStatusCancelled,
	} {
		assert.True(t, isValidRunTransition(status, status),
			"same-status transition should be idempotent for %s", status)
	}
}

func TestIsValidRunTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name  string
		from  models.RunLifecycleStatus
		to    models.RunLifecycleStatus
		valid bool
	}{
		{"queued to running", models.RunLifecycleStatusQueued, models.RunLifecycleStatusRunning, true},
		{"queued to cancelled", models.RunLifecycleStatusQueued, models.RunLifecycleStatusCancelled, true},
		{"queued to failed", models.RunLifecycleStatusQueued, models.RunLifecycleStatusFailed, true},
		{"running to waiting", models.RunLifecycleStatusRunning, models.RunLifecycleStatusWaiting, true},
		{"running to completed", models.RunLifecycleStatusRunning, models.RunLifecycleStatusCompleted, true},
		{"running to failed", models.RunLifecycleStatusRunning, models.RunLifecycleStatusFailed, true},
		{"running to cancelled", models.RunLifecycleStatusRunning, models.RunLifecycleStatusCancelled, true},
		{"waiting to running", models.RunLifecycleStatusWaiting, models.RunLifecycleStatusRunning, true},
		{"waiting to completed", models.RunLifecycleStatusWaiting, models.RunLifecycleStatusCompleted, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.valid, isValidRunTransition(tc.from, tc.to))
		})
	}
}

func TestIsValidRunTransition_TerminalStateRegressionRejected(t *testing.T) {
	terminalStates := []models.RunLifecycleStatus{
		models.RunLifecycleStatusCompleted,
		models.RunLifecycleStatusFailed,
		models.RunLifecycleStatusCancelled,
	}
	nonTerminalTargets := []models.RunLifecycleStatus{
		models.RunLifecycleStatusQueued,
		models.RunLifecycleStatusRunning,
		models.RunLifecycleStatusWaiting,
	}
	for _, terminal := range terminalStates {
		for _, target := range nonTerminalTargets {
			t.Run(string(terminal)+"->"+string(target), func(t *testing.T) {
				assert.False(t, isValidRunTransition(terminal, target),
					"terminal state %s must not regress to %s", terminal, target)
			})
		}
	}
}

func TestIsValidRunTransition_InvalidStatusRejected(t *testing.T) {
	assert.False(t, isValidRunTransition(models.RunLifecycleStatusRunning, models.RunLifecycleStatus("bogus")))
	assert.False(t, isValidRunTransition(models.RunLifecycleStatus("bogus"), models.RunLifecycleStatusRunning))
}

func TestSSEEventEnvelope_MarshalRoundTrip(t *testing.T) {
	data := []byte(`{"agent_id":"a1","status":"running"}`)
	env := sseEventEnvelope{
		Type: string(constants.EventAppAgentStatusUpdated),
		Data: data,
	}
	b, err := json.Marshal(env)
	assert.NoError(t, err)
	assert.Contains(t, string(b), string(constants.EventAppAgentStatusUpdated))
	assert.Contains(t, string(b), "agent_id")
}

func TestObserveProducerID_IsStableIdentifier(t *testing.T) {
	assert.Equal(t, "g8e-gateway-observe-producer", observeProducerID)
}

func TestAgentTransitions_AllStatusesHaveEntry(t *testing.T) {
	allStatuses := []models.AgentLifecycleStatus{
		models.AgentLifecycleStatusIdle,
		models.AgentLifecycleStatusQueued,
		models.AgentLifecycleStatusRunning,
		models.AgentLifecycleStatusWaiting,
		models.AgentLifecycleStatusCompleted,
		models.AgentLifecycleStatusFailed,
		models.AgentLifecycleStatusOffline,
	}
	for _, s := range allStatuses {
		_, ok := agentTransitions[s]
		assert.True(t, ok, "agentTransitions must have an entry for %s", s)
	}
}

func TestRunTransitions_AllStatusesHaveEntry(t *testing.T) {
	allStatuses := []models.RunLifecycleStatus{
		models.RunLifecycleStatusQueued,
		models.RunLifecycleStatusRunning,
		models.RunLifecycleStatusWaiting,
		models.RunLifecycleStatusCompleted,
		models.RunLifecycleStatusFailed,
		models.RunLifecycleStatusCancelled,
	}
	for _, s := range allStatuses {
		_, ok := runTransitions[s]
		assert.True(t, ok, "runTransitions must have an entry for %s", s)
	}
}

// TestAgentTransitions_EveryStatusCanReachTerminal ensures every non-terminal
// agent state can reach at least one terminal state (completed or failed) so
// agents do not get stuck in non-terminal states without a valid exit.
func TestAgentTransitions_EveryNonTerminalCanReachTerminal(t *testing.T) {
	nonTerminal := []models.AgentLifecycleStatus{
		models.AgentLifecycleStatusIdle,
		models.AgentLifecycleStatusQueued,
		models.AgentLifecycleStatusRunning,
		models.AgentLifecycleStatusWaiting,
	}
	for _, s := range nonTerminal {
		canComplete := isValidAgentTransition(s, models.AgentLifecycleStatusCompleted)
		canFail := isValidAgentTransition(s, models.AgentLifecycleStatusFailed)
		assert.True(t, canComplete || canFail,
			"non-terminal agent state %s must be able to reach a terminal state", s)
	}
}

// TestRunTransitions_EveryNonTerminalCanReachTerminal ensures every
// non-terminal run state can reach at least one terminal state.
func TestRunTransitions_EveryNonTerminalCanReachTerminal(t *testing.T) {
	nonTerminal := []models.RunLifecycleStatus{
		models.RunLifecycleStatusQueued,
		models.RunLifecycleStatusRunning,
		models.RunLifecycleStatusWaiting,
	}
	for _, s := range nonTerminal {
		canComplete := isValidRunTransition(s, models.RunLifecycleStatusCompleted)
		canFail := isValidRunTransition(s, models.RunLifecycleStatusFailed)
		canCancel := isValidRunTransition(s, models.RunLifecycleStatusCancelled)
		assert.True(t, canComplete || canFail || canCancel,
			"non-terminal run state %s must be able to reach a terminal state", s)
	}
}

// TestErrorConstants_ObserveProducerErrorsAreDistinct ensures the producer
// error constants are distinct and wrappable with errors.Is.
func TestErrorConstants_ObserveProducerErrorsAreDistinct(t *testing.T) {
	assert.NotEqual(t, constants.ErrObserveInvalidTransition, constants.ErrObserveStaleUpdate)
	assert.True(t, errors.Is(constants.ErrObserveInvalidTransition, constants.ErrObserveInvalidTransition))
	assert.True(t, errors.Is(constants.ErrObserveStaleUpdate, constants.ErrObserveStaleUpdate))
	assert.False(t, errors.Is(constants.ErrObserveInvalidTransition, constants.ErrObserveStaleUpdate))
}

// keep time import used for future expansion of timestamp-based assertions.
var _ = time.Time{}
