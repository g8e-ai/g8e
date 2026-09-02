// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestDecodeDHSDataServiceObservation_BindsTypedOperationToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsDataServiceExpectation{
		RunID:                  "dhs-run-123",
		ScenarioID:             "dhs-ingest",
		Action:                 "INGEST",
		RecordID:               "TRK-CBP-0001",
		Detail:                 "NIPR",
		OperationFound:         true,
		InitialStateFixtureRef: "dhs-sovereign-data-service-ready",
		TerminalStateAssertions: []string{
			"governed multi-source ingest is recorded in the sovereign data service",
		},
		NotBefore: notBefore,
	}
	raw := []byte(`{"action":"INGEST","detail":"NIPR","observed_at":"2026-09-02T12:31:30Z","operation_found":true,"operation_timestamp":"2026-09-02T12:31:00Z","record_id":"TRK-CBP-0001","run_id":"dhs-run-123","scenario_id":"dhs-ingest"}`)

	collection, err := decodeDHSDataServiceObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, dhsDataServiceCollectorID, collection.CollectorID)
	assert.Equal(t, dhsDataServiceCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, dhsDataServiceBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.RunID, collection.Observation.RunID)
	assert.Equal(t, expected.ScenarioID, collection.Observation.ScenarioID)
	assert.True(t, collection.Observation.OperationFound)
	assert.Equal(t, collectedAt, collection.CollectedAt)

	encoded, evidenceRef, err := encodeDHSDataServiceCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeDHSDataServiceCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeDHSDataServiceObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsDataServiceExpectation{
		RunID:                  "dhs-run-123",
		ScenarioID:             "dhs-ingest",
		Action:                 "INGEST",
		RecordID:               "TRK-CBP-0001",
		Detail:                 "NIPR",
		OperationFound:         true,
		InitialStateFixtureRef: "dhs-sovereign-data-service-ready",
		TerminalStateAssertions: []string{
			"governed multi-source ingest is recorded in the sovereign data service",
		},
		NotBefore: notBefore,
	}
	valid := `{"action":"INGEST","detail":"NIPR","observed_at":"2026-09-02T12:31:30Z","operation_found":true,"operation_timestamp":"2026-09-02T12:31:00Z","record_id":"TRK-CBP-0001","run_id":"dhs-run-123","scenario_id":"dhs-ingest"}`
	tests := []struct {
		name                  string
		raw                   string
		removeFixtureBind     bool
		removeTerminalBinding bool
	}{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "missing operation presence", raw: replaceJSONValue(valid, `"operation_found":true,`, "")},
		{name: "operation not found", raw: replaceJSONValue(valid, `"operation_found":true`, `"operation_found":false`)},
		{name: "wrong action", raw: replaceJSONValue(valid, "INGEST", "CUE")},
		{name: "wrong record", raw: replaceJSONValue(valid, "TRK-CBP-0001", "VPR-0001")},
		{name: "wrong detail", raw: replaceJSONValue(valid, "NIPR", "RETENTION-30D")},
		{name: "wrong run", raw: replaceJSONValue(valid, "dhs-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "dhs-ingest", "other-scenario")},
		{name: "missing observed timestamp", raw: replaceJSONValue(valid, `"observed_at":"2026-09-02T12:31:30Z",`, "")},
		{name: "stale collection", raw: replaceJSONValue(valid, "2026-09-02T12:31:30Z", "2026-09-02T12:29:59Z")},
		{name: "future collection", raw: replaceJSONValue(valid, "2026-09-02T12:31:30Z", "2026-09-02T12:32:01Z")},
		{name: "missing operation timestamp", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "")},
		{name: "stale operation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:29:59Z")},
		{name: "future operation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:32:01Z")},
		{name: "missing fixture binding", raw: valid, removeFixtureBind: true},
		{name: "missing terminal binding", raw: valid, removeTerminalBinding: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testExpected := expected
			if tt.removeFixtureBind {
				testExpected.InitialStateFixtureRef = ""
			}
			if tt.removeTerminalBinding {
				testExpected.TerminalStateAssertions = nil
			}
			_, err := decodeDHSDataServiceObservation([]byte(tt.raw), testExpected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}
