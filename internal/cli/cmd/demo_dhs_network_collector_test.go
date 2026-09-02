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

func TestDecodeDHSNetworkObservation_BindsTypedMembershipToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsNetworkExpectation{
		RunID:                  "dhs-run-123",
		ScenarioID:             "dhs-disconnected-operations",
		NetworkName:            constants.DemosDHSPerimeterNetwork,
		ContainerName:          constants.DemosDHSCoalitionDatalinkCtnr,
		Connected:              false,
		InitialStateFixtureRef: "dhs-datalink-connected-local-governance-ready",
		TerminalStateAssertions: []string{
			"mission partner datalink is detached while local governance remains available",
		},
		NotBefore: notBefore,
	}
	raw := []byte(`{"connected":false,"container_name":"dhs-coalition-datalink","network_name":"dhs-demo_net_perimeter","observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-123","scenario_id":"dhs-disconnected-operations"}`)

	collection, err := decodeDHSNetworkObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, dhsNetworkCollectorID, collection.CollectorID)
	assert.Equal(t, dhsNetworkCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, dhsNetworkBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.RunID, collection.Observation.RunID)
	assert.Equal(t, expected.ScenarioID, collection.Observation.ScenarioID)
	assert.False(t, collection.Observation.Connected)
	assert.Equal(t, collectedAt, collection.CollectedAt)

	encoded, evidenceRef, err := encodeDHSNetworkCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeDHSNetworkCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeDHSNetworkObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsNetworkExpectation{
		RunID:                  "dhs-run-123",
		ScenarioID:             "dhs-disconnected-operations",
		NetworkName:            constants.DemosDHSPerimeterNetwork,
		ContainerName:          constants.DemosDHSCoalitionDatalinkCtnr,
		Connected:              false,
		InitialStateFixtureRef: "dhs-datalink-connected-local-governance-ready",
		TerminalStateAssertions: []string{
			"mission partner datalink is detached while local governance remains available",
		},
		NotBefore: notBefore,
	}
	valid := `{"connected":false,"container_name":"dhs-coalition-datalink","network_name":"dhs-demo_net_perimeter","observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-123","scenario_id":"dhs-disconnected-operations"}`
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
		{name: "missing membership", raw: replaceJSONValue(valid, `"connected":false,`, "")},
		{name: "container remains connected", raw: replaceJSONValue(valid, `"connected":false`, `"connected":true`)},
		{name: "wrong run", raw: replaceJSONValue(valid, "dhs-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "dhs-disconnected-operations", "other-scenario")},
		{name: "wrong network", raw: replaceJSONValue(valid, constants.DemosDHSPerimeterNetwork, "other-network")},
		{name: "wrong container", raw: replaceJSONValue(valid, constants.DemosDHSCoalitionDatalinkCtnr, "other-container")},
		{name: "missing observed timestamp", raw: replaceJSONValue(valid, `"observed_at":"2026-09-02T12:31:00Z",`, "")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:32:01Z")},
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
			_, err := decodeDHSNetworkObservation([]byte(tt.raw), testExpected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}
