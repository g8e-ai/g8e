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
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

func TestEnrollCmdWithConfig_ConfigLoaderError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}

	cmd := enrollCmdWithConfig(failLoader)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config load error")
}

func TestEnrollCmdWithConfig_OperatorNotRunningReturnsError(t *testing.T) {
	tmpDir := chdirTemp(t)
	cfg := setupDataTestConfig(t, tmpDir)

	loader := func(string) (*config.Config, error) {
		return cfg, nil
	}

	cmd := enrollCmdWithConfig(loader)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}

func TestEnrollCmdWithConfig_NoTPMFlagOnNonWindows(t *testing.T) {
	cmd := enrollCmdWithConfig(loadConfig)
	tpmFlag := cmd.Flags().Lookup("tpm")
	if tpmFlag != nil {
		assert.Equal(t, "false", tpmFlag.DefValue)
	}
}

func TestEnrollCmdWithConfig_HasRunE(t *testing.T) {
	cmd := enrollCmdWithConfig(loadConfig)
	require.NotNil(t, cmd.RunE)
}

func TestPerformEnroll_NoLocalCredsGatewayDownReturnsError(t *testing.T) {
	tmpDir := chdirTemp(t)
	cfg := setupDataTestConfig(t, tmpDir)

	cmd := enrollCmdWithConfig(func(string) (*config.Config, error) {
		return cfg, nil
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := performEnroll(cmd, cfg, false)
	require.Error(t, err)
}

func TestPerformEnroll_WithLocalCredsGatewayDownReturnsError(t *testing.T) {
	tmpDir := chdirTemp(t)
	cfg := setupDataTestConfig(t, tmpDir)

	require.NoError(t, os.WriteFile(cfg.CredentialsFile(), []byte(`{"user_id":"u1","cli_session_id":"s1"}`), 0o600))
	require.NoError(t, os.WriteFile(cfg.CLICertFile(), []byte("-----BEGIN CERTIFICATE-----\nMIIBdummy==\n-----END CERTIFICATE-----\n"), 0o600))

	cmd := enrollCmdWithConfig(func(string) (*config.Config, error) {
		return cfg, nil
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := performEnroll(cmd, cfg, false)
	require.Error(t, err)
}

func TestEnrollCmdWithConfig_UsesInjectedConfigLoader(t *testing.T) {
	called := false
	loader := func(string) (*config.Config, error) {
		called = true
		return nil, errors.New("injected error")
	}

	cmd := enrollCmdWithConfig(loader)
	_ = cmd.RunE(cmd, nil)

	assert.True(t, called, "config loader should have been called")
}

func TestEnrollCmdWithConfig_PropagatesConfigError(t *testing.T) {
	expectedErr := constants.ErrConfigLoadFailed
	loader := func(string) (*config.Config, error) {
		return nil, expectedErr
	}

	cmd := enrollCmdWithConfig(loader)
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
}
