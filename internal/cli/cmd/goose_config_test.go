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
	"os"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGooseGovernanceConfig(t *testing.T) {
	tempHome := testutil.TempDir(t)
	t.Setenv("HOME", tempHome)

	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)
	require.NotEmpty(t, homeDir)

	configDir := filepath.Join(homeDir, ".config", "goose")
	err = os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	existingConfigPath := filepath.Join(configDir, "config.yaml")
	existingData := []byte("extensions:\n  existing:\n    enabled: true\n    config:\n      type: stdio\n      name: existing\n      cmd: existing-cmd\n")
	err = os.WriteFile(existingConfigPath, existingData, 0644)
	require.NoError(t, err)

	binaryPath, err := os.Executable()
	require.NoError(t, err)

	configPath, cleanup, err := WriteAgentConfig("goose", binaryPath)
	require.NoError(t, err)
	if cleanup != nil {
		defer cleanup()
	}
	assert.Equal(t, existingConfigPath, configPath)

	backupPath := configPath + ".bak"
	_, err = os.Stat(backupPath)
	require.NoError(t, err, "Backup file should exist")

	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	assert.Equal(t, existingData, backupData)

	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var cfg gooseConfig
	err = yaml.Unmarshal(configData, &cfg)
	require.NoError(t, err)

	assert.Contains(t, cfg.Extensions, "g8e")
	assert.True(t, cfg.Extensions["g8e"].Enabled)
	assert.Equal(t, "stdio", cfg.Extensions["g8e"].Config.Type)
	assert.Equal(t, "g8e", cfg.Extensions["g8e"].Config.Name)
	assert.Equal(t, binaryPath, cfg.Extensions["g8e"].Config.Cmd)
	assert.Equal(t, []string{"mcp", "stdio"}, cfg.Extensions["g8e"].Config.Args)
}

func TestGooseLaunchArgs_NoProfile(t *testing.T) {
	args, err := agentLaunchArgs("goose", "/tmp/mcp-config.json", "/fake/g8e")
	require.NoError(t, err)
	assert.Contains(t, args, "session")
	assert.Contains(t, args, "--no-profile")
	assert.Contains(t, args, "--with-extension")
}
