// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// validAgentReq returns a fully valid agent producer request for mutation in
// table-driven tests.
func validAgentReq() models.ObserveProducerAgentStateRequest {
	return models.ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "agent-1",
		DisplayName:   "Triage",
		Role:          "triage",
		Status:        models.AgentLifecycleStatusRunning,
		ObservedAt:    time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		WebSessionID:  "web-1",
	}
}

// validRunReq returns a fully valid run producer request for mutation in
// table-driven tests.
func validRunReq() models.ObserveProducerRunStateRequest {
	return models.ObserveProducerRunStateRequest{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		RunID:          "run-1",
		RunKind:        models.RunKindInvestigation,
		DisplayName:    "Investigation",
		Status:         models.RunLifecycleStatusRunning,
		CompletedTasks: 1,
		TotalTasks:     5,
		ObservedAt:     time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		WebSessionID:   "web-1",
	}
}

func TestValidateAgentProducerRequest_AcceptsEveryValidStatus(t *testing.T) {
	for _, status := range []models.AgentLifecycleStatus{
		models.AgentLifecycleStatusIdle, models.AgentLifecycleStatusQueued,
		models.AgentLifecycleStatusRunning, models.AgentLifecycleStatusWaiting,
		models.AgentLifecycleStatusCompleted, models.AgentLifecycleStatusFailed,
		models.AgentLifecycleStatusOffline,
	} {
		req := validAgentReq()
		req.Status = status
		assert.NoError(t, validateAgentProducerRequest(req),
			"valid status %s should be accepted", status)
	}
}

func TestValidateAgentProducerRequest_RejectsUnsupportedSchemaVersion(t *testing.T) {
	req := validAgentReq()
	req.SchemaVersion = "9.9.9"
	err := validateAgentProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveUnsupportedSchemaVersion))
}

func TestValidateAgentProducerRequest_RejectsEmptyDisplayName(t *testing.T) {
	req := validAgentReq()
	req.DisplayName = ""
	err := validateAgentProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveAgentDisplayNameRequired))
}

func TestValidateAgentProducerRequest_RejectsEmptyRole(t *testing.T) {
	req := validAgentReq()
	req.Role = ""
	err := validateAgentProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveAgentRoleRequired))
}

func TestValidateAgentProducerRequest_RejectsUnknownStatusEvenOnFirstWrite(t *testing.T) {
	req := validAgentReq()
	req.Status = models.AgentLifecycleStatus("bogus")
	err := validateAgentProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveInvalidTransition),
		"unknown first-write status must be rejected, not accepted as 'no existing projection'")
}

func TestValidateRunProducerRequest_AcceptsEveryValidRunKindAndStatus(t *testing.T) {
	for _, kind := range []models.RunKind{
		models.RunKindInvestigation, models.RunKindEval,
		models.RunKindDemo, models.RunKindWorkflow,
	} {
		for _, status := range []models.RunLifecycleStatus{
			models.RunLifecycleStatusQueued, models.RunLifecycleStatusRunning,
			models.RunLifecycleStatusWaiting, models.RunLifecycleStatusCompleted,
			models.RunLifecycleStatusFailed, models.RunLifecycleStatusCancelled,
		} {
			req := validRunReq()
			req.RunKind = kind
			req.Status = status
			assert.NoError(t, validateRunProducerRequest(req),
				"valid kind=%s status=%s should be accepted", kind, status)
		}
	}
}

func TestValidateRunProducerRequest_AcceptsValidZeroCounters(t *testing.T) {
	req := validRunReq()
	req.CompletedTasks = 0
	req.TotalTasks = 0
	assert.NoError(t, validateRunProducerRequest(req),
		"zero counters are valid")
}

func TestValidateRunProducerRequest_RejectsUnsupportedSchemaVersion(t *testing.T) {
	req := validRunReq()
	req.SchemaVersion = "0.0.1"
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveUnsupportedSchemaVersion))
}

func TestValidateRunProducerRequest_RejectsEmptyDisplayName(t *testing.T) {
	req := validRunReq()
	req.DisplayName = ""
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveRunDisplayNameRequired))
}

func TestValidateRunProducerRequest_RejectsUnknownRunKind(t *testing.T) {
	req := validRunReq()
	req.RunKind = models.RunKind("bogus")
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveRunKindRequired))
}

func TestValidateRunProducerRequest_RejectsUnknownStatusEvenOnFirstWrite(t *testing.T) {
	req := validRunReq()
	req.Status = models.RunLifecycleStatus("bogus")
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveInvalidTransition),
		"unknown first-write status must be rejected")
}

func TestValidateRunProducerRequest_RejectsNegativeCompletedTasks(t *testing.T) {
	req := validRunReq()
	req.CompletedTasks = -1
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveNegativeTaskCount))
}

func TestValidateRunProducerRequest_RejectsNegativeTotalTasks(t *testing.T) {
	req := validRunReq()
	req.TotalTasks = -1
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveNegativeTaskCount))
}

func TestValidateRunProducerRequest_RejectsCompletedExceedsTotal(t *testing.T) {
	req := validRunReq()
	req.CompletedTasks = 5
	req.TotalTasks = 3
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveCompletedExceedsTotal))
}

func TestValidateRunProducerRequest_RejectsEndedBeforeStarted(t *testing.T) {
	started := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	ended := started.Add(-1 * time.Minute)
	req := validRunReq()
	req.StartedAt = &started
	req.EndedAt = &ended
	err := validateRunProducerRequest(req)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrObserveEndBeforeStart))
}

func TestValidateRunProducerRequest_AcceptsEndedEqualToStarted(t *testing.T) {
	ts := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := validRunReq()
	req.StartedAt = &ts
	req.EndedAt = &ts
	assert.NoError(t, validateRunProducerRequest(req),
		"ended_at equal to started_at is valid (not strictly before)")
}

func TestValidateRunProducerRequest_AcceptsStartedOnly(t *testing.T) {
	started := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := validRunReq()
	req.StartedAt = &started
	assert.NoError(t, validateRunProducerRequest(req),
		"started_at without ended_at is valid")
}

func TestValidateRunProducerRequest_AcceptsEndedOnly(t *testing.T) {
	ended := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := validRunReq()
	req.EndedAt = &ended
	assert.NoError(t, validateRunProducerRequest(req),
		"ended_at without started_at is valid")
}

// TestDecodeProducerRequest_RejectsUnknownFields asserts the strict decode
// helper rejects unknown fields in the request body.
func TestDecodeProducerRequest_RejectsUnknownFields(t *testing.T) {
	body := []byte(`{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"r","status":"running","observed_at":"2026-09-09T12:00:00Z","web_session_id":"w","evil":"no"}`)
	var dst models.ObserveProducerAgentStateRequest
	err := decodeProducerRequest(body, &dst)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONBody))
}

// TestDecodeProducerRequest_RejectsTrailingJSON asserts the strict decode
// helper rejects trailing JSON after the top-level value.
func TestDecodeProducerRequest_RejectsTrailingJSON(t *testing.T) {
	body := []byte(`{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"r","status":"running","observed_at":"2026-09-09T12:00:00Z","web_session_id":"w"}{"extra":1}`)
	var dst models.ObserveProducerAgentStateRequest
	err := decodeProducerRequest(body, &dst)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONBody))
}

// TestDecodeProducerRequest_RejectsMalformedJSON asserts the strict decode
// helper rejects malformed JSON.
func TestDecodeProducerRequest_RejectsMalformedJSON(t *testing.T) {
	body := []byte(`{not json}`)
	var dst models.ObserveProducerAgentStateRequest
	err := decodeProducerRequest(body, &dst)
	require.Error(t, err)
	assert.True(t, errors.Is(err, constants.ErrInvalidJSONBody))
}

// TestDecodeProducerRequest_AcceptsValidJSON asserts the strict decode
// helper accepts valid JSON with no unknown fields or trailing content.
func TestDecodeProducerRequest_AcceptsValidJSON(t *testing.T) {
	body := []byte(`{"schema_version":"1.0.0","agent_id":"a","display_name":"A","role":"r","status":"running","observed_at":"2026-09-09T12:00:00Z","web_session_id":"w"}`)
	var dst models.ObserveProducerAgentStateRequest
	err := decodeProducerRequest(body, &dst)
	require.NoError(t, err)
	assert.Equal(t, "a", dst.AgentID)
	assert.Equal(t, "w", dst.WebSessionID)
}


