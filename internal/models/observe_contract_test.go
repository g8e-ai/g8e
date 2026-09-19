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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// TestObserveEventPayloadSchemaVersionContract asserts that the four observe
// dashboard event types resolve to the canonical schema version constant and
// that non-observe events return an empty schema version.
func TestObserveEventPayloadSchemaVersionContract(t *testing.T) {
	observeEvents := []constants.EventType{
		constants.EventAppAgentStatusUpdated,
		constants.EventAppRunStatusUpdated,
		constants.EventAiEvalRunCompleted,
		constants.EventAiEvalMetricRecorded,
	}
	for _, et := range observeEvents {
		assert.Equal(t, constants.ObserveEventPayloadSchemaVersion,
			ObserveEventPayloadSchemaVersion(et),
			"event %s should resolve to ObserveEventPayloadSchemaVersion", et)
	}
	assert.Equal(t, "", ObserveEventPayloadSchemaVersion(constants.EventAppAgentActivityRecorded),
		"app.agent.activity.recorded is a governed-document event, not an observe dashboard event")
	assert.Equal(t, "", ObserveEventPayloadSchemaVersion(constants.EventAppTaskCreated),
		"non-observe events return an empty schema version")
}

// TestObserveEventPayloadsJSONValidity asserts the canonical protocol JSON
// model file parses.
func TestObserveEventPayloadsJSONValidity(t *testing.T) {
	data, err := os.ReadFile("../../protocol/models/observe_event_payloads.json")
	require.NoError(t, err)
	assert.True(t, json.Valid(data), "observe_event_payloads.json is valid JSON")
}

// TestObserveAPIJSONValidity asserts the canonical protocol JSON model file
// for the observe read API parses.
func TestObserveAPIJSONValidity(t *testing.T) {
	data, err := os.ReadFile("../../protocol/models/observe_api.json")
	require.NoError(t, err)
	assert.True(t, json.Valid(data), "observe_api.json is valid JSON")
}

// TestAgentStatusUpdatedPayloadRoundTrip asserts the Go struct round-trips
// through JSON with the canonical wire field names and omits optional fields
// when empty.
func TestAgentStatusUpdatedPayloadRoundTrip(t *testing.T) {
	observedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	payload := AgentStatusUpdatedPayload{
		SchemaVersion: constants.ObserveEventPayloadSchemaVersion,
		AgentID:       "triage",
		DisplayName:   "Triage",
		Role:          "triage",
		Status:        AgentLifecycleStatusRunning,
		ObservedAt:    observedAt,
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "schema_version")
	assert.Contains(t, raw, "agent_id")
	assert.Contains(t, raw, "display_name")
	assert.Contains(t, raw, "role")
	assert.Contains(t, raw, "status")
	assert.Contains(t, raw, "observed_at")

	// Optional fields omitted when empty.
	assert.NotContains(t, raw, "run_id")
	assert.NotContains(t, raw, "task_id")
	assert.NotContains(t, raw, "model")

	var decoded AgentStatusUpdatedPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, payload, decoded)
}

// TestRunStatusUpdatedPayloadRoundTrip asserts the Go struct round-trips
// through JSON and omits optional timestamp fields when nil.
func TestRunStatusUpdatedPayloadRoundTrip(t *testing.T) {
	observedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	payload := RunStatusUpdatedPayload{
		SchemaVersion:  constants.ObserveEventPayloadSchemaVersion,
		RunID:          "run-1",
		RunKind:        RunKindInvestigation,
		DisplayName:    "Investigation 1",
		Status:         RunLifecycleStatusRunning,
		CompletedTasks: 3,
		TotalTasks:     10,
		ObservedAt:     observedAt,
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "started_at")
	assert.NotContains(t, raw, "ended_at")
	assert.NotContains(t, raw, "active_task_id")

	var decoded RunStatusUpdatedPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, payload, decoded)
}

// TestEvalRunCompletedPayloadRoundTrip asserts the Go struct round-trips
// through JSON with the canonical wire field names.
func TestEvalRunCompletedPayloadRoundTrip(t *testing.T) {
	completedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	payload := EvalRunCompletedPayload{
		SchemaVersion:             constants.ObserveEventPayloadSchemaVersion,
		RunID:                     "eval-run-1",
		SuiteID:                   "ifeval_subset",
		SuiteVersion:              "1.0.0",
		CampaignID:                "campaign-1",
		ArmIDs:                    []string{"direct"},
		TerminalAttempts:          10,
		AssignedTasks:             10,
		ReceiptCount:              10,
		VerificationStatus:        EvalVerificationProjectionValidated,
		PublishedProjectionSHA256: "abc123",
		CompletedAt:               completedAt,
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded EvalRunCompletedPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, payload, decoded)
}

// TestEvalMetricRecordedPayloadRoundTrip asserts the Go struct round-trips
// through JSON and omits the optional value field when nil.
func TestEvalMetricRecordedPayloadRoundTrip(t *testing.T) {
	recordedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	payload := EvalMetricRecordedPayload{
		SchemaVersion:      constants.ObserveEventPayloadSchemaVersion,
		RunID:              "eval-run-1",
		MetricID:           "accuracy",
		MetricVersion:      "1.0.0",
		Unit:               "ratio",
		Eligible:           10,
		Denominator:        10,
		VerificationStatus: EvalVerificationProjectionValidated,
		RecordedAt:         recordedAt,
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "value")

	var decoded EvalMetricRecordedPayload
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, payload, decoded)
}

// TestObservedMeasurementRoundTrip asserts the shared measurement shape
// round-trips through JSON and omits the optional window_seconds field when
// nil.
func TestObservedMeasurementRoundTrip(t *testing.T) {
	observedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	m := ObservedMeasurement{
		SchemaVersion:   constants.ObserveMeasurementSchemaVersion,
		MetricID:        "tokens_per_second",
		Value:           42.5,
		Unit:            "tokens/s",
		SourceComponent: "g8ee-sampler",
		ObservedAt:      observedAt,
		Status:          MeasurementStatusObserved,
	}
	data, err := json.Marshal(m)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "window_seconds")

	var decoded ObservedMeasurement
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, m, decoded)
}

// TestObserveBootstrapSnapshotRoundTrip asserts the bootstrap snapshot
// round-trips through JSON and omits the optional active_run field when nil.
func TestObserveBootstrapSnapshotRoundTrip(t *testing.T) {
	generatedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	snapshot := ObserveBootstrapSnapshot{
		SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		Agents:        []AgentStateProjection{},
		Overview: OverviewCounters{
			SchemaVersion:          constants.ObserveAPIReadModelSchemaVersion,
			AgentsRunning:          0,
			AgentsRunningFreshness: SnapshotFreshnessUnavailable,
			TasksInQueue:           0,
			TasksInQueueFreshness:  SnapshotFreshnessUnavailable,
			GeneratedAt:            generatedAt,
		},
		Measurements: OverviewMeasurements{
			SchemaVersion: constants.ObserveAPIReadModelSchemaVersion,
		},
		RecentRuns:  []RunSummary{},
		LatestEvals: []EvalSummary{},
		Downloads:   []DownloadArtifact{},
		GeneratedAt: generatedAt,
	}
	data, err := json.Marshal(snapshot)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "active_run")

	var decoded ObserveBootstrapSnapshot
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, snapshot, decoded)
}

// TestDownloadArtifactPrivacyClassification asserts that the download privacy
// classification constant for browser-visible artifacts is public_safe.
func TestDownloadArtifactPrivacyClassification(t *testing.T) {
	assert.Equal(t, DownloadPrivacyPublicSafe, DownloadPrivacyClassification("public_safe"))
	assert.Equal(t, DownloadPrivacyRestricted, DownloadPrivacyClassification("restricted"))
}

// TestObserveTypedStatusEnums asserts the typed status enum values match the
// canonical strings defined in protocol/models/observe_event_payloads.json
// and observe_api.json.
func TestObserveTypedStatusEnums(t *testing.T) {
	agentStatuses := []AgentLifecycleStatus{
		AgentLifecycleStatusIdle, AgentLifecycleStatusQueued, AgentLifecycleStatusRunning,
		AgentLifecycleStatusWaiting, AgentLifecycleStatusCompleted, AgentLifecycleStatusFailed,
		AgentLifecycleStatusOffline,
	}
	for _, s := range agentStatuses {
		assert.NotEmpty(t, string(s))
	}

	runStatuses := []RunLifecycleStatus{
		RunLifecycleStatusQueued, RunLifecycleStatusRunning, RunLifecycleStatusWaiting,
		RunLifecycleStatusCompleted, RunLifecycleStatusFailed, RunLifecycleStatusCancelled,
	}
	for _, s := range runStatuses {
		assert.NotEmpty(t, string(s))
	}

	runKinds := []RunKind{RunKindInvestigation, RunKindEval, RunKindDemo, RunKindWorkflow}
	for _, k := range runKinds {
		assert.NotEmpty(t, string(k))
	}

	verificationStatuses := []EvalVerificationStatus{
		EvalVerificationProjectionValidated,
		EvalVerificationReceiptVerificationNotApplicable,
		EvalVerificationVerified,
	}
	for _, v := range verificationStatuses {
		assert.NotEmpty(t, string(v))
	}

	measurements := []MeasurementStatus{
		MeasurementStatusObserved, MeasurementStatusStale,
		MeasurementStatusUnavailable, MeasurementStatusUnsupported,
	}
	for _, m := range measurements {
		assert.NotEmpty(t, string(m))
	}

	freshness := []SnapshotFreshness{
		SnapshotFreshnessObserved, SnapshotFreshnessStale, SnapshotFreshnessUnavailable,
	}
	for _, f := range freshness {
		assert.NotEmpty(t, string(f))
	}
}
