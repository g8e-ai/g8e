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
	"crypto/x509"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/frontendverify"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/compliance/evidence"
	compliancereport "github.com/g8e-ai/g8e/v2/internal/services/compliance/report"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	harnessclient "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/client"
	harnessconfig "github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/config"
)

var errFactory = errors.New("factory boom")

// panickingTrustInstaller is a mock auth.SystemTrustInstaller that panics on
// every method call. Used in factory-error tests to prove that downstream
// dependencies are not reached when fileSvcFactory fails.
type panickingTrustInstaller struct{}

func (p *panickingTrustInstaller) IsTrusted(context.Context, string) (bool, error) {
	panic("trustInstaller.IsTrusted should not be called when fileSvcFactory fails")
}

func (p *panickingTrustInstaller) InstallRoot(context.Context, *x509.Certificate, string) error {
	panic("trustInstaller.InstallRoot should not be called when fileSvcFactory fails")
}

func (p *panickingTrustInstaller) ListStaleAnchors(context.Context, string) ([]platform.StaleAnchor, error) {
	panic("trustInstaller.ListStaleAnchors should not be called when fileSvcFactory fails")
}

func (p *panickingTrustInstaller) RemoveStaleAnchors(context.Context, []platform.StaleAnchor) error {
	panic("trustInstaller.RemoveStaleAnchors should not be called when fileSvcFactory fails")
}

// configLoaderFor returns a config loader that always returns the given cfg.
func configLoaderFor(cfg *config.Config) func(string) (*config.Config, error) {
	return func(string) (*config.Config, error) { return cfg, nil }
}

func TestPublicInitCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := publicInitCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	cmd.SetArgs([]string{"--source-id", "deployment-a", "--mirror-origin", "https://mirror.example"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicConfigSetCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := publicConfigSetCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	cmd.SetArgs([]string{"--mirror-origin", "https://mirror.example"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicPublishCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := publicPublishCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	cmd.SetArgs([]string{constants.TestPublicFeedRecordsFilename})

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicPushCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := publicPushCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicStatusCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := publicStatusCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPublicRotateKeyCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	cmd := publicRotateKeyCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))

	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
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

func TestApprovePlatformEnrollmentCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := approvePlatformEnrollmentCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDenyPlatformEnrollmentCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := denyPlatformEnrollmentCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"req-001"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestPendingPlatformEnrollmentCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := pendingPlatformEnrollmentCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
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

func TestAuthContextCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := authContextCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(errFactory))
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

func TestEnrollUserCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	stubCheckOperatorRunning := func(*config.Config) error { return nil }
	cmd := enrollUserCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), stubCheckOperatorRunning, newDefaultEnrollmentCoordinator)
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

func TestComplianceDemoRunVerifyCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceDemoRunVerifyCmdWithConfig(
		failingFileSvcFactory(errFactory),
		func(string) evidence.ProvenanceSource {
			panic("provenance source should not be created when fileSvcFactory fails")
		},
	)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, []string{"any-run"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestComplianceEvidenceGraphVerifyCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceEvidenceGraphVerifyCmdWithConfig(
		failingFileSvcFactory(errFactory),
		func(string) evidence.ProvenanceSource {
			panic("provenance source should not be created when fileSvcFactory fails")
		},
	)
	require.NoError(t, cmd.Flags().Set("demo-run", "any-run"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestComplianceReportGenerateCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceReportGenerateCmdWithConfig(
		failingFileSvcFactory(errFactory),
		func(string) evidence.ProvenanceSource {
			panic("provenance source should not be created when fileSvcFactory fails")
		},
		func(context.Context, string, string) (*compliancereport.ComplianceReportSigningIdentity, error) {
			panic("signing identity should not be loaded when fileSvcFactory fails")
		},
	)
	require.NoError(t, cmd.Flags().Set("scope-id", "scope-1"))
	require.NoError(t, cmd.Flags().Set("window-start-unix-ms", "1700000000000"))
	require.NoError(t, cmd.Flags().Set("window-end-unix-ms", "1700000001000"))
	require.NoError(t, cmd.Flags().Set("demo-run", "any-run"))
	require.NoError(t, cmd.Flags().Set("report-id", "report-1"))
	require.NoError(t, cmd.Flags().Set("signing-metadata", constants.ComplianceReportSigningMetadataTestFilename))
	require.NoError(t, cmd.Flags().Set("signing-private-key", constants.ComplianceReportSigningPrivateKeyTestFilename))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestComplianceReleaseEvidenceCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := complianceReleaseEvidenceCmdWithConfig(
		failingFileSvcFactory(errFactory),
		func(string) evidence.ProvenanceSource {
			panic("provenance source should not be created when fileSvcFactory fails")
		},
	)
	require.NoError(t, cmd.Flags().Set("version", "v2.1.3"))
	require.NoError(t, cmd.Flags().Set("out", t.TempDir()))
	require.NoError(t, cmd.Flags().Set("scope-id", "test-scope"))
	require.NoError(t, cmd.Flags().Set("run-id", "test-run"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDemosRunCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := demosRunCmdWithConfig(failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{constants.DemosOrgFedRAMP, "2"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Docker start command (interactive walkthrough) ---

func TestDockerStartCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	writeRootCompose(t)

	stubCheckOperatorRunning := func(*config.Config) error { return nil }
	cmd := dockerStartCmdWithConfig(
		configLoaderFor(cfg),
		failingFileSvcFactory(errFactory),
		panickingClientFactory(),
		stubCheckOperatorRunning,
		panickingEnrollerFactory(),
	)
	cmd.Flags().Set("full", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Gateway connect command (Phase 4) ---

// TestGatewayConnectCmdWithConfig_FileSvcFactoryError verifies that a
// fileSvcFactory failure is surfaced as ErrFileServiceInit with the original
// factory error preserved via errors.Is. The command must fail before reaching
// any downstream dependency (ProcessManager, trust installer, verifier).
func TestGatewayConnectCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	deps := connectDeps{
		trustInstaller: &panickingTrustInstaller{},
		discoveryFetcher: func(context.Context, string, func() time.Time) (auth.TrustDiscoveryResult, error) {
			panic("discoveryFetcher should not be called when fileSvcFactory fails")
		},
		verifier: frontendverify.NewVerifier(frontendverify.VerifierDeps{
			HTTPClientFactory: func(*x509.CertPool, time.Duration) (*http.Client, error) {
				panic("verifier should not be called when fileSvcFactory fails")
			},
		}),
		browserOpener: func(string) error {
			panic("browserOpener should not be called when fileSvcFactory fails")
		},
	}

	cmd := gatewayConnectCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory), deps)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// --- Eval commands ---

func TestEvalCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)
	deps := nativeEvalDeps{
		configLoader:   configLoaderFor(cfg),
		fileSvcFactory: failingFileSvcFactory(errFactory),
		clientFactory: func(harnessconfig.Config) (*harnessclient.Client, error) {
			panic("client factory should not be called when fileSvcFactory fails")
		},
		authLoader: func(fs.RuntimeFileService, *config.Config) (*auth.ClientAuthContext, error) {
			panic("auth loader should not be called when fileSvcFactory fails")
		},
		now:   time.Now,
		newID: func() string { return "run-id" },
	}
	tests := []struct {
		name string
		args []string
	}{
		{name: "run", args: []string{"run", evaluation.CoreExecutionBoundarySuiteID}},
		{name: "verify", args: []string{"verify", "run-id"}},
		{name: "show", args: []string{"show", "run-id"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := evalCmdWithConfig(deps)
			cmd.SetArgs(test.args)
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			err := cmd.Execute()
			require.Error(t, err)
			assert.ErrorIs(t, err, constants.ErrFileServiceInit)
			assert.ErrorIs(t, err, errFactory)
		})
	}
}
