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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestGatewaySettingsCmdWithConfig_ConfigLoadError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}

	cmd := gatewaySettingsCmdWithConfig(failLoader, defaultAPIClientFactory, newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}

func TestGatewaySettingsCmdWithConfig_ClientCreationError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := gatewaySettingsCmdWithConfig(loader, failingClientFactory(errors.New("client creation error")), newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInternal)
}

func TestGatewaySettingsCmdWithConfig_GetRequestError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getErr: errors.New("network error")}
	cmd := gatewaySettingsCmdWithConfig(loader, mockClientFactory(client), newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed)
}

func TestGatewaySettingsCmdWithConfig_ValidResponsePrintsSettings(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	settingsJSON := []byte(`{"posture":"consensus","port":8443,"log_level":"info"}`)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getResp: settingsJSON}
	cmd := gatewaySettingsCmdWithConfig(loader, mockClientFactory(client), newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "posture")
	assert.Contains(t, output, "consensus")
	assert.Equal(t, []string{"/api/settings"}, client.getCalls)
}
