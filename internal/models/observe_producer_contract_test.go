// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// protocolModelsPath resolves a canonical protocol models JSON file relative to
// the internal/models package directory using the
// ProtocolSourceTreeRootFromInternalPkg constant. It avoids hand-rolled
// "../../protocol/models/..." literals in contract tests.
func protocolModelsPath(filename string) string {
	return filepath.Join(constants.ProtocolSourceTreeRootFromInternalPkg, constants.ProtocolDirname, constants.ProtocolModelsDirname, filename)
}

// TestObserveProducerResponseRoundTrip asserts the typed accepted response
// round-trips through JSON with the canonical wire field name and no extra
// fields.
func TestObserveProducerResponseRoundTrip(t *testing.T) {
	resp := ObserveProducerResponse{Accepted: true}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "accepted")
	assert.Len(t, raw, 1, "response must contain only the accepted field")

	var decoded ObserveProducerResponse
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, resp, decoded)
}

// TestObserveProducerAgentStateRequestRoundTrip asserts the agent producer
// request round-trips through JSON with canonical wire field names, omits
// optional fields when empty, and preserves every lifecycle enum.
func TestObserveProducerAgentStateRequestRoundTrip(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for _, status := range []AgentLifecycleStatus{
		AgentLifecycleStatusIdle, AgentLifecycleStatusQueued, AgentLifecycleStatusRunning,
		AgentLifecycleStatusWaiting, AgentLifecycleStatusCompleted, AgentLifecycleStatusFailed,
		AgentLifecycleStatusOffline,
	} {
		req := ObserveProducerAgentStateRequest{
			SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
			AgentID:       "triage",
			DisplayName:   "Triage",
			Role:          "triage",
			Status:        status,
			ObservedAt:    observedAt,
			WebSessionID:  "web-1",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err, "marshal failed for status %s", status)

		var decoded ObserveProducerAgentStateRequest
		require.NoError(t, json.Unmarshal(data, &decoded), "unmarshal failed for status %s", status)
		assert.Equal(t, req, decoded, "round-trip mismatch for status %s", status)
	}
}

// TestObserveProducerAgentStateRequestOmitsOptionalFields asserts the agent
// producer request omits optional fields when empty.
func TestObserveProducerAgentStateRequestOmitsOptionalFields(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "triage",
		DisplayName:   "Triage",
		Role:          "triage",
		Status:        AgentLifecycleStatusRunning,
		ObservedAt:    observedAt,
		WebSessionID:  "web-1",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "run_id")
	assert.NotContains(t, raw, "task_id")
	assert.NotContains(t, raw, "model")
	assert.NotContains(t, raw, "cli_session_id")
}

// TestObserveProducerRunStateRequestRoundTrip asserts the run producer
// request round-trips through JSON with canonical wire field names, omits
// optional timestamp fields when nil, and preserves every run kind and
// lifecycle enum.
func TestObserveProducerRunStateRequestRoundTrip(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for _, kind := range []RunKind{RunKindInvestigation, RunKindEval, RunKindDemo, RunKindWorkflow} {
		for _, status := range []RunLifecycleStatus{
			RunLifecycleStatusQueued, RunLifecycleStatusRunning, RunLifecycleStatusWaiting,
			RunLifecycleStatusCompleted, RunLifecycleStatusFailed, RunLifecycleStatusCancelled,
		} {
			req := ObserveProducerRunStateRequest{
				SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
				RunID:          "run-1",
				RunKind:        kind,
				DisplayName:    "Run",
				Status:         status,
				CompletedTasks: 1,
				TotalTasks:     5,
				ObservedAt:     observedAt,
				CLISessionID:   "cli-1",
			}
			data, err := json.Marshal(req)
			require.NoError(t, err, "marshal failed for kind=%s status=%s", kind, status)

			var decoded ObserveProducerRunStateRequest
			require.NoError(t, json.Unmarshal(data, &decoded), "unmarshal failed for kind=%s status=%s", kind, status)
			assert.Equal(t, req, decoded, "round-trip mismatch for kind=%s status=%s", kind, status)
		}
	}
}

// TestObserveProducerRunStateRequestOmitsOptionalFields asserts the run
// producer request omits optional fields when empty.
func TestObserveProducerRunStateRequestOmitsOptionalFields(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := ObserveProducerRunStateRequest{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		RunID:          "run-1",
		RunKind:        RunKindInvestigation,
		DisplayName:    "Run",
		Status:         RunLifecycleStatusRunning,
		CompletedTasks: 0,
		TotalTasks:     0,
		ObservedAt:     observedAt,
		WebSessionID:   "web-1",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "active_task_id")
	assert.NotContains(t, raw, "started_at")
	assert.NotContains(t, raw, "ended_at")
	assert.NotContains(t, raw, "cli_session_id")
}

// TestObserveProducerRequestWebRoutingSerializes asserts a web-routed producer
// request serializes with web_session_id set and cli_session_id omitted.
func TestObserveProducerRequestWebRoutingSerializes(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := ObserveProducerAgentStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "triage",
		DisplayName:   "Triage",
		Role:          "triage",
		Status:        AgentLifecycleStatusRunning,
		ObservedAt:    observedAt,
		WebSessionID:  "web-session-abc",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "web_session_id")
	assert.NotContains(t, raw, "cli_session_id")
}

// TestObserveProducerRequestCLIRoutingSerializes asserts a CLI-routed producer
// request serializes with cli_session_id set and web_session_id omitted.
func TestObserveProducerRequestCLIRoutingSerializes(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	req := ObserveProducerRunStateRequest{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		RunID:         "run-1",
		RunKind:       RunKindInvestigation,
		DisplayName:   "Run",
		Status:        RunLifecycleStatusRunning,
		TotalTasks:    3,
		ObservedAt:    observedAt,
		CLISessionID:  "cli-session-xyz",
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "cli_session_id")
	assert.NotContains(t, raw, "web_session_id")
}

// TestObserveProducerRequestNoUserIDField asserts the producer request wire
// shape contains no user_id field — the gateway derives user_id from the mTLS
// peer certificate, never from the request body.
func TestObserveProducerRequestNoUserIDField(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	reqs := []any{
		ObserveProducerAgentStateRequest{
			SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
			AgentID:       "triage",
			DisplayName:   "Triage",
			Role:          "triage",
			Status:        AgentLifecycleStatusRunning,
			ObservedAt:    observedAt,
			WebSessionID:  "web-1",
		},
		ObserveProducerRunStateRequest{
			SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
			RunID:         "run-1",
			RunKind:       RunKindInvestigation,
			DisplayName:   "Run",
			Status:        RunLifecycleStatusRunning,
			TotalTasks:    3,
			ObservedAt:    observedAt,
			WebSessionID:  "web-1",
		},
	}
	for _, req := range reqs {
		data, err := json.Marshal(req)
		require.NoError(t, err)
		assert.NotContains(t, string(data), "user_id",
			"producer request must not contain user_id field")
	}
}

// TestObserveAPIJSONContainsProducerResponse asserts the canonical protocol
// JSON model file contains the observe_producer_response entry.
func TestObserveAPIJSONContainsProducerResponse(t *testing.T) {
	data, err := os.ReadFile(protocolModelsPath(constants.ProtocolObserveAPIJSONFilename))
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Contains(t, raw, "observe_producer_response",
		"observe_api.json must contain observe_producer_response")
}
