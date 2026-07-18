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

func TestPerformEnroll_NoLocalCredsGatewayDownReturnsError(t *testing.T) {
	config.SetEndpointOverride("127.0.0.1:1")
	defer config.SetEndpointOverride("")

	fileSvc, cfg := newCmdTestEnv(t)

	cmd := enrollCmdWithConfig(func(string) (*config.Config, error) {
		return cfg, nil
	}, newFileSvc, auth.CheckOperatorRunning)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := performEnroll(cmd, fileSvc, cfg, false)
	require.Error(t, err)
}

func TestPerformEnroll_WithLocalCredsGatewayDownReturnsError(t *testing.T) {
	config.SetEndpointOverride("127.0.0.1:1")
	defer config.SetEndpointOverride("")

	fileSvc, cfg := newCmdTestEnv(t)

	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CredentialsFile()), []byte(`{"user_id":"u1","cli_session_id":"s1"}`), constants.PermFilePrivate))
	require.NoError(t, fileSvc.WriteFile(context.Background(), mustRel(t, fileSvc, cfg.CLICertFile()), []byte("-----BEGIN CERTIFICATE-----\nMIIBdummy==\n-----END CERTIFICATE-----\n"), constants.PermFilePrivate))

	cmd := enrollCmdWithConfig(func(string) (*config.Config, error) {
		return cfg, nil
	}, newFileSvc, auth.CheckOperatorRunning)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := performEnroll(cmd, fileSvc, cfg, false)
	require.Error(t, err)
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
