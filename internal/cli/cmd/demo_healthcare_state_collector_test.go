// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func TestDecodeHealthcareStateObservation_BindsTypedObservationToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := healthcareStateExpectation{
		RunID:                  "healthcare-run-123",
		ScenarioID:             "healthcare-gold-card",
		RequestID:              "PA-2026-0043",
		Action:                 "gold-card",
		ResourceType:           "ClaimResponse",
		Subject:                "Dr. Priya Nair",
		Status:                 "AUTO_APPROVED",
		MeasuredValue:          96,
		ThresholdValue:         90,
		AutoApproved:           true,
		InitialStateFixtureRef: "fixture:healthcare-pa-pending-review@1.0.0",
		TerminalStateAssertions: []string{
			"PA-2026-0043 status is AUTO_APPROVED",
		},
		NotBefore: notBefore,
	}
	raw := []byte(`{"action":"gold-card","auto_approved":true,"evaluated_at":"2026-09-01T12:31:00Z","measured_value":96,"reportable_to_oha":false,"request_id":"PA-2026-0043","resource_type":"ClaimResponse","run_id":"healthcare-run-123","scenario_id":"healthcare-gold-card","status":"AUTO_APPROVED","subject":"Dr. Priya Nair","threshold_value":90}`)

	collection, err := decodeHealthcareStateObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, healthcareStateCollectorID, collection.CollectorID)
	assert.Equal(t, healthcareStateCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, healthcareActuatorBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.RunID, collection.Observation.RunID)
	assert.Equal(t, expected.ScenarioID, collection.Observation.ScenarioID)
	assert.Equal(t, collectedAt, collection.CollectedAt)

	encoded, evidenceRef, err := encodeHealthcareStateCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeHealthcareStateCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestBuildHealthcareMetricEvidence_DerivesTypedGradeFromCollectedObservation(t *testing.T) {
	collection := &healthcareStateCollection{
		CollectorID: healthcareStateCollectorID, CollectorVersion: healthcareStateCollectorVersion, Boundary: healthcareActuatorBoundary,
		InitialStateFixtureRef: "fixture:healthcare-pa-pending-review@1.0.0", TerminalStateAssertions: []string{"PA-2026-0043 status is AUTO_APPROVED"},
		CollectedAt: time.Date(2026, time.September, 1, 12, 32, 0, 0, time.UTC),
		Observation: healthcareStateObservation{
			Action: "gold-card", RequestID: "PA-2026-0043", ResourceType: "ClaimResponse", Subject: "Dr. Priya Nair",
			MeasuredValue: 96, ThresholdValue: 90, RunID: "healthcare-run-123", ScenarioID: "healthcare-gold-card",
			Status: "AUTO_APPROVED", AutoApproved: true, EvaluatedAt: "2026-09-01T12:31:00Z",
		},
	}
	body, observationRef, err := encodeHealthcareStateCollection(collection)
	require.NoError(t, err)
	result := &compliancev1.DemoScenarioResult{
		RunId: "healthcare-run-123", ScopeId: constants.DemoScopeHealthcare,
		ScenarioRef: &compliancev1.VersionedReference{Id: "healthcare-gold-card", Version: "1.0.0"},
	}

	metric, err := buildHealthcareMetricEvidence(result, observationRef, string(body))
	require.NoError(t, err)

	assert.Equal(t, "healthcare-provider-approval-rate", metric.GetMetricId())
	assert.Equal(t, int64(96), metric.GetMeasuredValue())
	assert.Equal(t, int64(90), metric.GetThresholdValue())
	assert.Equal(t, "percent", metric.GetUnit())
	assert.True(t, metric.GetPassed())
	assert.Equal(t, observationRef, metric.GetSourceEvidenceRef())
	assert.Equal(t, constants.DemoMetricGraderID, metric.GetGraderRef().GetId())
}

func TestBuildHealthcareMetricEvidence_FailsClosedOnInvalidSourceBinding(t *testing.T) {
	result := &compliancev1.DemoScenarioResult{
		RunId: "healthcare-run-123", ScopeId: constants.DemoScopeHealthcare,
		ScenarioRef: &compliancev1.VersionedReference{Id: "healthcare-gold-card", Version: "1.0.0"},
	}
	tests := []struct {
		name, sourceRef, body string
	}{
		{name: "malformed content address", sourceRef: "state-observation:invalid", body: `{}`},
		{name: "digest mismatch", sourceRef: "state-observation:sha256:" + strings.Repeat("0", 64), body: `{}`},
		{name: "malformed observation", sourceRef: "state-observation:sha256:" + computeSHA256Hex([]byte(`{`)), body: `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildHealthcareMetricEvidence(result, tt.sourceRef, tt.body)
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrInvalidEvidenceGraph)
		})
	}
}

func TestDecodeHealthcareStateObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := healthcareStateExpectation{
		RunID:                  "healthcare-run-123",
		ScenarioID:             "healthcare-gold-card",
		RequestID:              "PA-2026-0043",
		Action:                 "gold-card",
		ResourceType:           "ClaimResponse",
		Subject:                "Dr. Priya Nair",
		Status:                 "AUTO_APPROVED",
		MeasuredValue:          96,
		ThresholdValue:         90,
		AutoApproved:           true,
		InitialStateFixtureRef: "fixture:healthcare-pa-pending-review@1.0.0",
		TerminalStateAssertions: []string{
			"PA-2026-0043 status is AUTO_APPROVED",
		},
		NotBefore: notBefore,
	}
	valid := `{"action":"gold-card","auto_approved":true,"evaluated_at":"2026-09-01T12:31:00Z","measured_value":96,"reportable_to_oha":false,"request_id":"PA-2026-0043","resource_type":"ClaimResponse","run_id":"healthcare-run-123","scenario_id":"healthcare-gold-card","status":"AUTO_APPROVED","subject":"Dr. Priya Nair","threshold_value":90}`
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "wrong run", raw: replaceJSONValue(valid, "healthcare-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "healthcare-gold-card", "healthcare-sla-breach")},
		{name: "wrong request", raw: replaceJSONValue(valid, "PA-2026-0043", "PA-2026-9999")},
		{name: "wrong action", raw: replaceJSONValue(valid, "gold-card", "submit")},
		{name: "wrong status", raw: replaceJSONValue(valid, "AUTO_APPROVED", "PENDING_REVIEW")},
		{name: "wrong measured value", raw: replaceJSONValue(valid, `"measured_value":96`, `"measured_value":95`)},
		{name: "wrong threshold value", raw: replaceJSONValue(valid, `"threshold_value":90`, `"threshold_value":91`)},
		{name: "missing false boolean", raw: replaceJSONValue(valid, `"reportable_to_oha":false,`, "")},
		{name: "missing evaluated timestamp", raw: replaceJSONValue(valid, `"evaluated_at":"2026-09-01T12:31:00Z",`, "")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-01T12:31:00Z", "2026-09-01T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-01T12:31:00Z", "2026-09-01T12:32:01Z")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeHealthcareStateObservation([]byte(tt.raw), expected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}

func replaceJSONValue(value, oldValue, newValue string) string {
	return strings.Replace(value, oldValue, newValue, 1)
}
