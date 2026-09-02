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

func TestDecodeFedRAMPCloudOperationObservation_BindsExactOperationToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := fedRAMPCloudOperationExpectation{
		RunID: "fedramp-run-123", ScenarioID: "fedramp-provision", Action: "PROVISION",
		ResourceID: "fedramp-vm-prod-01", Detail: "FIPS-199-MODERATE", OperationFound: true,
		InitialStateFixtureRef: "fedramp-cloud-resource-unprovisioned", TerminalStateAssertions: []string{"resource is provisioned"},
		NotBefore: notBefore,
	}
	raw := []byte(`{"action":"PROVISION","detail":"FIPS-199-MODERATE","observed_at":"2026-09-02T12:31:00Z","operation_found":true,"operation_timestamp":"2026-09-02T12:30:30Z","resource_id":"fedramp-vm-prod-01","run_id":"fedramp-run-123","scenario_id":"fedramp-provision"}`)

	collection, err := decodeFedRAMPCloudOperationObservation(raw, expected, collectedAt)
	require.NoError(t, err)
	assert.Equal(t, fedRAMPCloudOperationCollectorID, collection.CollectorID)
	assert.Equal(t, fedRAMPCloudOperationCollectorVersion, collection.CollectorVersion)
	assert.Equal(t, fedRAMPCloudServiceBoundary, collection.Boundary)
	assert.Equal(t, expected.InitialStateFixtureRef, collection.InitialStateFixtureRef)
	assert.Equal(t, expected.TerminalStateAssertions, collection.TerminalStateAssertions)
	assert.Equal(t, expected.ResourceID, collection.Observation.ResourceID)
	assert.True(t, collection.Observation.OperationFound)

	encoded, evidenceRef, err := encodeFedRAMPCloudOperationCollection(collection)
	require.NoError(t, err)
	encodedAgain, evidenceRefAgain, err := encodeFedRAMPCloudOperationCollection(collection)
	require.NoError(t, err)
	assert.Equal(t, encoded, encodedAgain)
	assert.Equal(t, evidenceRef, evidenceRefAgain)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeFedRAMPCloudOperationObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := fedRAMPCloudOperationExpectation{
		RunID: "fedramp-run-123", ScenarioID: "fedramp-provision", Action: "PROVISION",
		ResourceID: "fedramp-vm-prod-01", Detail: "FIPS-199-MODERATE", OperationFound: true,
		InitialStateFixtureRef: "fedramp-cloud-resource-unprovisioned", TerminalStateAssertions: []string{"resource is provisioned"},
		NotBefore: notBefore,
	}
	valid := `{"action":"PROVISION","detail":"FIPS-199-MODERATE","observed_at":"2026-09-02T12:31:00Z","operation_found":true,"operation_timestamp":"2026-09-02T12:30:30Z","resource_id":"fedramp-vm-prod-01","run_id":"fedramp-run-123","scenario_id":"fedramp-provision"}`
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
		{name: "missing operation presence", raw: replaceJSONValue(valid, `"operation_found":true,`, "")},
		{name: "operation absent", raw: replaceJSONValue(valid, `"operation_found":true`, `"operation_found":false`)},
		{name: "wrong action", raw: replaceJSONValue(valid, "PROVISION", "REVERT")},
		{name: "wrong resource", raw: replaceJSONValue(valid, "fedramp-vm-prod-01", "other-resource")},
		{name: "wrong detail", raw: replaceJSONValue(valid, "FIPS-199-MODERATE", "other-detail")},
		{name: "wrong run", raw: replaceJSONValue(valid, "fedramp-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "fedramp-provision", "other-scenario")},
		{name: "stale operation", raw: replaceJSONValue(valid, "2026-09-02T12:30:30Z", "2026-09-02T12:29:59Z")},
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
			_, err := decodeFedRAMPCloudOperationObservation([]byte(tt.raw), testExpected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}

func TestDecodeFedRAMPCloudLogObservation_BindsTypedPersistenceToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := fedRAMPCloudLogExpectation{
		RunID: "fedramp-run-123", ScenarioID: "fedramp-deny", LogPath: constants.ContainerCloudSvcOpsLog, Persisted: true,
		InitialStateFixtureRef: "fedramp-cloud-operations-log-present", TerminalStateAssertions: []string{"operations log remains present and non-empty"},
		NotBefore: notBefore,
	}
	raw := []byte(`{"entry_count":4,"log_path":"` + constants.ContainerCloudSvcOpsLog + `","observed_at":"2026-09-02T12:31:00Z","persisted":true,"run_id":"fedramp-run-123","scenario_id":"fedramp-deny","size_bytes":4096}`)

	collection, err := decodeFedRAMPCloudLogObservation(raw, expected, collectedAt)
	require.NoError(t, err)
	assert.Equal(t, fedRAMPCloudLogCollectorID, collection.CollectorID)
	assert.Equal(t, fedRAMPCloudServiceBoundary, collection.Boundary)
	assert.Equal(t, 4, collection.Observation.EntryCount)
	assert.Equal(t, int64(4096), collection.Observation.SizeBytes)

	encoded, evidenceRef, err := encodeFedRAMPCloudLogCollection(collection)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeFedRAMPCloudLogObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := fedRAMPCloudLogExpectation{
		RunID: "fedramp-run-123", ScenarioID: "fedramp-deny", LogPath: constants.ContainerCloudSvcOpsLog, Persisted: true,
		InitialStateFixtureRef: "fedramp-cloud-operations-log-present", TerminalStateAssertions: []string{"operations log remains present and non-empty"},
		NotBefore: notBefore,
	}
	valid := `{"entry_count":4,"log_path":"` + constants.ContainerCloudSvcOpsLog + `","observed_at":"2026-09-02T12:31:00Z","persisted":true,"run_id":"fedramp-run-123","scenario_id":"fedramp-deny","size_bytes":4096}`
	tests := []struct{ name, raw string }{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "missing persistence", raw: replaceJSONValue(valid, `"persisted":true,`, "")},
		{name: "log absent", raw: replaceJSONValue(valid, `"persisted":true`, `"persisted":false`)},
		{name: "missing entry count", raw: replaceJSONValue(valid, `"entry_count":4,`, "")},
		{name: "empty log", raw: replaceJSONValue(valid, `"entry_count":4`, `"entry_count":0`)},
		{name: "zero size", raw: replaceJSONValue(valid, `"size_bytes":4096`, `"size_bytes":0`)},
		{name: "wrong path", raw: replaceJSONValue(valid, constants.ContainerCloudSvcOpsLog, "/other/log")},
		{name: "wrong run", raw: replaceJSONValue(valid, "fedramp-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "fedramp-deny", "other-scenario")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:32:01Z")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeFedRAMPCloudLogObservation([]byte(tt.raw), expected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}

func TestDecodeFedRAMPAuditVaultObservation_BindsTypedPersistenceToCanonicalFixture(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := fedRAMPAuditVaultExpectation{
		RunID: "fedramp-run-123", ScenarioID: "fedramp-evidence-block", DatabasePath: constants.ContainerAuditVaultDB, Persisted: true,
		InitialStateFixtureRef: "fedramp-gateway-audit-vault-present", TerminalStateAssertions: []string{"audit vault remains present and non-empty"},
		NotBefore: notBefore,
	}
	raw := []byte(`{"database_path":"` + constants.ContainerAuditVaultDB + `","observed_at":"2026-09-02T12:31:00Z","persisted":true,"run_id":"fedramp-run-123","scenario_id":"fedramp-evidence-block","size_bytes":8192}`)

	collection, err := decodeFedRAMPAuditVaultObservation(raw, expected, collectedAt)
	require.NoError(t, err)
	assert.Equal(t, fedRAMPAuditVaultCollectorID, collection.CollectorID)
	assert.Equal(t, fedRAMPGatewayAuditVaultBoundary, collection.Boundary)
	assert.Equal(t, int64(8192), collection.Observation.SizeBytes)

	encoded, evidenceRef, err := encodeFedRAMPAuditVaultCollection(collection)
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)
	assert.Regexp(t, `^state-observation:sha256:[0-9a-f]{64}$`, evidenceRef)
}

func TestDecodeFedRAMPAuditVaultObservation_FailsClosedOnMalformedMismatchedOrStaleEvidence(t *testing.T) {
	notBefore := time.Date(2026, time.September, 2, 12, 30, 0, 0, time.UTC)
	collectedAt := notBefore.Add(2 * time.Minute)
	expected := fedRAMPAuditVaultExpectation{
		RunID: "fedramp-run-123", ScenarioID: "fedramp-evidence-block", DatabasePath: constants.ContainerAuditVaultDB, Persisted: true,
		InitialStateFixtureRef: "fedramp-gateway-audit-vault-present", TerminalStateAssertions: []string{"audit vault remains present and non-empty"},
		NotBefore: notBefore,
	}
	valid := `{"database_path":"` + constants.ContainerAuditVaultDB + `","observed_at":"2026-09-02T12:31:00Z","persisted":true,"run_id":"fedramp-run-123","scenario_id":"fedramp-evidence-block","size_bytes":8192}`
	tests := []struct{ name, raw string }{
		{name: "empty output"},
		{name: "malformed JSON", raw: `{`},
		{name: "unknown field", raw: valid[:len(valid)-1] + `,"unexpected":true}`},
		{name: "trailing JSON", raw: valid + `{}`},
		{name: "missing persistence", raw: replaceJSONValue(valid, `"persisted":true,`, "")},
		{name: "vault absent", raw: replaceJSONValue(valid, `"persisted":true`, `"persisted":false`)},
		{name: "missing size", raw: replaceJSONValue(valid, `"size_bytes":8192`, `"size_bytes":null`)},
		{name: "zero size", raw: replaceJSONValue(valid, `"size_bytes":8192`, `"size_bytes":0`)},
		{name: "wrong path", raw: replaceJSONValue(valid, constants.ContainerAuditVaultDB, "/other/db")},
		{name: "wrong run", raw: replaceJSONValue(valid, "fedramp-run-123", "other-run")},
		{name: "wrong scenario", raw: replaceJSONValue(valid, "fedramp-evidence-block", "other-scenario")},
		{name: "stale observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:29:59Z")},
		{name: "future observation", raw: replaceJSONValue(valid, "2026-09-02T12:31:00Z", "2026-09-02T12:32:01Z")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeFedRAMPAuditVaultObservation([]byte(tt.raw), expected, collectedAt)
			require.Error(t, err)
			assert.True(t, errors.Is(err, constants.ErrInvalidEvidenceGraph))
		})
	}
}
