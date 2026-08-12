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
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

func TestEnrollCmdWithConfig_ConfigLoaderError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}

	cmd := enrollCmdWithConfig(failLoader, newFileSvc, auth.CheckOperatorRunning)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config load error")
}

func TestEnrollCmdWithConfig_OperatorNotRunningReturnsError(t *testing.T) {
	config.SetEndpointOverride("127.0.0.1:1")
	defer config.SetEndpointOverride("")

	_, cfg := newCmdTestEnv(t)

	loader := func(string) (*config.Config, error) {
		return cfg, nil
	}

	cmd := enrollCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestEnrollCmdWithConfig_NoTPMFlagOnNonWindows(t *testing.T) {
	cmd := enrollCmdWithConfig(loadConfig, newFileSvc, auth.CheckOperatorRunning)
	tpmFlag := cmd.Flags().Lookup("tpm")
	if tpmFlag != nil {
		assert.Equal(t, "false", tpmFlag.DefValue)
	}
}

func TestEnrollCmdWithConfig_HasRunE(t *testing.T) {
	cmd := enrollCmdWithConfig(loadConfig, newFileSvc, auth.CheckOperatorRunning)
	require.NotNil(t, cmd.RunE)
}

// TestEnrollCmdWithConfig_FlagsRegistered verifies the command registers the
// --no-system-trust and --rotate-cli flags with the correct defaults. The
// coordinator itself is exercised by internal/cli/auth tests; this asserts the
// command adapter exposes the new options.
func TestEnrollCmdWithConfig_FlagsRegistered(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)
	cmd := enrollCmdWithConfig(func(string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc), auth.CheckOperatorRunning)
	noSystemTrustFlag := cmd.Flags().Lookup("no-system-trust")
	require.NotNil(t, noSystemTrustFlag)
	assert.Equal(t, "false", noSystemTrustFlag.DefValue)
	rotateFlag := cmd.Flags().Lookup("rotate-cli")
	require.NotNil(t, rotateFlag)
	assert.Equal(t, "false", rotateFlag.DefValue)
}

func TestEnrollCmdWithConfig_UsesInjectedConfigLoader(t *testing.T) {
	called := false
	loader := func(string) (*config.Config, error) {
		called = true
		return nil, errors.New("injected error")
	}

	cmd := enrollCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning)
	_ = cmd.RunE(cmd, nil)

	assert.True(t, called, "config loader should have been called")
}

func TestEnrollCmdWithConfig_PropagatesConfigError(t *testing.T) {
	expectedErr := constants.ErrConfigLoadFailed
	loader := func(string) (*config.Config, error) {
		return nil, expectedErr
	}

	cmd := enrollCmdWithConfig(loader, newFileSvc, auth.CheckOperatorRunning)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

// TestEnrollCmdWithConfig_GatewayDownReturnsError verifies that when the
// coordinator factory is the production default and the gateway is
// unreachable, the command returns an error rather than silently succeeding.
// This replaces the old TestPerformEnroll_* tests that exercised the deleted
// performEnroll function.
func TestEnrollCmdWithConfig_GatewayDownReturnsError(t *testing.T) {
	config.SetEndpointOverride("127.0.0.1:1")
	defer config.SetEndpointOverride("")

	fileSvc, cfg := newCmdTestEnv(t)

	cmd := enrollCmdWithConfig(func(string) (*config.Config, error) {
		return cfg, nil
	}, fileSvcFactoryFor(fileSvc), auth.CheckOperatorRunning)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// The production coordinator factory will try to reach the gateway
	// (CheckBootstrapStatus) and fail because the endpoint is unreachable.
	// CheckOperatorRunning already fails before the coordinator is built,
	// so this asserts the preflight check fails closed.
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

// TestLogoutCmdWithConfig_RemovesLocalCredentials verifies logout routes
// through CredentialStore.Clear and removes the local CLI credential material
// (credentials JSON, CLI cert, CLI key) while leaving the OS root CA untouched
// (Clear never touches the OS trust store).
func TestLogoutCmdWithConfig_RemovesLocalCredentials(t *testing.T) {
	fileSvc, cfg := newCmdTestEnv(t)

	creds := &auth.Credentials{
		OperatorSessionID: "op-sess-123",
		UserID:            "user-456",
		OperatorID:        "op-789",
		CLISessionID:      "cli-sess-abc",
	}
	require.NoError(t, auth.SaveCredentials(fileSvc, cfg, creds))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("cli-cert"), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()), []byte("cli-key"), constants.PermFilePrivate))

	cmd := logoutCmdWithConfig(func(_ string) (*config.Config, error) { return cfg, nil }, fileSvcFactoryFor(fileSvc))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	ctx := context.Background()
	cmd.SetContext(ctx)

	require.NoError(t, cmd.RunE(cmd, nil))
	assert.Contains(t, buf.String(), "Logged out successfully")

	exists, err := fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CredentialsFile()))
	require.NoError(t, err)
	assert.False(t, exists)
	exists, err = fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()))
	require.NoError(t, err)
	assert.False(t, exists)
	exists, err = fileSvc.FileExists(context.Background(), mustRel(t, fileSvc, cfg.CLIKeyFile()))
	require.NoError(t, err)
	assert.False(t, exists)
}
