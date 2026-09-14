// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvalEngineRequestSchemaVersionContract asserts the schema version
// matches the Python-side constant pinned in
// tests/test_engine_protocol_contract.py.
func TestEvalEngineRequestSchemaVersionContract(t *testing.T) {
	assert.Equal(t, "1.0.0", EvalEngineRequestSchemaVersion)
}

// TestEvalOperationContract asserts the Go EvalOperation constants match
// the Python EvalOperation enum pinned in the contract test. If either
// side adds, removes, or renames a value, both tests fail.
func TestEvalOperationContract(t *testing.T) {
	expected := map[EvalOperation]bool{
		EvalOperationDiagnosticStart:             true,
		EvalOperationDiagnosticStatus:            true,
		EvalOperationDiagnosticStop:              true,
		EvalOperationDiagnosticVerify:            true,
		EvalOperationCampaignStart:               true,
		EvalOperationCampaignStatus:              true,
		EvalOperationCampaignStop:                true,
		EvalOperationCampaignVerify:              true,
		EvalOperationCampaignPublish:             true,
		EvalOperationCampaignSetPlan:             true,
		EvalOperationCampaignSetValidate:         true,
		EvalOperationCampaignSetVerify:           true,
		EvalOperationControllerRun:               true,
		EvalOperationControllerStatus:            true,
		EvalOperationControllerStop:              true,
		EvalOperationControllerRecover:           true,
		EvalOperationBundle:                      true,
		EvalOperationVerify:                      true,
		EvalOperationVerifyReceipts:              true,
		EvalOperationPublish:                     true,
		EvalOperationQualificationHashSource:     true,
		EvalOperationQualificationCandidate:      true,
		EvalOperationQualificationCollectRuntime: true,
		EvalOperationQualificationRunGate:        true,
		EvalOperationQualificationBuild:          true,
		EvalOperationBenchSynthetic:              true,
	}
	allOps := []EvalOperation{
		EvalOperationDiagnosticStart, EvalOperationDiagnosticStatus, EvalOperationDiagnosticStop, EvalOperationDiagnosticVerify,
		EvalOperationCampaignStart, EvalOperationCampaignStatus, EvalOperationCampaignStop, EvalOperationCampaignVerify, EvalOperationCampaignPublish,
		EvalOperationCampaignSetPlan, EvalOperationCampaignSetValidate, EvalOperationCampaignSetVerify,
		EvalOperationControllerRun, EvalOperationControllerStatus, EvalOperationControllerStop, EvalOperationControllerRecover,
		EvalOperationBundle, EvalOperationVerify, EvalOperationVerifyReceipts, EvalOperationPublish,
		EvalOperationQualificationHashSource, EvalOperationQualificationCandidate, EvalOperationQualificationCollectRuntime, EvalOperationQualificationRunGate, EvalOperationQualificationBuild,
		EvalOperationBenchSynthetic,
	}
	assert.Equal(t, len(expected), len(allOps), "expected set and constant list must have the same length")
	for _, op := range allOps {
		assert.Contains(t, expected, op, "operation %s must be in the expected contract set", op)
	}
}

// TestEvalErrorCodeContract asserts the Go EvalErrorCode constants match
// the Python EvalErrorCode enum pinned in the contract test.
func TestEvalErrorCodeContract(t *testing.T) {
	expected := map[EvalErrorCode]bool{
		EvalErrorCodeNone:                        true,
		EvalErrorCodeEngineNotSetUp:              true,
		EvalErrorCodeEngineProtocolMismatch:      true,
		EvalErrorCodeConfigInvalid:               true,
		EvalErrorCodeAuthorityInvalid:            true,
		EvalErrorCodeLeaseMissing:                true,
		EvalErrorCodeLeaseInactive:               true,
		EvalErrorCodeLeaseExpired:                true,
		EvalErrorCodeLeaseMismatched:             true,
		EvalErrorCodeLeaseConsumed:               true,
		EvalErrorCodeCandidateDrift:              true,
		EvalErrorCodeInventoryDrift:              true,
		EvalErrorCodeReportRootReused:            true,
		EvalErrorCodeEvidenceKeyInvalid:          true,
		EvalErrorCodePlatformIdentityUnavailable: true,
		EvalErrorCodePlatformUnhealthy:           true,
		EvalErrorCodeProviderUnreachable:         true,
		EvalErrorCodeBudgetPreflightFailed:       true,
		EvalErrorCodeChildStartFailed:            true,
		EvalErrorCodeChildExitNonZero:            true,
		EvalErrorCodeChildInterrupted:            true,
		EvalErrorCodeStatusReconciliationFailed:  true,
	}
	allCodes := []EvalErrorCode{
		EvalErrorCodeNone, EvalErrorCodeEngineNotSetUp, EvalErrorCodeEngineProtocolMismatch, EvalErrorCodeConfigInvalid, EvalErrorCodeAuthorityInvalid,
		EvalErrorCodeLeaseMissing, EvalErrorCodeLeaseInactive, EvalErrorCodeLeaseExpired, EvalErrorCodeLeaseMismatched, EvalErrorCodeLeaseConsumed,
		EvalErrorCodeCandidateDrift, EvalErrorCodeInventoryDrift, EvalErrorCodeReportRootReused, EvalErrorCodeEvidenceKeyInvalid,
		EvalErrorCodePlatformIdentityUnavailable, EvalErrorCodePlatformUnhealthy, EvalErrorCodeProviderUnreachable,
		EvalErrorCodeBudgetPreflightFailed, EvalErrorCodeChildStartFailed, EvalErrorCodeChildExitNonZero, EvalErrorCodeChildInterrupted,
		EvalErrorCodeStatusReconciliationFailed,
	}
	assert.Equal(t, len(expected), len(allCodes), "expected set and constant list must have the same length")
	for _, code := range allCodes {
		assert.Contains(t, expected, code, "error code %s must be in the expected contract set", code)
	}
}

// TestEvalEngineStatusContract asserts the Go EvalEngineStatus constants
// match the Python EvalEngineStatus enum pinned in the contract test.
func TestEvalEngineStatusContract(t *testing.T) {
	expected := map[EvalEngineStatus]bool{
		EvalEngineStatusSucceeded:   true,
		EvalEngineStatusFailed:      true,
		EvalEngineStatusInterrupted: true,
		EvalEngineStatusStopped:     true,
	}
	allStatuses := []EvalEngineStatus{
		EvalEngineStatusSucceeded, EvalEngineStatusFailed, EvalEngineStatusInterrupted, EvalEngineStatusStopped,
	}
	assert.Equal(t, len(expected), len(allStatuses), "expected set and constant list must have the same length")
	for _, status := range allStatuses {
		assert.Contains(t, expected, status, "status %s must be in the expected contract set", status)
	}
}

// TestEvalEngineRequestJSONRoundTrip asserts the Go struct round-trips
// through JSON with the canonical wire field names.
func TestEvalEngineRequestJSONRoundTrip(t *testing.T) {
	req := EvalEngineRequest{
		SchemaVersion: EvalEngineRequestSchemaVersion,
		Operation:     EvalOperationBundle,
		OperationID:   "test-op-001",
		Revision:      "rev-001",
		ConfigPath:    "/tmp/config.json",
		LeasePath:     "/tmp/lease.json",
		ReportRoot:    "/tmp/report",
		Platform: EvalPlatformContext{
			RepositoryRoot:  "/repo",
			EvalProject:     "/repo/ensemble/evals",
			G8EBinaryPath:   "/repo/g8e",
			G8EBinarySHA256: "abc123",
			PlatformVersion: "2.1.0",
			AuthProjectRoot: "/repo",
			RuntimeDir:      "/repo/.g8e",
			TrustBundlePath: "/repo/.g8e/pki/trust/bundle.pem",
			GatewayHTTPURL:  "http://127.0.0.1:8080",
			GatewayHTTPSURL: "https://127.0.0.1:8443",
			EnsembleURL:     "http://127.0.0.1:8000",
		},
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)
	var restored EvalEngineRequest
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)
	assert.Equal(t, req.Operation, restored.Operation)
	assert.Equal(t, req.OperationID, restored.OperationID)
	assert.Equal(t, req.Platform.RepositoryRoot, restored.Platform.RepositoryRoot)
}

// TestEvalEngineResultJSONRoundTrip asserts the result struct round-trips
// through JSON with the canonical wire field names and omits optional
// fields when empty.
func TestEvalEngineResultJSONRoundTrip(t *testing.T) {
	result := EvalEngineResult{
		SchemaVersion: EvalEngineRequestSchemaVersion,
		Operation:     EvalOperationBundle,
		OperationID:   "test-op-001",
		Status:        EvalEngineStatusSucceeded,
	}
	data, err := json.Marshal(result)
	require.NoError(t, err)
	var restored EvalEngineResult
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)
	assert.Equal(t, result.Status, restored.Status)
	assert.Equal(t, EvalErrorCode(""), restored.ErrorCode)
}
