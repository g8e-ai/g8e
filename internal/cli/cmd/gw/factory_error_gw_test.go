// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/frontendverify"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

var errFactory = fmt.Errorf("factory boom")

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

func TestDataUsersCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := dataUsersCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataOperatorsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := dataOperatorsCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataSettingsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := dataSettingsCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataStoreCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := dataStoreCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestDataAuditListCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

	cmd := dataAuditListCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayStartCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := gatewayStartCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory), nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayStopCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := gatewayStopCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayStatusCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := gatewayStatusCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayRestartCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := gatewayRestartCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayLogsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := gatewayLogsCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewaySettingsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := gatewaySettingsCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), authcmd.PanickingClientFactory(), cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

func TestGatewayCleanCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := gatewayCleanCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory), defaultTrustInstallerFactory)
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
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	cmd := gatewayCleanCmdWithConfig(
		cmdtest.ConfigLoaderFor(cfg),
		cmdtest.FileSvcFactoryFor(fileSvc),
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

func TestSecurityValidateCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	cmd := securityValidateCmdWithConfig(cmdtest.FailingFileSvcFactory(errFactory))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}

// TestGatewayConnectCmdWithConfig_FileSvcFactoryError verifies that a
// fileSvcFactory failure is surfaced as ErrFileServiceInit with the original
// factory error preserved via errors.Is. The command must fail before reaching
// any downstream dependency (ProcessManager, trust installer, verifier).
func TestGatewayConnectCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := cmdtest.NewCmdTestEnv(t)

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

	cmd := gatewayConnectCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FailingFileSvcFactory(errFactory), deps)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"https://your-app.lovable.app"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, errFactory)
}
