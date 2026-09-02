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

// ── DHS local gateway health collector ──────────────────────────────────────

func TestDecodeDHSGatewayHealthObservation_BindsTypedAvailabilityToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsGatewayHealthExpectation{
		RunID:                   "dhs-run-123",
		ScenarioID:              "dhs-disconnected-operations",
		Endpoint:                "http://localhost:8087/api/v1/health",
		Available:               true,
		InitialStateFixtureRef:  "dhs-datalink-connected-local-stores-ready",
		TerminalStateAssertions: []string{"gateway remains operational while disconnected"},
		NotBefore:               notBefore,
	}
	raw := []byte(`{"available":true,"endpoint":"http://localhost:8087/api/v1/health","observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-123","scenario_id":"dhs-disconnected-operations"}`)

	collection, err := decodeDHSGatewayHealthObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, dhsGatewayHealthCollectorID, collection.CollectorID)
	assert.Equal(t, dhsGatewayHealthCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, dhsGatewayHealthBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.RunID, collection.Observation.RunID)
	assert.Equal(t, expected.ScenarioID, collection.Observation.ScenarioID)
	assert.True(t, collection.Observation.Available)
	assert.Equal(t, expected.Endpoint, collection.Observation.Endpoint)
	assert.Equal(t, collectedAt, collection.CollectedAt)

	encoded, evidenceRef, err := encodeDHSGatewayHealthCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeDHSGatewayHealthCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeDHSGatewayHealthObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsGatewayHealthExpectation{
		RunID:                   "dhs-run-123",
		ScenarioID:              "dhs-disconnected-operations",
		Endpoint:                "http://localhost:8087/api/v1/health",
		Available:               true,
		InitialStateFixtureRef:  "dhs-datalink-connected-local-stores-ready",
		TerminalStateAssertions: []string{"gateway remains operational while disconnected"},
		NotBefore:               notBefore,
	}
	valid := `{"available":true,"endpoint":"http://localhost:8087/api/v1/health","observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-123","scenario_id":"dhs-disconnected-operations"}`
	tests := []struct {
		name                  string
		raw                   string
		removeFixtureBinding  bool
		removeTerminalBinding bool
	}{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "missing available", raw: replaceJSONValue(valid, `"available":true,`, "")},
		{name: "gateway unavailable", raw: replaceJSONValue(valid, `"available":true`, `"available":false`)},
		{name: "wrong endpoint", raw: replaceJSONValue(valid, "http://localhost:8087/api/v1/health", "http://localhost:9999/api/v1/health")},
		{name: "wrong run", raw: replaceJSONValue(valid, "dhs-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "dhs-disconnected-operations", "other-scenario")},
		{name: "missing observed timestamp", raw: replaceJSONValue(valid, `"observed_at":"2026-09-02T12:31:00Z",`, "")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:32:01Z")},
		{name: "missing fixture binding", raw: valid, removeFixtureBinding: true},
		{name: "missing terminal binding", raw: valid, removeTerminalBinding: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testExpected := expected
			if tt.removeFixtureBinding {
				testExpected.InitialStateFixtureRef = ""
			}
			if tt.removeTerminalBinding {
				testExpected.TerminalStateAssertions = nil
			}
			_, err := decodeDHSGatewayHealthObservation([]byte(tt.raw), testExpected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}

// ── DHS local ledger persistence collector ──────────────────────────────────

func TestDecodeDHSLedgerPersistenceObservation_BindsTypedPersistenceToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsLedgerPersistenceExpectation{
		RunID:                   "dhs-run-123",
		ScenarioID:              "dhs-disconnected-operations",
		Container:               "operator",
		Directory:               constants.ContainerLedgerFilesDir,
		Persisted:               true,
		InitialStateFixtureRef:  "dhs-datalink-connected-local-stores-ready",
		TerminalStateAssertions: []string{"local ledger remains non-empty"},
		NotBefore:               notBefore,
	}
	raw := []byte(`{"persisted":true,"directory":"` + constants.ContainerLedgerFilesDir + `","entry_count":5,"observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-123","scenario_id":"dhs-disconnected-operations"}`)

	collection, err := decodeDHSLedgerPersistenceObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, dhsLedgerPersistenceCollectorID, collection.CollectorID)
	assert.Equal(t, dhsLedgerPersistenceCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, dhsLedgerPersistenceBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.RunID, collection.Observation.RunID)
	assert.Equal(t, expected.ScenarioID, collection.Observation.ScenarioID)
	assert.True(t, collection.Observation.Persisted)
	assert.Equal(t, expected.Directory, collection.Observation.Directory)
	assert.Equal(t, 5, collection.Observation.EntryCount)
	assert.Equal(t, collectedAt, collection.CollectedAt)

	encoded, evidenceRef, err := encodeDHSLedgerPersistenceCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeDHSLedgerPersistenceCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeDHSLedgerPersistenceObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsLedgerPersistenceExpectation{
		RunID:                   "dhs-run-123",
		ScenarioID:              "dhs-disconnected-operations",
		Container:               "operator",
		Directory:               constants.ContainerLedgerFilesDir,
		Persisted:               true,
		InitialStateFixtureRef:  "dhs-datalink-connected-local-stores-ready",
		TerminalStateAssertions: []string{"local ledger remains non-empty"},
		NotBefore:               notBefore,
	}
	valid := `{"persisted":true,"directory":"` + constants.ContainerLedgerFilesDir + `","entry_count":5,"observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-123","scenario_id":"dhs-disconnected-operations"}`
	tests := []struct {
		name                  string
		raw                   string
		removeFixtureBinding  bool
		removeTerminalBinding bool
	}{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "missing persisted", raw: replaceJSONValue(valid, `"persisted":true,`, "")},
		{name: "ledger not persisted", raw: replaceJSONValue(valid, `"persisted":true`, `"persisted":false`)},
		{name: "missing entry count", raw: replaceJSONValue(valid, `"entry_count":5,`, "")},
		{name: "zero entry count", raw: replaceJSONValue(valid, `"entry_count":5`, `"entry_count":0`)},
		{name: "wrong directory", raw: replaceJSONValue(valid, constants.ContainerLedgerFilesDir, "/other/dir")},
		{name: "wrong run", raw: replaceJSONValue(valid, "dhs-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "dhs-disconnected-operations", "other-scenario")},
		{name: "missing observed timestamp", raw: replaceJSONValue(valid, `"observed_at":"2026-09-02T12:31:00Z",`, "")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:32:01Z")},
		{name: "missing fixture binding", raw: valid, removeFixtureBinding: true},
		{name: "missing terminal binding", raw: valid, removeTerminalBinding: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testExpected := expected
			if tt.removeFixtureBinding {
				testExpected.InitialStateFixtureRef = ""
			}
			if tt.removeTerminalBinding {
				testExpected.TerminalStateAssertions = nil
			}
			_, err := decodeDHSLedgerPersistenceObservation([]byte(tt.raw), testExpected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}

// ── DHS local audit vault persistence collector ─────────────────────────────

func TestDecodeDHSAuditVaultPersistenceObservation_BindsTypedPersistenceToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsAuditVaultPersistenceExpectation{
		RunID:                   "dhs-run-123",
		ScenarioID:              "dhs-disconnected-operations",
		Container:               "operator",
		DatabasePath:            constants.ContainerAuditVaultDB,
		Persisted:               true,
		InitialStateFixtureRef:  "dhs-datalink-connected-local-stores-ready",
		TerminalStateAssertions: []string{"local audit vault remains non-empty"},
		NotBefore:               notBefore,
	}
	raw := []byte(`{"persisted":true,"database_path":"` + constants.ContainerAuditVaultDB + `","size_bytes":4096,"observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-123","scenario_id":"dhs-disconnected-operations"}`)

	collection, err := decodeDHSAuditVaultPersistenceObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, dhsAuditVaultPersistenceCollectorID, collection.CollectorID)
	assert.Equal(t, dhsAuditVaultPersistenceCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, dhsAuditVaultPersistenceBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.RunID, collection.Observation.RunID)
	assert.Equal(t, expected.ScenarioID, collection.Observation.ScenarioID)
	assert.True(t, collection.Observation.Persisted)
	assert.Equal(t, expected.DatabasePath, collection.Observation.DatabasePath)
	assert.Equal(t, int64(4096), collection.Observation.SizeBytes)
	assert.Equal(t, collectedAt, collection.CollectedAt)

	encoded, evidenceRef, err := encodeDHSAuditVaultPersistenceCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeDHSAuditVaultPersistenceCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeDHSAuditVaultPersistenceObservation_BindsToDestructionBlockFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsAuditVaultPersistenceExpectation{
		RunID:                   "dhs-run-456",
		ScenarioID:              "dhs-destruction-block",
		Container:               "operator",
		DatabasePath:            constants.ContainerAuditVaultDB,
		Persisted:               true,
		InitialStateFixtureRef:  "dhs-audit-vault-present-record-vpr-0001",
		TerminalStateAssertions: []string{"audit vault remains present after the blocked wipe"},
		NotBefore:               notBefore,
	}
	raw := []byte(`{"persisted":true,"database_path":"` + constants.ContainerAuditVaultDB + `","size_bytes":8192,"observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-456","scenario_id":"dhs-destruction-block"}`)

	collection, err := decodeDHSAuditVaultPersistenceObservation(raw, expected, collectedAt)
	require.NoError(t, err)

	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, "dhs-destruction-block", collection.Observation.ScenarioID)
	assert.True(t, collection.Observation.Persisted)
	assert.Equal(t, int64(8192), collection.Observation.SizeBytes)
}

func TestDecodeDHSAuditVaultPersistenceObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := dhsAuditVaultPersistenceExpectation{
		RunID:                   "dhs-run-123",
		ScenarioID:              "dhs-disconnected-operations",
		Container:               "operator",
		DatabasePath:            constants.ContainerAuditVaultDB,
		Persisted:               true,
		InitialStateFixtureRef:  "dhs-datalink-connected-local-stores-ready",
		TerminalStateAssertions: []string{"local audit vault remains non-empty"},
		NotBefore:               notBefore,
	}
	valid := `{"persisted":true,"database_path":"` + constants.ContainerAuditVaultDB + `","size_bytes":4096,"observed_at":"2026-09-02T12:31:00Z","run_id":"dhs-run-123","scenario_id":"dhs-disconnected-operations"}`
	tests := []struct {
		name                  string
		raw                   string
		removeFixtureBinding  bool
		removeTerminalBinding bool
	}{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "missing persisted", raw: replaceJSONValue(valid, `"persisted":true,`, "")},
		{name: "audit vault not persisted", raw: replaceJSONValue(valid, `"persisted":true`, `"persisted":false`)},
		{name: "missing size bytes", raw: replaceJSONValue(valid, `"size_bytes":4096,`, "")},
		{name: "zero size bytes", raw: replaceJSONValue(valid, `"size_bytes":4096`, `"size_bytes":0`)},
		{name: "wrong database path", raw: replaceJSONValue(valid, constants.ContainerAuditVaultDB, "/other/db")},
		{name: "wrong run", raw: replaceJSONValue(valid, "dhs-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "dhs-disconnected-operations", "other-scenario")},
		{name: "missing observed timestamp", raw: replaceJSONValue(valid, `"observed_at":"2026-09-02T12:31:00Z",`, "")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:32:01Z")},
		{name: "missing fixture binding", raw: valid, removeFixtureBinding: true},
		{name: "missing terminal binding", raw: valid, removeTerminalBinding: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testExpected := expected
			if tt.removeFixtureBinding {
				testExpected.InitialStateFixtureRef = ""
			}
			if tt.removeTerminalBinding {
				testExpected.TerminalStateAssertions = nil
			}
			_, err := decodeDHSAuditVaultPersistenceObservation([]byte(tt.raw), testExpected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}
