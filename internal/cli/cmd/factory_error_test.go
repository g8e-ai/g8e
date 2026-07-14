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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

var factoryErr = errors.New("factory boom")

// configLoaderFor returns a config loader that always returns the given cfg.
func configLoaderFor(cfg *config.Config) func(string) (*config.Config, error) {
	return func(string) (*config.Config, error) { return cfg, nil }
}

func TestApproveCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := approveCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(factoryErr))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, []string{"abc123"})
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, factoryErr)
}

func TestLogoutCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := logoutCmdWithConfig(configLoaderFor(cfg), failingFileSvcFactory(factoryErr))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, factoryErr)
}

func TestTUI_FileSvcFactoryError(t *testing.T) {
	cfg := setupTUITestConfig(t)

	deps := stubTUIDeps(t, cfg)
	deps.fileSvcFactory = failingFileSvcFactory(factoryErr)
	deps.loadCredentials = func(_ fs.RuntimeFileService, _ *config.Config) (*auth.Credentials, error) {
		panic("loadCredentials should not be called when fileSvcFactory fails")
	}

	cmd := tuiCmdWithDeps(deps)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, factoryErr)
}

func TestDataUsersCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataUsersCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(factoryErr))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, factoryErr)
}

func TestDataOperatorsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataOperatorsCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(factoryErr))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, factoryErr)
}

func TestDataSettingsCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataSettingsCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(factoryErr))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, factoryErr)
}

func TestDataStoreCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataStoreCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(factoryErr))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, factoryErr)
}

func TestDataAuditListCmdWithConfig_FileSvcFactoryError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	cmd := dataAuditListCmdWithConfig(configLoaderFor(cfg), panickingClientFactory(), failingFileSvcFactory(factoryErr))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrFileServiceInit)
	assert.ErrorIs(t, err, factoryErr)
}
