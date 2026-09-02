// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

func TestHarnessResult_ParsesAuthoritativeReceiptProjection(t *testing.T) {
	raw := `[{"name":"fedramp-provision","title":"FedRAMP Provision","persona":"claude-desktop","requires_posture":"consensus","started_at":"2026-09-01T16:53:31Z","duration_ms":1234,"run_id":"fedramp-run-123","scenario_id":"fedramp-provision","attempt_ids":["attempt-1"],"execution_ids":["execution-abc-123"],"transaction_ids":["tx-abc-123"],"investigation_ids":["investigation-abc-123"],"receipts":[{"execution_id":"execution-abc-123","transaction_id":"tx-abc-123","transaction_hash":"hash-abc","signer_key_id":"warden-key-1","signature":"sig-abc","investigation_id":"investigation-abc-123"}],"notes":["admitted"],"ok":true}]`

	var results []harnessResult
	err := json.Unmarshal([]byte(raw), &results)
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "fedramp-provision", r.Name)
	assert.Equal(t, "fedramp-run-123", r.RunID)
	assert.Equal(t, "fedramp-provision", r.ScenarioID)
	assert.True(t, r.OK)
	assert.Equal(t, []string{"attempt-1"}, r.AttemptIDs)
	assert.Equal(t, []string{"execution-abc-123"}, r.ExecutionIDs)
	assert.Equal(t, []string{"tx-abc-123"}, r.TransactionIDs)
	assert.Equal(t, []string{"investigation-abc-123"}, r.InvestigationIDs)
	require.Len(t, r.Receipts, 1)
	assert.Equal(t, "execution-abc-123", r.Receipts[0].ExecutionID)
	assert.Equal(t, "tx-abc-123", r.Receipts[0].TransactionID)
	assert.Equal(t, "hash-abc", r.Receipts[0].TransactionHash)
	assert.Equal(t, "warden-key-1", r.Receipts[0].SignerKeyID)
	assert.Equal(t, "sig-abc", r.Receipts[0].Signature)
	assert.Equal(t, "investigation-abc-123", r.Receipts[0].InvestigationID)
}

func TestApplyHarnessAuthoritativeIdentity_RetainsAttemptExecutionTransactionInvestigationAndReceiptReferences(t *testing.T) {
	result := &compliancev1.DemoScenarioResult{}
	var parsed []harnessResult
	require.NoError(t, json.Unmarshal([]byte(`[{"attempt_ids":["attempt-1"],"execution_ids":["execution-1"],"transaction_ids":["tx-1"],"investigation_ids":["investigation-1"],"receipts":[{"execution_id":"execution-1","transaction_id":"tx-1","investigation_id":"investigation-1"}]}]`), &parsed))
	require.Len(t, parsed, 1)

	applied := applyHarnessAuthoritativeIdentity(result, &parsed[0])

	assert.True(t, applied)
	assert.Equal(t, []string{"attempt-1"}, result.GetAttemptIds())
	assert.Equal(t, []string{"execution-1"}, result.GetExecutionIds())
	assert.Equal(t, []string{"tx-1"}, result.GetTransactionIds())
	assert.Equal(t, []string{"investigation-1"}, result.GetInvestigationIds())
	assert.Equal(t, []string{"action-receipt:tx-1"}, result.GetReceiptRefs())
}

func TestApplyHarnessAuthoritativeIdentity_FailsClosedWhenInvestigationIDsMissing(t *testing.T) {
	result := &compliancev1.DemoScenarioResult{}
	var parsed []harnessResult
	require.NoError(t, json.Unmarshal([]byte(`[{"attempt_ids":["attempt-1"],"execution_ids":["execution-1"],"transaction_ids":["tx-1"],"receipts":[{"execution_id":"execution-1","transaction_id":"tx-1"}]}]`), &parsed))
	require.Len(t, parsed, 1)

	applied := applyHarnessAuthoritativeIdentity(result, &parsed[0])

	assert.False(t, applied, "missing investigation IDs must fail closed")
	assert.Empty(t, result.GetInvestigationIds())
}

func TestHarnessResult_ParsesBlockedScenarioWithAuthoritativeIdentity(t *testing.T) {
	raw := `[{"name":"fedramp-deny","title":"FedRAMP Deny","persona":"claude-desktop","requires_posture":"doctrine","started_at":"2026-09-01T16:53:31Z","duration_ms":500,"run_id":"fedramp-run-123","scenario_id":"fedramp-deny","attempt_ids":["attempt-1"],"execution_ids":["execution-deny-456"],"transaction_ids":["tx-deny-456"],"investigation_ids":["investigation-deny-456"],"receipts":[{"execution_id":"execution-deny-456","transaction_id":"tx-deny-456","transaction_hash":"hash-deny","signer_key_id":"warden-key-1","signature":"sig-deny","investigation_id":"investigation-deny-456"}],"ok":true}]`

	var results []harnessResult
	err := json.Unmarshal([]byte(raw), &results)
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.True(t, r.OK)
	assert.Equal(t, []string{"execution-deny-456"}, r.ExecutionIDs)
	assert.Equal(t, []string{"tx-deny-456"}, r.TransactionIDs)
	assert.Equal(t, []string{"investigation-deny-456"}, r.InvestigationIDs)
	require.Len(t, r.Receipts, 1)
	assert.Equal(t, "execution-deny-456", r.Receipts[0].ExecutionID)
	assert.Equal(t, "tx-deny-456", r.Receipts[0].TransactionID)
	assert.Equal(t, "warden-key-1", r.Receipts[0].SignerKeyID)
	assert.Equal(t, "investigation-deny-456", r.Receipts[0].InvestigationID)
}

func TestHarnessResult_ParsesEmptyReceiptsArray(t *testing.T) {
	raw := `[{"name":"mcp-plain","title":"MCP Plain","persona":"claude-desktop","requires_posture":"doctrine","started_at":"2026-09-01T16:53:31Z","duration_ms":100,"ok":true}]`

	var results []harnessResult
	err := json.Unmarshal([]byte(raw), &results)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].TransactionIDs)
	assert.Empty(t, results[0].Receipts)
}

func TestHarnessResult_ParsesMultipleResults(t *testing.T) {
	raw := `[
		{"name":"dhs-destruction-block","ok":true,"attempt_ids":["attempt-1"],"execution_ids":["execution-block"],"transaction_ids":["tx-block"],"investigation_ids":["investigation-block"],"receipts":[{"execution_id":"execution-block","transaction_id":"tx-block","transaction_hash":"h1","signer_key_id":"wk1","signature":"s1","investigation_id":"investigation-block"}]},
		{"name":"dhs-destruction-purge","ok":true,"attempt_ids":["attempt-1"],"execution_ids":["execution-purge"],"transaction_ids":["tx-purge"],"investigation_ids":["investigation-purge"],"receipts":[{"execution_id":"execution-purge","transaction_id":"tx-purge","transaction_hash":"h2","signer_key_id":"wk1","signature":"s2","investigation_id":"investigation-purge"}]}
	]`

	var results []harnessResult
	err := json.Unmarshal([]byte(raw), &results)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "tx-block", results[0].TransactionIDs[0])
	assert.Equal(t, "investigation-block", results[0].InvestigationIDs[0])
	assert.Equal(t, "tx-purge", results[1].TransactionIDs[0])
	assert.Equal(t, "investigation-purge", results[1].InvestigationIDs[0])
}

func TestHarnessResult_FailsClosedOnMalformedJSON(t *testing.T) {
	raw := `{not valid json`
	var results []harnessResult
	err := json.Unmarshal([]byte(raw), &results)
	require.Error(t, err)
}

func writeFakeHarnessScript(t *testing.T, dir, payload string, exitCode int) string {
	t.Helper()
	scriptPath := filepath.Join(dir, "fake-harness.sh")
	content := fmt.Sprintf("#!/bin/sh\ncat <<'JSONEOF'\n%s\nJSONEOF\nexit %d\n", payload, exitCode)
	require.NoError(t, os.WriteFile(scriptPath, []byte(content), 0o755))
	return scriptPath
}

func TestRunHarnessWithJSON_ParsesValidOutput(t *testing.T) {
	tmpDir := t.TempDir()
	payload := `[{"name":"fedramp-provision","title":"FedRAMP Provision","persona":"test","requires_posture":"consensus","started_at":"2026-09-01T16:53:31Z","duration_ms":100,"run_id":"run-1","scenario_id":"fedramp-provision","attempt_ids":["attempt-1"],"execution_ids":["execution-1"],"transaction_ids":["tx-1"],"investigation_ids":["investigation-1"],"receipts":[{"execution_id":"execution-1","transaction_id":"tx-1","transaction_hash":"h1","signer_key_id":"wk1","signature":"s1","investigation_id":"investigation-1"}],"ok":true}]`
	scriptPath := writeFakeHarnessScript(t, tmpDir, payload, 0)

	results, err := runHarnessWithJSON(context.Background(), tmpDir, "test harness", []string{"sh", scriptPath})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "tx-1", results[0].TransactionIDs[0])
	assert.Equal(t, "investigation-1", results[0].InvestigationIDs[0])
	assert.Equal(t, "wk1", results[0].Receipts[0].SignerKeyID)
	assert.Equal(t, "investigation-1", results[0].Receipts[0].InvestigationID)
}

func TestRunHarnessWithJSON_ParsesOutputEvenOnNonzeroExit(t *testing.T) {
	tmpDir := t.TempDir()
	payload := `[{"name":"fedramp-deny","title":"FedRAMP Deny","persona":"test","requires_posture":"doctrine","started_at":"2026-09-01T16:53:31Z","duration_ms":50,"run_id":"run-1","scenario_id":"fedramp-deny","attempt_ids":["attempt-1"],"execution_ids":["execution-deny"],"transaction_ids":["tx-deny"],"investigation_ids":["investigation-deny"],"receipts":[{"execution_id":"execution-deny","transaction_id":"tx-deny","transaction_hash":"hd","signer_key_id":"wk1","signature":"sd","investigation_id":"investigation-deny"}],"ok":true}]`
	scriptPath := writeFakeHarnessScript(t, tmpDir, payload, 1)

	results, err := runHarnessWithJSON(context.Background(), tmpDir, "test harness", []string{"sh", scriptPath})
	require.Error(t, err, "nonzero exit should be returned as the command error")
	require.Len(t, results, 1, "results should still be parsed from stdout")
	assert.Equal(t, "tx-deny", results[0].TransactionIDs[0])
	assert.Equal(t, "investigation-deny", results[0].InvestigationIDs[0])
}

func TestRunHarnessWithJSON_FailsClosedOnEmptyOutput(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := writeFakeHarnessScript(t, tmpDir, "", 0)

	_, err := runHarnessWithJSON(context.Background(), tmpDir, "test harness", []string{"sh", scriptPath})
	require.Error(t, err)
}

func TestRunHarnessWithJSON_FailsClosedOnMalformedOutput(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := writeFakeHarnessScript(t, tmpDir, `{not valid json`, 0)

	_, err := runHarnessWithJSON(context.Background(), tmpDir, "test harness", []string{"sh", scriptPath})
	require.Error(t, err)
}
