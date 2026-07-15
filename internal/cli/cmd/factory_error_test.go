// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"net"
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
	cmd := gatewayCleanCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	cmd.Flags().Set("force", "true")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
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
	// Start a dummy TCP listener so CheckOperatorRunning does not fail before fileSvcFactory is called
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", constants.Ports.OperatorHttp))
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()
	config.SetEndpointOverride(fmt.Sprintf("127.0.0.1:%d", constants.Ports.OperatorHttp))
	defer config.SetEndpointOverride("")
	cmd := enrollCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err = cmd.RunE(cmd, nil)
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
	cmd := securityPKIEnrollCmdWithConfig(configLoaderFor(cfg), panickingEnrollFunc(), failingFileSvcFactory(errFactory))
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
	cmd := agentRunCmdWithConfig(failingFileSvcFactory(errFactory))
	err := cmd.RunE(cmd, []string{"claude"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// panickingEnrollFunc returns an enrollFunc that panics if called.
func panickingEnrollFunc() enrollFunc {
	return func(*config.Config, string, string, string, string) (*auth.RegistrationResponse, error) {
		panic("enroll should not be called when fileSvcFactory fails")
	}
}
