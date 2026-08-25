// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errMockNetwork = errors.New("network failure")

func setupAuditAPITestEnv(t *testing.T) *config.Config {
	t.Helper()
	fileSvc, cfg := newCmdTestEnv(t)

	creds := &auth.Credentials{
		OperatorSessionID: "op-sess-test",
		UserID:            "user-test",
		OperatorID:        "op-test",
		CLISessionID:      "cli-sess-test",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
	return cfg
}

// --- Receipts ---

func TestAuditReceiptsCmd_API_MockHappyPath(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getResp: []byte(`{"receipts":[{"transaction_hash":"abc123","action_type":"file_edit","target_resource":"/path/to/file","status":2,"executed_at":"2026-01-01T00:00:00Z","l2_valid":true,"l3_valid":false}]}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditReceiptsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "abc123")
	assert.Contains(t, buf.String(), "file_edit")
	assert.Contains(t, buf.String(), "L2 valid: 1")
}

func TestAuditReceiptsCmd_API_JSONOutput(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	rawJSON := `{"receipts":[]}`
	mockClient := &mockAPIClient{
		getResp: []byte(rawJSON),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditReceiptsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	cmd.Flags().Set("json", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), rawJSON)
}

func TestAuditReceiptsCmd_API_EmptyReceipts(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getResp: []byte(`{"receipts":[]}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditReceiptsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No receipts found")
}

func TestAuditReceiptsCmd_API_GetError(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getErr: errMockNetwork,
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditReceiptsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errMockNetwork)
}

func TestAuditReceiptsCmd_API_InvalidJSON(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getResp: []byte(`not json {{{`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditReceiptsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestAuditReceiptsCmd_API_ClientFactoryError(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) {
		return nil, constants.ErrNotAuthenticated
	}

	cmd := auditReceiptsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrNotAuthenticated)
}

// --- Export ---

func TestAuditExportCmd_API_MockHappyPath(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)
	tmpDir := testutil.TempDir(t)

	exportData := `{"receipts":[],"export_time":"2026-01-01T00:00:00Z"}`
	mockClient := &mockAPIClient{
		getResp: []byte(exportData),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditExportCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	outFile := tmpDir + "/export.json"
	cmd.Flags().Set("out", outFile)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Receipts export written")

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Equal(t, exportData, string(data))
}

func TestAuditExportCmd_API_GetError(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getErr: errMockNetwork,
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditExportCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errMockNetwork)
}

// --- Report ---

func TestAuditReportCmd_API_MockHappyPath(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)
	tmpDir := testutil.TempDir(t)

	reportJSON := `{"success":true,"report":{"generated_at":"2026-01-01T00:00:00Z","operator_session_id":"op-sess-test","events":[],"events_count":5,"receipts":[],"receipts_count":3,"total_records":8}}`
	mockClient := &mockAPIClient{
		getResp: []byte(reportJSON),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditReportCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	cmd.Flags().Set("out", tmpDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Compliance report written")
	assert.Contains(t, buf.String(), "Events:   5")
	assert.Contains(t, buf.String(), "Receipts: 3")
}

func TestAuditReportCmd_API_InvalidJSON(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)
	tmpDir := testutil.TempDir(t)

	mockClient := &mockAPIClient{
		getResp: []byte(`not json {{{`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditReportCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	cmd.Flags().Set("out", tmpDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestAuditReportCmd_API_GetError(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)
	tmpDir := testutil.TempDir(t)

	mockClient := &mockAPIClient{
		getErr: errMockNetwork,
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditReportCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	cmd.Flags().Set("out", tmpDir)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errMockNetwork)
}

// --- Events ---

func TestAuditEventsCmd_API_MockHappyPath(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	eventsJSON := `{"success":true,"events":[{"id":1,"operator_session_id":"op-sess-test","timestamp":"2026-01-01T00:00:00Z","type":"file_edit","command_raw":"echo hello","command_exit_code":0}],"count":1}`
	mockClient := &mockAPIClient{
		getResp: []byte(eventsJSON),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditEventsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "file_edit")
	assert.Contains(t, buf.String(), "echo hello")
	assert.Contains(t, buf.String(), "Total: 1 events")
}

func TestAuditEventsCmd_API_JSONOutput(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	rawJSON := `{"success":true,"events":[],"count":0}`
	mockClient := &mockAPIClient{
		getResp: []byte(rawJSON),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditEventsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	cmd.Flags().Set("json", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), rawJSON)
}

func TestAuditEventsCmd_API_EmptyEvents(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getResp: []byte(`{"success":true,"events":[],"count":0}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditEventsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No audit events found")
}

func TestAuditEventsCmd_API_GetError(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getErr: errMockNetwork,
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditEventsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errMockNetwork)
}

func TestAuditEventsCmd_API_InvalidJSON(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getResp: []byte(`not json {{{`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditEventsCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

// --- Summary ---

func TestAuditSummaryCmd_API_MockHappyPath(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	summaryJSON := `{"success":true,"events_summary":{"file_edit":5,"file_read":10},"events_total":15,"receipts_summary":{"approved":3,"rejected":1},"receipts_total":4,"total_records":19}`
	mockClient := &mockAPIClient{
		getResp: []byte(summaryJSON),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditSummaryCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Audit Summary")
	assert.Contains(t, buf.String(), "file_edit")
	assert.Contains(t, buf.String(), "Total records: 19")
}

func TestAuditSummaryCmd_API_EmptyRecords(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getResp: []byte(`{"success":true,"events_summary":{},"events_total":0,"receipts_summary":{},"receipts_total":0,"total_records":0}`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditSummaryCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No audit records found")
}

func TestAuditSummaryCmd_API_InvalidJSON(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getResp: []byte(`not json {{{`),
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditSummaryCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestAuditSummaryCmd_API_GetError(t *testing.T) {
	cfg := setupAuditAPITestEnv(t)

	mockClient := &mockAPIClient{
		getErr: errMockNetwork,
	}

	loader := func(string) (*config.Config, error) { return cfg, nil }
	factory := func(fs.RuntimeFileService, *config.Config) (apiClient, error) { return mockClient, nil }

	cmd := auditSummaryCmdWithConfig(loader, factory, newFileSvc)
	cmd.Flags().Set("session", "op-sess-test")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errMockNetwork)
}
