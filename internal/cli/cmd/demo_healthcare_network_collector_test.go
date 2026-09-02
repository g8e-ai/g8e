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

func TestDecodeHealthcareNetworkObservation_BindsTypedIsolationToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := healthcareNetworkExpectation{
		RunID:                  "healthcare-run-123",
		ScenarioID:             "healthcare-phi-blocked",
		SourceBoundary:         healthcareUntrustedBoundary,
		TargetBoundary:         healthcareInternalBoundary,
		TargetEndpoint:         healthcareInternalGatewayEndpoint,
		Reachable:              false,
		InitialStateFixtureRef: "healthcare-network-segmented",
		TerminalStateAssertions: []string{
			"net_untrusted cannot route to net_internal or net_secure",
		},
		NotBefore: notBefore,
	}
	raw := []byte(fmt.Sprintf(`{"observed_at":"2026-09-02T12:31:00Z","reachable":false,"run_id":"healthcare-run-123","scenario_id":"healthcare-phi-blocked","source_boundary":"%s","target_boundary":"%s","target_endpoint":"%s"}`, healthcareUntrustedBoundary, healthcareInternalBoundary, healthcareInternalGatewayEndpoint))

	collection, err := decodeHealthcareNetworkObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, healthcareNetworkCollectorID, collection.CollectorID)
	assert.Equal(t, healthcareNetworkCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, healthcareNetworkBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.RunID, collection.Observation.RunID)
	assert.Equal(t, expected.ScenarioID, collection.Observation.ScenarioID)
	assert.False(t, collection.Observation.Reachable)
	assert.Equal(t, collectedAt, collection.CollectedAt)

	encoded, evidenceRef, err := encodeHealthcareNetworkCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeHealthcareNetworkCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeHealthcareNetworkObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := healthcareNetworkExpectation{
		RunID:                  "healthcare-run-123",
		ScenarioID:             "healthcare-phi-blocked",
		SourceBoundary:         healthcareUntrustedBoundary,
		TargetBoundary:         healthcareInternalBoundary,
		TargetEndpoint:         healthcareInternalGatewayEndpoint,
		Reachable:              false,
		InitialStateFixtureRef: "healthcare-network-segmented",
		TerminalStateAssertions: []string{
			"net_untrusted cannot route to net_internal or net_secure",
		},
		NotBefore: notBefore,
	}
	valid := fmt.Sprintf(`{"observed_at":"2026-09-02T12:31:00Z","reachable":false,"run_id":"healthcare-run-123","scenario_id":"healthcare-phi-blocked","source_boundary":"%s","target_boundary":"%s","target_endpoint":"%s"}`, healthcareUntrustedBoundary, healthcareInternalBoundary, healthcareInternalGatewayEndpoint)
	tests := []struct {
		name              string
		raw               string
		removeFixtureBind bool
	}{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "missing reachability", raw: replaceJSONValue(valid, `"reachable":false,`, "")},
		{name: "target is reachable", raw: replaceJSONValue(valid, `"reachable":false`, `"reachable":true`)},
		{name: "wrong run", raw: replaceJSONValue(valid, "healthcare-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "healthcare-phi-blocked", "other-scenario")},
		{name: "wrong source boundary", raw: replaceJSONValue(valid, healthcareUntrustedBoundary, healthcareInternalBoundary)},
		{name: "wrong target boundary", raw: replaceJSONValue(valid, healthcareInternalBoundary, healthcareUntrustedBoundary)},
		{name: "wrong target endpoint", raw: replaceJSONValue(valid, healthcareInternalGatewayEndpoint, constants.GatewayHTTPBase)},
		{name: "missing observed timestamp", raw: replaceJSONValue(valid, `"observed_at":"2026-09-02T12:31:00Z",`, "")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:32:01Z")},
		{name: "missing fixture binding", raw: valid, removeFixtureBind: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testExpected := expected
			if tt.removeFixtureBind {
				testExpected.InitialStateFixtureRef = ""
			}
			_, err := decodeHealthcareNetworkObservation([]byte(tt.raw), testExpected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}
