// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

var errFactory = errors.New("factory boom")

// configLoaderFor returns a config loader that always returns the given cfg.
func configLoaderFor(cfg *config.Config) func(string) (*config.Config, error) {
	return func(string) (*config.Config, error) { return cfg, nil }
}

func TestApproveCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := approveCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"abc123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestApproveRecoveryCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := approveRecoveryCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"abc123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestLogoutCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := logoutCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestTUI_FileSvcFactoryError(t *testing.T) {
	cfg := setupTUITestConfig(t)

	deps := stubTUIDeps(t, cfg)
	deps.fileSvcFactory = failingFileSvcFactory(errFactory)
	deps.loadCredentials = func(_ fs.RuntimeFileService, _ *config.Config) (*auth.Credentials, error) {
		panic("loadCredentials should not be called when fileSvcFactory fails")
	}

	cmd := tuiCmdWithDeps(deps)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataUsersCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataUsersCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataOperatorsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataOperatorsCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataSettingsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataSettingsCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataStoreCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataStoreCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataAuditListCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataAuditListCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Audit commands (session 17) ---

func TestAuditReceiptsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := auditReceiptsCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestAuditExportCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := auditExportCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestAuditReportCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := auditReportCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestAuditEventsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := auditEventsCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestAuditSummaryCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := auditSummaryCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Gateway commands (session 17) ---

func TestGatewayStartCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := gatewayStartCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayStopCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := gatewayStopCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayStatusCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := gatewayStatusCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayRestartCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := gatewayRestartCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayLogsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := gatewayLogsCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewaySettingsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := gatewaySettingsCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayCleanCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := gatewayCleanCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), defaultTrustInstallerFactory)
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestGatewayCleanCmdWithConfig_TrustInstallerFactoryError verifies that a
// trustInstallerFactory failure is surfaced as a warning (best-effort OS
// cleanup) and does NOT abort the runtime wipe — the runtime wipe is the
// destructive primary action. The fileSvc factory succeeds so pm.Clean runs.
func TestGatewayCleanCmdWithConfig_TrustInstallerFactoryError(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	cmd := gatewayCleanCmdWithConfig(
		configLoaderFor(cfg),
		fileSvcFactoryFor(fileSvc),
		func() (systemTrustCleaner, error) {
			return nil, errFactory
		},
	)
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.NoError(t, err, "trust factory failure is best-effort and must not abort the runtime wipe")
	assert.Contains(t, buf.String(), "could not initialize OS trust cleaner")
	assert.Contains(t, buf.String(), "Clean complete")
}

// --- Vault commands (session 17) ---

func TestVaultInitCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultInitCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultUnlockCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultUnlockCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultRekeyCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultRekeyCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultStatusCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultStatusCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultResetCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultResetCmdWithConfig(failingFileSvcFactory(errFactory))
	cmd.Flags().Set("confirm", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultExportCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultExportCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestVaultImportCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := vaultImportCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Auth/enroll command (session 17) ---

func TestEnrollCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	stubCheckOperatorRunning := func(*config.Config) error { return nil }
	cmd := enrollCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), stubCheckOperatorRunning, newDefaultEnrollmentCoordinator)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Operator commands (session 18) ---

func TestOperatorListCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := operatorListCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestOperatorDeployCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := operatorDeployCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Security commands (session 18) ---

func TestSecurityValidateCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := securityValidateCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestSecurityPKIEnrollCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := securityPKIEnrollCmdWithConfig(configLoaderFor(cfg), panickingRemoteOperatorEnrollerFactory(), failingFileSvcFactory(errFactory))
	cmd.Flags().StringP("endpoint", "e", "localhost:8080", "Gateway endpoint")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Test command (session 18) ---

func TestTestE2ECmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	config.SetEndpointOverride("127.0.0.1:1")
	defer config.SetEndpointOverride("")
	cmd := testE2ECmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- MCP commands (session 18) ---

func TestMcpStdioCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	origConfigLoad := configLoad
	configLoad = func(string) (*config.Config, error) { return cfg, nil }
	t.Cleanup(func() { configLoad = origConfigLoad })
	cmd := mcpStdioCmdWithConfig(failingFileSvcFactory(errFactory))
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestAgentRunCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	origConfigLoad := configLoad
	configLoad = func(string) (*config.Config, error) { return cfg, nil }
	t.Cleanup(func() { configLoad = origConfigLoad })
	cmd := agentRunCmdWithConfig(failingFileSvcFactory(errFactory), panickingEnrollerFactory())
	err := cmd.RunE(cmd, []string{"claude"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Compliance commands (session 19) ---

func TestComplianceKSIHistoryCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceKSIHistoryCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestComplianceOverlayCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceOverlayCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// panickingRemoteOperatorEnrollerFactory returns a remoteOperatorEnroller
// factory whose enroller panics if called. Used to assert that enrollment is
// not attempted when an earlier dependency (e.g. fileSvcFactory) fails.
func panickingRemoteOperatorEnrollerFactory() func(*config.Config) remoteOperatorEnroller {
	return func(*config.Config) remoteOperatorEnroller {
		return &panickingRemoteOperatorEnroller{}
	}
}

type panickingRemoteOperatorEnroller struct{}

func (p *panickingRemoteOperatorEnroller) EnrollRemoteOperator(_ context.Context, _, _ string, _ *ecdsa.PrivateKey, _ string, _ *ecdsa.PrivateKey, _ string) (auth.EnrollmentArtifacts, error) {
	panic("enroll should not be called when fileSvcFactory fails")
}
