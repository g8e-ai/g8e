// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestDecodeFinanceStateObservation_BindsTypedAbsenceToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := financeStateExpectation{
		RunID:                  "finance-run-123",
		ScenarioID:             "finance-unauthorized-trade",
		ArtifactPath:           constants.ContainerFinanceUnauthorizedTrade,
		ArtifactExists:         false,
		InitialStateFixtureRef: "finance-trading-ledger-without-unauthorized-trade",
		TerminalStateAssertions: []string{
			"unauthorized trade execution is blocked before reaching the target system",
		},
		NotBefore: notBefore,
	}
	raw := []byte(fmt.Sprintf(`{"artifact_exists":false,"artifact_path":"%s","observed_at":"2026-09-01T12:31:00Z","run_id":"finance-run-123","scenario_id":"finance-unauthorized-trade"}`, constants.ContainerFinanceUnauthorizedTrade))

	collection, err := decodeFinanceStateObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, financeStateCollectorID, collection.CollectorID)
	assert.Equal(t, financeStateCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, financeTargetBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.RunID, collection.Observation.RunID)
	assert.Equal(t, expected.ScenarioID, collection.Observation.ScenarioID)
	assert.False(t, collection.Observation.ArtifactExists)
	assert.Equal(t, collectedAt, collection.CollectedAt)

	encoded, evidenceRef, err := encodeFinanceStateCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeFinanceStateCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeFinanceStateObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 1, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := financeStateExpectation{
		RunID:                  "finance-run-123",
		ScenarioID:             "finance-unauthorized-trade",
		ArtifactPath:           constants.ContainerFinanceUnauthorizedTrade,
		ArtifactExists:         false,
		InitialStateFixtureRef: "finance-trading-ledger-without-unauthorized-trade",
		TerminalStateAssertions: []string{
			"unauthorized trade execution is blocked before reaching the target system",
		},
		NotBefore: notBefore,
	}
	valid := fmt.Sprintf(`{"artifact_exists":false,"artifact_path":"%s","observed_at":"2026-09-01T12:31:00Z","run_id":"finance-run-123","scenario_id":"finance-unauthorized-trade"}`, constants.ContainerFinanceUnauthorizedTrade)
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "missing artifact existence", raw: replaceJSONValue(valid, `"artifact_exists":false,`, "")},
		{name: "wrong artifact existence", raw: replaceJSONValue(valid, `"artifact_exists":false`, `"artifact_exists":true`)},
		{name: "wrong artifact path", raw: replaceJSONValue(valid, constants.ContainerFinanceUnauthorizedTrade, constants.ContainerCloudSvcOpsLog)},
		{name: "wrong run", raw: replaceJSONValue(valid, "finance-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "finance-unauthorized-trade", "other-scenario")},
		{name: "missing observed timestamp", raw: replaceJSONValue(valid, `"observed_at":"2026-09-01T12:31:00Z",`, "")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-01T12:31:00Z", "2026-09-01T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-01T12:31:00Z", "2026-09-01T12:32:01Z")},
		{name: "missing fixture binding", raw: valid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testExpected := expected
			if tt.name == "missing fixture binding" {
				testExpected.InitialStateFixtureRef = ""
			}
			_, err := decodeFinanceStateObservation([]byte(tt.raw), testExpected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}
